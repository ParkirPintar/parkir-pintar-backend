resource "aws_iam_role" "nodes_parkirpintar" {
  name = "nodes_parkirpintar"
  assume_role_policy = jsonencode({
    Version = "2012-10-17",
    Statement = [
      {
        Effect = "Allow",
        Principal = {
          Service = "ec2.amazonaws.com"
        },
        Action = "sts:AssumeRole"
      }
    ]
  })
}


resource "aws_iam_role_policy_attachment" "nodes_amazon_eks_worker_node_policy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"
  role       = aws_iam_role.nodes_parkirpintar.name
}

resource "aws_iam_role_policy_attachment" "nodes_amazon_eks_cni_policy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy"
  role       = aws_iam_role.nodes_parkirpintar.name
}

resource "aws_iam_role_policy_attachment" "nodes_amazon_ec2_container_registry_read_only" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
  role       = aws_iam_role.nodes_parkirpintar.name
}

resource "aws_iam_role_policy_attachment" "nodes_amazon_ebs_csi_driver" {
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonEBSCSIDriverPolicy"
  role       = aws_iam_role.nodes_parkirpintar.name
}

resource "aws_iam_role_policy_attachment" "amazon_ssm_managed_instance_core" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
  role       = aws_iam_role.nodes_parkirpintar.name
}


resource "aws_eks_node_group" "parkirpintar" {
  cluster_name    = aws_eks_cluster.parkirpintar.name
  node_group_name = "parkirpintar"
  node_role_arn   = aws_iam_role.nodes_parkirpintar.arn

  subnet_ids = [
    aws_subnet.private_ap_southeast_1a.id,
    aws_subnet.private_ap_southeast_1b.id
  ]


  capacity_type  = "ON_DEMAND"
  instance_types = ["t3.medium"] # "t3.medium" is the default instance type

  scaling_config {
    desired_size = 4
    max_size     = 7
    min_size     = 3
  }

  update_config {
    max_unavailable = 1
  }

  labels = {
    role = "general"
  }


  depends_on = [
    aws_iam_role_policy_attachment.nodes_amazon_eks_worker_node_policy,
    aws_iam_role_policy_attachment.nodes_amazon_eks_cni_policy,
    aws_iam_role_policy_attachment.nodes_amazon_ec2_container_registry_read_only,
  ]
}
