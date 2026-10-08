#!/usr/bin/env python3
"""Isolated process-level per-memory regression. Never touches manual snapshots."""
from pathlib import Path
import socket
import struct
import subprocess
import tempfile
import time
import signal
import sys

ROOT = Path(__file__).resolve().parents[2]


def modbus(port, unit, fc, address, value=1):
    pdu = struct.pack(">BHH", fc, address, value)
    req = struct.pack(">HHHB", 1, 0, len(pdu) + 1, unit) + pdu
    with socket.create_connection(("127.0.0.1", port), 2) as sock:
        sock.settimeout(2)
        sock.sendall(req)
        hdr = receive(sock, 7)
        transaction, proto, length, unit_response = struct.unpack(">HHHB", hdr)
        assert (transaction, proto, unit_response) == (1, 0, unit)
        result = receive(sock, length - 1)
        assert result[0] == fc, f"FC{fc} Unit {unit}: {result.hex()}"
        return result


def receive(sock, n):
    result = bytearray()
    while len(result) < n:
        chunk = sock.recv(n - len(result))
        if not chunk:
            raise RuntimeError("short reply")
        result.extend(chunk)
    return bytes(result)


def read(port, unit):
    response = modbus(port, unit, 3, 0, 1)
    assert response[1] == 2
    return struct.unpack(">H", response[2:4])[0]


def write(port, unit, value):
    response = modbus(port, unit, 6, 0, value)
    assert response == struct.pack(">BHH", 6, 0, value)


def start(binary, config, log, port):
    p = subprocess.Popen([str(binary), str(config)], cwd=ROOT,
                         stdout=log, stderr=subprocess.STDOUT)
    for _ in range(100):
        if p.poll() is not None:
            raise RuntimeError(f"MMA2 exited early ({p.returncode})")
        try:
            with socket.create_connection(("127.0.0.1", port), 0.1):
                return p
        except OSError:
            time.sleep(0.05)
    raise RuntimeError("MMA2 did not bind")


def stop(p):
    if p is None or p.poll() is not None:
        return
    p.send_signal(signal.SIGTERM)
    try:
        p.wait(timeout=10)
    except subprocess.TimeoutExpired:
        p.kill()
        p.wait()
        raise RuntimeError("MMA2 shutdown timed out")


def main():
    with tempfile.TemporaryDirectory(prefix="mma2-multi-persistence-") as tmp:
        base = Path(tmp)
        binary = base / "mma2"
        subprocess.run(["go", "build", "-o", str(binary), "./cmd/mma2"], cwd=ROOT, check=True)
        # Bind an available local test port; close before launching the server.
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            port = listener.getsockname()[1]
        d1, d2 = base / "u1", base / "u2"
        cfg = base / "config.yaml"
        cfg.write_text(f"""listeners:
  - id: test
    listen: "127.0.0.1:{port}"
    memory:
      - unit_id: 1
        holding_registers: {{start: 0, count: 16}}
        persistence:
          enabled: true
          directory: "{d1}"
        policy:
          rules:
            - id: test
              source_ip: [127.0.0.1]
              allow_fc: [3, 6]
      - unit_id: 2
        holding_registers: {{start: 0, count: 16}}
        persistence:
          enabled: true
          directory: "{d2}"
        policy:
          rules:
            - id: test
              source_ip: [127.0.0.1]
              allow_fc: [3, 6]
      - unit_id: 3
        holding_registers: {{start: 0, count: 16}}
        policy:
          rules:
            - id: test
              source_ip: [127.0.0.1]
              allow_fc: [3, 6]
""")
        p = None
        logpath = base / "mma2.log"
        with logpath.open("w+") as log:
            try:
                p = start(binary, cfg, log, port)
                for unit in (1, 2, 3):
                    assert read(port, unit) == 0
                    write(port, unit, 100 + unit)
                stop(p)
                p = None
                assert list(d1.glob("*.bin")) and list(d2.glob("*.bin"))
                p = start(binary, cfg, log, port)
                assert read(port, 1) == 101
                assert read(port, 2) == 102
                assert read(port, 3) == 0
                stop(p)
                p = None
                primary = d1 / f"{port}-1.bin"
                data = bytearray(primary.read_bytes())
                data[-1] ^= 0x01
                primary.write_bytes(data)
                p = start(binary, cfg, log, port)
                assert read(port, 1) == 0, "Unit 1 should restore initial backup"
                assert read(port, 2) == 102, "Unit 2 must remain independent"
                assert read(port, 3) == 0, "disabled Unit 3 must reset"
                print("PASS: enabled/disabled memories, restart, corrupt primary, isolation")
            except Exception:
                log.flush()
                print(logpath.read_text(errors="replace")[-12000:], file=sys.stderr)
                raise
            finally:
                stop(p)


if __name__ == "__main__":
    main()
