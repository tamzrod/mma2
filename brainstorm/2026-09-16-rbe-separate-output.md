# RBE as the replacement for write-only notify

Status: idea  
Date: 2026-09-16  
Related: `internal/notify`, `docs/MMA_Notification_Engine_LOCKED.md`, state sealing

## Problem

The current notify engine emits an event for every successful write that intersects a configured rule, even when the incoming value is identical to the value already stored in memory.

Field clients such as SCADA, PLCs, and pollers often rewrite the same values continuously. This creates unnecessary event traffic, queue activity, adapter work, Influx writes, storage churn, and downstream processing even though memory did not actually change.

For PPC control, the useful question is not:

- did somebody write here?

The useful question is:

- did this configured memory range actually change?

That is RBE (report by exception).

## Direction

Retire the old write-only notify engine and replace it with a single RBE engine.

RBE emits only when a configured memory range actually changes.

Outputs are adapters. Initial outputs:

- TCP — primary ultra-fast PPC trigger path
- InfluxDB — optional event history / observability

Additional adapters may be added later without changing RBE semantics.

Core principle:

> **RBE = trigger. Modbus = value.**

```text
incoming write
  → determine whether an RBE rule intersects
  → capture pre-write sealing state when RBE observation is required
  → perform the memory write atomically with required raw change observation
  → successful commit
  → if memory was sealed before this write, suppress RBE
  → otherwise evaluate actual raw change for matching rule(s)
  → if changed, emit one RBE event per matching rule
      ├── TCP
      └── InfluxDB (optional)
```

## PPC use case

The PPC currently reads Modbus cyclically, for example every 100 ms. That leaves a 0–100 ms discovery window before the PPC notices a new setpoint or command.

RBE TCP becomes the fast trigger path:

```text
setpoint / command write
        ↓
      MMA memory
        ↓
   actual change?
        ↓ yes
      RBE TCP
        ↓
       PPC
        ↓
 immediate Modbus read of the configured range
```

Modbus remains the source of truth and the normal reconciliation / fallback path.

RBE TCP does not carry the value.

## Event semantics

RBE answers only:

> A configured memory range actually changed.

An RBE event MUST NOT include:

- old value
- new value
- decoded/scaled value
- semantic interpretation

The PPC or other subscriber reacts to the event and reads the current value through Modbus.

An internal RBE event may identify the rule and memory location using:

- RuleID
- Name
- Port
- UnitID
- Area
- Start
- Count
- StreamID
- Sequence
- Timestamp

No memory value is included.

### Rule identity

`name` is human-facing metadata for YAML, logs, and Influx.

The low-latency TCP wire should use a small numeric `RuleID` rather than a variable-length name string.

Rule IDs should be explicit in configuration so they remain deterministic across restarts and configuration reloads.

Example:

```yaml
- id: 1
  name: Active_Power_Setpoint
  start: 2
  count: 2
```

TCP may send `RuleID = 1`; Influx may still use `name=Active_Power_Setpoint`.

## RBE configuration

Use memory areas directly. Do not configure RBE using Modbus function codes because RBE is memory-change based and may be triggered by Raw Ingest or other write paths.

Use `start`, not `offset`, because MMA does not perform address shifting or remapping.

Example:

```yaml
rbe:
  tcp:
    enabled: true
    listen: 9001

  influx:
    enabled: true
    url: ...
    token: ...
    org: ...
    bucket: mma_rbe
    measurement: mma_rbe

listeners:
  - listen: 502
    memory:
      - unit_id: 1

        input_registers:
          start: 0
          count: 100

        rbe:
          input_registers:
            - id: 1
              name: Active_Power_Setpoint
              start: 2
              count: 2

            - id: 2
              name: Reactive_Power_Setpoint
              start: 4
              count: 2
```

Meaning:

```text
Input Registers

2..3 → RuleID 1 → Active_Power_Setpoint
4..5 → RuleID 2 → Reactive_Power_Setpoint
```

