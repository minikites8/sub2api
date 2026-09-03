#!/usr/bin/env bash
set -euo pipefail

target="${1:-artifacts/manual-user-ban/ROLLBACK_TARGET}"
repo="${2:-.}"
git -C "$repo" show "HEAD:backend/internal/handler/admin/user_handler.go" > "$target"
