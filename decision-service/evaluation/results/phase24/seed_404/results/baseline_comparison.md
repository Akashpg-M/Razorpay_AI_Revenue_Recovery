# Baseline comparison

| Baseline | Recovered (minor) | Rate | Attempts | Contacts | Avg latency (h) | Cost (minor) |
|---|---:|---:|---:|---:|---:|---:|
| no_recovery | 31258287 | 24.53% | 0 | 0 | 47.44 | 0 |
| fixed_retry | 45222781 | 33.07% | 509 | 241 | 27.69 | 28660 |
| rules | 50758033 | 38.00% | 230 | 452 | 24.59 | 108080 |
| contextual_retry | 31243875 | 25.33% | 275 | 0 | 33.26 | 9625 |
