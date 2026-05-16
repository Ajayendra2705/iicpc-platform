resource "aws_security_group" "msk" {
  name        = "${var.cluster_name}-msk"
  description = "MSK Serverless access from EKS node groups (SASL/IAM)."
  vpc_id      = module.vpc.vpc_id
}

resource "aws_vpc_security_group_ingress_rule" "msk_from_nodes" {
  security_group_id            = aws_security_group.msk.id
  referenced_security_group_id = module.eks.node_security_group_id
  ip_protocol                  = "tcp"
  from_port                    = 9098
  to_port                      = 9098
  description                  = "Kafka SASL/IAM port"
}

# MSK Serverless: pay-per-use, no broker sizing. Telemetry topic is single-tenant.
resource "aws_msk_serverless_cluster" "telemetry" {
  cluster_name = "${var.cluster_name}-telemetry"

  vpc_config {
    subnet_ids         = module.vpc.private_subnets
    security_group_ids = [aws_security_group.msk.id]
  }

  client_authentication {
    sasl {
      iam {
        enabled = true
      }
    }
  }
}
