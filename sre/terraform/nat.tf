resource "aws_eip" "parkirpintar" {
  vpc = true

  tags = {
    Name = "parkirpintar"
  }
}

resource "aws_nat_gateway" "parkirpintar" {
  allocation_id = aws_eip.parkirpintar.id
  subnet_id     = aws_subnet.public_ap_southeast_1a.id

  tags = {
    Name = "parkirpintar"
  }

  depends_on = [aws_internet_gateway.parkirpintar]
}
