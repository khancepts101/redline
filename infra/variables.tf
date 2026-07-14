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

variable "cardhunt_security_group_id" {
  type        = string
  description = "Security group attached to CardHunt ECS tasks"
  default     = "sg-003d21f5e6fb8304c"
}

variable "grafana_domain" {
  type        = string
  description = "Public DNS name for the Grafana HTTPS endpoint"
}

variable "glitchtip_domain" {
  type        = string
  description = "Public DNS name for the GlitchTip HTTPS endpoint"
}

variable "prometheus_domain" {
  type        = string
  description = "Public DNS name for the Prometheus HTTPS endpoint"
}

variable "acme_email" {
  type        = string
  description = "Email used for ACME certificate registration"
}
