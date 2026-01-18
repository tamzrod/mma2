# MMA 2.0 — Failure Model

## Purpose

This document defines the **failure model** of MMA 2.0.

Failure behavior is as important as success behavior.
Predictable failure is a core safety feature.

---

## Failure Principles

MMA 2.0 follows these principles under failure:

- failures must be explicit
- failures must be bounded
- failures must not corrupt memory
- failures must not cascade silently

Hidden recovery is forbidden.

---

## Failure Classes

Failures in MMA 2.0 fall into four classes:

1. Configuration failures
2. Startup failures
3. Transport failures
4. Memory operation failures

Each class has distinct behavior.

---

## Configuration Failures

Configuration failures occur when:
- required fields are missing
- authority rules are violated
- values are invalid

Behavior:
- the process must not start
- no listeners are created
- no memory is allocated
- the process exits immediately

Partial startup is forbidden.

---

## Startup Failures

Startup failures occur after configuration validation but before steady state.

Examples:
- port bind failure
- resource allocation failure

Behavior:
- startup halts
- allocated resources are released
- the process exits cleanly

Startup must not degrade into partial operation.

---

## Transport Failures

Transport failures occur during request handling.

Examples:
- malformed requests
- protocol violations
- network interruptions

Behavior:
- the request is rejected
- memory remains unchanged
- the transport remains available
- the process continues running

Transport failures must not affect other transports.

---

## Memory Operation Failures

Memory operation failures occur when:
- bounds are exceeded
- requests are invalid
- atomicity cannot be guaranteed

Behavior:
- memory remains unchanged
- the failure is explicit
- the process continues running

Memory corruption is unacceptable.

---

## No Silent Recovery

MMA 2.0 must never:
- retry requests with intent
- mask failures
- downgrade errors
- continue with partial state

Failures must be visible to the caller.

---

## Failure Isolation

Failures must be isolated.

Rules:
- one Unit ID failure must not affect another
- one port failure must not affect another
- one transport failure must not affect others

Isolation preserves system stability.

---

## Survival Guarantees

Under any failure condition:
- authority rules remain enforced
- memory ownership does not change
- no implicit behavior is introduced

The system must remain predictable.

---

## Stability Guarantee

The failure model is stable.

New failure classes may be documented.
Existing guarantees must not be weakened.

Any change that hides or softens failure is a breaking change.

---

**End of Failure Model**
