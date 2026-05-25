resource "aws_instance" "parkirpintar-observability" {
  ami                         = "ami-045b2b22162cf72ce"
  instance_type               = "t3.medium"
  subnet_id                   = aws_subnet.public_ap_southeast_3a.id
  vpc_security_group_ids      = [aws_security_group.parkirpintar.id]
  associate_public_ip_address = true

  tags = {
    Name = "parkirpintar-observability"
  }
}
