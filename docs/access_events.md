# MMA 2.0 — Access Event System

**STATUS: DESIGN DOCUMENT**

Scope: Access decision observation only\
Non-goal: Audit history, guaranteed delivery, semantic interpretation

---

## 1. Purpose

### What Access Events Are

Access Events are **read-only observations** of access control decisions made by the access control layer in MMA 2.0.

An access event is emitted each time the access control layer produces a decision:

- A read request was **allowed**
- A read request was **denied**
- A write request was **allowed**
- A write request was **denied**

Access events carry the decision outcome and the context under which it was made (source IP, port, unit ID, function code). They do not carry memory values.

### What Problem They Solve

The access control layer enforces policy silently. Without an event mechanism, there is no observable record that decisions are being made. This makes it impossible to:

- detect repeated denial patterns in flight
- observe which clients are accessing which units
- correlate access attempts across listeners

Access Events provide a **non-persistent, live signal** of policy enforcement activity.

### What They Are NOT (Explicit Non-Goals)

Access Events are NOT:

- An audit log. Events are not persisted. There is no history.
- A security enforcement mechanism. They do not influence decisions.
- A change detection system. Memory values are never included.
- A guaranteed delivery channel. Events may be dropped.
- A replacement for or extension of the Notification Engine.
- A semantic interpreter. Function codes are recorded, not interpreted.
- An alerting system. Consumers must implement alerting externally.

---

## 2. Architectural Placement

### Pipeline Position

Access Events sit between the Access Control layer and the Memory layer:

```
TCP → Firewall → Protocol → Access Control → Access Events → Memory
```

More precisely, the Access Event observer sits immediately **after** the access control decision is produced and **before** any memory operation is attempted or blocked.

### Behavioral Contract

- It **observes** decisions produced by the access control layer.
- It does **not influence** decisions. The decision is final before the observer runs.
- It does **not modify memory**. It has no write path to any memory area.
- It does **not block** execution. The Modbus response path must not wait on event emission.
- It is **not aware** of memory values. It receives only the decision metadata.

The Access Event system is a passive side-channel. Its presence or absence must have no effect on any Modbus response.

### Separation from Notification Engine

The Notification Engine fires on write commits to configured register ranges.
The Access Event System fires on access control decisions, regardless of whether a memory operation follows.

These are separate systems with different trigger conditions, different event models, and no shared code path.

---

## 3. Event Model

### Required Fields

| Field           | Type    | Description                                                     |
|-----------------|---------|-----------------------------------------------------------------|
| `ts`            | integer | Unix timestamp in nanoseconds (host OS wall clock)              |
| `port`          | integer | Listener port on which the request arrived                      |
| `unit`          | integer | Modbus Unit ID targeted by the request                          |
| `function_code` | integer | Modbus function code of the request (e.g. 3, 16)               |
| `action`        | string  | `"read"` or `"write"` — derived from function code semantics    |
| `status`        | string  | `"allowed"` or `"denied"` — the access control decision         |
| `src_ip`        | string  | Source IP address of the client (no port)                       |

### Optional Fields (Aggregation Only)

| Field        | Type    | Description                                                                                     |
|--------------|---------|-----------------------------------------------------------------------------------------------|
| `count`      | integer | Number of suppressed events within the window. Does NOT include the current (triggering) event. |
| `window_sec` | integer | Duration in seconds of the aggregation window that produced this summary.                       |

`count` and `window_sec` are only present on summary events emitted at the end of an aggregation window. A count of `1` is never emitted; the first event in a window is always emitted individually without these fields.

`count` represents **only** the number of events suppressed within the window. It does **not** include the current event that triggered window expiry detection.

### Enum Values

`action`:
- `"read"` — function codes: FC1, FC2, FC3, FC4
- `"write"` — function codes: FC5, FC6, FC15, FC16

`status`:
- `"allowed"` — access control permitted the request
- `"denied"` — access control rejected the request

### Example: Individual Event

```json
{"ts":1700000000000000000,"port":502,"unit":1,"function_code":3,"action":"read","status":"allowed","src_ip":"192.168.1.10"}
```

### Example: Summary Event (Aggregated)

```json
{"ts":1700000005000000000,"port":502,"unit":1,"function_code":3,"action":"read","status":"allowed","src_ip":"192.168.1.10","count":47,"window_sec":5}
```

---

