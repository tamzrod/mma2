# MMA2 Native Persistence — CWAL Handoff Log

Companion to [NATIVE_PERSISTENCE_CWAL.md](NATIVE_PERSISTENCE_CWAL.md).
One entry per completed micro-task, with evidence. Newest first.

---

## V01 — Optional-directory and snapshot naming enhancement

Status: PASS (after formatting correction)
Verification baseline: `main` @ `4f47048d69c539248c350be3f4d9fd6db2af739a`
Formatting correction: `main` @ `592c275bae9e70d78d453c442431b9bdf3efb595`

- `persistence.enabled: true` without `directory` stores snapshots beside the loaded YAML file, even when the process working directory differs.
- Explicit `directory` remains supported; two enabled memories can share a directory.
- Snapshot filenames are now `<port>-<unit_id>.bin` and `.bak`.
- Startup migrates legacy `mma2-<port>-<unit_id>.bin`/`.bak` snapshots; primary-only, backup-only, interrupted migration, and already-valid new-format cases passed.
- Mixed enabled/disabled process restart, register read-back, corrupted-primary recovery, failing closed when both images are invalid, and RBE regression passed.
- Independent QA: `go build ./...`, `go vet ./...`, `go test ./... -count=1`, `go test -race ./... -count=1`, `test_multi_memory.py`, `test/rbe_e2e/run_test.sh` all PASS.
- Initial gofmt FAIL only in `internal/config/persistence_test.go`; subsequent `gofmt -l internal/config/persistence_test.go` produced no output and `go test ./... -count=1` PASS at `592c275`.

Earlier C06/Pxx entries below are historical snapshots of the implementation at the time, including the former mandatory/distinct-directory rule and `mma2-` filename prefix. They do not supersede the current contract in `docs/PERSISTENCE.md`.

---

## C06 — Final VERIFY of the per-memory migration

Status: PASS
Verification point: `main` @ `4761fd7` (per-memory migration merged as PR #22,
head `55366d6ea4421b4cb21fa8a19e5878fbf36896e1`)

Independent review of the committed per-memory implementation against the
locked contract, with a real-binary process regression. No feature work.

### Contract checks

- Root-level `persistence` is rejected, not silently ignored: `Config.Persistence`
  is a `*PersistenceConfig` marker and `BuildPerMemoryPersistencePlans` errors on
  any non-nil root block (`internal/config/persistence.go`,
  `internal/config/persistence_per_memory.go`). PASS.
- Disabled by default; `enabled: true` requires a local nonempty `directory`;
  omitted ranges persist all allocated areas of the owning memory; explicit
  ranges must be nonzero, non-overlapping, and inside allocation (no clamping).
  PASS (`TestPerMemoryPersistence*`, resolver tests).
- Identity `(Port, UnitID)` derived from the listener; distinct directories are
  enforced; snapshot names are `mma2-<port>-<unit_id>.bin`/`.bak`. PASS.
- Startup restores each enabled memory after allocation and before listeners
  accept; missing snapshot creates initial primary+backup; invalid primary falls
  back to a validated backup; invalid both fail closed with no partial
  restore. PASS (`restore.go`, restore tests, integration tests).
- Fixed-offset binary format (v2) with header/descriptor metadata CRC and
  per-256-byte-block CRC32; targeted range writes refresh only touched blocks;
  metadata CRC never covers the payload. PASS (`format.go`, store/format tests).
- Known-good backup refreshed on the locked 60-second cadence from an
  already-validated primary only; the scheduler starts the clock at restore and
  a periodic ticker holds the cadence while idle. A failing flush never touches
  the backup and re-marks the drained ranges. PASS
  (`scheduler.go`, `failure_test.go`, `backup_interval_regression_test.go`).
- All authoritative writers mark dirty via the neutral memorycore observer, with
  RBE on or off; reads never mark dirty; protocol/ACK unchanged. PASS
  (`observer.go`, `transport_coverage_test.go`).
