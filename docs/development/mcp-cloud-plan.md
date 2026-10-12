# Ding MCP cloud support and implementation plan

## Implementation status — October 11, 2026

Implemented on `codex/cloud-mcp` in focused commits, starting from `origin/main`
at `bf0b0b3` and incorporating the subsequent `9341ccc` main update. The findings
below describe the original audit; this status
supersedes their descriptions of missing code.

- Phases 1–2: shared limits/admission, effective capabilities, workspace discovery
  and explicit targeting, cloud usage/health and embedded selection are implemented.
- Phase 3: composed hosted HTTP/JWT qualification and updated instructions are
  implemented. Live provider/model-host qualification remains open; the deployed
  service reported password-only login and 404 for MCP/discovery.
- Phase 4: optional private OAuth bridge, durable refresh, explicit local/cloud
  targets and operation-target journals are implemented. Local-only behavior stays
  independent of cloud sign-in.
- Phase 5: read-only eligibility/status and reviewed prepare/test/finish/cancel
  coordination reuse the handoff runtime. Move is separate owner consent; private
  receipt transport is restricted to approved native bridge clients. Tests cover
  lost activation, restart, cancellation and move-back.
- Deployment gap found on current main: Railway hard-coded password-only startup.
  Optional OAuth variables and an offline, explicit existing-account identity link
  now provide a configuration path while preserving closed enrollment and policy.

The remaining operator inputs and live acceptance procedure are in
[Cloud MCP connections and qualification](../integrations/cloud-mcp.md). No live
configuration, identity mapping or deployment was changed by this implementation.


## Original audit

Reviewed October 11, 2026 against `codex/shared-accounts` at `766d9ff` and the then-current cloud deployment plans. This section records the findings before implementation.

Ding already implements hosted MCP for inspecting and managing watches in an approved cloud workspace. It does not yet give the local MCP adapter a cloud connection, expose cloud usage or workspace discovery as tools, or support moving a watch through MCP. The next step is to make the hosted path accurate and qualify it, then add explicit targeting and selected-watch transfer.

The [Railway plan](railway-cloud-plan.md) remains the deployment plan. Its initial built-in-account pilot defers public OAuth MCP onboarding. Browser login alone does not enable MCP: retain the separate OAuth boundary, and leave the endpoint disabled when it is not configured. This plan does not change that sequencing or establish public service availability.

## Existing support

| Interaction | Current implementation |
| --- | --- |
| Connect a remote model host to cloud | Optional `/mcp` endpoint, Streamable HTTP, bearer verification and protected-resource metadata |
| Inspect cloud watches and history | The same tools for watches, events, deliveries, destinations and operation receipts used locally |
| Create or change cloud watches | Preview/apply, pause/resume/delete and delivery retry reuse the existing reviewed operations |
| Restrict access | Verified issuer, subject and OAuth client; explicit user/workspace/client consent; current membership, OAuth scopes and integration grant checks |
| Enforce hosted execution policy | Tenant runtime validates cloud source/destination policy and applies execution quotas and guarded outbound requests |
| Choose a workspace | `X-Ding-Workspace`, or automatic resolution only when exactly one membership-backed client binding matches; ambiguity is denied |
| Revoke a model connection | Console removes the binding and revokes the grant; retained internal clients recheck the binding and membership |
| Use embedded views | Hosted MCP reuses the MCP Apps workspace when the UI bundle is present |
| Connect a local stdio adapter to a private shared server | Exported pairing file is supported for self-hosted servers; this is distinct from public cloud OAuth |

Implementation: [hosted MCP](../../internal/cloud/mcp.go), [model consent](../../internal/cloud/mcp_connections.go), [workspace bindings](../../internal/serverstate/mcp.go), [tool routing](../../internal/mcpserver/server.go), [cloud runtime](../../internal/cloud/pool.go), and [local shared-server pairing](../../internal/cloud/local_agents.go). The shared catalog currently contains 15 tools.

The hosted resolver calls the scoped integration API in-process. It does not send the cloud bearer token to a laptop or use the browser administrator forwarding path. Preserve that boundary when adding cloud features.

## Gaps to close

### Limits and permissions are incompletely described

`ding_get_capabilities` returns an instance ID and the stored grant, but no execution location, workspace identity, hosting policy, usage, or effective permissions. The embedded app enables management from `grant.scopes`. A reduced OAuth scope or downgraded membership can therefore leave management controls visible even though the server denies the action. This is a discovery and UI inconsistency; it does not establish an authorization bypass.

There is also a concrete size mismatch. [Capabilities and preview handling](../../internal/control/integrations.go) advertise a 1 MiB manifest and an 8 MiB fixture; [hosted MCP](../../internal/cloud/mcp.go) caps the entire request at 96 KiB. The cloud REST handler separately enforces a 64 KiB manifest cap in [api.go](../../internal/cloud/api.go), but the MCP in-process route does not pass through that handler. Cloud bundle validation checks watch content, not the raw manifest byte count. The hosted paths need one effective limit policy, including JSON encoding overhead.

