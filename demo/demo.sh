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
cp ./mesos-cli "$TMPDIR_DEMO/clusterd-cli"
CLI="$TMPDIR_DEMO/clusterd-cli"

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

[compose.synthetic-compose]
principal = "synthetic-compose-user"
secret = "synthetic-compose-secret"
ssl_verify = true

[m3s.synthetic-m3s]
principal = "synthetic-m3s-user"
secret = "synthetic-m3s-secret"
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
  printf '%sClusterD CLI%s  %s// %s%s\n\n' "$blue" "$reset" "$dim" "$1" "$reset"
}
run_cli() {
  printf '%s$%s ./clusterd-cli %s\n' "$red" "$reset" "$*"
  "$CLI" "$@"
}

scene "Mesos agent inventory"
run_cli agent list
pause

scene "active and archived Mesos frameworks"
run_cli framework list --all
pause

scene "running and completed Mesos tasks"
run_cli task list --all
pause

scene "Mesos Compose services"
run_cli compose list synthetic-compose
pause

scene "Mesos M3S cluster health"
run_cli m3s status synthetic-m3s --m3s --kubernetes
pause

scene "inspect a Mesos task"
run_cli task inspect synthetic-task-running
printf '\n%s✓ Mesos, Mesos Compose, and Mesos M3S commands completed%s\n' "$green" "$reset"
pause
