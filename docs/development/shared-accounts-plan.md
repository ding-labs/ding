# Personal, shared-server, and cloud accounts

Proposed October 11, 2026. **Implementation plan; these capabilities are not yet shipped.**

Give a company individual user accounts on its own Ding server while keeping personal startup account-free. Use the same user, workspace, permission, and Console contracts in self-hosted and cloud deployments. Authentication does not move watches or connect an installation to Ding Cloud.

The working delivery choice is built-in accounts for the first shared-server release, then configurable company OpenID Connect (OIDC) sign-in. The cloud's existing OIDC sign-in continues to work throughout this work and supplies the reusable foundation. The first shared-server release serves one shared workspace. The data model supports multiple memberships immediately; additional self-hosted workspaces follow after that first release is qualified.

The owner is currently the only user, with no existing service migration requirement. Target fresh state and update the schema, clients, and test fixtures together. Do not spend this project on legacy database upgrades, compatibility shims, or personal-to-shared conversion tooling.

This extends the [local-first roadmap](local-first-roadmap.md) and [cloud plan](ding-cloud-plan.md). It does not authorize deployment or change the existing cloud launch gates.

## Product contract

| Deployment | Entry experience | Identity and ownership | Execution policy |
| --- | --- | --- | --- |
| Personal, the default | `ding setup` / `ding ui`; no registration or login form | Stable implicit local owner and one workspace | Existing local capabilities and machine credentials |
| Shared self-hosted server | Instance login page; built-in accounts initially, optional company SSO later | Named users with workspace memberships | Operator-controlled access to private services; host execution separately restricted |
| Ding Cloud | Same Console authentication shell, configured cloud identity provider | Named users with workspace memberships | Cloud egress, quotas, retention, and hosted services |

Model access mode (`personal` or `shared`), hosting (`self-hosted` or `cloud`), and enabled login methods separately. The cloud entry point always requires shared access. Binding a listener remotely does not silently change access mode, and an identity-provider failure never enables personal access.

Personal access continues to require the existing protected CLI handoff/session mechanism. It is not an unauthenticated administrator endpoint. A persistent local-owner identity is internal bookkeeping, with no email address or cloud registration required.

Anonymous guest access is deferred. If added, it will be an explicit workspace policy with restricted viewing permissions. Do not introduce an ambiguous `--anonymous` flag that can grant administrative access to network visitors.

Basic self-hosted accounts, workspace roles, and generic OIDC belong in the open-source product. Cloud capabilities are determined separately by the hosted service. No billing implementation is required for this work.

## Existing seams and constraints

| Current code | Reuse or required change |
| --- | --- |
| `internal/control/browser.go`, `internal/watchcli/ui.go` | Preserve the personal one-use launch link, session lifetime, exact origin checks, and separate ingest authentication. Add an explicit local-owner principal. |
| `internal/control/server.go`, `console.go`, `integrations.go`, `tools.go`, `console_system.go` | Inventory every route and enforce permissions for reads, writes, tests, backups, and system operations. Today's admin bearer is broader than a team member. |
| `internal/cloud/identity.go`, `login.go`, `state/sessions.go` | Reuse verified OIDC identity and session mechanisms; separate authentication from enrollment and workspace selection. Remove GitHub-specific assumptions from the reusable provider layer. |
| `internal/cloud/state/state.go`, `pool.go` | An account currently doubles as a workspace. Replace that model with separate users, workspaces, and memberships, and route the pool by verified workspace ID. |
| `internal/cloud/api.go`, `server.go` | Cloud requests currently forward into the watch API with an internal admin bearer. Carry a verified actor and effective permissions through this boundary without elevating a member to administrator. |
| `internal/cloud/account.go`, `state/accounts_delete.go` | Split user deletion, membership removal, and workspace deletion. Existing account deletion stops execution and removes workspace data. |
| `internal/cloud/mcp.go`, `state/mcp.go`, `internal/watchrun/integrations.go` | Bind connections to an actor and workspace, intersect scopes with current permissions, and preserve reviewed mutations and command/secret restrictions. |
| `internal/cloud/state/secrets.go`, `usage.go`, `handoffs.go`, `internal/cloudbackup` | Keep secrets, usage, transfers, writer fencing, and backup/restore attached to workspaces rather than individual members. |
| `web/console/src/app/App.tsx`, `execution.tsx`, `preferences.ts`, `api/contracts.ts` | Replace login decisions based solely on `local/cloud` with explicit authentication and capability discovery. Generated contracts remain generated. |

