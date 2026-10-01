# MMA 2.0 — Configuration Contract

## Purpose

This document defines the **configuration contract** for MMA 2.0.

Configuration is not a convenience layer.
Configuration is **the system definition**.

All runtime behavior must be a direct consequence of configuration.

---

## Configuration as Authority

In MMA 2.0:

> **Configuration defines the appliance.**

Configuration:
- declares listeners (ports)
- declares Unit IDs per listener
- declares memory areas and sizes per Unit ID
- declares optional policies, notifications, and state sealing

There is no other source of truth.

Code must not:
- infer missing configuration
- repair invalid configuration
- generate defaults implicitly
- provide runtime configuration reload

---

## Immutability Rules

Configuration is:
- loaded once at startup
- immutable at runtime
- not reloadable
- not hot-swappable

Any configuration change requires a full process restart.

This is intentional.

---

## Validation Requirements

Configuration must be validated **before** any listener starts.

Validation failures must:
- be explicit
- include the reason
- stop the process immediately

Warnings are not allowed.
Partial startup is not allowed.

---

## Configuration Structure

Configuration is organized into sections:

### Debug (Optional)

```yaml
debug: false   # omit or set to false to suppress protocol-level log output
```

`debug` is an optional top-level boolean (default: `false` when absent).

When `false` (default): protocol-level read errors such as malformed Modbus frames are silently discarded.  
When `true`: those errors are emitted to the log, which is useful during integration testing or troubleshooting.

This setting has no effect on memory behavior, authority enforcement, or steady-state operation.

### Device Identity (Optional)

```yaml
device_identity:
  vendor_name: "github.com/tamzrod"
  product_code: "MMA2"
  major_minor_revision: "2.0"
```

`device_identity` is a top-level, process-wide Modbus identity. It is not scoped to a listener or Unit ID.

All three fields are optional independently. When omitted, the defaults are:

- `vendor_name`: `github.com/tamzrod`
- `product_code`: `MMA2`
- `major_minor_revision`: the compiled MMA2 release version

An explicitly configured value must contain 1..244 ASCII bytes. Empty, non-ASCII, or oversized values fail startup validation. Identity is resolved once at startup and is immutable.

This configuration enables the identity values but does not bypass authority. Each memory policy that should permit FC43 / MEI14 reads must include function code `43` in `allow_fc`. State Sealing also applies before identity processing.

See [MODBUS_DEVICE_IDENTIFICATION.md](MODBUS_DEVICE_IDENTIFICATION.md) for the complete protocol and security behavior.

### Listeners (Required)

```yaml
listeners:
  - id: "gate_1"
    listen: "0.0.0.0:502"
    memory:
      - unit_id: 1
        coils:
          start: 0
          count: 100
        discrete_inputs:
          start: 0
          count: 100
        holding_registers:
          start: 0
          count: 100
        input_registers:
          start: 0
          count: 100
```

**IMPORTANT:** Memory definitions MUST be nested under `listeners[].memory`. The legacy root-level `memory.memories` format is NOT supported and will cause startup failure.

Fields:
- `id`: Unique identifier for the listener (for logging)
- `listen`: Address and port (IP:port format)
- `memory`: Array of memory definitions for this listener

### Memory Definition (Required)

Each memory definition must specify:
- `unit_id`: Modbus Unit ID (0-255)
- `coils`: Start and count (optional)
- `discrete_inputs`: Start and count (optional)
- `holding_registers`: Start and count (optional)
- `input_registers`: Start and count (optional)
- `policy`: Optional per-memory access control rules
- `notify`: Optional notification rules
- `state_sealing`: Optional sealing configuration

At least one memory area must be present.

### Memory Areas

```yaml
coils:
  start: 0
  count: 100
```

- `start`: Zero-based starting address
- `count`: Number of values (must be > 0)

All four areas are optional, but at least one must be defined for a memory.

---

## State Sealing Configuration

State Sealing is optional per memory.

```yaml
state_sealing:
  enabled: true
  area: coil
  address: 0
  exception: 0x0B
```

Fields:
- `enabled`: Optional. Defaults to `true` when omitted. Set to `false` to disable sealing without removing the block.
- `area`: Memory area containing the sealing flag. **MUST be "coil" (singular) - this is the ONLY supported value when sealing is enabled.**
- `address`: Zero-based address within the coils area when sealing is enabled
- `exception`: Optional Modbus exception code for sealed requests. Must be one of `0x01`, `0x02`, `0x03`, `0x04`, `0x05`, `0x06`, `0x08`, `0x0A`, `0x0B`. Defaults to `0x06`.

