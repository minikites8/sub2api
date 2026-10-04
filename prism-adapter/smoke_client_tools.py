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


def client_catalog(size=512):
    """A Codex-sized catalog, with the exercised tools at the end."""
    if size < 2:
        raise ValueError('catalog requires the two exercised client tools')
    tools = []
    for offset in range(0, size - 2, 32):
        tools.append({'type': 'namespace', 'name': f'connector_{offset // 32}', 'tools': [
            {'type': 'function', 'name': f'operation_{index}',
             'description': 'Codex connector fixture: preserve the complete client tool description. ' * 8,
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
    def reply(self, code, body, content_type='application/json', cache=False):
        if content_type == 'application/json':
            data = json.loads(body)
            if data.get('status') == 'completed':
                prompt = data['response']['payload']['output'][0]['content'][0]['text']
                marker = re.search(r'PRISM_CLIENT_TOOLS_V1:[a-f0-9]+',prompt).group()
                if 'custom_tool_call_output' in prompt:
                    result = {'kind':'final','text':'fixture-confirmed'}
                elif 'function_call_output' in prompt:
                    result = {'kind':'calls','calls':[{'name':'client.echo','input':'fixture-value'}]}
                else:
                    result = {'kind':'calls','calls':[{'name':'client.lookup','arguments':{'key':'fixture'}}]}
                data['response']['payload']['output'][0]['content'][0]['text'] = marker+'\n'+json.dumps(result)
                body = json.dumps(data)
        super().reply(code,body,content_type,cache)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--chrome',required=True)
    parser.add_argument('--catalog-size',type=int,default=512)
    args = parser.parse_args()
    upstream = ThreadingHTTPServer(('127.0.0.1',0),ToolFixture)
    upstream.daemon_threads = True
    upstream.lock = threading.Lock()
    upstream.asset_requests = 0
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
        payload={'model':'gpt-6.1-sol','stream':True,'reasoning':{'effort':'medium'},
            'tools':client_catalog(args.catalog_size),
            'input':[{'role':'user','content':'Look up the fixture value, echo it, and report the confirmed result.'}]}
        headers={'Authorization':'Bearer synthetic-key','X-Prism-OAuth-Token':'synthetic-token',
            'X-Prism-Account-ID':'300','X-Prism-Caller-ID':'a'*64,'X-Prism-Session-ID':'b'*64,
            'Content-Type':'application/json'}
        try:
            for turn,expected in enumerate(('function_call','custom_tool_call','message')):
                req=Request(f'http://127.0.0.1:{server.server_port}/v1/responses',data=json.dumps(payload).encode(),headers=headers)
                with urlopen(req,timeout=120) as reply:
                    events=[json.loads(line[6:]) for line in reply.read().decode().splitlines() if line.startswith('data: ')]
                assert events[-1]['type']=='response.completed'
                assert not any(event['type'].endswith('.delta') for event in events)
                response=events[-1]['response'];item=response['output'][0]
                assert response['model']=='gpt-6.1-sol' and response['usage'] is None
                assert item['type']==expected
                payload['input'] += response['output']
                if turn<2:
                    assert item['namespace']=='client'
                    payload['input'].append({'type':expected+'_output','call_id':item['call_id'],
                        'output':'fixture-value' if turn==0 else 'fixture-confirmed'})
                else:
                    assert item['content'][0]['text']=='fixture-confirmed'
            assert len(upstream.jobs)==3 and upstream.mismatches==0
            assert len({job['project'] for job in upstream.jobs.values()})==3
            assert not list(state.pending.iterdir())
            receipts=[json.loads(p.read_text()) for p in state.receipts.iterdir()]
            assert len(receipts)==3 and all(r['start_count']==1 for r in receipts)
            assert all(r['model']=='gpt-6.1-sol' and r['reasoning_effort']=='medium' for r in receipts)
            print(json.dumps({'result':'passed','scope':'real HTTP + Chromium + mock Prism',
                'upstream_starts':3,'fresh_projects':3,'function_calls':1,'custom_calls':1,'final_completed':True,
                'catalog_tools':args.catalog_size,'pending':0,'real_prism_requests':0}))
        finally:
            server.shutdown();server.server_close();worker.close()
            upstream.shutdown();upstream.server_close()


if __name__=='__main__':
    main()
