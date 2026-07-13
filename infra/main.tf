terraform { required_providers { aws = { source = "hashicorp/aws"; version = "~> 5.0" } } }
provider "aws" { region = var.region }
resource "aws_security_group" "redline" { name = "redline"; ingress { from_port=22;to_port=22;protocol="tcp";cidr_blocks=[var.ssh_cidr] }; ingress { from_port=80;to_port=80;protocol="tcp";cidr_blocks=["0.0.0.0/0"] }; ingress { from_port=443;to_port=443;protocol="tcp";cidr_blocks=["0.0.0.0/0"] }; egress { from_port=0;to_port=0;protocol="-1";cidr_blocks=["0.0.0.0/0"] } }
resource "aws_instance" "redline" { ami=var.ami_id;instance_type=var.instance_type;key_name=var.key_name;vpc_security_group_ids=[aws_security_group.redline.id];root_block_device { volume_size=30;volume_type="gp3" };tags={Name="redline"} }
resource "aws_eip" "redline" { instance=aws_instance.redline.id;domain="vpc" }
output "public_ip" { value=aws_eip.redline.public_ip }