Keep the evaluator and watch manifests independent of account providers. Introduce `internal/access` for principals and permission policy. Promote the existing cloud login/provider/session services into `internal/identity`, moving their tests with them; do not implement a parallel session or OIDC system for self-hosting. Extract reusable metadata persistence from cloud state behind a shared store interface, retaining one authoritative schema owner per database. Keep cloud scheduling, egress policy, usage accounting, and launch controls in `internal/cloud`.

Do this incrementally; a wholesale package relocation is not a prerequisite for the first permission checks.

## Fit with the existing cloud auth layers

The current cloud code already has distinct authentication and authorization boundaries. Keep their credential types distinct and add shared workspace permissions after credential verification.

| Existing layer | Keep | Extend |
| --- | --- | --- |
| Browser OIDC: `identity.go`, `login.go` | HTTPS issuer/endpoints, verified signature/issuer/audience/subject, PKCE, nonce, browser-bound state, one-use callback, bounded discovery and exchange | Resolve a user, then permitted memberships. Cloud enrollment can create a personal workspace under the existing enrollment policy; company login requires configured admission. Built-in login produces the same user/session result. |
| Browser sessions: `state/sessions.go`, `server.go` | Hashed opaque tokens, expiry/revocation, browser/device kind checks, secure cookies, CSRF, exact Host/Origin checks | Store user identity separately from workspace; evaluate current membership on each request. A password login uses this session layer too. |
| CLI device flow: `devices.go`, `state/devices.go` | User-visible code comparison and consent, private verifier/challenge, one-use approval/claim, separate expiring device credential | Approve a named user, chosen workspace, and explicit scopes. Recheck permissions on claim and subsequent calls. A device token never becomes an MCP OAuth token. |
| Public MCP token validation: `internal/mcpauth/verifier.go`, `cloud/mcp.go` | Configured signature algorithm/JWKS, issuer, exact MCP audience, expiry, subject, scopes, signed client identity, protected-resource discovery | Resolve verified user and an explicitly bound workspace before resolving the internal grant. Do not accept a browser cookie, browser ID token, or device token as an MCP access token. |
| MCP consent and internal grant: `mcp_connections.go`, `state/mcp.go`, `watchrun/integrations.go` | Explicit client consent, encrypted internal connection, scopes, expiry, credential references, command revision restrictions, operation receipts, revocation, and the closed `/v1/integrations/` transport | Key binding by user + workspace + OAuth client, intersect with current membership, and require explicit workspace binding for each connection. Never choose whichever workspace was last used in the browser. |
| Hosted API boundary: `server.go`, `api.go`, `network.go`, `internal/hostingpolicy` | Rate/body/time bounds, allowed routes, manifest policy even for compile/test, reviewed apply, tenant isolation, egress/usage constraints | Forward a verified actor and effective permissions to the common API. Workspace ownership does not bypass cloud restrictions. |

The internal admin token in `serveTenant` is currently a transport bridge for an already authenticated, single-owner workspace. It is not itself an exposed credential or a demonstrated bug. Multi-user access requires changing that bridge: trusted in-process context must retain the actor and permissions, and downstream actions must enforce them. Do not accept actor/role headers from clients. The MCP path already uses a restricted integration grant rather than this broad bridge; preserve that distinction.

Cloud browser login and MCP currently require the same issuer and stable subject (`internal/cloud/run.go`). Preserve that relationship unless explicit authenticated identity linking is implemented. Keep the minimal GitHub broker scope policy in the cloud profile when the reusable OIDC layer gains company-provider options.

