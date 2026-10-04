import contextlib
import json
import re
import tempfile
import threading
import unittest
from http.server import ThreadingHTTPServer
from urllib.error import HTTPError
from urllib.request import Request, urlopen

from reasoning_options import resolve_reasoning
from test_server import adapter
from tool_state import ToolState


def payload(reasoning, **changes):
    return dict({'model': 'gpt-6.1-sol', 'input': 'fixture', 'reasoning': reasoning}, **changes)


@contextlib.contextmanager
def http_adapter(browser, state=None):
    handler = type('ReasoningHandler', (adapter.Handler,), {
        'api_key': 'fixture-key', 'browser_turn': browser,
        'serialize_requests': False, 'tool_state': state})
    server = ThreadingHTTPServer(('127.0.0.1', 0), handler)
    server.daemon_threads = True
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        yield f'http://127.0.0.1:{server.server_port}/v1/responses'
    finally:
        server.shutdown()
        server.server_close()


def send(url, body):
    headers = {'Authorization': 'Bearer fixture-key', 'X-Prism-Account-ID': '300',
               'X-Prism-OAuth-Token': 'synthetic', 'X-Prism-Session-ID': 'a' * 64,
               'X-Prism-Caller-ID': 'b' * 64, 'Content-Type': 'application/json'}
    with urlopen(Request(url, data=json.dumps(body).encode(), headers=headers), timeout=5) as response:
        raw = response.read()
        if body.get('stream'):
            events = [json.loads(line[6:]) for line in raw.decode().splitlines() if line.startswith('data: ')]
            return events[-1]['response']
        return json.loads(raw)


class ReasoningTests(unittest.TestCase):
    def test_max_and_ultra_resolve_to_the_highest_prism_tier_on_all_models(self):
        for model in adapter.MODELS:
            for effort in ('max', 'ultra', ' MAX '):
                with self.subTest(model=model, effort=effort):
                    body = payload({'effort': effort}, model=model)
                    self.assertEqual(resolve_reasoning(body, adapter.AdapterError), (effort, 'xhigh'))
                    self.assertEqual(adapter.parse_prompt(body), ('[user]\nfixture', False))
                    self.assertEqual(body['reasoning']['effort'], effort)

    def test_canonical_tiers_defaults_and_low_aliases(self):
        for effort in adapter.EFFORTS:
            self.assertEqual(resolve_reasoning(payload({'effort': effort}), adapter.AdapterError), (effort, effort))
        for effort, expected in ((None, 'medium'), ('', 'medium'), ('minimal', 'low'), ('none', 'low'),
                                 ('extra-high', 'xhigh'), ('extra_high', 'xhigh'), ('x-high', 'xhigh')):
            self.assertEqual(resolve_reasoning(payload({'effort': effort}), adapter.AdapterError), (effort, expected))
        self.assertEqual(resolve_reasoning(payload(None), adapter.AdapterError), (None, 'medium'))
        self.assertEqual(resolve_reasoning({'model': 'gpt-6.1-sol', 'input': 'fixture'}, adapter.AdapterError), (None, 'medium'))

    def test_optional_summaries_accept_codex_preferences_with_truthful_response_content(self):
        for summary in (None, 'none', 'auto', 'concise', 'detailed'):
            body = payload({'effort': 'max', 'summary': summary})
            self.assertEqual(adapter.parse_prompt(body), ('[user]\nfixture', False))
        response = adapter.response_payload('fixture', 'answer', 'gpt-6.1-sol', 'xhigh')
        self.assertEqual(response['reasoning'], {'effort': 'xhigh'})

    def test_invalid_reasoning_shapes_efforts_and_summaries_keep_distinct_errors(self):
        cases = [(value, 'unsupported_reasoning') for value in ([], '', False, 0, 'max')]
        cases += [({'effort': value}, 'unsupported_reasoning') for value in ([], {}, False, 4, 'unsupported')]
        cases += [({'effort': 'max', 'summary': value}, 'unsupported_reasoning_summary')
                  for value in ('unsupported', [], {}, 4)]
        for reasoning, expected in cases:
            with self.subTest(reasoning=reasoning), self.assertRaises(adapter.AdapterError) as raised:
                adapter.parse_prompt(payload(reasoning))
            self.assertEqual((raised.exception.status, raised.exception.code), (422, expected))


class ReasoningHTTPTests(unittest.TestCase):
    def test_json_and_sse_send_xhigh_report_max_and_reject_invalid_options_before_submit(self):
        class Browser:
            calls = []

            def run(inner, account, token, prompt, session, model, effort):
                inner.calls.append((model, effort))
                return 'fixture', 'answer'

        browser = Browser()
        with http_adapter(browser) as url:
            for stream in (False, True):
                response = send(url, payload({'effort': 'max', 'summary': 'detailed'}, stream=stream))
                self.assertEqual(response['reasoning']['effort'], 'xhigh')
                self.assertEqual(response['metadata'], {
                    'prism_requested_reasoning_effort': 'max', 'prism_reasoning_effort': 'xhigh'})
            for reasoning, expected in (({'effort': 'unsupported'}, 'unsupported_reasoning'),
                                        ({'effort': 'max', 'summary': 'unsupported'}, 'unsupported_reasoning_summary')):
                with self.assertRaises(HTTPError) as raised:
                    send(url, payload(reasoning))
                self.assertEqual(raised.exception.code, 422)
                self.assertEqual(json.load(raised.exception)['error']['type'], expected)
        self.assertEqual(browser.calls, [('gpt-6.1-sol', 'xhigh')] * 2)

    def test_tool_response_merges_requested_tier_and_hosted_tool_metadata(self):
        class Browser:
            def run(inner, account, token, prompt, session, model, effort, reuse_project):
                self.assertEqual(effort, 'xhigh')
                self.assertFalse(reuse_project)
                marker = re.search(r'PRISM_CLIENT_TOOLS_V1:[a-f0-9]+', prompt).group()
                return 'fixture', marker + '\n' + json.dumps({'kind': 'final', 'text': 'confirmed'})

        with tempfile.TemporaryDirectory() as directory:
            state = ToolState(directory, adapter.AdapterError)
            with http_adapter(Browser(), state) as url:
                response = send(url, payload({'effort': 'max', 'summary': 'concise'}, tools=[
                    {'type': 'function', 'name': 'lookup', 'parameters': {'type': 'object', 'properties': {}}},
                    {'type': 'web_search'}]))
        self.assertEqual(response['reasoning']['effort'], 'xhigh')
        self.assertEqual(response['metadata'], {'prism_requested_reasoning_effort': 'max',
            'prism_reasoning_effort': 'xhigh', 'prism_unavailable_tools': 'web_search'})


if __name__ == '__main__':
    unittest.main()
