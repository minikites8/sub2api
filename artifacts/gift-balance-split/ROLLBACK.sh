#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
TARGET="${1:?target file is required}"
BACKUP="${2:?backup path is required}"

cp -- "$TARGET" "$BACKUP"
cp -- "$SCRIPT_DIR/BASELINE_MIGRATION.sql" "$TARGET"
printf 'restored %s from baseline\n' "$TARGET"
