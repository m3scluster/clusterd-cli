#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
DELAY=${DEMO_DELAY:-1.8}
TMPDIR_DEMO=$(mktemp -d)
MOCK_PID=

cleanup() {
  if [[ -n $MOCK_PID ]]; then
    kill "$MOCK_PID" 2>/dev/null || true
    wait "$MOCK_PID" 2>/dev/null || true
  fi
  rm -rf "$TMPDIR_DEMO"
}
trap cleanup EXIT

cd "$ROOT"
make all >/dev/null

python3 demo/mock_master.py "$TMPDIR_DEMO/port" >"$TMPDIR_DEMO/mock.log" 2>&1 &
MOCK_PID=$!
for _ in {1..100}; do
  [[ -s $TMPDIR_DEMO/port ]] && break
  kill -0 "$MOCK_PID" 2>/dev/null || {
    printf 'synthetic Mesos master failed to start\n' >&2
    exit 1
  }
  sleep 0.05
done
[[ -s $TMPDIR_DEMO/port ]] || {
  printf 'timed out waiting for synthetic Mesos master\n' >&2
  exit 1
}

cat >"$TMPDIR_DEMO/config.toml" <<TOML
[master]
address = "http://127.0.0.1:$(<"$TMPDIR_DEMO/port")"
ssl_verify = true
TOML
export MESOS_CLI_CONFIG="$TMPDIR_DEMO/config.toml"

if [[ -t 1 && -z ${NO_COLOR:-} ]]; then
  blue=$'\033[1;34m'
  red=$'\033[1;31m'
  green=$'\033[1;32m'
  dim=$'\033[2m'
  reset=$'\033[0m'
else
  blue= red= green= dim= reset=
fi

pause() { sleep "$DELAY"; }
scene() {
  if [[ -t 1 ]]; then
    printf '\033[2J\033[H'
  fi
  printf '%sGo Mesos CLI%s  %s// %s%s\n\n' "$blue" "$reset" "$dim" "$1" "$reset"
}
run() {
  printf '%s$%s %s\n' "$red" "$reset" "$*"
  "$@"
}

scene "Mesos agent inventory"
run ./mesos-cli agent list
pause

scene "active and archived Mesos frameworks"
run ./mesos-cli framework list --all
pause

scene "running and completed Mesos tasks"
run ./mesos-cli task list --all
pause

scene "inspect a Mesos task"
run ./mesos-cli task inspect synthetic-task-running
printf '\n%s✓ core Mesos commands completed%s\n' "$green" "$reset"
pause
