# MMA 2.0 — Authority Model

## Purpose

This document defines the **single authority model** of MMA 2.0.

Authority determines:
- how memory is owned
- how requests are resolved
- what is allowed
- what is forbidden

If a behavior is not explicitly allowed here, it is invalid.

---

## The Authority Chain (Hard Rule)

MMA 2.0 has exactly **one** authority chain:

Port  
→ Unit ID  
→ Memory

There are no alternative paths.

---

## Port Authority

A port:
- represents a listening endpoint
- owns all Unit IDs configured under it
- defines the first boundary of isolation

Rules:
- Unit IDs are **scoped to a port**
- The same Unit ID value on different ports refers to **different memory**
- There is no global Unit ID namespace

---

## Unit ID Authority

A Unit ID:
- exists only within a single port
- directly owns its memory
- is the **only valid selector** for memory within a port

Rules:
- A Unit ID cannot exist without memory
- A Unit ID cannot reference another Unit ID’s memory
- Unit IDs do not share memory

In MMA 2.0:
> **Unit ID and memory are inseparable.**

---

## Memory Authority

Memory:
- exists only inside a Unit ID
- is not addressable directly
- cannot be aliased or routed

Rules:
- There is no global memory registry
- There are no memory identifiers
- Memory cannot be reassigned at runtime

If memory exists, it exists **because a Unit ID exists**.

---

## Request Resolution

All external requests must resolve memory using:

- port
- unit_id

Resolution steps:
1. Identify the port on which the request arrived
2. Identify the Unit ID within that port
3. Access memory owned by that Unit ID

If any step fails, the request is rejected.

There is no fallback behavior.

---

## Forbidden Concepts

The following concepts **must never appear** in MMA 2.0:

- memory_id
- memory routing tables
- shared memory pools
- aliasing
- implicit defaults
- resolver chains
- cross-unit access

Any implementation that introduces these violates the authority model.

---

## Consequences of Violation

If the authority model is violated:
- behavior becomes non-deterministic
- failures become hidden
- debugging becomes unreliable

Such violations are considered **architectural defects**, not bugs.

---

## Stability Guarantee

This authority model is designed to be:
- simple
- explicit
- stable over time

Transports may change.
Configuration formats may evolve.
Memory sizes may differ.

The **authority chain must never change**.

---

**End of Authority Model**
