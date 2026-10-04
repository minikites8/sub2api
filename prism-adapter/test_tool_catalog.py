import copy
import json
import unittest
from unittest import mock

from smoke_client_tools import client_catalog
from test_server import adapter
from tool_bridge import ToolBridge, validate_batch
from tool_validation import validate
from catalog_prompt import MAX_PRISM_PROMPT_BYTES


def request(tools, **changes):
    return dict({'model': 'gpt-6.1-sol', 'tools': tools,
                 'input': [{'role': 'user', 'content': 'Use the declared client tools.'}]}, **changes)


class CodexCatalogTests(unittest.TestCase):
    def test_full_namespaced_catalog_preserves_long_descriptions_and_last_tools(self):
        payload = request(client_catalog())
        bridge = ToolBridge(payload, adapter)
        self.assertEqual(len(bridge.tools), 512)
        self.assertEqual(bridge.catalog_mode, 'shared')
        self.assertLessEqual(len(bridge.prompt.encode('utf-8')), MAX_PRISM_PROMPT_BYTES)
        self.assertLess(len(bridge.prompt.encode('utf-8')), 1 << 20)
        declared = payload['tools'][0]['tools'][0]
        self.assertEqual(bridge.tools['connector_0.operation_0']['catalog']['description'], declared['description'])
        self.assertIn(declared['description'], bridge.prompt)
        frame = bridge.marker + '\n' + json.dumps({'kind': 'calls', 'calls': [
            {'name': 'client.lookup', 'arguments': {'key': 'fixture'}},
            {'name': 'client.echo', 'input': 'fixture-value'}]})
        output, _ = bridge.output(frame, 'catalog-fixture')
        self.assertEqual([(item['namespace'], item['name']) for item in output],
                         [('client', 'lookup'), ('client', 'echo')])
        self.assertEqual(output[0]['arguments'], '{"key":"fixture"}')
        self.assertEqual(output[1]['input'], 'fixture-value')

    def test_flat_catalog_larger_than_old_list_limit(self):
        tools = [{'type': 'function', 'name': f'operation_{index}',
                  'parameters': {'type': 'object', 'properties': {}}} for index in range(256)]
        bridge = ToolBridge(request(tools), adapter)
        frame = bridge.marker + '\n' + json.dumps({'kind': 'calls', 'calls': [
            {'name': 'operation_255', 'arguments': {}}]})
        output, _ = bridge.output(frame, 'flat-fixture')
        self.assertEqual(output[0]['name'], 'operation_255')

    def test_catalogs_from_all_responses_fields_share_the_budget(self):
        tools = client_catalog()
        payload = request(tools[:8], additional_tools=tools[8:16], input=[
            {'type': 'additional_tools', 'tools': tools[16:]},
            {'role': 'user', 'content': 'Use the declared tools.'}])
        bridge = ToolBridge(payload, adapter)
        self.assertEqual(len(bridge.tools), 512)
        self.assertIn('client.lookup', bridge.tools)

    def test_max_catalog_and_history_fit_the_validation_budget(self):
        history = [{'role': 'user', 'content': 'Continue the completed client calls.'}]
        for index in range(64):
            call_id = f'call_prism_{index:032x}'
            history += [
                {'type': 'function_call', 'namespace': 'client', 'name': 'lookup',
                 'call_id': call_id, 'arguments': '{"key":"fixture"}'},
                {'type': 'function_call_output', 'call_id': call_id, 'output': 'fixture-value'}]
        bridge = ToolBridge(request(client_catalog(), input=history), adapter)
        self.assertEqual(len(bridge.commands), 576)
        self.assertEqual(len(bridge.results), 64)

    def test_flat_and_namespaced_overflow_fail_before_validation(self):
        tools = [{'type': 'function', 'name': f'operation_{index}'} for index in range(513)]
        for definitions in (tools, client_catalog(513)):
            with self.subTest(namespaced=definitions[0]['type'] == 'namespace'):
                with mock.patch('tool_bridge.validate_batch') as batch:
                    with self.assertRaises(adapter.AdapterError) as raised:
                        ToolBridge(request(definitions), adapter)
                    self.assertEqual((raised.exception.status, raised.exception.code), (422, 'too_many_tools'))
                    self.assertIn('512', str(raised.exception))
                    batch.assert_not_called()

    def test_large_catalog_still_validates_the_last_tool_schema_and_arguments(self):
        tools = client_catalog()
        invalid = copy.deepcopy(tools)
        invalid[-1]['tools'][0]['parameters'] = {'$ref': 'https://example.invalid/schema'}
        with self.assertRaises(adapter.AdapterError) as raised:
            ToolBridge(request(invalid), adapter)
        self.assertEqual(raised.exception.code, 'invalid_tool_payload')
        bridge = ToolBridge(request(tools), adapter)
        frame = bridge.marker + '\n' + json.dumps({'kind': 'calls', 'calls': [
            {'name': 'client.lookup', 'arguments': {'key': 123}}]})
        with self.assertRaises(adapter.AdapterError) as raised:
            bridge.output(frame, 'invalid-arguments')
        self.assertEqual((raised.exception.status, raised.exception.code), (502, 'invalid_tool_output'))

    def test_bridge_size_budget_counts_utf8_bytes(self):
        tools = [{'type': 'function', 'name': f'operation_{index}', 'description': '工具说明' * 7500}
                 for index in range(12)]
        with self.assertRaises(adapter.AdapterError) as raised:
            ToolBridge(request(tools), adapter)
        self.assertEqual(raised.exception.code, 'tool_request_too_large')

    def test_validation_retains_command_and_byte_budgets(self):
        command = {'kind': 'function', 'parameters': {'type': 'object', 'properties': {}}}
        with self.assertRaisesRegex(ValueError, 'command budget'):
            validate([command] * 577)
        with self.assertRaises(adapter.AdapterError) as raised:
            validate_batch([dict(command, value={'large': 'x' * (1 << 20)})], adapter.AdapterError)
        self.assertEqual((raised.exception.status, raised.exception.code), (422, 'invalid_tool_payload'))


if __name__ == '__main__':
    unittest.main()