- State Sealing untouched; no external service required. PASS
  (`isolation_test.go`).

### Evidence

- Toolchain: Go 1.25.0 linux/amd64.
- `go build ./...` — clean.
- `go vet ./...` — clean.
- `go test ./... -count=1` — all packages ok (config, ingress, memorycore,
  notify, persistence, rbe, transport/modbus, transport/rawingest, tools).
- `go test -race ./... -count=1` — all packages ok.
- `python3 test/persistence_manual/test_multi_memory.py` — PASS: real binary,
  units 1 and 2 enabled in distinct directories, unit 3 disabled; write, restart,
  read back (1→101, 2→102, 3→0); corrupt unit 1 primary → 0 while unit 2 stays
  102; disabled unit 3 resets.
- `bash test/rbe_e2e/run_test.sh` — PASS.

### C05 coverage completed here

The mixed enabled/disabled multi-memory process-level regression that C05 left
open is exercised end-to-end by `test/persistence_manual/test_multi_memory.py`
(separate listener port, isolated temp directories, never touches the manual
snapshot directory).

### Unresolved HARD assumptions

None new. The two P13 items stand as recorded decisions: snapshot filename and
256-byte block size are chosen conventions, and bit payload padding bits beyond
`Count` are assumed zero (they are only ever written in-range and memory starts
zeroed).

---

## P13 — Independent VERIFY gate

Status: PASS (after remediation)
Branch: feature/native-persistence

Independent review of all prior commits against the locked contract. No new
feature work; two defects found and fixed, each with a regression test.

### Findings and remediation

1. **HIGH — cross-segment dirty coalescing (fixed).** `coalesce` merged dirty
   byte ranges that were adjacent in the file but belonged to different
   segments (segments are laid out contiguously). A flush could then read a
   span with one area's interpretation, and could write bytes belonging to a
   neighboring segment using the wrong protocol/offset. Options considered:
   (a) tag ranges with the owning segment and only coalesce within a segment;
   (b) flush per segment without coalescing across areas. Chose (a) as the
   minimal, localized fix. Regression: `TestCoalesceDoesNotMergeAcrossSegments`,
   `TestSchedulerFlushDoesNotCorruptNeighboringSegment`.
2. **MEDIUM — non-byte-aligned bit segment flush (fixed).** `readBitSpanInto`
   asked for a full 8-bit byte that extended past a bit segment whose `Count` is
   not a multiple of 8 (e.g. coils count 12), triggering `ErrOutOfBounds` and
   failing the flush. Fixed by clamping to the segment's remaining bits and
   leaving padding bits zero (they are always zero on disk). Regression:
   `TestSchedulerFlushNonAlignedBitSegment`.

### Contract checks

- Disabled by default; enabled requires a directory; ranges validated inside
  allocations with no clamping — PASS.
- Identity `(Port, UnitID)` only; distinct files; no cross-identity data — PASS.
- Restore before listener acceptance; corrupt/incompatible fails closed; corrupt
  primary recovers from a validated backup — PASS.
- Fixed-offset binary layout with per-block CRC32; targeted writes never rewrite
  unrelated blocks; whole-file rehash never needed — PASS.
- Known-good backup refreshed on the locked 60-second cadence; never replaced
  with unvalidated data; a failing cycle is skipped — PASS.
- All authoritative writers (FC5/6/15/16, Raw Ingest, internal) mark dirty with
  RBE on or off; reads never mark dirty; protocol/ACK unchanged — PASS.
- State Sealing untouched; no RBE transport or external service required — PASS.
- Disk failures observable, sticky FAILED, unflushed data retained, no false
  success, no plausible corrupt snapshot — PASS.
- No synchronous disk IO on the memory write hot path; core/protocol separation
  preserved (memorycore hook is behavior-free) — PASS.

### Evidence

