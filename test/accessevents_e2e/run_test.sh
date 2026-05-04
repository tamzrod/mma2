#!/usr/bin/env bash
# =============================================================================
# run_test.sh — MMA2 Access Events End-to-End Test (Linux / macOS)
#
# What it does
# ------------
# 1. Builds the mma2 binary (go build).
# 2. Starts mma2 with test_config.yaml in the background.
# 3. Connects to /events and saves the NDJSON stream to evidence/events.ndjson.
# 4. Runs traffic_gen.py to produce allowed + denied access events.
# 5. Waits for the aggregation window to expire (7 s).
# 6. Prints the captured NDJSON as evidence and validates key fields.
# 7. Cleans up.
#
# Requirements
# ------------
#   go  1.21+  (or use docker compose — see docker-compose.yml)
#   python3    (stdlib only, no pip install required)
#   curl       (for streaming /events)
#   nc / bash  (for readiness check)
#
# Usage
# -----
#   cd test/accessevents_e2e
#   chmod +x run_test.sh
#   ./run_test.sh
#
# Evidence is written to:
#   test/accessevents_e2e/evidence/events.ndjson
#   test/accessevents_e2e/evidence/mma2.log
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
EVIDENCE_DIR="${SCRIPT_DIR}/evidence"
CONFIG="${SCRIPT_DIR}/test_config.yaml"
BINARY="${SCRIPT_DIR}/mma2"
MMA2_LOG="${EVIDENCE_DIR}/mma2.log"
EVENTS_FILE="${EVIDENCE_DIR}/events.ndjson"
MODBUS_PORT="${MODBUS_PORT:-5020}"   # use 5020 locally; override to 502 in Docker
HTTP_PORT="${HTTP_PORT:-9090}"
MMA2_PID=""
EVENTS_PID=""

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
cleanup() {
    echo ""
    echo "=== Cleanup ==="
    if [[ -n "${EVENTS_PID}" ]]; then
        kill "${EVENTS_PID}" 2>/dev/null || true
    fi
    if [[ -n "${MMA2_PID}" ]]; then
        kill "${MMA2_PID}" 2>/dev/null || true
    fi
    echo "Done."
}
trap cleanup EXIT

banner() { echo ""; echo ">>> $*"; echo ""; }

wait_for_port() {
    local port=$1 retries=20 delay=0.5
    echo -n "    Waiting for :${port}"
    for ((i=0; i<retries; i++)); do
        if bash -c "exec 3<>/dev/tcp/127.0.0.1/${port}" 2>/dev/null; then
            exec 3>&-
            echo " — ready"
            return 0
        fi
        echo -n "."
        sleep "${delay}"
    done
    echo " — TIMED OUT"
    return 1
}

# ---------------------------------------------------------------------------
# Step 1 — Build
# ---------------------------------------------------------------------------
banner "Step 1 — Build mma2"
cd "${REPO_ROOT}"
go build -o "${BINARY}" ./cmd/mma2
echo "    Binary: ${BINARY}"

# ---------------------------------------------------------------------------
# Step 2 — Prepare evidence directory
# ---------------------------------------------------------------------------
mkdir -p "${EVIDENCE_DIR}"
: > "${EVENTS_FILE}"
: > "${MMA2_LOG}"

# ---------------------------------------------------------------------------
# Step 3 — Start mma2
# ---------------------------------------------------------------------------
banner "Step 3 — Start mma2"
"${BINARY}" "${CONFIG}" > "${MMA2_LOG}" 2>&1 &
MMA2_PID=$!
echo "    mma2 PID: ${MMA2_PID}"

wait_for_port "${MODBUS_PORT}"
wait_for_port "${HTTP_PORT}"

# ---------------------------------------------------------------------------
# Step 4 — Attach /events stream in background
# ---------------------------------------------------------------------------
banner "Step 4 — Connect to /events (background capture)"
curl -sSN "http://127.0.0.1:${HTTP_PORT}/events" >> "${EVENTS_FILE}" &
EVENTS_PID=$!
echo "    curl PID: ${EVENTS_PID}   output: ${EVENTS_FILE}"
sleep 0.5   # let the subscription register

# ---------------------------------------------------------------------------
# Step 5 — Generate traffic
# ---------------------------------------------------------------------------
banner "Step 5 — Generate traffic (traffic_gen.py)"
python3 "${SCRIPT_DIR}/traffic_gen.py" 127.0.0.1 "${MODBUS_PORT}" 2>&1 | tee "${EVIDENCE_DIR}/traffic_gen.log"

# ---------------------------------------------------------------------------
# Step 6 — Wait for window expiry so summaries appear
# ---------------------------------------------------------------------------
banner "Step 6 — Wait 7 s for 5 s aggregation window to expire"
sleep 7
echo "    Done waiting."

# ---------------------------------------------------------------------------
# Step 7 — Show captured events
# ---------------------------------------------------------------------------
banner "Step 7 — Captured /events output (evidence/events.ndjson)"
echo "--------------------------------------------------------------------"
cat "${EVENTS_FILE}" | python3 -c "
import sys, json
lines = [l.strip() for l in sys.stdin if l.strip()]
for line in lines:
    obj = json.loads(line)
    print(json.dumps(obj, separators=(',', ':')))
"
echo "--------------------------------------------------------------------"
echo ""
echo "Total events captured: $(wc -l < "${EVENTS_FILE}")"

# ---------------------------------------------------------------------------
# Step 8 — Validate
# ---------------------------------------------------------------------------
banner "Step 8 — Validation"
python3 - "${EVENTS_FILE}" <<'PYEOF'
import json, sys

with open(sys.argv[1]) as f:
    lines = [l.strip() for l in f if l.strip()]

events = [json.loads(l) for l in lines]
total = len(events)

allowed_reads   = [e for e in events if e["status"] == "allowed" and e["action"] == "read"]
denied_writes   = [e for e in events if e["status"] == "denied"  and e["action"] == "write"]
allowed_writes  = [e for e in events if e["status"] == "allowed" and e["action"] == "write"]
summaries       = [e for e in events if e.get("count", 0) > 0]

print(f"  Total events       : {total}")
print(f"  Allowed reads      : {len(allowed_reads)}")
print(f"  Denied writes      : {len(denied_writes)}")
print(f"  Allowed writes     : {len(allowed_writes)}")
print(f"  Summary events     : {len(summaries)}")
print()

fails = []
if not allowed_reads:   fails.append("FAIL: no allowed-read events")
if not denied_writes:   fails.append("FAIL: no denied-write events")
if not allowed_writes:  fails.append("FAIL: no allowed-write events")
if not summaries:       fails.append("FAIL: no summary events (count > 0)")

for f in fails:
    print(f)

if not fails:
    print("  ALL CHECKS PASSED ✓")
    sys.exit(0)
else:
    sys.exit(1)
PYEOF

# ---------------------------------------------------------------------------
banner "Test complete — evidence in ${EVIDENCE_DIR}/"
# ---------------------------------------------------------------------------
