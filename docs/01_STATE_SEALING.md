# State Sealing — Contract (MMA2.0)

## Status
**Authoritative · Locked**

Any implementation, documentation, or behavior that deviates from this contract is **invalid**, even if it compiles or appears to work.

---

## 1. Purpose

**State Sealing** is a startup safety mechanism that prevents Modbus clients from interacting with a memory instance until external systems have completed initialization.

It exists to:
- prevent controllers acting on uninitialized or default values
- ensure deterministic and safe startup behavior
- preserve MMA’s role as a dumb, predictable memory appliance

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

## 3. Default Behavior (Critical)

### When `state_sealing` is NOT specified in the configuration

- State Sealing is **disabled**
- Memory starts in **RUN**
- **Modbus:** allowed immediately
- **Ingest:** allowed
- Behavior is **identical to legacy MMA**

This is the absolute default.

---

### When `state_sealing.enable = false`

```yaml
state_sealing:
  enable: false
```

Behavior is **identical** to not specifying State Sealing at all.

---

## 4. Enabling State Sealing

State Sealing is enabled **explicitly per memory**.

```yaml
state_sealing:
  enable: true
  flag_location:
    area: discrete_inputs
    address: <uint16>
```

Effects:
- Memory starts in **PRE-RUN (locked)** on process start
- No implicit start state exists
- Restart always returns the memory to PRE-RUN

---

## 5. Lifecycle States

Each memory may exist in **exactly one** lifecycle state.

### PRE-RUN (Locked)
- **Modbus:** ❌ blocked (all function codes, reads and writes)
- **Ingest:** ✅ allowed

### RUN (Unlocked)
- **Modbus:** ✅ allowed
- **Ingest:** ✅ allowed

No additional states exist.

---

## 6. Unlock Mechanism (Only One)

- Unlock is triggered **only via ingest**
- Writing value `1` to the configured `flag_location`:

```
PRE-RUN → RUN
```

- Writing `0`:
  - has no effect
  - does not re-lock
  - does not error

Modbus can **never** trigger unlock.

---

## 7. One-Way Lifecycle Rule

- Runtime re-locking is **not supported**
- Transition:

```
RUN → PRE-RUN
```

is forbidden

- Restart is the **only** way to return to PRE-RUN

This rule is intentional and non-negotiable.

---

## 8. Flag Location Semantics

- The flag is evaluated **only while the memory is in PRE-RUN**
- MMA **must not**:
  - write back `0`
  - clear the flag
  - reserve the address
  - mutate memory for lifecycle reasons

After unlock:
- the flag location becomes **ordinary memory**
- its value is whatever the last writer set
- MMA no longer treats it specially

---

## 9. Modbus Behavior While Locked

When Modbus targets a PRE-RUN memory:

- All requests are rejected
- No partial reads
- No fabricated data
- No read-only downgrade

Recommended response:

```
Exception 0x06 (Slave Device Busy)
```

or connection close.

---

## 10. Ingest Behavior

- Ingest is **always allowed**
- Ingest writes are accepted in PRE-RUN and RUN
- Ingest is the **only** path that can unlock State Sealing

---

## 11. Restart Semantics

- Lifecycle state is **not persisted**
- On restart:
  - sealing-enabled memories start in PRE-RUN
  - unlock flag must be written again if required

---

## 12. Configuration Constraints

Configuration:
- may enable or disable State Sealing
- may declare the flag location

Configuration must NOT:
- define lifecycle transitions
- define data-based gates beyond the single unlock flag
- introduce protocol-specific exceptions
- create global defaults

Configuration declares **policy**, not **behavior**.

---

## 13. Explicit Non-Goals

State Sealing is **not**:
- a PLC mode selector
- a runtime safety interlock
- a workflow engine
- a semantic system
- a Modbus feature

Any proposal that adds semantics or runtime control is out of scope.

---

## 14. Final Rule of Validity

If State Sealing:
- activates without explicit config
- mutates memory automatically
- can be triggered by Modbus
- oscillates at runtime
- becomes hard to delete

**It violates this contract and must be removed.**

---

## Final Statement

> **If `state_sealing` is not explicitly enabled, the memory starts in RUN and behaves exactly like legacy MMA.**

This contract is now **complete, minimal, and frozen**.

