import copy
import json
import time
import unittest
from types import SimpleNamespace
from unittest import mock

from test_server import adapter
from test_tool_bridge import request, FUNCTION
from tool_bridge import ToolBridge
from relay_prompt import (RelayPrompt, continuation_context, history_keys, part_prompt,
                          record_matches, relay_settings, split_prompt, transport_bytes)


class TransportTests(unittest.TestCase):
    def test_lossless_unicode_json_escaping_and_long_lines(self):
        for text in ('中文🙂' * 17000, '\\"\t\n' * 18000, 'x' * 190000,
                     'line\n' * 50000):
            with self.subTest(length=len(text)):
                parts = split_prompt(text, adapter.AdapterError)
                self.assertEqual(''.join(parts), text)
                self.assertGreater(len(parts), 1)
                for index, part in enumerate(parts, 1):
                    self.assertLessEqual(transport_bytes(part_prompt(part, index, len(parts))), 86000)

    def test_single_part_and_final_remainder_use_all_available_room(self):
        text = 'x' * 86000
        self.assertEqual(split_prompt(text, adapter.AdapterError), [text])
        parts = split_prompt('x' * 180000 + '\nfooter', adapter.AdapterError)
        self.assertEqual(len(parts), 3)
        self.assertTrue(parts[-1].endswith('\nfooter'))

    def test_part_limit_fails_before_any_submission(self):
        with self.assertRaises(adapter.AdapterError) as raised:
            split_prompt('中' * 80000, adapter.AdapterError, max_parts=2)
        self.assertEqual(raised.exception.code, 'request_too_large')

    def test_transport_settings_bound_resource_use(self):
        for env in ({'PRISM_ADAPTER_TURN_BYTES': '100000'}, {'PRISM_ADAPTER_MAX_TURN_PARTS': '17'},
                    {'PRISM_ADAPTER_PART_GAP_SECONDS': '-1'}):
            with mock.patch.dict('os.environ', env), self.assertRaises(ValueError):
                relay_settings()


