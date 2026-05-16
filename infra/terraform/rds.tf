resource "aws_db_subnet_group" "telemetry" {
  name       = "${var.cluster_name}-telemetry"
  subnet_ids = module.vpc.private_subnets
}

resource "aws_security_group" "rds" {
  name        = "${var.cluster_name}-rds"
  description = "Postgres access from EKS node groups."
  vpc_id      = module.vpc.vpc_id
}

resource "aws_vpc_security_group_ingress_rule" "rds_from_nodes" {
  security_group_id            = aws_security_group.rds.id
  referenced_security_group_id = module.eks.node_security_group_id
  ip_protocol                  = "tcp"
  from_port                    = 5432
  to_port                      = 5432
  description                  = "Postgres from EKS nodes"
}

resource "aws_vpc_security_group_egress_rule" "rds_all" {
  security_group_id = aws_security_group.rds.id
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "-1"
}

# TimescaleDB ships as an apt/yum extension; the engine boots with it loaded
# via shared_preload_libraries. The actual CREATE EXTENSION timescaledb is
# run by the migration in infra/timescaledb/migrations/001_telemetry_schema.sql.
resource "aws_db_parameter_group" "timescale" {
  name   = "${var.cluster_name}-pg16-timescale"
  family = "postgres16"

  parameter {
    name         = "shared_preload_libraries"
    value        = "timescaledb"
    apply_method = "pending-reboot"
  }
}

resource "aws_db_instance" "telemetry" {
  identifier = "${var.cluster_name}-telemetry"

  engine               = "postgres"
  engine_version       = "16.4"
  instance_class       = "db.t4g.medium"
  allocated_storage    = 100
  max_allocated_storage = 500
  storage_type         = "gp3"
  storage_encrypted    = true

  db_name  = "telemetry"
  username = "iicpc"
  password = var.db_password

  db_subnet_group_name   = aws_db_subnet_group.telemetry.name
  vpc_security_group_ids = [aws_security_group.rds.id]
  parameter_group_name   = aws_db_parameter_group.timescale.name

  multi_az                = var.environment == "production"
  backup_retention_period = 7
  backup_window           = "02:00-04:00"
  maintenance_window      = "Mon:04:00-Mon:06:00"

  performance_insights_enabled = true
  monitoring_interval          = 60
  monitoring_role_arn          = aws_iam_role.rds_monitoring.arn

  deletion_protection = var.environment == "production"
  skip_final_snapshot = var.environment != "production"
}

resource "aws_iam_role" "rds_monitoring" {
  name = "${var.cluster_name}-rds-monitoring"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action    = "sts:AssumeRole"
      Effect    = "Allow"
      Principal = { Service = "monitoring.rds.amazonaws.com" }
    }]
  })
}

resource "aws_iam_role_policy_attachment" "rds_monitoring" {
  role       = aws_iam_role.rds_monitoring.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonRDSEnhancedMonitoringRole"
}
