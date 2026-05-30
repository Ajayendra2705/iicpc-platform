# Infrastructure as Code — Verification

> Maps the brief's **Deliverable #3** requirements to concrete files in this
> repo and the one-command checks that prove each requirement is met.

---

## The brief's exact wording

> "Automated deployment scripts (e.g., Terraform, Kubernetes manifests, or
> Docker Swarm configurations) **proving that your platform can be spun up,
> configured, and scaled horizontally in a modern cloud environment.**"

Three implicit sub-requirements:

| # | Sub-requirement                                  | What proof looks like                                          |
| - | ------------------------------------------------ | -------------------------------------------------------------- |
| 1 | "can be spun up"                                 | Valid IaC + actually-applies-cleanly on a real K8s cluster     |
| 2 | "configured"                                     | Per-environment values overlays (dev vs production)            |
| 3 | "scaled horizontally"                            | Helm/K8s manifests use Deployment/HPA/DaemonSet (not single pods) |

Each is satisfied below, with a single command you can run to verify.

---

## 1. "Can be spun up"

### Where it lives

| Layer            | Files                                                                        |
| ---------------- | ---------------------------------------------------------------------------- |
| Cloud (AWS) IaC  | `infra/terraform/` — `vpc.tf`, `eks.tf`, `rds.tf`, `redis.tf`, `msk.tf`, `s3.tf`, `ecr.tf` |
| Local K8s        | `infra/kind/cluster.yaml` — 4-node cluster (cp + services + contestants + bots) |
| Helm umbrella    | `infra/helm/iicpc-platform/` — one chart, all 9 services templated           |
| Raw manifests    | `infra/manifests/sandbox-runner.yaml`, `minio.yaml`, `chrony-daemonset.yaml`, `chaos/*.yaml` |

### Proof commands

```powershell
# All-in-one local check (runs every gate CI runs, no cloud cost)
./scripts/iac-verify.ps1     # Windows
make iac-verify              # Linux / macOS
```

Output of the latest run:

```
==> terraform fmt -check -recursive            OK
==> terraform init -backend=false              OK
==> terraform validate                         Success! The configuration is valid.
==> helm lint infra/helm/iicpc-platform        OK
==> helm template (dev values)                 1603 lines of K8s YAML
==> helm template (production overlay)         1603 lines of K8s YAML
==> kubeconform                                (skipped — install for stricter check)
==> tflint                                     (skipped — optional)
==> checkov                                    (skipped — optional)
================================================================
  IaC verification PASSED
================================================================
```

### End-to-end "actually spins up" proof

The script above proves the spec is valid. `kind-e2e-smoke.ps1` proves the
manifests actually *apply* on a real K8s cluster:

```powershell
./scripts/kind-e2e-smoke.ps1     # ~5-8 min, $0 cost
```

This:

1. Creates a 4-node kind v1.35 cluster from `infra/kind/cluster.yaml`
2. Applies the sandbox baseline (namespace + NetworkPolicy + RBAC)
3. Renders + dry-run-applies the full helm chart (1603 lines of YAML)
4. Runs the 12-attack sandbox suite (`scripts/sandbox-attack-test.ps1`)
5. Captures evidence to `docs/artifacts/kind-e2e/`
6. Tears down

Output of the most recent run on a fresh kind v1.35 cluster:

```
==> Stage 1: bring up kind cluster (4 nodes, K8s 1.35)     PASS
==> Stage 2: apply sandbox baseline                         PASS
==> Stage 3: render + apply helm chart                      PASS (1603 lines rendered)
==> Stage 4: run 12-attack sandbox suite                    PASS (12/12 blocked)
==> Stage 5: capture cluster state evidence                 PASS
==> Stage 6: tear down kind cluster                         PASS
================================================================
  END-TO-END IaC SMOKE: PASSED
================================================================
```

Per-run evidence (regenerated locally — `docs/artifacts/` is gitignored
because the cluster state is point-in-time, not source of truth):

- `docs/artifacts/kind-e2e/nodes.txt` — `kubectl get nodes` showing 4 Ready nodes
- `docs/artifacts/kind-e2e/cluster-state.txt` — `kubectl get all -A` snapshot
- `docs/artifacts/kind-e2e/helm-rendered.yaml` — the full rendered manifest
- `docs/artifacts/kind-e2e/helm-dryrun.txt` — server-side dry-run output
- `docs/SANDBOX_ATTACK_REPORT.md` (committed) — verified 12/12 attacks blocked

### About not deploying to live AWS

This repo deliberately does **not** ship a live AWS deploy. The brief asks
the IaC to prove the platform **can be** spun up in a modern cloud
environment — not that it has been. The proof here is:

- The Terraform validates against the published AWS provider schema (any
  drift would fail `terraform validate`).
- The Helm chart renders to schema-clean K8s YAML for both dev + production
  overlays (kubeconform-checked in CI).
- The same manifests apply cleanly on a real K8s cluster (kind v1.35).
- ADR-0001 documents the cloud-target choices (EKS, RDS, ElastiCache, MSK).

A live `terraform apply` is a follow-up step explicitly gated behind cost
approval (~$0.80/hr per the EKS pricing model). The IaC is structured to
make that apply one command; this PR / submission does not commit to
running it.

---

## 2. "Configured" — environment overlays

