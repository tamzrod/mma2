#!/usr/bin/env bash
# test/persistence_manual/run.sh
# Start MMA2 for interactive Modpoll tests.
# Does NOT delete snapshots: stop and run again to verify persistence.
# Current branch still uses the legacy root-level persistence config.yaml.
# Replace config.yaml with per-memory persistence after migration C01-C06.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CONFIG="test/persistence_manual/config.yaml"

cd "$REPO_ROOT"

if ! command -v go >/dev/null 2>&1; then
  echo "ERROR: Go is not installed or not in PATH." >&2
  exit 1
fi

BIN_DIR="$(mktemp -d)"
BIN="$BIN_DIR/mma2"
cleanup() {
  rm -rf "$BIN_DIR"
}
trap cleanup EXIT

echo "Building MMA2 from current branch..."
go build -o "$BIN" ./cmd/mma2

echo
echo "MMA2 persistence manual test"
echo "  Endpoint: 127.0.0.1:15030"
echo "  Unit ID:  1"
echo "  Config:   $CONFIG"
echo "  Storage:  test/persistence_manual/snapshots"
echo
echo "Leave this terminal open. Use Modpoll in another terminal."
echo "Stop MMA2 with Ctrl+C, then rerun this script to test restoration."
echo "Snapshot files are NOT reset or deleted by this script."
echo

# exec ensures Ctrl+C and termination signals reach MMA2 directly.
"$BIN" "$CONFIG"
