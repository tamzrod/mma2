# MMA2 Native Persistence — CWAL Handoff Log

Companion to [NATIVE_PERSISTENCE_CWAL.md](NATIVE_PERSISTENCE_CWAL.md).
One entry per completed micro-task, with evidence. Newest first.

---

## P02 — Persisted-range resolver

Status: DONE
Branch: feature/native-persistence

### What changed

- `internal/persistence/resolver.go` (new package): `Segment`, `ResolveSegments`,
  `fullAreaSegments`, `explicitSegments`, `sortAndCheckSegments`, `orderedAreas`.
- `internal/persistence/resolver_test.go` (new): 9 tests.
- `internal/config/persistence.go`: added `MemoryAllocation` and
  `BuildMemoryAllocations`; factored `areaAllocations` (shared by validation
  and the resolver) so allocations are enumerated once from config evidence.

### Decisions recorded (with evidence)

1. **New package `internal/persistence`** rather than adding to `internal/config`:
   the resolver is the seed of the persistence owner described in P03. It depends
   on `config` (types) and `memorycore` (`Area`, `MemoryID`); it does not touch
   `memorycore` behavior, preserving core/protocol separation.
2. **Omitted ranges cover all allocated areas**: with no explicit ranges, the
   resolver emits the full allocation of every configured identity via
   `BuildMemoryAllocations`, ordered by the fixed area order
   (coils, discrete_inputs, holding_registers, input_registers).
3. **Explicit subsets**: emitted exactly as validated, unselected areas absent
   (unselected data still initializes normally in memorycore). Unallocated
   selections, out-of-allocation selections, zero counts, empty entries, and
   unknown identities all reject cleanly (defense in depth over P01 validation).
4. **Deterministic ordering**: partial insertion sort by area rank then ascending
   start; verified by `TestResolveSegmentsDeterministicOrdering`.
5. **Overlap rejection**: two segments in the same area that overlap are
   rejected (`sortAndCheckSegments`). P01 already rejects duplicate identity
   entries, so overlap is only reachable through future callers/tests.

### HARD assumptions

- Segment units match the area: bits for bit-areas, registers for register-areas
  (`memorycore.AreaLayout.Size` semantics). P04 must encode accordingly.

### Evidence

- `go vet ./internal/persistence/ ./internal/config/` — clean.
- `go test ./internal/persistence/ ./internal/config/ -count=1` — pass.

### Handoff

READY: P03 — Internal lifecycle and diagnostics model. Build the persistence
owner with DISABLED/RESTORING/READY/FAILED and last error/save/restore, wired to
the resolver output. No Modbus coil, State Sealing, RBE rule, or control plane.

---

## P01 — Schema and validation

Status: DONE
Branch: feature/native-persistence

### What changed

- `internal/config/config.go`: added optional root-level `persistence` block to `Config`.
- `internal/config/persistence.go` (new): `PersistenceConfig`, `PersistenceRange`,
  `PersistenceArea`, `ValidatePersistence`, `BuildPersistencePlan`,
  `ResolvedPersistence`, `SnapshotFileName`, `SnapshotPath`.
- `internal/config/validate.go`: `Validate` now calls `ValidatePersistence` after
  memory validation.
- `internal/config/persistence_test.go` (new): 10 tests.
- `docs/example.yaml`: documented (commented-out) canonical `persistence` block.

### Decisions recorded (with evidence)

1. **Placement**: root-level `persistence` block, matching existing process-wide
   top-level sections (`notify`, `rbe`, `access_events`) in `internal/config/config.go`.
   Chosen over per-memory nesting because snapshot storage is a process-wide
   filesystem concern and the enabled flag spans identities.
2. **Disabled-by-default and no effect on old configs**: `ValidatePersistence`
   returns nil immediately when the block is absent or `enabled: false`. Contents
   are validated only when enabled. Proven by `TestPersistenceDisabledByDefault`
   and `TestPersistenceDisabledExplicit`.
3. **Path strategy**: `directory` is required when enabled (no hidden default);
   one deterministic snapshot file per identity, `mma2-<port>-<unit_id>.bin`
   (`SnapshotFileName`), matching the fixed-offset binary layout adopted in the
   revised contract (P04/P05). Distinct `(Port, UnitID)` pairs map to distinct
   names. Multi-instance collision policy: two processes sharing one directory
   collide; the operator must give each process a distinct directory. MMA takes
   no lock.
4. **Ranges**: optional list keyed by `(port, unit_id)`. Each entry may select any
   subset of the four areas as explicit `{start, count}`. Entry with no area
   selection is rejected. Non-empty `ranges` means "only these selections";
   omitted ranges mean "every configured area of every configured identity"
   (`ResolvedPersistence.RangesEmpty`).
5. **Identity**: matched against listener-derived `(Port, UnitID)`; YAML `port`
   is a lookup key only. Consistent with `BuildMemoryStore` and
   `BuildDeviceIdentities` which derive port via `parseListenPort(listener.Listen)`.
6. **No clamping/remapping**: each selection must lie fully inside its allocated
   area or validation fails. Holes and out-of-bounds reject cleanly.
7. **Determinism**: invalid config fails `Validate` before any listener starts,
   consistent with the fail-fast startup contract.

### HARD assumptions

- Snapshot naming `mma2-<port>-<unit_id>.bin` under `directory` is the chosen
  contract (no existing repository convention found; recorded as the decision).
- `persistence.ranges[].unit_id` limited to `<= 255`, matching existing
  `unit_id` validation (`validateNestedMemoryDef`, `BuildDeviceIdentities`).
- Rebasing note: after P01 was first drafted, the contract owner committed
  `d8c9fe4` revising the locked snapshot format to fixed-layout binary `.bin`
  with targeted word-level writes (not whole-file integrity-protected
  snapshots). This work is rebased onto that head; the `.bin` naming already
  reflects it. P04/P05 must follow the revised contract.

### Evidence

- `go vet ./...` — clean.
- `go test ./... -count=1` — all packages pass.
- `go test ./internal/config/` — pass (includes new `persistence_test.go`).

### Handoff

READY: P02 — Persisted-range resolver. Recheck branch state, then translate the
validated `ResolvedPersistence` plan plus allocated layouts into ordered segments
per `MemoryID`. Note: `BuildPersistencePlan` (P01) already normalizes explicit
selections; P02 must extend it to expand the "ranges empty" default into concrete
ordered segments over all configured areas and to produce deterministic segment
ordering. Keep persistence out of `memorycore`.
