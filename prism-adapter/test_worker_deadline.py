import asyncio
import time
import unittest

from multiplex_runtime import AsyncBrowserWorker
from test_server import adapter


class WorkerDeadlineTests(unittest.TestCase):
    def test_catalog_turns_share_the_remaining_request_deadline(self):
        class Engine:
            calls = 0
            cancelled = False

            async def run(inner, prompt):
                inner.calls += 1
                if prompt == 'inspect':
                    return 'first', 'definition requested'
                try:
                    await asyncio.Event().wait()
                except asyncio.CancelledError:
                    inner.cancelled = True
                    raise

            async def prune(inner):
                pass

            async def close(inner):
                pass

        worker = AsyncBrowserWorker(Engine, adapter)
        try:
            worker.ready.wait(5)
            deadline = time.monotonic() + 0.2
            self.assertEqual(worker.run_until(deadline, 'inspect'), ('first', 'definition requested'))
            with self.assertRaises(adapter.AdapterError) as raised:
                worker.run_until(deadline, 'call')
            self.assertEqual((raised.exception.status, raised.exception.code), (504, 'unknown_outcome'))
            self.assertEqual(worker.engine.calls, 2)
            self.assertTrue(worker.engine.cancelled)
            with self.assertRaises(adapter.AdapterError) as expired:
                worker.run_until(deadline, 'expired')
            self.assertTrue(expired.exception.not_submitted)
            self.assertEqual(worker.engine.calls, 2)
        finally:
            worker.close()


if __name__ == '__main__':
    unittest.main()
