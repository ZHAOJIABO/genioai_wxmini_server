#!/bin/sh
set -eu
[ "${RENEWED_LINEAGE:-}" = /etc/letsencrypt/live/geniomini.appbobo.cn ] || exit 0
cd /opt/genio-backend-code/deploy/single-host
install -m 644 "$RENEWED_LINEAGE/fullchain.pem" private/tls/fullchain.pem
install -m 600 "$RENEWED_LINEAGE/privkey.pem" private/tls/privkey.pem