Use the existing tests as regression anchors: `TestOIDCVerifiesSignedIdentityPKCEAndNonce`, `TestLoginRequiresBrowserStateAndCannotReplay`, `TestSessionExpiryRevocationAndOneUseLogin`, `TestDeviceBrowserApprovalRequiresVisibleConsentAndCSRF`, `TestCloudBoundaryDerivesWorkspaceFromSessionAndRequiresCSRF`, and `TestHostedMCPUsesSeparateWorkspaceClientScopesAndRevocation`. Extend them with multiple members and revoked/downgraded roles, and retain the verifier's signature, claim, key-rotation, and bounded-refresh tests. Update fixtures to the new schema; retaining behavior does not require retaining old storage formats.

## Identity and workspace model

| Entity | Required meaning |
| --- | --- |
| User | Stable instance-local ID, display name, enabled/disabled state, creation time; survives changes to login method |
| Login identity | Built-in username/password credential or unique `(issuer, subject)` mapped to a user; never merge accounts solely by email |
| Workspace | Stable ID, name, runtime/store binding, lifecycle state, execution policy; owns watches, history, destinations, and credentials |
| Membership | Unique user/workspace pair with role and authorization version |
| Browser session | User, expiry, revocation state, CSRF binding; workspace access is checked against current membership |
| Device or integration grant | User or service principal, fixed workspace, allowed scopes/resources, expiry, revocation state, and creating actor |
| Service principal | Workspace-owned automation identity; persists across the departure of its creator and has its own revocable grants |
| Audit record | Actor, workspace, action, resource, outcome, timestamp, request/operation ID; no passwords, tokens, or credential values |

Use SQLite for identity metadata and the existing watch stores. Adding accounts does not require a database-engine migration or distributed execution. Personal installations can derive their stable local-owner identity from the existing store identity without creating a login database at startup.

For the first shared-server release, initialize its single workspace in a fresh state directory. Multiple cloud workspaces continue to use separate stores. Never select a store from an unchecked path, username, or client-supplied workspace ID.

Separate the host operator from workspace roles. Possession of the daemon's filesystem, encryption keys, or original admin credential is already host-level trust. Team members get individual credentials; they do not receive that admin credential.

## Permission contract

Use four workspace roles initially. Owners can also perform every administrator action.

| Action | Viewer | Editor | Administrator | Owner |
| --- | --- | --- | --- | --- |
| Inspect watches, events, retained evidence, delivery state; export definitions | Yes | Yes | Yes | Yes |
| Edit/apply ordinary HTTP or push watches; pause/resume/delete; retry deliveries | No | Yes | Yes | Yes |
| Change destinations, create/update credentials, authorize credential use | No | No | Yes | Yes |
| Invite/remove viewers and editors; manage workspace service grants | No | No | Yes | Yes |
| Add/remove administrators or owners; transfer ownership; delete workspace | No | No | No | Yes |
| Configure executable commands, filesystem access, host policy, restore, or service updates | No | No | Host operator only | Host operator only |

All workspace members can see its retained evidence in this version. Private watches, field-level permissions, and custom roles are deferred; use separate workspaces where data visibility must differ.

Credential use needs its own resource restriction: allowing an editor to reference a saved credential must not let them send it to an arbitrary source or destination. Bind approved usage to the relevant origin/destination and execution policy; changes require administrator review. A definition export contains references, not credential values. Enforce destination permissions even when destinations are embedded in a whole-bundle apply request.

Command watches in shared mode require host-operator approval of executable definitions. Editing, testing, importing, resuming, or recreating a command watch must respect that policy. Apply the same rule to any future filesystem/log sources. A workspace role is not an OS sandbox, and the shared process does not provide isolation against a malicious host operator.

Authorization is the intersection of current membership, grant scopes/resource restrictions, and deployment policy. Enforce it inside the common action boundary used by browser, CLI, MCP, and internal forwarding, with default denial for unmapped actions. Check permission again when committing a reviewed mutation. Serialize membership revocation with mutation admission so that after revocation completes, newly admitted operations cannot use old permissions; document that already committed work and in-flight delivery can finish.

