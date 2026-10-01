# Modbus Device Identification

## Purpose

MMA 2.0 supports Modbus Encapsulated Interface Transport FC43 (0x2B), MEI type 14 (0x0E), Read Device Identification.

The feature exposes the identity of the MMA appliance without adding identity data to core memory. Device identification is protocol metadata owned by the Modbus transport.

The configured identity is global to the MMA process. It is not configured per listener, per port, or per Unit ID.

---

## Configuration

Device identity is an optional top-level YAML section:

```yaml
device_identity:
  vendor_name: "github.com/tamzrod"
  product_code: "MMA2"
  major_minor_revision: "2.0"
```

All fields are optional independently.

| Field | Modbus object | Object ID | Default |
|---|---|---:|---|
| `vendor_name` | VendorName | `0x00` | `github.com/tamzrod` |
| `product_code` | ProductCode | `0x01` | `MMA2` |
| `major_minor_revision` | MajorMinorRevision | `0x02` | compiled MMA2 release version |

If the complete `device_identity` section is absent, all compiled defaults are used.

A partial override changes only the fields that are present:

```yaml
device_identity:
  product_code: "MMA2-PPC"
```

The example above keeps the default VendorName and compiled release version.

Identity is resolved once during startup and is immutable for the lifetime of the process. There is no Modbus register or runtime interface for changing it.

---

## Validation

Each configured identity value must:

- be present as a non-empty value when explicitly configured;
- contain only ASCII bytes;
- contain at most 244 bytes.

Invalid identity configuration fails startup validation.

Values are never truncated to fit a Modbus response.

---

## Supported Read Device Identification Requests

MMA supports Read Device Identification codes 1 through 4.

| Code | Access | MMA behavior |
|---:|---|---|
| `0x01` | Basic stream | returns available Basic objects |
| `0x02` | Regular stream | returns the available Basic objects implemented by MMA |
| `0x03` | Extended stream | returns the available Basic objects implemented by MMA |
| `0x04` | Individual | returns the requested Basic object |

MMA currently implements only the three mandatory Basic identification objects:

- Object `0x00` — VendorName
- Object `0x01` — ProductCode
- Object `0x02` — MajorMinorRevision

The conformity level returned by MMA is `0x81`: Basic identification with individual access supported.

Responses preserve whole objects and are limited to the Modbus maximum PDU size of 253 bytes. If the remaining objects do not fit, `More Follows` is set and `Next Object ID` identifies the continuation point.

For stream access (codes 1-3), an unknown starting Object ID restarts the stream at Object `0x00`.

For individual access (code 4), an unknown Object ID returns Illegal Data Address (`0x02`).

---

## Request and Response Shape

A Basic stream request for Object `0x00` has this PDU:

```text
2B 0E 01 00
│  │  │  └─ Object ID 0x00
│  │  └──── Read Device ID code 0x01 (Basic)
│  └─────── MEI type 0x0E
└────────── FC43 / 0x2B
```

With the default identity, the response PDU begins:

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

The object list that follows contains object ID, object length, and object value for each returned object.

---

## Exception Behavior

| Condition | Modbus exception |
|---|---:|
| Unsupported MEI type | `0x01` Illegal Function |
| Unknown object for individual access | `0x02` Illegal Data Address |
| Invalid request length or Read Device ID code | `0x03` Illegal Data Value |

These are protocol-level FC43/MEI14 errors. Authority and State Sealing may reject the request before MEI14 processing, as described below.

---

## Authority and State Sealing

Device identification does not bypass MMA security controls.

The Modbus request path remains:

```text
Modbus request
    ↓
State Sealing
    ↓
Authority policy
    ↓
FC43 / MEI14 device-identification handler
```

Therefore:

1. If the addressed `(Port, UnitID)` memory is sealed, FC43 receives the configured State Sealing exception.
2. If authority denies FC43, the request receives the authority denial response.
3. Only an allowed and unsealed request reaches the device-identification handler.

To permit identity reads, the matching memory policy must explicitly include function code 43:

```yaml
policy:
  rules:
    - id: allow-client
      source_ip:
        - 192.168.1.0/24
      allow_fc: [1, 2, 3, 4, 43]
```

Adding `device_identity` to the YAML does not automatically authorize FC43.

Because authority and State Sealing are memory-scoped, the client must address an existing `(Port, UnitID)` whose policy permits FC43. The identity value returned is nevertheless the same process-wide MMA identity.

---

## Observability

FC43 passes through the authority decision path, but the current Access Event engine classifies only FC1-FC4 as `read` and FC5, FC6, FC15, and FC16 as `write`.

FC43 is a non-memory Modbus service and is not currently classified as either action. Therefore:

- allowed FC43 requests do not emit Access Events;
- denied FC43 requests do not emit Access Events;
- FC43 does not produce Notification Engine write events;
- FC43 does not produce RBE memory events.

This is an observability boundary only. State Sealing and authority enforcement still apply normally.

---

## Raw Ingest

Raw Ingest is unaffected.

Device identification exists only in the Modbus transport. Raw Ingest does not expose or modify the identity and does not use FC43.

---

## Architectural Boundary

The implementation intentionally separates configuration from Modbus protocol mechanics:

```text
YAML
 ↓
config.DeviceIdentityConfig
 ↓
config.BuildDeviceIdentityValues()
 ↓
process runtime
 ↓
modbus.NewDeviceIdentity()
 ↓
HandleConnWithIdentity()
 ↓
FC43 / MEI14
```

The memory core does not store, construct, interpret, or dispatch device identity.

This preserves the MMA rule that core memory remains protocol-agnostic.

---

## Operational Notes

- Identity changes require an MMA restart.
- The default revision follows the compiled MMA release version.
- Custom revision text is descriptive identity metadata; it does not change the running binary version.
- Device identity is read-only.
- Device identity does not allocate Modbus registers.
- FC43 is not a memory read and does not produce Access Events, Notification Engine write events, or RBE memory events.

---

**End of Modbus Device Identification**
