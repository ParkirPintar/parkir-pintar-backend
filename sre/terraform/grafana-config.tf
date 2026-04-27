# ---------------------------------------------------------------------------
# Lookup Istio Ingress Gateway Classic ELB
# ---------------------------------------------------------------------------
data "aws_elb" "istio_ingress" {
  count = var.istio_elb_name != "" ? 1 : 0
  name  = var.istio_elb_name
}

locals {
  istio_elb_hostname = var.istio_elb_name != "" ? data.aws_elb.istio_ingress[0].dns_name : ""
}

provider "kubernetes" {
  host                   = aws_eks_cluster.parkirpintar.endpoint
  cluster_ca_certificate = base64decode(aws_eks_cluster.parkirpintar.certificate_authority[0].data)
  exec {
    api_version = "client.authentication.k8s.io/v1beta1"
    command     = "aws"
    args        = ["eks", "get-token", "--cluster-name", aws_eks_cluster.parkirpintar.name]
  }
}

# ---------------------------------------------------------------------------
# Namespace — monitoring (created automatically so ConfigMap can land)
# ---------------------------------------------------------------------------
resource "kubernetes_namespace" "monitoring" {
  metadata {
    name = "monitoring"
  }

  depends_on = [aws_eks_node_group.parkirpintar]
}

# ---------------------------------------------------------------------------
# ConfigMap — inject ELB hostname into Grafana env
# ---------------------------------------------------------------------------
resource "kubernetes_config_map" "grafana_env" {
  metadata {
    name      = "grafana-env"
    namespace = kubernetes_namespace.monitoring.metadata[0].name
  }

  data = {
    GF_SERVER_ROOT_URL            = "http://${local.istio_elb_hostname}/monitor"
    GF_SERVER_SERVE_FROM_SUB_PATH = "true"
  }

  depends_on = [kubernetes_namespace.monitoring]
}
