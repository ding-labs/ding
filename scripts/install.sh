#!/bin/sh
set -eu

# Homebrew installs the complete bundle, including the native Mac helper.
# Downloading only the ding executable leaves notifications and Keychain broken.
if ! command -v brew >/dev/null 2>&1; then
  echo 'Install Homebrew from https://brew.sh, then run:' >&2
  echo '  brew install ding-labs/tap/ding' >&2
  echo '  ding setup' >&2
  exit 1
fi
brew install ding-labs/tap/ding
printf '\nRun ding setup to enable background startup and create your first watch.\n'
