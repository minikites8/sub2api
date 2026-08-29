#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:?usage: ROLLBACK.sh <workspace-root>}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

patch --batch --forward --reverse --directory "$ROOT" --strip=1 --input "$SCRIPT_DIR/DIFF_FILE"
printf 'ROLLBACK_OK root=%s\n' "$ROOT"
