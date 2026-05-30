resource "aws_elasticache_subnet_group" "parkirpintar" {
  name       = "parkirpintar"
  subnet_ids = [aws_subnet.private_ap_southeast_3a.id, aws_subnet.private_ap_southeast_3b.id]
}

resource "aws_elasticache_replication_group" "parkirpintar" {
  replication_group_id = "parkirpintar"
  description          = "ParkirPintar Redis Multi-AZ with automatic failover"

  engine               = "redis"
  engine_version       = "7.0"
  node_type            = "cache.t3.small"
  num_cache_clusters   = 2 # 1 primary (3a) + 1 replica (3b)
  parameter_group_name = "default.redis7"
  port                 = 6379

  automatic_failover_enabled = true
  multi_az_enabled           = true

  subnet_group_name  = aws_elasticache_subnet_group.parkirpintar.name
  security_group_ids = [aws_security_group.parkirpintar.id]

  # Encryption at rest only (transit encryption requires code changes for TLS)
  at_rest_encryption_enabled = true
  transit_encryption_enabled = false

  tags = {
    App = "parkirpintar"
  }
}
