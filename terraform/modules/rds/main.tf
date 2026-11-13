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
