# Modbus Device Identification

## Purpose

MMA 2.0 supports Modbus Encapsulated Interface Transport FC43 (0x2B), MEI type 14 (0x0E), Read Device Identification.

Device identification is Modbus protocol metadata associated with a logical MMA device identified by `(Port, UnitID)`. It is configured per `listeners[].memory[]` entry.

FC43 metadata is not Modbus memory data. It is owned by the Modbus/configuration edge and is not stored in or interpreted by `memorycore`.

---

## Configuration

Device identity is configured with an optional `fc43` section under each memory entry:

```yaml
listeners:
  - id: modbus
    listen: "0.0.0.0:502"
    memory:
      - unit_id: 1
        fc43:
          vendor_name: "github.com/tamzrod"
          product_code: "MMA2"
          major_minor_revision: "2.0"
        holding_registers:
          start: 0
          count: 100

      - unit_id: 2
        fc43:
          product_code: "MMA2-Unit2"
        holding_registers:
          start: 0
          count: 100
```

The listener port and `unit_id` form the runtime identity used to select FC43 metadata:

```text
MemoryID = (Port, UnitID)
```

Each logical device may therefore expose an independent FC43 identity.

| Field | Modbus object | Object ID | Default |
|---|---|---:|---|
| `vendor_name` | VendorName | `0x00` | `github.com/tamzrod` |
| `product_code` | ProductCode | `0x01` | `MMA2` |
| `major_minor_revision` | MajorMinorRevision | `0x02` | compiled MMA2 release version |

All fields are optional independently. If `fc43` is absent for a memory, all compiled defaults are used for that logical device. A partial override changes only fields that are present.

The former root-level `device_identity` configuration is no longer supported and is rejected during validation.

Identity is resolved once during startup and remains immutable for the lifetime of the process. There is no register or runtime interface for changing it.

---

## Validation

Each explicitly configured identity value must:

- be non-empty;
- contain only ASCII bytes;
- contain at most 244 bytes.

Invalid identity configuration fails startup validation. Values are never truncated.

Duplicate logical identities for the same `(Port, UnitID)` are rejected.

---

## Supported Read Device Identification Requests

MMA supports Read Device Identification codes 1 through 4.

| Code | Access | MMA behavior |
|---:|---|---|
| `0x01` | Basic stream | returns available Basic objects |
| `0x02` | Regular stream | returns the available Basic objects implemented by MMA |
| `0x03` | Extended stream | returns the available Basic objects implemented by MMA |
| `0x04` | Individual | returns the requested Basic object |

MMA currently implements the three mandatory Basic identification objects:

- Object `0x00` — VendorName
- Object `0x01` — ProductCode
- Object `0x02` — MajorMinorRevision

The conformity level returned by MMA is `0x81`: Basic identification with individual access supported.

Responses preserve whole objects and are limited to the Modbus maximum PDU size of 253 bytes. If remaining objects do not fit, `More Follows` is set and `Next Object ID` identifies the continuation point.

For stream access (codes 1-3), an unknown starting Object ID restarts the stream at Object `0x00`.

For individual access (code 4), an unknown Object ID returns Illegal Data Address (`0x02`).

---

## Request and Response Shape

A Basic stream request for Object `0x00` has this PDU:

```text
2B 0E 01 00
│  │  │  └─ Object ID 0x00
│  │  └──── Read Device ID code 0x01
│  └─────── MEI type 0x0E
└────────── FC43 / 0x2B
```

The response begins:

```text
2B 0E 01 81 00 00 03 ...
│  │  │  │  │  │  └─ number of objects
│  │  │  │  │  └──── next object ID
│  │  │  │  └─────── more follows
│  │  │  └────────── conformity level
│  │  └───────────── echoed Read Device ID code
│  └──────────────── MEI type
└─────────────────── FC43
```

The object list contains object ID, object length, and object value for each returned object.

---

## Exception Behavior

| Condition | Modbus exception |
|---|---:|
| Addressed logical `(Port, UnitID)` does not exist | `0x02` Illegal Data Address |
| Unsupported MEI type | `0x01` Illegal Function |
| Unknown object for individual access | `0x02` Illegal Data Address |
| Invalid request length or Read Device ID code | `0x03` Illegal Data Value |

Authority and State Sealing may reject the request before FC43/MEI14 processing.

---

## Authority and State Sealing

FC43 does not bypass MMA security controls.

The request path remains:

```text
Modbus request
    ↓
resolve (Port, UnitID)
    ↓
State Sealing
    ↓
Authority policy
    ↓
FC43 / MEI14 handler
    ↓
per-memory FC43 metadata
```

Therefore:

1. If the addressed memory is sealed, FC43 receives the configured State Sealing exception.
2. If authority denies FC43, the request receives the authority denial response.
3. Only an allowed and unsealed request reaches FC43 processing.
4. The identity returned belongs only to the addressed `(Port, UnitID)`.

To permit identity reads, the matching memory policy must explicitly include function code 43:

```yaml
policy:
  rules:
    - id: allow-client
      source_ip:
        - 192.168.1.0/24
      allow_fc: [1, 2, 3, 4, 43]
```

Configuring `fc43` does not automatically authorize FC43.

---

## Raw Ingest, RBE, and Memory Core

Raw Ingest is unaffected.

FC43:

- does not allocate coils or registers;
- does not read or write Modbus memory areas;
- does not modify memorycore;
- does not produce Notification Engine write events;
- does not produce RBE memory events.

The feature shares `(Port, UnitID)` identity with the memory definition while remaining protocol metadata.

---

## Architectural Boundary

The implementation keeps FC43 metadata outside core memory:

```text
listeners[].memory[].fc43
          ↓
config.BuildDeviceIdentities()
          ↓
map[MemoryID]DeviceIdentity
          ↓
process runtime
          ↓
modbus.DeviceIdentities
          ↓
HandleConnWithIdentities()
          ↓
resolve (Port, UnitID)
          ↓
FC43 / MEI14 response
```

`memorycore` continues to store raw memory and remains unaware of FC43.

---

## Operational Notes

- Different Unit IDs on the same listener may expose different identities.
- The same Unit ID on different listener ports may expose different identities.
- Identity changes require an MMA restart.
- The default revision follows the compiled MMA release version.
- Custom revision text is descriptive metadata and does not change the running binary version.
- Device identity is read-only.
- FC43 is not a memory read or write.

---

**End of Modbus Device Identification**