Protect last-owner invariants transactionally, including concurrent demotions/removals. Losing membership revokes user-bound access but does not stop workspace watches or its separately authorized service principals. Disabling a user revokes all of that user's sessions and grants.

## Login, recovery, and administration

The first shared-server experience is self-contained and requires neither SMTP nor an external identity service:

1. The operator explicitly initializes shared mode from the host and creates the first named owner using a private terminal flow. Bootstrap succeeds once; subsequent setup uses authenticated administration or host recovery.
2. The owner creates users and chooses their roles. Ding provides bounded, expiring, one-use activation links for manual distribution. There is no public self-registration by default and no automatic message sending.
3. A user sets a password, signs in, and sees the shared workspace. Username normalization and uniqueness are fixed in the contract; email is optional contact metadata.
4. Users can change passwords and revoke their sessions. Administrators can disable access and issue one-use resets for users within their administrative scope. They cannot reset a higher-privilege user's account; cross-workspace users require instance-level recovery.
5. Host recovery resets access without replacing the workspace. Require filesystem ownership/exclusive maintenance access, revoke affected sessions and recovery tokens, and record the recovery event.

Hash built-in passwords with versioned, salted Argon2id credentials using a maintained implementation. Benchmark parameters at or above the [OWASP recommendation](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html), bound password input and concurrent hashing work, and rate-limit failed logins and activation/reset attempts. Avoid storing credentials in shell history or printing them to daemon logs.

Use random revocable browser sessions, secure HttpOnly cookies for shared HTTPS deployments, bounded expiry, CSRF protection, session rotation at authentication/reset, and exact Host/Origin validation. Preserve personal loopback behavior. Generalize the existing device approval flow for CLI sign-in without passing passwords through command flags. Shared access needs documented TLS/reverse-proxy setup; an unavailable authentication store fails closed.

Company OIDC configuration follows the built-in self-hosted release; cloud OIDC remains in the shared foundation from the start. Reuse the existing code/PKCE/nonce verification, make provider labeling and allowed scopes configurable, and support a company-operated issuer. Self-hosted enrollment must require an invitation or explicit operator admission policy. Valid authentication alone does not grant workspace membership. Link a second login identity only through an authenticated linking/recovery flow. Provider outage leaves watches running; host recovery remains available. SAML, SCIM, LDAP, and automatic group synchronization are later work.

## Console and client contracts

Add a small public `GET /v1/auth/config` discovery response containing hosting/access mode, instance display name, enabled login methods, and allowed entry routes. Do not expose users, memberships, secrets, or operational data. A failed discovery request shows a connection failure; it must not be interpreted as personal mode.

Extend `/v1/browser/session` with user identity, accessible workspace summary, selected workspace, role, and effective capabilities. Update the Go and client contracts together and generate TypeScript through `cmd/console-contract`. Add authenticated workspace/membership/user administration routes; freeze their method/permission matrix in the first implementation slice.

Browser requests carry an explicit workspace selection, checked on every request. Keep selection per tab/request rather than mutating one account-wide session setting; two tabs must be able to use different workspaces safely. A single available workspace is selected automatically. A device, service, ingest, or MCP grant remains bound to its approved workspace and cannot override it with a header.

Use one login shell with instance-appropriate fields or provider buttons. Personal launch opens the existing Console directly. Shared mode adds account/session controls, member administration, and role-aware actions. The server remains authoritative when a stale page attempts a forbidden action. Treat expired identity as `401`, authenticated denial as `403`, and inaccessible resource identities consistently without revealing another workspace's existence.

Partition queries, drafts, pending review receipts, and saved preferences by instance, user, and workspace. Cancel old requests and discard late responses during account/workspace changes. Never carry a prepared mutation into another workspace. Show where checks actually execute: this computer, the named self-hosted server, or Ding Cloud.

