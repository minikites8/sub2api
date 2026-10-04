import copy
import json
import re
import tempfile
import unittest
from unittest import mock
from urllib.error import HTTPError
from types import SimpleNamespace

import multiplex_browser
from test_reasoning_options import http_adapter, send
from test_server import adapter
from test_terminal_errors import failure
from test_tool_bridge import FUNCTION, request
from tool_bridge import ToolBridge
from tool_state import ToolState


class InputRejectionTests(unittest.TestCase):
    def test_terminal_413_survives_as_a_request_size_error(self):
        error = adapter.terminal_text(failure(httpStatus=413,
            message='This request is too large to send. Shorten your message or selected text and try again.'))
        self.assertEqual((error.status, error.code), (413, 'prism_input_too_large'))

    def test_serial_browser_attests_size_rejection_only_on_trusted_start(self):
        for path in (adapter.START, adapter.STATUS):
            with self.subTest(path=path):
                state = mock.Mock()
                turn = adapter.BrowserRequest(state, '300', SimpleNamespace())
                turn.request_id = 'fixture-request'
                accepted = object()
                turn.accepted.add(accepted)
                data = failure(httpStatus=413)
                data['request_id'] = turn.request_id
                response = SimpleNamespace(request=accepted, url=adapter.BASE + path,
                    status=200, json=lambda: data)
                turn.response(response)
                self.assertEqual(turn.terminal[1].input_rejected_before_processing, path == adapter.START)
                turn.terminal = None
                response.request = object()
                turn.response(response)
                self.assertIsNone(turn.terminal)

    def test_53_tool_continuation_at_observed_size_is_compacted_before_submission(self):
        class Browser:
            prompts = []

            def run(inner, _account, _token, prompt, _session, _model, _effort, _reuse):
                inner.prompts.append(prompt)
                marker = re.search(r'PRISM_CLIENT_TOOLS_V1:[a-f0-9]+', prompt).group()
                if len(prompt.encode('utf-8')) > 64 * 1024:
                    error = adapter.terminal_text(failure(httpStatus=413,
                        message='This request is too large to send. Shorten your message or selected text and try again.'))
                    error.terminal_request_id = 'fixture-rejected'
                    error.input_rejected_before_processing = True
                    raise error
                result = ({'kind': 'calls', 'calls': [{'name': 'lookup', 'arguments': {'key': 'fixture'}}]}
                          if len(inner.prompts) == 1 else {'kind': 'final', 'text': 'confirmed'})
                return 'fixture-' + str(len(inner.prompts)), marker + '\n' + json.dumps(result)

        with tempfile.TemporaryDirectory() as directory, mock.patch('tool_bridge.validate_batch'):
            browser = Browser()
            state = ToolState(directory, adapter.AdapterError)
            with http_adapter(browser, state) as url:
                body = request(tools=[FUNCTION])
                first = send(url, body)
                body['input'] += first['output'] + [{'type': 'function_call_output',
                    'call_id': first['output'][0]['call_id'], 'output': 'workspace files read successfully'}]
                body['instructions'] = 'x' * 35000
                body['tools'] += [{'type': 'function', 'name': 'connector_' + str(index),
                    'description': f'Operation {index}. ' + 'd' * 1000, 'parameters': {'type': 'object'}}
                    for index in range(52)]
                # Reproduce the original inline prompt at the observed byte
                # count, independently of the corrected submission budget.
                with mock.patch('tool_bridge.MAX_PRISM_PROMPT_BYTES', 112 * 1024), \
                        mock.patch('tool_bridge.CATALOG_COMPACTION_BYTES', 112 * 1024):
                    original = ToolBridge(body, adapter)
                    padding = 102108 - len(original.prompt.encode('utf-8'))
                    self.assertGreater(padding, 0)
                    body['tools'][1]['description'] += 'd' * padding
                    original = ToolBridge(body, adapter)
                    self.assertEqual(original.catalog_mode, 'inline')
                    self.assertEqual(len(original.prompt.encode('utf-8')), 102108)
                final = send(url, body)
                self.assertEqual(final['output'][0]['content'][0]['text'], 'confirmed')
                self.assertEqual(final['metadata']['prism_catalog_mode'], 'indexed')
                self.assertLessEqual(len(browser.prompts[-1].encode('utf-8')), 64 * 1024)
                self.assertEqual(send(url, body), final)
                self.assertEqual(len(browser.prompts), 2)

    def test_confirmed_start_size_rejection_keeps_result_available_and_immutable(self):
        class Browser:
            count = 0

            def run(inner, _account, _token, prompt, _session, _model, _effort, _reuse):
                inner.count += 1
                marker = re.search(r'PRISM_CLIENT_TOOLS_V1:[a-f0-9]+', prompt).group()
                if inner.count == 2:
                    error = adapter.terminal_text(failure(httpStatus=413, message='request too large'))
                    error.terminal_request_id = 'fixture-rejected'
                    error.input_rejected_before_processing = True
                    raise error
                result = ({'kind': 'calls', 'calls': [{'name': 'lookup', 'arguments': {'key': 'fixture'}}]}
                          if inner.count == 1 else {'kind': 'final', 'text': 'confirmed'})
                return 'fixture-' + str(inner.count), marker + '\n' + json.dumps(result)

        with tempfile.TemporaryDirectory() as directory, mock.patch('tool_bridge.validate_batch'):
            browser = Browser()
            state = ToolState(directory, adapter.AdapterError)
            with http_adapter(browser, state) as url:
                body = request(tools=[FUNCTION])
                first = send(url, body)
                call_id = first['output'][0]['call_id']
                body['input'] += first['output'] + [{'type': 'function_call_output',
                    'call_id': call_id, 'output': 'immutable result'}]
                with self.assertRaises(HTTPError) as rejected:
                    send(url, body)
                self.assertEqual(rejected.exception.code, 413)
                self.assertEqual(json.load(rejected.exception)['error']['type'], 'prism_input_too_large')
                with state.connect() as db:
                    row = db.execute('SELECT state, result_hash, lease FROM calls WHERE call_id=?', (call_id,)).fetchone()
                self.assertEqual(row['state'], 'issued')
                self.assertIsNotNone(row['result_hash'])
                self.assertIsNone(row['lease'])
                changed = copy.deepcopy(body)
                changed['input'][-1]['output'] = 'changed result'
                with self.assertRaises(HTTPError) as tampered:
                    send(url, changed)
                self.assertEqual(json.load(tampered.exception)['error']['type'], 'tool_result_conflict')
                self.assertEqual(browser.count, 2)
                # The corrected result remains resumable across an adapter restart.
                restarted = ToolState(directory, adapter.AdapterError)
                with http_adapter(browser, restarted) as restarted_url:
                    final = send(restarted_url, body)
                self.assertEqual(final['output'][0]['content'][0]['text'], 'confirmed')
                self.assertEqual(browser.count, 3)

    def test_polled_size_failure_keeps_consumed_result_guard(self):
        class Browser:
            count = 0

            def run(inner, _account, _token, prompt, _session, _model, _effort, _reuse):
                inner.count += 1
                if inner.count > 1:
                    error = adapter.terminal_text(failure(httpStatus=413))
                    error.terminal_request_id = 'fixture-polled-failure'
                    raise error
                marker = re.search(r'PRISM_CLIENT_TOOLS_V1:[a-f0-9]+', prompt).group()
                return 'fixture', marker + '\n' + json.dumps({
                    'kind': 'calls', 'calls': [{'name': 'lookup', 'arguments': {'key': 'fixture'}}]})

        with tempfile.TemporaryDirectory() as directory, mock.patch('tool_bridge.validate_batch'):
            browser = Browser()
            state = ToolState(directory, adapter.AdapterError)
            with http_adapter(browser, state) as url:
                body = request(tools=[FUNCTION])
                first = send(url, body)
                body['input'] += first['output'] + [{'type': 'function_call_output',
                    'call_id': first['output'][0]['call_id'], 'output': 'fixture result'}]
                with self.assertRaises(HTTPError) as rejected:
                    send(url, body)
                self.assertEqual(rejected.exception.code, 413)
                with self.assertRaises(HTTPError) as duplicate:
                    send(url, body)
                self.assertEqual(json.load(duplicate.exception)['error']['type'], 'tool_result_replayed')
                self.assertEqual(browser.count, 2)


