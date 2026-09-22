# Run the MMA2 benchmark (Ubuntu Legion)

This first executable slice measures **end-to-end Modbus TCP FC3 read latency and throughput**, through MMA2's actual network listener. It does not yet measure RBE, sealing, CPU/RSS, or produce PDF reports. Do not label it a complete appliance benchmark or claim safety guarantees.

From the repository root, with Go installed:

```bash
go build -o /tmp/mma2-benchmark-server ./cmd/mma2
go build -o /tmp/mma2-benchmark-client ./benchmark/tools
/tmp/mma2-benchmark-server benchmark/configs/loopback.yaml
```

In a second terminal, from the same repository root:

```bash
/tmp/mma2-benchmark-client -addr 127.0.0.1:15020 -unit 1 -clients 1 -warmup 2s -duration 10s -out benchmark/results/local-c1
/tmp/mma2-benchmark-client -addr 127.0.0.1:15020 -unit 1 -clients 8 -warmup 2s -duration 10s -out benchmark/results/local-c8
```

Outputs: `summary.csv`, `latency_ms.csv`, and `summary.svg` under each selected output directory. Client reports a nonzero exit status for any errors or zero successes. Stop the server with Ctrl+C. Confirm configuration validation and listener startup in server logs before running. If the current application rejects the fixture, fix the fixture against the current schema rather than changing production configuration.

The client uses persistent TCP connections, validates MBAP transaction ID and unit ID, and requires a successful FC3 response with one holding register. Request timeouts are 1 second. Latency excludes initial TCP connection establishment; connection failures count as errors. Warm-up is excluded. Benchmark results are not collected automatically in CI; execute on the actual Ubuntu Legion and record the exact Git commit, machine load and Go version.

**Pending:** runtime smoke test on a machine with repository checkout, full feature workloads (RBE, state sealing, access control), CPU/RSS sampling, multiple repetitions, comparison plots and Go PDF report generator. No measured results or PDF are included yet.