Add named remote CLI connection profiles with explicit endpoint and workspace selection, reusing `ding cloud login`'s device approval implementation. Keep the local CLI experience direct; a remote connection must not silently redirect local administrative commands. Update cloud CLI callers alongside shared endpoint changes rather than adding old-protocol compatibility layers. CLI credentials are user-bound and revocable. Ingest credentials remain separate and become explicitly scoped to workspace/watch in self-hosted mode; browser cookies and ordinary CLI sessions do not authorize ingest. This does not enable push sources in the current public-HTTP-only cloud profile.

For MCP, retain existing operation receipts, preview constraints, and explicit command/secret grants. Resolve the actor plus workspace, not just `(issuer, subject)` to an account. Two users using the same OAuth client must have distinct bindings. Local-password deployments can use the existing local adapter with an individually scoped connection; a public HTTP MCP endpoint still requires the supported OAuth authorization infrastructure. Building an OAuth authorization server is outside this plan.

## Audit and durable operations

Record user/membership/session administration in the identity metadata transaction. Record committed watch/destination/grant mutations with their actor in the same watch-store transaction as the mutation or durable receipt. Do not depend on a successful second database write to attribute an already committed operation. Merge these journals for a bounded audit view using store/source IDs; do not imply a total transactional order across databases.

Audit login failures and host recovery without logging secret inputs. Reserve capacity or fail privileged mutations visibly if their required audit record cannot commit. Define bounded retention and export in release documentation. Attribute personal-mode actions to the local owner; do not invent individual attribution for use of a shared host credential.

## Fresh setup and runtime lifecycle

Initialize the new identity schema directly and recreate development/test accounts and connections as needed. No existing-install upgrade, old-session translation, historical ciphertext migration, legacy account-delete endpoint shim, or automated personal/shared conversion is required. Existing local files are not automatically deleted: use a new state directory for qualification, and document any deliberate developer reset separately.

Persist the chosen access mode and reject mismatched startup configuration rather than falling back to personal mode. A private host operator credential must never authenticate as a team member through the shared public API. Shared credential lookup is workspace-scoped and cannot enumerate or resolve arbitrary process-environment secrets.

The new schema assigns secrets, usage, probes, transfer proofs, and watch stores to workspaces. It assigns sessions, device approvals, and user-bound grants to users with explicit workspace authorization. MCP binding encryption includes user/workspace/client context so a stored connection cannot be substituted across these boundaries.

Replace the account-delete operation with explicit user, membership, and workspace lifecycle operations, updating all callers in the same change. Removing a user must not delete their team's watches. Require ownership transfer before removing the last owner. Keep the cloud's durable deletion/recovery behavior for explicit workspace deletion.

Retain normal runtime durability: workspace encryption, writer fencing, usage reservation, transfer holds, and recovery of interrupted operations. Extend current backup/restore to the new metadata and audit schema, keeping keys separate and restores quarantined until runner ownership is resolved. Revocation of restored sessions/grants remains part of restore behavior. These are correctness requirements for newly created services, not historical migration work.

## Implementation sequence

Each item is a reviewable implementation slice, potentially several small PRs. Shared mode stays unavailable until its authorization and recovery gates pass. Exercise the existing cloud sign-in and MCP paths from the first slice onward.

