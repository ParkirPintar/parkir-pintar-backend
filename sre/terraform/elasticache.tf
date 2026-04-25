resource "aws_elasticache_subnet_group" "parkirpintar" {
  name       = "parkirpintar"
  subnet_ids = [aws_subnet.private_ap_southeast_1a.id, aws_subnet.private_ap_southeast_1b.id, aws_subnet.public_ap_southeast_1a.id, aws_subnet.public_ap_southeast_1b.id]
}

resource "aws_elasticache_cluster" "parkirpintar" {
  cluster_id           = "parkirpintar"
  engine               = "redis"
  engine_version       = "7.0"
  node_type            = "cache.t4g.small"
  num_cache_nodes      = 1
  parameter_group_name = "default.redis7"
  port                 = 6379
  subnet_group_name    = aws_elasticache_subnet_group.parkirpintar.name
  security_group_ids   = [aws_security_group.parkirpintar.id]

  tags = {
    App = "parkirpintar"
  }
}
