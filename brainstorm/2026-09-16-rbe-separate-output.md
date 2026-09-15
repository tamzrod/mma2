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

```text
memory write
  → check configured RBE rules
  → capture previous raw state atomically with the write when required
  → successful commit
  → suppress if state sealing is active
  → compare previous vs new raw state
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

A minimal event should identify the rule and memory location, for example:

- Name
- Port
- UnitID
- Area
- Start
- Count
- Sequence
- Timestamp

No memory value is included.

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
            - name: Active_Power_Setpoint
              start: 2
              count: 2

            - name: Reactive_Power_Setpoint
              start: 4
              count: 2
```

Meaning:

```text
Input Registers

2..3 → Active_Power_Setpoint
4..5 → Reactive_Power_Setpoint
```

If any cell inside a configured rule's intersection with a successful write actually changes, emit one event for that rule.

The event `start` / `count` should describe the evaluated rule/write intersection. It does not claim that every cell inside that intersection changed.

Do not emit per-address events and do not split a single rule into multiple changed-span events in v1.

## State sealing behavior

State sealing suppresses RBE output.

A successful memory update may still occur through permitted internal / Raw Ingest paths while the memory is sealed, but no RBE event is emitted while sealing is active.

Exact behavior:

```text
successful memory commit
        ↓
is this (Port, UnitID) sealed?
   ├── yes → no RBE event
   └── no  → compare old/new
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
unseal         no catch-up RBE
1200 → 1300   RBE emitted
```

This is important for PPC operation: while a memory is intentionally sealed, setpoint-change and trip-related RBE triggers are silent even if Raw Ingest is refreshing or restoring underlying memory.

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

Sequence information should be included so the subscriber can detect discontinuity or missed/restarted streams.

The exact wire protocol is not defined by this brainstorm yet.

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

No memory values are included in the event.

## Performance model

Do not maintain a separate last-value table in the RBE package. Memory already holds the current state.

Only perform comparison work when a configured RBE rule intersects the incoming write.

Raw Ingest must be considered in the cost model. Its writes may be much larger than normal Modbus FC15 / FC16 limits, so do not blindly snapshot an entire incoming Raw Ingest range when only a small RBE rule intersects it.

Prefer capturing and comparing only the relevant RBE intersection(s).

## Atomicity requirement

A transport-level sequence of:

```text
Read → Compare → Write
```

is not sufficient because another writer could modify the same memory between the read and the write.

RBE requires the previous raw state associated with the actual committed write.

Any implementation must preserve atomicity under the existing memory lock.

Memorycore must remain semantically unaware of RBE. It may expose a neutral atomic memory primitive that can return overwritten raw state while committing a write, but it must not:

- know RBE rules
- compare values for RBE semantics
- emit events
- know output adapters
- store source IP

Change interpretation remains outside memorycore.

## Output architecture

RBE should expose an adapter boundary similar to:

```text
rbe.Adapter
  ├── TCPAdapter
  ├── InfluxAdapter
  └── MultiAdapter
```

One actual change may fan out to multiple configured sinks without changing event semantics.

## Old notify removal

If RBE is promoted to an implementation phase, remove the old write-only notify feature rather than keeping two overlapping event systems.

Expected removal / replacement scope includes:

- `internal/notify`
- old `notify:` YAML configuration
- old `mma_notify` measurement references
- notify wiring from Modbus
- notify wiring from Raw Ingest
- `docs/MMA_Notification_Engine_LOCKED.md`

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
- Emit only after a successful memory commit.
- Identical-value writes emit nothing.
- Sealed memory emits no RBE events.
- No catch-up events after unsealing.
- Adapter failure must not change memory-write success.
- TCP output is intended for low-latency direct PPC notification.
- Influx is optional and used for historical/event observation.
- No per-address event storms.
- No rule merging or deduplication across independently configured RBE rules.

## Non-goals

- Change detection logic inside memorycore
- Sending register/coil values in RBE events
- Replacing Modbus as the source of truth
- Scaling or interpreting data
- A historian inside MMA
- A protection relay replacement
- Per-address event storms
- Catch-up/replay of changes that happened while sealed

## Open questions before promotion to a phase

- Exact neutral atomic memory primitive required to capture pre-write state safely
- Exact RBE TCP binary frame layout
- TCP subscriber model: one subscriber or multiple simultaneous subscribers
- Reconnect behavior and sequence reset semantics
- Queue/backpressure policy for TCP without blocking memory writes
- Whether TCP adapter should expose one listening endpoint globally or allow per-output endpoints
