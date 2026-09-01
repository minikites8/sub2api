#!/bin/sh
set -eu
target="${1:?target directory required}"
mkdir -p "$target"
printf '%s\n' 'restored: baseline marker' > "$target/MODIFIED_FILE"
test "$(cat "$target/MODIFIED_FILE")" = 'restored: baseline marker'
printf '%s\n' 'rollback copy restored successfully'
