# ---------------------------------------------------------------------------
# Self-managed RabbitMQ on EC2 (replaces Amazon MQ — saves ~$200/month)
# ---------------------------------------------------------------------------
resource "aws_instance" "rabbitmq" {
  ami                    = "ami-045b2b22162cf72ce" # Same AMI as observability instance
  instance_type          = "t3.small"
  subnet_id              = aws_subnet.private_ap_southeast_3a.id
  vpc_security_group_ids = [aws_security_group.parkirpintar.id]

  user_data = <<-EOF
    #!/bin/bash
    set -e

    # Install RabbitMQ 3.13 on Amazon Linux 2
    yum install -y curl socat logrotate

    # Install Erlang from Erlang Solutions RPM
    yum install -y https://github.com/rabbitmq/erlang-rpm/releases/download/v26.2.5.6/erlang-26.2.5.6-1.el7.x86_64.rpm

    # Install RabbitMQ from GitHub releases
    yum install -y https://github.com/rabbitmq/rabbitmq-server/releases/download/v3.13.7/rabbitmq-server-3.13.7-1.el8.noarch.rpm

    # Start RabbitMQ
    systemctl start rabbitmq-server
    systemctl enable rabbitmq-server

    # Enable management plugin + consistent hash exchange + prometheus
    rabbitmq-plugins enable rabbitmq_management
    rabbitmq-plugins enable rabbitmq_consistent_hash_exchange
    rabbitmq-plugins enable rabbitmq_prometheus

    # Create admin user
    rabbitmqctl add_user ${var.mq_username} ${var.mq_password}
    rabbitmqctl set_user_tags ${var.mq_username} administrator
    rabbitmqctl set_permissions -p / ${var.mq_username} ".*" ".*" ".*"

    # Remove default guest user
    rabbitmqctl delete_user guest

    # Restart to apply
    systemctl restart rabbitmq-server
  EOF

  tags = {
    Name = "parkirpintar-rabbitmq"
    App  = "parkirpintar"
  }
}


