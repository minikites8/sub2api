#!/usr/bin/env sh
set -eu

target=${1:?target path is required}
backup=${2:?backup path is required}
/usr/bin/cp.exe "$backup" "$target"
printf 'restored: %s\n' "$target"
