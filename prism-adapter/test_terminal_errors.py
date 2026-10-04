import unittest

from test_reasoning_options import http_adapter, payload, send
from test_server import adapter
from urllib.error import HTTPError
import json


def failure(reason='unknown', **fields):
    return {'status': 'completed', 'response': {'status': 'error', 'payload': {'reason': reason, **fields}}}


class TerminalErrorTests(unittest.TestCase):
    def test_known_reasons_survive_both_top_level_and_nested_terminal_failures(self):
        cases = {'sandbox_reconnecting': (503, 'sandbox_reconnecting'),
                 'project_edit_access_required': (403, 'project_edit_access_required'),
                 'conversation_too_large': (422, 'conversation_too_large')}
        for status in ('completed', 'failed', 'error'):
            for reason, expected in cases.items():
                with self.subTest(status=status, reason=reason):
                    body = failure(reason)
                    body['status'] = status
                    error = adapter.terminal_text(body)
                    self.assertEqual((error.status, error.code), expected)

    def test_documented_diagnostics_produce_specific_runtime_errors(self):
        cases = {'workspace_sync_timeout': (504, 'prism_workspace_sync_timeout'),
                 'workspace_sync_unavailable': (503, 'prism_workspace_sync_unavailable'),
                 'sandbox_disconnected': (503, 'prism_sandbox_disconnected')}
        for code, expected in cases.items():
            with self.subTest(code=code):
                body = failure(diagnostics={'code': code, 'operation': 'start', 'httpStatus': 503,
                                             'requestId': 'private', 'token': 'private'})
                error = adapter.terminal_text(body)
                self.assertEqual((error.status, error.code), expected)
                self.assertEqual(error.upstream, {'upstream_code': code, 'upstream_operation': 'start',
                                                  'upstream_status': 503})
                self.assertNotIn('private', json.dumps(error.upstream))

    def test_payload_http_status_is_used_when_nested_status_is_missing(self):
        self.assertEqual(adapter.terminal_failure_diagnostics(failure(httpStatus=413)), {'upstream_status': 413})
        self.assertEqual(adapter.terminal_failure_diagnostics(failure(httpStatus=413, diagnostics={})),
                         {'upstream_status': 413})
        self.assertEqual(adapter.terminal_failure_diagnostics(failure(httpStatus=413, diagnostics={'httpStatus': 504})),
                         {'upstream_status': 504})
        for status in (True, '503', 200, 600, None):
            self.assertEqual(adapter.terminal_failure_diagnostics(failure(httpStatus=status)), {})

    def test_context_size_hints_are_safe_labels_with_generic_error(self):
        for text in ('maximum context exceeded', 'prompt is too long', 'request too large', '上下文长度超出限制'):
            with self.subTest(text=text):
                error = adapter.terminal_text(failure(message=text + ' private request text'))
                self.assertEqual(error.code, 'prism_failed')
                self.assertEqual(error.upstream, {'upstream_hints': ['context']})
                self.assertNotIn('private', str(error))

    def test_arbitrary_text_nested_objects_and_codes_stay_private(self):
        error = adapter.terminal_text(failure(message='Bearer private', rootCause={'token': 'private timeout'},
            diagnostics={'code': 'private', 'operation': 'private', 'httpStatus': 'private'}, httpStatus=True))
        self.assertEqual((error.status, error.code, error.upstream), (502, 'prism_failed', {}))
        self.assertEqual(str(error), 'Prism turn failed')

    def test_http_error_contains_only_safe_upstream_diagnostics(self):
        class Browser:
            def run(inner, *_args):
                raise adapter.terminal_text(failure(httpStatus=413, message='request too large private',
                    rootCause='Bearer private', diagnostics={'token': 'private'}))

        with http_adapter(Browser()) as url:
            with self.assertRaises(HTTPError) as raised:
                send(url, payload({'effort': 'high'}))
            self.assertEqual(raised.exception.code, 502)
            response = json.load(raised.exception)
        self.assertEqual(response, {'error': {'type': 'prism_failed', 'message': 'Prism turn failed',
                         'upstream': {'upstream_status': 413, 'upstream_hints': ['context']}}})
        self.assertNotIn('private', json.dumps(response))


if __name__ == '__main__':
    unittest.main()
