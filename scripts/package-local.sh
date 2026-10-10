#!/bin/sh
set -eu
# Run on the target OS after building Console/MCP assets. Never publishes.
ding_version=${1:?usage: package-local.sh VERSION OUTPUT_DIRECTORY}
ding_output=${2:?output directory required}
ding_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ding_root"
case "$ding_version" in *[!0-9A-Za-z.+-]*|'') echo 'Invalid version' >&2; exit 1;; esac
ding_os=$(go env GOOS)
ding_arch=$(go env GOARCH)
case "$ding_os/$ding_arch" in darwin/arm64|darwin/amd64|linux/arm64|linux/amd64) ;; *) echo 'Use the native Windows packager for Windows.' >&2; exit 1;; esac
ding_public=${DING_UPDATE_PUBLIC_KEY:-}
case "$ding_public" in *[!A-Za-z0-9+/=]*) echo 'Invalid public update key' >&2; exit 1;; esac
ding_release=${DING_PACKAGE_RELEASE:-0}
if [ "$ding_release" = 1 ]; then
  test -n "$ding_public" || { echo 'A release requires the pinned update public key.' >&2; exit 1; }
  if [ "$ding_os" = darwin ]; then
    : "${DING_CODESIGN_IDENTITY:?Developer ID Application identity required}"
    : "${DING_NOTARY_PROFILE:?notarytool Keychain profile required}"
    test "$DING_CODESIGN_IDENTITY" != -
  fi
fi
mkdir -p "$ding_output"
ding_output=$(CDPATH= cd -- "$ding_output" && pwd)
ding_archive="$ding_output/ding_${ding_os}_${ding_arch}.tar.gz"
test ! -e "$ding_archive" || { echo 'Refusing to overwrite an artifact.' >&2; exit 1; }
ding_temp=$(mktemp -d)
trap 'rm -rf "$ding_temp"' EXIT HUP INT TERM
mkdir "$ding_temp/payload"
CGO_ENABLED=0 go build -trimpath -tags console,mcpui \
  -ldflags="-s -w -X main.version=$ding_version -X github.com/ding-labs/ding/internal/update.PublicKey=$ding_public" \
  -o "$ding_temp/payload/ding" ./cmd/ding
cp LICENSE README.md ding.yaml.example "$ding_temp/payload/"
if [ "$ding_os" = darwin ]; then
  sh scripts/build-notifications-macos.sh "$ding_temp/payload/DingNotifications.app"
  if [ "$ding_release" = 1 ]; then
    codesign --force --options runtime --timestamp --sign "$DING_CODESIGN_IDENTITY" "$ding_temp/payload/ding"
    ditto -c -k --keepParent "$ding_temp/payload" "$ding_temp/notarize.zip"
    xcrun notarytool submit "$ding_temp/notarize.zip" --keychain-profile "$DING_NOTARY_PROFILE" --wait
    xcrun stapler staple "$ding_temp/payload/DingNotifications.app"
    codesign --verify --strict "$ding_temp/payload/ding"
    codesign --verify --strict "$ding_temp/payload/DingNotifications.app"
    spctl --assess --type execute "$ding_temp/payload/ding"
  fi
fi
"$ding_temp/payload/ding" version --json
# An explicit file list preserves the updater's canonical archive paths.
if [ "$ding_os" = darwin ]; then
  COPYFILE_DISABLE=1 tar -C "$ding_temp/payload" -czf "$ding_archive" ding DingNotifications.app LICENSE README.md ding.yaml.example
else
  tar -C "$ding_temp/payload" -czf "$ding_archive" ding LICENSE README.md ding.yaml.example
fi
echo "Built $ding_archive (release signing mode: $ding_release). Qualification and publication remain separate."
