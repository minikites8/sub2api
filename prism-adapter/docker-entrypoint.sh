#!/bin/sh
set -eu

if [ "$(id -u)" = "0" ]; then
    state_dir=${PRISM_ADAPTER_STATE_DIR:-/var/lib/sub2api-prism}
    mkdir -p "$state_dir"
    chown sub2api:sub2api "$state_dir"
    chmod 700 "$state_dir"
    exec gosu sub2api "$0" "$@"
fi

exec "$@"
