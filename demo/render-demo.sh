#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"

# Chromium's setuid sandbox is unavailable in common Nix and container
# environments. VHS still runs locally against the generated demo only.
export VHS_NO_SANDBOX=${VHS_NO_SANDBOX:-true}

if command -v vhs >/dev/null 2>&1; then
  vhs demo/demo.tape
elif command -v nix >/dev/null 2>&1; then
  nix shell nixpkgs#vhs --command vhs demo/demo.tape
else
  printf 'error: VHS is required (https://github.com/charmbracelet/vhs)\n' >&2
  exit 1
fi
