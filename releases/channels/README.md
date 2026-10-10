# Qualified watch release channels

No qualified watch-runtime channel is published yet. The legacy stable release
remains v0.14.0. Do not create placeholder metadata or advance a stable pointer
to make an installation test pass.

`stable.json` and `preview.json` are exact-byte, detached Ed25519-signed metadata;
the adjacent `.sig` contains base64. The public key is pinned into release
binaries with `-X github.com/ding-labs/ding/internal/update.PublicKey=BASE64`.
Development builds have no trust root and refuse online updates. Keep the private
signing key outside the repository and release artifacts, in a protected signing
environment. The same public key must be compiled into `ding-release`.

After signing/notarizing native packages and passing the release gates, run:

```sh
go build -ldflags "-X github.com/ding-labs/ding/internal/update.PublicKey=$DING_UPDATE_PUBLIC_KEY" -o dist/ding-release ./cmd/ding-release
dist/ding-release --version vVERSION --channel preview --artifacts dist --key-file /protected/update-signing.key
```

Review and publish the metadata in a separate release PR. Metadata expires in 30
days; a scheduled operator procedure must re-sign unchanged eligible releases
before expiration. Expiry prevents accepting indefinitely replayed channel data;
it does not disable local watches. Rollbacks and schema-changing upgrades require
an explicit migration procedure, never a rewritten version number. Key rotation
requires a release trusted by the existing key before retiring that key.

Qualification must include actual N → N+1 archives, failed readiness recovery,
macOS notification helper signatures, Windows installer upgrades, schema
compatibility, pending deliveries, and preservation of MCP grants. A checksum
alone is not evidence of publisher authenticity.
