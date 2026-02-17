# MMA 2.x --- Notification Engine (Rule-Centric, Write-Only)

**STATUS: LOCKED**

Scope: Mutation notification only\
Non-goal: Change detection, semantic processing, history tracking

------------------------------------------------------------------------

## 1. Purpose

Notification emits an event when a write operation intersects a
configured notify rule.

It answers:

-   A write happened.
-   Which rule(s) were matched?
-   From where did it originate?
-   When did it occur?

It does NOT:

-   Inspect values\
-   Compare previous state\
-   Detect change\
-   Merge rules\
-   Modify write behavior

------------------------------------------------------------------------

## 2. Core Architectural Guarantee

Memory core remains pure.

Memory core:

-   Does not know notify exists\
-   Does not store metadata\
-   Does not store source IP\
-   Does not emit events\
-   Does not compare values

Notification is implemented entirely outside `memorycore`.

------------------------------------------------------------------------

## 3. YAML Configuration (Rule-Based)

``` yaml
listeners:
  - listen: 502
    memory:
      - unit_id: 1
        holding_registers:
          start: 0
          count: 500

        notify:
          holding_registers:
            - start: 300
              count: 1
              name: active_power_setpoint

            - start: 300
              count: 5
              name: control_block

            - start: 400
              count: 2
```

------------------------------------------------------------------------

## 4. Rule Semantics

Each entry under `notify.<area>` is an independent rule.

Rules:

1.  If a write intersects a rule → emit one event for that rule.\
2.  One write may generate multiple events.\
3.  Overlapping rules are allowed.\
4.  No rule merging.\
5.  No deduplication.\
6.  Rule evaluation order does not matter.\
7.  Event identity is tied to the rule.

------------------------------------------------------------------------

## 5. Event Trigger Condition

A rule matches when:

    (write_start ≤ rule_end) AND (write_end ≥ rule_start)

Standard range intersection logic.

------------------------------------------------------------------------

## 6. Event Structure (Internal Contract)

Each emitted event contains:

-   Port\
-   UnitID\
-   Area\
-   Start (original write start)\
-   Count (original write count)\
-   Source (transport class)\
-   SourceIP (client IP)\
-   Timestamp (OS wall clock)\
-   Name (optional, if defined in rule)

No memory values included.

------------------------------------------------------------------------

## 7. Timestamp Policy

Timestamp source:

> Host OS wall clock at successful write commit.

Not client time.\
Not device time.\
Not protocol time.

------------------------------------------------------------------------

## 8. Name Field Policy

-   `name` is optional.\
-   If present in rule → included as tag in event.\
-   If absent → event has no `name` field.\
-   Overlapping named rules produce separate events.\
-   No conflict resolution required.

------------------------------------------------------------------------

## 9. Influx Line Protocol Output

Measurement:

    mma_notify

Tags:

-   port\
-   unit\
-   area\
-   source\
-   src_ip\
-   name (optional)

Fields:

-   start\
-   count

Example (named rule):

    mma_notify,port=502,unit=1,area=holding,source=modbus,src_ip=192.168.1.50,name=active_power_setpoint start=300i,count=1i 1700000000000000000

Example (unnamed rule):

    mma_notify,port=502,unit=1,area=holding,source=modbus,src_ip=192.168.1.50 start=400i,count=2i 1700000005000000000

------------------------------------------------------------------------

## 10. Non-Goals (Explicitly Forbidden)

Notification is NOT:

-   A change detector\
-   A value comparison engine\
-   A semantic interpreter\
-   A rule processor affecting memory\
-   A control enforcement mechanism\
-   A historian

Change detection must be done externally (e.g., via delta query in
Influx).

------------------------------------------------------------------------

## 11. Concurrency & Safety

-   Event emission must be non-blocking.\
-   Adapter failure must not affect write success.\
-   Write path must not wait on adapter IO.\
-   No additional read-before-write operations allowed.

------------------------------------------------------------------------

## 12. Final Behavioral Model

System behavior:

> Write operation\
> → Match notify rules\
> → Emit one event per matched rule\
> → Continue normally

Deterministic.\
Predictable.\
Layered.\
Architecturally clean.
