# Baseline comparison

| Baseline | Recovered (minor) | Rate | Attempts | Contacts | Avg latency (h) | Cost (minor) |
|---|---:|---:|---:|---:|---:|---:|
| no_recovery | 28776924 | 22.67% | 0 | 0 | 47.06 | 0 |
| fixed_retry | 44872633 | 31.73% | 501 | 249 | 29.10 | 28740 |
| rules | 53330344 | 37.60% | 260 | 409 | 26.57 | 104100 |
| contextual_retry | 36999556 | 28.67% | 292 | 0 | 35.55 | 10220 |
