"""Exercise private loopback, persisted bridge keys and real sandboxed Chromium."""
import argparse
import json
from pathlib import Path
import subprocess
import time
import uuid


def docker(*arguments):
    return subprocess.check_output(["docker", *arguments], text=True).strip()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--image", required=True)
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[2]
    token = uuid.uuid4().hex[:12]
    gateway, adapter = f"prism-gateway-{token}", f"prism-adapter-{token}"
    data, state = f"prism-data-{token}", f"prism-state-{token}"
    profile = str(root / "deploy/prism-seccomp.json")
    gateway_command = (
        'ln -s "$(command -v gosu)" /usr/local/bin/su-exec; '
        'mkdir -p /app; cp /tests/gateway-entrypoint.sh /app/gateway-entrypoint.sh; '
        'cp /tests/gateway-fixture.sh /app/sub2api; chmod 755 /app/gateway-entrypoint.sh /app/sub2api; '
        'exec /app/gateway-entrypoint.sh /app/sub2api'
    )
    health = "import json, urllib.request; status=json.load(urllib.request.urlopen('http://127.0.0.1:8319/health', timeout=3)); assert status['mode']=='multiplex' and status['max_inflight']==4"
    key_hash = "from pathlib import Path; print(Path('/app/data/gateway-key.sha256').read_text())"
    original_hash = None
    try:
        for _ in range(2):
            docker("run", "-d", "--name", gateway, "--security-opt", "no-new-privileges:true",
                   "-e", "GATEWAY_PRISM_BROWSER_ENABLED=true", "-v", f"{data}:/app/data",
                   "-v", f"{root / 'deploy/docker-entrypoint.sh'}:/tests/gateway-entrypoint.sh:ro",
                   "-v", f"{root / 'deploy/tests/prism_gateway_fixture.sh'}:/tests/gateway-fixture.sh:ro",
                   "--entrypoint", "sh", args.image, "-ec", gateway_command)
            docker("run", "-d", "--name", adapter, "--network", f"container:{gateway}",
                   "--security-opt", "no-new-privileges:true", "--security-opt", f"seccomp={profile}",
                   "--shm-size", "256m", "--memory", "900m", "--memory-swap", "900m", "--pids-limit", "256",
                   "-v", f"{data}:/gateway-data:ro", "-v", f"{state}:/var/lib/sub2api-prism",
                   "-v", f"{root / 'prism-adapter'}:/tests:ro", args.image)
            deadline = time.monotonic() + 90
            while True:
                try:
                    docker("exec", gateway, "python", "-c", health)
                    break
                except subprocess.CalledProcessError:
                    if time.monotonic() >= deadline:
                        raise RuntimeError("Prism adapter failed loopback readiness") from None
                    time.sleep(1)
            current_hash = docker("exec", gateway, "python", "-c", key_hash)
            if original_hash is None:
                original_hash = current_hash
            assert current_hash == original_hash, "gateway key changed during container recreation"
            # Authenticate a deliberately unsupported model, before any upstream call.
            authentication = """import json, urllib.request, urllib.error
from pathlib import Path
key=Path('/app/data/prism-adapter/bridge.key').read_text()
request=urllib.request.Request('http://127.0.0.1:8319/v1/responses', data=b'{"model":"unsupported","input":"fixture"}', headers={'Authorization':'Bearer '+key,'X-Prism-Account-ID':'1','X-Prism-OAuth-Token':'fixture','Content-Type':'application/json'})
try:
    urllib.request.urlopen(request)
    raise AssertionError('unsupported model was accepted')
except urllib.error.HTTPError as error:
    assert error.code == 422
    assert json.load(error)['error']['type'] == 'unsupported_model'
print('Prism bridge authentication passed')
"""
            print(docker("exec", "--user", "1000", gateway, "python", "-c", authentication))
            if _ == 0:
                paths = json.loads(docker("exec", "--user", "1000", adapter, "python", "-c",
                                        "import json, docker_runtime; print(json.dumps([str(p) for p in docker_runtime.browser_paths()]))"))
                print(docker("exec", "--user", "1000", "-e", f"CHROME_DEVEL_SANDBOX={paths[1]}", adapter,
                             "python", "/tests/smoke_browser.py", "--chrome", paths[0]))
                print(docker("exec", "--user", "1000", "-e", f"CHROME_DEVEL_SANDBOX={paths[1]}", adapter,
                             "python", "/tests/smoke_multiplex.py", "--chrome", paths[0],
                             "--concurrency", "4", "--rounds", "2", "--poll-failures", "1"))
                print(docker("exec", "--user", "1000", "-e", f"CHROME_DEVEL_SANDBOX={paths[1]}", adapter,
                             "python", "/tests/smoke_client_tools.py", "--chrome", paths[0],
                             "--catalog-size", "512"))
                print(docker("exec", "--user", "1000", "-e", f"CHROME_DEVEL_SANDBOX={paths[1]}", adapter,
                             "python", "/tests/smoke_client_tools.py", "--chrome", paths[0],
                             "--catalog-size", "512", "--unique-catalog"))
                print(docker("exec", "--user", "1000", adapter, "python", "-m", "unittest",
                             "discover", "-s", "/tests", "-p", "test_*.py"))
                docker("exec", "--user", "1000", adapter, "python", "-c",
                       "from pathlib import Path; Path('/var/lib/sub2api-prism/container-recreation.fixture').write_text('retained')")
            else:
                assert docker("exec", "--user", "1000", adapter, "cat", "/var/lib/sub2api-prism/container-recreation.fixture") == "retained"
            docker("rm", "-f", adapter, gateway)
        print("Prism Docker bootstrap, sandbox, bridge and persistence smoke passed")
    except Exception:
        for name in (gateway, adapter):
            subprocess.run(["docker", "logs", name], check=False)
        raise
    finally:
        subprocess.run(["docker", "rm", "-f", adapter, gateway], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        subprocess.run(["docker", "volume", "rm", data, state], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


if __name__ == "__main__":
    main()