## 4. Rate Aggregation Behavior

### Purpose

Without aggregation, a high-frequency client polling at 100 Hz produces 100 events per second per unit per function code. This creates unbounded output volume. Rate aggregation suppresses repetition while preserving signal.

### Aggregation Key

Each unique combination of the following fields defines one aggregation key:

```
key = (src_ip, function_code, action, status, port, unit)
```

Events sharing the same key within the same time window are aggregated together. Events with different keys are independent.

### Algorithm

**Step 1 — First event in a window:**

When an event arrives for a key that has no active window:

1. Emit the event immediately with no `count` or `window_sec` fields.
2. Start a new window for this key. Record:
   - `window_start`: timestamp of first event
   - `suppressed_count`: 0

**Step 2 — Subsequent events within the window:**

When an event arrives for a key that has an active window and `now < window_start + window_sec`:

1. Do NOT emit an event.
2. Increment `suppressed_count` by 1.

**Step 3 — Window expiry:**

When `now >= window_start + window_sec` for an active key and a new event arrives:

1. If `suppressed_count > 0`:
   1. Emit a summary event **FIRST** with:
      - `ts`: current time (emission time, not window start)
      - all key fields (`src_ip`, `function_code`, `action`, `status`, `port`, `unit`)
      - `count`: value of `suppressed_count`
      - `window_sec`: configured window duration
   2. Emit the current (triggering) event **SECOND**, immediately after the summary, with no `count` or `window_sec` fields.
   3. Clear the window state and open a new window for this key.
2. If `suppressed_count == 0`:
   1. Clear the window state for this key.
   2. Emit the current event immediately. Open a new window.

Window expiry is evaluated **only during event processing**. There are no background timers that trigger event emission.

**Step 4 — New event after window expiry:**

If a new event arrives for a key whose window has expired, treat it as Step 3 above (summary emitted first if applicable, then the current event, new window opened).

### Timing Behavior

- Window duration is configured globally as `window` (in seconds).
- Window start is the timestamp of the first event in the window.
- Window expiry is checked lazily: **only on the next event for the same key**. There are no background timers that trigger event emission.
- Expiry occurs on access, not on a schedule.
- Summary events are emitted at the moment of expiry detection, not at the scheduled window boundary.

**Window expiry is evaluated only during event processing. No background goroutine triggers emission.**

### Reset Behavior

- A window resets when its expiry is reached and state is cleared.
- There is no early reset. An active window cannot be cancelled.
- If no subsequent events arrive for a key within the window, the suppressed count is lost. Summary events are only emitted when a new event triggers expiry detection.

### Decision Summary

| Condition                                     | Action                                                              |
|-----------------------------------------------|---------------------------------------------------------------------|
| New key, no active window                     | Emit immediately, open window                                       |
| Same key, window active, not yet expired      | Suppress, increment counter                                         |
| Same key, window expired, counter > 0         | Emit summary FIRST, emit current event SECOND, clear window, open new |
| Same key, window expired, counter == 0        | Clear window, emit current event immediately, open new              |

---

## 5. State and Limits

### Key Storage

Aggregation state is stored in a single in-memory map:

```
key → { window_start: int64, suppressed_count: int64 }
```

The key is the string representation of `(src_ip, function_code, action, status, port, unit)`.

This map exists only in process memory. It is not persisted, replicated, or restored on restart.

### TTL Cleanup Behavior

A background cleanup pass runs at configurable intervals (recommended: `window * 2`).

Cleanup is **strictly a memory hygiene mechanism**.

Responsibilities:
- Remove expired keys from the map.
- Free memory.

Cleanup MUST NOT:
- Emit events.
- Flush counters.
- Trigger any external output.

All event emission is strictly event-driven and occurs only during request processing.

During cleanup:

1. Iterate all keys in the map.
2. For each key where `now >= window_start + ttl`:
   - Delete the key silently, regardless of `suppressed_count`.

Cleanup intervals are best-effort. Cleanup does not guarantee precise timing. It guarantees that stale state is eventually removed.

### `max_keys` Safety Limit

The map is bounded by `max_keys` (configured globally).

When a new key would be inserted and the current map size equals `max_keys`:

> **The new event is dropped. No aggregation entry is created.**

This is the overflow policy. It is intentional. The system does not evict existing keys to make room. The operator must size `max_keys` appropriately for their environment.

