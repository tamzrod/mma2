# RBE v1 test report

Branch: `feature/rbe-tcp-v1`
Date: 2026-09-17
Scope: test the one-byte RBE implementation, fix verified defects, add regressions.
Constraints honored: `main` not modified, architecture not redesigned, nothing deployed.

This report covers the current tip. It supersedes the earlier iteration on this
branch, which fixed an overflow path that could emit reserved `0x00` and shipped
the first regression set. This iteration independently re-verified the
implementation, added process-level end-to-end coverage, and fixed two further
defects.

## Commands actually run

Toolchain: Go 1.25.0 (installed locally in the sandbox; the base image had no Go).
Module download used `GOPROXY=https://proxy.golang.org,direct`.

```text
go build ./...
go vet ./...
go test ./... -count=1
go test -race ./... -count=1
go test ./internal/rbe/ ./internal/config/ -count=30
bash test/rbe_e2e/run_test.sh          # process-level end-to-end
python3 test/rbe_e2e/e2e.py            # negative control also run
```

## Results

| Package | `go test ./...` | `-race` | notes |
|---|---|---|---|
| `internal/rbe` | ok | ok | incl. 4000-iteration differential check |
| `internal/memorycore` | ok | ok | |
| `internal/config` | ok | ok | |
| `internal/transport/modbus` | ok | ok | |
| `internal/transport/rawingest` | ok | ok | |
| `tools` | ok (no tests) | n/a | |
| `cmd/mma2`, `internal/ingress`, `internal/notify`, others | no test files | n/a | |

`go build ./...`, `go vet ./...`, `go test ./...` and `go test -race ./...`:
**PASS** on this tip. `internal/rbe` and `internal/config` also passed 30
consecutive runs (flakiness check).

Process-level end-to-end (`test/rbe_e2e/run_test.sh`): **PASS**. It builds
`cmd/mma2`, starts it on `test/rbe_e2e/config.yaml`, and drives the real binary
over TCP:

- a fresh RBE connection is silent (no snapshot/replay)
- a watched input-register write while sealed commits but emits nothing
- the unsealing write emits nothing (no catch-up)
- an identical refresh emits nothing
- a real change in rule 1 emits exactly one byte `0x01`
- a real change in rule 2 emits exactly one byte `0x02`
- Modbus FC4 then reads the value written via Raw Ingest
- no extra bytes follow the expected events

A negative control (rule 1 moved off the tested range) makes the harness report
`FAIL - rule 1 event missing`, confirming it detects wrong results rather than
always passing. That closes the earlier report's open item (3).

No measured subscriber trigger-to-read p95/p99 numbers were collected. The draft
explicitly forbids inventing latency guarantees; there is no application-like
path in this environment. One lab used a plant controller and Node-RED as
subscribers; that is evidence for those clients, not a definition of RBE.

## Defects verified and fixed in this iteration

### 1. Influx line-protocol injection via rule name

`rbe.escapeTag` escaped `\\`, space, `,` and `=` but not CR/LF. A configured rule
name such as `EVIL\\ninjected line,name=x` produced a multi-line HTTP body, so
one RBE event became several line-protocol points. The measurement name was
already validated against `" ,\\r\\n"`, but rule names were not.

Fix:

- `NewInfluxSink` rejects rule names containing `\\r` or `\\n` before the sink starts.
- `BuildRBERules` rejects such names at configuration validation, with a clear
  `listeners[...].rbe.<area>[i].name` path.
- `escapeTag` defensively strips CR/LF so one event always serializes to one line.

Regressions: `TestInfluxSinkRejectsRuleNameWithLineBreak`,
`TestInfluxSinkSingleEventSingleLine`,
`TestBuildRBERulesRejectsRuleNameWithLineBreak`.

Reverted-fix check: the two Influx tests and the config test fail on the
unfixed tip, then pass with the fix. Verified.

### 2. Transient TCP accept error permanently disabled RBE delivery

`TCPPublisher.acceptLoop` treated any non-closed `Accept` error as terminal and
called `Close()`. Real listeners can return transient/temporary errors (for
example descriptor exhaustion). One such error stopped RBE for the process
lifetime.

Fix: retry temporary errors with a small bounded backoff (5 ms) and exit the
loop only on closure or a permanent error, matching the standard-library server
pattern. No change to wire semantics, queues, or the memory write path.

Regression: `TestTCPPublisherTransientAcceptErrorIsNotFatal`.

Reverted-fix check: the test fails on the unfixed tip (connection refused) and
passes with the fix. Verified.

## Defect fixed in the previous iteration (retained)

`TCPPublisher` overflow could leave a writer parked on a full per-client queue,
and closing that queue without an `ok` check could emit reserved `0x00`. Fixed
by closing the queue on overflow, checking `ok` in `writeLoop`, and closing
remaining queues on `Close`. Regressions:
`TestTCPPublisherOverflowDisconnectsSlowSubscriber`,
`TestTCPPublisherIndependentSubscriberQueues`.

## Additional verification performed

- Randomized differential test (`TestEngineDifferentialRandomWrites`): 6 random
  rules over registers and coils, 4000 random writes, compared against an
  independent reference model. Emit decisions matched exactly, with at most one
  event per intersecting rule.
- Absolute-address handling (`TestEngineRuleAgainstNonZeroAreaStart`): rules are
  compared in absolute address space, not a remapped offset.
- Sealing is per memory; unsealing one memory does not suppress another
  (verified during probing; covered by existing sealing tests plus the e2e run).
- Concurrent writers through the engine under `-race`: no races; memory stays
  authoritative and the engine adds no last-value cache.

## Remaining blockers (do not treat as done)

1. Contract is still **NOT LOCKED**. `docs/MMA_Notification_Engine_LOCKED.md`
   must stay until review + cutover.
2. No measured trigger-to-read p95/p99 latency on a real subscriber path.
3. No live InfluxDB; Influx isolation is covered by an in-process HTTP test
   server only.
4. Do not remove legacy `notify` until the contract is reviewed and the latency
   gate is measured on a realistic subscriber path.
5. The e2e harness uses fixed localhost ports (15020/19001) and requires them
   to be free.

## Not done, by instruction

- no commit or checkout of `main`
- no architecture redesign
- no deploy
