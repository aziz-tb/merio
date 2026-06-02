#!/usr/bin/env bash
# Pull latest images and restart the Merio production stack.

set -euo pipefail

cd /opt/merio

docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml up -d --remove-orphans

sleep 5

docker compose -f docker-compose.prod.yml ps
docker compose -f docker-compose.prod.yml logs --tail=20 app temporal-worker

echo "Deploy complete."
