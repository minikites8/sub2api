import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest import mock


spec = importlib.util.spec_from_file_location("docker_runtime", Path(__file__).with_name("docker_runtime.py"))
runtime = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runtime)


class DockerRuntimeTests(unittest.TestCase):
    def test_shared_key_survives_container_bootstraps(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "bridge.key"
            path.write_text("bridge-secret-" + "x" * 32, encoding="utf-8")
            first = runtime.wait_for_key(path, timeout=0)
            self.assertEqual(first, runtime.wait_for_key(path, timeout=0))

    def test_read_waits_for_gateway_to_create_key(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "bridge.key"
            key = "x" * 64
            with mock.patch.object(runtime.time, "sleep", side_effect=lambda _: path.write_text(key, encoding="utf-8")):
                self.assertEqual(key, runtime.wait_for_key(path))

    def test_missing_or_invalid_keys_fail_before_adapter_start(self):
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaisesRegex(SystemExit, "missing"):
                runtime.wait_for_key(Path(directory) / "absent", timeout=0)
            for key in ("", "short", "x" * 64 + "\r", "x" * 64 + " y", "x" * 64 + "\x00"):
                with self.subTest(length=len(key)), self.assertRaises(SystemExit):
                    runtime.validate_key(key)

    def test_key_errors_keep_secrets_out_of_diagnostics(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "bridge.key"
            secret = "private-secret"
            path.write_text(secret, encoding="utf-8")
            with self.assertRaises(SystemExit) as caught:
                runtime.wait_for_key(path, timeout=0)
            self.assertNotIn(secret, str(caught.exception))


if __name__ == "__main__":
    unittest.main()
