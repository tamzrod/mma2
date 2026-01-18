# MMA 2.0 — Architecture

## Architectural Goal

MMA 2.0 is designed as a **deterministic memory appliance** with strict and visible boundaries.

Its architecture prioritizes:
- predictability
- isolation
- failure containment
- ease of reasoning under stress

The system is intentionally simple at the core and explicit at the edges.

---

## High-Level Structure

MMA 2.0 consists of four conceptual layers:

1. Configuration
2. Core Memory
3. Transport Adapters
4. Process Runtime

Each layer has a single responsibility and a strict dependency direction.

---

## Layer 1: Configuration

Configuration defines the appliance.

It declares:
- which ports exist
- which Unit IDs exist per port
- how much memory each Unit ID owns

Configuration is:
- loaded once at startup
- immutable at runtime
- the sole source of truth

All other layers must conform to configuration.

---

## Layer 2: Core Memory

The core memory layer is the heart of MMA.

Responsibilities:
- store raw Modbus memory
- enforce bounds
- guarantee atomic reads and writes

The core:
- has no knowledge of protocols
- has no knowledge of configuration format
- applies no meaning to values

It is purely mechanical.

---

## Layer 3: Transport Adapters

Transport adapters expose the core memory to the outside world.

Examples:
- Modbus TCP
- REST
- MQTT
- Raw TCP ingest

Adapters:
- translate external requests into explicit memory operations
- perform protocol validation only
- never infer intent
- never apply logic

Adapters may reject invalid requests but must never guess.

---

## Layer 4: Process Runtime

The runtime layer is responsible for:
- startup sequencing
- listener lifecycle
- graceful shutdown
- OS integration

It does not participate in memory logic.

---

## Dependency Direction

Dependencies are one-way only:

Configuration  
→ Core Memory  
→ Transport Adapters  
→ Runtime

Reverse dependencies are forbidden.

If a lower layer influences a higher layer, the architecture is broken.

---

## Architectural Constraints

The architecture intentionally forbids:
- shared global memory pools
- implicit routing
- hidden defaults
- adaptive behavior

Any feature that requires these violates the design.

---

## Archi