| ID | Deliverable | Dependencies | Acceptance gate |
| --- | --- | --- | --- |
| A01 | Cloud auth regression inventory, principal/role/capability contracts, complete route/action inventory, explicit discovery contract, local-owner adapter | None | Every existing route has a permission classification; existing cloud browser/device/MCP auth tests remain covered; personal startup remains direct; unknown actions deny access |
| A02 | Extract reusable cloud identity/session services; fresh user/workspace/membership schema; explicit user/workspace deletion | A01 | Cloud OIDC creates the new user/session/workspace model; device and MCP fixtures use it; two users can share a workspace in integration tests; user removal does not remove watches |
| A03 | Common authorization enforcement, scoped grant ownership, credential-use policy, durable actor attribution | A01–A02 | Browser, CLI, MCP, direct API, tools, exports, and internal forwarding obey identical permissions; revoked users cannot use stale grants |
| A04 | Built-in password adapter using the shared session layer; one-time bootstrap, invitations/activation, reset/recovery, explicit shared configuration | A02–A03 | Two users authenticate to one workspace without external identity services; restart/recovery works; cloud still uses its existing OIDC path; no public bootstrap or operator-token bypass |
| A05 | Shared Console for self-hosting and cloud, membership/account UI, named CLI connections, workspace selection and cache isolation | A03–A04 | Both deployment profiles use the same role-aware views; owner invites viewer/editor; viewer cannot mutate; editor changes an allowed watch with audit attribution; two-user/tab isolation passes |
| A06 | Scoped ingest/automation, updated backup/restore, member-aware cloud transfers/quotas, operator docs, first-release qualification | A02–A05 | Fresh personal/shared/cloud journeys pass; host recovery and workspace restore work; cloud restrictions and transfer fencing survive the refactor; revoked grants fail through all entry points |
| A07 | Company OIDC configuration, admission and authenticated identity linking | A02–A06 | Company issuer and existing cloud provider share the same implementation; private-company enrollment is controlled; identity failure grants no fallback access |
| A08 | Additional self-hosted workspaces and operator lifecycle controls | A06–A07 | No cross-workspace data, secrets, tokens, caches, or machine privileges leak; resource limits and backup inventories remain bounded |

**First usable milestone:** A01–A06: a fresh single-workspace self-hosted installation with built-in accounts, four roles, attributed mutations, scoped CLI/agent/producer access, and recovery; the cloud uses the same membership/permission model with its existing OIDC and MCP authentication. Company-provider configuration and additional self-hosted workspaces are separate follow-ups.

Defer anonymous viewing, per-watch private ACLs, custom role builders, personal alert subscriptions, SAML/SCIM/group sync, distributed runtime, database-engine migration, billing implementation, and automatic cloud synchronization. Design does not require those features to ship useful shared accounts.

## Verification and release evidence

Use table-driven authorization tests for the real route/action matrix, plus end-to-end journeys. Follow [OWASP's permission-check guidance](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html) at every request and sensitive action boundary.

Required scenarios:

- Fresh personal setup through first watch and local MCP: no registration, network identity dependency, or permission regression.
- Fresh shared installation with two users, role changes, password reset, logout, restart, and host recovery; default-denied anonymous/API access.
- Direct requests bypassing hidden UI buttons; whole-bundle destination changes; live test/preview routes; backup/system routes; command and secret-reference changes.
- Last-owner concurrent removals; user disable and grant revocation; role downgrade after preview but before commit; service automation after its creator leaves.
- Cross-user/workspace IDs, cached results, late network responses, draft/review reuse, shared OAuth client IDs, CSRF, cookie confusion across instances, and forged forwarded identity headers.
- Editor attempts to redirect a saved credential; unauthorized command test/import/resume; environment-secret lookup and filesystem operations from shared APIs.
- Fresh-schema initialization and restart, encrypted-secret round trips, active incidents and pending deliveries, outstanding transfers, deleting workspaces, backup/restore, and user removal without workspace loss.
- OIDC issuer/subject mismatch, unapproved enrollment, identity linking, provider outage, and still-valid JWTs after membership/binding revocation.
- Named CLI connections and scoped ingest/MCP calls cannot fall back to host administrator access or silently target another instance.

For implementation, run focused Go tests as each slice changes, then the repository's release checks: `go test -race ./...`, `go vet ./...`, generated contract checks, `make test-console`, and `make test-mcp` where applicable. Build the Console and MCP assets before a final `go test -race -tags console,mcpui ./...` and both tagged binaries. Qualify the private-server HTTPS recipe on a real second machine and exercise built-in-account operation without an external identity service.

Record actual commands and results in a future implementation ledger; a plan or mocked-provider test is not deployment evidence. Public cloud availability remains governed by its existing operations, capacity, interoperability, and budget gates.

## Starting point

Start A01 by tracing and testing the existing cloud browser, device, and MCP paths, then add the route/action matrix and local-owner principal. Freeze permission names, workspace selection, and operator boundaries. A02 extracts those existing services and introduces the new schema with fresh fixtures. The built-in password form then becomes another entry into that shared system.
