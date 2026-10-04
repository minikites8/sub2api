import copy
import json
import tempfile
import time
import unittest
from unittest import mock

from catalog_prompt import shared_catalog, MAX_PRISM_PROMPT_BYTES, MAX_CATALOG_INSPECTIONS
from smoke_client_tools import client_catalog
from test_reasoning_options import http_adapter, send
from test_server import adapter
from tool_bridge import ToolBridge
from tool_state import ToolState


def request(**changes):
    return dict({'model': 'gpt-6.1-sol', 'reasoning': {'effort': 'max'},
                 'tools': client_catalog(unique=True), 'input': 'Look up the fixture value.'}, **changes)


def frame(bridge, value):
    return bridge.marker + '\n' + json.dumps(value)


class CatalogPromptTests(unittest.TestCase):
    def setUp(self):
        # Protocol tests isolate catalog construction; Linux CI separately
        # exercises the real validator and complete HTTP/browser exchange.
        patch = mock.patch('tool_bridge.validate_batch')
        self.validation = patch.start()
        self.addCleanup(patch.stop)

    def test_shared_fields_expand_to_the_exact_original_catalog(self):
        bridge = ToolBridge(request(tools=client_catalog()), adapter)
        self.assertEqual(bridge.catalog_mode, 'shared')
        self.assertLessEqual(len(bridge.prompt.encode()), MAX_PRISM_PROMPT_BYTES)
        packed = json.loads(shared_catalog(bridge.catalog))
        expanded = []
        for tool in packed['tools']:
            tool = dict(tool)
            for key in ('description', 'parameters', 'format'):
                if 'shared_' + key in tool:
                    tool[key] = packed['shared'][tool.pop('shared_' + key)]
            expanded.append(tool)
        self.assertEqual(expanded, bridge.catalog)
        self.assertIn('client.echo', {tool['name'] for tool in expanded})

    def test_index_keeps_all_names_and_loads_exact_tail_schema_and_grammar(self):
        payload = request()
        original = copy.deepcopy(payload)
        bridge = ToolBridge(payload, adapter)
        self.assertEqual(bridge.catalog_mode, 'indexed')
        self.assertEqual(len(bridge.tools), 512)
        index = json.loads(bridge.prompt.split('Client tool catalog:\n', 1)[1].split('\n')[1])
        self.assertEqual({tool['name'] for tool in index['index']}, set(bridge.tools))
        self.assertEqual(index['full_declarations'], [])
        self.assertTrue(bridge.expand_catalog(frame(bridge, {'kind': 'inspect', 'names': ['client.lookup', 'client.echo']})))
        index = json.loads(bridge.prompt.split('Client tool catalog:\n', 1)[1].split('\n')[1])
        self.assertEqual(index['full_declarations'], [bridge.tools[name]['catalog'] for name in ('client.lookup', 'client.echo')])
        self.assertLessEqual(len(bridge.prompt.encode()), MAX_PRISM_PROMPT_BYTES)
        self.assertEqual(payload, original)

    def test_unloaded_calls_are_inspected_before_any_call_identity_is_emitted(self):
        bridge = ToolBridge(request(), adapter)
        calls = frame(bridge, {'kind': 'calls', 'calls': [{'name': 'client.lookup', 'arguments': {'key': 'fixture'}}]})
        with self.assertRaises(adapter.AdapterError):
            bridge.output(calls, 'first')
        self.assertTrue(bridge.expand_catalog(calls))
        self.assertFalse(bridge.expand_catalog(calls))
        output, identities = bridge.output(calls, 'second')
        self.assertEqual((output[0]['namespace'], output[0]['name']), ('client', 'lookup'))
        self.assertEqual(len(identities), 1)

    def test_invalid_inspections_forced_choices_and_parallel_limits_fail_closed(self):
        for value in ({'kind': 'inspect', 'names': ['absent']},
                      {'kind': 'inspect', 'names': ['client.lookup'] * 2},
                      {'kind': 'inspect', 'names': []},
                      {'kind': 'inspect', 'names': ['client.lookup'], 'extra': True},
                      {'kind': 'inspect', 'names': [None]}):
            bridge = ToolBridge(request(), adapter)
            with self.subTest(value=value), self.assertRaises(adapter.AdapterError):
                bridge.expand_catalog(frame(bridge, value))
        bridge = ToolBridge(request(tool_choice={'type': 'function', 'namespace': 'client', 'name': 'lookup'}), adapter)
        self.assertIn('client.lookup', bridge.loaded)
        with self.assertRaises(adapter.AdapterError):
            bridge.expand_catalog(frame(bridge, {'kind': 'inspect', 'names': ['client.echo']}))
        bridge = ToolBridge(request(parallel_tool_calls=False), adapter)
        with self.assertRaises(adapter.AdapterError):
            bridge.expand_catalog(frame(bridge, {'kind': 'calls', 'calls': [
                {'name': 'client.lookup', 'arguments': {}}, {'name': 'client.echo', 'input': 'fixture-value'}]}))

    def test_inspections_have_a_finite_budget_and_repeat_requests_fail(self):
        bridge = ToolBridge(request(), adapter)
        for index in range(MAX_CATALOG_INSPECTIONS):
            self.assertTrue(bridge.expand_catalog(frame(bridge, {'kind': 'inspect', 'names': [f'connector_0.operation_{index}']})))
        with self.assertRaises(adapter.AdapterError) as raised:
            bridge.expand_catalog(frame(bridge, {'kind': 'inspect', 'names': ['client.lookup']}))
        self.assertEqual(raised.exception.code, 'tool_catalog_inspection_limit')
        bridge = ToolBridge(request(), adapter)
        bridge.expand_catalog(frame(bridge, {'kind': 'inspect', 'names': ['client.lookup']}))
        with self.assertRaises(adapter.AdapterError):
            bridge.expand_catalog(frame(bridge, {'kind': 'inspect', 'names': ['client.lookup']}))

    def test_selected_definition_overflow_is_explicit_and_utf8_safe(self):
        tools = client_catalog(unique=True)
        tools[-1]['tools'][1]['format'] = {'type': 'grammar', 'syntax': 'regex', 'definition': '字' * 16000}
        bridge = ToolBridge(request(tools=tools, input='历史 ' + '字' * 22000), adapter)
        with self.assertRaises(adapter.AdapterError) as raised:
            bridge.expand_catalog(frame(bridge, {'kind': 'inspect', 'names': ['client.echo']}))
        self.assertEqual(raised.exception.code, 'tool_prompt_too_large')

    def test_small_catalog_keeps_original_protocol_without_inspection(self):
        tools = client_catalog(2)
        bridge = ToolBridge(request(tools=tools), adapter)
        self.assertEqual(bridge.catalog_mode, 'inline')
        self.assertFalse(bridge.expand_catalog(frame(bridge, {'kind': 'final', 'text': 'answer'})))

    def test_http_hides_inspection_turns_and_returns_only_validated_client_call(self):
        import re

        class Browser:
            request_timeout = 285
            deadlines = []
            prompts = []

            def run_until(inner, deadline, *_args):
                inner.deadlines.append(deadline)
                return inner.run(*_args)

            def run(inner, _account, _token, prompt, _session, model, effort, reuse_project):
                self.assertEqual((model, effort, reuse_project), ('gpt-6.1-sol', 'xhigh', False))
                self.assertLessEqual(len(prompt.encode()), MAX_PRISM_PROMPT_BYTES)
                inner.prompts.append(prompt)
                marker = re.search(r'PRISM_CLIENT_TOOLS_V1:[a-f0-9]+', prompt).group()
                result = ({'kind': 'inspect', 'names': ['client.lookup']} if len(inner.prompts) == 1 else
                          {'kind': 'calls', 'calls': [{'name': 'client.lookup', 'arguments': {'key': 'fixture'}}]})
                return f'fixture-{len(inner.prompts)}', marker + '\n' + json.dumps(result)

        browser = Browser()
        with tempfile.TemporaryDirectory() as directory:
            state = ToolState(directory, adapter.AdapterError)
            with http_adapter(browser, state) as url:
                response = send(url, request(stream=True))
        self.assertEqual(len(browser.prompts), 2)
        self.assertEqual(browser.deadlines, [browser.deadlines[0]] * 2)
        self.assertGreater(browser.deadlines[0], time.monotonic())
        self.assertEqual(len(response['output']), 1)
        self.assertEqual(response['output'][0]['type'], 'function_call')
        self.assertEqual(response['output'][0]['namespace'], 'client')
        self.assertEqual(response['metadata']['prism_catalog_inspections'], 1)


if __name__ == '__main__':
    unittest.main()
