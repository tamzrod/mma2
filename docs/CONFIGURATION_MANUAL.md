# MMA 2.0 Configuration Manual

## Overview

This manual provides practical guidance for configuring MMA 2.0. It is organized by use case and includes complete working examples.

For the formal configuration contract and validation rules, see [04_CONFIGURATION.md](04_CONFIGURATION.md).

---

## Table of Contents

1. [Quick Start](#quick-start)
2. [Configuration File Structure](#configuration-file-structure)
3. [Basic Examples](#basic-examples)
4. [Listener Configuration](#listener-configuration)
5. [Memory Configuration](#memory-configuration)
6. [State Sealing Configuration](#state-sealing-configuration)
7. [Access Control (Policy)](#access-control-policy)
8. [Notification Configuration](#notification-configuration)
9. [Debug Logging](#debug-logging)
10. [Access Events Configuration](#access-events-configuration)
11. [Common Use Cases](#common-use-cases)
12. [Validation and Troubleshooting](#validation-and-troubleshooting)

---

## Quick Start

### Minimal Configuration

The simplest valid MMA 2.0 configuration:

```yaml
listeners:
  - id: "main"
    listen: "0.0.0.0:502"
    memory:
      - unit_id: 1
        coils:
          start: 0
          count: 100
        holding_registers:
          start: 0
          count: 100
```

**What this does:**
- Starts a Modbus TCP listener on port 502
- Defines one memory instance at Unit ID 1
- Provides 100 coils (addresses 0-99)
- Provides 100 holding registers (addresses 0-99)
- No access control (all IPs allowed)
- No state sealing (memory immediately accessible)

---

## Configuration File Structure

MMA 2.0 uses YAML configuration files with the following top-level sections:

```yaml
debug: false        # OPTIONAL - Enable verbose protocol-level logging (default: false)

listeners:          # REQUIRED - TCP ingress definitions
  - id: ...
    listen: ...
    memory: ...     # REQUIRED: Use "memory" (NOT "memories")

notify:             # OPTIONAL - Global notification output adapters
  influx: ...
```

### Key Principles

1. **Memory Identity**: Every memory instance is uniquely identified by `(Port, UnitID)`
2. **Nested Model**: Memory definitions are nested under listeners using `memory:` field
3. **Legacy Model Not Supported**: Root-level `memory.memories` is NOT supported and causes startup failure
4. **Immutability**: Configuration is loaded once at startup and never reloaded
5. **Fail-Fast**: Invalid configuration prevents startup entirely
6. **Silent by Default**: Protocol-level read errors are suppressed unless `debug: true`

---

## Basic Examples

### Example 1: Single Memory Instance

```yaml
listeners:
  - id: "default"
    listen: ":502"
    memory:
      - unit_id: 1
        coils:
          start: 0
          count: 256
        discrete_inputs:
          start: 0
          count: 256
        holding_registers:
          start: 0
          count: 1000
        input_registers:
          start: 0
          count: 1000
```

### Example 2: Multiple Unit IDs on Same Port

```yaml
listeners:
  - id: "modbus_main"
    listen: "0.0.0.0:502"
    memory:
      - unit_id: 1
        holding_registers:
          start: 0
          count: 100
      
      - unit_id: 2
        holding_registers:
          start: 0
          count: 200
      
      - unit_id: 3
        holding_registers:
          start: 0
          count: 150
```

**Note:** Each Unit ID has independent memory. Register 0 at Unit ID 1 is completely separate from register 0 at Unit ID 2.

### Example 3: Multiple Listeners (Ports)

```yaml
listeners:
  - id: "production"
    listen: "0.0.0.0:502"
    memory:
      - unit_id: 1
        holding_registers:
          start: 0
          count: 1000
  
  - id: "testing"
    listen: "0.0.0.0:503"
    memory:
      - unit_id: 1
        holding_registers:
          start: 0
          count: 100
```

**Note:** Unit ID 1 on port 502 is a different memory than Unit ID 1 on port 503.

---

## Listener Configuration

### Fields

```yaml
listeners:
  - id: "unique_identifier"      # REQUIRED: unique string identifier
    listen: "address:port"       # REQUIRED: listen address
    memory: []                   # REQUIRED: array of memory definitions
```

### Listen Address Formats

```yaml
# Listen on all interfaces, port 502
listen: ":502"
listen: "0.0.0.0:502"

# Listen on specific interface
listen: "192.168.1.100:502"

# Listen on localhost only
listen: "127.0.0.1:502"
listen: "localhost:502"

# IPv6
listen: "[::]:502"
listen: "[::1]:502"
```

### Multiple Listeners Example

```yaml
listeners:
  - id: "public_interface"
    listen: "192.168.1.100:502"
    memory:
      - unit_id: 1
        holding_registers:
          start: 0
          count: 100
  
  - id: "internal_interface"
    listen: "10.0.0.50:502"
    memory:
      - unit_id: 1
        holding_registers:
          start: 0
          count: 200
```

---

## Memory Configuration

### Memory Definition Structure

```yaml
memory:
  - unit_id: 1                    # REQUIRED: 0-255

    # All areas are OPTIONAL, but at least one must be present
    coils:
      start: 0                    # Starting address (zero-based)
      count: 100                  # Number of coils
    
    discrete_inputs:
      start: 0
      count: 100
    
    holding_registers:
      start: 0
      count: 1000
    
    input_registers:
      start: 0
      count: 1000
    
    # OPTIONAL configurations
    state_sealing:                # State sealing (see dedicated section)
      area: coil
      address: 0
    
    policy:                       # Access control (see dedicated section)
      rules: []
    
    notify:                       # Notification rules (see dedicated section)
      coils: []
```

### Memory Area Guidelines

**Start and Count:**
- `start`: Zero-based starting address (0-65535)
- `count`: Number of values (1-65535)
- Valid addresses: `[start, start + count)`
- Must not exceed 16-bit address space

**Examples:**

```yaml
# Valid: addresses 0 through 99
coils:
  start: 0
  count: 100

# Valid: addresses 1000 through 1999
holding_registers:
  start: 1000
  count: 1000

# Invalid: exceeds 16-bit space
holding_registers:
  start: 60000
  count: 10000  # 60000 + 10000 = 70000 > 65535
```

### Minimal Memory Configurations

```yaml
# Coils only
memory:
  - unit_id: 1
    coils:
      start: 0
      count: 256

# Registers only
memory:
  - unit_id: 1
    holding_registers:
      start: 0
      count: 1000

# Mixed (common pattern)
memory:
  - unit_id: 1
    coils:
      start: 0
      count: 128
    holding_registers:
      start: 0
      count: 500
    input_registers:
      start: 0
      count: 500
```

---

## State Sealing Configuration

State Sealing prevents Modbus clients from accessing memory until it is explicitly unsealed. This is useful for startup safety and initialization sequences.

For complete details, see [01_STATE_SEALING.md](01_STATE_SEALING.md).

### Basic State Sealing

```yaml
memory:
  - unit_id: 1
    coils:
      start: 0
      count: 100
    holding_registers:
      start: 0
      count: 500
    
    state_sealing:
      area: coil        # REQUIRED: Must be "coil" (singular) - this is the ONLY supported value
      address: 0        # Coil address that controls sealing
```

### How It Works

1. **On Startup**: Memory is **sealed** (flag bit = 0)
2. **Sealed Behavior**: All Modbus requests return exception `0x06 (Device Busy)`
3. **Unsealing**: Write `1` to the sealing flag bit (coil 0 in example above)
4. **Unsealed Behavior**: Normal Modbus access is allowed
5. **Raw Ingest**: Always allowed regardless of sealing state

### Unsealing Methods

**Method 1: Raw Ingest (Recommended)**

Use MMA's Raw Ingest protocol to write to the sealing flag:

```bash
# Example using a Raw Ingest client
raw_ingest_send --port 502 --unit 1 --area coils --addr 0 --value 1
```

**Method 2: External Control**

Another system writes to the sealing flag through its own logic.

### State Sealing Example with Policy

```yaml
memory:
  - unit_id: 1
    coils:
      start: 0
      count: 16
    holding_registers:
      start: 0
      count: 100
    
    state_sealing:
      area: coil
      address: 0
    
    policy:
      rules:
        - id: "controller_only"
          source_ip:
            - "192.168.1.50"
          allow_fc: [1, 3, 5, 6, 15, 16]
```

**Evaluation Order:**
1. State sealing check (if sealed, return 0x06 immediately)
2. Policy evaluation (only if unsealed)
3. Memory operation

### Important Notes

- **CRITICAL**: `state_sealing.area` must be set to `"coil"` (singular). This is the ONLY supported value. Any other value will cause startup failure.
- Sealing flag address must be within the configured coils area
- Restarting MMA resets memory to sealed state
- Once unsealed, cannot re-seal without restart
- Raw Ingest bypasses sealing (by design, for unsealing)

---

## Access Control (Policy)

Policies control which IP addresses can perform which Modbus operations on a memory instance.

For complete details, see [03_AUTHORITY_MODEL.md](03_AUTHORITY_MODEL.md).

### Basic Policy Structure

```yaml
policy:
  rules:
    - id: "rule_name"              # REQUIRED: unique rule identifier
      source_ip:                   # REQUIRED: list of IPs or CIDRs
        - "192.168.1.0/24"
        - "10.0.0.50"
      allow_fc:                    # REQUIRED: allowed function codes
        - 1                        # Read Coils
        - 3                        # Read Holding Registers
        - 5                        # Write Single Coil
        - 6                        # Write Single Register
```

### Function Codes Reference

| Code | Name | Type |
|------|------|------|
| 1 | Read Coils | Read |
| 2 | Read Discrete Inputs | Read |
| 3 | Read Holding Registers | Read |
| 4 | Read Input Registers | Read |
| 5 | Write Single Coil | Write |
| 6 | Write Single Register | Write |
| 15 | Write Multiple Coils | Write |
| 16 | Write Multiple Registers | Write |

### Common Policy Patterns

**Note:** Policy rules use `source_ip` and `allow_fc` fields only. Shorthand aliases like "allow: ro/rw/wo" are NOT supported.

**Pattern 1: Allow All**

```yaml
policy:
  rules:
    - id: "allow_all"
      source_ip:
        - "0.0.0.0/0"              # All IPv4
        - "::/0"                   # All IPv6
      allow_fc: [1, 2, 3, 4, 5, 6, 15, 16]
```

**Pattern 2: Read-Only for Network, Read-Write for Localhost**

```yaml
policy:
  rules:
    - id: "localhost_full"
      source_ip:
        - "127.0.0.1"
        - "::1"
      allow_fc: [1, 2, 3, 4, 5, 6, 15, 16]
    
    - id: "network_readonly"
      source_ip:
        - "192.168.0.0/16"
      allow_fc: [1, 2, 3, 4]       # Read only
```

**Pattern 3: Write-Only (Setpoint Injection)**

```yaml
policy:
  rules:
    - id: "controller_writes"
      source_ip:
        - "192.168.1.100"
      allow_fc: [5, 6, 15, 16]     # Write only
```

**Pattern 4: Layered Access**

```yaml
policy:
  rules:
    - id: "admin_full"
      source_ip:
        - "192.168.1.10"
      allow_fc: [1, 2, 3, 4, 5, 6, 15, 16]
    
    - id: "operators_read"
      source_ip:
        - "192.168.1.0/24"
      allow_fc: [1, 3]
    
    - id: "deny_default"
      source_ip:
        - "0.0.0.0/0"
      allow_fc: []                 # Deny all
```

### Policy Evaluation Rules

1. **Top-Down**: Rules are evaluated in order
2. **First Match Wins**: First matching rule determines access
3. **Default Deny**: If no rules match, access is denied
4. **No Policy = Allow**: If no policy section exists, all access is allowed

### IP Address Formats

```yaml
# Single IPv4
source_ip:
  - "192.168.1.50"

# IPv4 CIDR
source_ip:
  - "192.168.0.0/16"
  - "10.0.0.0/8"

# Single IPv6
source_ip:
  - "::1"
  - "2001:db8::1"

# IPv6 CIDR
source_ip:
  - "2001:db8::/32"

# Mixed
source_ip:
  - "127.0.0.1"
  - "::1"
  - "192.168.1.0/24"
```

---

## Notification Configuration

### Per-Memory Notification Rules

Notifications trigger when writes occur to specified memory ranges.

```yaml
notify:
  coils:
    - start: 0
      count: 10
      name: "alarm_bits"           # OPTIONAL: human-readable name
    
    - start: 20
      count: 5
  
  holding_registers:
    - start: 0
      count: 100
      name: "setpoint_values"
  
  input_registers:
    - start: 0
      count: 50
```

### Global Notification Output

Configure where notifications are sent:

```yaml
# At root level (not under listeners)
notify:
  influx:
    url: "http://localhost:8086"
    token: "your_influx_token"
    org: "your_org"
    bucket: "mma_events"
    measurement: "modbus_writes"    # OPTIONAL: defaults to "mma_notify"
```

**Behavior:**
- If `notify.influx` is configured: InfluxDB adapter is used
- If `notify.influx` is missing: stdout adapter is used (events logged to console, though currently muted in code)
- Influx configuration is NOT validated at startup

### Complete Example with Notifications

```yaml
listeners:
  - id: "main"
    listen: ":502"
    memory:
      - unit_id: 1
        holding_registers:
          start: 0
          count: 1000
        
        notify:
          holding_registers:
            - start: 0
              count: 100
              name: "critical_setpoints"
            
            - start: 500
              count: 50
              name: "control_parameters"

# Global output configuration
notify:
  influx:
    url: "http://influxdb.local:8086"
    token: "mytoken"
    org: "plant_monitoring"
    bucket: "modbus_events"
```

### Notification Semantics

- **Independent Rules**: Each rule is evaluated independently
- **Overlaps Allowed**: One write can match multiple rules
- **No Deduplication**: Multiple events may be generated for overlapping rules
- **Write-Only**: Notifications only trigger on write operations (FC 5, 6, 15, 16)

---

## Debug Logging

### Overview

MMA 2.0 suppresses protocol-level read errors by default. In production environments, events such as port-scanner probes or clients that send malformed Modbus frames produce an `invalid PDU length` condition. Logging these on every occurrence creates noise without operational value.

The `debug` flag controls whether these errors are emitted to the log.

### Configuration

```yaml
# Top-level option — place before "listeners"
debug: false   # default; omit entirely for the same result
```

| Value | Behavior |
|-------|----------|
| `false` (default) | Protocol-level read errors are silently discarded |
| `true` | Protocol-level read errors are written to the log |

### When to Enable

Enable `debug: true` when:

- Integrating a new Modbus client and its framing needs verification
- Diagnosing unexpected connection drops
- Troubleshooting an `invalid PDU length` message observed during a previous run

Disable (or omit) in production to keep terminal output clean.

### What Is Logged

When `debug: true`, errors such as the following appear in the log:

```
modbus read error: invalid PDU length
modbus read error: invalid MBAP length
```

These indicate a frame was received whose MBAP header specifies a PDU length of zero or less. This is valid protocol-level rejection behavior — the connection is closed and all memory and authority state remains unaffected.

### Example

```yaml
debug: true

listeners:
  - id: "main"
    listen: "0.0.0.0:502"
    memory:
      - unit_id: 1
        holding_registers:
          start: 0
          count: 100
```

---

## Access Events Configuration

### Overview

The Access Events system is a **passive observer** of access control decisions made by MMA 2.0. It emits a live stream of access events — one per Modbus request that passes through the access control layer — as NDJSON over HTTP.

Key properties:

- **Observer-only**: It observes decisions; it does not influence them. No Modbus response is altered, delayed, or rejected based on event state.
- **Non-blocking**: Event emission never delays the Modbus response path. When the system is at capacity, events are dropped silently.
- **No persistence**: Events are not stored. There is no replay mechanism or audit history. If no consumer is connected, events are discarded.
- **Best-effort delivery**: Slow consumers, full channel buffers, and key map overflow all result in silent drops. The Modbus operation continues normally regardless.

The system is entirely separate from the Notification Engine. These two systems share no code, no state, and no configuration path.

---

### YAML Structure

Access events are configured under the top-level `access_events` key:

```yaml
access_events:
  enabled: true
  mode: rate
  window: 5
  key_fields:
    - src_ip
    - function_code
    - action
    - status
    - port
    - unit
  include_counter: true
  limits:
    max_keys: 1000
    ttl: 30
  output:
    type: http_stream
    path: /events
    listen: ":9090"
```

---

### Field Reference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `enabled` | boolean | yes | Enables or disables the entire access event system. When `false`, no listener is started, no map is allocated, and all other fields are ignored. |
| `mode` | string | yes | Aggregation mode. Only `"rate"` is supported. Any other value causes startup failure. |
| `window` | integer | yes | Aggregation window duration in seconds. Must be `> 0`. |
| `key_fields` | list | yes | Defines the aggregation key. Must contain exactly the six fields listed below — no more, no fewer, no duplicates. |
| `include_counter` | boolean | yes | When `true`, summary events include `count` and `window_sec` fields. When `false`, these fields are omitted from all events. |
| `limits.max_keys` | integer | yes | Maximum number of concurrent aggregation keys. Must be `> 0`. When this limit is reached, new keys are dropped. |
| `limits.ttl` | integer | yes | Maximum age (in seconds) of an inactive aggregation key before cleanup removes it. Must be `>= 2 × window`. |
| `output.type` | string | yes | Output transport. Only `"http_stream"` is supported. Any other value causes startup failure. |
| `output.path` | string | yes | HTTP path for the streaming endpoint. Must begin with `"/"`. |
| `output.listen` | string | yes | TCP bind address for the HTTP server (e.g. `":9090"`). Required when `output.type` is `"http_stream"`. |

#### `key_fields` — Required Values

The `key_fields` list must contain **exactly** these six values (order does not matter):

```yaml
key_fields:
  - src_ip
  - function_code
  - action
  - status
  - port
  - unit
```

Any missing, extra, or duplicate field causes startup failure.

---

### Validation Rules

All rules apply only when `enabled: true`. When `enabled: false`, no validation is performed on other fields.

- `mode` must be `"rate"`. Any other value → startup failure.
- `window` must be `> 0`. Zero or negative → startup failure.
- `key_fields` must contain exactly the six defined fields with no extras and no duplicates → startup failure if violated.
- `limits.max_keys` must be `> 0`. Zero or negative → startup failure.
- `limits.ttl` must be `>= 2 × window`. Violation → startup failure.
- `output.type` must be `"http_stream"`. Any other value → startup failure.
- `output.path` must begin with `"/"`. Any other value → startup failure.
- `output.listen` must not be empty. Empty value → startup failure.

---

### Event Format

Events are emitted as **NDJSON** (Newline-Delimited JSON): one JSON object per line, UTF-8 encoded, no outer array wrapper. Each line is self-contained.

**Individual event** (first occurrence of a key in a window):

```json
{"ts":1700000000000000000,"port":502,"unit":1,"function_code":3,"action":"read","status":"allowed","src_ip":"192.168.1.10"}
```

**Summary event** (emitted when a window expires and `include_counter: true`):

```json
{"ts":1700000005000000000,"port":502,"unit":1,"function_code":3,"action":"read","status":"allowed","src_ip":"192.168.1.10","count":47,"window_sec":5}
```

#### Event Fields

| Field | Type | Always Present | Description |
|-------|------|----------------|-------------|
| `ts` | integer | yes | Unix timestamp in nanoseconds (host OS wall clock). |
| `port` | integer | yes | Listener port on which the request arrived. |
| `unit` | integer | yes | Modbus Unit ID targeted by the request. |
| `function_code` | integer | yes | Modbus function code (e.g. 3, 16). |
| `action` | string | yes | `"read"` (FC 1, 2, 3, 4) or `"write"` (FC 5, 6, 15, 16). Unknown function codes are silently ignored. |
| `status` | string | yes | `"allowed"` or `"denied"` — the access control decision. |
| `src_ip` | string | yes | Source IP address of the client (no port). |
| `count` | integer | no | Number of events suppressed within the closed window. Absent on individual events. Only present when `include_counter: true`. |
| `window_sec` | integer | no | Duration in seconds of the window that produced this summary. Only present when `include_counter: true`. |

---

### Behavioral Rules

#### Aggregation Key

Each unique combination of the following six fields defines one independent aggregation bucket:

```
key = (src_ip, function_code, action, status, port, unit)
```

Events sharing the same key within the same window are aggregated. Events with different keys are independent.

#### Rate Aggregation Algorithm

**Step 1 — First event (no active window for this key):**

1. If the key map is at `max_keys`, drop the event silently. Do not create a new entry.
2. Otherwise, emit the event immediately (no `count` or `window_sec` fields).
3. Open a new window for this key, recording `window_start` = current timestamp and `suppressed_count = 0`.

**Step 2 — Subsequent event within an active window (`now < window_start + window`):**

1. Do NOT emit an event.
2. Increment `suppressed_count` by 1.

**Step 3 — Event arrives after window expiry (`now >= window_start + window`):**

1. If `suppressed_count > 0`: emit a summary event **first** (with `count` and `window_sec` when `include_counter: true`).
2. Emit the current (triggering) event **second**, with no `count` or `window_sec`.
3. Open a new window for this key.

Window expiry is evaluated lazily — only when a new event arrives for that key. There are no background timers that trigger event emission.

#### Emission Order (Step 3)

When a window expires on a new event and there were suppressed events:

1. Summary event (for the closed window) — emitted first
2. Current event (the event that triggered expiry) — emitted second

#### Suppression Logic

Events within an active window are silently discarded. Only the first event in each window is emitted immediately. The count of suppressed events is carried in the summary emitted at window expiry (if `include_counter: true`).

`count` represents only the number of suppressed events within the closed window. It does **not** include the event that triggered window expiry detection.

#### TTL Cleanup

A background cleanup goroutine runs at an interval of `window × 2` seconds. It removes aggregation keys whose `window_start` age exceeds `ttl`. Cleanup is a memory hygiene mechanism only — it never emits events. Suppressed counts for expired keys are discarded silently.

The cleanup goroutine recovers from panics and restarts itself automatically.

---

### Critical Constraints

```
- limits.ttl must be >= 2 × window (enforced at startup)
- limits.max_keys overflow: new keys are dropped, existing keys are unaffected
- event emission is non-blocking: broadcast channel capacity is 1024 events; per-client channel capacity is 64 events; full channels result in silent drops
- system is best-effort: drops are allowed and expected under load
- cleanup does NOT emit events; suppressed counts for expired keys are discarded silently
- unknown function codes (not in FC 1,2,3,4,5,6,15,16) are silently ignored; no event is emitted
- output.listen bind failure causes immediate startup failure (not a silent background crash)
```

---

### HTTP Streaming Transport

The access events HTTP server is started on `output.listen` at `output.path`. It accepts `GET` requests only.

- Response begins immediately with `HTTP 200 OK`.
- `Content-Type: application/x-ndjson`
- Connection is held open indefinitely; events are written as they are produced.
- Multiple simultaneous clients are supported; each receives events independently.
- Slow clients have events dropped rather than back-pressuring the engine.
- Normal client disconnect is silent (no error logged).

---

### Example Use Case

**Scenario**: Monitor all access control decisions on a Modbus server, suppressing high-frequency repeated accesses to one event per 5-second window per unique source/target combination.

```yaml
listeners:
  - id: "main"
    listen: "0.0.0.0:502"
    memory:
      - unit_id: 1
        holding_registers:
          start: 0
          count: 1000
        policy:
          rules:
            - id: "allow_controllers"
              source_ip:
                - "192.168.1.0/24"
              allow_fc: [1, 3, 5, 6]
            - id: "deny_default"
              source_ip:
                - "0.0.0.0/0"
              allow_fc: []

access_events:
  enabled: true
  mode: rate
  window: 5
  key_fields:
    - src_ip
    - function_code
    - action
    - status
    - port
    - unit
  include_counter: true
  limits:
    max_keys: 1000
    ttl: 30
  output:
    type: http_stream
    path: /events
    listen: ":9090"
```

Connect a consumer:

```bash
curl http://localhost:9090/events
```

---

### Validation Checklist (Access Events)

Before starting MMA with access events enabled:

- [ ] `mode` is `"rate"`
- [ ] `window` is a positive integer
- [ ] `key_fields` contains exactly the six required fields, no duplicates
- [ ] `limits.max_keys` is a positive integer
- [ ] `limits.ttl` >= `2 × window`
- [ ] `output.type` is `"http_stream"`
- [ ] `output.path` begins with `"/"`
- [ ] `output.listen` is not empty

### Common Errors (Access Events)

**Error: `access_events.mode must be "rate"`**

Only `mode: rate` is supported.

**Error: `access_events.window must be > 0`**

`window: 0` or a negative value is not allowed.

**Error: `access_events.key_fields must contain exactly 6 fields`**

The `key_fields` list must contain exactly: `src_ip`, `function_code`, `action`, `status`, `port`, `unit`.

**Error: `access_events.limits.ttl must be at least 2x window`**

Increase `ttl` so that it is at least twice the value of `window`.

**Error: `access events: failed to bind :9090`**

The bind address is already in use or the process lacks permission to bind the port.

---

## Common Use Cases

### Use Case 1: Simple Modbus Slave Device

**Scenario**: Single device, no access control, immediate access

```yaml
listeners:
  - id: "device"
    listen: "0.0.0.0:502"
    memory:
      - unit_id: 1
        holding_registers:
          start: 0
          count: 1000
        input_registers:
          start: 0
          count: 500
```

### Use Case 2: Multi-Device Gateway

**Scenario**: Multiple devices behind one gateway, each on different Unit ID

```yaml
listeners:
  - id: "gateway"
    listen: "0.0.0.0:502"
    memory:
      - unit_id: 1
        holding_registers:
          start: 0
          count: 100
      
      - unit_id: 2
        holding_registers:
          start: 0
          count: 200
      
      - unit_id: 3
        holding_registers:
          start: 0
          count: 150
      
      - unit_id: 4
        holding_registers:
          start: 0
          count: 300
```

### Use Case 3: Secure Startup with State Sealing

**Scenario**: Prevent access until initialization complete

```yaml
listeners:
  - id: "secure_device"
    listen: "0.0.0.0:502"
    memory:
      - unit_id: 1
        coils:
          start: 0
          count: 16
        holding_registers:
          start: 0
          count: 500
        
        state_sealing:
          area: coil
          address: 0
        
        policy:
          rules:
            - id: "allow_after_unseal"
              source_ip:
                - "0.0.0.0/0"
              allow_fc: [1, 3, 5, 6, 15, 16]
```

**Initialization Sequence:**
1. MMA starts (memory sealed)
2. Initialization system writes data via Raw Ingest
3. Initialization system writes `1` to coil 0 (unseals memory)
4. Modbus clients can now access memory

### Use Case 4: Read-Only Public, Read-Write Controller

**Scenario**: HMIs can read, only control system can write

```yaml
listeners:
  - id: "scada"
    listen: "0.0.0.0:502"
    memory:
      - unit_id: 1
        holding_registers:
          start: 0
          count: 1000
        
        policy:
          rules:
            - id: "controller"
              source_ip:
                - "192.168.1.100"
              allow_fc: [1, 3, 5, 6, 15, 16]
            
            - id: "hmi_readonly"
              source_ip:
                - "192.168.1.0/24"
              allow_fc: [1, 3]
            
            - id: "deny_rest"
              source_ip:
                - "0.0.0.0/0"
              allow_fc: []
```

### Use Case 5: Multi-Port Segregation

**Scenario**: Different networks on different ports with different access levels

```yaml
listeners:
  - id: "production_network"
    listen: "192.168.1.10:502"
    memory:
      - unit_id: 1
        holding_registers:
          start: 0
          count: 1000
        
        policy:
          rules:
            - id: "production_readonly"
              source_ip:
                - "192.168.1.0/24"
              allow_fc: [1, 3]
  
  - id: "control_network"
    listen: "10.0.0.10:502"
    memory:
      - unit_id: 1
        holding_registers:
          start: 0
          count: 1000
        
        policy:
          rules:
            - id: "control_full"
              source_ip:
                - "10.0.0.0/24"
              allow_fc: [1, 3, 5, 6, 15, 16]
```

### Use Case 6: Event Monitoring with Notifications

**Scenario**: Track all writes to critical registers

```yaml
listeners:
  - id: "monitored"
    listen: "0.0.0.0:502"
    memory:
      - unit_id: 1
        holding_registers:
          start: 0
          count: 1000
        
        notify:
          holding_registers:
            - start: 0
              count: 10
              name: "safety_setpoints"
            
            - start: 100
              count: 20
              name: "power_limits"

notify:
  influx:
    url: "http://influxdb:8086"
    token: "monitoring_token"
    org: "plant"
    bucket: "modbus_audit"
```

---

## Validation and Troubleshooting

### Validation Checklist

Before starting MMA:

- [ ] YAML syntax is valid
- [ ] Each listener has unique `id`
- [ ] Listen addresses are valid (format: `address:port`)
- [ ] Each memory has unique `(port, unit_id)` pair
- [ ] At least one memory area defined per memory instance
- [ ] Memory area `count > 0`
- [ ] `start + count` doesn't exceed 65536
- [ ] State sealing address is within configured area bounds
- [ ] Policy IP addresses are valid IPs or CIDRs
- [ ] Policy function codes are in range 1-16
- [ ] Notification ranges are within configured area bounds

### Common Errors

**Error: "memory identity conflict"**

```
memory identity conflict: (port=502 unit=1) defined in 
listeners[0](main).memory[0] and listeners[1](backup).memory[0]
```

**Cause**: Same `(port, unit_id)` defined multiple times

**Fix**: Ensure each memory has unique combination of port and unit_id

---

**Error: "state_sealing.address out of bounds"**

```
listeners[0](main).memory[0]: state_sealing.address (100) out of 
bounds for coils [0..16)
```

**Cause**: Sealing flag address not within coils area

**Fix**: Adjust address or increase coils count

```yaml
# Before (broken)
coils:
  start: 0
  count: 16
state_sealing:
  area: coil
  address: 100

# After (fixed)
coils:
  start: 0
  count: 128
state_sealing:
  area: coil
  address: 0
```

---

**Error: "start+count exceeds 16-bit address space"**

```
memory[device1].holding_registers: start(40000)+count(30000) 
exceeds 16-bit address space
```

**Cause**: Memory area too large

**Fix**: Reduce count

```yaml
# Before (broken)
holding_registers:
  start: 40000
  count: 30000   # 40000 + 30000 = 70000 > 65535

# After (fixed)
holding_registers:
  start: 40000
  count: 25535   # 40000 + 25535 = 65535
```

---

**Error: "invalid ip/cidr"**

```
memory[device1].policy.rules[0].source_ip[0]: invalid ip/cidr 
"192.168.1.256": address out of range
```

**Cause**: Invalid IP address

**Fix**: Use valid IP

```yaml
# Before (broken)
source_ip:
  - "192.168.1.256"

# After (fixed)
source_ip:
  - "192.168.1.100"
```

---

### Testing Your Configuration

**Step 1: Validate Syntax**

```bash
# Check YAML syntax
yamllint config.yaml
```

**Step 2: Test Startup**

```bash
# Start MMA with your config
./mma2 --config config.yaml
```

If configuration is valid, you'll see:
```
[INFO] Configuration loaded successfully
[INFO] Memory store initialized: 3 instances
[INFO] Listener started: main (0.0.0.0:502)
[INFO] MMA 2.0 ready
```

If invalid, you'll see specific error messages and the process will exit.

**Step 3: Test Modbus Access**

```bash
# Using modpoll or similar tool
modpoll -m tcp -a 1 -r 0 -c 10 -t 4 localhost

# Expected for sealed memory:
# Exception 0x06 (Device Busy)

# Expected for accessible memory:
# Successfully read registers
```

---

## Complete Working Examples

### Example: Production-Ready Configuration

```yaml
listeners:
  # Primary Modbus TCP interface
  - id: "production"
    listen: "0.0.0.0:502"
    
    memory:
      # Main control device with state sealing
      - unit_id: 1
        coils:
          start: 0
          count: 128
        holding_registers:
          start: 0
          count: 2000
        input_registers:
          start: 0
          count: 1000
        
        state_sealing:
          area: coil
          address: 0
        
        policy:
          rules:
            - id: "controller_rw"
              source_ip:
                - "192.168.1.100"
                - "192.168.1.101"
              allow_fc: [1, 3, 5, 6, 15, 16]
            
            - id: "scada_readonly"
              source_ip:
                - "192.168.1.0/24"
              allow_fc: [1, 3, 4]
            
            - id: "deny_default"
              source_ip:
                - "0.0.0.0/0"
              allow_fc: []
        
        notify:
          holding_registers:
            - start: 0
              count: 100
              name: "critical_setpoints"
      
      # Secondary monitoring device
      - unit_id: 2
        input_registers:
          start: 0
          count: 500
        
        policy:
          rules:
            - id: "allow_monitoring"
              source_ip:
                - "192.168.1.0/24"
              allow_fc: [4]

  # Maintenance/debug port (localhost only)
  - id: "debug"
    listen: "127.0.0.1:5020"
    
    memory:
      - unit_id: 1
        holding_registers:
          start: 0
          count: 100
        
        policy:
          rules:
            - id: "localhost_full"
              source_ip:
                - "127.0.0.1"
                - "::1"
              allow_fc: [1, 2, 3, 4, 5, 6, 15, 16]

# Global notification output
notify:
  influx:
    url: "http://influxdb.local:8086"
    token: "production_token"
    org: "facility"
    bucket: "modbus_events"
    measurement: "mma_writes"
```

---

## Additional Resources

- **Configuration Contract**: [04_CONFIGURATION.md](04_CONFIGURATION.md)
- **State Sealing Details**: [01_STATE_SEALING.md](01_STATE_SEALING.md)
- **Authority Model**: [03_AUTHORITY_MODEL.md](03_AUTHORITY_MODEL.md)
- **Architecture Overview**: [02_ARCHITECTURE.md](02_ARCHITECTURE.md)
- **Access Events Design**: [access_events.md](access_events.md)
- **Example Configuration**: [example.yaml](example.yaml)

---

**End of Configuration Manual**
