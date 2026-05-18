variable "region" {
  description = "AWS region for all resources."
  type        = string
  default     = "ap-south-1"
}

variable "environment" {
  description = "Deployment environment (staging | production)."
  type        = string
  default     = "staging"
}

variable "cluster_name" {
  description = "Name prefix used for the EKS cluster and most child resources."
  type        = string
  default     = "iicpc-platform"
}

variable "k8s_version" {
  description = "EKS Kubernetes minor version."
  type        = string
  default     = "1.30"
}

variable "vpc_cidr" {
  description = "VPC CIDR block."
  type        = string
  default     = "10.0.0.0/16"
}

variable "az_suffixes" {
  description = "AZ letter suffixes appended to region for subnet placement."
  type        = list(string)
  default     = ["a", "b", "c"]
}

variable "private_subnet_cidrs" {
  type    = list(string)
  default = ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
}

variable "public_subnet_cidrs" {
  type    = list(string)
  default = ["10.0.101.0/24", "10.0.102.0/24", "10.0.103.0/24"]
}

variable "db_password" {
  description = "Master password for RDS Postgres. Set via TF_VAR_db_password or terraform.tfvars (gitignored)."
  type        = string
  sensitive   = true
}

variable "node_groups" {
  description = "EKS managed node group definitions, keyed by pool name. Set kubelet_extra_args on the contestants pool to enable CPU Manager static policy (CPU pinning)."
  type = map(object({
    instance_types = list(string)
    min_size       = number
    max_size       = number
    desired_size   = number
    labels         = map(string)
    taints = optional(list(object({
      key    = string
      value  = string
      effect = string
    })), [])
    # Extra kubelet flags wired into the bootstrap user-data. The contestants
    # pool uses this to enable the CPU Manager static policy so that Guaranteed
    # QoS pods get whole-core pinning (PS: "CPU pinning").
    kubelet_extra_args = optional(string, "")
  }))

  default = {
    services = {
      instance_types = ["m6g.large"]
      min_size       = 2
      max_size       = 6
      desired_size   = 3
      labels         = { pool = "services" }
    }
    contestants = {
      instance_types = ["c6g.large"]
      min_size       = 0
      max_size       = 20
      desired_size   = 2
      labels         = { pool = "contestants" }
      taints = [{
        key    = "pool"
        value  = "contestants"
        effect = "NO_SCHEDULE"
      }]
      # CPU Manager static policy pins whole cores for Guaranteed QoS pods.
      # --reserved-cpus=0 keeps CPU 0 for system/DaemonSet workloads.
      kubelet_extra_args = "--cpu-manager-policy=static --reserved-cpus=0"
    }
    bots = {
      instance_types = ["c6g.xlarge"]
      min_size       = 0
      max_size       = 10
      desired_size   = 2
      labels         = { pool = "bots" }
    }
  }
}
