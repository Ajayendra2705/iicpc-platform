# EKS staging smoke — D28 runbook

End-to-end recipe to provision AWS infra, build + push every image, deploy
the stack, run a benchmark, capture metrics, and **tear it all down before
the bill grows**.

Estimated wall-clock: ~45 min provision → 10 min benchmark → ~15 min destroy.
Estimated cost: **~$0.80** if you destroy promptly (under 1 hour active).

## 0. Prereqs

| Tool | Version | Why |
|---|---|---|
| AWS CLI v2 | ≥ 2.15 | terraform + ECR login |
| terraform | ≥ 1.15 | infra/terraform |
| kubectl | ≥ 1.30 | EKS access |
| helm | ≥ 3.16 | chart install |
| docker buildx | latest | multi-arch builds |
| Node ≥ 20 | for `web` builds | |
| AWS account with `AdministratorAccess` for the IAM principal | first run only | |

Set credentials:
```powershell
$env:AWS_PROFILE = "iicpc-staging"
$env:AWS_REGION  = "ap-south-1"
$env:TF_VAR_db_password = (-join ((48..57) + (65..90) + (97..122) | Get-Random -Count 32 | ForEach-Object {[char]$_}))
```

## 1. Provision infrastructure (~20 min)

```powershell
cd infra/terraform
terraform init
terraform plan -out=tfplan
terraform apply tfplan       # creates VPC + EKS + RDS + Redis + MSK + S3 + ECR
```

Outputs you'll need next:
```powershell
$REGISTRY = (terraform output -json ecr_repos | ConvertFrom-Json)."submission-svc" -replace '/iicpc/submission-svc$',''
$CLUSTER  = (terraform output -raw cluster_name)
```

## 2. Configure kubectl

```powershell
aws eks update-kubeconfig --region $env:AWS_REGION --name $CLUSTER
kubectl get nodes        # expect 7 nodes across 3 pools
```

## 3. Build + push all images (~10 min, multi-arch)

```powershell
aws ecr get-login-password --region $env:AWS_REGION |
  docker login --username AWS --password-stdin $REGISTRY

cd ..\..\
./scripts/deploy/build-images.ps1 -Registry $REGISTRY -Tag "v0.1.0" -Push
```

This builds 10 images (9 services + web) for `linux/arm64` (Graviton
nodes) and pushes to each per-service ECR repo.

## 4. Apply non-helm K8s baseline

The helm chart deploys the services; one-shots come first.

```powershell
kubectl apply -f infra/manifests/chrony-daemonset.yaml
kubectl apply -f infra/manifests/sandbox-runner.yaml
kubectl apply -f infra/manifests/bot-namespace.yaml   # iicpc-bots ns (PSA) + bot-coordinator RBAC
```

## 5. Run the TimescaleDB migration

```powershell
$RDS_HOST = (terraform -chdir=infra/terraform output -raw rds_endpoint).Split(':')[0]
psql "host=$RDS_HOST user=iicpc password=$env:TF_VAR_db_password dbname=telemetry sslmode=require" `
  -f infra/timescaledb/migrations/001_telemetry_schema.sql
```

(Run from a host in the same VPC or via SSM port-forward — RDS is private.)

## 6. Install the platform chart

```powershell
helm upgrade --install iicpc ./infra/helm/iicpc-platform `
  -n iicpc --create-namespace `
  -f ./infra/helm/iicpc-platform/values.yaml `
  -f ./infra/helm/iicpc-platform/values.production.yaml `
  --set global.imageRegistry=$REGISTRY `
  --set global.imageTag="v0.1.0"

kubectl get pods -n iicpc -w   # wait for all Running
```

## 7. Smoke test

In another shell:
```powershell
./scripts/deploy/smoke-eks.ps1 -ContestantID smoke-1 -NumWorkers 100 -DurationS 60
```

Or open the UI by port-forwarding the web service:
```powershell
kubectl port-forward -n iicpc svc/web 3000:3000
# browser: http://localhost:3000
```

Capture for the demo video:
- `kubectl get pods -n iicpc` showing all replicas Ready
- `kubectl top pods -n iicpc` showing CPU/mem under load
- Leaderboard UI with live scores
- `/contestant/smoke-1` detail page

## 8. Run chaos suite (optional, recommended)

```powershell
./scripts/chaos/run-suite.ps1 -ContestantID smoke-1
```

Capture the score-recovery dip on the UI.

## 9. TEARDOWN — do this before the bill

```powershell
helm uninstall iicpc -n iicpc
kubectl delete ns iicpc iicpc-contestants iicpc-bots --wait=true

cd infra/terraform
terraform destroy -auto-approve
```

Verify everything is gone:
```powershell
aws eks list-clusters
aws rds describe-db-instances --query "DBInstances[].DBInstanceIdentifier"
aws elasticache describe-replication-groups --query "ReplicationGroups[].ReplicationGroupId"
```

## Numbers worth recording for the demo (D29–31)

- Cold-start time per service (first `Running` minus `Pending`)
- End-to-end latency: bot send → telemetry → aggregator window → leaderboard WS → browser
- Sustained TPS at 100 / 500 / 1000 workers
- Score recovery time after chaos scenario 2 ends

## Pitfalls

- Forgetting to `terraform destroy` — accumulates ~$60/day for production
  defaults. Set a calendar reminder.
- ECR auth tokens expire after 12 hours — re-login if pushes start
  401-ing during a long session.
- RDS is in private subnets; the migration step needs a bastion or
  SSM session.