The `max_keys` value must be set explicitly in configuration. There is no implicit default.

### `ttl`

`ttl` defines the maximum age (in seconds) of an inactive key before cleanup removes it, regardless of whether the window has expired.

`ttl` MUST be ≥ 2 × `window`. If `ttl` < 2 × `window`, startup must fail with an explicit validation error.

This ensures that keys are not prematurely removed before a new event can trigger window rollover, which would otherwise cause duplicate "first events" and break aggregation correctness.

### Overflow Behavior

> **Events may be dropped under pressure. This is by design.**

When `max_keys` is reached:
- The event is not emitted.
- No error is returned to the caller.
- The Modbus operation proceeds normally.
- No counter or alert is generated by the system itself.

The operator is responsible for setting `max_keys` appropriately. The system does not self-tune.

---

## 6. Output Model

### Stream Model

Access Events are emitted as a **live stream only**.

There is:
- No persistence layer
- No event log
- No replay mechanism
- No delivery guarantee
- No buffering guarantee

If a consumer is not connected, events are discarded. If a consumer is slow, events may be discarded. Consumers must tolerate gaps.

### Format

Events are emitted as **NDJSON** (Newline-Delimited JSON):

- One JSON object per line
- Lines terminated by `\n`
- UTF-8 encoding
- No outer array wrapper
- Compatible with SSE body format if `data:` prefix is added by a wrapper

Each line is a complete, self-contained event object. Consumers must not assume ordering or completeness.

### Example Stream

```
{"ts":1700000000000000000,"port":502,"unit":1,"function_code":3,"action":"read","status":"allowed","src_ip":"192.168.1.10"}
{"ts":1700000000100000000,"port":502,"unit":2,"function_code":16,"action":"write","status":"denied","src_ip":"10.0.0.5"}
{"ts":1700000005000000000,"port":502,"unit":1,"function_code":3,"action":"read","status":"allowed","src_ip":"192.168.1.10","count":47,"window_sec":5}
```

---

## 7. Transport Layer

### Endpoint

```
GET /events
```

### Method

`GET` only. No other HTTP methods are supported on this endpoint.

### Behavior

The endpoint opens a persistent HTTP response and streams events to the connected client as they are emitted.

- The response begins immediately with HTTP `200 OK`.
- The `Content-Type` header is `application/x-ndjson`.
- The connection is held open indefinitely.
- Events are written to the response body as they are produced.
- No keep-alive pings are sent. Idle periods produce no output.

### Client Disconnect Handling

- When the client disconnects, the server detects the closed connection and stops writing.
- No error is logged for normal client disconnects.
- The Modbus path is not affected by client disconnect or connect events.

### No Buffering Guarantees

- There is no internal write buffer that accumulates events for slow consumers.
- If the client's TCP receive window is full, the write attempt may block or be discarded, depending on implementation.
- The implementation MUST NOT allow a slow consumer to apply back-pressure to the Modbus execution path.
- The implementation MUST NOT block event emission waiting for a connected consumer.

### Single Listener

The `/events` endpoint is a single global stream. It is not scoped per port or per unit. All events from all listeners are multiplexed on the same stream.

If multiple clients connect, each receives events independently. The implementation may fan-out to multiple clients or support only one client; this is a future implementation decision and must be defined before implementation begins.

---

## 8. Configuration (YAML)

### Structure

Access event configuration lives under a top-level `access_events` key:

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
    max_keys: 10000
    ttl: 60
  output:
    type: http_stream
    path: /events
    listen: ":8080"
