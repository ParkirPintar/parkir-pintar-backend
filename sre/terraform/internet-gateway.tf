resource "aws_internet_gateway" "parkirpintar" {
  vpc_id = aws_vpc.parkirpintar.id

  tags = {
    Name = "parkirpintar"
  }

}
