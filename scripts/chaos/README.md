# Chaos scripts

Three reproducible chaos scenarios used in D27. Full prose and expected
observables: [`../../docs/CHAOS.md`](../../docs/CHAOS.md).

| Script | Tests | Cluster requirement |
|---|---|---|
| `kill-bot-pod.ps1` | Deployment + HPA self-heal | any K8s ≥ 1.27 |
| `isolate-contestant.ps1` | failure-path scoring (timeouts → score drops) | CNI with NetworkPolicy enforcement (Calico / Cilium / EKS) |
| `inject-latency.ps1` | latency penalty in score formula | Pumba-capable nodes — see docs/CHAOS.md "Limitations" |
| `run-suite.ps1` | sequences all three back-to-back for the demo video | as above |

All scripts assume `kubectl` is already configured. They print expected
observable signals to stdout so you can correlate against the UI.
