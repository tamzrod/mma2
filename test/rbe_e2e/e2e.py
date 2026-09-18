#!/usr/bin/env python3
"""Process-level RBE v1 end-to-end check against a running mma2 binary.

Drives the real binary over TCP: Raw Ingest to unseal and to change watched
input registers, Modbus FC3 to read values, and a one-byte RBE TCP subscriber.
No memory values are expected on the RBE stream.
"""
import socket
import struct
import sys
import time

MODBUS_ADDR = ("127.0.0.1", 15020)
RBE_ADDR = ("127.0.0.1", 19001)

MAGIC = b"RI"
VER = 0x01
AREA_COILS = 1
AREA_INPUT_REGS = 4

RESP_OK = 0x00


def raw_ingest(sock, unit, area, addr, count, payload):
    hdr = MAGIC + bytes([VER, area]) + struct.pack(">HHH", unit, addr, count)
    sock.sendall(hdr + payload)
    ack = sock.recv(1)
    return ack[0] if ack else None


def regs_payload(vals):
    return b"".join(struct.pack(">H", v) for v in vals)


def modbus_read(sock, unit, fc, addr, count):
    pdu = bytes([fc]) + struct.pack(">HH", addr, count)
    frame = struct.pack(">HHHB", 1, 0, len(pdu) + 1, unit) + pdu
    sock.sendall(frame)
    hdr = recv_exact(sock, 7)
    length = struct.unpack(">H", hdr[4:6])[0]
    return recv_exact(sock, length - 1)


def recv_exact(sock, n):
    buf = b""
    while len(buf) < n:
        chunk = sock.recv(n - len(buf))
        if not chunk:
            raise EOFError("connection closed")
        buf += chunk
    return buf


def expect_silence(rbe, failures, what):
    rbe.settimeout(0.3)
    try:
        data = rbe.recv(16)
        if data:
            failures.append(f"{what}: RBE emitted {data!r}")
    except socket.timeout:
        pass


def main():
    failures = []

    rbe = socket.create_connection(RBE_ADDR, timeout=2)
    # A new RBE connection must be silent (no snapshot / replay).
    expect_silence(rbe, failures, "connect")

    # Ingress classifies each connection on its first bytes, so Raw Ingest and
    # Modbus need separate sockets.
    raw = socket.create_connection(MODBUS_ADDR, timeout=2)

    # 1. Memory starts sealed: a watched input-register change commits but emits nothing.
    if raw_ingest(raw, 1, AREA_INPUT_REGS, 2, 2, regs_payload([100, 200])) != RESP_OK:
        failures.append("sealed raw write was rejected")
    expect_silence(rbe, failures, "sealed write")

    # 2. The unsealing write must not emit (sealed immediately before it).
    if raw_ingest(raw, 1, AREA_COILS, 0, 1, b"\x01") != RESP_OK:
        failures.append("unseal write was rejected")
    expect_silence(rbe, failures, "unseal write")

    # 3. Identical refresh after unseal must not emit.
    raw_ingest(raw, 1, AREA_INPUT_REGS, 2, 2, regs_payload([100, 200]))
    expect_silence(rbe, failures, "identical refresh")

    # 4. A real change inside rule 1 emits exactly one byte 0x01.
    raw_ingest(raw, 1, AREA_INPUT_REGS, 2, 2, regs_payload([101, 200]))
    rbe.settimeout(2)
    try:
        data = recv_exact(rbe, 1)
        if data != b"\x01":
            failures.append(f"rule 1 emitted {data!r}, want b'\\x01'")
    except (socket.timeout, EOFError) as exc:
        failures.append(f"rule 1 event missing: {exc}")

    # 5. A change inside rule 2 emits exactly one byte 0x02.
    raw_ingest(raw, 1, AREA_INPUT_REGS, 4, 2, regs_payload([7, 8]))
    try:
        data = recv_exact(rbe, 1)
        if data != b"\x02":
            failures.append(f"rule 2 emitted {data!r}, want b'\\x02'")
    except (socket.timeout, EOFError) as exc:
        failures.append(f"rule 2 event missing: {exc}")

    # 6. Modbus now allowed (unsealed) and returns the value written via Raw Ingest.
    # RBE rules cover input_registers, which Modbus reads with FC4.
    mb = socket.create_connection(MODBUS_ADDR, timeout=2)
    resp = modbus_read(mb, 1, 4, 2, 2)
    if resp[0] != 4:
        failures.append(f"Modbus FC4 exception after unseal: {resp.hex()}")
    else:
        values = struct.unpack(">HH", resp[2:6])
        if values != (101, 200):
            failures.append(f"Modbus read {values}, want (101, 200)")

    # 7. No extra bytes: the stream is exactly the events we caused.
    expect_silence(rbe, failures, "after events")

    rbe.close()
    raw.close()
    mb.close()

    if failures:
        print("FAIL")
        for f in failures:
            print(" -", f)
        return 1
    print("PASS: process-level RBE v1 checks passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
