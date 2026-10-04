import json
import re
import tempfile
import unittest
from unittest import mock
from urllib.error import HTTPError

from catalog_prompt import MAX_PRISM_PROMPT_BYTES
from test_reasoning_options import http_adapter, send
from test_server import adapter
from tool_bridge import ToolBridge
from tool_state import ToolState


def codex_request(**changes):
    return dict({
        'model': 'gpt-6.1-sol', 'reasoning': {'effort': 'max', 'summary': 'auto'},
        'include': ['reasoning.encrypted_content'],
        'instructions': 'Follow the client instructions.\n' * 1100,
        'input': [
            {'role': 'developer', 'content': [{'type': 'input_text', 'text': 'Keep all context.'}]},
            {'role': 'user', 'content': [{'type': 'input_text', 'text': '生成 320 个整数。'}]},
        ],
    }, **changes)


class PromptLimitTests(unittest.TestCase):
    def test_text_between_64_and_96_kib_preserves_all_utf8_content(self):
        for model in adapter.MODELS:
            for character in ('x', '字', '😀'):
                text = '完整上下文\n' + character * (80000 // len(character.encode('utf-8')))
                with self.subTest(model=model, character=character):
                    prompt, stream = adapter.parse_prompt({'model': model, 'input': text, 'stream': True})
                    self.assertGreater(len(prompt.encode('utf-8')), 64 * 1024)
                    self.assertLess(len(prompt.encode('utf-8')), MAX_PRISM_PROMPT_BYTES)
                    self.assertEqual(prompt, '[user]\n' + text)
                    self.assertTrue(stream)

    def test_codex_context_above_32000_characters_preserves_all_messages(self):
        for model in adapter.MODELS:
            body = codex_request(model=model, stream=True)
            expected = ('[instructions]\n' + body['instructions']
                        + '\n\n[developer]\nKeep all context.\n\n[user]\n生成 320 个整数。')
            self.assertGreater(len(expected), 32000)
            self.assertLess(len(expected.encode('utf-8')), MAX_PRISM_PROMPT_BYTES)
            self.assertEqual(adapter.parse_prompt(body), (expected, True))

    def test_submission_budget_counts_utf8_and_message_framing(self):
        for character in ('x', '字', '😀'):
            available = MAX_PRISM_PROMPT_BYTES - len('[user]\n'.encode('utf-8'))
            count, padding = divmod(available, len(character.encode('utf-8')))
            text = character * count + 'x' * padding
            body = {'model': adapter.MODEL, 'input': text}
            with self.subTest(character=character):
                prompt, _ = adapter.parse_prompt(body)
                self.assertEqual(prompt, '[user]\n' + text)
                self.assertEqual(len(prompt.encode('utf-8')), MAX_PRISM_PROMPT_BYTES)
                with self.assertRaises(adapter.AdapterError) as raised:
                    adapter.parse_prompt(dict(body, input=text + 'x'))
                self.assertEqual((raised.exception.status, raised.exception.code), (413, 'request_too_large'))
                self.assertIn('96 KiB', str(raised.exception))

    def test_budget_includes_instructions_history_and_content_parts(self):
        body = {'model': adapter.MODEL, 'instructions': 'x' * 30000, 'input': [
            {'role': 'user', 'content': 'x' * 30000},
            {'role': 'assistant', 'content': [
                {'type': 'output_text', 'text': 'x' * 30000},
                {'type': 'output_text', 'text': 'x' * 30000},
            ]},
        ]}
        with self.assertRaises(adapter.AdapterError) as raised:
            adapter.parse_prompt(body)
        self.assertEqual((raised.exception.status, raised.exception.code), (413, 'request_too_large'))

    def test_empty_messages_keep_the_content_error(self):
        for text in ('', ' ', '\n\t'):
            with self.subTest(text=text), self.assertRaises(adapter.AdapterError) as raised:
                adapter.parse_prompt({'model': adapter.MODEL, 'input': text})
            self.assertEqual((raised.exception.status, raised.exception.code), (400, 'invalid_request'))
            self.assertEqual(str(raised.exception), 'message content must not be empty')


class PromptLimitHTTPTests(unittest.TestCase):
    def test_long_codex_text_and_tool_requests_reach_browser_in_json_and_sse(self):
        class Browser:
            prompts = []

            def run(inner, _account, _token, prompt, _session, model, effort, *options):
                self.assertEqual((model, effort), ('gpt-6.1-sol', 'xhigh'))
                self.assertGreater(len(prompt), 32000)
                self.assertLessEqual(len(prompt.encode('utf-8')), MAX_PRISM_PROMPT_BYTES)
                inner.prompts.append(prompt)
                if options:
                    self.assertEqual(options, (False,))
                    marker = re.search(r'PRISM_CLIENT_TOOLS_V1:[a-f0-9]+', prompt).group()
                    return 'fixture', marker + '\n' + json.dumps({'kind': 'final', 'text': 'confirmed'})
                return 'fixture', 'confirmed'

        browser = Browser()
        tools = [{'type': 'function', 'name': 'lookup', 'parameters': {'type': 'object'}}]
        with tempfile.TemporaryDirectory() as directory, mock.patch('tool_bridge.validate_batch'):
            state = ToolState(directory, adapter.AdapterError)
            with http_adapter(browser, state) as url:
                for stream in (False, True):
                    for catalog in ([], tools):
                        body = codex_request(stream=stream, tools=catalog)
                        response = send(url, body)
                        history, _ = adapter.parse_prompt(dict(body, tools=[]))
                        self.assertIn(history, browser.prompts[-1])
                        self.assertEqual(response['output'][0]['content'][0]['text'], 'confirmed')
                        self.assertEqual(response['reasoning']['effort'], 'xhigh')
                        self.assertIsNone(response['usage'])
                with self.assertRaises(HTTPError) as raised:
                    send(url, codex_request(instructions='字' * 40000))
                self.assertEqual(raised.exception.code, 413)
                self.assertEqual(json.load(raised.exception)['error']['type'], 'request_too_large')
        self.assertEqual(len(browser.prompts), 4)

    def test_tool_policy_and_catalog_still_share_the_submission_budget(self):
        text = 'x' * (MAX_PRISM_PROMPT_BYTES - len('[user]\n'))
        body = codex_request(instructions='', input=text, tools=[
            {'type': 'function', 'name': 'lookup', 'parameters': {'type': 'object'}},
        ])
        with mock.patch('tool_bridge.validate_batch'), self.assertRaises(adapter.AdapterError) as raised:
            ToolBridge(body, adapter)
        self.assertEqual(raised.exception.code, 'tool_prompt_too_large')


if __name__ == '__main__':
    unittest.main()
