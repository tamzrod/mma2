# MMA 2.0 — Transports

## Purpose

This document defines the **transport adapter model** of MMA 2.0.

Transports are the only components that interact with the outside world.
They exist to safely translate external requests into explicit memory operations.

Transports must never influence core behavior.

---

## Transport Definition

A transport is an **adapter**.

It:
- receives external input
- validates protocol correctness
- resolves explicit targets
- performs bounded memory operations

It does not:
- infer intent
- apply logic
- modify authority
- interpret meaning

---

## Adapter Boundary

Transports sit **outside** the core memory.

They depend on:
- configuration
- authority model
- memory API

The core memory must never depend on transports.

---

## Read vs Write Expectations

Transports may be:
- read/write
- write-only

These expectations are fixed per transport type and must not change dynamically.

---

## Modbus TCP

Modbus TCP is a **read/write transport**.

Responsibilities:
- enforce Modbus protocol correctness
- respect function code semantics
- map external addresses to internal zero-based addressing
- reject invalid or out-of-bounds requests
- enforce access control via authority model
- enforce state sealing (return 0x06 Device Busy if sealed)

Supported function codes:
- FC1: Read Coils
- FC2: Read Discrete Inputs
- FC3: Read Holding Registers
- FC4: Read Input Registers
- FC5: Write Single Coil
- FC6: Write Single Register (Holding Registers only)
- FC15: Write Multiple Coils
- FC16: Write Multiple Registers (Holding Registers only)

Restrictions:
- no retries with intent
- no request aggregation
- no memory inference

---

## Raw Ingest (TCP)

Raw Ingest is a **write-only ingest transport**.

Characteristics:
- stateless
- binary protocol
- fixed frame format

Responsibilities:
- accept explicit write payloads
- decode frame structure (magic, version, area, unit_id, address, count, payload)
- resolve target memory by unit_id
- perform writes atomically
- reply with single-byte status (0x00 = success, 0x01 = error)

Memory areas supported:
- Coils
- Discrete Inputs
- Holding Registers
- Input Registers

Protocol Format (v1):

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

Response:
- Single byte: 0x00 (success) or 0x01 (error)

Restrictions:
- no protocol-level decode beyond alignment
- no retries with meaning
- no freshness tracking

Access Control Bypass:
- Raw Ingest **bypasses authority model** (no policy enforcement)
- Raw Ingest **bypasses state sealing** (writes allowed even if sealed)
- Raw Ingest performs bounds checking only

---

## Explicit Targeting Requirement

All transports must require explicit targeting.

A valid request must always specify:
- port (derived from listening endpoint for Modbus TCP; resolved for raw ingest)
- unit_id (explicit in protocol)
- memory area (explicit in protocol)
- address (explicit in protocol)
- values (explicit in protocol payload)

If any of these are missing or ambiguous, the request must be rejected.

---

## Failure Behavior

On transport failure:
- memory must remain unchanged
- the failure must be explicit
- the process must remain alive

Transports must not hide or soften errors.

---

## Forbidden Transport Behaviors

Transports must never:
- cache memory state
- share memory across Unit IDs
- apply transformations
- introduce defaults
- repair malformed requests
- infer protocol intent

Such behaviors violate determinism.

---

## Stability Guarantee

The transport model is stable.

The two implemented transports (Modbus TCP and Raw Ingest) are fixed.

New transports may be added in the future.
Existing transports may evolve within their specified scope.

The adapter boundary must never be weakened.

---

**End of Transports**
