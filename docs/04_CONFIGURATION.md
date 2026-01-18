# MMA 2.0 — Configuration Contract

## Purpose

This document defines the **configuration contract** for MMA 2.0.

Configuration is not a convenience layer.
Configuration is **the system definition**.

All runtime behavior must be a direct consequence of configuration.

---

## Configuration as Authority

In MMA 2.0:

> **Configuration defines the appliance.**

Configuration:
- declares ports
- declares Unit IDs per port
- declares memory sizes per Unit ID

There is no other source of truth.

Code must not:
- infer missing configuration
- repair invalid configuration
- generate defaults implicitly

---

## Immutability Rules

Configuration is:
- loaded once at startup
- immutable at runtime
- not reloadable
- not hot-swappable

Any configuration change requires a full process restart.

This is intentional.

---

## Validation Requirements

Configuration must be validated **before** any listener starts.

Validation failures must:
- be explicit
- include the reason
- stop the process immediately

Warnings are not allowed.
Partial startup is not allowed.

---

## Required Structural Properties

A valid configuration must define:

- at least one port
- at least one Unit ID per port
- at least one memory area per Unit ID
- strictly positive memory sizes

If any of these are missing, configuration is invalid.

---

## Authority Binding

Configuration must encode the authority model exactly:

Port  
→ Unit ID  
→ Memory

Rules:
- Unit IDs are scoped to a port
- Memory exists only within a Unit ID
- Memory cannot be shared or aliased

If configuration attempts to violate these rules, it must be rejected.

---

## No Implicit Defaults

Configuration must be explicit.

The following are forbidden:
- implicit ports
- implicit Unit IDs
- implicit memory areas
- implicit sizes

If a value is required and missing, startup must fail.

---

## Separation from Transports

Configuration defines **what exists**, not **how it is accessed**.

Transport-specific settings:
- must not alter authority
- must not define memory
- must not introduce routing

Configuration that mixes these concerns is invalid.

---

## Failure Behavior

On configuration failure:
- no listeners start
- no memory is allocated
- no partial state exists

The process must exit cleanly and predictably.

---

## Stability Guarantee

The configuration contract is designed to be stable.

New fields may be added in the future.
Existing rules must not be weakened.

Any change that alters authority semantics is a breaking change.

---

**End of Configuration Contract**
