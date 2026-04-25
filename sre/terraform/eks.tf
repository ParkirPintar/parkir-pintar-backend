resource "aws_iam_role" "eks_cluster_parkirpintar" {
  name               = "eks_cluster_parkirpintar"
  assume_role_policy = <<POLICY
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "eks.amazonaws.com"
      },
      "Action": "sts:AssumeRole"
    }
  ]
}
POLICY
}

resource "aws_iam_role_policy_attachment" "eks_cluster_parkirpintar_AmazonEKSClusterPolicy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"
  role       = aws_iam_role.eks_cluster_parkirpintar.name
}

# ---------------------------------------------------------------------------
# OIDC Provider for IRSA
# ---------------------------------------------------------------------------
resource "aws_iam_openid_connect_provider" "eks" {
  url            = aws_eks_cluster.parkirpintar.identity[0].oidc[0].issuer
  client_id_list = ["sts.amazonaws.com"]

  thumbprint_list = [
    data.tls_certificate.eks.certificates[0].sha1_fingerprint
  ]
}

data "tls_certificate" "eks" {
  url = aws_eks_cluster.parkirpintar.identity[0].oidc[0].issuer
}

# ---------------------------------------------------------------------------
# IRSA — EBS CSI Driver
# ---------------------------------------------------------------------------
resource "aws_iam_role" "ebs_csi_driver" {
  name = "parkirpintar-ebs-csi-driver"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Federated = aws_iam_openid_connect_provider.eks.arn
      }
      Action = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringEquals = {
          "${replace(aws_eks_cluster.parkirpintar.identity[0].oidc[0].issuer, "https://", "")}:sub" = "system:serviceaccount:kube-system:ebs-csi-controller-sa"
          "${replace(aws_eks_cluster.parkirpintar.identity[0].oidc[0].issuer, "https://", "")}:aud" = "sts.amazonaws.com"
        }
      }
    }]
  })
}

resource "aws_iam_role_policy_attachment" "ebs_csi_driver" {
  role       = aws_iam_role.ebs_csi_driver.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonEBSCSIDriverPolicy"
}

resource "aws_eks_addon" "ebs_csi_driver" {
  cluster_name             = aws_eks_cluster.parkirpintar.name
  addon_name               = "aws-ebs-csi-driver"
  service_account_role_arn = aws_iam_role.ebs_csi_driver.arn
  resolve_conflicts        = "OVERWRITE"

  depends_on = [
    aws_eks_cluster.parkirpintar,
    aws_iam_role_policy_attachment.ebs_csi_driver,
    aws_iam_openid_connect_provider.eks,
  ]
}

resource "aws_eks_cluster" "parkirpintar" {
  name     = "parkirpintar"
  role_arn = aws_iam_role.eks_cluster_parkirpintar.arn

  vpc_config {
    endpoint_private_access = false
    endpoint_public_access  = true
    subnet_ids = [
      aws_subnet.private_ap_southeast_1a.id,
      aws_subnet.private_ap_southeast_1b.id,
      aws_subnet.public_ap_southeast_1a.id,
      aws_subnet.public_ap_southeast_1b.id
    ]
  }

  depends_on = [
    aws_iam_role_policy_attachment.eks_cluster_parkirpintar_AmazonEKSClusterPolicy
  ]
}
