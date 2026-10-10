# Run Ding on your own machine continuously

An always-on Mac mini, Linux server, or container is a complete free deployment.
No Ding account, relay, cloud credential, or model subscription is needed. Checks
run where the daemon runs; that machine must remain powered, awake and connected.
Closing the LLM client does not stop the daemon.

## Mac mini or workstation

Run `ding setup` as the user who owns the watches. Its LaunchAgent starts at that
user's **login**. Keep the same stable installation and state paths. Adjust the
machine's Energy settings explicitly if it must stay awake; Ding does not change
power or security settings. FileVault can require a local unlock after power loss.
A locked screen is different from logout or sleep. Use a remote delivery channel
for unattended monitoring; desktop acceptance cannot prove someone saw a banner.

`ding status --json` combines service registration, authenticated daemon status,
source freshness and delivery health. Overdue checks are gaps, not successful
observations. Test login and a real reboot on the intended machine before relying
on this arrangement. An unattended macOS boot daemon under a separate account is
not qualified by the user LaunchAgent; do not run a second supervisor on its state.

## Linux without a logged-in session

For a personal systemd installation, use `ding setup --yes --headless`, then
deliberately enable lingering with `sudo loginctl enable-linger "$USER"`. This is
an explicit OS administration choice: it keeps that user's systemd manager active
after logout and starts it at boot. Reverse it with `disable-linger`. Validate the
executable is on a persistent mounted filesystem and credentials remain readable.

For a dedicated server, use the reviewed template at `deploy/local/ding.service`.
Install the complete archive at `/opt/ding`, create a non-root `ding` system user,
copy the unit into `/etc/systemd/system`, then run `systemctl daemon-reload` and
`systemctl enable --now ding.service`. The unit creates `/var/lib/ding` for that
user. Do **not** also run `ding setup` for the same state. Operate it through
`systemctl`; Ding's `service` commands own only its generated user services.

Run CLI commands as that user, always passing `--state-dir /var/lib/ding`.
Pipe credentials into `ding secret set NAME --stdin --store private-file` under
that identity. Never put secrets in the unit, shell arguments or manifests.
Commands and local/private sources remain available, with the service user's
permissions and explicit absolute paths. A desktop session is not available:
choose console evidence or a webhook/Slack/Discord destination.

## Windows and containers

The Windows installer registers a notification identity and offers sign-in startup.
Its managed task runs as the current, least-privileged interactive user; this is
not an unattended Windows Service. DPAPI credentials are bound to that user and
state path. For unattended boot, use the generated SCM template below. The same Go daemon
runs in-process under SCM, reports readiness and drains on stop/shutdown. A service
account uses remote delivery; desktop notification bridging is not claimed.

The repository's local `Dockerfile` runs `ding daemon`. Build it, pin the resulting
image digest, bind a persistent owner-only directory at `/var/lib/ding`, and choose
the container runtime's restart policy explicitly. Run as a non-root UID with
permission to the volume; mount credentials as private files owned by that UID.
Do not mount the Docker socket, host root or a second writer on the same volume.
CLI access can use `docker exec` under that same user. Exposing a browser remotely
requires an explicitly trusted HTTPS origin and TLS proxy; keep the admin API
private. See [daemon operation](index.md) and [backups](backup.md).

## Local MCP on an always-on machine

Run `ding mcp setup` after daemon readiness and a useful watch are verified.
The generated adapter configuration is for an LLM client running on that machine.
For another machine, the administrator must deliberately configure the existing
authenticated HTTP MCP transport, TLS, audience and scoped grants. Never expose
the local admin token as a public MCP credential. Official marketplace eligibility
is independent of this working local protocol path; see the
[desktop proof package](../integrations/desktop-proof.md).

Before considering cloud, verify terminal close, model-client close, service
restart, loss/recovery of network, a real login/reboot and a failed delivery. Keep
exports, database backups and credential recovery separately. These are also the
acceptance checks for a Mac mini that developers operate continuously.

## Generate an explicit boot registration

`ding service boot-template --executable ABSOLUTE_DING_PATH --state-dir ABSOLUTE_NEW_STATE
--user SERVICE_ACCOUNT` prints the native registration without installing it. Add
`--group SERVICE_GROUP` on Unix when needed. Create an existing dedicated non-root
account; this command does not elevate or create accounts. Choose a new state path
and rebind credentials under that identity, with private-file storage when its
native user credential store is unavailable.

On Linux, review the generated system unit, create the private state directory
owned by that account, install the unit as administrator and enable it through
systemd. On macOS, review the LaunchDaemon plist, install it root-owned at its
printed `/Library/LaunchDaemons` path with mode 0644, then bootstrap it in the
`system` launchd domain. Use a dedicated account with access only to the required
sources/state; FileVault boot-unlock requirements still apply.

On Windows, review the generated PowerShell before running it as administrator.
It prompts for an existing service account (passwords are not written into the
script), creates a new private state directory/ACL and registers an automatic
SCM service. Grant that account the OS's Log on as a service right according to
local policy. Manage stop/start/removal through Windows Services or PowerShell.
It uses `ding service host` internally, not an interactive scheduled task.

These administrator-owned registrations are managed with their OS supervisor;
`ding service repair/uninstall` manage Ding's separate user registration only.
Never install both against the same state. Real boot/reboot, credential access,
power recovery and native recovery policies must be qualified on each host before
unattended operation is advertised. Templates do not establish those outcomes.
