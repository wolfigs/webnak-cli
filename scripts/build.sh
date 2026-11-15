#!/usr/bin/env bash
# Builds webnak for every platform into dist/, plus checksums.txt.
#   VERSION=0.1.0 scripts/build.sh
set -euo pipefail
cd "$(dirname "$0")/.."
VERSION="${VERSION:-dev}"
rm -rf dist && mkdir -p dist
for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do
  os="${target%/*}"; arch="${target#*/}"
  ext=""; [ "$os" = windows ] && ext=".exe"
  out="dist/webnak_${os}_${arch}${ext}"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o "$out" .
  echo "built $out"
done
( cd dist && shasum -a 256 webnak_* > checksums.txt )
cat dist/checksums.txt