If any cell inside a configured rule's intersection with a successful write actually changes, emit one event for that rule.

The event `start` / `count` should describe the evaluated rule/write intersection. It does not claim that every cell inside that intersection changed.

Do not emit per-address events and do not split a single rule into multiple changed-span events in v1.

## State sealing behavior

State sealing suppresses RBE output.

A successful memory update may still occur through permitted internal / Raw Ingest paths while the memory is sealed, but no RBE event is emitted for a write that originates while the memory is sealed.

### Sealing state timing

RBE eligibility is determined from the sealing state that protected the memory **before the write is committed**.

This rule is important for the write that unseals memory itself.

```text
SEALED
  ↓
Raw Ingest changes data   → no RBE
Raw Ingest writes seal 0→1→ no RBE
  ↓
UNSEALED
next actual data change   → RBE allowed
```

The unsealing write MUST NOT produce a catch-up event or make other changes from that same sealed write eligible for RBE.

Conceptually:

```text
incoming write
        ↓
capture pre-write sealing state
        ↓
atomic memory commit
        ↓
was memory sealed before this write?
   ├── yes → no RBE event
   └── no  → evaluate actual change
               ↓
             changed?
             ├── no  → nothing
             └── yes → emit RBE
```

Changes that occur while sealed are NOT replayed later.

Unsealing MUST NOT generate a catch-up RBE event for values that changed during the sealed period.

Example:

```text
sealed
1000 → 1100   memory changes, no RBE
1100 → 1200   memory changes, no RBE
unseal         no RBE from the unsealing write and no catch-up
1200 → 1300   RBE emitted
```

This applies to every RBE rule under that sealed `(Port, UnitID)`, including setpoint and trip-related rules.

If a trip must remain available while PPC control memory is sealed, it must use a memory/control path that is not suppressed by that sealing policy or an independent protection mechanism. RBE does not bypass sealing.

## TCP output

TCP is the primary low-latency output for PPC.

The connection should be persistent. Do not connect/send/disconnect for each event.

RBE TCP is a trigger path, not a data transport for the underlying register values.

Conceptually:

```text
MMA
  ├── Modbus → source of truth / normal polling / fallback
  └── RBE TCP → immediate change trigger
```

The TCP framing should be small, deterministic, binary, and versioned.

### Stream identity and sequence

Sequence alone is not sufficient because sequence numbers may restart after MMA or the RBE engine restarts.

Each RBE TCP stream should therefore have:

- `StreamID` / boot-session identifier
- monotonically increasing `Sequence` within that stream

Conceptually:

```text
same StreamID + sequence gap
    → subscriber knows one or more events were missed

new StreamID
    → subscriber knows the RBE stream restarted
```

No event replay is required. After a gap or new stream, the PPC reconciles current state through Modbus.

The exact field widths, endian rules, framing length, and StreamID representation are not defined by this brainstorm yet.

## Influx output

InfluxDB remains an optional RBE adapter.

Its semantics change from the old notify model:

Old notify:

> a successful write intersected this rule

RBE Influx:

> this configured range actually changed

Use a separate measurement such as:

```text
mma_rbe
```

Influx may retain the human-facing rule `name` as a tag in addition to numeric rule identity.

No memory values are included in the event.

## Performance model

Do not maintain a separate last-value table in the RBE package. Memory already holds the current state.

Only invoke additional RBE observation work when at least one configured RBE rule intersects the incoming write.

Raw Ingest must be considered in the cost model. Its writes may be much larger than normal Modbus FC15 / FC16 limits.

Correctness and core-layer purity take precedence over premature partial-range optimization.

The implementation must NOT pass RBE rules, RBE names, or RBE-specific policy into `memorycore` merely to avoid copying or comparison work.