- `go vet ./...` — clean.
- `go test ./... -count=1` — 9 packages ok, no failures.
- `go test -race ./... -count=1` — no failures.
- `bash test/rbe_e2e/run_test.sh` — PASS.
- Process E2E (P08/P12): real binary, Modbus FC6 write → snapshot → restart →
  FC3 read-back confirmed.

### Unresolved HARD assumptions

- Snapshot filename `mma2-<port>-<unit_id>.bin`/`.bak` and the 256-byte block
  size are recorded decisions, not derived from a pre-existing convention.
- Bit payload padding bits beyond `Count` are assumed always zero; this holds
  because `writeBits` only ever writes in-range bits and memory starts zeroed.

---

## P12 — Integration, regression and documentation

Status: DONE
Branch: feature/native-persistence

### What changed

- `docs/PERSISTENCE.md` (new): configuration, identity/files, on-disk format,
  startup behavior, runtime behavior, durability contract, lifecycle states.
- `README.md`: short "Native Persistence" section linking the doc.
- `docs/example.yaml`: canonical commented `persistence` block (from P01).
- `internal/persistence/integration_test.go` (new): 4 tests.

### Decisions recorded (with evidence)

1. **End-to-end restart**: start → write (register + bit) → orderly shutdown
   flush → restart → read back the same values, with neighboring words/bits
   preserved (`TestIntegrationRestartRoundtrip`).
2. **Corrupt primary → valid backup**: recovery path restores from backup and
   reports `RestoreSource="backup"` (`TestIntegrationCorruptPrimaryRecoversFromBackup`).
3. **Layout/range change**: an existing snapshot whose layout no longer matches
   configuration fails closed (`TestIntegrationLayoutChangeFailsClosed`).
4. **Disabled preserves old behavior**: no manager activity, no files/dirs, no
   protocol effect (`TestIntegrationDisabledPreservesOldBehavior`).

### Evidence

- `go vet ./...`, `go test ./... -count=1` — all pass.
- Process E2E (P08 evidence) covered the real binary over Modbus FC6/FC3.

### Handoff

READY: P13 — Independent VERIFY gate. Separately review all prior commits against
the locked contract; run `go test`/`-race`/`go vet`; record PASS/FAIL and any
findings. No implementation under this gate.

---

## P11 — Independence and isolation

Status: DONE
Branch: feature/native-persistence

### What changed

- `internal/persistence/isolation_test.go` (new): 5 tests.

### Decisions recorded (with evidence)

1. **Snapshots cannot cross identities**: distinct `(Port, UnitID)` pairs get
   distinct files and restore only their own values
   (`TestSnapshotsIsolatedPerIdentity`).
2. **State Sealing untouched**: persistence neither reads nor writes the sealing
   metadata or its coil; the sealing flag survives restore unchanged
   (`TestStateSealingUnaffected`).
3. **Multi-port / multi-UnitID**: port 502 unit 1, port 503 unit 1, and port 502
   unit 2 are independent, matching the locked `(Port, UnitID)` identity.
4. **Cross-memory concurrency**: 60 concurrent writes across three identities,
   flushed together, each restore into a fresh manager yields that identity's own
   value under `-race`.
5. **No external dependency**: restore works with no listeners, no RBE, and no
   network (`TestNoExternalServiceRequired`).

### Evidence

- `go vet ./...`, `go test ./... -count=1` — all pass.
- `go test ./internal/persistence/ -race -count=1` — pass.

### Handoff

READY: P12 — Integration, regression and documentation. End-to-end restart test,
missing/corrupt/incompatible cases, range/layout changes, orderly shutdown,
docs and examples; confirm old behavior is preserved when persistence is
disabled.

---

## P10 — Independent transport coverage

Status: DONE
Branch: feature/native-persistence

### What changed

- `internal/persistence/transport_coverage_test.go` (new): 4 tests
  (`TestModbusWritePathsMarkDirty`, `TestModbusReadPathsDoNotMarkDirty`,
  `TestRawIngestWritePathsMarkDirty`, `TestTransportResponsesUnchangedWithObserver`).

### Decisions recorded (with evidence)

