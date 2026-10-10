# Build and qualify native local packages

These are release-engineering instructions for the watch-runtime source preview.
They do not change the legacy v0.14.0 stable download. The complete artifact embeds
the Console, MCP UI and adapter; macOS also includes its notification/Keychain app.
Users do not need Go, Python, Node, jq or model credentials.

## Build candidates

Run the **Local package qualification** workflow manually for all six OS/architecture
pairs. It uploads explicitly unqualified artifacts and has no publishing permission.
Build locally on each target after `npm ci` and `npm run build` in both `web/console`
and `web/mcp-app`:

```sh
sh scripts/package-local.sh 0.15.0-preview.1 dist/native
# macOS, after its native archive has been built:
sh scripts/package-macos-pkg.sh dist/native/ding_darwin_arm64.tar.gz 0.15.0 dist/native/Ding.pkg
```

Windows uses PowerShell and Inno Setup at build time:

```powershell
./scripts/package-windows.ps1 -Version 0.15.0-preview.1 -Output dist/native
```

Archives use the updater's canonical `ding_OS_ARCH.tar.gz` names. A standalone Unix
install belongs to the user and can later use the signed updater. The macOS system
package owns `/usr/local/lib/ding` and the matching `/usr/local/bin/ding` symlink;
it refuses to overwrite another installer and requires running users to stop Ding
before an upgrade. Its installation record is externally owned. The Windows
installer is per-user, registers Ding's notification identity and retains state
on uninstall. Its optional final checkbox explicitly enables sign-in startup.
After a package upgrade, run `ding setup` to refresh ownership metadata/startup.
Custom-state installations must be stopped separately before replacing their binary.

## Establish public trust

On protected native signing hosts, set `DING_PACKAGE_RELEASE=1` and
`DING_UPDATE_PUBLIC_KEY` to the base64 Ed25519 public key that will sign channel
metadata. That trust root is compiled into the binary. Do not generate a new key
per build: key rotation requires a reviewed trust transition for existing users.

On macOS configure `DING_CODESIGN_IDENTITY` (Developer ID Application),
`DING_INSTALLER_IDENTITY` (Developer ID Installer) and `DING_NOTARY_PROFILE` (an
already configured notarytool Keychain profile). The scripts sign hardened runtime
executables, submit for notarization, staple supported bundles/packages and verify
them. Do not use ad-hoc signing or Gatekeeper bypass instructions for a public
release. Follow [Apple's notarization workflow](https://developer.apple.com/documentation/security/customizing-the-notarization-workflow).

Windows release builds require `DING_SIGNING_THUMBPRINT` for a protected certificate
accessible to `signtool`; the executable, installer and uninstaller are signed and
timestamped. The packager verifies executable/installer Authenticode. The Inno
Setup definition uses [per-user installation](https://jrsoftware.org/ishelp/topic_setup_privilegesrequired.htm)
and a registered Start-menu AppUserModelID. Test actual toast visibility; a signed
installer alone cannot establish that. Linux archives use the signed metadata and
hash-pinned bootstrap; they are not advertised as a qualified distro package.

## Qualify before publication

Keep one evidence record per native OS/architecture, with version, artifact SHA256,
schema, OS version, command output and human observations. Cross-compilation is not
native acceptance. Minimum gates:

1. Fresh install without development tools; correct PATH/shortcut; explicit startup
   consent; useful owned endpoint; observed check; visible notification within the
   five-minute hypothesis. The fixture demo does not count as useful activation.
2. Close terminal and model client; stop/start/restart; logout/login; real reboot;
   exactly one writer; stored credentials survive under the same user.
3. Exercise missing/locked credentials, denied notifications, offline network, sleep
   gaps and a failed destination. Status must explain the actual evidence.
4. N→N+1 compatible update with state/outbox retained, interrupted replacement,
   failed readiness rollback, low disk, signature/hash mismatch, wrong platform,
   downgrade and newer-schema refusal. Use an isolated populated test installation.
5. Uninstall removes owned registration/files and retains state; reinstall reuses it.
   A foreign task/unit/install must survive untouched. Verify notification identity
   and package-manager ownership on each advertised path.
6. Run the [local MCP host proof](../integrations/desktop-proof.md) on the real clients,
   plus the existing [runtime release gates](watch-checklist.md), browser journeys,
   accessibility checks and elapsed-time soak.

Signed archives, native installers and recorded acceptance evidence are required
inputs to promotion. Schema 5 introduces transfer ownership fences; old runtimes
must refuse this state. Automatic update only permits equal schemas, never an
automatic database rollback. Back up and follow a separately reviewed migration
procedure for a schema-changing release.

## Sign and publish an explicitly selected channel

Compile `cmd/ding-release` with the same pinned public key. Run it with the protected
base64 Ed25519 private-key file and the six qualified native archives:

```sh
ding-release -artifacts dist/qualified -out dist/channels \
  -version v0.15.0-preview.1 -channel preview -key-file /PRIVATE/signing-key
```

It verifies the key pairing, requires all platforms, and writes an expiring signed
manifest, detached signature and version-specific hash-pinned installer. Upload the
exact archives/installers to that GitHub tag first. Then review and commit only the
chosen channel's three generated files under `releases/channels/`. Check the real
HTTPS downloads, archived file layout, installed version and update verification
from a clean machine before linking the channel from the website or documentation.
The bootstrap's initial trust is the official HTTPS distribution and inspection;
subsequent updates verify the compiled Ed25519 trust root.

Channel metadata expires after 30 days. Renew it deliberately for the same current
release before expiration; expiry must produce an actionable update-check error,
not stop local monitoring. Keep the private signing key out of repository history,
artifacts and logs. Retain previous signed artifacts and backup/migration guidance.

The former tag-triggered cross-build route is blocked because it omits native
helpers/signing. Do not re-enable it to bypass this procedure. Homebrew formula
publication must include the complete macOS helper beside the executable and retain
Homebrew ownership; it remains a release gate until that native path is qualified.
