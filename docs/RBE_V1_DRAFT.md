# RBE v1 implementation draft (NOT LOCKED)

On `main` as of 2026-09-18 (PR #18 functional checkpoint, PR #19 follow-up). This document describes in-progress code and does not supersede the current locked notify contract.

## Purpose

Replace write-only notify with change-only RBE. RBE = trigger; Modbus = value. Memory remains authoritative. TCP does not carry old/new/scaled register or coil values. Influx is not part of MMA.

MMA2 is a memory appliance. RBE TCP is a generic change-notification socket. Any number of subscribers may connect. A plant controller is one possible subscriber, not the protocol and not the product.

## Configuration

Global `rbe.tcp.listen` is required when `rbe` is present (for example `:9001`). Subscribers are TCP clients. `rbe.influx` is rejected. Rules are under `listeners[].memory[].rbe.<area>`; each has `id`, `name`, `start`, and `count`. IDs are globally unique in 1..255 and rule ranges must be contained in allocated memory. Legacy `notify` and `rbe` must not coexist in one configuration. See `examples/rbe-tcp.yaml`.

## TCP v1 wire contract

- Persistent TCP connection: MMA listens; subscribers connect.
- Each event is EXACTLY ONE BYTE: configured RuleID (0x01..0xFF).
- 0x00 is reserved; no header, length, magic, name, address, sequence, timestamp, payload, or memory value.
- TCP is a byte stream: each received byte is one complete event, even when multiple bytes arrive in one read.
- One or more subscribers may connect; each has a separate bounded queue.
- No client authentication is provided by this protocol. Restrict the bind address and firewall to trusted control-network peers.
- No replay or snapshot. An event during a disconnect is not buffered for a later connection.
- A slow subscriber whose queue fills is disconnected; it must reconnect and reconcile via Modbus. A subscriber reads current Modbus state AFTER EVERY connection before trusting event notifications.
- No sequence/gap detection is possible in one byte. Keep periodic Modbus reconciliation and application-specific stale-state handling. The TCP channel is not a protection relay.

### RuleID 0x00 — DECIDED 2026-09-18

`0x00` is reserved. It is not a valid RuleID and must never appear as an event.

- YAML `id` must be 1..255. Config validation rejects 0 and values above 255.
- `rbe.NewEngine` rejects a rule whose ID is 0.
- TCP `Publish(0)` is a no-op and puts nothing on the wire.
- A subscriber that receives `0x00` has seen a protocol or transport fault, not a rule. Reconnect and reconcile over Modbus.
- `0x00` is not a gap, heartbeat, or end-of-stream marker. Overflow disconnects the slow subscriber instead of encoding a sentinel.

This closes the earlier discrepancy with the request that all 256 byte values be legal IDs. The rest of this draft remains NOT LOCKED.

## Change detection and state sealing

- Emit one event per matching independent rule if at least one raw cell in the write/rule intersection changed.
- Identical writes and changes outside rule intersections emit nothing; overlapping rules are independent.
- Probe the sealing coil in the SAME atomic memory operation that observes and commits the write. If sealed before the write, emit nothing, including for the write that unseals.
- No catch-up after unsealing. Already queued events generated before a subsequent sealing action may still arrive afterward; the subscriber must handle a Modbus sealing exception.
- Memorycore provides only neutral previous bytes and optional previous bit observation; it does not know RBE rules or output adapters.
- Raw Ingest can write all four areas; Modbus writes remain FC5/6/15/16 and keep their usual behavior and error responses.
- A multi-register value MUST be written atomically in one operation. If a command is momentary, RBE cannot guarantee that a subscriber reads it before it clears; use latched/acknowledged command semantics. Critical protection must use independent mechanisms.

## Outputs

TCP Publish enqueues without network IO on the memory-write path, disconnecting individual slow subscribers. History stores are external: subscribe to RBE TCP from another process.

## Cutover and release gates

Existing notify remains for configurations without root `rbe` during development. Do not remove the locked notify document until the replacement is reviewed and verified. The implementation needs a complete `go test ./...` and race test, end-to-end Raw Ingest/Modbus/TCP integration tests, concurrent-write and seal transition tests, connection/overflow tests, and measured end-to-end p95/p99 subscriber trigger-to-read latency before production use. There are no measured latency guarantees in this draft.
