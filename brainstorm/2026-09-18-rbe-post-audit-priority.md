# RBE post-audit work list (priority / ROI)

Date: 2026-09-18
Status: scratch — not locked, not scheduled
Rule: one item at a time. Do not start the next until the current one is closed.
Scope: follow-up after RBE TCP v1 functional checkpoint (`feature/rbe-tcp-v1` @ `8004e0d`, merged to `main` via PR #18). Contract still NOT LOCKED.

Related: issue #17, `docs/RBE_V1_DRAFT.md`, `docs/RBE_V1_TEST_REPORT.md`.

**Current item:** 2 (`TCPPublisher` lock).

## Sequence

| # | Job | Why this order | Effort | Done when |
|---|---|---|---|---|
| 1 | Lock RuleID `0x00` | **DONE 2026-09-18 — option A.** `0x00` reserved. Legal IDs `1..255`. Draft § RuleID 0x00; issue #17 checked. No runtime change. | Small | Written rule: `0x00` is reserved **or** legal. Engine, sinks, validator, draft, tests, and issue #17 all say the same thing. |
| 2 | Fix `TCPPublisher` lock | Real deadlock on overflow/Close. Overflow is the designed slow-PPC path. | Small | Close and overflow no longer hold `mu` while closing queues. Contention test passes. |
| 3 | Stop shipping the `mma2` binary | 9 MB blob on `main`. `.gitignore` misses `./mma2`. Cheap hygiene. | Tiny | `mma2` untracked, `.gitignore` has `/mma2`, Dockerfile still builds from `cmd/mma2`. |
| 4 | Add CI: `vet` + `test` + `-race` + `test/rbe_e2e` | Tests already exist. CI is what keeps 1–2 from regressing. | Small | Push runs those four; job fails on non-zero. |
| 5 | Delete or wire `authority.Sealing` | Dead second seal model. Cheap now; expensive after someone calls `Seal()`. | Small | One sealing source: the memory coil. No unused `authority.Sealing` path in `Evaluate`. |
| 6 | Process shutdown | Needed before plant use. Signal → close ingress, RBE TCP, Influx loop. | Medium | SIGTERM exits cleanly; e2e still passes; no Accept busy-loop after listener close. |
| 7 | Legacy notify regression test | Cutover gate. Proves old YAML still works after RBE landed on `main`. | Medium | One fixture: root `notify`, no `rbe`, write still notifies, Modbus/Raw responses unchanged. |
| 8 | Neutral example YAML + docs pass | Issue #17 already asked. Stops lab PPC YAML being copied as the contract. | Small–medium | Example shows HR via Modbus **and** IR via Raw Ingest. README / `docs/02_ARCHITECTURE.md` no longer end at `## Archi`. |
| 9 | Influx: timestamp + schema decision | Only if Influx stays in scope. TCP path does not depend on this. | Medium | Written choice: commit-time vs write-time; `mma_rbe` vs old measurement; token not required for TCP-only. |
| 10 | Ingress CIDR before classify | High security ROI, but a new phase (`docs/todo.md`). Do not mix with RBE semantics. | Medium | Connection dropped by listener policy before protocol peek. |
| 11 | Plant p95/p99 | Valuable only after 1–7. Measuring an unlocked contract wastes the run. | Large | Measured trigger→Modbus-read distribution. No invented numbers in docs. |
| 12 | Lock RBE + retire notify | Last. Irreversible. | Large | `RBE_V1` locked, notify doc superseded, notify code/config removable. |

## Defer until someone asks

- REST / MQTT adapters
- filling `internal/domain` stubs
- deleting leftover `copilot/*` branches
- live Influx soak
- version bump past `2.0.2`

## Item 1 — closed

Decision A, 2026-09-18: `0x00` is reserved. Not a RuleID, not a gap/heartbeat. Config/engine reject 0; sinks drop `Publish(0)`. Recorded in `docs/RBE_V1_DRAFT.md` and issue #17.
