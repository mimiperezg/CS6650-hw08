# Regular AZs only. Opted-in Local Zones (e.g. us-west-2-lax-1a) sort ahead
# of us-west-2a and may not offer db.t3.micro.
data "aws_availability_zones" "available" {
  state = "available"

  filter {
    name   = "opt-in-status"
    values = ["opt-in-not-required"]
  }
}

# New subnets in the default VPC would pick up its main route table, which
# routes 0.0.0.0/0 to the internet gateway. This table has only the implicit
# local route, so the DB subnets have no path to or from the internet.
resource "aws_route_table" "private" {
  vpc_id = var.vpc_id

  tags = {
    Name = "${var.service_name}-db-private"
  }
}

resource "aws_subnet" "private" {
  count             = length(var.private_subnet_cidrs)
  vpc_id            = var.vpc_id
  cidr_block        = var.private_subnet_cidrs[count.index]
  availability_zone = data.aws_availability_zones.available.names[count.index]

  tags = {
    Name = "${var.service_name}-db-private-${count.index}"
  }
}

resource "aws_route_table_association" "private" {
  count          = length(aws_subnet.private)
  subnet_id      = aws_subnet.private[count.index].id
  route_table_id = aws_route_table.private.id
}

resource "aws_db_subnet_group" "this" {
  name       = "${lower(var.service_name)}-db"
  subnet_ids = aws_subnet.private[*].id
}

# Only the ECS tasks' security group may open MySQL connections. No egress
# rules: the database never initiates connections.
resource "aws_security_group" "db" {
  name        = "${var.service_name}-db-sg"
  description = "Allow MySQL from ECS tasks only"
  vpc_id      = var.vpc_id

  ingress {
    from_port       = 3306
    to_port         = 3306
    protocol        = "tcp"
    security_groups = [var.ecs_security_group_id]
    description     = "MySQL from ECS tasks"
  }
}

# Kept out of tfvars and git. override_special drops characters RDS rejects
# ('/', '@', '"', ' ') and ones that could confuse the Go MySQL DSN parser.
resource "random_password" "db" {
  length           = 20
  special          = true
  override_special = "!#%^*-_=+"
}

resource "aws_db_instance" "this" {
  identifier     = "${lower(var.service_name)}-mysql"
  engine         = "mysql"
  engine_version = "8.0"
  instance_class = "db.t3.micro"

  allocated_storage = 20
  storage_type      = "gp2"

  db_name  = var.db_name
  username = var.db_username
  password = random_password.db.result

  db_subnet_group_name   = aws_db_subnet_group.this.name
  vpc_security_group_ids = [aws_security_group.db.id]
  publicly_accessible    = false
  multi_az               = false

  # Assignment settings: tear down cleanly with terraform destroy.
  skip_final_snapshot     = true
  deletion_protection     = false
  backup_retention_period = 0
  apply_immediately       = true
}
