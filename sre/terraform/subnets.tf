resource "aws_subnet" "private_ap_southeast_3a" {
  vpc_id            = aws_vpc.parkirpintar.id
  cidr_block        = "10.0.0.0/19"
  availability_zone = "ap-southeast-3a"

  tags = {
    "Name"                             = "private_ap_southeast_3a"
    "kubernetes.io/cluster/parkirpintar" = "shared"
    "kubernetes.io/role/internal-elb"  = "1"
  }

}

resource "aws_subnet" "private_ap_southeast_3b" {
  vpc_id            = aws_vpc.parkirpintar.id
  cidr_block        = "10.0.32.0/19"
  availability_zone = "ap-southeast-3b"

  tags = {
    "Name"                             = "private_ap_southeast_3b"
    "kubernetes.io/cluster/parkirpintar" = "shared"
    "kubernetes.io/role/internal-elb"  = "1"
  }

}


resource "aws_subnet" "public_ap_southeast_3a" {
  vpc_id                          = aws_vpc.parkirpintar.id
  cidr_block                      = "10.0.64.0/19"
  availability_zone               = "ap-southeast-3a"

  tags = {
    "Name"                             = "public_ap_southeast_3a"
    "kubernetes.io/cluster/parkirpintar" = "owned"
    "kubernetes.io/role/elb"           = "1"
  }
}


resource "aws_subnet" "public_ap_southeast_3b" {
  vpc_id                          = aws_vpc.parkirpintar.id
  cidr_block                      = "10.0.96.0/19"
  availability_zone               = "ap-southeast-3b"

  tags = {
    "Name"                             = "public_ap_southeast_3b"
    "kubernetes.io/cluster/parkirpintar" = "owned"
    "kubernetes.io/role/elb"           = "1"
  }
}
