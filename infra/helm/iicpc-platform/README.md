# iicpc-platform — Helm chart

Single umbrella chart that deploys all 9 backend services + web UI into one
restricted-PSA namespace. `bot-worker` is **not** in this chart — it's
spawned as a per-benchmark K8s Job by `bot-coordinator`.

## What it provisions per service

- **Deployment** with PSA-`restricted`-compliant securityContext (non-root,
  read-only rootfs, drop ALL caps, seccomp RuntimeDefault), tmp emptyDir
- **Service** (ClusterIP)
- **ServiceAccount** (one per service — IRSA hooks slot in here later)
- **HorizontalPodAutoscaler** targeting `CPU 80%` (configurable)
- **PodDisruptionBudget** with `minAvailable: 1`

## What it provisions cluster-wide (once)

- **Namespace** `iicpc` with PodSecurityAdmission enforce=restricted
- **NetworkPolicies**:
  - `default-deny` — denies all ingress + egress in the namespace
  - `allow-same-namespace` — services can reach each other
  - `allow-dns-egress` — UDP/TCP 53 to kube-dns
  - `allow-same-namespace-egress` — services can call peers
  - `allow-aws-egress` — only data-plane services (submission-svc,
    telemetry-ingester, aggregator, validator, leaderboard-svc) can reach
    AWS APIs (S3 443 / RDS 5432 / Redis 6379 / MSK 9098), and **IMDS
    169.254.169.254 is explicitly blocked**

## Install

```bash
# kind / dev (uses localhost:5000 registry)
helm upgrade --install iicpc ./infra/helm/iicpc-platform \
  --create-namespace -n iicpc

# AWS production (uses ECR images)
helm upgrade --install iicpc ./infra/helm/iicpc-platform \
  -f ./infra/helm/iicpc-platform/values.yaml \
  -f ./infra/helm/iicpc-platform/values.production.yaml \
  --set global.imageRegistry="$(terraform -chdir=infra/terraform output -raw cluster_name)..." \
  --set global.imageTag="v0.1.0" \
  -n iicpc --create-namespace
```

## Validation

```bash
helm lint ./infra/helm/iicpc-platform
helm template iicpc ./infra/helm/iicpc-platform | kubectl apply --dry-run=client -f -
```

Both commands run on every push via `.github/workflows/ci.yml`.

## Values structure

`values.yaml` ships dev-friendly defaults. `values.production.yaml` is the
overlay applied on top during AWS deploys (raises replicas, bumps resource
requests, sets ECR image registry placeholder).

Each entry under `services:` becomes one Deployment + Service + HPA + PDB
+ ServiceAccount. Per-service overrides win over `defaults:`. Set
`disabled: true` on any service to skip it for a release.

## Not yet wired (D27+ work)

- **IRSA** — `ServiceAccount` annotations pointing to IAM role ARNs
  (per-service AWS permissions)
- **Secrets** — Postgres DSN / Redis auth / MSK credentials currently
  must be supplied via separate `Secret` manifests; planned move to
  external-secrets-operator pulling from AWS Secrets Manager
- **Ingress** — `expose: true` services need an `Ingress` resource (or
  Gateway API); blocked on choosing ingress class (ALB controller vs nginx)
- **ConfigMap** — long lists of env vars (Kafka brokers, broker count)
  should move from inline `env:` to mounted ConfigMap entries
