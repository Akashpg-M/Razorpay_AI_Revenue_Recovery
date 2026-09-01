# Baseline comparison

| Baseline | Recovered (minor) | Rate | Attempts | Contacts | Avg latency (h) | Cost (minor) |
|---|---:|---:|---:|---:|---:|---:|
| no_recovery | 27142409 | 21.87% | 0 | 0 | 51.35 | 0 |
| fixed_retry | 42359379 | 33.87% | 536 | 214 | 27.85 | 28390 |
| rules | 39102773 | 35.07% | 258 | 405 | 25.65 | 92075 |
| contextual_retry | 28206932 | 26.67% | 292 | 0 | 39.15 | 10220 |
