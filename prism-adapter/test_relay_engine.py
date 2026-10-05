import asyncio
import json
import tempfile
import unittest
from collections import OrderedDict
from types import SimpleNamespace
from unittest import mock

from test_server import adapter
from multiplex_browser import BrowserContinuation, MultiplexBrowser, AccountBrowser
from relay_prompt import RelayPrompt


class RelayEngineTests(unittest.IsolatedAsyncioTestCase):
    def setup_engine(self, directory):
        engine = MultiplexBrowser(adapter.State(directory), 'fixture', adapter)
        actor = SimpleNamespace(refs=0, projects=OrderedDict(), conversations=OrderedDict())
        async def account(*_args):
            actor.refs += 1
            return actor
        engine.account = account
        return engine, actor

    def plan(self, items):
        body = {'model': 'gpt-6.1-sol', 'input': items}
        return RelayPrompt(adapter.parse_prompt(body, max_bytes=1 << 20)[0], body, adapter)

    async def test_committed_turn_continues_only_new_history_with_current_cursor(self):
        with tempfile.TemporaryDirectory() as directory:
            engine, actor = self.setup_engine(directory)
            seen = []
            async def turn(_actor, journal, text, _session, _model, _effort, _reuse, context=None, secrets=()):
                seen.append((text, context))
                journal.update(stage='polling', request_id=str(len(seen)))
                return str(len(seen)), 'answer', {'response_id': str(len(seen))}
            engine.execute_turn = turn
            items = [{'role': 'user', 'content': 'first'}]
            first = self.plan(items)
            await engine.run('300', 'token', first, 'session', 'gpt-6.1-sol', 'xhigh')
            first.commit([{'role': 'assistant', 'content': 'answer'}])
            next_plan = self.plan(items + [{'role': 'assistant', 'content': 'answer'},
                                         {'role': 'user', 'content': 'next'}])
            await engine.run('300', 'token', next_plan, 'session', 'gpt-6.1-sol', 'xhigh')
            self.assertEqual(seen[1], ('[user]\nnext', {'response_id': '1'}))
            self.assertEqual(next_plan.mode, 'delta')
            self.assertEqual((actor.refs, engine.admission.running), (0, 0))
            self.assertEqual(list(engine.state.pending.iterdir()), [])

    async def test_other_session_and_uncommitted_output_start_full_history(self):
        with tempfile.TemporaryDirectory() as directory:
            engine, actor = self.setup_engine(directory)
            seen = []
            async def turn(_actor, _journal, text, _session, *_args):
                seen.append(text)
                return 'request', 'answer', {'response_id': 'response'}
            engine.execute_turn = turn
            first = self.plan([{'role': 'user', 'content': 'first'}])
            await engine.run('300', 'token', first, 'session')
            next_plan = self.plan([{'role': 'user', 'content': 'first'}, {'role': 'user', 'content': 'next'}])
            await engine.run('300', 'token', next_plan, 'session')
            self.assertEqual(next_plan.mode, 'full')
            next_plan.commit([{'role': 'assistant', 'content': 'answer'}])
            other = self.plan(next_plan.payload['input'] + [{'role': 'assistant', 'content': 'answer'},
                                                           {'role': 'user', 'content': 'other'}])
            await engine.run('300', 'token', other, 'other-session')
            self.assertEqual(other.mode, 'full')
            self.assertIn('[user]\nfirst', seen[-1])

    async def test_unknown_partial_start_retains_journal_known_unsent_partial_retires_it(self):
        for sent in (False, True):
            with self.subTest(sent=sent), tempfile.TemporaryDirectory() as directory:
                engine, actor = self.setup_engine(directory)
                count = 0
                async def turn(_actor, journal, _text, _session, *_args):
                    nonlocal count
                    count += 1
                    journal.update(stage='polling' if sent else 'preparing', request_id=str(count))
                    if count == 2:
                        raise adapter.AdapterError(502, 'fixture_error', 'fixture', not_submitted=not sent)
                    return 'first', 'ACK', {'response_id': 'first'}
                engine.execute_turn = turn
                plan = self.plan([{'role': 'user', 'content': 'x' * 180000}])
                with mock.patch.dict('os.environ', {'PRISM_ADAPTER_PART_GAP_SECONDS': '0'}), \
                        self.assertRaises(adapter.AdapterError) as raised:
                    await engine.run('300', 'token', plan, 'session')
                self.assertFalse(raised.exception.not_submitted)
                pending = list((engine.state.pending / '300').glob('*.json'))
                self.assertEqual(len(pending), 1 if sent else 0)
                self.assertEqual(getattr(raised.exception, 'terminal_request_id', None), None if sent else 'first')
                self.assertEqual((actor.refs, engine.admission.running), (0, 0))

    async def test_invalid_ack_and_missing_snapshot_stop_later_parts(self):
        for answer, context in (('tool-call', {'response_id': 'first'}), ('ACK', None)):
            with self.subTest(answer=answer), tempfile.TemporaryDirectory() as directory:
                engine, _ = self.setup_engine(directory)
                engine.execute_turn = mock.AsyncMock(return_value=('first', answer, context))
                with self.assertRaises(adapter.AdapterError) as raised:
                    await engine.run('300', 'token', self.plan([{'role': 'user', 'content': 'x' * 180000}]), 'session')
                self.assertEqual(raised.exception.terminal_request_id, 'first')
                self.assertEqual(engine.execute_turn.await_count, 1)
                self.assertEqual(list(engine.state.pending.iterdir()), [])

    async def test_split_limit_is_checked_before_browser_submission(self):
        with tempfile.TemporaryDirectory() as directory:
            engine, _ = self.setup_engine(directory)
            engine.execute_turn = mock.AsyncMock()
            with mock.patch.dict('os.environ', {'PRISM_ADAPTER_MAX_TURN_PARTS': '1'}), \
                    self.assertRaises(adapter.AdapterError) as raised:
                await engine.run('300', 'token', self.plan([{'role': 'user', 'content': 'x' * 180000}]), 'session')
            self.assertTrue(raised.exception.not_submitted)
            engine.execute_turn.assert_not_awaited()
            self.assertEqual(list(engine.state.pending.iterdir()), [])

    async def test_explicit_immediate_size_refusal_reduces_budget_and_preserves_text(self):
        with tempfile.TemporaryDirectory() as directory:
            engine, _ = self.setup_engine(directory)
            seen = []
            async def turn(_actor, _journal, text, _session, *_args):
                seen.append(text)
                if len(seen) == 1:
                    error = adapter.AdapterError(413, 'prism_input_too_large', 'fixture')
                    error.input_rejected_before_processing = True
                    error.terminal_request_id = 'rejected'
                    raise error
                return str(len(seen)), 'ACK' if len(seen) == 2 else 'answer', {'response_id': str(len(seen))}
            engine.execute_turn = turn
            plan = self.plan([{'role': 'user', 'content': 'x' * 80000}])
            with mock.patch.dict('os.environ', {'PRISM_ADAPTER_PART_GAP_SECONDS': '0'}):
                result = await engine.run('300', 'token', plan, 'session')
            self.assertEqual(result[1], 'answer')
            self.assertEqual(plan.parts, 2)
            self.assertTrue(all(len(part.encode('utf-8')) < 43000 for part in seen[1:]))
            pieces = [part.split('\n<request_part>\n')[1].rsplit('\n</request_part>\n', 1)[0] for part in seen[1:]]
            self.assertEqual(''.join(pieces), str(plan))
            self.assertEqual(list(engine.state.pending.iterdir()), [])

    async def test_explicit_full_conversation_refusal_replays_full_validated_history(self):
        with tempfile.TemporaryDirectory() as directory:
            engine, _ = self.setup_engine(directory)
            seen = []
            async def turn(_actor, _journal, text, _session, _model, _effort, _reuse, context=None, secrets=()):
                seen.append((text, context))
                if len(seen) == 2:
                    error = adapter.AdapterError(422, 'conversation_too_large', 'fixture')
                    error.input_rejected_before_processing = True
                    error.terminal_request_id = 'rejected'
                    raise error
                return str(len(seen)), 'answer', {'response_id': str(len(seen))}
            engine.execute_turn = turn
            first = self.plan([{'role': 'user', 'content': 'first'}])
            await engine.run('300', 'token', first, 'session')
            first.commit([{'role': 'assistant', 'content': 'answer'}])
            next_plan = self.plan(first.payload['input'] + [{'role': 'assistant', 'content': 'answer'},
                                                          {'role': 'user', 'content': 'next'}])
            await engine.run('300', 'token', next_plan, 'session')
            self.assertEqual(seen[1][0], '[user]\nnext')
            self.assertEqual(seen[2], (str(next_plan), None))
            self.assertEqual(next_plan.mode, 'full')

    async def test_polled_size_failure_keeps_terminal_consumption_and_single_start(self):
        with tempfile.TemporaryDirectory() as directory:
            engine, _ = self.setup_engine(directory)
            error = adapter.AdapterError(413, 'prism_input_too_large', 'fixture')
            error.input_rejected_before_processing = False
            error.terminal_request_id = 'failed'
            engine.execute_turn = mock.AsyncMock(side_effect=error)
            with self.assertRaises(adapter.AdapterError) as raised:
                await engine.run('300', 'token', self.plan([{'role': 'user', 'content': 'first'}]), 'session')
            self.assertFalse(raised.exception.input_rejected_before_processing)
            engine.execute_turn.assert_awaited_once()
            self.assertEqual(list(engine.state.pending.iterdir()), [])

    async def test_conversation_admission_is_held_across_all_parts(self):
        with tempfile.TemporaryDirectory() as directory:
            engine, _ = self.setup_engine(directory)
            release, entered = asyncio.Event(), asyncio.Event()
            seen = []
            async def turn(_actor, _journal, text, _session, *_args):
                seen.append(text)
                if len(seen) == 1:
                    entered.set()
                    await release.wait()
                answer = 'ACK' if text.startswith(('[PRISM_TRANSPORT_PART 1/', '[PRISM_TRANSPORT_PART 2/')) else 'answer'
                return str(len(seen)), answer, {'response_id': str(len(seen))}
            engine.execute_turn = turn
            with mock.patch.dict('os.environ', {'PRISM_ADAPTER_PART_GAP_SECONDS': '0'}):
                first = asyncio.create_task(engine.run('300', 'token',
                    self.plan([{'role': 'user', 'content': 'x' * 180000}]), 'session'))
                await entered.wait()
                second = asyncio.create_task(engine.run('300', 'token',
                    self.plan([{'role': 'user', 'content': 'other'}]), 'session'))
                await asyncio.sleep(0)
                self.assertEqual(len(seen), 1)
                self.assertEqual(engine.admission.running, 1)
                release.set()
                await asyncio.gather(first, second)
            self.assertEqual(len(seen), 4)
            self.assertEqual(seen[-1], '[user]\nother')


