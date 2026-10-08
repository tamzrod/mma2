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

## Raw Ingest

Raw Ingest now returns structured 1-byte diagnostic response codes for improved observability.

See [docs/RAW_INGEST.md](docs/RAW_INGEST.md) for the full response code table and behavior specification.

---

## Native Persistence

MMA2 can persist authoritative raw memory to disk, configured **per
`listeners[].memory[]` entry only**. Each (Port, UnitID) independently enables
persistence, optionally chooses its directory and selects ranges. When no
snapshot directory is specified, files are saved beside the loaded YAML
configuration, named by Port and UnitID. Omitted
persistence means disabled; omitted ranges on an enabled memory persist its
allocated areas. No root-level persistence setting is supported.

The runtime restores verified binary snapshots before listeners accept requests.
Targeted writes maintain the latest `.bin`; a CRC32-validated `.bak` is
refreshed every 60 seconds for recovery. See
[docs/PERSISTENCE.md](docs/PERSISTENCE.md) for configuration, disk behavior,
durability limitations and the manual Modbus Poll restart test.

---

## Modbus Device Identification

FC43 / MEI 14 (`0x2B / 0x0E`) exposes the Basic ASCII objects VendorName,
ProductCode, and MajorMinorRevision. Defaults are `github.com/tamzrod`, `MMA2`,
and the release identifier in `internal/version/version.go`. An optional `fc43` section under each `listeners[].memory[]` entry overrides
each field independently for that logical device:

```yaml
listeners:
  - id: modbus
    listen: "0.0.0.0:502"
    memory:
      - unit_id: 1
        fc43:
          vendor_name: "github.com/tamzrod"
          product_code: "MMA2"
          major_minor_revision: "2.0"
        holding_registers: {start: 0, count: 100}
      - unit_id: 2
        fc43:
          product_code: "MMA2-Unit2"
        holding_registers: {start: 0, count: 100}
```

Each identity is selected by `(Port, UnitID)`, where Port comes from the listener.
Metadata stays in the Modbus layer, outside memorycore. The former root-level
`device_identity` section is rejected; move its fields under each memory
entry's `fc43`. Omitted sections and fields retain their defaults independently. Explicit empty values, non-ASCII values,
and values longer than 244 bytes fail startup validation; objects are never
truncated. Identity is fixed at startup and has no register write interface.

Basic, Regular, and Extended stream requests (codes 1–3) return the available
Basic objects; individual access (code 4) returns one object. Conformity is
`0x81`. Responses paginate whole objects within the 253-byte PDU limit.
Unknown stream object IDs restart at object 0; unknown individual IDs return
exception 2. Invalid request lengths/codes return exception 3; unsupported MEI
types return exception 1. Existing memory sealing and per-memory authority
checks apply: the matching policy must allow FC43 (`allow_fc: [43]`, alongside
any other required function codes).

See [docs/MODBUS_DEVICE_IDENTIFICATION.md](docs/MODBUS_DEVICE_IDENTIFICATION.md) for the complete configuration, protocol, authority, State Sealing, pagination, and exception behavior.
