#!/usr/bin/env bash
set -Eeuo pipefail

# Run from the MMA2 repository on the Legion, not on the Raspberry Pi.
# Override TARGET/PORT/UNIT/DURATION/WARMUP if necessary.
TARGET="${TARGET:-10.5.1.8}"
PORT="${PORT:-15020}"
UNIT="${UNIT:-1}"
DURATION="${DURATION:-30s}"
WARMUP="${WARMUP:-5s}"

cd "$(dirname "${BASH_SOURCE[0]}")/.."
command -v go >/dev/null || { echo 'Go is required on the Legion.' >&2; exit 1; }
command -v timeout >/dev/null || { echo 'GNU timeout is required.' >&2; exit 1; }

printf 'Checking TCP connectivity to %s:%s ...\n' "$TARGET" "$PORT"
if ! timeout 5 bash -c 'echo > /dev/tcp/"$1"/"$2"' _ "$TARGET" "$PORT" 2>/dev/null; then
  echo "Cannot reach $TARGET:$PORT. Check Pi container port mapping, network, and firewall." >&2
  exit 1
fi

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
base="benchmark/results/raspberry-${stamp}"
mkdir -p "$base"
{
  echo "target=$TARGET:$PORT"
  echo "unit=$UNIT"
  echo "duration=$DURATION"
  echo "warmup=$WARMUP"
  echo "utc=$stamp"
  echo "legion_commit=$(git rev-parse HEAD 2>/dev/null || echo unknown)"
  echo "legion_go=$(go version)"
  echo "legion_kernel=$(uname -srmo)"
} > "$base/environment.txt"

# The generator reads FC3 holding register 0, quantity 1; run only against test MMA2.
for clients in 1 2 4 8; do
  out="$base/clients-${clients}"
  echo "=== Raspberry Pi: ${clients} client(s), ${DURATION} measured ==="
  go run ./benchmark/tools \
    -addr "$TARGET:$PORT" \
    -unit "$UNIT" \
    -clients "$clients" \
    -duration "$DURATION" \
    -warmup "$WARMUP" \
    -out "$out"
done

printf '\nResults: %s\n' "$base"
for file in "$base"/clients-*/summary.csv; do
  echo "--- $file"
  cat "$file"
done
