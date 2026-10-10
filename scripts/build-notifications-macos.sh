#!/bin/sh
set -eu
# Build-time Xcode only; users receive this bundled native executable.
ding_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ding_output=${1:-"$ding_root/dist/DingNotifications.app"}
ding_identity=${DING_CODESIGN_IDENTITY:--}
mkdir -p "$ding_output/Contents/MacOS"
cp "$ding_root/native/macos/notifications/Info.plist" "$ding_output/Contents/Info.plist"
ding_temp=$(mktemp -d)
trap 'rm -rf "$ding_temp"' EXIT HUP INT TERM
for ding_arch in arm64 x86_64; do
  xcrun swiftc -O -swift-version 6 -parse-as-library -target "$ding_arch-apple-macos13" \
    "$ding_root/native/macos/notifications/main.swift" -o "$ding_temp/$ding_arch"
done
xcrun lipo -create "$ding_temp/arm64" "$ding_temp/x86_64" -output "$ding_output/Contents/MacOS/DingNotifications"
if [ "$ding_identity" = - ]; then
  codesign --force --sign - "$ding_output"
  echo 'Built development helper with ad-hoc signing; release signing/notarization remains required.'
else
  codesign --force --options runtime --timestamp --sign "$ding_identity" "$ding_output"
fi
codesign --verify --strict "$ding_output"
