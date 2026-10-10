# Local qualification evidence

Verified October 10, 2026 on macOS ARM64. These are local development results,
not a claim of marketplace acceptance or testing inside actual ChatGPT/Claude.

| Area | Result |
| --- | --- |
| Existing Go runtime | `go test -race ./...` passed across all packages |
| Integration transaction boundary | Concurrent retries, restart/expiry, principal isolation, stale reviews, generation guards, revoked access, denied commands/secrets, receipt-quota rollback, and delivery retry deduplication passed |
| Python/MCP contracts | 28 tests passed, including a real daemon, native packaged stdio subprocess, HTTP OAuth checks, scopes, two-subject isolation, bounded transport behavior, and package assembly |
| Protocol compatibility | HTTP calls accepted for 2026-07-28 and 2025-11-25 negotiation formats |
| UI components | Three tests passed: explicit apply, stable operation keys including repeated primary-button clicks, expired previews, hostile text rendering |
| Browser protocol/UI | Official MCP Apps AppBridge test passed under restrictive CSP, with keyboard navigation, light theme, dark mobile layout, and zero external network requests |
| Native runtime | macOS ARM64 dependency-complete executable built and exercised against an actual daemon; served UI matches the final bundle exactly; no system Python required by the resulting executable |
| Container | Self-hosted image built from the frozen Python lock and bundled UI; read-only, network-disabled version check passed and HTTP startup without OAuth configuration was rejected |
| Workflow skills | Both shared skills passed the skill-creator validator |
| Source checks | Go formatting, Ruff checks/format, TypeScript compilation, UI bundle size, and Git whitespace checks passed |

Browser screenshots are generated under `web/mcp-app/test-results/`. Native
artifacts are under `dist/integrations/darwin-arm64/`; assembled packages are
under `dist/plugins/`. These generated outputs are intentionally not committed.

The added CI matrix covers native Linux AMD64/ARM64, macOS Intel/ARM64, and
Windows AMD64. Those remote jobs have not run as part of this local session.
Actual client installation, publisher review, signing/notarization, real IdP
login/refresh, sustained upgrade testing, and official marketplace publication
remain [release gates](release.md). Optional MCP Events/chat wakeups and a full
service installer/updater are not part of this implemented first version.