### Hosted MCP skips the ordinary workspace request limiter

[Server routing](../../internal/cloud/server.go) dispatches `/mcp` before session authentication and the workspace rate limiter. Global request admission still applies, integration previews have a concurrency limit, and outbound execution remains budgeted. However, the ordinary per-workspace API request limiter does not cover hosted tool calls. Add authenticated workspace admission at the MCP boundary so one client cannot consume the shared admission budget unchecked by a tenant limit.

### Workspace and cloud operations are missing from the tools

The current catalog has no workspace discovery, cloud usage, transfer eligibility, or transfer status tools. Workspace selection depends on an HTTP header; a client approved in multiple workspaces receives the same generic denial used for other connection failures if it omits the header. There is no model-visible way to resolve this ambiguity.

[The local adapter](../../internal/mcpcli/command.go) loads one scoped pairing file and calls one integration API. `ding cloud login` uses a separate device credential and does not connect that adapter to hosted MCP. Pointing the pairing file at a cloud origin is insufficient: cloud deliberately does not accept exported self-hosted pairing credentials. There is no automatic local/cloud routing or combined view.

### Cloud onboarding and qualification are incomplete

Hosted consent currently requires entering a registered OAuth client ID in the Console. [CloudModels](../../web/console/src/pages/CloudModels.tsx) offers this setup based on hosting mode, without a feature flag proving that MCP is configured. Server instructions still say “durable local watches,” and the embedded app directs users to local pairing to enable management.

[The hosted MCP test](../../internal/cloud/mcp_test.go) exercises the resolver directly with constructed token information. It verifies consent, scope/client separation, a rejected private source and revocation. [HTTP adapter tests](../../internal/mcpserver/http_test.go) separately exercise JWTs and protocol behavior for the self-hosted adapter. A composed test of the actual cloud HTTP handler, verifier, workspace resolver and all tools is still needed, followed by real-provider and real-host qualification.

### Watch transfer exists outside MCP

The CLI already has prepare, test, finish, status and cancel flows, a durable transfer journal, source fencing, and recovery after an uncertain response. See [CLI orchestration](../../internal/watchcli/cloud_move.go), [transfer client](../../internal/cloudlink/transfer.go), and [cloud handoffs](../../internal/cloud/handoffs.go).

These operations use `/v1/handoffs`, outside the MCP integration allowlist. Handoff mutations require the `owners` action, which none of the four existing integration scopes grants. Exposing them requires a separate delegation design. Reusing `manage` or silently borrowing a CLI device credential would broaden existing consent.

## Implementation sequence

### Phase 1 Align policy and capability discovery

Keep the existing 15 watch tools. Extend capability results additively with execution mode, instance/workspace identity, effective permissions, supported features and effective request/manifest/fixture limits. Compute permissions from current membership, OAuth scopes and the integration grant; keep original grant details distinct. For account-free local operation, preserve existing pairing behavior and limits.

Enforce cloud manifest bytes and fixture limits on the scoped integration path as well as the REST path. Derive discovery and enforcement from the same policy. Keep the public request cap bounded and explicitly account for JSON escaping and envelope bytes. Add per-workspace admission after MCP identity and workspace resolution, retaining global admission for unauthenticated traffic. Return actionable limit errors without weakening uncertain-write reconciliation.

Update the embedded app and tool instructions to identify where the watch runs and use effective capabilities. Expose whether hosted MCP is configured to the Console, and hide unavailable OAuth setup. Coordinate that feature discovery with Railway's separation of login methods from hosting policy.

**Acceptance:** local, shared-server and cloud capabilities describe their actual restrictions; reduced scopes and role changes remove unavailable controls; payload boundaries agree across transports; hosted tool bursts respect workspace admission without blocking another workspace's ordinary calls. Add regression cases for the currently inconsistent size and permission responses.

### Phase 2 Add workspace selection and cloud inspection

Add a hosted `ding_list_workspaces` tool that lists only the verified user's approved client bindings with current membership and usable grants. Resolve this before opening a tenant runtime. Add an optional `workspace_id` argument to workspace-scoped tools and preserve the existing single-workspace default. Retain the header for configured clients, but reject conflicting header and argument values. Omission with multiple valid choices returns a specific selection-required result.

Resolve the target on every call; do not introduce a shared mutable active workspace. Include target identity in results, previews and operation recovery views. Keep receipt keys, pagination cursors and preview handles scoped to their original workspace. An explicit target on an incompatible local connection must fail clearly rather than be ignored. Version the additive contract changes and update schema fixtures deliberately.

Add a read-only `ding_get_cloud_status` tool backed by a narrow scoped endpoint. Return workspace usage, monthly limits and reset period, execution restrictions, and relevant health/storage pressure. Add authorized handoff status and offline eligibility inspection when their scoped contracts are ready. Keep secret values and full account exports outside model results; expose approved credential reference names through existing grant metadata.

