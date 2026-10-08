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
- Preferred snapshot is fixed-layout binary `.bin` with address-to-file offsets, enabling word-level (`WriteAt` 2 bytes) and bit-byte-level (read-modify-write containing byte) updates without a whole-file rewrite.
- Initial snapshot creation and layout changes may use temporary-file atomic replacement. Runtime changes should use targeted offsets; the exact crash-consistency mechanism must be planned explicitly. Do not claim in-place writes have whole-snapshot atomicity.
- Use CRC32 per fixed-size data block (initial candidate 256 bytes) so targeted changes only require their affected block checksum. Exact layout is finalized during P04.
- Maintain a known-good backup image `snapshot.bak` separately from the current `snapshot.bin`. Never refresh the backup from a primary that has not passed integrity validation; never destroy the only valid recovery copy.
- At startup validate primary first; if invalid, validate backup; if backup valid, restore and report recovery. If neither valid, enter FAILED without making partial/corrupt data externally available.
- Define backup rotation/checkpoint rules so in-place writes never erase the only recoverable state. CRC32 identifies mismatches but does not itself guarantee atomicity of data+CRC writes.
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
5. Choose binary fixed-offset layout and word-level update contract. Specify header/identity/layout metadata, alignment, bit packing, update granularity, CRC32 block granularity, flush/sync, backup generation and recovery/rotation rules. Verify that no write can corrupt both primary and only good backup; reserve atomic rename for snapshot initialization/rebuild, not every word update.
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
Scope: fixed-layout `.bin` with deterministic offsets by selected memory identity, area, start and count; header version/layout validation, bit packing and register big-endian encoding. Support direct 2-byte word offsets and containing-byte bit offsets. Checksum coverage must not force a whole-file rewrite per update.
Gate: deterministic roundtrip and offset calculations; per-block CRC32 detects corruption; identity, version, length and range mismatch reject; metadata integrity validated.

### P05 — Binary disk store and targeted writes
Scope: initial atomic `.bin` creation/rebuild via temp file and rename; runtime positional writes to selected 2-byte register or containing bit byte; preserve adjacent words/bits; serialized file access and explicit sync policy.
Gate: changing one register does not rewrite unaffected payload; adjacent bits survive targeted updates; failures are reported. Keep a separately validated known-good backup before primary in-place mutations; document that ordinary in-place writes alone are not power-loss atomic.

### P06 — Startup restore orchestration
Scope: after memory allocation, before listener acceptance; no TCP loopback.
Gate: missing snapshot initializes defaults and writes first snapshot; corrupt/incompatible snapshot fails closed; no partly restored state becomes externally readable.

### P07 — Unify committed mutation observation
Scope: all direct and observed bit/register writes under appropriate synchronization; no disk IO in memorycore.
Gate: each successful committed change can mark persistence dirty exactly as required; failed writes never mark dirty; no dependency on observer/RBE and no races.

### P08 — Runtime dirty scheduler and flush
Scope: coalescing targeted dirty address ranges, serialized positional writer, dirty generations, bounded write delay, final shutdown flush; assess whether journal/checkpoint is necessary for requested power-loss consistency.
Gate: successful writes eventually reach their correct offsets without rewriting unrelated words; concurrent changes are not lost; abrupt crash guarantees and limitations explicitly documented.

### P09 — Runtime failure handling
Scope: transition to FAILED, diagnostics and retry/recovery policy, with no false success claims.
Gate: disk-full/permission/partial positional write/sync failure is observable; corrupt primary restores from verified backup and reports degraded recovery; both invalid yield FAILED (no unsupported atomicity claims).

### P10 — Independent transport coverage
Scope: FC5, FC6, FC15, FC16, Raw Ingest, internal writes, including discrete inputs and input registers.
Gate: all mutations covered with RBE disabled and enabled, preserving existing protocol/ACK behavior.

### P11 — Independence and isolation
Scope: State Sealing variants; multi-port/multi-UnitID; cross-memory concurrency.
Gate: State Sealing stays untouched; snapshots cannot cross identities; no RBE transport or external service needed.

### P12 — Integration, regression and documentation
Scope: end-to-end restart tests, missing/corrupt/incompatible cases, change of ranges/layout, ordinary shutdown, docs/examples.
Gate: start → write → confirm targeted bytes on disk → stop → restart → read same values works without external assistance; corrupt primary falls back to valid backup; corrupt both fail closed; unchanged words and neighboring bits are preserved; old behavior preserved when disabled.

### P13 — Independent VERIFY gate
Scope: separate review of all prior commits against locked contract, go test/race where practical, reproducible evidence; no implementation under this gate.
Gate: no unresolved HARD assumptions, protocol drift, concurrency races, partial restoration, or hidden coupling. Record PASS/FAIL and findings.

## Execution discipline

For each Pxx: inspect bounded files; list HARD/SOFT assumptions; implement only one micro-task; add tests; run tests; commit; normal non-force push; record paths, exact test command/results and SHA; update handoff; stop. Do not merge or force push. Do not implement during this planning invocation.

## First CODE handoff

READY: P01 — Schema and validation. Before CODE, recheck main/branch state and read live `internal/config/config.go`, config validation/loading files, examples, and startup wiring. Resolve config placement, path and ranges by evidence; implement only P01; tests, commit and non-force push; hand off P02.
