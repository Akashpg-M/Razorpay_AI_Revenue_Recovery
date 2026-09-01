# Baseline comparison

| Baseline | Recovered (minor) | Rate | Attempts | Contacts | Avg latency (h) | Cost (minor) |
|---|---:|---:|---:|---:|---:|---:|
| no_recovery | 21657868 | 17.73% | 0 | 0 | 48.25 | 0 |
| fixed_retry | 39448265 | 31.07% | 513 | 237 | 27.38 | 28620 |
| rules | 42776621 | 33.33% | 253 | 401 | 24.77 | 89145 |
| contextual_retry | 34150846 | 25.87% | 275 | 0 | 38.38 | 9625 |
