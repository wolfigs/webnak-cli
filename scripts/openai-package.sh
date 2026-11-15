#!/bin/sh
# Builds the ChatGPT plugin package (see openai/README.md).
set -eu
cd "$(dirname "$0")/.."
mkdir -p dist
rm -f dist/webnak-chatgpt.zip
cd openai
zip -qr ../dist/webnak-chatgpt.zip plugin.json .mcp.json assets
echo "dist/webnak-chatgpt.zip"
