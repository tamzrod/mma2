# RBE as a separate output from write-only notify

Status: idea  
Date: 2026-09-16  
Related: `internal/notify`, `docs/MMA_Notification_Engine_LOCKED.md`

## Problem

Notify today is write-only. A successful write that intersects a rule emits an event.

Field clients (SCADA, PLC, pollers) often rewrite the same value. That traffic cannot be controlled at the source. Write-notify therefore repeats forever even when memory did not change.

On-change / RBE (report by exception) is a different question:

- notify answers: a write landed on this range
- RBE answers: the overlapping cells actually became different

Replacing notify with on-change would break the locked write-only engine.
Folding both into one Influx measurement would mix a high-rate write tap with a sparse exception tap.

## Idea

Keep write-notify as-is. Add RBE as a second engine and a second output.

```
write commit
  → notify.OnWrite      (range intersected a notify rule)
  → snapshot vs incoming (only if an RBE rule intersects)
  → rbe.OnChange         (only if old != new on the intersection)
```

Two sinks, two configs, two measurements. Same address may appear on both rule lists.

## Why it is cheap

The new payload is already on the wire. Memory already holds the old value.
RBE only needs a snapshot of the write range before commit, then a compare.

- FC5 / FC6: 1 coil or 1 register
- FC16 worst case: 123 registers = 246 bytes
- FC15 worst case: ~246 bytes of packed coils

Compare only when an RBE rule matches. Identical refreshes skip RBE.
Do not store a last-value table in the RBE package. Memory is that table.

## Constraints (carry forward if this becomes a phase)

- Memorycore stays unaware of notify and RBE. No events from memory.
- No values in v1 RBE events (no old/new payload). That is a historian.
- RBE `start` / `count` should be the changed intersection, not the whole write block.
- Coil compare must use the same packing as `writeBits` (ignore unused high bits in the last byte).
- Adapter failure must not affect write success. Non-blocking, drop if full.
- Do not reuse `mma_notify` / `src_ip` drift. RBE gets its own measurement.

## Sketch config

```yaml
notify:
  influx:
    url: ...
    bucket: mma_writes
    measurement: mma_notify

rbe:
  influx:
    url: ...
    bucket: mma_rbe
    measurement: mma_rbe
```

Per memory:

```yaml
notify:
  holding_registers:
    - start: 300
      count: 5
      name: control_block

rbe:
  holding_registers:
    - start: 300
      count: 1
      name: active_power_setpoint
```

## Non-goals for this idea

- Change detection inside memorycore
- Per-address event storms
- Rule merging / dedup across notify and RBE
- Replacing the locked write-only notify engine

## Open questions

- Snapshot then write as two ops, or one `Write*IfChanged` under the existing memory lock?
- Shared range-normalize with notify, or a separate `internal/rbe` package that only copies the types it needs?
- Should identical-value writes still hit notify even when RBE is configured? (Default yes.)
