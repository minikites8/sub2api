"""Render each deployment with Compose to check the gateway/adapter contract."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]


class PrismComposeTests(unittest.TestCase):
    def test_shared_namespace_keys_and_state_for_all_templates(self):
        with tempfile.TemporaryDirectory() as directory:
            env_file = Path(directory) / "empty.env"
            env_file.write_text("", encoding="utf-8")
            for path in sorted((ROOT / "deploy").glob("docker-compose*.yml")):
                for source in (None, "PRISM_ADAPTER_API_KEY", "GATEWAY_PRISM_BROWSER_API_KEY"):
                    with self.subTest(template=path.name, key_source=source):
                        env = dict(os.environ, POSTGRES_PASSWORD="compose-fixture",
                                   DATABASE_HOST="postgres", DATABASE_PASSWORD="compose-fixture",
                                   REDIS_HOST="redis", GATEWAY_PRISM_BROWSER_ENABLED="true")
                        for key in ("PRISM_ADAPTER_API_KEY", "GATEWAY_PRISM_BROWSER_API_KEY"):
                            env.pop(key, None)
                        if source:
                            env[source] = "compose-fixture-" + "x" * 32
                        result = subprocess.run(
                            ["docker", "compose", "--env-file", str(env_file), "-f", str(path),
                             "config", "--format", "json"], env=env, capture_output=True,
                            text=True, encoding="utf-8", check=True)
                        services = json.loads(result.stdout)["services"]
                        gateway, adapter = services["sub2api"], services["prism-adapter"]
                        self.assertEqual("service:sub2api", adapter["network_mode"])
                        self.assertEqual("http://127.0.0.1:8319/v1",
                                         gateway["environment"]["GATEWAY_PRISM_BROWSER_BASE_URL"])
                        self.assertEqual(env.get(source, "") if source else "",
                                         gateway["environment"]["GATEWAY_PRISM_BROWSER_API_KEY"])
                        self.assertEqual(gateway["environment"]["GATEWAY_PRISM_BROWSER_API_KEY"],
                                         adapter["environment"]["PRISM_ADAPTER_API_KEY"])
                        self.assertTrue(adapter["depends_on"]["sub2api"]["restart"])
                        self.assertFalse(adapter.get("ports"))
                        volumes = {volume["target"]: volume for volume in adapter["volumes"]}
                        shared = volumes["/gateway-data"]
                        self.assertTrue(shared["read_only"])
                        gateway_data = next(volume for volume in gateway["volumes"]
                                            if volume["target"] == "/app/data")
                        self.assertEqual(gateway_data["source"], shared["source"])
                        self.assertIn("/var/lib/sub2api-prism", volumes)
                        self.assertIn("seccomp=./prism-seccomp.json", adapter["security_opt"])
                        self.assertIn("no-new-privileges:true", adapter["security_opt"])


if __name__ == "__main__":
    unittest.main()
