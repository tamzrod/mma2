# MMA 2.0 — Overview

## What This Is

MMA 2.0 (Modbus Memory Appliance) is a **deterministic, minimal, and opinionated Modbus TCP memory appliance**.

Its single responsibility is:

> **Store and serve raw Modbus memory correctly, predictably, and safely — under all conditions.**

MMA is infrastructure, not an application.

---

## What Problem It Solves

Industrial systems often fail not because devices are wrong, but because:

* upstream SCADA sends malformed or excessive requests
* field devices behave inconsistently under load
* protocol edge cases accumulate state and crash systems

MMA 2.0 exists to sit **between clients and devices**, absorbing pressure and providing:

* deterministic memory behavior
* strict bounds enforcement
* predictable failure modes

When MMA is healthy, downstream systems remain stable.

---

## What MMA 2.0 Is Not

MMA 2.0 is **not**:

* a PLC
* a SCADA system
* a protocol translator
* a data modeler
* a control engine
* a historian
* a semantic processor

If a feature implies **meaning**, **logic**, **intent**, or **decision-making**, it does not belong here.

---

## Core Principle

> **Configuration defines the appliance.**

The configuration file:

* defines ports
* defines Unit IDs
* defines memory sizes

Code does not infer, repair, or reinterpret configuration.

Invalid configuration must fail fast.

---

## Design Intent

MMA 2.0 is designed to be:

* boring
* strict
* predictable
* easy to reason about
* safe under stress

If MMA ever becomes clever, it has already failed.

---

## Document Order

This document is the entry point.

Subsequent documents define:

1. Philosophy
2. Architecture
3. Authority model
4. Configuration
5. Memory model
6. Transports
7. Failure behavior
8. Non-goals

Each document builds on the previous one.

---

**End of Overview**
