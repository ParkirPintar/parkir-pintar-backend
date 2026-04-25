resource "aws_mq_broker" "parkirpintar" {
  broker_name                = "parkirpintar"
  engine_type                = "RabbitMQ"
  engine_version             = "3.13"
  host_instance_type         = "mq.m5.large"
  publicly_accessible        = false
  auto_minor_version_upgrade = true
  deployment_mode            = "SINGLE_INSTANCE"
  subnet_ids                 = [aws_subnet.private_ap_southeast_1a.id]
  security_groups            = [aws_security_group.parkirpintar.id]

  tags = { App = "parkirpintar" }

  user {
    username = var.mq_username
    password = var.mq_password
  }
}