class HistoryTests(unittest.TestCase):
    def test_message_ids_and_wire_shapes_preserve_semantic_history(self):
        left = [{'role': 'assistant', 'content': 'answer'}]
        right = [{'type': 'message', 'id': 'msg-1', 'role': 'assistant', 'status': 'completed',
                  'content': [{'type': 'output_text', 'text': 'answer', 'annotations': []}]}]
        self.assertEqual(history_keys(left), history_keys(right))

    def test_call_arguments_are_canonical_but_ids_results_and_roles_are_exact(self):
        call = {'type': 'function_call', 'call_id': 'call-1', 'name': 'lookup', 'arguments': '{"a":1,"b":2}'}
        same = dict(call, arguments='{ "b": 2, "a": 1 }', status='completed')
        self.assertEqual(history_keys([call]), history_keys([same]))
        for changed in (dict(call, call_id='call-2'), dict(call, name='changed')):
            self.assertNotEqual(history_keys([call]), history_keys([changed]))
        self.assertNotEqual(history_keys([{'role': 'user', 'content': 'answer'}]),
                            history_keys([{'role': 'assistant', 'content': 'answer'}]))

    def test_empty_reasoning_and_additional_catalog_do_not_shift_render_positions(self):
        base = [{'role': 'user', 'content': 'next'}]
        self.assertEqual(history_keys(base), history_keys([{'type': 'reasoning', 'summary': []},
            {'type': 'additional_tools', 'tools': []}] + base))

    def test_matches_require_committed_exact_prefix_options_model_effort_and_age(self):
        body = {'model': 'gpt-6.1-sol', 'input': [{'role': 'user', 'content': 'first'}]}
        first = RelayPrompt('first', body, adapter)
        record = {'signature': first.signature, 'keys': first.keys, 'model': body['model'],
                  'effort': 'xhigh', 'at': time.monotonic(), 'committed': True}
        following = dict(body, input=body['input'] + [{'role': 'assistant', 'content': 'answer'},
                                                   {'role': 'user', 'content': 'next'}])
        prompt = RelayPrompt('following', following, adapter)
        self.assertTrue(record_matches(record, prompt, body['model'], 'xhigh'))
        for update in ({'committed': False}, {'at': time.monotonic() - 901}, {'effort': 'low'},
                       {'model': 'gpt-5.6-sol'}, {'signature': b'changed'}, {'keys': (b'changed',)}):
            self.assertFalse(record_matches(dict(record, **update), prompt, body['model'], 'xhigh'))
        self.assertFalse(record_matches(record, first, body['model'], 'xhigh'))

    def test_relay_catalog_keeps_large_current_tool_result_and_full_instructions(self):
        from test_tool_bridge import call_output
        with mock.patch('tool_bridge.validate_batch'):
            first = ToolBridge(request(), adapter)
            output, calls = call_output(first)
            body = request(input=request()['input'] + output + [{'type': 'function_call_output',
                'call_id': calls[0]['call_id'], 'output': '完整结果\n' + 'x' * 180000 + '\nend'}],
                instructions='完整规则\n' + 'i' * 40000)
            original = copy.deepcopy(body)
            bridge = ToolBridge(body, adapter, relay=True)
            self.assertEqual(bridge.compacted_results, 0)
            self.assertIn('x' * 180000, bridge.prompt)
            self.assertIn(body['instructions'], bridge.prompt)
            plan = RelayPrompt(bridge.prompt, body, adapter, bridge)
            delta = plan.delta(2, [{'type': 'function_call', 'call_id': calls[0]['call_id'], 'name': 'lookup'}])
            self.assertIn('x' * 180000, delta)
            self.assertIn(calls[0]['call_id'], delta)
            self.assertNotIn(body['instructions'], delta)
            self.assertEqual(body, original)

    def test_tool_policy_and_catalog_change_force_full_replay(self):
        with mock.patch('tool_bridge.validate_batch'):
            body = request(tools=[FUNCTION])
            bridge = ToolBridge(body, adapter, relay=True)
            first = RelayPrompt(bridge.prompt, body, adapter, bridge)
            for update in ({'instructions': 'new instructions'}, {'tool_choice': 'none'},
                           {'tools': [dict(FUNCTION, description='changed schema contract')]}):
                changed = dict(body, **update)
                next_bridge = ToolBridge(changed, adapter, relay=True)
                self.assertNotEqual(first.signature,
                    RelayPrompt(next_bridge.prompt, changed, adapter, next_bridge).signature)

    def test_large_selected_schema_is_preserved_for_split_submission(self):
        declaration = copy.deepcopy(FUNCTION)
        declaration['parameters']['description'] = 'schema contract ' * 6000
        body = request(tools=[declaration], tool_choice={'type': 'function', 'name': 'lookup'})
        with mock.patch('tool_bridge.validate_batch'):
            bridge = ToolBridge(body, adapter, relay=True)
        self.assertEqual(bridge.catalog_mode, 'indexed')
        self.assertIn(declaration['parameters']['description'], bridge.prompt)
        parts = split_prompt(bridge.prompt, adapter.AdapterError)
        self.assertGreater(len(parts), 1)
        self.assertEqual(''.join(parts), bridge.prompt)


class ContextTests(unittest.TestCase):
    def test_continuation_requires_server_response_conversation_and_listen_snapshot(self):
        template = {'metadata': {'projectId': 'project'}, 'conversationId': 'conversation'}
        start = SimpleNamespace(template=template, project='project')
        data = {'response': {'payload': {'id': 'response', 'conversationId': 'conversation'}}}
        snapshot = {'project_id': 'project', 'conversation_id': 'conversation', 'codex_session_id': 'session'}
        self.assertEqual(continuation_context(start, data, snapshot)['response_id'], 'response')
        for invalid in (None, dict(snapshot, conversation_id='foreign'), dict(snapshot, project_id='foreign'),
                        dict(snapshot, codex_session_id='')):
            self.assertIsNone(continuation_context(start, data, invalid))
        self.assertIsNone(continuation_context(start, {'response': {'payload': {}}}, snapshot))

    def test_server_snapshot_refreshes_sandbox_metadata(self):
        start = SimpleNamespace(template={'metadata': {'projectId': 'project', 'sandbox_token': 'previous'},
                                         'conversationId': 'conversation'}, project='project')
        data = {'response': {'payload': {'id': 'response', 'conversationId': 'conversation'}}}
        snapshot = {'project_id': 'project', 'conversation_id': 'conversation', 'codex_session_id': 'session',
                    'sandbox_token': 'current', 'sandbox_url': 'https://fixture.invalid/sandbox/'}
        context = continuation_context(start, data, snapshot)
        self.assertEqual(context['metadata']['sandbox_token'], 'current')
        self.assertEqual(context['metadata']['sandbox_url'], snapshot['sandbox_url'])
        self.assertEqual(start.template['metadata']['sandbox_token'], 'previous')


if __name__ == '__main__':
    unittest.main()
