resource "aws_route_table" "private_parkirpintar" {
  vpc_id = aws_vpc.parkirpintar.id
  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.parkirpintar.id
  }
  tags = {
    Name = "private_parkirpintar"
  }
}

resource "aws_route_table" "public_parkirpintar" {
  vpc_id = aws_vpc.parkirpintar.id
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.parkirpintar.id
  }
  tags = {
    Name = "public_parkirpintar"
  }
}

resource "aws_route_table_association" "private_ap_southeast_1a_parkirpintar" {
  subnet_id      = aws_subnet.private_ap_southeast_1a.id
  route_table_id = aws_route_table.private_parkirpintar.id
}

resource "aws_route_table_association" "private_ap_southeast_1b_parkirpintar" {
  subnet_id      = aws_subnet.private_ap_southeast_1b.id
  route_table_id = aws_route_table.private_parkirpintar.id
}

resource "aws_route_table_association" "public_ap_southeast_1a_parkirpintar" {
  subnet_id      = aws_subnet.public_ap_southeast_1a.id
  route_table_id = aws_route_table.public_parkirpintar.id
}

resource "aws_route_table_association" "public_ap_southeast_1b_parkirpintar" {
  subnet_id      = aws_subnet.public_ap_southeast_1b.id
  route_table_id = aws_route_table.public_parkirpintar.id
}



