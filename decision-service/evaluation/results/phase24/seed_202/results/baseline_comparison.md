# Baseline comparison

| Baseline | Recovered (minor) | Rate | Attempts | Contacts | Avg latency (h) | Cost (minor) |
|---|---:|---:|---:|---:|---:|---:|
| no_recovery | 27477584 | 23.60% | 0 | 0 | 49.64 | 0 |
| fixed_retry | 43748526 | 31.87% | 535 | 215 | 27.48 | 28400 |
| rules | 48296761 | 36.27% | 282 | 396 | 24.63 | 119390 |
| contextual_retry | 34455642 | 27.73% | 312 | 0 | 44.15 | 10920 |
