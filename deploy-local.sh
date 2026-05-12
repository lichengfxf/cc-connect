#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_NAME="cc-connect"
INSTALL_PATH="${INSTALL_PATH:-/usr/bin/cc-connect}"
LOG_OUT="${LOG_OUT:-/tmp/cc-connect.out}"
LOG_ERR="${LOG_ERR:-/tmp/cc-connect.err}"
START_WORKDIR="${START_WORKDIR:-$HOME}"
RESTART_AFTER_DEPLOY="${RESTART_AFTER_DEPLOY:-1}"
USE_MAKE_BUILD="${USE_MAKE_BUILD:-1}"

say() {
  printf '[deploy-local] %s\n' "$*"
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'Missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

require_cmd sudo
require_cmd go

cd "$ROOT_DIR"

if [[ "$USE_MAKE_BUILD" == "1" ]]; then
  require_cmd make
  say "building web assets and binary via make build"
  make build
else
  say "building binary via go build"
  go build -o "$APP_NAME" ./cmd/cc-connect
fi

if [[ ! -x "$ROOT_DIR/$APP_NAME" ]]; then
  printf 'Build did not produce executable: %s\n' "$ROOT_DIR/$APP_NAME" >&2
  exit 1
fi

say "installing binary to $INSTALL_PATH"
sudo install -m 755 "$ROOT_DIR/$APP_NAME" "$INSTALL_PATH"

if [[ "$RESTART_AFTER_DEPLOY" != "1" ]]; then
  say "deployment finished without restart"
  exit 0
fi

running_pids="$(pgrep -x "$APP_NAME" || true)"
if [[ -n "$running_pids" ]]; then
  say "stopping existing $APP_NAME processes: $(echo "$running_pids" | tr '\n' ' ')"
  while IFS= read -r pid; do
    [[ -n "$pid" ]] || continue
    kill "$pid" || true
  done <<< "$running_pids"
  sleep 1
fi

say "starting $INSTALL_PATH"
cd "$START_WORKDIR"
setsid "$INSTALL_PATH" >"$LOG_OUT" 2>"$LOG_ERR" < /dev/null &
new_pid=$!
sleep 2

if ! kill -0 "$new_pid" 2>/dev/null; then
  say "startup failed, showing logs"
  [[ -f "$LOG_ERR" ]] && cat "$LOG_ERR" >&2
  [[ -f "$LOG_OUT" ]] && cat "$LOG_OUT" >&2
  exit 1
fi

say "deployment complete"
say "pid: $new_pid"
say "binary: $INSTALL_PATH"
say "stdout: $LOG_OUT"
say "stderr: $LOG_ERR"
