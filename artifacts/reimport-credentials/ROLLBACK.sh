#!/usr/bin/env bash
set -euo pipefail

ROOT=${1:?usage: ROLLBACK.sh ROOT_DIRECTORY}
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

patch --batch --reverse --strip=1 --directory="$ROOT" --input="$SCRIPT_DIR/DIFF_FILE"
printf 'ROLLBACK_OK root=%s\n' "$ROOT"
