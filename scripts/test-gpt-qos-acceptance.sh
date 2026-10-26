#!/usr/bin/env bash
# Only disposable Testcontainers PostgreSQL/Redis and loopback mock upstreams.
set -euo pipefail
TASK_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
TASK_EVIDENCE="${SUB2API_ACCEPTANCE_OUTPUT:-$TASK_ROOT/artifacts/gpt-qos-final-local}"
TASK_BASELINE_REF="${SUB2API_ACCEPTANCE_BASELINE_REF:-21f675138}"
command -v go >/dev/null
command -v docker >/dev/null
docker info >/dev/null
mkdir -p "$TASK_EVIDENCE/baseline-source"
TASK_EVIDENCE="$(cd "$TASK_EVIDENCE" && pwd)"
(cd "$TASK_ROOT/backend" && go build -o "$TASK_EVIDENCE/server" ./cmd/server)
git -C "$TASK_ROOT" archive "$TASK_BASELINE_REF" backend | tar -x -C "$TASK_EVIDENCE/baseline-source"
(cd "$TASK_EVIDENCE/baseline-source/backend" && go build -o "$TASK_EVIDENCE/server-baseline" ./cmd/server)
export SUB2API_ACCEPTANCE_BINARY="$TASK_EVIDENCE/server"
export SUB2API_ACCEPTANCE_BASELINE_BINARY="$TASK_EVIDENCE/server-baseline"
cd "$TASK_ROOT/backend"
# Each command creates and destroys its own real Redis and PostgreSQL containers.
for TASK_TEST in TwoProcesses ProcessCrashLeaseRecovery SSEAfterContentNoReplay TimeoutDiagnostics; do
  export SUB2API_ACCEPTANCE_EVIDENCE="$TASK_EVIDENCE/$TASK_TEST"
  go test -tags=integration ./internal/repository -run "^TestGPTQoSAcceptance${TASK_TEST}\$" -count=1 -v -timeout=5m | tee "$TASK_EVIDENCE/$TASK_TEST.log"
done
go test -tags=integration ./internal/repository -run '^TestGPT(QoSRealRedis|QoSGroupConfig|QualityAudit)' -count=1 -v | tee "$TASK_EVIDENCE/redis-integration.log"
go test -race -tags=unit ./internal/service ./internal/repository ./internal/handler ./internal/handler/admin -run 'Test(GPTQoS|GPTQuality|UpdateAccount|StreamWrittenGuard|FailoverRetryWaitBudget|FailoverClientGone)' -count=1 -json > "$TASK_EVIDENCE/race.jsonl"
printf 'Evidence: %s\n' "$TASK_EVIDENCE"
