#!/usr/bin/env bash
# Process-level RBE v1 end-to-end check.
#
# Builds cmd/mma2, starts it on test/rbe_e2e/config.yaml, drives the real
# binary over TCP (Raw Ingest + Modbus + one-byte RBE subscriber) and stops it.
# Requires Go and python3. Uses fixed localhost ports 15020/19001.
set -u

cd "$(dirname "$0")/../.." || exit 1

BIN="$(mktemp -d)/mma2"
LOG="$(mktemp)"
go build -o "$BIN" ./cmd/mma2 || { echo "build failed"; exit 1; }

"$BIN" test/rbe_e2e/config.yaml > "$LOG" 2>&1 &
PID=$!
trap 'kill "$PID" 2>/dev/null' EXIT

# Wait for both listeners.
for _ in $(seq 1 50); do
    if grep -q "ingress started" "$LOG"; then
        break
    fi
    sleep 0.1
done

python3 test/rbe_e2e/e2e.py
STATUS=$?

if [ "$STATUS" -ne 0 ]; then
    echo "--- mma2 log ---"
    cat "$LOG"
fi
exit "$STATUS"
