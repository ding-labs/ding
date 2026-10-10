# Ding integration data handling

This describes the software in this repository. It is a technical disclosure,
not a published operator privacy policy or legal terms for a marketplace listing.

Ding stores watch definitions, selected observations, event evidence, delivery
history, integration grants, previews, and mutation receipts on the user's Ding
instance. The MCP adapter sends requested results to the connected LLM host.
That provider processes them under its own account and service terms. A
self-hosted instance does not make model-visible information local to that host.

The adapter has no Ding-operated relay, analytics endpoint, or model API key.
External requests are to the configured daemon, self-hosted OAuth provider, and
the LLM host over its connection. The daemon independently contacts the sources
and destinations configured by the user. Embedded UI scripts/styles are bundled;
the widget makes tool calls through the host, without contacting third-party CDNs.

Pairing reads local administrator credentials only to create/revoke grants. It
stores the new scoped token in a private file. The daemon stores the token hash;
the plaintext token is returned only at grant creation. Tokens and environment
secret values are never tool arguments, resource contents, or ordinary results.
Secret reference names and status can be visible. Source observations can contain
sensitive content; configure field selection accordingly.

Revocation immediately blocks subsequent tool access and writes, including saved
receipt reads. It does not remove information already sent to the LLM provider
or cancel existing watches. Delete/pause watches separately when intended.
Notification retry can duplicate a message already accepted by a remote receiver.

Preview handles expire after 15 minutes; expired previews are pruned at the next
preview creation. There are at most 64 outstanding previews and 256 grant records
per instance. Mutation receipts intentionally do not expire or disappear on
revocation, preventing an old retry from becoming a new action. They are capped
at 10,000 per instance, after which new integration writes fail atomically.
Receipts may include reviewed definitions. A future administrative retention or
archival policy must preserve idempotency tombstones before changing this rule.
Ordinary event retention remains governed by Ding's daemon history settings.