1. **All authoritative writers participate**: with persistence observing a
   memory, Modbus FC5/6/15/16 and Raw Ingest writes all mark persistence dirty.
   Covered with RBE **disabled** and **enabled** (the observer is a separate
   mechanism from RBE; Raw Ingest's direct-write path is covered too).
2. **Discrete inputs and input registers**: Raw Ingest writes to
   `discrete_inputs` and `input_registers` mark dirty. This proves the neutral
   memorycore hook — not the RBE observer — is the universal persistence path.
3. **Reads never mark dirty**: FC1/2/3/4 produce no dirty ranges.
4. **Protocol/ACK behavior unchanged**: response PDUs for FC6 and Raw Ingest are
   byte-identical to the pre-persistence constructs while the observer still
   fires; persistence adds no protocol drift.

### Evidence

- `go vet ./...`, `go test ./... -count=1` — all pass.
- `go test ./internal/persistence/ -race -count=1` — pass.

### Handoff

READY: P11 — Independence and isolation. Verify State Sealing stays untouched,
snapshots cannot cross identities, multi-port/multi-UnitID and cross-memory
concurrency behave, and no RBE transport or external service is needed.

---

## P09 — Runtime failure handling

Status: DONE
Branch: feature/native-persistence

### What changed

- `internal/persistence/lifecycle.go`: `Failed()` and `LastError()` accessors.
- `internal/persistence/scheduler.go`: `flushAll` stops once FAILED; a failed
  `flushIdentity` re-marks the drained dirty ranges, records a wrapped error,
  logs it, and transitions to FAILED; `Close` is a mutex-guarded idempotent stop.
- `internal/persistence/observer.go`: `remarkDirty`.
- `internal/persistence/failure_test.go` (new): 4 tests.

### Decisions recorded (with evidence)

1. **Observable failures**: disk-full/permission/partial-write/sync failures
   surface through `Diagnostics.LastError`/`LastErrorAt`, `State()==FAILED`,
   `Failed()`, and a startup log line. The scheduler stops attempting flushes
   once FAILED (no repeated failed writes, no false success).
2. **No plausible corrupt snapshot**: a failing flush only touches the primary
   via positional writes; the known-good backup is never written by the flush
   path or by a checkpoint whose primary failed validation.
3. **Unflushed data retained**: on flush failure the drained ranges are
   re-marked, so the lost data stays visible rather than silently dropped.
4. **FAILED is sticky**: `markSaved`/`markRestored` do not clear FAILED; recovery
   requires an explicit future path.
5. **Recovery policy**: startup already restores from a valid backup and rebuilds
   the primary (degraded recovery, P06); a runtime FAILED state keeps memory
   serving reads/writes in volatile mode and is reported, not hidden.

### Evidence

- `go vet ./...`, `go test ./... -count=1` — all pass.
- `go test ./internal/persistence/ -race -count=1` — pass.
- Failure tests: unwritable primary → FAILED + recorded error + retained dirty +
  unchanged backup; retries stop; corrupt primary never overwrites backup.

### Handoff

READY: P10 — Independent transport coverage. Verify FC5, FC6, FC15, FC16, Raw
Ingest, and internal writes all mark persistence dirty (including discrete inputs
and input registers), with RBE disabled and enabled, preserving existing protocol
and ACK behavior.

---

## P08 — Runtime dirty scheduler and flush

Status: DONE
Branch: feature/native-persistence

### What changed

- `internal/persistence/scheduler.go` (new): `Scheduler`, `NewScheduler`,
  `Start`, `Notify`, `Close`, `flushAll`, `flushIdentity`, `applyRanges`,
  `readFileSpan`, `maybeCheckpoint`; defaults `DefaultFlushDelay` (50ms) and
  `DefaultCheckpointInterval` (30s).
- `internal/persistence/store.go`: `ApplyRange` (positional span write +
  touched-block CRCs).
- `internal/persistence/observer.go`: `SetNotifier`; `MarkCommitted` notifies
  the scheduler after releasing the manager lock.
- `internal/persistence/lifecycle.go`: `memories` map + `memory(id)`.
- `cmd/mma2/main.go`: start the scheduler, point the manager's notifier at it,
  and flush on orderly shutdown (`scheduler.Close` in the shutdown list).
- `internal/persistence/scheduler_test.go` (new): 6 tests.

### Decisions recorded (with evidence)

1. **Single serialized writer**: one goroutine drains all identities; store
   methods are also mutex-serialized, so there is never a concurrent file
   writer.
2. **Bounded coalescing**: `Notify` arms a 50 ms timer; bursts coalesce before a
   flush. `DirtySnapshot` drains ranges atomically and the generation counter
   lets a future iteration detect writes arriving mid-copy.
3. **Targeted flush**: each dirty byte range is re-read from current memory and
   applied with `ApplyRange`, refreshing only the touched block CRC(s) — never a
   whole-file rewrite.
4. **Checkpoint**: the backup is refreshed (atomic rename) from a primary that
   has first passed `ParseLayout`, on a fixed 60-second cadence
   (`BackupInterval`, locked by contract) with no journal or multi-generation
   scheme. A corrupt primary is never copied over the good backup; a failed
   cycle is skipped. The interval is applied by a periodic ticker so the cadence
   holds even when idle.
5. **Shutdown flush**: `scheduler.Close` runs in the shutdown chain so pending
   dirty ranges reach disk on orderly exit.
6. **Abrupt-crash contract**: writes since the last completed flush may be lost.
   Targeted in-place writes are identified by block CRC and recovered from the
   backup at startup; no atomicity beyond that is claimed.

### Evidence

- `go vet ./...`, `go test ./... -count=1` — all pass.
- `go test ./internal/persistence/ -race -count=1` — pass.
- Process E2E: Modbus FC6 wrote `0xBEEF` to a persisted holding register; the
  binary created `mma2-15503-1.bin` + `.bak`; after restart, FC3 read back
  `0xBEEF` (response `...03 02 beef`). Value survived a full restart.

### Handoff

READY: P09 — Runtime failure handling. Make disk-full/permission/partial-write/
sync failures observable, keep FAILED sticky with diagnostics, and ensure a
failure never yields a plausible-looking corrupt snapshot (never overwrite the
good backup with unvalidated data). Define the retry/recovery policy.

---

## P07 — Unify committed mutation observation

Status: DONE
Branch: feature/native-persistence

### What changed

- `internal/memorycore/observer.go` (new): neutral `CommittedWriteObserver`
  interface and `Memory.SetCommittedWriteObserver`; `notifyCommitted` called
  under the memory lock.
- `internal/memorycore/memory.go`, `observed_write.go`: `WriteBits`,
  `WriteRegs`, `WriteBitsObserved`, `WriteRegsObserved` each notify the observer
  once per successful commit, immediately after mutation and before the lock is
  released. Failed writes (out of bounds, bad length, invalid area, bad probe)
  return before the notification.
- `internal/memorycore/observer_test.go` (new): 5 tests.
- `internal/persistence/observer.go` (new): `memoryObserver`,
  `Manager.AttachMemory`, `MarkCommitted`, exact byte-range computation
  (`committedDirtyRanges`, `byteSpanWithinSegment`), `coalesce`,
  `DirtySnapshot`/`DirtyRanges`, per-identity generation counter.
- `internal/persistence/lifecycle.go`: cache `layouts` at construction;
  `LayoutFor`; `dirty`/`dirtyGen` maps.
- `internal/persistence/observer_test.go` (new): 8 tests.
- `cmd/mma2/main.go`: `AttachMemory` for each persisted identity after restore.

### Decisions recorded (with evidence)

1. **One canonical hook, in memorycore, behavior-free**: the observer callback
   carries only `(area, address, count)`. memorycore knows nothing about
   persistence/RBE; it performs no IO and holds no dependency upward. This is a
   neutral primitive, not a persistence feature.
2. **Exactly one notification per successful commit; none on failure**: every
   commit path notifies after the mutation under the same lock; every failure
   path returns before notifying. This is independent of RBE/observer presence
   and covers `DiscreteInputs` and `InputRegs` too.
3. **No disk IO on the write hot path**: `OnCommittedWrite` only computes byte
   ranges and coalesces them under the manager mutex; the flush happens
   off-path in P08.
4. **Exact byte ranges, not whole segments**: bit writes mark the containing
   byte span; register writes mark 2 bytes/register. Ranges are intersected with
   persisted segments, so unpersisted areas never dirty persistence.
5. **Ordering/no dropped notifications**: marking happens synchronously under
   the memory write lock, so a committed write can never race past the mark;
   `dirtyGen` lets P08 detect writes that arrive during a copy.
6. **Cached layouts**: layout is computed once per identity at `New`, removing
   per-write allocation on the hot path.

### HARD assumptions

- `notifyCommitted` runs while the memory write lock is held, so the observer
  must be non-blocking and must not re-enter the same `Memory`. `MarkCommitted`
  takes the manager mutex, never a memory lock, so lock ordering is
  memory-lock → manager-mutex and cannot deadlock.

### Evidence

- `go vet ./...`, `go test ./... -count=1` — all pass.
- `go test ./internal/persistence/ ./internal/memorycore/ -race -count=1` — pass.

### Handoff

READY: P08 — Runtime dirty scheduler and flush. Drain dirty ranges, apply
targeted word/bit writes plus block CRC(s) via `FileStore.ApplyWord`/
`ApplyBitByte`, coalesce, serialize the writer, and flush on orderly shutdown.
Refresh the backup on a bounded checkpoint schedule. Document abrupt-crash
guarantees.

---

## Contract reconciliation — per-block CRC32 and known-good backup

Status: DONE
Branch: feature/native-persistence

The contract owner committed `605f7f9` after P03–P06 were pushed, revising the
snapshot format (per-block CRC32) and requiring a validated `snapshot.bak`
recovery image with primary→backup startup recovery. This is a forward commit
(no history rewrite) that supersedes the format/store/restore details recorded
under P04–P06 below.

### Changes

- `internal/persistence/format.go` (version 2): layout is now
  `[24B header][16B*N descriptors][4B metadata CRC][4B*B block CRC table][payload]`.
  Metadata CRC covers only header+descriptors. Payload is split into 256-byte
  blocks, each with its own CRC32. `ParseLayout` verifies every block CRC and
  rejects any mismatch; `NewLayout` exposes `BlockSize`, `BlockCRCOffset`,
  `BlockCount`.
- `internal/persistence/store.go`: `ReplaceBoth` (initial primary+backup),
  `InstallBackup`, `ReadPrimary`/`ReadBackup`, `PrimaryExists`/`BackupExists`,
  `ApplyWord`/`ApplyBitByte` now recompute and write only the touched block
  CRC(s) (one register or bit-byte never rewrites unrelated blocks).
  `BackupPath` derives `snapshot.bak` from `snapshot.bin`.
- `internal/persistence/restore.go`: startup order is primary → backup.
  Valid primary restores directly; corrupt/missing primary with a valid backup
  restores from backup, rebuilds the primary from the verified backup, and
  reports `RestoreSource="backup"` (degraded recovery); both invalid fails
  closed (FAILED) with no partial exposure. Missing both initializes and writes
  primary+backup.
- `internal/persistence/lifecycle.go`: `Diagnostics.RestoreSource`.
- Tests updated (`format_test.go`, `store_test.go`, `restore_test.go`) to cover
  metadata CRC independent of payload, per-block CRC granularity, targeted
  updates preserving other blocks, backup fallback, and corrupt-both fail-closed.

### Decisions recorded (with evidence)

1. **Block size 256 bytes** (initial candidate from the contract), recorded in
   the header so it is validated on read.
2. **Backup never refreshed from unverified data**: the backup is only written
   from an image already accepted by `ParseLayout` (initial creation, or a
   validated primary). In-place primary writes never touch the backup, so the
   only good copy cannot be erased by a primary corruption.
3. **Backup rotation/checkpoint**: P08 will refresh the backup on a bounded
   checkpoint schedule from a validated current image; targeted writes never
   touch it (preserving a recoverable baseline between checkpoints).
4. **CRC identifies, does not atomically protect**: a torn in-place data+CRC
   write is detected at startup by block CRC and recovered from the backup; no
   unsupported atomicity claim is made.

### Evidence

- `go vet ./...`, `go test ./... -count=1` — all pass.
- `go test ./internal/persistence/ -race -count=1` — pass.
- Process smoke test re-run (below) — primary and backup both created.

---

## P06 — Startup restore orchestration

Status: DONE
Branch: feature/native-persistence

### What changed

- `internal/persistence/restore.go` (new): `Manager.RestoreMemory`,
  `Manager.PersistMemory`, `Directory`, `layoutFor`, `buildMemoryImage`,
  `readSegment`, `applyPayload`, `sameLayout`, `validatePayloadRange`.
- `internal/persistence/lifecycle.go`: added `PersistedIdentities` (deterministic
  ascending port/unit order).
- `internal/persistence/restore_test.go` (new): 8 tests.
- `cmd/mma2/main.go`: after `BuildMemoryStore` and before any listener starts,
  build the persistence plan + manager, create the directory, and restore every
  persisted identity. Any restore error `log.Fatalf`s (fails closed).

### Decisions recorded (with evidence)

1. **Ordering**: restore runs after `config.BuildMemoryStore` (memory allocated)
   and before the ingress loop starts (`main.go`). Listeners therefore never
   expose unrestored memory.
2. **Missing snapshot**: normal initial memory values are captured and written as
   the first snapshot via atomic `ReplaceAtomic`.
3. **Corrupt/incompatible snapshot fails closed**: `ParseLayout` rejects bad
   magic/version/CRC; identity and full layout (segments, offsets, lengths,
   counts) must match configuration. On failure the manager becomes FAILED and
   startup aborts; memory is never partly restored (all segments validated
   before any `applyPayload`).
4. **Cross-package integration via raw memorycore primitives**: restore and
   persistence reads use the existing public `ReadBits`/`ReadRegs`/
   `WriteBits`/`WriteRegs`, so persistence needs no new memorycore API and no
   coupling. Writes land under each memory's own lock.
5. **No TCP loopback**: restore reads memory directly and, on first run, writes
   the image using values read from memory; nothing is sent over a socket.

### Evidence

- `go vet ./...`, `go build ./...` — clean.
- `go test ./... -count=1` — all packages pass (incl. `internal/persistence`).
- `go test ./internal/persistence/ -race` — pass.
- Process smoke test: binary started twice against the same directory; first run
  logged `persistence ready: 1 identities` and created
  `state/mma2-15502-1.bin` (52 bytes) before `ingress ... listening`; second run
  restored it and also logged ready. Snapshot is parseable.

### Handoff

READY: P07 — Unify committed mutation observation. Introduce a canonical
committed-write hook (no memorycore coupling, no disk IO in memorycore) so every
successful bit/register commit from any writer marks persistence dirty exactly
once; failed writes never mark dirty.

---

## P05 — Binary disk store and targeted writes

Status: DONE
Branch: feature/native-persistence

### What changed

- `internal/persistence/store.go` (new): `FileStore` with `ReplaceAtomic`
  (temp file + fsync + rename + dir fsync + stale-temp cleanup), `WriteWord`
  (2-byte big-endian positional), `WriteBitByte` (read-modify-write one bit in
  its containing byte), `Sync`, `ReadAll`, `Exists`. All public methods
  serialized by a mutex.
- `internal/persistence/store_test.go` (new): 6 tests, including concurrent
  targeted writes under `-race`.

### Decisions recorded (with evidence)

1. **Atomic for create/rebuild only**: `ReplaceAtomic` is file-level atomic
   (rename). Targeted `WriteWord`/`WriteBitByte` preserve unaffected bytes but
   are NOT power-loss atomic; documented explicitly in the type comment.
2. **Durability contract**: targeted writes are visible to subsequent reads
   immediately (page cache) but durable only after an explicit `Sync`. No
   implied per-write durability; the caller's sync schedule defines it.
3. **Stale temp cleanup**: leftovers matching `<base>.tmp-*` are removed before
   each replacement, so an interrupted replacement cannot accumulate or confuse.
4. **No inter-process lock**: two processes sharing one directory will corrupt
   each other; operators must use distinct directories (carried from P01).

### Evidence

- `go vet ./internal/persistence/` — clean.
- `go test ./internal/persistence/ -count=1 -race` — pass.

---

## P04 — Fixed-offset binary snapshot format and codec

Status: DONE
Branch: feature/native-persistence

### What changed

- `internal/persistence/format.go` (new): `SegmentLayout`, `Layout`,
  `NewLayout`, `EncodeHeader`, `ParseLayout`, `BuildSnapshot`,
  `RegisterOffset`, `BitByteOffset`.
- `internal/persistence/format_test.go` (new): 5 tests.

### Decisions recorded (with evidence)

1. **Layout**: `[24-byte header][16 * N descriptors][4-byte header CRC][payload]`.
   Header carries magic `MMA2PERS`, version `1`, `(Port, UnitID)`, segment count,
   and payload start. Each descriptor carries area, start, count, file offset and
   payload length. Register value is always the low 16 bits.
2. **Checksum coverage**: the CRC32 covers only the header and descriptors, never
   the payload — so a single word/bit update never rewrites or rehashes the whole
   file. This is the revised contract from `d8c9fe4`.
3. **Deterministic offsets**: `PayloadStart = 24 + 16*N + 4`; segments laid out in
   the resolver's deterministic order; `TotalSize = PayloadStart + Σ lengths`.
   Length = `ceil(count/8)` for bit-areas, `count*2` for register-areas.
4. **Targeted offset helpers**: `RegisterOffset` (2 bytes) and `BitByteOffset`
   (containing byte + bit index), both verifying the address is inside a
   persisted segment.
5. **Parse rejects**: short/truncated files, bad magic, unsupported version,
   header CRC mismatch, zero-count or malformed descriptors, wrong descriptor
   length, non-contiguous/mismatched offsets, and truncated payload.

### Evidence

- `go test ./internal/persistence/ -count=1` — pass (roundtrip + tamper cases).

---

## P03 — Internal persistence lifecycle and diagnostics

Status: DONE
Branch: feature/native-persistence

### What changed

- `internal/persistence/lifecycle.go` (new): `State`
  (DISABLED/RESTORING/READY/FAILED), `Diagnostics`, `Manager`, `New`,
  `Enabled`, `State`, `Segments`, `Diagnostics`, and internal transition helpers.
- `internal/persistence/lifecycle_test.go` (new): 5 tests.

### Decisions recorded (with evidence)

1. **Persistence owner**: `Manager` holds the resolved ordered segments per
   identity and lifecycle state; it is entirely internal and separate from State
   Sealing.
2. **State model**: `New(nil, ...)` → DISABLED; a non-nil plan starts in
   RESTORING and only becomes READY after a successful restore (P06). FAILED is
   sticky and records the last error and timestamp.
3. **Diagnostics**: snapshot of state, directory, identity/segment counts, and
   last error/restore/save times — loggable, read-only, no new control plane.
4. **No protocol surface**: no Modbus coil, State Sealing address, RBE rule, or
   external service is used to observe or control persistence state.

### Evidence

- `go test ./internal/persistence/ -count=1` — pass.
- `go vet ./internal/persistence/` — clean.

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
