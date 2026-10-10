#!/bin/sh
set -eu
ding_archive=${1:?usage: package-macos-pkg.sh ARCHIVE VERSION OUTPUT.pkg}
ding_version=${2:?version required}
ding_output=${3:?output package required}
ding_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
test ! -e "$ding_output"
ding_temp=$(mktemp -d)
trap 'rm -rf "$ding_temp"' EXIT HUP INT TERM
mkdir -p "$ding_temp/root/usr/local/lib/ding" "$ding_temp/root/usr/local/bin"
# Input is the trusted output of package-local.sh, not an arbitrary download.
tar -xzf "$ding_archive" -C "$ding_temp/root/usr/local/lib/ding"
printf 'external\n' > "$ding_temp/root/usr/local/lib/ding/installation-owner"
ln -s /usr/local/lib/ding/ding "$ding_temp/root/usr/local/bin/ding"
pkgbuild --root "$ding_temp/root" --identifier ing.ding.watch --version "$ding_version" \
  --scripts "$ding_root/native/macos/installer" --install-location / "$ding_temp/unsigned.pkg"
if [ "${DING_PACKAGE_RELEASE:-0}" = 1 ]; then
  : "${DING_INSTALLER_IDENTITY:?Developer ID Installer identity required}"
  : "${DING_NOTARY_PROFILE:?notarytool Keychain profile required}"
  productsign --sign "$DING_INSTALLER_IDENTITY" "$ding_temp/unsigned.pkg" "$ding_output"
  xcrun notarytool submit "$ding_output" --keychain-profile "$DING_NOTARY_PROFILE" --wait
  xcrun stapler staple "$ding_output"
  spctl --assess --type install "$ding_output"
else
  cp "$ding_temp/unsigned.pkg" "$ding_output"
fi
