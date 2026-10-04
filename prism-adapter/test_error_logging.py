import json
import tempfile
import unittest
from types import SimpleNamespace
from unittest import mock

from error_redaction import redact_error
import multiplex_browser
from test_terminal_errors import failure
from test_server import adapter


class ErrorLoggingTests(unittest.TestCase):
    def test_upstream_error_text_and_unicode_are_logged_verbatim(self):
        body = failure(message='Could not start Codex', rootCause='argument list too long: 输入过长')
        self.assertEqual(adapter.terminal_failure_log_fields(body), {
            'upstream_message': 'Could not start Codex', 'upstream_root_cause': 'argument list too long: 输入过长'})
        self.assertEqual(adapter.terminal_failure_log_fields(failure()),
                         {'upstream_message': None, 'upstream_root_cause': None})

    def test_known_credentials_prompt_headers_jwt_and_keys_are_redacted(self):
        private = ('actual-oauth-fixture', 'complete prompt body')
        samples = [*private, 'Bearer bearer-fixture', 'Basic basic-fixture',
                   'eyJheader.payload.signature', 'sk-123456789-private',
                   'Cookie: session=private-cookie; next=private-cookie',
                   'Authorization: Bearer private-bearer',
                   'https://fixture.invalid/?access_token=private-query&code=500',
                   'password=private-password', 'sandbox_token: private-sandbox',
                   '{"token":"private-json","message":"failed"}']
        for sample in samples:
            with self.subTest(sample=sample):
                result = redact_error(sample, private)
                self.assertIn('[REDACTED]', result)
                self.assertNotIn('private', result)
                self.assertNotIn('actual-oauth-fixture', result)
                self.assertNotIn('complete prompt body', result)

    def test_structured_causes_keep_error_fields_and_omit_payload_objects(self):
        result = json.loads(redact_error({'message': 'spawn failed', 'code': 'E2BIG',
            'headers': {'token': 'private'}, 'request': {'input': 'private'},
            'cause': {'message': 'input too long', 'token': 'private'}}))
        self.assertEqual(result, {'message': 'spawn failed', 'code': 'E2BIG',
                                 'cause': {'message': 'input too long'}})
        self.assertNotIn('private', json.dumps(result))

    def test_errors_are_bounded_and_json_remains_single_log_record(self):
        text = redact_error('error\n' + 'x' * 10000)
        self.assertLessEqual(len(text), 4096)
        self.assertTrue(text.endswith('[truncated]'))
        record = json.dumps({'upstream_message': text})
        self.assertEqual(len(record.splitlines()), 1)
        self.assertEqual(json.loads(record)['upstream_message'], text)

    def test_numeric_and_boolean_values_remain_absent(self):
        for value in (None, 503, False, []):
            self.assertIsNone(redact_error(value))


class LifecycleErrorLoggingTests(unittest.IsolatedAsyncioTestCase):
    async def test_failed_start_logs_upstream_details_once_and_preserves_failure(self):
        with tempfile.TemporaryDirectory() as directory:
            engine = multiplex_browser.MultiplexBrowser(adapter.State(directory), 'fixture', adapter)
            actor = SimpleNamespace(refs=0, projects={})
            starts = []

            async def account(*_args):
                actor.refs += 1
                return actor

            engine.account = account

            class Start:
                def __init__(inner, _engine, _actor, _journal, _session, _model, _effort):
                    inner.sent = inner.violation = inner.cache_hit = False
                    inner.request_id = 'fixture-request'
                    inner.project = 'fixture-project'
                    inner.phase = 'awaiting_start'

                async def run(inner, _prompt):
                    inner.sent = True
                    starts.append(inner.request_id)
                    return failure(message='Codex startup failed',
                        rootCause='spawn E2BIG; token=private-oauth; user prompt'), None

                async def close(inner):
                    pass

            with mock.patch.object(multiplex_browser, 'BrowserStart', Start), \
                    mock.patch.object(multiplex_browser, 'cgroup_memory_bytes', return_value=None), \
                    self.assertLogs('prism.lifecycle') as captured:
                with self.assertRaises(adapter.AdapterError) as raised:
                    await engine.run('300', 'private-oauth', 'user prompt', 'fixture-session')
            self.assertEqual(raised.exception.code, 'prism_failed')
            self.assertEqual(raised.exception.terminal_request_id, 'fixture-request')
            failures = [json.loads(record.getMessage()) for record in captured.records
                        if json.loads(record.getMessage())['event'] == 'prism_upstream_terminal_failure']
            self.assertEqual(len(failures), 1)
            self.assertEqual(failures[0]['upstream_message'], 'Codex startup failed')
            self.assertIn('spawn E2BIG', failures[0]['upstream_root_cause'])
            self.assertNotIn('private-oauth', ''.join(captured.output))
            self.assertNotIn('user prompt', ''.join(captured.output))
            self.assertEqual(starts, ['fixture-request'])
            self.assertEqual((actor.refs, engine.admission.running), (0, 0))
            self.assertEqual(list(engine.state.pending.iterdir()), [])


if __name__ == '__main__':
    unittest.main()
