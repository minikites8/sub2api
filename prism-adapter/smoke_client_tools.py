"""Real HTTP + Chromium + simulated Prism, three-turn client tool exchange.

No OAuth credentials or production requests. The fixture client only returns
synthetic strings; neither the gateway nor the fixture runs model-authored code.
"""
import argparse
import json
import re
import tempfile
import threading
from http.server import ThreadingHTTPServer
from urllib.request import Request, urlopen

import server as api
from multiplex_browser import MultiplexBrowser
from multiplex_runtime import AsyncBrowserWorker
from smoke_multiplex import Fixture
from tool_state import ToolState
from catalog_prompt import MAX_PRISM_PROMPT_BYTES
from relay_prompt import TURN_BYTES, transport_bytes


def client_catalog(size=512, unique=False):
    """A Codex-sized catalog, with the exercised tools at the end."""
    if size < 2:
        raise ValueError('catalog requires the two exercised client tools')
    tools = []
    for offset in range(0, size - 2, 32):
        tools.append({'type': 'namespace', 'name': f'connector_{offset // 32}', 'tools': [
            {'type': 'function', 'name': f'operation_{index}',
             'description': (f'Codex connector fixture {index}: preserve the complete client tool description. ' if unique
                             else 'Codex connector fixture: preserve the complete client tool description. ') * 8,
             'parameters': {'type': 'object', 'properties': {'key': {'type': 'string'}},
                            'required': ['key'], 'additionalProperties': False}, 'strict': True}
            for index in range(offset, min(offset + 32, size - 2))]})
    tools.append({'type': 'namespace', 'name': 'client', 'tools': [
        {'type': 'function', 'name': 'lookup', 'parameters': {'type': 'object',
            'properties': {'key': {'type': 'string'}}, 'required': ['key'], 'additionalProperties': False}},
        {'type': 'custom', 'name': 'echo', 'format': {
            'type': 'grammar', 'syntax': 'regex', 'definition': 'fixture-value'}}]})
    return tools