class ContinuationTests(unittest.IsolatedAsyncioTestCase):
    async def test_start_uses_confirmed_ids_snapshot_and_current_model(self):
        actor = SimpleNamespace(continue_turn=mock.AsyncMock(return_value={'request_id': 'next', 'turn_state': 'opaque'}))
        journal = SimpleNamespace(update=mock.Mock())
        context = {'metadata': {'projectId': 'project', 'sandbox_token': 'sandbox'},
                   'conversation_id': 'conversation', 'response_id': 'previous',
                   'snapshot': {'conversation_id': 'conversation', 'codex_session_id': 'session', 'transcript_cursor': 9}}
        start = BrowserContinuation(SimpleNamespace(api=adapter), actor, journal, context, 'gpt-6.1-sol', 'xhigh')
        data, poll = await start.run('new data')
        body = actor.continue_turn.await_args.args[1]
        self.assertEqual(body['previousResponseId'], 'previous')
        self.assertEqual(body['conversationId'], 'conversation')
        self.assertEqual(body['input'][0]['content'][0]['text'], 'new data')
        self.assertEqual(json.loads(body['metadata']['codex_listen_snapshot'])['transcript_cursor'], 9)
        self.assertEqual(body['metadata']['reasoning_effort'], 'xhigh')
        self.assertEqual(poll, {'request_id': 'next', 'turn_state': 'opaque'})

    async def test_browser_gate_allows_one_exact_continuation_body(self):
        engine = SimpleNamespace(api=adapter)
        actor = AccountBrowser(engine, '300', 'token')
        start = SimpleNamespace(sent=False, journal=SimpleNamespace(update=mock.Mock()))
        from multiplex_browser import fingerprint
        body = {'metadata': {'projectId': 'project'}, 'conversationId': 'conversation', 'input': ['new']}
        actor.expected_start = {'sent': False, 'start': start, 'fingerprint': fingerprint(body)}
        def route(payload):
            return SimpleNamespace(request=SimpleNamespace(url=adapter.BASE + adapter.START, method='POST',
                post_data_json=payload), continue_=mock.AsyncMock(), abort=mock.AsyncMock())
        wrong = route(dict(body, conversationId='foreign'))
        await actor.route(wrong)
        wrong.abort.assert_awaited_once()
        exact = route(body)
        await actor.route(exact)
        exact.continue_.assert_awaited_once()
        self.assertTrue(start.sent)
        again = route(body)
        await actor.route(again)
        again.abort.assert_awaited_once()


if __name__ == '__main__':
    unittest.main()
