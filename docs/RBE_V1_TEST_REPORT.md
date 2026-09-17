# RBE v1 test report

Branch: `feature/rbe-tcp-v1`  
Date: 2026-09-17  
Scope: test the one-byte RBE implementation, fix verified defects, add regressions.  
Constraints honored: `main` not modified, architecture not redesigned, nothing deployed.

## Commands actually run

Working tree: clone of `tamzrod/mma2` at `feature/rbe-tcp-v1`.

```text
GOPROXY=https://proxy.golang.org,direct
go test ./... -count=1
go test -race ./internal/rbe ./internal/memorycore \
  ./internal/transport/modbus ./internal/transport/rawingest \
  ./internal/config -count=1
```

The sandbox default `GOPROXY=http://35.245.43.102/go/` returned `502` for
`gopkg.in/yaml.v3@v3.0.1`. Tests that import config were re-run against
`https://proxy.golang.org,direct`. That is an environment issue, not an RBE defect.

## Results

| Package | `go test ./...` | `-race` |
|---|---|---|
| `internal/rbe` | ok | ok |
| `internal/memorycore` | ok | ok |
| `internal/config` | ok | ok |
| `internal/transport/modbus` | ok | ok |
| `internal/transport/rawingest` | ok | ok |
| `tools` | ok (no tests) | n/a |
| `cmd/mma2`, `internal/ingress`, `internal/notify`, others | no test files | n/a |

`go test ./...` after the fixes: **PASS**.  
Focused race tests after the fixes: **PASS**.

No measured PPC trigger-to-read p95/p99 numbers were collected. The draft
explicitly forbids inventing latency guarantees.

## Verified defect and fix

**TCP overflow / shutdown could leave a writer parked on a full per-client
queue, and closing that queue without an `ok` check would emit reserved
`0x00`.**

`TCPPublisher.Publish` disconnected a slow subscriber by deleting it from the
client map and closing the socket while the write loop was still selected on
the live queue. After overflow nobody sends on that queue again. Closing the
publisher later closed the same class of channel; `case id := <-queue` treats
a closed channel as `id == 0` and would write the reserved byte.

Fix in `internal/rbe/tcp.go`:

- overflow closes the per-client queue, then closes the socket outside the lock
- `writeLoop` treats `!ok` as disconnect and never writes `0x00`
- `Close` closes remaining queues so writers unblock

Regression: `TestTCPPublisherOverflowDisconnectsSlowSubscriber`.

## Regressions added

- `internal/rbe`: overflow disconnect, independent subscriber queues, overlapping
  rules, unaligned coil change-only compare, concurrent writers, MultiSink fan-out
- `internal/transport/modbus`: FC16 change-only RBE, reads emit nothing
- `internal/transport/rawingest`: sealed Raw Ingest write commits with no event,
  unseal has no catch-up, later unsealed change emits one RuleID

Existing engine / config / Influx metadata-only tests still pass.

## Remaining blockers (do not treat as done)

1. Contract is still **NOT LOCKED**. `docs/MMA_Notification_Engine_LOCKED.md`
   must stay until review + cutover.
2. No live MMA2 process + real PPC p95/p99 trigger-to-read measurement.
3. No process-level binary test of `cmd/mma2` on `examples/rbe-tcp.yaml`
   (config listen ports are fixed; `:0` is rejected by validation).
4. Influx isolation was unit-tested (metadata-only HTTP sink + MultiSink).
   There was no live InfluxDB in this environment.
5. Do not remove legacy `notify` or merge to `main` until the contract is
   reviewed and the latency gate is measured on a plant-like path.

## Not done, by instruction

- no commit or checkout of `main`
- no architecture redesign
- no deploy
