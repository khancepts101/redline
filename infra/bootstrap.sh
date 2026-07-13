#!/usr/bin/env bash
set -euo pipefail
exec > >(tee /var/log/redline-bootstrap.log | logger -t redline-bootstrap -s 2>/dev/console) 2>&1

# A small swap file keeps image startup and migrations from OOM-killing a 2 GiB host.
if ! swapon --show | grep -q /swapfile; then
  fallocate -l 2G /swapfile
  chmod 600 /swapfile
  mkswap /swapfile
  swapon /swapfile
  echo '/swapfile none swap sw 0 0' >> /etc/fstab
fi

apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y docker.io docker-compose-v2 git openssl
systemctl enable --now docker

install -d -o ubuntu -g ubuntu /opt/redline
if [ ! -d /opt/redline/.git ]; then
  sudo -u ubuntu git clone https://github.com/khancepts101/redline.git /opt/redline
else
  sudo -u ubuntu git -C /opt/redline pull --ff-only
fi

cd /opt/redline
umask 077
PUBLIC_IP="$(curl -fsS https://checkip.amazonaws.com | tr -d '[:space:]')"
printf 'GLITCHTIP_SECRET_KEY=%s\nGLITCHTIP_DOMAIN=http://%s:8000\n' "$(openssl rand -hex 32)" "$PUBLIC_IP" > .env.aws
docker compose --env-file .env.aws -f infra/compose.aws.yml up -d postgres redis
docker compose --env-file .env.aws -f infra/compose.aws.yml run --rm glitchtip ./manage.py migrate --noinput
docker compose --env-file .env.aws -f infra/compose.aws.yml up -d
