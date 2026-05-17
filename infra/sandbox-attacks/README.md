# Sandbox Attack Suite

> Reproducible "pentest" proving the contestant sandbox actually blocks the
> attacks its config claims to. Most hackathon submissions ship `gVisor +
> NetworkPolicy + seccomp + PSA restricted` and stop there. This suite
> proves each layer rejects a concrete attack.

Two layers of defence are exercised:

| Layer                | What it blocks                    | Tests live in    |
| -------------------- | --------------------------------- | ---------------- |
| **Admission-time**   | API server rejects malformed pod  | `admission/*.yaml` |
| **Runtime**          | Kernel/sandbox rejects bad syscall | `runtime/*.yaml` |

## How to run

```powershell
# 1. Bring up the kind cluster (one-time)
kind create cluster --config infra/kind/cluster.yaml

# 2. Apply the cluster baseline (namespace + PSA + NetworkPolicy)
kubectl apply -f infra/manifests/sandbox-runner.yaml

# 3. Run the full attack suite — produces docs/SANDBOX_ATTACK_REPORT.md
./scripts/sandbox-attack-test.ps1
```

Each attack is:
1. A pod manifest or in-pod command in `admission/` or `runtime/`.
2. An expected outcome (rejection message or non-zero exit).
3. Captured into the report by the runner script.

If an attack ever **succeeds** when it should fail, the runner script exits
non-zero — making this safe to wire into CI as a regression guard.
