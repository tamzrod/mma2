# RBE implementation guide

Status: implementation documentation, **not a locked or production-certified contract**. Read alongside `RBE_V1_DRAFT.md`, `RBE_V1_TEST_REPORT.md`, and `MMA_Notification_Engine_LOCKED.md`. This guide describes the inspected `internal/rbe/engine.go` and `internal/rbe/tcp.go` implementation; it does not replace the legacy notify contract.

## Purpose and ownership

MMA2 is a deterministic memory appliance. RBE (report by exception) is a change trigger, **not a second register store, replication protocol, or historian**. The authoritative values remain in memorycore. Subscribers receive an indication that a configured range changed and obtain the current value separately via Modbus. The engine maintains no last-value cache: it compares the previous raw cells returned by an observed write against the incoming cells.

## Rules and change detection

An RBE rule has a globally unique nonzero byte ID, a name, a memory ID, a memory area, an absolute start address, and a count. `NewEngine` rejects ID zero, duplicate IDs, zero-length or overflowing ranges, invalid areas, and a missing sink. Configuration validation additionally checks allocated memory boundaries; see `internal/config/build_rbe_rules.go`.

On `WriteBits` or `WriteRegs`, an operation with no intersecting rule follows the ordinary memory write path. An intersecting write uses memorycore's observed write, which captures previous raw bytes and the seal probe under the same memory operation as the commit. Errors return without publishing. After a successful write, the engine checks each intersecting rule independently. If any bit or register within that rule's intersection differs from its previous raw value, it publishes that rule's ID **once per write**. An identical write, or a change outside the rule, produces no event. Overlapping rules can each produce an event. No scaled or interpreted values are compared.

## State sealing

If a memory has state sealing, the engine probes the sealing bit before the write in the same atomic memory operation. If the memory was sealed before the write, the write may still commit according to memorycore rules but produces no RBE event. The write that unseals does not generate a catch-up event. Previously queued events may arrive after sealing; subscribers must handle sealing and reconcile authoritative state. The engine does not implement a replay log.

## TCP output contract

MMA2 listens on the configured TCP socket and subscribers connect. Each event is **exactly one byte**, the rule ID `0x01` through `0xFF`. `0x00` is reserved: the engine rejects it and `TCPPublisher.Publish(0)` does nothing. There are no addresses, values, timestamps, lengths, sequence numbers, or framing headers. TCP is a stream; process every received byte as a separate event. The same event is fanned out to each connected subscriber using an independent bounded queue. `Publish` queues without network I/O on the write path.

A new connection receives neither a snapshot nor replay. A full subscriber queue causes that subscriber to be disconnected rather than silently dropping an event while leaving the connection apparently healthy. On every connection or reconnection, read the current state over Modbus before relying on notifications; retain periodic reconciliation and stale-state handling. The one-byte stream cannot detect missed events or guarantee that a momentary value remains available when read. Use latched/acknowledged commands when required. This socket provides no client authentication: restrict its listening address and network access. Do not treat it as a protection relay.

## Configuration and compatibility

See `examples/rbe-tcp.yaml` for a runnable-shaped example and `RBE_V1_DRAFT.md` for configuration details. The draft specifies `rbe.tcp.listen` and rules under `listeners[].memory[].rbe.<area>` with `id`, `name`, `start`, and `count`; IDs are globally unique 1..255. Legacy `notify` and root `rbe` cannot coexist. In-process Influx output was removed; `rbe.influx` is rejected. A historian must subscribe externally.

## Verification and limitations

`RBE_V1_TEST_REPORT.md` records a historical Go build, vet, unit, race, repeated-run, and process-level end-to-end pass on its tested branch, including sealed/unsealed behavior, identical writes, one-byte events, Modbus readback, and a negative control. These are **reported historical results**, not tests rerun for this documentation change. The report identifies missing real-subscriber p95/p99 trigger-to-read latency measurements and says the contract is not locked. Do not claim production readiness or latency guarantees from this guide.

## Implementation references

- `internal/rbe/engine.go`: rule validation, observed writes, seal check, change comparison, and publish.
- `internal/rbe/tcp.go`: subscriber accept loop, one-byte output, independent queues, overflow disconnect, and shutdown.
- `internal/memorycore/observed_write.go`: atomic previous-value and bit observation.
- `internal/config/build_rbe_rules.go`: configuration validation.
- `docs/RBE_V1_DRAFT.md`: evolving contract.
- `docs/RBE_V1_TEST_REPORT.md`: historical test evidence and remaining release gates.
