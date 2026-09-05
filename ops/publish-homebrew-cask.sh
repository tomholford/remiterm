#!/usr/bin/env sh
# Publish the rewritten Homebrew cask to tomholford/homebrew-tap.
# GoReleaser generates the cask but skip_upload is true so this owns the push.

set -e

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT="$SCRIPT_DIR/.."
cd "$REPO_ROOT"

CASK="$REPO_ROOT/dist/homebrew/Casks/remiterm.rb"
if [ ! -f "$CASK" ]; then
  echo "missing generated cask: $CASK"
  exit 1
fi

if [ -z "$HOMEBREW_TAP_TOKEN" ]; then
  echo "HOMEBREW_TAP_TOKEN is not set"
  exit 1
fi

ruby "$SCRIPT_DIR/rewrite-cask-postflight.rb" "$CASK"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

git clone --depth 1 \
  "https://x-access-token:${HOMEBREW_TAP_TOKEN}@github.com/tomholford/homebrew-tap.git" \
  "$TMP"

cp "$CASK" "$TMP/Casks/remiterm.rb"
cd "$TMP"

if git diff --quiet -- Casks/remiterm.rb; then
  echo "tap cask already up to date"
  exit 0
fi

git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
git add Casks/remiterm.rb
git commit -m "Brew cask update for remiterm version ${GITHUB_REF_NAME:-unknown}"
git push
