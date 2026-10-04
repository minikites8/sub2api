#!/bin/sh
# A gateway process fixture for the container bootstrap smoke; no OAuth traffic.
set -eu
python -c 'import hashlib, os; from pathlib import Path; key=os.environ["GATEWAY_PRISM_BROWSER_API_KEY"]; assert len(key)>=32; Path("/app/data/gateway-key.sha256").write_text(hashlib.sha256(key.encode()).hexdigest(), encoding="utf-8")'
exec python -c 'import time; time.sleep(3600)'
