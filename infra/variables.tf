variable "region" {
  type    = string
  default = "us-east-1"
}

variable "instance_type" {
  type    = string
  default = "t4g.small"
}

variable "key_name" {
  type    = string
  default = "achilles"
}

variable "admin_cidr" {
  type        = string
  description = "Public administrator IPv4 CIDR allowed to reach SSH and observability UIs"
  default     = "172.1.78.135/32"
}
