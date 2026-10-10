# Local qualification evidence

Verified October 10, 2026 on macOS ARM64. The adapter now uses the official MCP
Go SDK v1.8.0 and JWX v3.3.0. These are local development results, not marketplace
acceptance or testing inside actual ChatGPT/Claude.

| Area | Result |
| --- | --- |
| Portable contract | All 15 tool schemas/annotations and 16 structured/text result cases match captured FastMCP fixtures, including defaults, pagination, extra fields, and hostile evidence |
| Response failures | Malformed required data is rejected without exposing parser details; uncertain mutations retain same-key reconciliation and are never automatically retried |
| Daemon boundary | Real temporary daemon exercised through pairing, reads, preview/apply, receipt replay, lifecycle, evidence, and revocation; adapter serving imports no store/runtime package |
| Existing authorization | Concurrent retries, restart/expiry, principal isolation, stale reviews, generation guards, revoked access, denied commands/secrets, receipt-quota rollback, and delivery retry deduplication pass |
| Independent client | Official TypeScript client passes native stdio tools, UI-byte equality, both CLI entry points, mutation retries, revocation, and daemon survival after adapter close |
| HTTP | Both 2026-07-28 and 2025-11-25 formats pass, including concurrent identities, scope denial, binding rotation, discovery/challenges, and Host/Origin protection |
| JWT/JWKS | All six configured asymmetric algorithms pass; signature/claim/algorithm rejection, key rotation/removal, bounded unknown-key refresh, stale-cache failure, and redirect refusal pass |
| Pairing | A pairing created by the old packaged adapter works unchanged through both Go entry points; private files, existing-file refusal, bounded setup, nonce/Host/Origin/expiry checks, and concurrent pairing pass locally |
| Headless behavior | No UI resources or UI tool metadata are advertised without the bundle; unknown tools cannot reach the daemon; cancellation reaches the upstream request |
| UI components | Three tests pass: explicit apply, stable operation keys, expired previews, and hostile text |
| Browser UI | Two AppBridge journeys pass: restrictive CSP/light/dark/mobile/keyboard/offline behavior and actual native-Go-backed preview followed by explicit apply |
| Native package | Extracted macOS ARM64 ZIP is exercised by the independent TypeScript client; executable embeds the exact UI bundle and requires no installed Python/Node |
| Container | Scratch image contains the Go executable and CA certificates; read-only/no-network version smoke passes; missing OAuth config fails; real container verifies a synthetic HTTPS issuer and makes an authenticated HTTPS daemon call, rejecting missing/invalid tokens |
| Cross-compilation | MCP executable builds for Linux AMD64/ARM64, macOS Intel/ARM64, Windows AMD64; Windows private-ACL tests compile |
| Repository checks | Module tidiness, Go formatting/vet, normal and `console,mcpui` race suites, TypeScript build, bundle budget, workflow YAML parsing, and Git whitespace checks |
| Workflow skills | Both shared skills passed the skill-creator validator during the integration foundation work |

`make test-mcp` builds and tests the UI, runs the relevant Go race suites, and
executes the independent client and browser journeys. Install Playwright Chromium
once before running it locally. Container qualification is reproducible with:

```sh
docker build -f integrations/mcp/Dockerfile -t ding-mcp:qualification .
DING_MCP_CONTAINER=ding-mcp:qualification go test ./internal/mcpserver \
  -run TestContainerTLSAndAuthentication -count=1 -v
```

The container test uses temporary synthetic credentials and its own certificate
authority. It tests TLS verification and bearer authentication in the built
artifact; real identity-provider login/registration/refresh is a separate gate.

## Local artifact comparison

Same macOS ARM64 machine, version 0.1.0 and the same offline React UI. The
reference is the earlier dependency-complete FastMCP/PyInstaller qualification
artifact. Results are approximate and specific to these local builds.

| Measurement | Previous packaged adapter | Native Go adapter |
| --- | ---: | ---: |
| Runtime directory disk use | 55.6 MiB | 13.3 MiB |
| Plugin ZIP | 28.8 MiB | 5.0 MiB |
| Initialize plus first tool listing, median of 10 sequential subprocess runs | 508 ms | 15 ms |
| `--version` peak resident memory, one `/usr/bin/time -lp` sample | 53.8 MiB | 13.3 MiB |

The startup comparison uses the same independent SDK, temporary daemon, and
pairing. It includes process startup, MCP initialization, and `tools/list`.
It is not a cold-cache or sustained-load benchmark; the memory sample is a CLI
startup measurement, not steady-state server memory. Build time was not measured
comparably. Node remains a frontend build/test dependency; Python is removed
from integration source, dependency locks, builds, tests, packaging, and runtime.

Browser screenshots are generated under `web/mcp-app/test-results/`. Native
artifacts are under `dist/go-mcp/`; assembled packages are under `dist/plugins/`.
Generated artifacts and the temporary old runtime reference are not committed.

The native CI matrix covers Linux AMD64/ARM64, macOS Intel/ARM64, and Windows
AMD64, including extracted-artifact execution. Those remote jobs have not run
in this local session. Cross-compilation does not establish native behavior,
including Windows ACL enforcement. Actual client installation, publisher review,
signing/notarization, real IdP login/refresh, sustained upgrade testing, and
marketplace publication remain [release gates](release.md). MCP Events/chat
wakeups and a full service installer/updater remain outside this port.
