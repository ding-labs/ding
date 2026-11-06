# Ding plugins

This is the source for official marketplace packages, kept in the Ding monorepo.
It does not create a personal marketplace or install anything into this checkout's
LLM client. Shared skills and assets are copied into each release bundle by
`go run ./cmd/package-integrations`.

- `claude/ding`: Claude's native manifest. Native bundles include a compiled
  Go runtime and a **Ding Setup** launcher under `runtime/` (no top-level
  `bin/`). Remote bundles use a fixed, explicitly supplied HTTPS endpoint.
- `chatgpt/ding`: OpenAI's portable manifest with onboarding and UI metadata.
  Its `mcp.json` is generated only when a real self-hosted endpoint is supplied.
- `chatgpt/cloud`: Ding Cloud listing, browser onboarding, cloud watch guidance and review cases. Build with `cloud-chatgpt`; the endpoint is fixed and publisher configuration contains no secrets.
- `shared`: local/self-hosted workflow skills and shared vector assets.

Source directories are packaging inputs, not installable release directories.
The builder refuses missing runtimes, non-HTTPS remote endpoints, existing output
directories, and credentials embedded in endpoint URLs. Credentials are never
included. It emits a package checksum and qualification manifest.

Marketplace publication remains gated by publisher verification, host review,
native signing/notarization where required, and approval of the local/self-hosted
connection route. A generated ZIP is not evidence of directory acceptance. See
[release qualification](../docs/integrations/release.md).

For the Ding Cloud marketplace route, follow the [ChatGPT publication runbook](../docs/integrations/chatgpt-cloud-publication.md).

For a private Cloud journey test, add `-private` to `-mode cloud-chatgpt`.
The public OAuth client configuration is still required, but absent listing URLs
are omitted rather than invented. The archive is labeled **Ding — Journey Test**
and **PRIVATE QUALIFICATION ONLY**; its qualification record has `private: true`.
Provided URLs still undergo credential-free HTTPS validation. Never submit this
archive for public publication. Build without `-private` and supply published
service terms, privacy and support URLs for public review.

An existing private ChatGPT development app has a host-assigned plugin name and
an `.app.json` binding. To update its skills without creating another OAuth
connection, download its ZIP from ChatGPT, then run:

```sh
python3 scripts/package-chatgpt-update.py \
  --existing /private/exported-plugin.zip \
  --journey /tmp/ding-cloud-chatgpt/ding.zip \
  --output /tmp/ding-cloud-chatgpt/existing-plugin-update.zip
```

The journey input must be built with `-private`. The adapter preserves the exact
app ID, plugin name and display name, ships the Cloud skills and assets, and
omits the standalone MCP definition. Upload the result with **Upload new version**
on that same private plugin. A matching name alone does not qualify discovery:
verify the skills and refreshed tools in a new conversation. This compatibility
format follows [OpenAI packaging](https://developers.openai.com/plugins/build/plugins).
