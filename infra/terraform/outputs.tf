output "cluster_name" {
  description = "EKS cluster name (use to configure kubectl)."
  value       = module.eks.cluster_name
}

output "cluster_endpoint" {
  description = "EKS API server URL."
  value       = module.eks.cluster_endpoint
}

output "cluster_certificate" {
  description = "Base64-encoded cluster CA certificate."
  value       = module.eks.cluster_certificate_authority_data
  sensitive   = true
}

output "vpc_id" {
  value = module.vpc.vpc_id
}

output "private_subnet_ids" {
  value = module.vpc.private_subnets
}

output "rds_endpoint" {
  description = "Postgres endpoint host:port."
  value       = aws_db_instance.telemetry.endpoint
  sensitive   = true
}

output "redis_endpoint" {
  description = "Redis primary endpoint."
  value       = aws_elasticache_replication_group.leaderboard.primary_endpoint_address
}

output "msk_bootstrap_brokers" {
  description = "MSK Serverless SASL/IAM bootstrap brokers."
  value       = aws_msk_serverless_cluster.telemetry.bootstrap_brokers_sasl_iam
}

output "submissions_bucket" {
  description = "S3 bucket for contestant source archives."
  value       = aws_s3_bucket.submissions.id
}

output "ecr_repos" {
  description = "Map of service name → ECR repository URL."
  value = {
    for k, v in aws_ecr_repository.service : k => v.repository_url
  }
}

output "configure_kubectl" {
  description = "Command to populate ~/.kube/config for this cluster."
  value       = "aws eks update-kubeconfig --region ${var.region} --name ${module.eks.cluster_name}"
}
