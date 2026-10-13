#!/usr/bin/env bash
set -euo pipefail

REPO="ramesht007/pineapple-cli"

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) echo "error: unsupported OS: $(uname -s) (macOS and Linux only)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "error: unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

tag=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
  | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
[ -n "$tag" ] || { echo "error: could not determine latest release" >&2; exit 1; }
version=${tag#v}

file="pine_${version}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$tag"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading pine $tag ($os/$arch)..."
curl -fsSL -o "$tmp/$file" "$base/$file"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt"

if command -v sha256sum >/dev/null 2>&1; then
  sha() { sha256sum "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then
  sha() { shasum -a 256 "$1" | cut -d' ' -f1; }
else
  echo "error: need sha256sum or shasum to verify download" >&2; exit 1
fi
want=$(grep " $file\$" "$tmp/checksums.txt" | cut -d' ' -f1)
[ -n "$want" ] || { echo "error: $file not in checksums.txt" >&2; exit 1; }
[ "$(sha "$tmp/$file")" = "$want" ] || { echo "error: checksum mismatch for $file" >&2; exit 1; }

tar -xzf "$tmp/$file" -C "$tmp" pine

if [ -w /usr/local/bin ]; then
  dest=/usr/local/bin
else
  dest="$HOME/.local/bin"
  mkdir -p "$dest"
  case ":$PATH:" in
    *":$dest:"*) ;;
    *) echo "warning: $dest is not on your PATH; add it to your shell profile" >&2 ;;
  esac
fi
install -m 755 "$tmp/pine" "$dest/pine"
echo "Installed to $dest/pine"
"$dest/pine" --version
