import asyncio
from concurrent.futures import ThreadPoolExecutor
import threading
import unittest

import server as adapter
from multiplex_runtime import Admission, AsyncBrowserWorker


class WorkerHealthTests(unittest.TestCase):
    def test_serial_worker_reports_occupied_channel_and_shutdown(self):
        entered, release = threading.Event(), threading.Event()

        class Browser:
            def run(self, *args):
                entered.set()
                release.wait(3)
                return "request", "answer"

            def expire(self):
                pass

            def close(self):
                pass

        worker = adapter.BrowserWorker(Browser)
        try:
            self.assertTrue(worker.ready.wait(3))
            with ThreadPoolExecutor(max_workers=1) as pool:
                future = pool.submit(worker.run, "fixture")
                try:
                    self.assertTrue(entered.wait(3))
                    self.assertEqual(worker.health(), {"status": "ok", "mode": "browser",
                                                      "active": 1, "queued": 0, "max_inflight": 1})
                finally:
                    release.set()
                self.assertEqual(future.result(3), ("request", "answer"))
            self.assertEqual(worker.health()["active"], 0)
        finally:
            release.set()
            worker.close()
        self.assertEqual(worker.health()["status"], "unavailable")

    def test_multiplex_deadline_releases_admission_and_accepts_next_turn(self):
        class Engine:
            def __init__(self):
                self.admission = Admission(adapter, active=1, per_account=1, queued=1)

            async def run(self, prompt):
                async with self.admission.enter("fixture", None):
                    if prompt == "slow":
                        await asyncio.sleep(10)
                    return "request", prompt

            async def prune(self):
                pass

            async def close(self):
                pass

        worker = AsyncBrowserWorker(Engine, adapter, request_timeout=0.1)
        try:
            with self.assertRaises(adapter.AdapterError) as error:
                worker.run("slow")
            self.assertEqual((error.exception.status, error.exception.code), (504, "unknown_outcome"))
            self.assertEqual(worker.health()["status"], "ok")
            self.assertEqual((worker.health()["active"], worker.health()["queued"]), (0, 0))
            self.assertEqual(worker.run("next"), ("request", "next"))
        finally:
            worker.close()
        self.assertEqual(worker.health()["status"], "unavailable")

    def test_failed_worker_reports_unavailable(self):
        def fail():
            raise ValueError("private fixture")

        worker = AsyncBrowserWorker(fail, adapter)
        try:
            self.assertTrue(worker.ready.wait(3))
            self.assertEqual(worker.health()["status"], "unavailable")
            self.assertNotIn("private fixture", str(worker.health()))
        finally:
            worker.close()


if __name__ == "__main__":
    unittest.main()
