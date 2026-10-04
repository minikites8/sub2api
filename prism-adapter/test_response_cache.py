import copy
import json
import re
import tempfile
import unittest
from http.client import RemoteDisconnected
from unittest import mock
from urllib.error import HTTPError
from urllib.request import Request, urlopen

from response_cache import ResponseCache
from test_reasoning_options import http_adapter, send
from test_server import adapter
from test_tool_bridge import FUNCTION, request
from tool_state import ToolState


class ResponseCacheTests(unittest.TestCase):
    def test_byte_count_entry_count_and_oversized_responses_are_bounded(self):
        cache = ResponseCache(max_bytes=80, max_entries=2)
        for key in ('a', 'b', 'c'):
            cache.put(key, {'text': '字' * 5})
        self.assertIsNone(cache.get('a'))
        self.assertEqual(len(cache.entries), 2)
        self.assertEqual(cache.bytes, sum(len(body) for _, body in cache.entries.values()))
        self.assertLessEqual(cache.bytes, 80)
        cache.put('large', {'text': 'x' * 100})
        self.assertIsNone(cache.get('large'))
        cache.put('d', {'text': 'x' * 50})
        self.assertEqual(len(cache.entries), 1)
        self.assertEqual(cache.get('d'), {'text': 'x' * 50})

    def test_expiry_is_fixed_and_get_returns_an_independent_response(self):
        cache = ResponseCache(ttl=10)
        with mock.patch('response_cache.time.monotonic', return_value=100):
            cache.put('request', {'output': [{'call_id': 'stable-call'}]})
        with mock.patch('response_cache.time.monotonic', return_value=109):
            value = cache.get('request')
            value['output'][0]['call_id'] = 'changed'
            self.assertEqual(cache.get('request')['output'][0]['call_id'], 'stable-call')
        with mock.patch('response_cache.time.monotonic', return_value=110):
            self.assertIsNone(cache.get('request'))
        self.assertEqual(cache.bytes, 0)