```

| Field                   | Type    | Required | Description                                                                   |
|-------------------------|---------|----------|-------------------------------------------------------------------------------|
| `enabled`               | boolean | yes      | Enables or disables the access event system entirely                          |
| `mode`                  | string  | yes      | Aggregation mode. Only `rate` is supported.                                   |
| `window`                | integer | yes      | Aggregation window duration in seconds. Must be > 0.                         |
| `key_fields`            | list    | yes      | Fields that form the aggregation key. Must include all six defined fields.    |
| `include_counter`       | boolean | yes      | If true, summary events include `count` and `window_sec` fields.              |
| `limits.max_keys`       | integer | yes      | Maximum number of concurrent aggregation keys. Must be > 0.                  |
| `limits.ttl`            | integer | yes      | Maximum age of an inactive key in seconds. Must be ≥ 2 × `window`.           |
| `output.type`           | string  | yes      | Output transport. Only `http_stream` is supported.                            |
| `output.path`           | string  | yes      | HTTP path for the streaming endpoint. Must begin with `/`.                    |
| `output.listen`         | string  | yes      | TCP bind address for the HTTP server (e.g. `":8080"`). Required when `output.type` is `http_stream`. |

### Validation Rules

- `mode` must be `"rate"`. Any other value causes startup failure.
- `output.type` must be `"http_stream"`. Any other value causes startup failure.
- `window` must be a positive integer. Zero or negative causes startup failure.
- `limits.ttl` must be ≥ 2 × `limits.window`. Violation causes startup failure.
- `limits.max_keys` must be > 0. Zero or negative causes startup failure.
- `key_fields` must contain exactly the six defined fields. Missing or extra fields cause startup failure.
- `output.path` must begin with `/`. Any other value causes startup failure.
- `output.listen` must not be empty when `output.type` is `http_stream`. Empty value causes startup failure.
- If `enabled` is `false`, all other fields are ignored. No listener is started. No map is allocated.

### Minimal Config Example

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
    max_keys: 10000
    ttl: 60
  output:
    type: http_stream
    path: /events
    listen: ":8080"
```

### Full Config Example

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
    max_keys: 50000
    ttl: 120
  output:
    type: http_stream
    path: /events
    listen: ":8080"
```

Note: The minimal and full config examples differ only in `max_keys` and `ttl`. These values must be tuned by the operator based on the number of distinct clients, function codes, and expected request rates.

---

## 9. Non-Goals (STRICT)

The following are explicitly **not supported** by the Access Event System:

- **No audit history.** Events are not stored. Once emitted (or dropped), they are gone.
- **No guaranteed delivery.** The system makes no delivery promise. Slow consumers, full buffers, and key overflow all result in silent drops.
- **No read tracking persistence.** There is no on-disk or in-database record of access events.
- **No semantic interpretation.** Function codes are recorded as integers. The system does not interpret or classify their meaning beyond read/write categorization.
- **No coupling to external systems.** The access event system does not call, query, or depend on any external system at runtime.
- **No modification of Modbus responses.** Access events are entirely invisible to Modbus protocol behavior. No Modbus frame is altered, delayed, or rejected based on event state.
- **No integration with the Notification Engine.** The two systems are independent. They share no code, no state, and no configuration path.
- **No value inclusion.** Memory values are never included in access events.
- **No aggregation modes other than `rate`.** There is no per-event mode, no sampling mode, and no deduplicated-only mode.
- **No dynamic configuration.** Configuration is fixed at startup. There is no runtime reload.

---

## 10. Safety Guarantees

### Non-Blocking Emission

Event emission must never block the Modbus execution path.

- Event writes to the stream channel or output buffer must be non-blocking (drop-on-full semantics).
- The access control decision must be returned to the protocol layer immediately, independent of emission success.
- No goroutine or lock held by the event system may block forward progress of a Modbus handler.

### Failure Isolation

Failures in the access event system must not propagate to the Modbus path.

- If the HTTP stream has no connected consumer: events are discarded silently.
- If the aggregation map is at capacity: events are discarded silently.
- If the cleanup goroutine panics: it must recover and restart without affecting any other component.
- If the `/events` HTTP listener fails to start: this must cause a startup failure (explicit error, clean exit) — it must not silently degrade.

### Bounded Memory Usage

The aggregation map is bounded by `max_keys`.

- Each key entry has a fixed, known size (key string + two int64 values).
- Total memory used by the aggregation map is bounded by `max_keys * (key_size + 16 bytes)`.
- The operator must size `max_keys` to fit within available memory.
- The system does not allocate beyond `max_keys` entries regardless of request volume.

### Deterministic Behavior

The access event system must behave deterministically under all inputs.

- Given the same event sequence and the same configuration, the output must be identical.
- There are no adaptive behaviors (no auto-tuning of window sizes, no dynamic key eviction policies).
- TTL cleanup is deterministic: it fires at configured intervals and follows the defined algorithm exactly.
- Overflow follows the drop policy without exception. There is no fallback or retry.

---

**End of Access Event System Design**

---

## Consistency Verification

- Cleanup behavior is strictly non-emitting
- TTL constraint is ≥ 2 × window
- Event emission is fully event-driven
- No background-triggered outputs exist
