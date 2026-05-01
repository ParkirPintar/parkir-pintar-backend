provider "aws" {
  region  = "ap-southeast-1"
  profile = "terraform"
}

provider "kubernetes" {
  host                   = try(aws_eks_cluster.parkirpintar.endpoint, "")
  cluster_ca_certificate = try(base64decode(aws_eks_cluster.parkirpintar.certificate_authority[0].data), "")
  exec {
    api_version = "client.authentication.k8s.io/v1beta1"
    command     = "aws"
    args        = ["eks", "get-token", "--cluster-name", try(aws_eks_cluster.parkirpintar.name, "parkirpintar"), "--profile", "terraform"]
  }
}

terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 4.46"
    }
    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.0"
    }
  }
}
