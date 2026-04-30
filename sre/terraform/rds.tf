resource "aws_db_subnet_group" "parkirpintar" {
  name        = "parkirpintar"
  description = "parkirpintar private subnets"
  subnet_ids  = [aws_subnet.private_ap_southeast_1a.id, aws_subnet.private_ap_southeast_1b.id]
}

# --- Reservation DB ---
resource "aws_db_instance" "reservation" {
  identifier             = "parkirpintar-reservation"
  allocated_storage      = 20
  storage_type           = "gp3"
  engine                 = "postgres"
  engine_version         = "16.6"
  instance_class         = "db.t3.micro"
  db_name                = "reservation"
  username               = var.db_username
  password               = var.db_password
  parameter_group_name   = "default.postgres16"
  publicly_accessible    = false
  skip_final_snapshot    = true
  backup_retention_period = 7
  vpc_security_group_ids = [aws_security_group.parkirpintar.id]
  db_subnet_group_name   = aws_db_subnet_group.parkirpintar.name

  tags = { Name = "parkirpintar-reservation" }
}

# --- Read Replica (Search + Reservation reads) ---
resource "aws_db_instance" "reservation_replica" {
  identifier             = "parkirpintar-reservation-replica"
  replicate_source_db    = aws_db_instance.reservation.identifier
  instance_class         = "db.t3.micro"
  publicly_accessible    = false
  skip_final_snapshot    = true
  backup_retention_period = 0

  tags = { Name = "parkirpintar-reservation-replica" }
}

# --- Billing DB ---
resource "aws_db_instance" "billing" {
  identifier             = "parkirpintar-billing"
  allocated_storage      = 20
  storage_type           = "gp3"
  engine                 = "postgres"
  engine_version         = "16.6"
  instance_class         = "db.t3.micro"
  db_name                = "billing"
  username               = var.db_username
  password               = var.db_password
  parameter_group_name   = "default.postgres16"
  publicly_accessible    = false
  skip_final_snapshot    = true
  vpc_security_group_ids = [aws_security_group.parkirpintar.id]
  db_subnet_group_name   = aws_db_subnet_group.parkirpintar.name

  tags = { Name = "parkirpintar-billing" }
}

# --- Payment DB ---
resource "aws_db_instance" "payment" {
  identifier             = "parkirpintar-payment"
  allocated_storage      = 20
  storage_type           = "gp3"
  engine                 = "postgres"
  engine_version         = "16.6"
  instance_class         = "db.t3.micro"
  db_name                = "payment"
  username               = var.db_username
  password               = var.db_password
  parameter_group_name   = "default.postgres16"
  publicly_accessible    = false
  skip_final_snapshot    = true
  vpc_security_group_ids = [aws_security_group.parkirpintar.id]
  db_subnet_group_name   = aws_db_subnet_group.parkirpintar.name

  tags = { Name = "parkirpintar-payment" }
}

# --- Analytics DB ---
resource "aws_db_instance" "analytics" {
  identifier             = "parkirpintar-analytics"
  allocated_storage      = 20
  storage_type           = "gp3"
  engine                 = "postgres"
  engine_version         = "16.6"
  instance_class         = "db.t3.micro"
  db_name                = "analytics"
  username               = var.db_username
  password               = var.db_password
  parameter_group_name   = "default.postgres16"
  publicly_accessible    = false
  skip_final_snapshot    = true
  vpc_security_group_ids = [aws_security_group.parkirpintar.id]
  db_subnet_group_name   = aws_db_subnet_group.parkirpintar.name

  tags = { Name = "parkirpintar-analytics" }
}
