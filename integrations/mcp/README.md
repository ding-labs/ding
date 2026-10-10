# Ding MCP

The native Go MCP adapter for Ding, using the official MCP Go SDK. The Go daemon
owns watch evaluation, authorization,
previews, and mutation receipts. The adapter owns MCP tools and embedded views.
Its source lives in
`internal/mcp*` and `cmd/ding-mcp`; this directory contains the self-hosted
container recipe.

See [the integration guide](../../docs/integrations/README.md) for setup and
[the implementation plan](../../docs/development/llm-integration-plan.md) for
platform constraints and marketplace qualification.
