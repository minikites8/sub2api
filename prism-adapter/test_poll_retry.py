import asyncio
import copy
import json
import tempfile
import unittest
from datetime import datetime, timedelta, timezone
from email.utils import format_datetime
from types import SimpleNamespace
from unittest import mock

import multiplex_browser
from test_server import adapter


class RetryAfterTests(unittest.TestCase):
    def test_seconds_dates_and_invalid_headers(self):
        self.assertEqual(multiplex_browser.retry_after_seconds(' 2 '), 2)
        self.assertEqual(multiplex_browser.retry_after_seconds('0'), 0)
        self.assertEqual(multiplex_browser.retry_after_seconds('999999'), 86400)
        future = format_datetime(datetime.now(timezone.utc) + timedelta(seconds=20), usegmt=True)
        self.assertTrue(18 <= multiplex_browser.retry_after_seconds(future) <= 20)
        past = format_datetime(datetime.now(timezone.utc) - timedelta(seconds=20), usegmt=True)
        self.assertEqual(multiplex_browser.retry_after_seconds(past), 0)
        for value in (None, 4, 'invalid', '-1', 'NaN', '1.5', '9' * 129):
            self.assertIsNone(multiplex_browser.retry_after_seconds(value))


class PollRetryTests(unittest.IsolatedAsyncioTestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.engine = multiplex_browser.MultiplexBrowser(adapter.State(self.directory.name), 'fixture', adapter)
        self.engine.observe = mock.Mock()
        self.journal = SimpleNamespace(local_id='local-fixture')
        self.body = {'request_id': 'request-private', 'turn_state': {'opaque': 'state-private'}}
        self.bodies = []

    def actor(self, results, *, sent=True):
        actor = multiplex_browser.AccountBrowser(self.engine, '300', 'token-private')
        results = iter(results)

        async def evaluate(_script, params):
            self.bodies.append(copy.deepcopy(params['body']))
            actor.expected[multiplex_browser.fingerprint(params['body'])]['sent'] = sent
            return next(results)

        actor.page = SimpleNamespace(evaluate=mock.AsyncMock(side_effect=evaluate))
        return actor

    async def test_transient_statuses_retry_same_identity_then_recover(self):
        result = {'request_id': self.body['request_id'], 'status': 'completed'}
        for status in multiplex_browser.POLL_RETRY_STATUSES:
            with self.subTest(status=status):
                self.bodies.clear()
                actor = self.actor([{'status': status}, {'status': 200, 'data': result}])
                with mock.patch.object(multiplex_browser.asyncio, 'sleep', new_callable=mock.AsyncMock) as sleep:
                    self.assertEqual(await self.engine.poll_turn(actor, self.body, self.journal), result)
                self.assertEqual(self.bodies, [self.body, self.body])
                self.assertEqual(actor.expected, {})
                sleep.assert_awaited_once_with(1)

    async def test_exhaustion_has_four_attempts_and_exponential_backoff(self):
        actor = self.actor([{'status': 503}] * 4)
        with mock.patch.object(multiplex_browser.asyncio, 'sleep', new_callable=mock.AsyncMock) as sleep:
            with self.assertRaises(adapter.AdapterError) as raised:
                await self.engine.poll_turn(actor, self.body, self.journal)
        self.assertEqual(raised.exception.code, 'poll_failed')
        self.assertEqual(self.bodies, [self.body] * 4)
        self.assertEqual(sleep.await_args_list, [mock.call(1), mock.call(2), mock.call(4)])
        self.assertEqual(actor.expected, {})
        self.engine.observe.assert_called_with('prism_poll_retry_exhausted', self.journal,
                                              attempt=4, http_status=503)

    async def test_auth_conflicts_unknown_transport_and_unsent_polls_keep_single_attempt(self):
        cases = [(status, True) for status in (0, 400, 401, 403, 404, 409, 422)] + [(503, False)]
        for status, sent in cases:
            with self.subTest(status=status, sent=sent):
                self.bodies.clear()
                actor = self.actor([{'status': status}], sent=sent)
                with mock.patch.object(multiplex_browser.asyncio, 'sleep', new_callable=mock.AsyncMock) as sleep:
                    with self.assertRaises(adapter.AdapterError):
                        await self.engine.poll_turn(actor, self.body, self.journal)
                self.assertEqual(self.bodies, [self.body])
                sleep.assert_not_awaited()
                self.assertEqual(actor.expected, {})

    async def test_retry_after_is_honored_and_long_delay_preserves_pending(self):
        actor = self.actor([{'status': 503, 'retry_after': '3'}, {'status': 200, 'data': {}}])
        with mock.patch.object(multiplex_browser.asyncio, 'sleep', new_callable=mock.AsyncMock) as sleep:
            self.assertEqual(await self.engine.poll_turn(actor, self.body, self.journal), {})
        sleep.assert_awaited_once_with(3)
        actor = self.actor([{'status': 503, 'retry_after': '60'}])
        with mock.patch.object(multiplex_browser.asyncio, 'sleep', new_callable=mock.AsyncMock) as sleep:
            with self.assertRaises(adapter.AdapterError):
                await self.engine.poll_turn(actor, self.body, self.journal)
        sleep.assert_not_awaited()

    async def test_wall_clock_budget_and_cancellation_stop_retries(self):
        actor = self.actor([{'status': 503}])
        clock = SimpleNamespace(monotonic=mock.Mock(side_effect=[0, 0, 30]))
        with mock.patch.object(multiplex_browser, 'time', clock), \
                mock.patch.object(multiplex_browser.asyncio, 'sleep', new_callable=mock.AsyncMock) as sleep:
            with self.assertRaises(adapter.AdapterError):
                await self.engine.poll_turn(actor, self.body, self.journal)
        sleep.assert_not_awaited()
        actor = self.actor([{'status': 503}])
        with mock.patch.object(multiplex_browser.asyncio, 'sleep', side_effect=asyncio.CancelledError):
            with self.assertRaises(asyncio.CancelledError):
                await self.engine.poll_turn(actor, self.body, self.journal)
        self.assertEqual(actor.expected, {})

    async def test_slow_retry_obeys_whole_poll_window(self):
        actor = self.actor([{'status': 503}])
        evaluate = actor.page.evaluate.side_effect
        calls = 0

        async def slow_retry(script, params):
            nonlocal calls
            calls += 1
            if calls == 1:
                return await evaluate(script, params)
            await asyncio.Event().wait()

        actor.page.evaluate.side_effect = slow_retry
        clock = SimpleNamespace(monotonic=mock.Mock(side_effect=[0, 0, 0, 29.98]))
        with mock.patch.object(multiplex_browser.asyncio, 'sleep', new_callable=mock.AsyncMock), \
                mock.patch.object(multiplex_browser, 'time', clock):
            with self.assertRaises(adapter.AdapterError) as raised:
                await self.engine.poll_turn(actor, self.body, self.journal)
        self.assertEqual((raised.exception.status, raised.exception.code), (504, 'poll_failed'))
        self.assertEqual(calls, 2)
        self.assertEqual(actor.expected, {})

    async def test_retry_diagnostics_contain_only_operational_fields(self):
        actor = self.actor([{'status': 503}, {'status': 200, 'data': {}}])
        with mock.patch.object(multiplex_browser.asyncio, 'sleep', new_callable=mock.AsyncMock):
            await self.engine.poll_turn(actor, self.body, self.journal)
        retry = next(call for call in self.engine.observe.call_args_list if call.args[0] == 'prism_poll_retry')
        self.assertEqual(retry.args, ('prism_poll_retry', self.journal))
        self.assertEqual(retry.kwargs, {'attempt': 1, 'http_status': 503, 'delay_seconds': 1})
        self.assertNotIn('private', str(self.engine.observe.call_args_list))

    async def test_successful_rotation_can_retry_next_poll_with_latest_state(self):
        actor = self.actor([{'status': 503}, {'status': 200, 'data': {'turn_state': 'rotated'}},
                            {'status': 503}, {'status': 200, 'data': {'status': 'completed'}}])
        with mock.patch.object(multiplex_browser.asyncio, 'sleep', new_callable=mock.AsyncMock):
            result = await self.engine.poll_turn(actor, self.body, self.journal)
            self.body['turn_state'] = result['turn_state']
            await self.engine.poll_turn(actor, self.body, self.journal)
        self.assertEqual(self.bodies[0], self.bodies[1])
        self.assertEqual(self.bodies[2], self.bodies[3])
        self.assertEqual(self.bodies[2]['turn_state'], 'rotated')

    async def test_exhaustion_retains_journal_single_start_and_releases_slots(self):
        actor = self.actor([{'status': 503}] * 4)
        actor.projects = {}
        self.engine.account = mock.AsyncMock(return_value=actor)
        started = []

        class Start:
            def __init__(inner, _engine, _actor, journal, _session, _model, _effort):
                inner.journal = journal
                inner.sent = inner.violation = inner.cache_hit = False
                inner.request_id = 'request-private'
                inner.project = 'project-fixture'
                inner.phase = 'awaiting_poll_handoff'

            async def run(inner, _prompt):
                actor.refs += 1
                inner.sent = True
                started.append(inner.request_id)
                inner.journal.update(stage='polling', request_id=inner.request_id, turn_state='opaque')
                return {'status': 'started'}, {'request_id': inner.request_id, 'turn_state': 'opaque'}

            async def close(inner):
                pass

        with mock.patch.object(multiplex_browser, 'BrowserStart', Start), \
                mock.patch.object(multiplex_browser, 'cgroup_memory_bytes', return_value=None), \
                mock.patch.object(multiplex_browser.asyncio, 'sleep', new_callable=mock.AsyncMock):
            with self.assertRaises(adapter.AdapterError):
                await self.engine.run('300', 'token-private', 'prompt', 'scope-fixture')
        self.assertEqual(started, ['request-private'])
        self.assertEqual(len(self.bodies), 4)
        records = list((self.engine.state.pending / '300').iterdir())
        self.assertEqual(len(records), 1)
        self.assertEqual(json.loads(records[0].read_text())['turn_state'], 'opaque')
        self.assertEqual((actor.refs, self.engine.admission.running, self.engine.admission.outstanding), (0, 0, 0))
        self.engine.observe.assert_called_with('prism_turn_error', mock.ANY,
                                              code='poll_failed', phase='polling', submitted=True)
        with self.assertRaises(adapter.AdapterError) as raised:
            await self.engine.run('300', 'token-private', 'prompt', 'scope-fixture')
        self.assertEqual(raised.exception.code, 'pending_turn')
        self.assertEqual(started, ['request-private'])


if __name__ == '__main__':
    unittest.main()
