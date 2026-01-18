# MMA 2.0 — Memory Model

## Purpose

This document defines the **memory model** used by MMA 2.0.

Memory is the core responsibility of MMA.
All other functionality exists to safely read from or write to memory.

---

## What Memory Is

In MMA 2.0, memory is:

- raw
- typed
- bounded
- deterministic
- owned by a Unit ID

Memory stores values only.
It has no understanding of meaning or intent.

---

## What Memory Is Not

Memory must never:

- parse data formats
- apply scaling
- apply units
- infer structure
- emit events
- trigger logic
- store metadata

Any of the above would introduce intelligence and violate determinism.

---

## Supported Memory Areas

MMA 2.0 supports only the following Modbus memory areas:

- Coils → boolean
- Discrete Inputs → boolean
- Holding Registers → 16-bit unsigned integer
- Input Registers → 16-bit unsigned integer

No other memory types are allowed.

---

## Addressing Rules

All memory is addressed using **zero-based internal addressing**.

Rules:
- Address 0 refers to the first element
- No address shifting is performed internally
- External protocol conventions must be handled by adapters

Internal consistency is prioritized over external convenience.

---

## Bounds Enforcement

Every memory access must be bounds-checked.

Rules:
- Reads outside bounds are rejected
- Writes outside bounds are rejected
- Partial writes are not allowed

If a request would exceed bounds, **no memory is modified**.

---

## Atomicity Guarantees

Memory operations must be atomic.

Rules:
- Multi-value writes are applied as a single operation
- Reads return a consistent snapshot
- Concurrent access must not expose partial state

There must be no observable intermediate states.

---

## Concurrency Model

Memory may be accessed concurrently.

Requirements:
- Ownership must be explicit
- Shared state must be protected
- Locking must be minimal and obvious

Complex concurrency patterns are forbidden.

---

## Failure Behavior

On any memory operation failure:

- memory remains unchanged
- the failure is explicit
- the process remains alive

Memory corruption is unacceptable.

---

## Stability Guarantee

The memory model is intentionally minimal.

New memory types will not be added.
Existing rules must not be weakened.

Any change to this model is a breaking change.

---

**End of Memory Model**
