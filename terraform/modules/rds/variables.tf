variable "service_name" {
  type        = string
  description = "Base name for RDS resources"
}

variable "vpc_id" {
  type        = string
  description = "VPC to place the database in"
}

# A default VPC gets one /20 per AZ starting at 172.31.0.0. No region has
# more than six AZs (172.31.0.0-172.31.95.255), so these /24s are free in any
# region whose default VPC hasn't been changed.
variable "private_subnet_cidrs" {
  type        = list(string)
  default     = ["172.31.96.0/24", "172.31.97.0/24"]
  description = "CIDRs for the private DB subnets, one per AZ (RDS needs at least two AZs)"
}
