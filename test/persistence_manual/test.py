#!/usr/bin/env python3
"""Process-level MMA2 persistence test using Python standard library only.

Run from any directory: python3 test/persistence_manual/test.py
Requires Go and Python 3. Uses localhost port 15030.
WARNING: resets ONLY test/persistence_manual/snapshots (test-owned data).
"""
from pathlib import Path
import shutil
import signal
import socket
import struct
import subprocess
import sys
import tempfile
import time

HERE = Path(__file__).resolve().parent
ROOT = HERE.parent.parent
SNAPS = HERE / "snapshots"
PORT = 15030
UNIT = 1
TX = 0


def request(fc, body):
    global TX
    TX = (TX + 1) & 0xFFFF
    pdu = bytes([fc]) + body
    frame = struct.pack(">HHHB", TX, 0, len(pdu) + 1, UNIT) + pdu
    with socket.create_connection(("127.0.0.1", PORT), timeout=2) as s:
        s.settimeout(2)
        s.sendall(frame)
        hdr = recv_exact(s, 7)
        tx, proto, length, unit = struct.unpack(">HHHB", hdr)
        assert (tx, proto, unit) == (TX, 0, UNIT), (tx, proto, unit)
        data = recv_exact(s, length - 1)
        assert data and not (data[0] & 0x80), f"Modbus exception: {data.hex()}"
        assert data[0] == fc, f"wrong function: {data.hex()}"
        return data[1:]


def recv_exact(s, count):
    out = bytearray()
    while len(out) < count:
        chunk = s.recv(count - len(out))
        if not chunk:
            raise RuntimeError("short Modbus reply")
        out.extend(chunk)
    return bytes(out)


def read_register(address):
    data = request(3, struct.pack(">HH", address, 1))
    assert len(data) == 3 and data[0] == 2, data.hex()
    return struct.unpack(">H", data[1:])[0]


def write_register(address, value):
    body = struct.pack(">HH", address, value)
    assert request(6, body) == body


def read_coil(address):
    data = request(1, struct.pack(">HH", address, 1))
    assert data[0] == 1, data.hex()
    return bool(data[1] & 1)


def write_coil(address, value):
    body = struct.pack(">HH", address, 0xFF00 if value else 0)
    assert request(5, body) == body


def wait_for_port(proc):
    for _ in range(80):
        if proc.poll() is not None:
            raise RuntimeError(f"MMA2 exited unexpectedly with {proc.returncode}")
        try:
            with socket.create_connection(("127.0.0.1", PORT), timeout=0.1):
                return
        except OSError:
            time.sleep(0.1)
    raise RuntimeError("MMA2 did not open the test port")


def start(binary, log):
    proc = subprocess.Popen([str(binary), "test/persistence_manual/config.yaml"],
                            cwd=ROOT, stdout=log, stderr=subprocess.STDOUT)
    wait_for_port(proc)
    return proc


def stop(proc):
    if proc is None or proc.poll() is not None:
        return
    proc.send_signal(signal.SIGTERM)
    try:
        proc.wait(timeout=10)
    except subprocess.TimeoutExpired:
        proc.kill()
        proc.wait(timeout=5)
        raise RuntimeError("MMA2 failed to shut down gracefully")


def main():
    if shutil.which("go") is None:
        raise RuntimeError("Go executable not found")
    # Never affect real deployment snapshots.
    if SNAPS.exists():
        if SNAPS.is_symlink():
            raise RuntimeError("Refusing to delete symlink snapshot directory")
        shutil.rmtree(SNAPS)
    SNAPS.mkdir(parents=True)
    proc = None
    with tempfile.TemporaryDirectory(prefix="mma2-persistence-test-") as td:
        binary = Path(td) / "mma2"
        logfile = Path(td) / "mma2.log"
        subprocess.run(["go", "build", "-o", str(binary), "./cmd/mma2"],
                       cwd=ROOT, check=True)
        with logfile.open("w+") as log:
            try:
                print("[1/5] Fresh startup: expect zero-initialized memory", flush=True)
                proc = start(binary, log)
                assert read_register(3) == 0
                assert read_coil(5) is False

                print("[2/5] FC6 and FC5 writes; wait for targeted disk flush", flush=True)
                write_register(3, 0xBEEF)
                write_coil(5, True)
                assert read_register(3) == 0xBEEF
                assert read_coil(5) is True
                time.sleep(0.4)
                primary = SNAPS / f"mma2-{PORT}-{UNIT}.bin"
                backup = SNAPS / f"mma2-{PORT}-{UNIT}.bak"
                assert primary.is_file() and backup.is_file(), "snapshot pair missing"
                print(f"      primary={primary.name} ({primary.stat().st_size} bytes)")
                print(f"      backup ={backup.name} ({backup.stat().st_size} bytes)")
                stop(proc)
                proc = None

                print("[3/5] Restart: latest written values must restore", flush=True)
                proc = start(binary, log)
                assert read_register(3) == 0xBEEF
                assert read_coil(5) is True
                print("      primary restore PASS")
                stop(proc)
                proc = None

                print("[4/5] Corrupt primary while OFF: backup recovery", flush=True)
                with primary.open("r+b") as f:
                    f.seek(-1, 2)
                    old = f.read(1)
                    f.seek(-1, 2)
                    f.write(bytes([old[0] ^ 0x01]))
                proc = start(binary, log)
                # Initial backup contains the original all-zero state.
                assert read_register(3) == 0
                assert read_coil(5) is False
                print("      backup fallback PASS (expected earlier zero state)")
                stop(proc)
                proc = None

                print("[5/5] Verify rebuilt primary survives another restart", flush=True)
                proc = start(binary, log)
                assert read_register(3) == 0
                assert read_coil(5) is False
                stop(proc)
                proc = None
                print("PASS: write, restart, CRC recovery and rebuilt primary")
                print("NOTE: This script does not wait 60 seconds to validate periodic refresh.")
            except Exception:
                log.flush()
                print(f"FAIL. MMA2 log follows from {logfile}:", file=sys.stderr)
                print(logfile.read_text(errors="replace")[-12000:], file=sys.stderr)
                raise
            finally:
                stop(proc)


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(f"FAIL: {exc}", file=sys.stderr)
        sys.exit(1)
