#!/bin/sh
# Installs the Webnak CLI on macOS or Linux.
#   curl -fsSL https://raw.githubusercontent.com/sathninduk/webnak-cli/main/install.sh | sh
# Options (environment): WEBNAK_VERSION=0.1.0 to pin a version,
# WEBNAK_INSTALL_DIR=/usr/local/bin to choose where it goes.
set -eu
REPO="sathninduk/webnak-cli"
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in darwin|linux) ;; *) echo "webnak: use install.ps1 on Windows" >&2; exit 1 ;; esac
case "$(uname -m)" in x86_64|amd64) arch=amd64 ;; arm64|aarch64) arch=arm64 ;; *) echo "webnak: no build for $(uname -m)" >&2; exit 1 ;; esac
if [ -n "${WEBNAK_VERSION:-}" ]; then
  tag="v${WEBNAK_VERSION#v}"
else
  tag=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)
fi
[ -n "$tag" ] || { echo "webnak: could not find the latest release" >&2; exit 1; }
name="webnak_${os}_${arch}"
base="https://github.com/$REPO/releases/download/$tag"
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
echo "Downloading webnak $tag for $os/$arch…"
curl -fsSL "$base/$name" -o "$tmp/webnak"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"
want=$(awk -v f="$name" '$2==f{print $1}' "$tmp/checksums.txt")
if command -v sha256sum >/dev/null 2>&1; then got=$(sha256sum "$tmp/webnak" | awk '{print $1}'); else got=$(shasum -a 256 "$tmp/webnak" | awk '{print $1}'); fi
[ -n "$want" ] && [ "$want" = "$got" ] || { echo "webnak: checksum mismatch, not installing" >&2; exit 1; }
chmod +x "$tmp/webnak"
dir="${WEBNAK_INSTALL_DIR:-}"
if [ -z "$dir" ]; then
  if [ -w /usr/local/bin ]; then dir=/usr/local/bin; else dir="$HOME/.local/bin"; fi
fi
mkdir -p "$dir"
mv "$tmp/webnak" "$dir/webnak"
echo "Installed $("$dir/webnak" --version) to $dir/webnak"
case ":$PATH:" in *":$dir:"*) ;; *) echo "Add $dir to your PATH to run webnak from anywhere." ;; esac
echo "Next: webnak login"
