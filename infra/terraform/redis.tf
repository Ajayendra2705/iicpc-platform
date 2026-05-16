resource "aws_elasticache_subnet_group" "leaderboard" {
  name       = "${var.cluster_name}-leaderboard"
  subnet_ids = module.vpc.private_subnets
}

resource "aws_security_group" "redis" {
  name        = "${var.cluster_name}-redis"
  description = "Redis access from EKS node groups."
  vpc_id      = module.vpc.vpc_id
}

resource "aws_vpc_security_group_ingress_rule" "redis_from_nodes" {
  security_group_id            = aws_security_group.redis.id
  referenced_security_group_id = module.eks.node_security_group_id
  ip_protocol                  = "tcp"
  from_port                    = 6379
  to_port                      = 6379
  description                  = "Redis from EKS nodes"
}

resource "aws_elasticache_replication_group" "leaderboard" {
  replication_group_id = "${var.cluster_name}-leaderboard"
  description          = "ZSET-backed leaderboard"

  node_type            = "cache.t4g.small"
  num_cache_clusters   = var.environment == "production" ? 2 : 1
  parameter_group_name = "default.redis7"
  engine_version       = "7.1"
  port                 = 6379

  subnet_group_name  = aws_elasticache_subnet_group.leaderboard.name
  security_group_ids = [aws_security_group.redis.id]

  automatic_failover_enabled = var.environment == "production"
  at_rest_encryption_enabled = true
  transit_encryption_enabled = true

  apply_immediately = var.environment != "production"
}
