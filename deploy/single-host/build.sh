#!/bin/bash
set -euo pipefail
DEPLOY_DIR=$(cd "$(dirname "$0")" && pwd)
BACKEND_DIR=$(cd "$DEPLOY_DIR/../.." && pwd)
BRAIN_DIR=${1:?Usage: bash build.sh --backend-only [release-tag] OR /absolute/path/to/ai-brain [release-tag]}
RELEASE_TAG=${2:-first-install}
[[ "$RELEASE_TAG" =~ ^[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}$ ]] || exit 1
if [[ "$BRAIN_DIR" != "--backend-only" ]]; then
  test -f "$BRAIN_DIR/cmd/server/main.go"
fi
docker info >/dev/null
BUILD_TMP=$(mktemp -d)
trap 'rm -rf "$BUILD_TMP"' EXIT
# Use curated build contexts; production YAML, env files and credentials stay outside images.
mkdir -p "$BUILD_TMP/backend/conf" "$BUILD_TMP/backend/assets" "$BUILD_TMP/brain"
cp "$BACKEND_DIR/go.mod" "$BACKEND_DIR/go.sum" "$BUILD_TMP/backend/"
cp -R "$BACKEND_DIR/cmd" "$BACKEND_DIR/internal" "$BACKEND_DIR/pkg" "$BUILD_TMP/backend/"
cp "$BACKEND_DIR"/conf/*.go "$BACKEND_DIR/conf/model_config.json" "$BUILD_TMP/backend/conf/"
cp "$BACKEND_DIR/assets/nutrition_reference.json" "$BUILD_TMP/backend/assets/"
cp -R "$BACKEND_DIR/assets/migrations" "$BUILD_TMP/backend/assets/"
docker build --platform linux/amd64 -f "$DEPLOY_DIR/backend.Dockerfile" \
  -t "genio-backend:$RELEASE_TAG" "$BUILD_TMP/backend"
IMAGES=("genio-backend:$RELEASE_TAG")
if [[ "$BRAIN_DIR" != "--backend-only" ]]; then
  cp "$BRAIN_DIR/go.mod" "$BRAIN_DIR/go.sum" "$BUILD_TMP/brain/"
  cp -R "$BRAIN_DIR/cmd" "$BRAIN_DIR/internal" "$BRAIN_DIR/pkg" "$BUILD_TMP/brain/"
  docker build --platform linux/amd64 -f "$DEPLOY_DIR/brain.Dockerfile" \
    -t "genio-ai-brain:$RELEASE_TAG" "$BUILD_TMP/brain"
  IMAGES+=("genio-ai-brain:$RELEASE_TAG")
fi
# Bundle dependencies for the mainland server rather than requiring Docker Hub there.
for dependency in mysql:8.0 redis:7.2-alpine nginx:1.28-alpine; do
  docker pull --platform linux/amd64 "$dependency"
  IMAGES+=("$dependency")
done
docker image save "${IMAGES[@]}" \
  | gzip > "$DEPLOY_DIR/images-$RELEASE_TAG.tar.gz"
tar -czf "$DEPLOY_DIR/deployment-config.tar.gz" -C "$DEPLOY_DIR" \
  compose.yaml mysql-init.sh nginx.conf generation-disabled.conf prepare.py \
  server.yaml.example providers.yaml.example README.md BACKEND_ONLY.md
echo "Created images-$RELEASE_TAG.tar.gz; set the server .env RELEASE_TAG to $RELEASE_TAG."
echo "Created deployment-config.tar.gz (no private configuration or credentials)."
