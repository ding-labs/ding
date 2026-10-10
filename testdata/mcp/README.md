# Portable adapter fixtures

These sanitized fixtures were captured from Ding's FastMCP 4.1 adapter before
its migration to the official Go SDK. They contain synthetic IDs, times, and
source text, never live credentials or instance data.

- `fastmcp-contract.json`: tool input/output schemas, explicit annotations, and
  MCP Apps resource metadata. The active `internal/mcpcontract/catalog.json`
  differs only by removing private FastMCP tags.
- `fastmcp-results.json`: real FastMCP client calls for all 15 tools, plus an
  invalid preview. Each entry records the call, expected scoped HTTP request,
  synthetic daemon data, and serialized structured result. Optional defaults
  and compatible extra fields are intentionally included.

Go tests compare the active protocol against these fixtures without a Python
installation. Required-field validation, hostile text preservation, text fallback,
malformed-response redaction, and uncertain mutation handling are also exercised.
Update fixtures only for an intentional public contract change.
