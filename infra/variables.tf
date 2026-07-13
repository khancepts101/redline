variable "region" { type = string; default = "us-east-1" }
variable "instance_type" { type = string; default = "t4g.small" }
variable "ami_id" { type = string; description = "ARM64 Ubuntu or Amazon Linux AMI" }
variable "ssh_cidr" { type = string; default = "0.0.0.0/0" }
variable "key_name" { type = string }
