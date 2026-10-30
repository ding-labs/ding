#!/bin/sh
set -eu
# Build-time AppKit only; committed PNGs work on every notification provider.
ding_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ding_output=${1:-"$ding_root/design/assets/notifications"}
ding_temp=$(mktemp -d)
trap 'rm -rf "$ding_temp"' EXIT HUP INT TERM
mkdir -p "$ding_output"
for ding_color in green yellow red; do
  ding_mark="$ding_root/design/assets/bell-$ding_color.svg"
  if [ "$ding_color" = green ]; then ding_mark="$ding_root/design/assets/mark.svg"; fi
  xcrun swift "$ding_root/scripts/render-macos-icon.swift" "$ding_mark" "$ding_temp/$ding_color.iconset"
  cp "$ding_temp/$ding_color.iconset/icon_256x256@2x.png" "$ding_output/$ding_color.png"
done
