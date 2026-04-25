resource "aws_vpc" "parkirpintar" {
  # CIDR block for the VPC
  cidr_block = "10.0.0.0/16"

  # instances shared on the host
  instance_tenancy = "default"

  # Required for EKS. This enables DNS resolution for the VPC
  enable_dns_support = true

  # Required for EKS. This enables DNS hostname for the VPC
  enable_dns_hostnames = true

  assign_generated_ipv6_cidr_block = false

  tags = {
    Name = "parkirpintar"
  }
}
