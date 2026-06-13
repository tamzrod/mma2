# State Sealing — Contract (MMA2.0)

## Status

**Authoritative · Implementation-Aligned**

This document reflects the current implemented behavior of state sealing in MMA 2.0.

---

## 1. Purpose

**State Sealing** is a memory-scoped safety mechanism that prevents Modbus clients from accessing a memory instance while it is sealed.

It exists to:
- prevent controllers from acting on uninitialized or incorrect values
- ensure deterministic and safe startup behavior
- preserve MMA's role as a dumb, predictable memory appliance

State Sealing is a **guard rail**, not a control system.

---

## 2. Scope & Identity

State Sealing is scoped **per memory instance**.

Memory identity is defined **only** by:

```
MemoryID = (Port:uint16, UnitID:uint16)
```

State Sealing is:
- not global
- not listener-scoped
- not IP-scoped
- not protocol-owned

Lifecycle state is **owned by the memory**, not by Modbus.

---

## 3. Default Behavior

### When `state_sealing` is NOT specified in the configuration

- State Sealing is **disabled**
- Memory is **unsealed by default**
- **Modbus:** allowed immediately
- **Raw Ingest:** allowed
- Behavior is **identical to legacy MMA**

This is the absolute default.

---

## 4. Enabling State Sealing

State Sealing is enabled when the `state_sealing` section is present and either:

- `enabled` is omitted (backward-compatible default), or
- `enabled: true`

```yaml
state_sealing:
  enabled: true
  area: coil
  address: 0
  exception: 0x0B
```

To keep the block but disable sealing explicitly:

```yaml
state_sealing:
  enabled: false
```

**Effects:**
- Memory starts **sealed** on process start
- The sealing flag is located at the specified area and address
- Restart always returns the memory to sealed state

---

## 5. Sealing Flag Semantics

The sealing flag is a single bit (one coil or discrete input bit).

**Semantics:**

```
Flag value == 0  →  SEALED   (memory inaccessible to Modbus)
Flag value == 1  →  UNSEALED (memory accessible to Modbus)
```

The flag location is evaluated **only when the memory is sealed**.

---

## 6. Sealed State Behavior

When a memory is **sealed** (flag bit == 0):

- **Modbus TCP:** All requests are rejected with the configured exception code
- If `exception` is omitted, the default remains **0x06 (Device Busy)**
- No partial reads
- No fabricated data
- No read-only downgrade
- **Raw Ingest:** NOT affected by sealing; writes always allowed

---

## 7. Unsealing Behavior

When the flag bit becomes 1:

- Memory transitions to unsealed
- Modbus TCP requests are allowed
- Raw Ingest continues as normal
- The transition is immediate upon flag change

---

## 8. Flag Location Constraints

The sealing flag address **must be valid** for its configured area.

**Validation rules:**

- The area must exist in the memory layout (coils, discrete_inputs, holding_registers, or input_registers)
- The address must fall within the bounds of that area

If validation fails at startup:

- The process must not start
- A clear error message must be logged
- Configuration loading must fail fast

---

## 9. Flag Location After Sealing Begins

While the memory is sealed, the flag location is special.

After the memory is unsealed (flag becomes 1):

- The flag location becomes **ordinary memory**
- Its value is whatever the last writer set
- MMA **does not** treat it specially
- Future sealing is not supported (restart required)

---

## 10. Restart Semantics

- Lifecycle state is **not persisted**
- On restart:
  - sealed-enabled memories start **sealed** (flag == 0)
  - the flag must be written again to unseal
  - no automatic unsealing

---

## 11. Raw Ingest Access

- Raw Ingest is **always allowed**, regardless of sealing state
- Raw Ingest can write to any memory area, including the sealing flag
- Raw Ingest writes succeed in both sealed and unsealed states

---

## 12. Protocol-Specific Behavior

### Modbus TCP

When a Modbus request targets a sealed memory:

- The request is rejected **before** it reaches memorycore
- The configured Modbus exception is returned (default **0x06 / Device Busy**)
- No memory operation occurs

### Raw Ingest

When a Raw Ingest request targets a sealed memory:

- The request proceeds normally
- The write is applied to memory
- Sealing is **not** an access control for Raw Ingest

---

## 13. Configuration Validation

At startup, configuration validation must verify:

1. If `state_sealing.enabled` is omitted or `true`, the area name must be valid
2. The area must exist in the memory layout
3. The address must be within bounds for that area
4. If `exception` is set, it must be one of `0x01`, `0x02`, `0x03`, `0x04`, `0x05`, `0x06`, `0x08`, `0x0A`, `0x0B`

Example (invalid configuration):

```yaml
state_sealing:
  area: coils
  address: 100
coils:
  start: 0
  count: 10
```

This fails: address 100 is out of bounds [0..10).

---

## 14. Architecture: Memory Core Is Unaware

The memory core:
- stores the sealing flag as ordinary data
- has no knowledge of sealing semantics
- does not enforce access control

Sealing enforcement is implemented **only** at the Modbus TCP transport layer.

---

## 15. Explicit Non-Goals

State Sealing is **not**:
- a PLC mode selector
- a runtime safety interlock
- a workflow engine
- a semantic system
- a control enforcement mechanism for all transports
- reversible without restart

Any proposal that adds these capabilities is out of scope.

---

## 16. Final Contract

If `state_sealing` is present in configuration and enabled:

- The memory is sealed at startup (flag must be 1 to unseal)
- A single flag bit controls sealed/unsealed state
- Modbus TCP rejects sealed access with the configured exception (default 0x06)
- Raw Ingest always succeeds
- Restart resets to sealed

If `state_sealing` is absent, or `enabled: false`:

- The memory is unsealed by default
- Modbus TCP and Raw Ingest both immediately allowed
- Behavior is identical to legacy MMA

**This contract is stable and reflects current implementation.**

---

**End of State Sealing Contract**
