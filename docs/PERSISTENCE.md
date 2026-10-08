# MMA2 Native Persistence — Per-Memory Architecture (LOCKED)

> **Architecture decision:** persistence is exclusively a property of each
> `listeners[].memory[]` entry. There is **no root/global `persistence` block**,
> no global enable flag, no global persistence directory, and no global range
> selector. Current branch implementation still uses the superseded global
> configuration and **must be migrated before merging**. The YAML below defines
> the target contract; it does not yet run on the existing implementation.

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

## Implementation migration gate

The current feature branch still declares global persistence config in
`internal/config/config.go`, `internal/config/persistence.go` and related
runtime wiring. This document **supersedes** the old global contract. Migrate
the schema, validation, plan resolution, runtime storage ownership, example,
manual test configuration and tests before any merge. Run unit, race, process
restart and backup recovery tests for independently enabled/disabled memories.
No claim of implementation compliance is made by this documentation update.
