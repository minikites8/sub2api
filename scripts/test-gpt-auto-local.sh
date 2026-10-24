#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
export COMPOSE_PROJECT_NAME=gpt-auto-local
env_file=.env.gpt-auto-local
evidence=artifacts/gpt-auto-local
mkdir -p "$evidence"
init() {
  python3 - <<'PY'
import hashlib, os, pathlib, secrets, socket, subprocess
p = pathlib.Path('.env.gpt-auto-local')
if not p.exists():
    for port in (18080, 18081, 18082):
        with socket.socket() as s:
            s.bind(('127.0.0.1', port))
    env = pathlib.Path('.env.gpt-auto-local.example').read_text()
    for name in ('DB_PASSWORD', 'ADMIN_PASSWORD', 'JWT_SECRET', 'TOTP_KEY'):
        env = env.replace('LOCAL_'+name+'=GENERATE_WITH_INIT', 'LOCAL_'+name+'='+secrets.token_hex(32))
    fd = os.open(p, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    with os.fdopen(fd, 'w') as f: f.write(env)
files = subprocess.check_output(['git', 'ls-files', '-co', '--exclude-standard', '-z']).split(b'\0')
h = hashlib.sha256()
for raw in sorted(set(files)):
    path = pathlib.Path(os.fsdecode(raw))
    if raw and path.is_file() and (str(path).startswith(('backend/', 'frontend/', 'docs/legal/')) or str(path) in ('Dockerfile', '.dockerignore')):
        h.update(raw); h.update(path.read_bytes())
pathlib.Path('artifacts/gpt-auto-local/source-manifest.sha256').write_text(h.hexdigest()+'\n')
lines = [l for l in p.read_text().splitlines() if not l.startswith('LOCAL_SOURCE_HASH=')]
p.write_text('\n'.join(lines)+'\nLOCAL_SOURCE_HASH='+h.hexdigest()+'\n')
print('Local credentials: .env.gpt-auto-local (0600); source SHA256:', h.hexdigest())
# Jev requires HTTPS. Trust this development CA only inside the test backend.
tls = pathlib.Path('artifacts/gpt-auto-local/tls')
tls.mkdir(exist_ok=True)
if not (tls/'mock.crt').exists():
    old = os.umask(0o077)
    try:
        commands = [
            ['openssl','req','-x509','-newkey','rsa:2048','-nodes','-days','30','-subj','/CN=GPT Auto Local Test CA','-keyout',str(tls/'ca.key'),'-out',str(tls/'ca.crt')],
            ['openssl','req','-newkey','rsa:2048','-nodes','-subj','/CN=mock','-keyout',str(tls/'mock.key'),'-out',str(tls/'mock.csr')],
            ['openssl','x509','-req','-in',str(tls/'mock.csr'),'-CA',str(tls/'ca.crt'),'-CAkey',str(tls/'ca.key'),'-CAcreateserial','-days','30','-extfile',str(tls/'server.ext'),'-out',str(tls/'mock.crt')],
        ]
        (tls/'server.ext').write_text('subjectAltName=DNS:mock\nextendedKeyUsage=serverAuth\nbasicConstraints=CA:FALSE\n')
        for cmd in commands: subprocess.run(cmd, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        (tls/'ca.crt').chmod(0o644)
        (tls/'mock.crt').chmod(0o644)
    finally:
        os.umask(old)
PY
}
dc() { docker compose --env-file "$env_file" -p gpt-auto-local -f compose.gpt-auto-local.yml "$@"; }
logs() { dc logs --no-color > "$evidence/containers.log"; dc ps > "$evidence/compose-ps.txt"; }
case "${1:-all}" in
  init) init ;;
  build) init; dc config --quiet; dc build backend 2>&1 | tee "$evidence/build.log" ;;
  start) dc config --quiet; dc up -d --wait --wait-timeout 240 postgres redis mock; dc up -d --wait --wait-timeout 300 backend web; logs ;;
  smoke|full)
    status=0
    python3 scripts/gpt-auto-local/verify.py "$1" || status=$?
    logs
    exit "$status"
    ;;
  phase2)
    status=0
    python3 scripts/gpt-auto-local/verify_phase2.py || status=$?
    logs
    cp "$evidence/containers.log" artifacts/gpt-auto-phase2/containers.log 2>/dev/null || true
    exit "$status"
    ;;
  logs) logs ;;
  all)
    "$0" build
    "$0" start
    "$0" full
    ;;
  *) echo "Usage: $0 {init|build|start|smoke|full|phase2|logs|all}" >&2; exit 2 ;;
esac
