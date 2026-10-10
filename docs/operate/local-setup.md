# Local setup in the development checkout

These commands describe the current development implementation. The published
v0.14.0 installer remains the legacy runtime. A signed watch-runtime installer,
native reboot qualification, and public marketplace acceptance are still gates.

## Build the complete local experience

Build tools are needed for this source preview; eventual release packages embed
the Console, MCP UI, and native helper so end users do not need them.

```sh
npm ci --prefix web/console
npm run build --prefix web/console
npm ci --prefix web/mcp-app
npm run build --prefix web/mcp-app
mkdir -p dist/local
go build -tags console,mcpui -o dist/local/ding ./cmd/ding
# macOS only; Xcode command-line tools are a build-time dependency:
sh scripts/build-notifications-macos.sh dist/local/DingNotifications.app
./dist/local/ding setup
```

Keep the resulting directory in place: the service records its absolute path.
The development macOS helper is ad-hoc signed, not a notarized public package.
Setup explains user-login startup before enabling it and opens the local Console.
Select **Create your first watch**, enter your own HTTP endpoint, review the exact
behavior, and test desktop notifications. No account, model, or YAML is required.
Confirm that you saw the test; an OS acceptance receipt cannot prove visibility.

Headless operation is explicit:

```sh
ding setup --yes --headless
ding watch create https://YOUR-SERVICE.example/health --delivery console
ding watch create https://YOUR-SERVICE.example/health --delivery console --yes
ding status
```

The example hostname is a placeholder. Replace it with an endpoint you own.
Console delivery records activity; use a supported webhook, Slack, or Discord
destination for notifications away from this machine. Timeouts produce source
health evidence; they are not invented HTTP 500 responses.

`ding demo` runs a separate one-minute localhost fixture through healthy, failing,
and recovered states, then removes its temporary database and watch. Add
`--desktop` for native demo notifications. It never edits existing watches and
does not count as a first useful watch.

## Operate and connect

`ding status --json` works without a running daemon and separates native service
state, daemon readiness, acquisition freshness, credential presence, and delivery
health. An overdue observation indicates an unexplained gap; it is not proof of
sleep. Use `ding service start`, `stop`, `restart`, or `status` for this installation.
Private background logs rotate at 1 MiB with two retained files in the state
directory. `ding service uninstall` removes startup registration and retains data.

After a real watch works, run `ding mcp setup` for guided local pairing. Closing
the model client or removing its adapter does not stop the background daemon.
Pairing/revocation and tool grants remain separate from service ownership.
Public ChatGPT directory eligibility is [still unresolved](../development/desktop-marketplace-plan.md).

Pass the same `--state-dir` on every command when choosing a custom directory.
Do not run another supervisor against that directory. A user login service needs
a logged-in user session; it does not promise unattended boot after a power loss.
The machine must stay awake, connected, and powered. An always-on Mac mini is a
complete free deployment; Ding does not require a cloud control plane.

## Credentials and recovery

Pipe credentials into `ding secret set NAME --stdin`; never put their values in
command arguments or a model conversation. macOS defaults to Keychain using the
bundled helper. A locked/missing Keychain reference fails closed and is retried
without a background permission prompt. Headless operation can explicitly select
`--store private-file`. Other platforms currently use the protected private file.
Restart Ding after adding a new reference. `ding secret list` prints names only.
Moving a state directory to another machine requires rebinding native credentials.

Use `ding backup --out /ABSOLUTE/NEW-FILE.db` for a verified database backup.
Credential stores and external executables need separate protected recovery.
Homebrew installations belong to Homebrew. Qualified standalone releases can use
`ding update check` and `ding update install --yes`: signed metadata, digest and
schema checks precede a backup, service stop, replacement, and readiness check.
`ding update recover` reconciles an interrupted transaction. Development builds
have no release trust key and refuse online updates. Schema changes require the
release's migration procedure; an old executable must not reopen a newer schema.
Windows in-place updates currently require the native installer path.

## Verification recorded here

On macOS ARM64: real launchd install/start/restart/stop/uninstall with persistent
watch state; synthetic native Keychain roundtrip/isolation/cleanup; universal
helper build; real HTTP onboarding in Chromium; accessibility scan; real scheduler
demo firing/recovery; signature, archive, transaction, and installer negative
tests. These results do not establish login/reboot behavior, notification visibility,
Windows/Linux native service behavior, or public package trust. See the
[implementation ledger](../development/local-first-progress.md).
