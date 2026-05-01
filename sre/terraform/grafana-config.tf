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

# ---------------------------------------------------------------------------
# Namespace — monitoring (created automatically so ConfigMap can land)
# ---------------------------------------------------------------------------
resource "kubernetes_namespace" "monitoring" {
  metadata {
    name = "monitoring"
  }

  lifecycle {
    ignore_changes = all
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

  lifecycle {
    ignore_changes = all
  }

  depends_on = [kubernetes_namespace.monitoring]
}
