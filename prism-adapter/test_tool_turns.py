import copy
import json
import re
import tempfile
import unittest
from unittest import mock
from urllib.error import HTTPError

from test_reasoning_options import http_adapter, send
from test_server import adapter
from test_tool_bridge import FUNCTION, request, call_output
from tool_bridge import ToolBridge
from tool_state import ToolState, digest


class ToolTurnTests(unittest.TestCase):
    def setUp(self):
        patch = mock.patch('tool_bridge.validate_batch')
        patch.start()
        self.addCleanup(patch.stop)
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.state = ToolState(self.directory.name, adapter.AdapterError)
        self.scope = digest(['account', 'caller', 'session'])
        first = ToolBridge(request(), adapter)
        output, calls = call_output(first)
        self.state.complete(self.scope, first.lease, 'first', calls)
        self.history = request()['input'] + output + [{
            'type': 'function_call_output', 'call_id': calls[0]['call_id'], 'output': 'fixture result',
        }]
        second = ToolBridge(request(input=self.history), adapter)
        self.reserve(second)
        self.state.complete(self.scope, second.lease, 'second', [])

    def reserve(self, bridge):
        return self.state.reserve(self.scope, bridge.calls, bridge.results, bridge.lease, bridge.needs_fresh)

    def test_new_user_turn_survives_trailing_assistant_context_and_reasoning(self):
        tails = [
            [{'role': 'assistant', 'content': 'I will check the next request.'}],
            [{'role': 'developer', 'content': 'Current workspace context.'},
             {'role': 'assistant', 'content': 'I will check the next request.'}],
            [{'type': 'reasoning', 'summary': [{'type': 'summary_text', 'text': 'Next step.'}]},
             {'role': 'assistant', 'content': 'I will check the next request.'}],
            [{'role': 'system', 'content': 'Current context.'}],
        ]
        for tail in tails:
            with self.subTest(tail=tail):
                history = self.history + [{'role': 'user', 'content': 'Check the next request.'}] + tail
                original = copy.deepcopy(history)
                bridge = ToolBridge(request(input=history), adapter)
                self.assertFalse(bridge.needs_fresh)
                self.assertEqual(self.reserve(bridge), [])
                self.assertIn('Check the next request.', bridge.history_prompt)
                self.assertEqual(history, original)

    def test_old_consumed_results_with_only_context_still_require_a_fresh_result(self):
        for role in ('assistant', 'developer', 'system'):
            with self.subTest(role=role):
                history = self.history + [{'role': role, 'content': 'Current context.'}]
                bridge = ToolBridge(request(input=history), adapter)
                self.assertTrue(bridge.needs_fresh)
                with self.assertRaises(adapter.AdapterError) as raised:
                    self.reserve(bridge)
                self.assertEqual(raised.exception.code, 'tool_result_replayed')

    def test_tool_results_after_a_new_user_message_resume_result_checks(self):
        history = copy.deepcopy(self.history)
        history.insert(-1, {'role': 'user', 'content': 'Continue this tool turn.'})
        bridge = ToolBridge(request(input=history), adapter)
        self.assertTrue(bridge.needs_fresh)
        with self.assertRaises(adapter.AdapterError) as raised:
            self.reserve(bridge)
        self.assertEqual(raised.exception.code, 'tool_result_replayed')

    def test_new_turn_can_issue_and_continue_another_tool_with_consumed_history(self):
        history = self.history + [{'role': 'user', 'content': 'Read another value.'},
                                  {'role': 'assistant', 'content': 'I will read it.'}]
        bridge = ToolBridge(request(input=history), adapter)
        self.reserve(bridge)
        output, calls = call_output(bridge, value={'key': 'next'})
        self.state.complete(self.scope, bridge.lease, 'next', calls)
        history += output + [{'type': 'function_call_output', 'call_id': calls[0]['call_id'], 'output': 'next value'}]
        continuation = ToolBridge(request(input=history), adapter)
        self.assertTrue(continuation.needs_fresh)
        self.assertEqual(self.reserve(continuation), [calls[0]['call_id']])
        self.state.complete(self.scope, continuation.lease, 'final', [])
        with self.assertRaises(adapter.AdapterError) as raised:
            self.reserve(ToolBridge(request(input=history), adapter))
        self.assertEqual(raised.exception.code, 'tool_result_replayed')

    def test_new_user_turn_keeps_scope_result_integrity_and_pending_checks(self):
        body = request(input=self.history + [{'role': 'user', 'content': 'Next request.'},
                                            {'role': 'assistant', 'content': 'Starting next request.'}])
        for mutation, expected in (('scope', 'unknown_tool_call'), ('result', 'tool_result_conflict'),
                                   ('pending', 'pending_tool_result')):
            with self.subTest(mutation=mutation):
                changed = copy.deepcopy(body)
                if mutation == 'result':
                    changed['input'][-3]['output'] = 'changed result'
                bridge = ToolBridge(changed, adapter)
                if mutation == 'pending':
                    with self.state.connect() as db:
                        db.execute("UPDATE calls SET state='reserved'")
                scope = digest(['other-account']) if mutation == 'scope' else self.scope
                with self.assertRaises(adapter.AdapterError) as raised:
                    self.state.reserve(scope, bridge.calls, bridge.results, bridge.lease, bridge.needs_fresh)
                self.assertEqual(raised.exception.code, expected)


class ToolTurnHTTPTests(unittest.TestCase):
    def test_new_codex_turn_after_completed_tools_reaches_browser_in_json_and_sse(self):
        for stream in (False, True):
            with self.subTest(stream=stream), tempfile.TemporaryDirectory() as directory, \
                    mock.patch('tool_bridge.validate_batch'):
                class Browser:
                    count = 0

                    def run(inner, _account, _token, prompt, _session, _model, _effort, reuse_project):
                        self.assertFalse(reuse_project)
                        inner.count += 1
                        marker = re.search(r'PRISM_CLIENT_TOOLS_V1:[a-f0-9]+', prompt).group()
                        if inner.count in (1, 3):
                            result = {'kind': 'calls', 'calls': [{'name': 'lookup', 'arguments': {'key': 'fixture'}}]}
                        else:
                            result = {'kind': 'final', 'text': 'confirmed'}
                        return 'fixture-' + str(inner.count), marker + '\n' + json.dumps(result)

                browser = Browser()
                state = ToolState(directory, adapter.AdapterError)
                body = request(stream=stream, tools=[FUNCTION])
                with http_adapter(browser, state) as url:
                    first = send(url, body)
                    body['input'] += first['output'] + [{'type': 'function_call_output',
                        'call_id': first['output'][0]['call_id'], 'output': 'fixture value'}]
                    completed = send(url, body)
                    self.assertEqual(send(url, body), completed)
                    self.assertEqual(browser.count, 2)
                    body['input'] += completed['output'] + [
                        {'role': 'user', 'content': 'Read another value.'},
                        {'role': 'developer', 'content': 'Updated workspace context.'},
                        {'role': 'assistant', 'content': 'I will read another value.'},
                    ]
                    next_turn = send(url, body)
                    body['input'] += next_turn['output'] + [{'type': 'function_call_output',
                        'call_id': next_turn['output'][0]['call_id'], 'output': 'next fixture value'}]
                    final = send(url, body)
                    self.assertEqual(final['output'][0]['content'][0]['text'], 'confirmed')
                    self.assertEqual(browser.count, 4)
                    self.assertIsNone(final['usage'])


if __name__ == '__main__':
    unittest.main()
