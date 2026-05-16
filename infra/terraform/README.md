# IICPC Platform — Terraform (AWS)

Provisions the production stack: VPC, EKS, RDS Postgres + TimescaleDB,
ElastiCache Redis, MSK Serverless, S3, ECR. Designed for `ap-south-1` by
default but region-agnostic.

## Architecture

```
                       ┌──────────────────────────┐
                       │   internet / api gateway │
                       └──────────────┬───────────┘
                                      │
        ┌─────────────────────────────▼─────────────────────────────┐
        │                       VPC (10.0.0.0/16)                   │
        │  ┌──────────┐  ┌──────────┐  ┌──────────┐  3 AZs          │
        │  │ public-a │  │ public-b │  │ public-c │  → NAT GW       │
        │  └────┬─────┘  └────┬─────┘  └────┬─────┘                 │
        │  ┌────▼──────────────▼──────────────▼────┐                │
        │  │  private-{a,b,c}: EKS / RDS / Redis  │                 │
        │  └───────────────────────────────────────┘                │
        └────┬───────┬───────┬───────┬───────┬─────────────────────┘
             │       │       │       │       │
            EKS     RDS    Redis    MSK     S3 + ECR
            │       │       │       │
            └─services + contestants + bots node pools
```

## Modules

| File | Provisions |
|---|---|
| `vpc.tf` | VPC, 3-AZ public + private subnets, NAT, route tables (terraform-aws-modules/vpc/aws) |
| `eks.tf` | EKS control plane, 3 managed node pools (`services` / `contestants` taints / `bots`), addons (coredns, vpc-cni, ebs-csi) |
| `rds.tf` | Postgres 16 + TimescaleDB extension (via `shared_preload_libraries`), parameter group, SG, monitoring role |
| `redis.tf` | ElastiCache 7.1 replication group, at-rest + transit encryption |
| `msk.tf` | MSK Serverless cluster with SASL/IAM auth |
| `s3.tf` | Submissions bucket: versioned, encrypted, public-access blocked, 30d noncurrent expiration |
| `ecr.tf` | One repo per service (10 total), scan-on-push, lifecycle policy keeps last 20 tagged + expires untagged after 7d |
| `outputs.tf` | Endpoints, kubectl bootstrap command, ECR repo URLs |

## Prerequisites

- Terraform ≥ 1.5
- AWS CLI v2 configured with credentials that have `AdministratorAccess`
  (narrow this for production via IAM)
- kubectl ≥ 1.28
- ~$60–120/day for the staging defaults if left running 24h. **Always
  `terraform destroy` after testing.**

## Usage

```bash
cd infra/terraform

# 1. Set secrets out-of-band (NOT in tfvars committed to git)
export TF_VAR_db_password="$(openssl rand -base64 24)"

# 2. Optional: copy example tfvars for non-secret overrides
cp terraform.tfvars.example terraform.tfvars
# edit region / environment / cluster_name

# 3. Initialise providers + modules
terraform init

# 4. Preview
terraform plan -out=tfplan

# 5. Apply (15–20 min for EKS + RDS)
terraform apply tfplan

# 6. Configure kubectl
$(terraform output -raw configure_kubectl)
kubectl get nodes
```

## State backend

This config uses **local state** by default — fine for hackathon use.
For team / production, add a backend block:

```hcl
terraform {
  backend "s3" {
    bucket         = "iicpc-tfstate"
    key            = "platform/terraform.tfstate"
    region         = "ap-south-1"
    dynamodb_table = "iicpc-tflocks"
    encrypt        = true
  }
}
```

## Cost ballpark (staging defaults, ap-south-1, on-demand)

| Resource | $/day |
|---|---|
| EKS control plane | $2.40 |
| 3× m6g.large nodes (services pool) | ~$5 |
| 2× c6g.large (contestants) | ~$2.50 |
| 2× c6g.xlarge (bots) | ~$5 |
| 1× NAT gateway | ~$1.10 |
| RDS db.t4g.medium + 100 GB gp3 | ~$2.30 |
| ElastiCache cache.t4g.small single-node | ~$0.50 |
| MSK Serverless idle | ~$0 (pay per use) |
| S3 + ECR | pennies |
| **~$19/day** | |

Production with `environment = "production"` adds multi-AZ RDS, second
ElastiCache replica, and 3-AZ NAT gateways → ~$45–60/day.

## Notes

- **Graviton everywhere.** Node groups use `AL2023_ARM_64_STANDARD`. Build
  service images with `--platform linux/arm64` in CI.
- **TimescaleDB** is loaded via `shared_preload_libraries` in the parameter
  group, but the extension itself is created by the migration in
  `infra/timescaledb/migrations/001_telemetry_schema.sql`. Run it once
  after RDS is provisioned.
- **No IRSA wiring yet.** Service-account → IAM-role mappings come in D26
  alongside the Helm charts.
- **`db_password` is required.** No default — fail-fast over committing a
  placeholder secret.
