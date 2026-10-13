#!/usr/bin/env bash
set -euo pipefail

tag=${1:?usage: release.sh vX.Y.Z}
[[ $tag =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "error: tag must look like vX.Y.Z" >&2; exit 1; }

cd "$(dirname "$0")/.."
[ -z "$(git status --porcelain)" ] || { echo "error: git tree is not clean" >&2; exit 1; }

go test ./...
git tag "$tag"
git push origin "$tag"

# Needs GITHUB_TOKEN with repo scope; the tap repo ramesht007/homebrew-tap must exist.
goreleaser release --clean
