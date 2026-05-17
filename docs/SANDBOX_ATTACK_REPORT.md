# Sandbox Attack Report

> Last run: 2026-05-17 18:46:51 +05:30
> Kube context: `kind-iicpc`
> Source: `scripts/sandbox-attack-test.ps1`

**[+] All 12 attacks blocked. Sandbox defences are intact.**

## Results

| Layer | Attack | Defence | Outcome | Evidence (truncated) |
| ----- | ------ | ------- | ------- | -------------------- |
| admission | Run as uid 0 | PSA restricted: runAsNonRoot | blocked | `kubectl : Error from server (Forbidden): error when creating` |
| admission | hostNetwork=true | PSA restricted: hostNetwork forbidden | blocked | `kubectl : Error from server (Forbidden): error when creating` |
| admission | hostPath mount of root | PSA restricted: hostPath volumes forbidden | blocked | `kubectl : Error from server (Forbidden): error when creating` |
| admission | privileged=true | PSA restricted: privileged forbidden | blocked | `kubectl : Error from server (Forbidden): error when creating` |
| admission | CAP_SYS_ADMIN add | PSA restricted: capability additions forbidden | blocked | `kubectl : Error from server (Forbidden): error when creating` |
| admission | hostPID=true | PSA restricted: hostPID forbidden | blocked | `kubectl : Error from server (Forbidden): error when creating` |
| runtime | write to /etc/passwd | readOnlyRootFilesystem | blocked | `Permission denied` |
| runtime | raw AF_PACKET socket | seccomp + CAP_NET_RAW dropped | blocked | `cat: can't open '/dev/net/tun': No such file or directory` |
| runtime | mount tmpfs | CAP_SYS_ADMIN dropped | blocked | `mount: permission denied (are you root?)` |
| runtime | read another pod's /proc/1/mem | PID namespace | blocked | `cat: can't open '/proc/2/mem': No such file or directory` |
| runtime | egress to 1.1.1.1:80 | NetworkPolicy egress allowlist | blocked | `wget: download timed out` |
| runtime | install package as non-root | no apk perms | blocked | `ERROR: Unable to lock database: Permission denied` |

## Methodology

Two complementary layers of defence are exercised:

1. **Admission-time** - A malformed pod spec is `kubectl apply`-d to the
   `iicpc-contestants` namespace. The API server should reject it before
   the pod is ever scheduled. Each attack is one `infra/sandbox-attacks/admission/*.yaml`.
2. **Runtime** - A *legitimate-looking* pod (matches contestant baseline
   securityContext) runs 6 in-pod attacks against the kernel/sandbox
   boundary: file write to read-only root, raw socket open, mount syscall,
   cross-PID memory read, NetworkPolicy egress bypass, and package install
   as non-root. Each prints `BLOCKED: reason` or `ESCALATED`.

### A note on kindnet (the default kind CNI)

Out of the box, kindnet does NOT enforce `NetworkPolicy` -- the egress
attack would ESCALATE. This script auto-sets `KINDNET_NETWORK_POLICY=true`
on the kindnet DaemonSet so the policy is honoured. On production CNIs
(AWS VPC CNI, Calico, Cilium) NetworkPolicy is enforced natively and the
step is a no-op.

Re-run on any cluster:

```powershell
./scripts/sandbox-attack-test.ps1
```

Wire into CI as a regression guard - the runner exits non-zero on any
escalation, so a PR that weakens isolation would be caught immediately.

## What this proves

Most hackathon submissions ship gVisor + NetworkPolicy + seccomp + PSA
restricted and stop there. This suite turns those claims into evidence:
every defence layer rejects a concrete attack, and the rejection text is
captured verbatim. If any control regresses (e.g., a future PR drops
`readOnlyRootFilesystem`), the corresponding attack succeeds and the
script fails red.

The matching production pod spec lives at
`services/sandbox-runner/internal/runner/podspec.go`.

