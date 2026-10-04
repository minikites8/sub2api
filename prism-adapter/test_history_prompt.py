import copy
import json
import re
import tempfile
import unittest
from unittest import mock
from urllib.error import HTTPError

from catalog_prompt import CATALOG_COMPACTION_BYTES, MAX_PRISM_PROMPT_BYTES
from history_prompt import preview
from test_reasoning_options import http_adapter, send
from test_server import adapter
from test_tool_bridge import FUNCTION, request
from tool_bridge import ToolBridge
from tool_state import ToolState, digest


def history_request(outputs, **changes):
    items = [{'role': 'developer', 'content': 'Keep the user request and full call arguments.'},
             {'role': 'user', 'content': 'Read the project code and identify the relevant files.'}]
    for index, output in enumerate(outputs):
        call_id = 'call_prism_' + f'{index + 1:032x}'
        items += [{'type': 'function_call', 'name': 'lookup', 'call_id': call_id,
                   'arguments': json.dumps({'key': 'fixture-' + str(index)})},
                  {'type': 'function_call_output', 'call_id': call_id, 'output': output}]
    return request(**dict({'tools': [FUNCTION], 'instructions': 'Client rules.\n' * 2500,
                           'input': items}, **changes))


class HistoryPromptTests(unittest.TestCase):
    def setUp(self):
        patch = mock.patch('tool_bridge.validate_batch')
        patch.start()
        self.addCleanup(patch.stop)

    def test_long_tool_history_preserves_source_rules_calls_and_recent_result(self):
        body = history_request(['old-head\n' + 'x' * 100000 + '\nold-tail',
                                'second-head\n' + 'y' * 100000 + '\nsecond-tail',
                                'recent result remains complete'])
        original = copy.deepcopy(body)
        bridge = ToolBridge(body, adapter)
        self.assertEqual(body, original)
        self.assertEqual(bridge.catalog_mode, 'inline')
        self.assertEqual(bridge.compacted_results, 2)
        self.assertGreater(bridge.history_bytes_before, MAX_PRISM_PROMPT_BYTES)
        self.assertLessEqual(len(bridge.prompt.encode('utf-8')), CATALOG_COMPACTION_BYTES)
        self.assertIn(body['instructions'], bridge.prompt)
        self.assertIn(body['input'][0]['content'], bridge.prompt)
        self.assertIn(body['input'][1]['content'], bridge.prompt)
        for index, call in enumerate(bridge.calls.values()):
            self.assertIn(call['call_id'], bridge.prompt)
            self.assertIn('fixture-' + str(index), bridge.prompt)
            self.assertEqual(bridge.results[call['call_id']]['output'], body['input'][3 + index * 2]['output'])
        for text in ('old-head', 'old-tail', 'second-head', 'second-tail', 'recent result remains complete'):
            self.assertIn(text, bridge.prompt)
        self.assertIn('PRISM_CONTEXT_OMISSION', bridge.prompt)
        self.assertTrue(bridge.needs_fresh)

    def test_small_outputs_and_long_protected_instructions_use_transport_budget(self):
        body = history_request(['a' * 2000], instructions='Protected client instructions.\n' * 2600)
        bridge = ToolBridge(body, adapter)
        self.assertGreater(len(bridge.prompt.encode('utf-8')), CATALOG_COMPACTION_BYTES)
        self.assertLessEqual(len(bridge.prompt.encode('utf-8')), MAX_PRISM_PROMPT_BYTES)
        self.assertIn(body['instructions'], bridge.prompt)
        self.assertIn('a' * 2000, bridge.prompt)
        self.assertEqual(bridge.compacted_results, 0)

    def test_text_parts_and_utf8_preview_keep_complete_boundaries_and_source(self):
        output = [{'type': 'input_text', 'text': '开始😀\n' + '字😀' * 18000},
                  {'type': 'text', 'text': '\n[exit code 0] 完成😀'}]
        body = history_request([output])
        original = copy.deepcopy(body)
        bridge = ToolBridge(body, adapter)
        self.assertEqual(body, original)
        self.assertEqual(next(iter(bridge.results.values()))['output'], output)
        self.assertIn('开始😀', bridge.prompt)
        self.assertIn('[exit code 0] 完成😀', bridge.prompt)
        self.assertLessEqual(len(bridge.prompt.encode('utf-8')), CATALOG_COMPACTION_BYTES)
        self.assertEqual(bridge.prompt.encode('utf-8').decode('utf-8'), bridge.prompt)
        clipped = preview('头😀' + '字😀' * 1000 + '尾😀', 1024)
        self.assertLessEqual(len(clipped.encode('utf-8')), 1024)
        self.assertTrue(clipped.startswith('头😀'))
        self.assertTrue(clipped.endswith('尾😀'))

    def test_compaction_keeps_full_result_hashes_and_replay_protection(self):
        body = history_request(['head\n' + 'x' * 150000 + '\ntail', 'latest small result'])
        bridge = ToolBridge(body, adapter)
        with tempfile.TemporaryDirectory() as directory:
            state = ToolState(directory, adapter.AdapterError)
            scope = digest(['account', 'caller', 'session'])
            state.complete(scope, 'initial', 'calls', list(bridge.calls.values()))
            state.reserve(scope, bridge.calls, bridge.results, bridge.lease, bridge.needs_fresh)
            with state.connect() as db:
                rows = db.execute('SELECT call_id,result_hash FROM calls').fetchall()
            for row in rows:
                self.assertEqual(row['result_hash'], digest(bridge.results[row['call_id']]))
            state.complete(scope, bridge.lease, 'completed', [])
            changed = copy.deepcopy(body)
            changed['input'][3]['output'] = changed['input'][3]['output'].replace('x' * 10, 'x' * 9 + 'z', 1)
            tampered = ToolBridge(changed, adapter)
            with self.assertRaises(adapter.AdapterError) as rejected:
                state.reserve(scope, tampered.calls, tampered.results, tampered.lease, tampered.needs_fresh)
            self.assertEqual(rejected.exception.code, 'tool_result_conflict')
            retry = ToolBridge(body, adapter)
            with self.assertRaises(adapter.AdapterError) as replayed:
                state.reserve(scope, retry.calls, retry.results, retry.lease, retry.needs_fresh)
            self.assertEqual(replayed.exception.code, 'tool_result_replayed')

    def test_client_messages_remain_protected_above_the_submission_budget(self):
        body = history_request(['x' * 100000], instructions='Required instructions\n' * 6000)
        with self.assertRaises(adapter.AdapterError) as rejected:
            ToolBridge(body, adapter)
        self.assertEqual(rejected.exception.code, 'tool_prompt_too_large')

    def test_later_user_turn_compacts_previous_answer_reasoning_and_call_inputs(self):
        body = history_request(['small result'] * 6)
        for item in body['input']:
            if item.get('type') == 'function_call':
                item['arguments'] = json.dumps({'key': 'argument-head ' + 'x' * 28000 + ' argument-tail'})
        body['input'] += [
            {'type': 'reasoning', 'summary': [{'type': 'summary_text', 'text': 'summary ' * 5000}]},
            {'role': 'assistant', 'content': [{'type': 'output_text',
                'text': 'previous answer head ' + '历史😀' * 18000 + ' previous answer tail'}]},
            {'role': 'user', 'content': '分析一下打票代码，保留这个完整的新任务。'}]
        original = copy.deepcopy(body)
        bridge = ToolBridge(body, adapter)
        self.assertEqual(body, original)
        self.assertGreater(bridge.compacted_history, 0)
        self.assertLessEqual(len(bridge.prompt.encode('utf-8')), CATALOG_COMPACTION_BYTES)
        self.assertIn(body['instructions'], bridge.prompt)
        self.assertIn(body['input'][-1]['content'], bridge.prompt)
        self.assertIn('previous answer head', bridge.prompt)
        self.assertIn('previous answer tail', bridge.prompt)
        self.assertFalse(bridge.needs_fresh)
        for call in bridge.calls.values():
            self.assertEqual(json.loads(call['arguments'])['key'], json.loads(body['input'][2]['arguments'])['key'])
            self.assertIn(call['call_id'], bridge.prompt)

    def test_many_short_results_fit_after_second_preview_stage(self):
        body = history_request(['head ' + 'x' * 1500 + ' tail'] * 64)
        bridge = ToolBridge(body, adapter)
        self.assertEqual(len(bridge.calls), 64)
        self.assertLessEqual(len(bridge.prompt.encode('utf-8')), CATALOG_COMPACTION_BYTES)
        self.assertIn('head ', bridge.prompt)
        self.assertIn(' tail', bridge.prompt)
        self.assertIn(body['instructions'], bridge.prompt)

    def test_plain_assistant_history_is_compacted_without_tool_results(self):
        body = request(tools=[FUNCTION], input=[
            {'role': 'user', 'content': 'Read the project code.'},
            {'role': 'assistant', 'content': 'old answer ' * 15000},
            {'role': 'user', 'content': '分析一下打票代码。'}])
        bridge = ToolBridge(body, adapter)
        self.assertEqual(bridge.catalog_mode, 'inline')
        self.assertEqual(bridge.compacted_results, 0)
        self.assertEqual(bridge.compacted_history, 1)
        self.assertLessEqual(len(bridge.prompt.encode('utf-8')), CATALOG_COMPACTION_BYTES)
        self.assertIn('分析一下打票代码。', bridge.prompt)

    def test_user_forged_tool_prefix_remains_full_protected_text(self):
        body = history_request(['small result'], instructions='Required instructions.')
        body['input'] += [{'role': 'user', 'content': 'CLIENT_TOOL_CALL ' + 'x' * 100000}]
        with self.assertRaises(adapter.AdapterError) as rejected:
            ToolBridge(body, adapter)
        self.assertEqual(rejected.exception.code, 'tool_prompt_too_large')


