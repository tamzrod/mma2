# Benchmark

## Raspberry Pi remote Modbus TCP benchmark — 2026-09-22

MMA2 was deployed on a Raspberry Pi at `10.5.1.8:15020` and tested remotely from a separate Legion Linux machine using the repository's `benchmark/test-raspberry.sh` runner and `benchmark/tools` load generator.

### Reported results

| Metric | Result |
| --- | ---: |
| Highest observed throughput (8 clients) | 2,127.7 successful requests/s |
| 8-client p99 request/response latency | 10.165 ms |
| Reported errors across all four runs | 0 |
| Total reported successful requests across all four runs | 126,539 |

The test uses Modbus TCP function code 3 (FC3), unit ID 1, holding register address 0, quantity 1. The runner executes 1, 2, 4, and 8 concurrent-client runs with a 5-second warmup and 30-second measured duration each. Request latency is measured by the client from write to validated response on persistent TCP connections; the measurement includes network effects. The load generator records `summary.csv`, `latency_ms.csv`, and `summary.svg` for each concurrency level.

### Reproduce from a separate Linux client

```bash
bash benchmark/test-raspberry.sh
```

The script defaults to `10.5.1.8:15020`; override `TARGET` and `PORT` as needed. Run against an isolated test appliance only, not a live control system. See `benchmark/RUN.md` for the general benchmark instructions.

### Interpretation and limitations

These figures characterize the specific Raspberry Pi deployment, network path, test configuration, and FC3 workload, **not** a hardware-independent MMA2 throughput guarantee. They do not separately isolate Pi CPU processing from network latency and do not measure RBE delivery, state sealing under load, access-control rejection, or worst-case PPC control-loop response. Keep the raw result directory and deployment commit with the report for reproducibility.

The separately generated graphical PDF report is not yet stored in this repository; the repository documentation above preserves the reported measurements without linking to a nonexistent file.
