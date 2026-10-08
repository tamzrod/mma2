# MMA2 Native Persistence — Operation CWAL Task Chain

Status: PLANNED — NO PRODUCT CODE IMPLEMENTED
Branch: feature/native-persistence
Base: main
Mode: CODE one bounded task at a time, then independent VERIFY.

## Locked contract

- Persistence is native disk-backed state for MMA2's existing authoritative raw memory, not a transport, RBE subscriber, or State Sealing mechanism.
- Configuration defaults to disabled; `persistence.enabled: true` with no ranges persists every configured area of every configured memory identity, unless planning discovers and records a justified explicit scoping alternative.
- Identity remains `(Port:uint16, UnitID:uint16)`, derived from the listener and UnitID; never from YAML keys or IP.
- Optional ranges are explicit, inside allocated areas, never silently clamped/remapped. Unselected data initializes normally.
- Restore from validated disk snapshots occurs before protocol listeners expose restored memory. Corrupt/incompatible snapshots fail closed without partial exposure.
- Missing first snapshot uses normal initial memory values and creates an initial disk snapshot.
- Writes from Modbus FC5/6/15/16, Raw Ingest, and any authoritative internal writer participate, regardless of RBE being enabled.
- Persistence lifecycle state is internal: DISABLED, RESTORING, READY, FAILED; separate from State Sealing and any ordinary memory address.
- Save uses a single deterministic, versioned, integrity-protected snapshot per selected identity or equivalent verified atomic format, with safe temporary-write/rename, bounded coalescing, and flush on orderly shutdown.
- Disk errors are observable. No guarantee of last-write durability between flushes is implied; precisely document the flush/durability contract.
- Preserve existing core/protocol separation and avoid synchronous disk IO on memory-write hot paths.

## Repository evidence inspected on main

- `internal/config/config.go`: `Config`, `IngressGate`, `MemoryDefinition`, four `Area` fields; canonical nesting at `listeners[].memory[]`.
- `internal/config/build_memory_store.go`: `BuildMemoryStore` allocates `memorycore.Memory` and registers in `Store` by `(Port, UnitID)`.
- `internal/memorycore/memory.go`: raw bit/register backing; per-memory `sync.RWMutex`; direct `WriteBits` and `WriteRegs`.
- `internal/memorycore/observed_write.go`: `WriteBitsObserved` and `WriteRegsObserved` are additional commit paths under the same memory lock.
- `internal/transport/rawingest/handle_conn.go`: switches between RBE observer writes and direct `mem.Write*` depending on observer presence. This proves an RBE observer is NOT a universal persistence hook.
- Before modifying implementation, inspect all remaining consumers, especially `internal/rbe/engine.go`, `internal/transport/modbus/dispatch_memorycore.go`, startup/listener orchestration, shutdown and config validation.

## Required pre-CODE inspection/decisions

1. Find exact startup call graph, listener acceptance boundary, normal defaults, and shutdown hooks.
2. Enumerate all memory commit paths, including observed writes and future internal mutation entrypoints.
3. Design one canonical committed-write hook or shared internal primitive without coupling memorycore to persistence or RBE. Confirm ordering/locking and how to prevent dropped dirty notifications.
4. Define snapshot path ownership and multi-instance collision policy; define exact config placement and multiple custom range syntax according to actual repository conventions.
5. Choose crash consistency contract, including fsync/rename behavior and metadata integrity, failure policy, and compatibility/range changes.
6. Record HARD assumptions with source path, exact function/type and evidence before CODE.

## Ordered micro-task chain

### P01 — Schema and validation
Scope: config definitions, parser validation, canonical example, tests.
Deliver: disabled-by-default `persistence` block, explicit disk path strategy, supported range syntax, validation including out-of-bounds, identity scope and conflicts.
Gate: invalid config fails deterministically; old configs unchanged.

### P02 — Persisted-range resolver
Scope: translate validated config and allocated layouts into ordered segments per MemoryID.
Gate: omitted ranges cover all allocated areas; custom subsets and unallocated holes reject cleanly; deterministic segment ordering.

### P03 — Internal lifecycle and diagnostics model
Scope: persistence owner only, with DISABLED/RESTORING/READY/FAILED and last error/save/restore.
Gate: no Modbus coil, State Sealing, RBE rule, or new control plane required.

### P04 — Snapshot format and codec
Scope: versioned identity/layout/segment metadata, payload, checksum, bit packing and register big-endian encoding.
Gate: deterministic roundtrip; identity, version, checksum, length and range mismatch reject.

### P05 — Atomic disk store
Scope: selected file layout, temp file write, appropriate sync/close, rename, stale temporary cleanup.
Gate: interrupted write never exposes a partially written valid snapshot; precise failure reporting.

### P06 — Startup restore orchestration
Scope: after memory allocation, before listener acceptance; no TCP loopback.
Gate: missing snapshot initializes defaults and writes first snapshot; corrupt/incompatible snapshot fails closed; no partly restored state becomes externally readable.

### P07 — Unify committed mutation observation
Scope: all direct and observed bit/register writes under appropriate synchronization; no disk IO in memorycore.
Gate: each successful committed change can mark persistence dirty exactly as required; failed writes never mark dirty; no dependency on observer/RBE and no races.

### P08 — Runtime dirty scheduler and flush
Scope: coalescing, serialized writer, dirty generations, bounded write delay, final shutdown flush.
Gate: successful writes eventually snapshot; concurrent updates are not lost; explicit guarantees for abrupt crashes and failures.

### P09 — Runtime failure handling
Scope: transition to FAILED, diagnostics and retry/recovery policy, with no false success claims.
Gate: disk-full/permission/rename failure is observable and does not produce a plausible corrupt snapshot.

### P10 — Independent transport coverage
Scope: FC5, FC6, FC15, FC16, Raw Ingest, internal writes, including discrete inputs and input registers.
Gate: all mutations covered with RBE disabled and enabled, preserving existing protocol/ACK behavior.

### P11 — Independence and isolation
Scope: State Sealing variants; multi-port/multi-UnitID; cross-memory concurrency.
Gate: State Sealing stays untouched; snapshots cannot cross identities; no RBE transport or external service needed.

### P12 — Integration, regression and documentation
Scope: end-to-end restart tests, missing/corrupt/incompatible cases, change of ranges/layout, ordinary shutdown, docs/examples.
Gate: start → write → confirm snapshot → stop → restart → read same values works without external assistance; old behavior preserved when disabled.

### P13 — Independent VERIFY gate
Scope: separate review of all prior commits against locked contract, go test/race where practical, reproducible evidence; no implementation under this gate.
Gate: no unresolved HARD assumptions, protocol drift, concurrency races, partial restoration, or hidden coupling. Record PASS/FAIL and findings.

## Execution discipline

For each Pxx: inspect bounded files; list HARD/SOFT assumptions; implement only one micro-task; add tests; run tests; commit; normal non-force push; record paths, exact test command/results and SHA; update handoff; stop. Do not merge or force push. Do not implement during this planning invocation.

## First CODE handoff

READY: P01 — Schema and validation. Before CODE, recheck main/branch state and read live `internal/config/config.go`, config validation/loading files, examples, and startup wiring. Resolve config placement, path and ranges by evidence; implement only P01; tests, commit and non-force push; hand off P02.
