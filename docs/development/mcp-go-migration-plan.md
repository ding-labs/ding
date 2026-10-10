# Ding MCP adapter migration to Go

Implemented October 10, 2026. Replace the Python FastMCP adapter with the official
`github.com/modelcontextprotocol/go-sdk` in the existing Ding monorepo. Preserve
the local and self-hosted product, the 15-tool contract, guided pairing, embedded
UI, and platform packages. Remove Python from integration development, build,
test, and runtime requirements after the Go implementation passes qualification.

The six implementation phases below are complete: the Go adapter, shared CLI,
authenticated HTTP, embedded UI, Go package assembler, and Python removal are
in place. The official Go SDK is pinned to v1.8.0 and JWX to v3.3.0. This replaces
the language/framework choice in the [original integration plan](llm-integration-plan.md).

Local qualification covers the Go runtime, independent TypeScript client,
browser bridge, macOS ARM64 package, and self-hosted container. Five platform
builds cross-compile; the configured native CI jobs and actual host/provider
qualification remain release gates. See [verification evidence](../integrations/verification.md).
The sections below retain the implementation requirements for future maintenance.

## Architecture and scope

The Go adapter continues to call the daemon's closed `/v1/integrations/*` API
with a scoped grant. It does not open SQLite, call privileged runtime methods,
inherit an administrator credential during serving, or own daemon lifetime.
The existing schema 4 grants, previews, operation receipts, permission checks,
and watch execution remain authoritative. No database migration is required.

```text
Local LLM host ── stdio ── Go MCP adapter ─────────────┐
                                                    │ scoped API
Remote LLM host ── HTTPS/OAuth ── Go MCP adapter ──────┼── Ding daemon
                                                    │
Embedded React UI ── host MCP bridge ── same tools ───┘
```

HTTP serving keeps the `--transport http` flag. TLS remains at the operator's
reverse proxy. There is no Ding-operated relay.

Use one implementation with two small command entry points:

- `ding mcp setup|pair|doctor|grants|serve` integrates with the existing CLI.
- `ding-mcp setup|pair|doctor|grants|serve` remains the plugin executable, compiled
  from the same command package. Keeping this narrow executable avoids bundling
  the full daemon and Console into each local plugin and preserves current
  launcher commands. Both share command behavior and report their build version.

Keep Node as a build/test dependency for the React UI. Ship only compiled Go
executables and bundled static assets; users install neither Python nor Node.

The migration does not add chat wakeups, OS service installation, new tools,
new OAuth providers, broader permissions, or marketplace publication. Those
remain separate product and release work.

## Repository layout

| Location | Responsibility |
| --- | --- |
| `internal/mcpserver/` | Official SDK registration, tool schemas, structured results, annotations, stdio, HTTP, and MCP Apps metadata |
| `internal/mcpclient/` | Bounded HTTP client for the scoped daemon API, typed responses, and sanitized errors |
| `internal/mcpconfig/` | Existing connection and HTTP config formats, private file access, default paths, validation |
| `internal/mcpauth/` | JWT/JWKS verification, OAuth metadata, request identity, and per-tool scope checks |
| `internal/mcpsetup/` | Local grant provisioning, short-lived browser setup, grant inspection/revocation |
| `internal/mcpcli/` | Shared Cobra command construction and command error handling |
| `internal/mcpui/` | Embedded setup HTML and generated MCP workspace HTML |
| `cmd/ding-mcp/main.go` | Thin plugin executable entry point |
| `internal/watchcli/` | Mount the shared command tree under `ding mcp` |
| `testdata/mcp/` | Sanitized protocol fixtures and compatibility expectations |
| `web/mcp-app/` | Existing React UI and browser tests; independent protocol client tests if needed |
| `internal/pluginpackage/`, `cmd/package-integrations/` | Go package assembler replacing the Python packaging script |

The adapter packages must not depend on `internal/store` or `internal/watchrun`.
Reuse suitable public API DTOs where possible, extracting a small neutral
contract package only if existing imports pull in privileged runtime code.
Do not expand this into a general API refactor.

## Phase 1 Establish the compatibility baseline

