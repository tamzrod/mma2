# MMA2 Native Persistence — Per-Memory Architecture (LOCKED)

> **Architecture decision:** persistence is exclusively a property of each
> `listeners[].memory[]` entry. There is **no root/global `persistence` block**,
> no global enable flag, no global persistence directory, and no global range
> selector. This per-memory design is implemented and verified on
> `feature/native-persistence` (PR #22). The YAML below is supported by that
> branch; it becomes available on `main` when the PR is merged.

Native persistence stores raw MMA2 memory on disk, independently of RBE, Modbus
transport, and State Sealing. Each `(Port, UnitID)` decides whether and what
to persist, including the storage directory.

## Canonical YAML

```yaml
listeners:
  - id: main
    listen: ":502"
    memory:
      - unit_id: 1
        coils: {start: 0, count: 128}
        holding_registers: {start: 0, count: 256}
        persistence:
          enabled: true
          directory: /var/lib/mma2/unit1
          # ranges omitted = all allocated areas of THIS memory
        policy:
          rules:
            - id: local
              source_ip: [127.0.0.1]
              allow_fc: [1, 2, 3, 4, 5, 6, 15, 16]

      - unit_id: 2
        holding_registers: {start: 0, count: 100}
        persistence:
          enabled: false  # optional; omitted also means disabled

  - id: other
    listen: ":503"
    memory:
      - unit_id: 1
        coils: {start: 0, count: 64}
        holding_registers: {start: 0, count: 100}
        persistence:
          enabled: true
          directory: /var/lib/mma2/other-unit1
          ranges:
            holding_registers:
              - start: 20
                count: 10
              - start: 60
                count: 10
```

## Configuration rules

- The block is accepted **only** inside the memory entry that owns it.
- Missing `persistence`, or `enabled: false`, means disabled **for that memory only**.
- `enabled: true` requires a nonempty `directory` **in the same memory entry**. No global/default directory is inferred.
- Omitted `ranges` means every configured area **of that same memory only** (coils, discrete inputs, holding registers, input registers).
- If `ranges` is present, only explicitly listed areas/ranges are persisted. Range arrays follow the direct existing multi-block style (no `segments:` wrapper). Ranges must be positive, nonoverlapping, contained in allocated memory, and never silently clamped or remapped.
- Root-level `persistence` must fail validation as unsupported, rather than silently being ignored.
- Duplicate storage targets across memories/process instances must fail validation where discoverable. Paths used for snapshots must be isolated to prevent collisions.
- One memory's disabled or failed persistence cannot silently alter another memory's persistence configuration. Before listener exposure, an invalid required snapshot still fails closed for startup.
- Configured listener port plus UnitID determines memory identity; neither a directory nor YAML key affects routing.

## Identity, files, and disk behavior

Identity remains `(Port:uint16, UnitID:uint16)`. Each enabled memory owns
`mma2-<port>-<unit_id>.bin` and `mma2-<port>-<unit_id>.bak` in **its own**
configured directory. There are no shared/global snapshot settings.

The snapshot is fixed-offset binary: versioned metadata, 256-byte CRC32 payload
blocks, LSB-first bits, big-endian registers, targeted writes and background
coalesced flushes. The known-good backup refreshes every 60 seconds **per
enabled memory** and is installed by validated atomic replacement. No journal
or multi-generation backup system.

Startup restores each enabled memory before protocol listeners accept requests.
Missing snapshots create initial primary+backup from normal initial values.
Invalid primary falls back to a verified compatible backup. If neither validates,
startup fails closed without exposing partial restored values.

Only changes committed to the matching memory may dirty its snapshot. Disabled
memories never create snapshot files. Persistence must not depend on RBE or
State Sealing, and disk IO must not block Modbus writes.

## Manual Modbus Poll test

Use `test/persistence_manual/config.yaml` and run from the repository root:

```bash
bash test/persistence_manual/run.sh
```

The manual listener binds to `:15030` and exposes Unit ID 1. Connect Windows
Modbus Poll to the Linux host's actual IP address and TCP port 15030; select
FC3 (holding registers) and write a test value with FC6. Stop MMA2 using Ctrl+C,
then start the script again and read back the same register. The value should
survive a graceful restart. The sample permits any IPv4 client
(`0.0.0.0/0`) **for lab testing only**; restrict `source_ip` for deployment.

Snapshots remain in `test/persistence_manual/snapshots/` across runs of
`run.sh`. **Do not run `test.py` against snapshots you want to retain:**
it resets this test-owned directory at the beginning. The Python test checks
startup, register/coil writes, primary restore and deliberately corrupted
primary fallback to backup.

The startup log for an enabled memory should contain
`persistence ready: port=15030 unit=1 directory=...`.
`persistence disabled (no enabled memories)` indicates there are no enabled
per-memory definitions. Ensure another MMA2 process does not already own port
15030 before manual testing.

## Verification and limitations

- The operator reports that per-memory persistence now works in manual testing.
- Independent verification reported PASS for `go vet ./...`,
  `go test ./... -count=1`, `go test -race ./... -count=1`,
  `python3 test/persistence_manual/test_multi_memory.py`, and
  `bash test/rbe_e2e/run_test.sh`. The per-memory implementation is merged to
  `main` (PR #22); the final VERIFY (C06) was run on `main` at `4761fd7` with
  Go 1.25.0.
- Persistence makes no per-write power-loss durability guarantee. Updates since
  the last completed disk flush can be lost after an abrupt termination.
- A bad primary is detected with CRC32 and restored from a valid backup. The
  backup runs on a 60-second interval; recovery can roll state back.
- Per-memory directories must be distinct; sharing a directory between
  processes is not a supported locking model.
