output "eks_cluster_name" {
  value = aws_eks_cluster.parkirpintar.name
}

output "eks_cluster_endpoint" {
  value = aws_eks_cluster.parkirpintar.endpoint
}

output "rds_user_endpoint" {
  value = aws_db_instance.user.endpoint
}

output "rds_reservation_endpoint" {
  value = aws_db_instance.reservation.endpoint
}

output "rds_reservation_replica_endpoint" {
  value = aws_db_instance.reservation_replica.endpoint
}

output "rds_billing_endpoint" {
  value = aws_db_instance.billing.endpoint
}

output "rds_payment_endpoint" {
  value = aws_db_instance.payment.endpoint
}

output "rds_analytics_endpoint" {
  value = aws_db_instance.analytics.endpoint
}

output "redis_endpoint" {
  value = aws_elasticache_cluster.parkirpintar.cache_nodes[0].address
}

output "rabbitmq_endpoint" {
  value = aws_mq_broker.parkirpintar.instances[0].endpoints
}

# ---------------------------------------------------------------------------
# GitHub Actions — set these as GitHub repository secrets
# ---------------------------------------------------------------------------
output "github_actions_role_arn" {
  description = "Set as GitHub secret: AWS_ROLE_ARN"
  value       = aws_iam_role.github_actions.arn
}

output "ecr_registry" {
  description = "ECR registry URL (account-id.dkr.ecr.region.amazonaws.com)"
  value       = "${data.aws_caller_identity.current.account_id}.dkr.ecr.ap-southeast-1.amazonaws.com"
}

output "ecr_repository_urls" {
  description = "ECR repository URLs per service"
  value       = { for k, v in aws_ecr_repository.services : k => v.repository_url }
}

output "ingress_hostname" {
  description = "Istio Ingress Gateway ELB hostname"
  value       = local.istio_elb_hostname
}