**Semantics:**
- Flag bit == 0 → sealed (Modbus blocked)
- Flag bit == 1 → unsealed (Modbus allowed)

**Validation:**
- If `state_sealing.enabled` is omitted or `true`, the area must be set to "coil"
- The coils area must exist in the memory layout when sealing is enabled
- The address must be within bounds for the coils area when sealing is enabled
- `exception`, when set, must be a supported Modbus exception code
- Configuration loading fails if validation fails

**Default:**
- If `state_sealing` is absent, or `enabled: false`, the memory is unsealed (default behavior)

---

## Policy Configuration

Policies are optional per memory.

```yaml
policy:
  rules:
    - id: "allow_local"
      source_ip:
        - "127.0.0.1"
        - "192.168.0.0/16"
      allow_fc:
        - 1
        - 2
        - 3
        - 4
    - id: "deny_all"
      source_ip:
        - "0.0.0.0/0"
      allow_fc: []
```

Fields:
- `rules`: Array of access control rules evaluated top-down
- `id`: Unique rule identifier
- `source_ip`: List of CIDR or bare IP addresses (bare IPs treated as /32 or /128)
- `allow_fc`: List of allowed Modbus function codes (empty = deny)

**Evaluation:**
- Rules are evaluated top-down
- First matching rule determines access
- Default deny if no rules match or no policy present

---

## Notification Configuration

Notifications are optional per memory.

```yaml
notify:
  coils:
    - start: 0
      count: 10
      name: "critical_bits"
    - start: 10
      count: 5
  holding_registers:
    - start: 0
      count: 100
      name: "power_readings"
```

Fields:
- Per-area notify rules (coils, discrete_inputs, holding_registers, input_registers)
- `start`: Start address
- `count`: Range size
- `name`: Optional human-readable rule name

**Semantics:**
- Each rule is independent
- One write may match multiple rules
- Overlaps are allowed
- No merging or deduplication

---

## Notification Output Configuration

Influx is not part of MMA. A leftover `notify.influx` or `rbe.influx` block fails startup.

Notify events (legacy configs without root `rbe`) use the in-process stdout adapter.
Persist history from an external process that subscribes to RBE TCP or consumes notify logs.

---

## Authority Binding

Configuration must encode the authority model exactly:

Listener (Port)  
→ Unit ID  
→ Memory

Rules:
- Unit IDs are scoped to a listener (port)
- Memory exists only within a Unit ID
- Memory cannot be shared or aliased

If configuration attempts to violate these rules, it must be rejected.

---

## No Implicit Topology or Authority Defaults

Configuration that defines runtime topology, memory, or authority must be explicit.

The following are forbidden:
- implicit listeners
- implicit Unit IDs
- implicit memory areas
- implicit sizes
- implicit policies
- implicit addresses

If a required topology, memory, or authority value is missing, startup must fail.

Optional features may define **documented deterministic defaults** when their contract explicitly specifies them. These are not inferred from runtime conditions. Current examples include `debug: false`, the State Sealing default exception when sealing is configured, and the compiled `device_identity` values.

For Device Identity specifically, omission of the section or of an individual identity field selects the fixed compiled default documented above. MMA does not derive identity from the listener, Unit ID, host name, network interface, or memory contents.

---

## Startup Sequence

1. Load configuration file
2. Parse YAML structure
3. Validate all sections (ingress, memory, policies, sealing, notify)
4. Fail fast on any validation error
5. Build internal data structures (memory store, authority policies, notify registry)
6. Start listeners

If any step fails, the process exits cleanly without starting.

---

## Failure Behavior

On configuration failure:
- no listeners start
- no memory is allocated
- no partial state exists

The process must exit cleanly and predictably.

---

## Supported Memory Identities

Memory identity is always:

```
(Port:uint16, UnitID:uint16)
```

Runtime resolves all requests using this pair.

---

## Stability Guarantee

The configuration contract is designed to be stable.

New optional fields may be added in the future.
Existing rules must not be weakened.

Any change that alters the authority model or validation semantics is a breaking change.

---

**End of Configuration Contract**
