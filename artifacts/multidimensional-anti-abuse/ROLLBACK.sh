#!/usr/bin/env bash
set -euo pipefail

artifact_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
target="${1:-${artifact_dir}/MODIFIED_FILE.json}"
cp "${artifact_dir}/ORIGINAL_FILE.json" "${target}"
printf 'restored:%s\n' "${target}"
