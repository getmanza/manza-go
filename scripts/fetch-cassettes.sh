#!/usr/bin/env bash
# Downloads the cassette tarball published by manza-ruby's release workflow.
# CI calls this before running tests so we don't have to commit cassettes
# into both repos.
#
#   scripts/fetch-cassettes.sh            # the pinned release (v1.0.0)
#   scripts/fetch-cassettes.sh v1.0.1     # specific tag
#
# Cassettes land under testdata/cassettes/.
set -euo pipefail

REPO="getmanza/manza-ruby"
PINNED_TAG="v1.0.0" # bump on purpose: an unpinned latest tag lets one release break every SDK CI at once
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEST="$ROOT/testdata/cassettes"

TAG="${1:-$PINNED_TAG}"
AUTH=()
if [[ -n "${GH_TOKEN:-}" ]]; then
  AUTH=(-H "Authorization: Bearer $GH_TOKEN")
fi

URL="https://github.com/$REPO/releases/download/$TAG/cassettes-$TAG.tar.gz"
echo "Fetching cassettes from $URL"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
curl -fsSL --retry 8 --retry-all-errors --retry-delay 10 "${AUTH[@]}" -H "Accept: application/octet-stream" -o "$TMP/cassettes.tar.gz" "$URL"

mkdir -p "$DEST"
tar -xzf "$TMP/cassettes.tar.gz" -C "$(dirname "$DEST")"
echo "Cassettes extracted to $DEST"