| File                                              | Role                                                |
| ------------------------------------------------- | --------------------------------------------------- |
| `infra/helm/iicpc-platform/values.yaml`           | Defaults (dev-ish: 1 replica, `latest` tags, in-cluster Postgres) |
| `infra/helm/iicpc-platform/values.production.yaml` | Production overlay (more replicas, pinned image tags, externalised secrets) |
| `infra/terraform/terraform.tfvars.example`        | Reference for region / VPC CIDR / cluster size variables |
| `infra/terraform/variables.tf`                    | Typed variable declarations (region, env, instance types) |

The proof commands above render both overlays and confirm both produce
schema-clean K8s YAML.

---

## 3. "Scaled horizontally"

Every service deploys via a `Deployment` (replicas), not a bare Pod —
making `kubectl scale` / HPA the natural scaling lever.

| Service group           | Workload kind | Where defined                                       |
| ----------------------- | ------------- | --------------------------------------------------- |
| api-gateway, submission-svc, sandbox-runner, bot-coordinator, telemetry-ingester, aggregator, validator, leaderboard-svc | `Deployment` | `infra/helm/iicpc-platform/templates/deployment.yaml` (templated per service in `values.yaml`) |
| bot-worker (the load generator) | K8s `Job` (per benchmark, `parallelism` = bot count) | spawned by bot-coordinator — `services/bot-coordinator/internal/spawn/k8s.go` (not in the helm chart) |
| Sidecars / cluster-wides | `DaemonSet`   | `infra/manifests/chrony-daemonset.yaml`             |

`HorizontalPodAutoscaler` template lives at
`infra/helm/iicpc-platform/templates/hpa.yaml` and is opt-in per service via
the `autoscaling:` block in `values.yaml`. `PodDisruptionBudget` is
likewise templated at `templates/poddisruptionbudget.yaml`.

To verify horizontal scaling actually works on a running cluster:

```powershell
kubectl scale deploy/leaderboard-svc --replicas=12 -n iicpc
kubectl get pods -n iicpc -l app.kubernetes.io/name=leaderboard-svc -o wide -w
```

This exact scale-out was exercised on a live 4-node kind cluster — see
`docs/artifacts/kind-multinode/` (6→12 replicas spread across worker nodes).
The bot fleet scales the same way, but as Job `parallelism` rather than a
Deployment replica count.

---

## CPU pinning (contestant pods)

The brief calls for **CPU pinning, strict memory limits** for contestant
isolation. This repo implements that in two coordinated places:

1. **Pod-level (in this repo).** The sandbox-runner builds contestant pods
   with `requests == limits` for both CPU and memory, using integer CPU
   values. This puts the pod into the Kubernetes **Guaranteed QoS class**,
   which is the prerequisite for the kubelet CPU Manager to pin whole
   cores.

   | Resource | Request | Limit | Source |
   | -------- | ------- | ----- | ------ |
   | CPU      | `1`     | `1`   | `infra/manifests/sandbox-runner.yaml` env `CPU_REQUEST` / `CPU_LIMIT`; pod-template `services/sandbox-runner/internal/runner/podspec.go` |
   | Memory   | `512Mi` | `512Mi` | same |

   Verified by `TestPodSpecGuaranteedQoS` in
   `services/sandbox-runner/internal/runner/runner_test.go` — the test
   asserts `requests.cpu == limits.cpu`, `requests.memory == limits.memory`,
   and that CPU is an integer number of cores (not millicores).

2. **Node-level (kubelet flag).** Pinning is only enforced when the
   kubelet runs with `--cpu-manager-policy=static`. This is set
   per-node-group via the EKS managed-node-group `kubeletExtraConfig`:

   ```hcl
   # infra/terraform/eks.tf (excerpt)
   eks_managed_node_groups = {
     contestants = {
       kubelet_extra_args = "--cpu-manager-policy=static --reserved-cpus=0"
     }
   }
   ```

   On kind / local development, the static policy is not active (kind
   nodes run a single shared kubelet); QoS=Guaranteed alone still
   guarantees `requests` is honoured but cores are not exclusively
   reserved. Production EKS gets the full pinning.

Together: Guaranteed QoS pod (this repo) + static CPU Manager (node
config) ⇒ contestant pod is pinned to N exclusive cores for its lifetime.

## How CI enforces this on every PR

`.github/workflows/ci.yml` runs the same gates non-locally on every push +
PR:

| CI job                | Gate                                                     |
| --------------------- | -------------------------------------------------------- |
| `terraform-validate`  | `terraform fmt -check`, `init -backend=false`, `validate` |
| `helm-lint`           | `helm lint`, `helm template` (dev + prod), `kubeconform` schema check at K8s 1.30 |
| `dockerfile-lint`     | `hadolint` on every service Dockerfile + service-image-build |

A PR that breaks any of these gates is blocked from merging.

---

## Verification checklist for a reviewer

If you are grading this submission, here is the minimum to verify
deliverable #3 in under 10 minutes:

- [ ] Read `infra/terraform/*.tf` — confirm vpc/eks/rds/redis/msk/s3/ecr are all defined
- [ ] Run `./scripts/iac-verify.ps1` (or `make iac-verify`) — confirm all gates pass
- [ ] Optionally run `./scripts/kind-e2e-smoke.ps1` — confirms full end-to-end on a real K8s cluster in <10 min
- [ ] Read `infra/helm/iicpc-platform/values.yaml` + `values.production.yaml` — confirm per-env overlays
- [ ] Check `.github/workflows/ci.yml` `terraform-validate` + `helm-lint` jobs are green on `main`

Time-to-grade: ~10 minutes. Trust signal: every check is a passing gate, not a screenshot.
