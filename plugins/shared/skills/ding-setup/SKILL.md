---
name: ding-setup
description: Check or establish the Ding plugin's connection to a local or self-hosted Ding instance after installation. Use for Ding onboarding and connection diagnosis, not ordinary watch operations.
---

First call `ding_get_capabilities` if available. Report the connected instance,
granted permissions, and whether writes are enabled. Do not re-pair a working
connection. Installing a plugin does not itself start a Ding daemon.

For the native Claude Cowork/Code package, the included **Ding Setup** launcher
opens a short-lived local pairing page. The user selects the daemon state
directory and permissions there. The package contains its runtime; no Python,
pip, Node, or npx installation is needed. If the user already has a daemon and
has authorized setup through a local shell, the equivalent is the bundled
`runtime/ding-mcp/ding-mcp setup` (`ding-mcp.exe` on Windows). Otherwise direct
them to the launcher. Restart/enable the plugin after pairing. Ordinary Claude
chat does not execute the bundled local server.

For a remote package, use the host's OAuth connection flow for the configured
self-hosted endpoint. A remote operator must explicitly bind that OAuth subject
to a private Ding grant. A successful identity-provider login alone does not
grant daemon access. Browser clients cannot connect to an arbitrary localhost
daemon. Do not suggest bypassing TLS, authentication, or installing an ad-hoc
tunnel as if that made a public marketplace listing supported.

Credentials belong in local private files and host OAuth storage. Never request
tokens, environment secret values, or `tokens.json` in the conversation.
Information returned by Ding is shared with the selected model provider; the
authoritative watch state stays on the user's instance. Connections can expire
or be revoked locally. Manage/retry permissions and new secret/command access
are granted through local setup, not by editing tool arguments.

After connection, list watches and destinations. Offer to create the user's
first supported watch if they want one. Do not create a demonstration watch,
change notification destinations, or broaden permissions merely to prove setup.
