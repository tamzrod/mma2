# RBE replaces write-only notify

Status: brainstorm / implementation in progress (NOT LOCKED)
Date: 2026-09-16; decisions updated 2026-09-17; language corrected 2026-09-18
Related: `docs/RBE_V1_DRAFT.md`, `internal/rbe`, state sealing, legacy `internal/notify`

Note (2026-09-18): early drafts named a plant controller as "the" TCP client.
That was one use case. MMA2 is the appliance. RBE subscribers are generic TCP
clients. A plant controller may be one of them.

## Problem and objective

Legacy notify emits for every successful matching write, including identical periodic refreshes. This causes unnecessary downstream event, queue and Influx traffic. Subscribers need to know that the actual raw memory changed, not merely that a write occurred.

**RBE = trigger. Modbus = value.** Retire old write-notify after the new engine has a reviewed contract and has passed tests; keep the current locked notify document until cutover. No change to Modbus or Raw Ingest wire formats.

## Committed design choices

1. The engine matches configured memory area/range, independently of Modbus function codes. `start` is an absolute zero-based memory address, not a remapping offset.
2. Emit one event per matching rule only if at least one cell in the intersection changed. Identical writes emit nothing. Overlapping rules remain independent; no per-address storm. Internal start/count metadata describes the evaluated intersection, not necessarily every changed cell.
3. RBE sends NO old, new, decoded, scaled or semantic memory values via any output. A subscriber reads current values using Modbus when notified and continues periodic reconciliation.
4. MMA2 is the RBE TCP server. Subscribers are TCP clients. The persistent listening endpoint belongs in MMA2's global `rbe.tcp.listen`, e.g. `:9001`. MMA does not configure a subscriber's destination IP.
5. TCP v1 event is EXACTLY ONE BYTE: RuleID 0x01..0xFF. 0x00 reserved. No magic, version header, length, address, name, StreamID, Sequence, timestamp, or payload in the TCP byte stream. Each byte is an event; TCP reads may combine multiple events.
6. Rule IDs are explicit and globally unique in configuration, not merely unique within a memory. Names are human-facing configuration/Influx metadata. Reject out-of-range, duplicate IDs, nonexistent area and out-of-bounds ranges at startup.
7. Output sinks are independent: TCP low-latency direct trigger and optional Influx event history (`mma_rbe`). Either or both may be configured. Influx records metadata, never memory values.
8. Seal eligibility uses the seal-bit state immediately BEFORE the observed write, sampled atomically with the write. Writes originating while sealed emit nothing, including the unsealing write. No catch-up/replay after unseal. Events already queued from an earlier unsealed write may still arrive after a subsequent sealing operation.
9. Memorycore stays unaware of RBE, sources, sinks and rules. It can expose a neutral atomic operation returning prior raw bytes and an optional generic observed bit. Do not pass RBE-specific subranges into memorycore. Compare the raw values outside the core.
10. Adapter failure never changes write success; network I/O must never execute on the memory write path or while the memory lock is held.

## Example YAML (target schema)

```yaml
rbe:
  tcp:
    listen: ":9001"
  # Optional additional output:
  # influx:
  #   url: "http://influxdb:8086"
  #   token: "YOUR_TOKEN"
  #   org: "mma"
  #   bucket: "events"
  #   measurement: "mma_rbe"

listeners:
  - id: lab
    listen: ":502"
    memory:
      - unit_id: 1
        coils: {start: 0, count: 1}
        holding_registers: {start: 0, count: 100}
        input_registers: {start: 0, count: 100}
        state_sealing:
          area: coil
          address: 0
          exception: 0x06
        rbe:
          holding_registers:
            - {id: 1, name: Watched_HR_10, start: 10, count: 1}
          input_registers:
            - {id: 2, name: Watched_IR_2_3, start: 2, count: 2}
            - {id: 3, name: Watched_IR_4_5, start: 4, count: 2}
```

## TCP delivery semantics

MMA2 listens; subscribers establish a persistent connection. No handshakes, snapshots or replay are encoded in the one-byte wire format. A client must reconcile via Modbus after EVERY connect/reconnect. There is no sequence number and therefore no loss/gap detection for dropped events while the socket stays connected; maintain periodic Modbus reconciliation. Use bounded per-subscriber queues; if a queue overflows, disconnect that subscriber to force resynchronization. A disconnected client misses events until it reconnects. TCP output requires trusted-network firewall/bind protection; the byte stream has no built-in authentication.

## Performance and atomicity

Only observe old state when a matching rule intersects the incoming write. The previous state and seal probe must belong to the actual committed write under a single memory lock. Transport-level Read→Compare→Write is racy. Large Raw Ingest writes can make whole-write snapshots expensive; correctness and memorycore purity take priority. Optimize with generic, not RBE-specific, memory operations if profiling demonstrates a need. For bit areas use packed LSB-first comparisons and ignore unused high bits.

## Multi-register and momentary-command caveats

Two-register values must be updated with a single coherent write, not two independent half-register writes. Momentary bits can disappear before the Modbus follow-up read: design latched/acknowledged command semantics if the subscriber needs to observe them. State sealing suppresses every rule under the sealed memory. Independent protection remains necessary for safety-critical actions. An already queued event may arrive after sealing; the subscriber must tolerate a Modbus sealing exception.

## Cutover / current status

Implementation is in progress on `feature/rbe-tcp-v1`. Existing notify remains available for non-RBE configurations until replacement passes verification. Do not delete `docs/MMA_Notification_Engine_LOCKED.md` before the replacement is approved, implemented and verified. Tests required: identical/change/overlap, concurrent writers, sealing transitions, unused bits, end-to-end Modbus/Raw Ingest, TCP disconnect/overflow, optional Influx isolation, `go test ./...`, race tests and actual subscriber p95/p99 latency measurement. No measured latency guarantee is made.
