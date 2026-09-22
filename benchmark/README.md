# MMA2 benchmark workspace

Target: Ubuntu Legion desktop. Scope: full MMA2 appliance. Repository: tamzrod/mma2.

## Verified implementation entry point

`cmd/mma2/main.go` accepts exactly one argument: a `.yaml` or `.yml` configuration file. It loads and validates configuration, constructs the memory store and authority policies, optionally starts notification, RBE TCP and access-events HTTP, then starts ingress listeners. Ingress dispatches Modbus TCP through `modbus.HandleConnWithRBE` and raw ingest through `rawingest.HandleConnWithRBE`.

## Benchmark protocol

1. Record exact Git commit, Go version, kernel, CPU governor, CPU model, memory, and configuration. Never benchmark against a production plant or operational control endpoint.
2. Inspect `docs/04_CONFIGURATION.md`, `docs/06_TRANSPORTS.md`, `docs/01_STATE_SEALING.md`, `docs/03_AUTHORITY_MODEL.md`, `docs/RBE_IMPLEMENTATION_GUIDE.md`, `docs/access_events.md`, and `examples/rbe-tcp.yaml`; confirm behavior against current code before implementing a load generator.
3. Prepare an isolated loopback-only configuration with representative memory and explicit policies. Validate it with the actual executable; do not assume sample configuration is suitable for load testing.
4. Establish correctness first: successful reads/writes, rejected unauthorized requests, state-sealing behavior, RBE framing and delivery, and clean shutdown. Do not count rejected or malformed requests as successful throughput.
5. Run warm-up followed by multiple measured repetitions, one client and increasing concurrent clients. Record operations/s, successes, errors, p50/p95/p99 latency, CPU, RSS, and exact test conditions. Separate protocol response latency from state-change-to-RBE-delivery latency.
6. Compare only equivalent workloads and explicitly identify changes in configuration, security, and observability. Report raw samples and methodology alongside aggregates. Never claim hard real-time guarantees or trip safety from throughput benchmarks.

## Proposed layout

- `README.md`: single index, verified entry point, benchmark methodology and progress.
- `configs/`: isolated benchmark configurations after schema/code verification.
- `tools/`: implementation-specific load generator and runner after transport inspection.
- `results/`: machine-readable measurements, environment manifest, and reports after execution.

## Status

Workspace created. Entry point inspected. Full configuration schema, wire-level request formats, and feature behavior still require code inspection before an executable load generator can be committed. No measurements have been collected. Keep all future benchmark-related scripts, fixtures, documentation, and reports under this directory.
