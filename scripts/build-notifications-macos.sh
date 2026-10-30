#!/bin/sh
set -eu
# Build-time Xcode only; users receive this bundled native executable.
ding_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ding_output=${1:-"$ding_root/dist/DingNotifications.app"}
ding_version=${2:-0.1.0}
# Bundle versions are numeric, including when the CLI release is a preview.
ding_bundle_version=${ding_version%%[-+]*}
case "$ding_bundle_version" in *[!0-9.]*|'') echo 'Invalid bundle version' >&2; exit 1;; esac
ding_identity=${DING_CODESIGN_IDENTITY:--}
mkdir -p "$ding_output/Contents/MacOS" "$ding_output/Contents/Resources"
mkdir -p "$ding_output/Contents/Resources/NotificationIcons"
for ding_color in green yellow red; do
  cp "$ding_root/design/assets/notifications/$ding_color.png" "$ding_output/Contents/Resources/NotificationIcons/$ding_color.png"
done
cp "$ding_root/native/macos/notifications/Info.plist" "$ding_output/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $ding_bundle_version" "$ding_output/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Set :CFBundleVersion $ding_bundle_version" "$ding_output/Contents/Info.plist"
ding_temp=$(mktemp -d)
trap 'rm -rf "$ding_temp"' EXIT HUP INT TERM
xcrun swift "$ding_root/scripts/render-macos-icon.swift" "$ding_root/design/assets/mark.svg" "$ding_temp/Ding.iconset"
iconutil --convert icns --output "$ding_output/Contents/Resources/Ding.icns" "$ding_temp/Ding.iconset"
for ding_arch in arm64 x86_64; do
  xcrun swiftc -O -swift-version 6 -parse-as-library -target "$ding_arch-apple-macos13" \
    "$ding_root/native/macos/notifications/main.swift" "$ding_root/native/macos/notifications/credentials.swift" -o "$ding_temp/$ding_arch"
done
xcrun lipo -create "$ding_temp/arm64" "$ding_temp/x86_64" -output "$ding_output/Contents/MacOS/DingNotifications"
if [ "$ding_identity" = - ]; then
  codesign --force --sign - "$ding_output"
  echo 'Built development helper with ad-hoc signing; release signing/notarization remains required.'
else
  codesign --force --options runtime --timestamp --sign "$ding_identity" "$ding_output"
fi
codesign --verify --strict "$ding_output"
