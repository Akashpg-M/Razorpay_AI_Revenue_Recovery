# Baseline comparison

| Baseline | Recovered (minor) | Rate | Attempts | Contacts | Avg latency (h) | Cost (minor) |
|---|---:|---:|---:|---:|---:|---:|
| no_recovery | 23934707 | 21.60% | 0 | 0 | 50.17 | 0 |
| fixed_retry | 42608073 | 34.80% | 515 | 235 | 28.29 | 28600 |
| rules | 41892917 | 35.47% | 253 | 414 | 27.10 | 113305 |
| contextual_retry | 34009174 | 26.53% | 277 | 0 | 41.91 | 9695 |