A neutral memory primitive may return raw pre-write state or neutral raw change metadata while committing the write. If later optimization is required, any observed-range primitive must remain generic memory functionality and must not depend on RBE concepts.

## Atomicity requirement

A transport-level sequence of:

```text
Read → Compare → Write
```

is not sufficient because another writer could modify the same memory between the read and the write.

RBE requires change information associated with the actual committed write.

Any implementation must preserve atomicity under the existing memory lock.

Memorycore must remain semantically unaware of RBE. It may expose a neutral atomic memory primitive that can return overwritten raw state or neutral change metadata while committing a write, but it must not:

- know RBE rules
- know RuleID or rule names
- emit events
- know output adapters
- store source IP
- decide whether an RBE event should exist

Raw equality/change metadata is permitted only as a neutral memory-operation result; RBE interpretation remains outside `memorycore`.

### Rejected direction

Do not make `memorycore` accept RBE-specific subranges or RBE rule objects in order to perform selective snapshots.

That would leak observation policy into the core.

## Output architecture

RBE should expose an adapter boundary similar to:

```text
rbe.Adapter
  ├── TCPAdapter
  ├── InfluxAdapter
  └── MultiAdapter
```

One actual change may fan out to multiple configured sinks without changing event semantics.

Adapter failure must never change whether the memory write succeeds.

## Old notify removal and documentation cutover

If RBE is promoted to an implementation phase, remove the old write-only notify feature rather than keeping two overlapping event systems.

Expected removal / replacement scope includes:

- `internal/notify`
- old `notify:` YAML configuration
- old `mma_notify` measurement references
- notify wiring from Modbus
- notify wiring from Raw Ingest

Documentation cutover must be ordered safely:

1. Define and approve the locked RBE architecture contract.
2. Implement and verify RBE behavior.
3. Remove old notify runtime wiring/configuration.
4. Retire `docs/MMA_Notification_Engine_LOCKED.md` only when the new locked RBE document becomes authoritative.

Do not delete the existing locked notify contract before its replacement contract exists.

Replacement concepts:

- `internal/rbe`
- `rbe:` YAML configuration
- `mma_rbe`
- new locked RBE architecture document

## Constraints

- Memorycore stays unaware of RBE semantics.
- No memory values in RBE events.
- No old/new payloads.
- No scaling or semantic interpretation.
- RBE rules are memory-area based, not function-code based.
- Rule IDs are explicit and numeric for deterministic low-latency TCP identity.
- Rule names remain human-facing metadata.
- Emit only after a successful memory commit.
- Identical-value writes emit nothing.
- RBE eligibility uses the pre-write sealing state.
- A write that unseals a sealed memory emits no RBE.
- Sealed memory emits no RBE events.
- No catch-up events after unsealing.
- Adapter failure must not change memory-write success.
- TCP output is intended for low-latency direct PPC notification.
- TCP uses a persistent connection.
- TCP stream identity and sequence allow discontinuity/restart detection.
- After stream discontinuity/restart, Modbus is used for reconciliation rather than RBE replay.
- Influx is optional and used for historical/event observation.
- No per-address event storms.
- No rule merging or deduplication across independently configured RBE rules.
- RBE-specific policy must not leak into `memorycore`.

## Non-goals

- RBE semantics inside memorycore
- Sending register/coil values in RBE events
- Replacing Modbus as the source of truth
- Scaling or interpreting data
- A historian inside MMA
- A protection relay replacement
- Per-address event storms
- Catch-up/replay of changes that happened while sealed
- RBE-specific snapshot/range policy inside memorycore

## Open questions before promotion to a phase

- Exact neutral atomic memory primitive / return shape for safe raw change observation
- Exact RBE TCP binary frame field widths and endian/framing rules
- TCP subscriber model: one subscriber or multiple simultaneous subscribers
- Queue/backpressure policy for TCP without blocking memory writes
- Whether TCP adapter should expose one listening endpoint globally or allow multiple configured endpoints