class ToolFixture(Fixture):
    def reply(self, code, body, content_type='application/json', cache=False, retry_after=None):
        if content_type == 'application/json':
            data = json.loads(body)
            if data.get('status') == 'completed':
                if data['response']['status'] == 'error':
                    super().reply(code, body, content_type, cache, retry_after)
                    return
                prompt = data['response']['payload']['output'][0]['content'][0]['text']
                part = re.match(r'\[PRISM_TRANSPORT_PART (\d+)/(\d+)\]', prompt)
                if part:
                    index, count = map(int, part.groups())
                    piece = prompt.split('\n<request_part>\n', 1)[1].rsplit('\n</request_part>\n', 1)[0]
                    cid = data['response']['payload']['conversationId']
                    if index == 1:
                        self.server.part_buffers[cid] = []
                    self.server.part_buffers[cid].append(piece)
                    if index < count:
                        data['response']['payload']['output'][0]['content'][0]['text'] = 'ACK'
                        super().reply(code, json.dumps(data), content_type, cache, retry_after)
                        return
                    prompt = ''.join(self.server.part_buffers.pop(cid))
                marker = re.search(r'PRISM_CLIENT_TOOLS_V1:[a-f0-9]+',prompt).group()
                current = prompt.rsplit('[user]\nNext task: verify the ticket code.', 1)[-1]
                if 'custom_tool_call_output' in current:
                    result = {'kind':'final','text':'fixture-confirmed' + ('\n' + 'y' * self.server.large_answer_bytes
                              if self.server.large_answer_bytes else '')}
                elif 'function_call_output' in current:
                    result = {'kind':'calls','calls':[{'name':'client.echo','input':'fixture-value'}]}
                else:
                    result = {'kind':'calls','calls':[{'name':'client.lookup','arguments':{'key':'fixture'}}]}
                start = prompt.find('{"index":')
                if start >= 0 and result['kind'] == 'calls':
                    catalog, _ = json.JSONDecoder().raw_decode(prompt[start:])
                    loaded = {tool['name'] for tool in catalog['full_declarations']}
                    wanted = result['calls'][0]['name']
                    if wanted not in loaded:
                        result = {'kind': 'inspect', 'names': [wanted]}
                data['response']['payload']['output'][0]['content'][0]['text'] = marker+'\n'+json.dumps(result)
                body = json.dumps(data)
        super().reply(code,body,content_type,cache,retry_after)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--chrome',required=True)
    parser.add_argument('--catalog-size',type=int,default=512)
    parser.add_argument('--unique-catalog', action='store_true')
    parser.add_argument('--large-output-bytes', type=int, default=0)
    parser.add_argument('--large-answer-bytes', type=int, default=0)
    args = parser.parse_args()
    upstream = ThreadingHTTPServer(('127.0.0.1',0),ToolFixture)
    upstream.daemon_threads = True
    upstream.lock = threading.Lock()
    upstream.asset_requests = 0
    upstream.reconnect_first = False
    upstream.large_answer_bytes = args.large_answer_bytes
    upstream.part_buffers = {}
    upstream.reconnected = set()
    upstream.max_prompt_bytes, upstream.oversized_starts = TURN_BYTES, 0
    upstream.jobs,upstream.release,upstream.mismatches,upstream.target = {},None,0,1
    threading.Thread(target=upstream.serve_forever,daemon=True).start()
    api.BASE = f'http://127.0.0.1:{upstream.server_port}'
    with tempfile.TemporaryDirectory() as directory:
        state = api.State(directory)
        worker = AsyncBrowserWorker(lambda:MultiplexBrowser(state,args.chrome,api,poll_seconds=0.05),api)
        handler = type('ToolsSmokeHandler',(api.Handler,),{'api_key':'synthetic-key','browser_turn':worker,
            'serialize_requests':False,'tool_state':ToolState(directory,api.AdapterError)})
        server = ThreadingHTTPServer(('127.0.0.1',0),handler)
        server.daemon_threads = True
        threading.Thread(target=server.serve_forever,daemon=True).start()
        payload={'model':'gpt-6.1-sol','stream':True,'reasoning':{'effort':'max','summary':'detailed'},
            'tools':client_catalog(args.catalog_size, unique=args.unique_catalog),
            'input':[{'role':'user','content':'Look up the fixture value, echo it, and report the confirmed result.'}]}
        headers={'Authorization':'Bearer synthetic-key','X-Prism-OAuth-Token':'synthetic-token',
            'X-Prism-Account-ID':'300','X-Prism-Caller-ID':'a'*64,'X-Prism-Session-ID':'b'*64,
            'Content-Type':'application/json'}
        try:
            task_count = 2 if args.large_answer_bytes else 1
            for step,expected in enumerate(('function_call','custom_tool_call','message') * task_count):
                turn = step % 3
                req=Request(f'http://127.0.0.1:{server.server_port}/v1/responses',data=json.dumps(payload).encode(),headers=headers)
                with urlopen(req,timeout=120) as reply:
                    events=[json.loads(line[6:]) for line in reply.read().decode().splitlines() if line.startswith('data: ')]
                assert events[-1]['type']=='response.completed'
                assert not any(event['type'].endswith('.delta') for event in events)
                response=events[-1]['response'];item=response['output'][0]
                assert response['model']=='gpt-6.1-sol' and response['usage'] is None
                assert response['reasoning']['effort']=='xhigh'
                assert response['metadata']['prism_requested_reasoning_effort']=='max'
                assert response['metadata']['prism_reasoning_effort']=='xhigh'
                assert item['type']==expected
                assert response['metadata']['prism_relay_mode'] == ('full' if step == 0 else 'delta')
                if args.large_output_bytes and turn == 1:
                    assert response['metadata']['prism_relay_parts'] > 1
                if args.large_answer_bytes and step == 3:
                    assert response['metadata']['prism_relay_parts'] == 1
                    assert response['metadata']['prism_relay_transport_bytes'] < TURN_BYTES
                if turn:
                    retry = Request(f'http://127.0.0.1:{server.server_port}/v1/responses',
                        data=json.dumps(payload).encode(), headers=headers)
                    count = len(upstream.jobs)
                    with urlopen(retry, timeout=120) as reply:
                        repeated = [json.loads(line[6:]) for line in reply.read().decode().splitlines()
                                    if line.startswith('data: ')][-1]['response']
                    assert repeated == response and len(upstream.jobs) == count
                payload['input'] += response['output']
                if turn<2:
                    assert item['namespace']=='client'
                    output = 'fixture-value' if turn == 0 else 'fixture-confirmed'
                    if turn == 0 and args.large_output_bytes:
                        output += '\n' + 'x' * args.large_output_bytes + '\n[exit code 0]'
                    payload['input'].append({'type':expected+'_output','call_id':item['call_id'], 'output': output})
                else:
                    assert item['content'][0]['text'].startswith('fixture-confirmed')
                    if step == 2 and task_count == 2:
                        payload['input'].append({'role':'user','content':'Next task: verify the ticket code.'})
            expected_starts = len(upstream.jobs)
            assert expected_starts >= 3 * task_count and upstream.mismatches==0
            assert upstream.oversized_starts == 0
            assert len({job['project'] for job in upstream.jobs.values()})==1
            assert len({job['cid'] for job in upstream.jobs.values()})==1
            peak_prompt_bytes = max(len(job['input'].encode('utf-8')) for job in upstream.jobs.values())
            assert max(transport_bytes(job['input']) for job in upstream.jobs.values()) <= TURN_BYTES
            assert not list(state.pending.iterdir())
            receipts=[json.loads(p.read_text()) for p in state.receipts.iterdir()]
            assert len(receipts)==expected_starts and all(r['start_count']==1 for r in receipts)
            assert all(r['model']=='gpt-6.1-sol' and r['reasoning_effort']=='xhigh' for r in receipts)
            assert all(job['effort']=='xhigh' for job in upstream.jobs.values())
            print(json.dumps({'result':'passed','scope':'real HTTP + Chromium + mock Prism',
                'upstream_starts':expected_starts,'fresh_projects':1, 'continued_starts':expected_starts-1,
                'function_calls':task_count,'custom_calls':task_count,'final_completed':True,
                'catalog_tools':args.catalog_size,'requested_effort':'max','prism_effort':'xhigh',
                'peak_prompt_bytes':peak_prompt_bytes,
                'large_output_bytes':args.large_output_bytes,
                'large_answer_bytes':args.large_answer_bytes,
                'pending':0,'real_prism_requests':0}))
        finally:
            server.shutdown();server.server_close();worker.close()
            upstream.shutdown();upstream.server_close()


if __name__=='__main__':
    main()
