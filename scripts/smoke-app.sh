#!/bin/bash
# Starts an app executable, checks that it is still running after N seconds
# (default 8), then stops it. A crash or an exit before that fails the check
# and prints what the app wrote.
# Usage: scripts/smoke-app.sh <executable> [seconds]
set -uo pipefail

exe=${1:?usage: smoke-app.sh <executable> [seconds]}
secs=${2:-8}
[ -x "$exe" ] || { echo "::error::not an executable file: $exe"; exit 2; }

out=$(mktemp)
"$exe" > "$out" 2>&1 &
pid=$!

stop() {
  kill "$pid" 2> /dev/null || return 0
  for _ in 1 2 3 4 5; do
    kill -0 "$pid" 2> /dev/null || return 0
    sleep 1
  done
  kill -9 "$pid" 2> /dev/null || true
}

for ((i = 1; i <= secs; i++)); do
  sleep 1
  if ! kill -0 "$pid" 2> /dev/null; then
    code=0
    wait "$pid" || code=$?
    echo "::error::the app exited after ${i}s with status $code"
    echo "--- output of the app"
    cat "$out"
    rm -f "$out"
    exit 1
  fi
done

stop
wait "$pid" 2> /dev/null
rm -f "$out"
echo "the app was still running after ${secs}s"