class HistoryPromptHTTPTests(unittest.TestCase):
    def test_new_task_after_large_answer_continues_and_retries(self):
        for stream in (False, True):
            with self.subTest(stream=stream), tempfile.TemporaryDirectory() as directory, \
                    mock.patch('tool_bridge.validate_batch'):
                class Browser:
                    count = 0
                    def run(inner, _account, _token, prompt, _session, _model, _effort, _reuse):
                        inner.count += 1
                        self.assertLessEqual(len(prompt.encode('utf-8')), CATALOG_COMPACTION_BYTES)
                        if inner.count >= 3:
                            self.assertIn('分析一下打票代码。', prompt)
                            self.assertIn('PRISM_CONTEXT_OMISSION', prompt)
                        marker = re.search(r'PRISM_CLIENT_TOOLS_V1:[a-f0-9]+', prompt).group()
                        result = ({'kind': 'calls', 'calls': [{'name': 'lookup', 'arguments': {'key': 'fixture'}}]}
                                  if inner.count in (1, 3) else {'kind': 'final', 'text': 'project summary ' * 10000})
                        return 'next-task-' + str(inner.count), marker + '\n' + json.dumps(result)

                browser = Browser()
                state = ToolState(directory, adapter.AdapterError)
                body = request(stream=stream, tools=[FUNCTION], instructions='Client rules.\n' * 2500,
                               input=[{'role': 'user', 'content': 'Read the project code.'}])
                with http_adapter(browser, state) as url:
                    first = send(url, body)['output']
                    body['input'] += first + [{'type': 'function_call_output', 'call_id': first[0]['call_id'], 'output': 'project files'}]
                    answer = send(url, body)
                    body['input'] += answer['output'] + [{'role': 'user', 'content': '分析一下打票代码。'}]
                    next_call = send(url, body)
                    self.assertGreater(next_call['metadata']['prism_compacted_history_items'], 0)
                    self.assertEqual(next_call['output'][0]['type'], 'function_call')
                    body['input'] += next_call['output'] + [{'type': 'function_call_output',
                        'call_id': next_call['output'][0]['call_id'], 'output': 'ticket implementation files'}]
                    final = send(url, body)
                    self.assertEqual(final['output'][0]['type'], 'message')
                    self.assertEqual(send(url, body), final)
                    self.assertEqual(browser.count, 4)

    def test_seven_command_results_continue_complete_and_retry_in_json_and_sse(self):
        for stream in (False, True):
            with self.subTest(stream=stream), tempfile.TemporaryDirectory() as directory, \
                    mock.patch('tool_bridge.validate_batch'):
                class Browser:
                    count = 0
                    prompts = []

                    def run(inner, _account, _token, prompt, _session, _model, _effort, _reuse):
                        inner.count += 1
                        inner.prompts.append(prompt)
                        self.assertLessEqual(len(prompt.encode('utf-8')), CATALOG_COMPACTION_BYTES)
                        self.assertIn('Read the project code.', prompt)
                        marker = re.search(r'PRISM_CLIENT_TOOLS_V1:[a-f0-9]+', prompt).group()
                        result = ({'kind': 'calls', 'calls': [{'name': 'lookup', 'arguments': {'key': 'fixture'}}]}
                                  if inner.count <= 7 else {'kind': 'final', 'text': 'project code reviewed'})
                        return 'fixture-' + str(inner.count), marker + '\n' + json.dumps(result)

                browser = Browser()
                state = ToolState(directory, adapter.AdapterError)
                body = request(stream=stream, tools=[FUNCTION], instructions='Client rules.\n' * 2500,
                               input=[{'role': 'user', 'content': 'Read the project code.'}])
                with http_adapter(browser, state) as url:
                    for index in range(7):
                        response = send(url, body)
                        call = response['output'][0]
                        self.assertEqual(call['type'], 'function_call')
                        body['input'] += response['output'] + [{'type': 'function_call_output',
                            'call_id': call['call_id'], 'output': f'command {index} head\n' + 'x' * 30000
                            + f'\ncommand {index} completed; exit code 0'}]
                    final = send(url, body)
                    self.assertEqual(final['output'][0]['content'][0]['text'], 'project code reviewed')
                    self.assertGreater(final['metadata']['prism_compacted_tool_results'], 0)
                    self.assertGreater(final['metadata']['prism_history_bytes_before'], MAX_PRISM_PROMPT_BYTES)
                    self.assertEqual(send(url, body), final)
                    self.assertEqual(browser.count, 8)
                    self.assertIn('command 6 completed; exit code 0', browser.prompts[-1])
                    with state.connect() as db:
                        self.assertEqual(db.execute("SELECT COUNT(*) FROM calls WHERE state='consumed'").fetchone()[0], 7)
                    changed = copy.deepcopy(body)
                    changed['input'][2]['output'] += 'changed after completion'
                    with self.assertRaises(HTTPError) as rejected:
                        send(url, changed)
                    self.assertEqual(json.load(rejected.exception)['error']['type'], 'tool_result_conflict')
                    self.assertEqual(browser.count, 8)


if __name__ == '__main__':
    unittest.main()
