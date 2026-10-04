#!/bin/sh
set -e

# Fix data directory permissions when running as root.
# Docker named volumes / host bind-mounts may be owned by root,
# preventing the non-root sub2api user from writing files.
if [ "$(id -u)" = "0" ]; then
    mkdir -p /app/data
    # Use || true to avoid failure on read-only mounted files (e.g. config.yaml:ro)
    chown -R sub2api:sub2api /app/data 2>/dev/null || true
    # Re-invoke this script as sub2api so the flag-detection below
    # also runs under the correct user.
    exec su-exec sub2api "$0" "$@"
fi

# Compatibility: if the first arg looks like a flag (e.g. --help),
# prepend the default binary so it behaves the same as the old
# ENTRYPOINT ["/app/sub2api"] style.
if [ "${1#-}" != "$1" ]; then
    set -- /app/sub2api "$@"
fi

# Compose shares this private key file with the Prism adapter container.
# Generate it once, so container recreation retains the same bridge identity.
case "${GATEWAY_PRISM_BROWSER_ENABLED:-false}" in
    true|TRUE|True|t|T|1) prism_enabled=true ;;
    *) prism_enabled=false ;;
esac
if [ "$1" = "/app/sub2api" ] && [ "$prism_enabled" = "true" ]; then
    if [ -n "${GATEWAY_PRISM_BROWSER_API_KEY:-}" ] && [ -n "${PRISM_ADAPTER_API_KEY:-}" ] &&
       [ "$GATEWAY_PRISM_BROWSER_API_KEY" != "$PRISM_ADAPTER_API_KEY" ]; then
        echo 'Prism gateway and adapter bridge keys must match' >&2
        exit 1
    fi
    prism_key_dir=${DATA_DIR:-/app/data}/prism-adapter
    prism_key_file=$prism_key_dir/bridge.key
    mkdir -p "$prism_key_dir"
    chmod 700 "$prism_key_dir"
    prism_key=${GATEWAY_PRISM_BROWSER_API_KEY:-${PRISM_ADAPTER_API_KEY:-}}
    if [ -z "$prism_key" ] && [ -f "$prism_key_file" ]; then
        prism_key=$(cat "$prism_key_file")
    fi
    if [ -z "$prism_key" ]; then
        prism_key=$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')
    fi
    case "$prism_key" in
        *[[:space:]]*) echo 'Prism bridge key must contain at least 32 characters without whitespace' >&2; exit 1 ;;
    esac
    if [ "${#prism_key}" -lt 32 ]; then
        echo 'Prism bridge key must contain at least 32 characters without whitespace' >&2
        exit 1
    fi
    prism_original_umask=$(umask)
    umask 077
    prism_key_tmp=$(mktemp "$prism_key_dir/.bridge.XXXXXX")
    printf '%s' "$prism_key" > "$prism_key_tmp"
    chmod 600 "$prism_key_tmp"
    mv -f "$prism_key_tmp" "$prism_key_file"
    umask "$prism_original_umask"
    export GATEWAY_PRISM_BROWSER_API_KEY=$prism_key
    export GATEWAY_PRISM_BROWSER_BASE_URL=${GATEWAY_PRISM_BROWSER_BASE_URL:-http://127.0.0.1:8319/v1}
fi

exec "$@"
