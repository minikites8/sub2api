#!/usr/bin/env bash
set -euo pipefail

target="${1:?target path required}"
backup="${2:?backup path required}"
cp -- "$backup" "$target"
printf 'restored %s from %s\n' "$target" "$backup"