**Acceptance:** a user can discover and inspect either of two approved workspaces; concurrent calls cannot cross targets; revoked/expired/unapproved workspaces are excluded; wrong-target handles and receipts fail; usage and exhausted-budget states are understandable through text and the embedded app.

### Phase 3 Qualify the hosted connection

Exercise `Server.Handler` with `MCPHandler`, a test issuer and real temporary tenant stores. Cover initialization, tool listing, UI resources, all watch tools, reviewed writes and same-key receipt recovery through actual HTTP. Include wrong audience/issuer/client, missing scopes, expiry, consent removal, membership downgrade/removal, multiple workspaces, restart, policy rejection, body limits, cancellation and tenant admission.

Document one complete connection flow using the selected real identity provider and each supported model host: client registration, login, workspace consent, refresh, grant expiry, reconnect and revocation. Confirm stable browser/MCP subject identity and exact resource audience. Enable the public endpoint only for the configured OAuth profile. Built-in-only pilot accounts remain usable through Console and CLI until that profile is deliberately enabled and qualified.

Update the integration guide, packaged setup/workflow instructions and embedded help with distinct local, self-hosted and cloud paths. Record actual host/provider results in integration verification; generated plugin archives and synthetic issuer tests remain separate evidence.

**Acceptance:** a clean user connects a real supported host, creates a cloud watch through preview/apply, observes remote delivery with the laptop off, reconnects after token refresh and loses access immediately after consent removal. Revoked access must not be recreated by a still-valid JWT.

### Phase 4 Connect local MCP to cloud explicitly

For model hosts that support remote MCP, use a separate cloud connection first and retain local Ding as its own connection. The local plugin should explain the cloud connection path and show which instance it is using.

For a single local adapter that can access both, add an explicit cloud profile and user-approved connection flow. Use credentials intended for that bridge, stored privately and scoped to the selected workspace and client. Do not inherit local administrator authority, silently reuse an existing CLI login, or reinterpret browser/device tokens as public MCP bearer tokens. Prefer reusing the qualified hosted MCP boundary; keep token refresh and transport code outside the watch runtime.

Add explicit connection targeting before combining results. Persist the target with pending writes, previews and receipt lookups. A missing, expired or unreachable selected connection must not fall back to another instance. Cloud sign-in cancellation leaves local monitoring intact, and local operations remain available offline.

**Acceptance:** identical watch IDs in local and cloud stores cannot confuse reads or mutations; parallel conversations do not switch each other's targets; refresh/revocation and same-key recovery survive adapter restart; local-only users acquire no account dependency.

### Phase 5 Expose reviewed watch transfer

Reuse the existing transfer state machine and journal through a narrow integration service. Start with read-only eligibility and status. Add a separate move permission, constrained by current ownership on the remote side and explicit local consent; existing manage grants gain no transfer authority automatically.

Expose prepare, delivery test, finish, status and cancel as distinct operations with durable transfer IDs. Bind review to both instances/workspaces, source revision/generation, destination configuration and the exact information leaving the laptop. Keep eligibility checks local before authentication or upload. Secret rebinding happens in protected setup; the model receives references and readiness only.

Preparation keeps the source running and target paused. Finish requires the user's delivery confirmation and transfer authorization, then reuses source fencing, activation and observation confirmation. A hosted-only endpoint must direct the user to their local connection for source-side actions; it cannot operate a sleeping laptop. An unreachable target never proves it is safe to resume the source.

**Acceptance:** eligible move and move-back succeed; unsupported/private sources, stale reviews, pending work and missing credentials fail before activation; duplicate calls, lost responses, restart and cancellation preserve one execution owner. Tests confirm that sign-in or preparation alone cannot pause the source.

## Scope and ordering

Ship Phases 1–3 as the hosted MCP milestone after the relevant Railway identity work. Phase 2 and the HTTP qualification harness can progress after Phase 1 establishes the shared contracts. Phases 4–5 form a later local-to-cloud workflow milestone; direct hosted watch management does not depend on transfer.

Credential creation, OAuth consent, account deletion, membership administration, service credentials and host operations remain in their existing protected browser/operator flows. Ordinary webhook, Slack and Discord delivery remains the notification path. Chat wakeups, billing and automatic synchronization are separate features.

## Verification for this review

The following existing package suites passed with the race detector on October 11, 2026:

```sh
go test -race ./internal/cloud ./internal/serverstate ./internal/mcpserver \
  ./internal/mcpclient ./internal/mcpauth ./internal/mcpcontract
```

This establishes the current test baseline. The limit/admission/capability findings above come from the inspected request paths; the proposed regression tests are not implemented by this plan. Live provider login, deployed cloud behavior, actual model-host interoperability and tagged UI qualification were not exercised in this review. Future changes must also run the relevant control/runtime tests, contract checks and Console/MCP UI checks before release.
