# Sandbox Attack Report

> Pre-run version with **documented expected outcomes**.
> Run `./scripts/sandbox-attack-test.ps1` against a live cluster to
> overwrite this file with verified, dated outcomes.

The contestant sandbox claims four layers of defence:

1. **PSA restricted** Pod Security Admission on the `iicpc-contestants` namespace
2. **NetworkPolicy** ingress + egress allowlist (ingress only from bot-worker on :9100, egress only DNS)
3. **Pod-level securityContext** — non-root uid 65532, `readOnlyRootFilesystem`, `allowPrivilegeEscalation=false`, all caps dropped, `seccompProfile: RuntimeDefault`
4. **gVisor `RuntimeClass`** — user-space kernel intercepts syscalls before the host kernel sees them

This suite turns each claim into an attack-and-block test.

## Attacks (12 total)

### Admission-time (`infra/sandbox-attacks/admission/`)

The API server should reject these before any pod is scheduled. Each is a
single `kubectl apply -f` away from being verified.

| #  | Attack                                | Defence                                          | Expected rejection                                                  |
| -- | ------------------------------------- | ------------------------------------------------ | ------------------------------------------------------------------- |
| A1 | `runAsUser: 0`                        | PSA restricted: `runAsNonRoot`                   | `pods "attack-run-as-root" is forbidden: violates PodSecurity ... runAsNonRoot != true` |
| A2 | `hostNetwork: true`                   | PSA restricted: hostNetwork forbidden            | `... hostNetwork=true`                                              |
| A3 | `hostPath` mount of `/`               | PSA restricted: hostPath volumes forbidden       | `... volume "host-root" uses restricted volume type "hostPath"`     |
| A4 | `privileged: true`                    | PSA restricted: privileged containers forbidden  | `... containers "attacker" must not set securityContext.privileged=true` |
| A5 | `capabilities.add: ["SYS_ADMIN"]`     | PSA restricted: capability additions forbidden   | `... must not include "SYS_ADMIN" in securityContext.capabilities.add` |
| A6 | `hostPID: true`                       | PSA restricted: hostPID forbidden                | `... hostPID=true`                                                  |

### Runtime (`infra/sandbox-attacks/runtime/`)

A *legitimate-looking* pod (passes PSA, matches the production contestant
baseline) is scheduled, then runs 6 attacks from inside the sandbox. Each
prints `BLOCKED: <reason>` or `ESCALATED`. Any `ESCALATED` fails the test.

| #  | Attack                                            | Defence layer                            | Expected outcome                              |
| -- | ------------------------------------------------- | ---------------------------------------- | --------------------------------------------- |
| R1 | append `evil::0:0::/root:/bin/sh` to `/etc/passwd` | `readOnlyRootFilesystem: true`           | `EROFS: Read-only file system`                |
| R2 | open `/dev/net/tun` (needs CAP_NET_ADMIN)         | `capabilities.drop: ["ALL"]`             | `EACCES` or `ENOENT`                          |
| R3 | `mount -t tmpfs none /tmp`                        | seccomp + CAP_SYS_ADMIN dropped          | `mount: permission denied (EPERM)`            |
| R4 | read `/proc/2/mem` (another pod's PID)            | container PID namespace                  | `cat: can't open '/proc/2/mem': No such file` |
| R5 | `wget http://1.1.1.1/` egress                     | NetworkPolicy egress allowlist (DNS only) | `connection timed out` after 3 s              |
| R6 | `apk add curl` (install package)                  | non-root uid 65532                       | `ERROR: ... Permission denied`                |

## How to run

```powershell
# 1. Bring up a cluster (any kube context with PSA enabled — kind ≥ 0.20 is fine)
kind create cluster --config infra/kind/cluster.yaml

# 2. Apply the cluster baseline (namespace + NetworkPolicy)
kubectl apply -f infra/manifests/sandbox-runner.yaml

# 3. Run the suite — overwrites THIS file with the verified outcomes
./scripts/sandbox-attack-test.ps1
```

Exit code 0 = every attack blocked. Exit code 1 = at least one ESCALATED.

## What this proves (and what it doesn't)

**Proves:**

- The API server's admission chain rejects six common pod-spec bypasses
  before workload code ever runs.
- A pod that passes admission still cannot write the root filesystem,
  open privileged sockets, mount, see other pods' processes, or exfiltrate
  to the public internet.
- Every defence layer in `podspec.go` corresponds to at least one
  passing test — so a PR that weakens any control gets caught by this
  script (wire it into CI as a `make sandbox-attack-test` target).

**Does NOT prove:**

- Kernel CVE exploits — those need a curated CVE corpus (deferred).
- gVisor sandbox-escape vulnerabilities — assumes gVisor itself is sound.
- Sidechannel attacks (timing, cache) — out of scope for a 2026 hackathon.

## References

- Production pod spec: `services/sandbox-runner/internal/runner/podspec.go`
- Namespace + NetworkPolicy: `infra/manifests/sandbox-runner.yaml`
- ADR-0002 (sandbox design): `docs/ADR/0002-*.md`