Pin the official Go SDK to **v1.8.0** initially, the latest stable release checked
for this plan. Its documented protocol support covers both 2026-07-28 and
2025-11-25, which our adapter already tests. Review changes before taking a newer
version during implementation. [SDK release](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.8.0),
[protocol compatibility](https://github.com/modelcontextprotocol/go-sdk#version-compatibility).

Capture language-neutral fixtures from the working FastMCP adapter against a
synthetic daemon. Include initialization/capabilities, all tool input/output
schemas, tool annotations, resources and UI metadata, structured and text
results, errors, OAuth metadata, config files, and CLI behavior. Normalize only
volatile values such as timestamps, generated IDs, and ports. Never record real
credentials. Compare JSON and validation behavior semantically rather than
depending on property ordering or schema reference names.

Keep these tool names and argument names:

| Scope | Tools |
| --- | --- |
| Inspect | `ding_get_capabilities`, `ding_list_watches`, `ding_get_watch`, `ding_list_events`, `ding_get_event`, `ding_list_deliveries`, `ding_get_delivery`, `ding_list_destinations`, `ding_get_operation` |
| Preview | `ding_preview_changes` |
| Manage | `ding_apply_changes`, `ding_pause_watch`, `ding_resume_watch`, `ding_delete_watch` |
| Retry | `ding_retry_delivery` |

Preserve `{view, data, query, evidenceNotice}`, pagination semantics, input
constraints/defaults, error codes, and `ui://ding/workspace.html`. Preserve the
current policy for additional daemon response fields while validating required
fields; ordinary Go decoding must not silently drop compatible new data.

Build a small SDK compatibility proof covering one read, one mutation, the UI
resource, and authenticated HTTP before porting every command. Exercise typed
schema generation with the actual enums, integer bounds, UUID-like operation
keys, regexes, required fields, nullable values, and empty arrays. A malformed
response must not become a plausible zero-valued success.

Test effective annotation values and advertised capabilities explicitly. The
SDK documents limitations around false-valued annotation serialization and
default capabilities. Resolve any host-required explicit fields before cutover;
do not fork the SDK or silently weaken declarations. [SDK rough edges](https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/docs/rough_edges.md).

**Exit:** both implementations can be judged against the same contract, and no
unresolved SDK limitation blocks our existing tools, UI, or transports.

## Phase 2 Port the local adapter and commands

Implement the scoped HTTP client with the existing 5-second connection and
35-second request budgets, a 16 MiB decoded response limit, cancellation, and
bounded concurrency. Disable ambient proxy settings and redirects. Keep the
route allowlist and ID validation, including encoded path traversal cases.

Preserve operation keys, preview handles, revision/generation guards, and error
classification. Never automatically repeat a mutation after an uncertain
transport outcome. Return `outcome_unknown` with the same-key reconciliation
instruction; inspect operation receipts before deciding how to retry. Treat
truncated or unreadable write responses conservatively, even if the connection
itself succeeded. The daemon continues to commit effects and receipts together.

Register the 15 tools through the SDK's typed `mcp.AddTool`, supplying explicit
schema constraints where inference does not match the existing contract. Use
the SDK for protocol framing, negotiation, lifecycle, and error responses.
Reserve stdout exclusively for MCP traffic in stdio mode; diagnostics go to
stderr. Closing the adapter must leave watches running.

Port `pair`, `doctor`, and `grants`, preserving flags and config fields
(`daemon_url`, `token`, `grant_id`; HTTP fields remain unchanged). Match existing
platform-specific default directories with fixtures, including the distinction
between Ding daemon state and MCP config paths on Windows. Existing valid
pairings should work without issuing a new grant.

Preserve exclusive creation, private permissions, ownership checks, regular-file
and size checks, symlink rejection, and cleanup/revocation on failed pairing.
Validate Windows ACL behavior rather than treating `chmod(0600)` as sufficient.
Administrator credentials are read only for explicit pairing/grant management
commands and never become serving credentials or tool parameters.

Port guided setup with its existing HTML, loopback ephemeral port, 10-minute
expiry, fragment nonce, constant-time nonce validation, exact Host/Origin
checks, bounded JSON bodies, CSP, and no credential display. Bind before opening
the browser, shut down cleanly on success/cancellation/expiry, and serialize
pairing so simultaneous POSTs cannot create duplicate grants. Invoke OS browser
launchers without shell interpolation.

**Exit:** current local workflows pass against a real temporary daemon; an
existing Python-created pairing works through both Go entry points.

## Phase 3 Port authenticated self-hosted HTTP

Use the SDK Streamable HTTP handler in stateless mode. Use its
`auth.RequireBearerToken` middleware and protected-resource metadata handler,
with a maintained JWT/JWKS library behind the verifier interface. Select and pin
that library during Phase 1 after checking algorithm support, key rotation,
timeouts, and cache controls; do not implement cryptographic primitives.
[Official SDK auth primitives](https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/auth/auth.go).

Preserve the configured issuer, audience, asymmetric algorithm allowlist,
required expiration, subject binding, and `ding:inspect` baseline. Require
`ding:preview`, `ding:manage`, or `ding:retry` for the appropriate tools, followed
by the daemon's grant check. Verify the actual JWT algorithm names against the
old config, including the Ed25519/EdDSA naming distinction; any necessary alias
must be explicit and tested.

Only the configured HTTPS JWKS endpoint may supply keys. Ignore token-provided
key URLs, validate signature/issuer/audience/expiry before using claims, bound
key fetches and caches, and test key rotation and unknown-key refresh without
allowing unbounded refresh traffic. Fail closed on unverifiable tokens. Sanitize
verifier errors before middleware can put them into an HTTP response.

Set request identity from the verified issuer and subject, keep it immutable and
request-local, and reload that subject's private Ding connection for each tool
call. Do not forward the OAuth token to Ding or share a mutable active user.
Exercise two identities concurrently, including credential rotation and grant
revocation during an existing host connection.

Preserve `/mcp`, `/.well-known/oauth-protected-resource/mcp`, public resource
identity, scope metadata, and authentication challenge headers. Test the
middleware's standards-compliant 401/403 distinctions; fixture differences must
be deliberate and verified with clients rather than forcing Python's exact
HTTP status for every invalid token.

HTTP startup still requires explicit private config and validates subject files.
Keep loopback binding by default, intentional container binding, exact public
Host validation, Origin protection, body/time limits, and trusted reverse-proxy
configuration. Public discovery metadata does not expose credentials. Logs must
not contain authorization headers, private config, or source evidence.

The identity provider continues to own login, registration, consent, and token
issuance. A controlled test issuer validates our integration; real-provider and
real-host login/refresh remain required for release qualification.

**Exit:** both supported protocol formats work over HTTP, isolation and auth
failure tests pass, and malformed config cannot start an unauthenticated server.

## Phase 4 Preserve and embed the MCP Apps UI

Keep the React application, view names, tool results, host bridge, and design
tokens. Implement the protocol metadata that FastMCP's `AppConfig` currently
generates: resource URI, MIME type, UI association/visibility, CSP, and display
preferences. Serve the existing offline HTML through the SDK resource handler.
The UI protocol is independent of server language. [MCP Apps architecture](https://modelcontextprotocol.io/extensions/apps/overview).

Change the UI build output to `internal/mcpui/dist/workspace.html` and embed it
with Go. During side-by-side qualification, feed the identical generated bytes
to both adapters. Retain the 1 MiB UI budget and verify the resource bytes match
the final build exactly.

Use an `mcpui` build tag, following the existing Console embedding pattern.
Normal Go development/headless builds remain possible without installing Node;
they expose text tools without advertising a missing UI resource. Every official
plugin/release build includes `mcpui`; CI must fail if its bundle is absent or
stale. Main Ding release builds use both `console` and `mcpui` tags. Setup HTML
is embedded separately and remains available without the React build.

Run the existing component and restrictive-CSP AppBridge browser tests, then add
a browser journey backed by the actual Go MCP server instead of only a test
bridge. Check light/dark/narrow layouts, keyboard operation, hostile text,
explicit apply, same-key retry, and text fallback.

**Exit:** the same UI works through both transports and the native executable
serves the exact qualified bundle without external network requests.

## Phase 5 Replace packaging and release tooling

Replace PyInstaller with ordinary Go builds. Target the current qualification
matrix: Linux AMD64/ARM64, macOS Intel/ARM64, and Windows AMD64. Build with
`CGO_ENABLED=0` where supported and verify the resulting dependency requirements;
cross-compilation does not replace executing tests on each advertised platform.
Do not add Windows ARM64 qualification claims solely because compilation works.

Replace `scripts/package-integrations.py` with the Go assembler. Preserve plugin
manifests, skills, assets, setup launchers, remote endpoint validation, archives,
checksums, and unsigned/unapproved qualification metadata. Preserve the local
plugin executable path where practical; the Python `_internal` directory and
framework symlinks disappear. Generate and validate launcher paths, executable
permissions, and versions from one build version.

Update `Makefile`, `.github/workflows/integrations.yml`, `.goreleaser.yaml`, the
main release workflow, Docker builds, `.gitignore`, and `.dockerignore`. Replace
the Python MCP image with a Go image carrying CA certificates and a non-root
user, compatible with a read-only root filesystem and read-only config mounts.
Keep actual daemon execution independent from the adapter container.

Record native archive/unpacked size, startup latency, idle RSS, and build time on
the same machine and inputs as the existing adapter. The current macOS baseline
is approximately 56 MiB unpacked runtime and a 29 MiB plugin ZIP; measure Go
before making improvement claims.

**Exit:** extract each plugin artifact on its target OS, run setup/doctor/stdio
tests, verify checksums, and confirm neither Python nor Node is required at
runtime. Container version and authenticated HTTP checks pass.

## Phase 6 Cut over and remove Python

Run the Python adapter only as a temporary compatibility reference using
synthetic instances. Never mirror live mutations into two adapters: compare
independent test instances or replay only identical keys where explicitly
testing the daemon's receipt semantics.

Move protocol and integration tests to Go and use an independent official
TypeScript MCP client in dev tests to catch assumptions shared by the Go server
and Go client. Keep the existing React/browser tests. Preserve all meaningful
Python assertions before deleting their implementation; the number of tests
need not remain 28.

Once the local, HTTP, UI, and packaging gates pass, switch plugin commands,
development instructions, and CI to Go. Continue using the existing config files,
grants, and database unchanged. Keep a known compatible Python artifact only as
a temporary rollback reference; do not automatically download it or fall back
to it at runtime. Rollback must preserve the same credentials and operation keys.

Remove `integrations/mcp` Python sources, lockfiles, entry point, Python tests,
PyInstaller builder, and Python package assembler after their replacements are
qualified. Keep any useful language-neutral deployment documentation under the
integration directory. Search active docs, workflows, package scripts, and
developer targets for leftover `uv`, Python, PyInstaller, and FastMCP dependency
requirements. Historical design discussion may still mention FastMCP.

Update the original integration plan, setup/API guides, release checklist,
verification evidence, and workflow skills to describe the Go implementation.
Only update verification claims for checks actually completed.

**Exit:** integration build/test/runtime paths need Go and frontend build tools,
with no Python dependency, and existing users do not need to re-pair.

## Qualification and completion criteria

| Boundary | Required evidence |
| --- | --- |
| Protocol | All 15 tool contracts, effective annotations, structured/text results, UI resource, legacy/current negotiation, invalid inputs, unknown tools, cancellation, and clean stdio |
| Daemon safety | Existing Go race suite plus stale previews, concurrent retries, lost responses, restart, generation guards, receipt lookup, revoke, command/secret restrictions, and denied admin routes |
| Pairing | Old config compatibility, private file handling on all OS targets, existing-file refusal, failed-pair cleanup, setup CSRF/nonce/expiry/concurrency, no credential output |
| HTTP | Issuer/audience/signature/expiry/algorithm/scope rejection; metadata/challenges; key rotation; bounded fetches; concurrent identity isolation; Host/Origin/body limits |
| UI | Existing component/browser journeys plus Go-backed tool calls, artifact-resource equality, restrictive CSP, and text-only fallback |
| Distribution | Extracted native packages, setup launchers, container TLS/auth, checksums, clean-machine operation, all advertised OS jobs green |
| Regression | `go vet`, appropriate normal and `console,mcpui` race suites, existing Console contracts, and independent-client interoperability |

The largest risks are JWT/JWKS behavior, schema differences between Pydantic and
Go, private file handling across operating systems, and stale embedded assets.
The corresponding tests are prerequisites for cutover, not cleanup afterward.

Implement the phases as six reviewable changes: baseline and SDK proof; local
adapter and commands; authenticated HTTP; UI embedding; native distribution;
cutover and Python removal. Phase 1 settles SDK/library compatibility before the
larger port. Phases 2–4 may share fixtures, but every final artifact must contain
the same qualified implementation and UI.

Completion of the port means feature parity and Python removal. Signing,
notarization, actual client install/login qualification, approved self-hosted
marketplace routing, and publisher approval remain the existing [release
gates](../integrations/release.md). Changing languages does not satisfy them.
