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

Restrictions:
- no retries with intent
- no request aggregation
- no memory inference

---

## REST

REST is a **write-only ingest transport**.

Responsibilities:
- accept explicit write requests
- validate request structure
- reject ambiguous targeting

Restrictions:
- no Modbus emulation
- no read semantics
- no routing inference

---

## MQTT

MQTT is a **write-only ingest transport**.

Responsibilities:
- accept explicit payloads
- require explicit target identification
- write values directly to memory

Restrictions:
- no topic-based inference
- no retained-state logic
- no status-driven behavior

---

## Raw Ingest (TCP)

Raw Ingest is a **low-level write-only transport**.

Characteristics:
- stateless
- blind
- alignment-only

Responsibilities:
- accept raw payloads
- write aligned values to explicit memory targets

Restrictions:
- no decode beyond alignment
- no retries with meaning
- no freshness tracking

---

## Explicit Targeting Requirement

All transports must require explicit targeting.

A valid request must always specify:
- port
- unit_id
- memory area
- address
- values

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

Such behaviors violate determinism.

---

## Stability Guarantee

The transport model is stable.

New transports may be added.
Existing transports may evolve.

The adapter boundary must never be weakened.

---

**End of Transports**
