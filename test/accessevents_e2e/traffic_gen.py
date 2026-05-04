#!/usr/bin/env python3
"""
traffic_gen.py — Pure-stdlib Modbus TCP traffic generator for MMA2 e2e tests.

No external libraries required.

Scenarios
---------
A) Allowed reads  : 20 x FC3  to Unit 1  → events: status=allowed, action=read
B) Denied writes  : 20 x FC16 to Unit 1  → events: status=denied,  action=write
C) Allowed writes : 20 x FC16 to Unit 2  → events: status=allowed, action=write

The script sends each batch in a tight loop so that the rate-aggregation
window can observe suppression within the 5 s window.

Usage
-----
    python3 traffic_gen.py [host] [port]

Defaults: host=127.0.0.1, port=502
"""

import socket
import struct
import sys
import time

HOST = sys.argv[1] if len(sys.argv) > 1 else "127.0.0.1"
PORT = int(sys.argv[2]) if len(sys.argv) > 2 else 502

_tid = 0


def _next_tid() -> int:
    global _tid
    _tid = (_tid + 1) & 0xFFFF
    return _tid


def _mbap(tid: int, length: int, unit_id: int) -> bytes:
    """Build a 6-byte Modbus Application Header."""
    return struct.pack(">HHHB", tid, 0x0000, length, unit_id)


def fc3_request(unit_id: int, start: int, count: int) -> bytes:
    """FC3 — Read Holding Registers PDU wrapped in MBAP."""
    pdu = struct.pack(">BHH", 0x03, start, count)
    return _mbap(_next_tid(), 1 + len(pdu), unit_id) + pdu


def fc16_request(unit_id: int, start: int, values: list[int]) -> bytes:
    """FC16 — Write Multiple Registers PDU wrapped in MBAP."""
    count = len(values)
    byte_count = count * 2
    regs = b"".join(struct.pack(">H", v) for v in values)
    pdu = struct.pack(">BHHB", 0x10, start, count, byte_count) + regs
    return _mbap(_next_tid(), 1 + len(pdu), unit_id) + pdu


def send_request(sock: socket.socket, frame: bytes, label: str) -> None:
    """Send one Modbus frame and read (and discard) the response."""
    try:
        sock.sendall(frame)
        # Read at most 256 bytes; we only care that MMA2 replied.
        resp = sock.recv(256)
        fc = resp[7] if len(resp) > 7 else 0xFF
        exception = fc & 0x80
        status = "EXCEPTION" if exception else "OK"
        print(f"  {label:45s}  → {status}")
    except Exception as exc:
        print(f"  {label:45s}  → ERROR: {exc}")


def open_connection(unit_id: int) -> socket.socket:
    sock = socket.create_connection((HOST, PORT), timeout=5)
    sock.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)
    print(f"  [connected to {HOST}:{PORT}  unit_id={unit_id}]")
    return sock


def run_scenario_a(repeats: int = 20) -> None:
    """Scenario A — Allowed reads (FC3, Unit 1)."""
    print("\n=== Scenario A — Allowed Reads (FC3, Unit 1) ===")
    sock = open_connection(1)
    for i in range(repeats):
        frame = fc3_request(unit_id=1, start=0, count=4)
        send_request(sock, frame, f"FC3 read  unit=1  iter={i+1:02d}")
        time.sleep(0.05)
    sock.close()


def run_scenario_b(repeats: int = 20) -> None:
    """Scenario B — Denied writes (FC16, Unit 1 — only reads allowed)."""
    print("\n=== Scenario B — Denied Writes (FC16, Unit 1) ===")
    sock = open_connection(1)
    for i in range(repeats):
        frame = fc16_request(unit_id=1, start=0, values=[0xABCD, 0x1234])
        send_request(sock, frame, f"FC16 write unit=1  iter={i+1:02d}  (expect DENY)")
        time.sleep(0.05)
    sock.close()


def run_scenario_c(repeats: int = 20) -> None:
    """Scenario C — Allowed writes (FC16, Unit 2 — full access)."""
    print("\n=== Scenario C — Allowed Writes (FC16, Unit 2) ===")
    sock = open_connection(2)
    for i in range(repeats):
        frame = fc16_request(unit_id=2, start=0, values=[0xBEEF, 0xCAFE])
        send_request(sock, frame, f"FC16 write unit=2  iter={i+1:02d}  (expect ALLOW)")
        time.sleep(0.05)
    sock.close()


def main() -> None:
    print(f"MMA2 Access Events — Traffic Generator")
    print(f"Target: {HOST}:{PORT}")
    print(f"Window: 5 s  |  Repeats per scenario: 20")

    run_scenario_a(20)
    run_scenario_b(20)
    run_scenario_c(20)

    print("\n--- First pass complete. Waiting 6 s for window to expire... ---")
    time.sleep(6)

    print("\n--- Second pass (triggers summary events after window expiry) ---")
    run_scenario_a(5)
    run_scenario_b(5)
    run_scenario_c(5)

    print("\nTraffic generation complete.")


if __name__ == "__main__":
    main()
