#!/bin/bash
set -euo pipefail

DEPLOY_DIR=$(cd "$(dirname "$0")" && pwd)
BRAIN_DIR=${1:?Usage: bash build-brain.sh /absolute/path/to/ai-brain [release-tag]}
RELEASE_TAG=${2:-first-install}
[[ "$RELEASE_TAG" =~ ^[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}$ ]] || exit 1
test -f "$BRAIN_DIR/go.mod"
test -f "$BRAIN_DIR/cmd/server/main.go"
docker info >/dev/null

BUILD_TMP=$(mktemp -d)
trap 'rm -rf "$BUILD_TMP"' EXIT
cp "$BRAIN_DIR/go.mod" "$BRAIN_DIR/go.sum" "$BUILD_TMP/"
cp -R "$BRAIN_DIR/cmd" "$BRAIN_DIR/internal" "$BRAIN_DIR/pkg" "$BUILD_TMP/"

# Keep tracked or local .env, provider credentials, and test fixtures out of the image.
docker build --platform linux/amd64 -f "$DEPLOY_DIR/brain.Dockerfile" \
  --build-arg "GOPROXY=${GOPROXY:-https://goproxy.cn,direct}" \
  -t "genio-ai-brain:$RELEASE_TAG" "$BUILD_TMP"
