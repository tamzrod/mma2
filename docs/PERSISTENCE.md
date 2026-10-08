# MMA2 Native Persistence

Native persistence stores MMA2's authoritative raw memory on disk so that
values survive a restart. It is disabled by default and is not a transport, an
RBE subscriber, or a State Sealing mechanism.

## Configuration

```yaml
persistence:
  enabled: true
  directory: /var/lib/mma2
  ranges:
    - port: 502
      unit_id: 1
      coils:            {start: 0, count: 128}
      holding_registers: {start: 0, count: 256}
```

- `enabled`: when absent or `false`, persistence is fully disabled and existing
  configurations are unaffected.
- `directory`: required when enabled. Holds one primary snapshot and one backup
  per identity.
- `ranges`: optional. Omit it to persist **every configured area of every
  configured memory identity**. Provide it to persist explicit subsets only.
  Each entry is keyed by `(port, unit_id)`; `port` is derived from the listener
  and matched against the runtime identity. An entry with no area is rejected.
  A range must lie fully inside its allocated area; MMA never clamps or remaps.

Unselected data is not persisted and initializes normally on every start.

## Identity and files

Identity is always `(Port:uint16, UnitID:uint16)`. Each identity owns:

- `mma2-<port>-<unit_id>.bin`: the latest targeted-write image.
- `mma2-<port>-<unit_id>.bak`: a known-good recovery image.

Distinct identities never share a file. Two processes must not share one
directory: MMA takes no lock, and concurrent processes will corrupt each other.

## On-disk format

```
[ header (24B) ][ segment descriptors (16B x N) ][ metadata CRC32 (4B) ][ block CRC32 (4B x B) ][ payload ]
```

- The metadata CRC covers only the header and descriptors.
- The payload is divided into fixed 256-byte blocks, each with its own CRC32.
- Register bytes are big-endian; bit areas are packed LSB-first.
- A single register or bit change rewrites only its containing bytes and the
  CRC of the block(s) it touches. The whole file is never rehashed.

## Startup behavior

Restore runs after memory is allocated and before any listener accepts
connections.

1. If neither primary nor backup exists, memory keeps its normal initial values
   and both files are created.
2. If the primary is valid for the current layout, its payload is applied.
3. If the primary is missing or invalid but the backup is valid, the backup is
   applied, reported as degraded recovery, and used to rebuild the primary.
4. If neither is valid, startup fails closed: no partial state is exposed.

A snapshot is incompatible if its identity or layout does not match the current
configuration. Changing persisted ranges or area sizes therefore fails closed on
restart rather than silently misapplying data.

## Runtime behavior

Every committed write from any transport (Modbus FC5/6/15/16, Raw Ingest, and
internal writers) marks the changed file bytes dirty, regardless of whether RBE
is enabled. A single background writer drains dirty ranges after a bounded
coalescing delay, applies targeted writes, and flushes on orderly shutdown.

The known-good backup is refreshed once every 60 seconds from a primary that has
first passed integrity validation. A failing cycle is skipped; the backup is
never overwritten with an invalid or internally inconsistent image.

## Durability contract

- Targeted in-place writes are visible to subsequent reads immediately but are
  durable across power loss only after a sync. There is no implied per-write
  durability.
- In-place data+CRC writes are identified by CRC32 but are not themselves atomic.
  A torn write is detected at startup and recovered from the backup.
- Writes since the last completed flush may be lost on an abrupt crash.
- Disk errors are observable: the lifecycle state becomes FAILED, the last error
  is recorded, and the scheduler stops writing. No plausible-looking corrupt
  snapshot is produced and the good backup is preserved.

## Lifecycle states

Internal only: `DISABLED`, `RESTORING`, `READY`, `FAILED`. This state is not a
Modbus coil, a State Sealing address, an RBE rule, or a control plane.
