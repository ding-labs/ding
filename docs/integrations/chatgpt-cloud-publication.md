# Publish Ding Cloud in ChatGPT

The customer flow is install → browser sign-in and consent → return connected.
This runbook follows OpenAI's [authentication](https://developers.openai.com/plugins/build/auth),
[package](https://developers.openai.com/plugins/build/plugins),
[submission](https://developers.openai.com/plugins/deploy/submission), and
[plugin guidelines](https://developers.openai.com/plugins/plugin-guidelines)
reviewed on 2026-10-12. Recheck the portal before submitting; successful local
packaging is not publication approval.

## Finish the service first

1. Identify the existing Ding authorization provider or choose one. Prove S256
   PKCE, refresh, discovery, resource audience and signed token claims, plus a
   supported browser consent continuation. Reuse Ding's verifier.
2. Configure a ChatGPT client once for the publisher. Prefer CIMD when the
   provider supports it; a provided client also keeps customer setup to sign-in.
   Copy the exact callback shown in the ChatGPT portal. Put any client secret
   only in the provider and portal.
3. Integrate account/workspace approval into that authenticated continuation.
   Preserve existing accounts, create eligible new accounts once, skip workspace
   selection when only one is available, and support cancel/retry safely.
4. Add durable, revocable connected-app grants. Existing seven-day default grants
   would otherwise force avoidable reconnection. OAuth refresh does not renew a
   Ding grant. Test disconnect and membership removal against retained tokens.
5. Deploy the reviewed code and production OAuth configuration. Verify `/mcp`,
   resource metadata, issuer discovery, token audience and scope upgrades.
   Railway was pinned to `c5d9213` with no OAuth variables at the last inspection.

The server now declares OAuth scopes on each tool and returns ChatGPT's linking
challenge for insufficient scopes. Workspace/grant denials stay denials. This
code does not itself implement the provider's browser consent continuation.

## Assemble the cloud package

Create a local publisher JSON file with these fields using real configuration:

- `client.mode`: `provided`, `cimd` or `dcr`, matching the provider.
- For `provided`: `client.clientId` and `client.tokenEndpointAuthMethod` (`none`,
  `client_secret_basic`, `client_secret_post` or `private_key_jwt`).
- `supportURL`, `privacyPolicyURL`, `termsOfServiceURL`: published HTTPS pages.

CIMD/DCR omit the provided client fields. The builder rejects unknown fields,
including client secrets. The product website is `https://cloud.ding.ing` and
the MCP endpoint is fixed at `https://cloud.ding.ing/mcp`. These are publisher
settings; customers never supply them. The included technical disclosure is
not a substitute for the operator's published privacy policy and terms.

```sh
go run ./cmd/package-integrations -mode cloud-chatgpt \
  -cloud-config /private/ding-publisher.json \
  -version 0.1.0 -output /tmp/ding-cloud-chatgpt-0.1.0
```

The new output directory contains `ding.zip`, its checksum, and a qualification
record. The ZIP includes portable manifests, cloud skills, brand assets and
review cases. It contains no native runtime or local setup workflow. Confirm
public URLs are reachable and accurately describe the deployed service; URL
syntax validation does not establish that the pages exist.

## Qualify the actual customer journey

Use ChatGPT's custom MCP connection workflow for a private pilot, then upload
the resulting ZIP in the developer portal. Test with a fresh eligible account
and an existing account. Record the final package checksum and deployed commit.

Exercise install, sign-in, approval, cancellation, multiple workspaces, refresh,
missing scope, grant revocation, membership removal, disconnect and reconnect.
Repeat the essential flow on desktop and mobile. Test the first watch with no
notification destination: sign-in must finish successfully, and credentials
must be entered through the secure browser interface when needed.

Use a dedicated reviewer account with isolated sample data and usable sign-in.
Prepare a public HTTPS fixture, a configured test notification destination and
a running `review-health` watch with recorded firing/recovery events and delivery
history. The five positive and three negative cases in the package are review
procedures, not a claim they have passed. Run them in order; the interval change
and pause mutate this test watch. Restore the fixture between reviewer runs.
Supply reviewer credentials privately in the portal, never in the ZIP.

## Submit and publish

1. Verify the publisher identity. Serve the exact portal-provided domain token
   at `/.well-known/openai-apps-challenge` on the eligible HTTPS origin. This
   route still needs deployment configuration; do not invent the token.
2. Upload the ZIP with its MCP definition in the initial submission. Check the
   imported listing, auth registration mode, onboarding and review cases.
3. Confirm all four listing URLs, icon, accurate tool annotations and approved
   country availability. Enter the reviewer account and attach a real walkthrough
   recording. Do not put credentials into public review notes or recordings.
4. Complete required tool and skill scans. Fix findings, rerun affected tests,
   and update the ZIP if its metadata or skills changed.
5. Submit only after the full browser journey and review cases pass. Publish
   after approval; record the approved version and production configuration.

Runtime MCP changes can trigger automatic scans. Package skill or metadata
updates need a new ZIP. Recheck authentication after provider/client changes;
keep the registered client valid for existing connections.
