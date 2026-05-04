# MMA 2.0 — Raw Ingest Transport

## Purpose

Raw Ingest is a **write-only ingest transport** for MMA 2.0.

It accepts explicit binary write payloads over TCP and commits them directly to core memory.

Raw Ingest is stateless. Each connection carries exactly one write operation.

---

## Protocol Format (v1)

```
Magic      (2 bytes)   = 'R' 'I' (0x52 0x49)
Version    (1 byte)    = 0x01
Area       (1 byte)    = 1 (Coils) | 2 (DiscreteInputs) | 3 (HoldingRegs) | 4 (InputRegs)
UnitID     (2 bytes)   = target unit (big-endian uint16)
Address    (2 bytes)   = start address (big-endian uint16)
Count      (2 bytes)   = number of values (big-endian uint16)
Payload    (variable)  = bit-packed or word-aligned data
```

Payload encoding:
- Bit areas (Coils, Discrete Inputs): bits packed LSB-first, padded to byte boundary
- Register areas (Holding, Input): big-endian uint16 words, 2 bytes each

---

## Response Codes (v1)

After processing a packet, the server sends exactly **1 byte** indicating the outcome.

| Code | Name             | Meaning                      |
| ---- | ---------------- | ---------------------------- |
| 0x00 | OK               | Write committed              |
| 0x10 | INVALID_MAGIC    | Bad magic bytes              |
| 0x11 | INVALID_VERSION  | Unknown version              |
| 0x12 | INVALID_AREA     | Unknown area                 |
| 0x13 | INVALID_COUNT    | Count is zero                |
| 0x14 | INVALID_LENGTH   | Payload length mismatch      |
| 0x20 | MEMORY_NOT_FOUND | No memory for (Port, UnitID) |
| 0x21 | OUT_OF_BOUNDS    | Address exceeds layout       |
| 0x30 | INTERNAL_ERROR   | Unexpected failure           |

---

## Response Behavior

- The response is always **exactly 1 byte**.
- `0x00` means the write was committed.
- Any other code means **no write occurred**.
- On error, the server sends the response code and **immediately closes the connection**.
- EOF (clean client disconnect before a complete packet) produces **no response**.

---

## Error Classification

Response codes are grouped by failure category:

- **0x10–0x14** — Decoder errors: the packet header or payload did not conform to the protocol.
- **0x20** — Memory lookup failure: no memory block exists for the given (Port, UnitID) pair. Port is derived from the listening endpoint; UnitID is explicit in the packet.
- **0x21** — Bounds violation: the requested address range exceeds the configured memory layout.
- **0x30** — Unexpected failure: an internal condition prevented normal processing.

These codes describe observable behavior only. Internal implementation details are not exposed.

---

## Diagnostic Note

Response codes are **diagnostic only**.

They do not affect:
- memory behavior
- protocol semantics
- access control

A non-zero response code indicates the write was rejected. It carries no further side effects.

---

## Access Control

Raw Ingest **bypasses the authority model** (no policy enforcement).

Raw Ingest **bypasses state sealing** (writes are accepted even if the appliance is sealed).

Raw Ingest performs bounds checking only.

---

## Restrictions

- No protocol-level decode beyond alignment
- No automatic retry mechanism
- No freshness tracking
- No multi-packet sessions

---

**End of Raw Ingest**