class ContinuationReplayHTTPTests(unittest.TestCase):
    def test_completed_tool_continuation_retries_keep_response_and_call_ids(self):
        for stream in (False, True):
            with self.subTest(stream=stream), tempfile.TemporaryDirectory() as directory, \
                    mock.patch('tool_bridge.validate_batch'):
                class Browser:
                    count = 0

                    def run(inner, _account, _token, prompt, _session, _model, _effort, _reuse):
                        inner.count += 1
                        marker = re.search(r'PRISM_CLIENT_TOOLS_V1:[a-f0-9]+', prompt).group()
                        return 'fixture-' + str(inner.count), marker + '\n' + json.dumps({
                            'kind': 'calls', 'calls': [{'name': 'lookup', 'arguments': {'key': 'fixture'}}],
                        })

                browser = Browser()
                state = ToolState(directory, adapter.AdapterError)
                body = request(stream=stream, tools=[FUNCTION, {'type': 'web_search'}])
                with http_adapter(browser, state) as url:
                    first = send(url, body)
                    body['input'] += first['output'] + [{'type': 'function_call_output',
                        'call_id': first['output'][0]['call_id'], 'output': 'private fixture result'}]
                    completed = send(url, body)
                    replay = send(url, body)
                    self.assertEqual(replay, completed)
                    self.assertEqual(send(url, dict(body, stream=not stream)), completed)
                    self.assertEqual(browser.count, 2)
                    self.assertEqual(replay['output'][0]['call_id'], completed['output'][0]['call_id'])
                    self.assertEqual(replay['metadata']['prism_unavailable_tools'], 'web_search')
                    with state.connect() as db:
                        rows = db.execute('SELECT state FROM calls ORDER BY created_at').fetchall()
                    self.assertCountEqual([row['state'] for row in rows], ['consumed', 'issued'])
                    self.assertNotIn(b'private fixture result', state.path.read_bytes())
                    self.assertNotIn(b'"arguments"', state.path.read_bytes())
                    for field, change, expected in (
                        ('instructions', 'Different task.', 'tool_result_replayed'),
                        ('reasoning', {'effort': 'high'}, 'tool_result_replayed'),
                        ('input', copy.deepcopy(body['input']), 'tool_result_conflict'),
                    ):
                        if field == 'input':
                            change[-1]['output'] = 'changed fixture result'
                        with self.subTest(field=field), self.assertRaises(HTTPError) as denied:
                            send(url, dict(body, **{field: change}))
                        self.assertEqual(json.load(denied.exception)['error']['type'], expected)
                    headers = {'Authorization': 'Bearer fixture-key', 'X-Prism-Account-ID': '300',
                               'X-Prism-OAuth-Token': 'synthetic', 'X-Prism-Session-ID': 'a' * 64,
                               'X-Prism-Caller-ID': 'b' * 64, 'Content-Type': 'application/json'}
                    for name in ('X-Prism-Account-ID', 'X-Prism-Caller-ID', 'X-Prism-Session-ID'):
                        changed = dict(headers)
                        changed[name] = '301' if name == 'X-Prism-Account-ID' else 'c' * 64
                        with self.subTest(header=name), self.assertRaises(HTTPError) as denied:
                            urlopen(Request(url, data=json.dumps(body).encode(), headers=changed), timeout=5)
                        self.assertEqual(json.load(denied.exception)['error']['type'], 'unknown_tool_call')
                    state.responses = ResponseCache(ttl=0)
                    with self.assertRaises(HTTPError) as expired:
                        send(url, body)
                    self.assertEqual(json.load(expired.exception)['error']['type'], 'tool_result_replayed')
                    self.assertEqual(browser.count, 2)
                self.assertIsNone(ToolState(directory, adapter.AdapterError).cached_response('scope', 'request'))

    def test_response_lost_after_commit_is_available_on_the_next_http_attempt(self):
        class Browser:
            count = 0

            def run(inner, _account, _token, prompt, _session, _model, _effort, _reuse):
                inner.count += 1
                marker = re.search(r'PRISM_CLIENT_TOOLS_V1:[a-f0-9]+', prompt).group()
                result = ({'kind': 'calls', 'calls': [{'name': 'lookup', 'arguments': {'key': 'fixture'}}]}
                          if inner.count == 1 else {'kind': 'final', 'text': 'confirmed result'})
                return 'fixture-' + str(inner.count), marker + '\n' + json.dumps(result)

        browser = Browser()
        captured = []
        original = adapter.Handler.send_completion

        def lose_response(handler, response, stream, tool_response=False):
            if browser.count == 2 and not captured:
                captured.append(copy.deepcopy(response))
                raise ConnectionResetError('synthetic client disconnect')
            return original(handler, response, stream, tool_response)

        with tempfile.TemporaryDirectory() as directory, mock.patch('tool_bridge.validate_batch'), \
                mock.patch.object(adapter.Handler, 'send_completion', lose_response):
            state = ToolState(directory, adapter.AdapterError)
            with http_adapter(browser, state) as url:
                body = request(tools=[FUNCTION])
                first = send(url, body)
                body['input'] += first['output'] + [{'type': 'function_call_output',
                    'call_id': first['output'][0]['call_id'], 'output': 'fixture result'}]
                with self.assertRaises(RemoteDisconnected):
                    send(url, body)
                self.assertEqual(send(url, body), captured[0])
                self.assertEqual(browser.count, 2)

    def test_completion_between_cache_lookup_and_reservation_reuses_response(self):
        class Browser:
            count = 0

            def run(inner, _account, _token, prompt, _session, _model, _effort, _reuse):
                inner.count += 1
                marker = re.search(r'PRISM_CLIENT_TOOLS_V1:[a-f0-9]+', prompt).group()
                result = ({'kind': 'calls', 'calls': [{'name': 'lookup', 'arguments': {'key': 'fixture'}}]}
                          if inner.count == 1 else {'kind': 'final', 'text': 'confirmed'})
                return 'fixture-' + str(inner.count), marker + '\n' + json.dumps(result)

        with tempfile.TemporaryDirectory() as directory, mock.patch('tool_bridge.validate_batch'):
            browser = Browser()
            state = ToolState(directory, adapter.AdapterError)
            with http_adapter(browser, state) as url:
                body = request(tools=[FUNCTION])
                first = send(url, body)
                body['input'] += first['output'] + [{'type': 'function_call_output',
                    'call_id': first['output'][0]['call_id'], 'output': 'fixture result'}]
                completed = send(url, body)
                with mock.patch.object(state, 'cached_response', side_effect=[None, completed]):
                    self.assertEqual(send(url, body), completed)
                self.assertEqual(browser.count, 2)


if __name__ == '__main__':
    unittest.main()
