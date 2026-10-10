# MCP compatibility baseline

`fastmcp-contract.json` was captured October 10, 2026 from Ding's FastMCP 4.1.0
adapter using synthetic configuration. It contains all tool schemas, annotations,
and UI resource metadata, without HTML or real credentials. The portable runtime
catalog in `internal/mcpcontract` removes only FastMCP's private tag metadata.

The SDK contract test checks schema/metadata parity, explicit annotation booleans,
default arguments, and input validation through a real MCP connection. Integration
tests exercise results, errors, identity boundaries, and writes against a daemon.
