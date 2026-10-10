# Ding plugins

This is the source for official marketplace packages, kept in the Ding monorepo.
It does not create a personal marketplace or install anything into this checkout's
LLM client. Shared skills and assets are copied into each release bundle by
`scripts/package-integrations.py`.

- `claude/ding`: Claude's native manifest. Native bundles include a compiled
  FastMCP runtime and a **Ding Setup** launcher under `runtime/` (no top-level
  `bin/`). Remote bundles use a fixed, explicitly supplied HTTPS endpoint.
- `chatgpt/ding`: OpenAI's portable manifest with onboarding and UI metadata.
  Its `mcp.json` is generated only when a real self-hosted endpoint is supplied.
- `shared`: canonical workflow skills and vector assets.

Source directories are packaging inputs, not installable release directories.
The builder refuses missing runtimes, non-HTTPS remote endpoints, existing output
directories, and credentials embedded in endpoint URLs. Credentials are never
included. It emits a package checksum and qualification manifest.

Marketplace publication remains gated by publisher verification, host review,
native signing/notarization where required, and approval of the local/self-hosted
connection route. A generated ZIP is not evidence of directory acceptance. See
[release qualification](../docs/integrations/release.md).
