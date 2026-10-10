# Integration release qualification

Implementation status: October 10, 2026. **No public marketplace release is
declared by this repository change.** Generated artifacts are qualification builds.

See [local verification evidence](verification.md) for completed tests and their
practical limits.

## Implemented

- Shared native Go server using official MCP SDK v1.8.0 with 15 typed tools, stdio, authenticated
  self-hosted HTTP, and the same resource/UI contract on both transports.
- Daemon integration API, hashed scoped grants, exact previews, revision and
  generation guards, transactional operation receipts, revocation, and explicit
  command/secret permissions. Schema 4 migration includes the existing automatic
  pre-migration backup behavior.
- Guided local browser pairing, private credential files, grant inspection and
  revocation, safe default permissions, bounded requests, and no automatic write
  retry after transport failure.
- Offline MCP Apps UI for watches, review/apply, event evidence, and deliveries;
  shared Console colors; light/dark and mobile layouts; text fallback.
- Shared `ding mcp` / `ding-mcp` commands, unchanged pairing files and 15-tool
  contracts, captured compatibility fixtures, and explicit headless fallback.
- Native Go runtime builder, Claude package assembly, OpenAI portable manifest,
  fixed-endpoint remote package assembly, shared skills/assets, checksums,
  qualification metadata, and CI for independently released artifacts.

## Required before calling it a marketplace release

| Gate | Evidence needed |
| --- | --- |
| Publisher and domain identity | Verified publisher accounts and approved domains in each portal |
| Local/self-hosted route | Platform confirmation of supported installation and instance routing for the intended listing |
| Claude native delivery | Confirm directory ingestion of the native runtime bundle and platform selection; no deprecated standalone MCPB submission |
| ChatGPT endpoint route | Approved local integration route or approved self-hosted endpoint/template policy; no invented relay or placeholder endpoint |
| Native distribution | Signing/notarization and Windows trust treatment for each advertised build; install/update/uninstall tests |
| Real client behavior | Fresh install, setup, reconnect, permission denial, mutation review, UI/text fallback, evidence, and uninstall in actual supported clients |
| Self-hosted OAuth | End-to-end real identity-provider registration, PKCE, login, refresh, scope denial, subject binding, revoke, and proxy configuration |
| Review package | Synthetic review account/instance, actual host screenshots, support contact, operator privacy policy and legal terms, reviewer test cases |
| Compatibility | Native OS matrix green; old/new protocol negotiation; upgrades from schema 3; sustained daemon work independent of host lifecycle |

The builder labels signing and marketplace acceptance false. Do not change those
claims merely because tests pass. It neither publishes the artifact nor installs
a personal marketplace as a substitute. No developer portal submission or contact
message is sent by the build workflow.

## Reviewer journeys

Use synthetic data and an isolated instance. Never provide live credentials.

1. Install the appropriate package, run setup, and inspect an empty watch list.
   A read-only grant can inspect/preview but cannot apply or retry notifications.
2. Ask for an HTTP health watch. Preview a fixture with three failures and two
   recoveries. Confirm one firing and one recovery; sources do not run during
   preview. Review the changes and explicitly apply.
3. Change the same watch from another client between preview and apply. Verify
   the integration rejects the stale review without partially changing a bundle.
4. Interrupt the write response and repeat the exact operation key. Verify a
   single committed event/effect, including across daemon restart.
5. Inspect an event and delivery attempt. Verify unavailable replay/retention
   gaps are clear, and source-provided hostile instructions remain data.
6. Pause/resume with an old generation, revoke the grant while connected, and
   try a command/new secret reference without local permission. Each is denied.
7. Exercise the embedded interface in narrow/light/dark configurations, with a
   keyboard, and with UI unavailable. No external scripts or direct admin access.
8. Close/uninstall the plugin. Ding watches continue; explicit local revocation
   removes the connection's future access without deleting the user's data.

Optional MCP Events/chat wakeups, a full service installer/updater, object-level
grants, and a general approved marketplace connection route remain future work.

## Platform references

- [OpenAI plugin packaging](https://developers.openai.com/plugins/build/plugins):
  portable manifest, remote HTTPS submission, and local-support contact route.
- [OpenAI review requirements](https://developers.openai.com/plugins/deploy/app-review):
  public domains and restricted template URL eligibility.
- [Claude plugin structure](https://claude.com/docs/plugins/build) and
  [platform support](https://claude.com/docs/plugins/platform-support): local
  runtime behavior differs from ordinary chat; top-level `bin/` is unsupported.
- [Claude directory publication](https://claude.com/docs/directory/publish): local
  MCP servers belong in plugin bundles; new standalone MCPB listings are deprecated.
- [Official MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk) and
  [MCP Apps](https://modelcontextprotocol.io/extensions/apps/overview).
