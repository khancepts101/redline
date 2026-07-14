terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = { source = "hashicorp/aws", version = "~> 6.0" }
  }
}

provider "aws" { region = var.region }

data "aws_vpc" "default" { default = true }

data "aws_ami" "ubuntu_arm64" {
  most_recent = true
  owners      = ["099720109477"] # Canonical
  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-arm64-server-*"]
  }
  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

resource "aws_security_group" "redline" {
  name_prefix = "redline-"
  description = "Redline observability host"
  vpc_id      = data.aws_vpc.default.id

  dynamic "ingress" {
    for_each = { ssh = 22, https = 443 }
    content {
      description = ingress.key
      from_port   = ingress.value
      to_port     = ingress.value
      protocol    = "tcp"
      cidr_blocks = [var.admin_cidr]
    }
  }

  ingress {
    description = "HTTP for ACME validation and HTTPS redirect"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_vpc_security_group_ingress_rule" "cardhunt_glitchtip" {
  security_group_id            = aws_security_group.redline.id
  referenced_security_group_id = var.cardhunt_security_group_id
  description                  = "GlitchTip ingestion from CardHunt ECS tasks"
  ip_protocol                  = "tcp"
  from_port                    = 8000
  to_port                      = 8000
}

resource "aws_vpc_security_group_ingress_rule" "cardhunt_pushgateway" {
  security_group_id            = aws_security_group.redline.id
  referenced_security_group_id = var.cardhunt_security_group_id
  description                  = "Pushgateway ingestion from CardHunt ECS batch tasks"
  ip_protocol                  = "tcp"
  from_port                    = 9091
  to_port                      = 9091
}

resource "aws_instance" "redline" {
  ami                    = data.aws_ami.ubuntu_arm64.id
  instance_type          = var.instance_type
  key_name               = var.key_name
  vpc_security_group_ids = [aws_security_group.redline.id]
  user_data = join("\n", [
    "#!/usr/bin/env bash",
    "export GRAFANA_DOMAIN=${jsonencode(var.grafana_domain)}",
    "export GLITCHTIP_DOMAIN=${jsonencode(var.glitchtip_domain)}",
    "export PROMETHEUS_DOMAIN=${jsonencode(var.prometheus_domain)}",
    "export ACME_EMAIL=${jsonencode(var.acme_email)}",
    replace(file("${path.module}/bootstrap.sh"), "#!/usr/bin/env bash\n", "")
  ])
  user_data_replace_on_change = true

  metadata_options {
    http_tokens = "required"
  }

  root_block_device {
    volume_size           = 30
    volume_type           = "gp3"
    encrypted             = true
    delete_on_termination = true
  }

  tags = { Name = "redline-observability" }
}

resource "aws_eip" "redline" {
  instance = aws_instance.redline.id
  domain   = "vpc"
  tags     = { Name = "redline-observability" }
}

output "public_ip" { value = aws_eip.redline.public_ip }
output "grafana_url" { value = "https://${var.grafana_domain}" }
output "prometheus_url" { value = "https://${var.prometheus_domain}" }
output "glitchtip_url" { value = "https://${var.glitchtip_domain}" }
output "ssh_command" { value = "ssh -i ~/.ssh/achilles.pem ubuntu@${aws_eip.redline.public_ip}" }
