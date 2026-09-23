#!/bin/sh
set -eu
cd /opt/genio-backend-code/deploy/single-host
docker compose --profile https up -d nginx
