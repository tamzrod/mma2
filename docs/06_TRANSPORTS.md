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
- enforce state sealing (return the configured exception, default 0x06 / Device Busy, if sealed)

Supported function codes:
- FC1: Read Coils
- FC2: Read Discrete Inputs
- FC3: Read Holding Registers
- FC4: Read Input Registers
- FC5: Write Single Coil
- FC6: Write Single Register (Holding Registers only)
- FC15: Write Multiple Coils
- FC16: Write Multiple Registers (Holding Registers only)
- FC43 / MEI14: Read Device Identification

### FC43 / MEI14 Device Identification

FC43 (`0x2B`) with MEI type 14 (`0x0E`) is a **Modbus service request**, not a core-memory read or write.

It:
- exposes the process-wide Basic identity objects VendorName, ProductCode, and MajorMinorRevision
- is addressed through the request's listener port and Unit ID so State Sealing and authority remain memory-scoped
- requires function code `43` to be permitted by the matching authority policy
- does not specify a memory area, register/coil address, count, or values
- does not allocate or access core memory values
- is read-only and immutable after startup
- does not produce an RBE memory event
- is not currently classified as a read/write Access Event and therefore is not emitted by the Access Event engine

State Sealing and authority are evaluated before the FC43 handler. The identity returned after those checks is global to the MMA process.

See [MODBUS_DEVICE_IDENTIFICATION.md](MODBUS_DEVICE_IDENTIFICATION.md) for configuration, object IDs, pagination, conformity, and exception behavior.

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
- reply with a structured 1-byte response code (see RAW_INGEST.md — Response Codes (v1))

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
- Structured 1-byte response code: 0x00 (write committed) or a classified error code. See RAW_INGEST.md.

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

All transport requests must explicitly identify the protocol target required by that operation.

For **memory operations**, a valid request must identify:
- port (derived from the listening endpoint for Modbus TCP; resolved for Raw Ingest)
- unit_id
- memory area
- address
- the operation-specific count and/or values required by the protocol

For **Modbus non-memory services**, the request must contain the fields defined by that Modbus service. FC43 / MEI14 Read Device Identification is the current non-memory service: it carries the Unit ID plus MEI14 request fields and intentionally has no memory area, memory address, count, or values.

FC43 still uses the request's `(Port, UnitID)` to select the State Sealing and authority context before the process-wide identity is returned.

Missing or ambiguous fields that are required by the specific operation must be rejected. Transports must never invent a memory target or infer omitted protocol fields.

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
