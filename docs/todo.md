# MMA2 — Post Phase 3B TODO (Locked)

> Status: Planned  
> Scope: Control-plane hardening and ergonomics  
> Rule: No changes to core semantics without a new phase

---

## 1. Firewall (Ingress-Level Hard Gate)

**Goal:**  
Drop unauthorized traffic **before protocol parsing**.

**Current State:**
- Ingress gate exists
- IP filtering partially present
- Authority currently handles access *after* parsing

**TODO:**
- Add ingress-level firewall rules:
  - Allow / deny CIDR per ingress gate
  - Connection-level rejection
- Ensure firewall:
  - Is protocol-agnostic
  - Applies before Modbus / RawIngest classification
- Keep firewall logic:
  - Out of `authority`
  - Out of `transport`
  - Owned by `ingress`

**Non-goals:**
- No deep packet inspection
- No per-function filtering (that stays in Authority)

---

## 2. YAML Reorganization (Human-Friendly Layer)

**Goal:**  
Improve readability without changing internal identity rules.

**Current Reality (Locked):**
- Memory identity = `(Port, UnitID)`
- Core and Authority depend on this
- YAML keys are NOT identity

**Problem:**
- Current YAML is verbose
- Humans expect memories to be grouped under ingress / listen ports

**TODO:**
- Design a **human-friendly YAML schema** that:
  - Groups memories under ingress or port
  - Avoids repeating `port:` everywhere
  - Compiles deterministically into current internal config
- Implement as:
  - A translation layer in `internal/config`
  - NOT a change to memorycore or authority
- Maintain:
  - Backward compatibility (optional)
  - Zero ambiguity

**Non-goals:**
- No runtime inference
- No implicit defaults
- No weakening of strict policy semantics

---

## Principles (Reaffirmed)

- Pain now > pain forever
- No assumptions at runtime
- Human convenience belongs in config, not core
- Every new behavior is a new phase

---

**End of TODO**
