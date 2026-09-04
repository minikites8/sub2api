#!/usr/bin/env bash
set -euo pipefail

target="${1:-artifacts/manual-user-ban-status/ROLLBACK_TARGET}"
baseline="${2:-artifacts/manual-user-ban-status/BASELINE_FILE}"
cp "$baseline" "$target"