class BrowserInputRejectionTests(unittest.IsolatedAsyncioTestCase):
    async def test_multiplex_attests_size_rejection_only_before_polling(self):
        for immediate in (True, False):
            with self.subTest(immediate=immediate), tempfile.TemporaryDirectory() as directory:
                engine = multiplex_browser.MultiplexBrowser(adapter.State(directory), 'fixture', adapter)
                actor = SimpleNamespace(refs=0, projects={})

                async def account(*_args):
                    actor.refs += 1
                    return actor

                engine.account = account
                failed = failure(httpStatus=413)
                failed['request_id'] = 'fixture-request'

                class Start:
                    def __init__(inner, *_args):
                        inner.sent = inner.violation = inner.cache_hit = False
                        inner.request_id = 'fixture-request'
                        inner.project = 'fixture-project'
                        inner.phase = 'awaiting_start'

                    async def run(inner, _prompt):
                        inner.sent = True
                        return (failed if immediate else {'status': 'running'}), {}

                    async def close(inner):
                        pass

                with mock.patch.object(multiplex_browser, 'BrowserStart', Start), \
                        mock.patch.object(multiplex_browser, 'cgroup_memory_bytes', return_value=None), \
                        mock.patch.object(engine, 'poll_turn', new=mock.AsyncMock(return_value=failed)) as poll:
                    with self.assertRaises(adapter.AdapterError) as raised:
                        await engine.run('300', 'fixture', 'prompt', 'fixture-session')
                self.assertEqual((raised.exception.status, raised.exception.code), (413, 'prism_input_too_large'))
                self.assertEqual(raised.exception.terminal_request_id, 'fixture-request')
                self.assertEqual(raised.exception.input_rejected_before_processing, immediate)
                self.assertEqual(poll.await_count, int(not immediate))
                self.assertEqual(list(engine.state.pending.iterdir()), [])
                self.assertEqual((actor.refs, engine.admission.running, engine.polling), (0, 0, 0))


if __name__ == '__main__':
    unittest.main()
