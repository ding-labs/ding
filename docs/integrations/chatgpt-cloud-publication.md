# Publish Ding Cloud in ChatGPT

The customer flow is install → browser sign-in and consent → return connected.
This runbook follows OpenAI's [authentication](https://developers.openai.com/plugins/build/auth),
[package](https://developers.openai.com/plugins/build/plugins),
[submission](https://developers.openai.com/plugins/deploy/submission), and
[plugin guidelines](https://developers.openai.com/plugins/plugin-guidelines)
reviewed on 2026-10-12. Recheck the portal before submitting; successful local
packaging is not publication approval.

## Finish the service first

1. Use **WorkOS AuthKit with Connect**. The [provider runbook](workos-authkit.md)
   records the configured staging client, public identifiers, deployment profile,
   and remaining qualification gates. The provider choice is complete.
2. Qualify the separate Console client and copy ChatGPT's actual callback URI
   into its provided public OAuth client. Hosted WorkOS sign-in and consent
   replace the proposed custom Ding consent page.
3. Qualify the full flow against WorkOS: fresh account, repeated access, refresh,
   cancellation, disconnect, and a genuinely new consent on reconnect. Local
   tests cover automatic personal-workspace creation and revocation tombstones.
   WorkOS consent reset/new `sid` behavior still needs live qualification.
4. Activate and configure a production WorkOS environment, then deploy reviewed
   code with `DING_AUTHKIT_JSON`. Verify `/mcp`, metadata, issuer, audience and
   scopes. Railway was pinned to `c5d9213` at the last inspection.
5. Keep the pilot to personal accounts with one workspace until organization and
   multi-workspace authorization have an explicit provider-to-workspace mapping.

The server declares per-tool OAuth scopes and returns ChatGPT's linking challenge
for insufficient scopes. For the configured AuthKit client, verified provider
consent automatically creates the account and connection. Requests renew the
private grant without reviving a disconnected consent. Customers do not need a
second Console setup screen.

## Assemble the cloud package

Create a local publisher JSON file with these fields using real configuration:

- `client.mode`: `provided` for the initial WorkOS integration.
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

Exercise install, sign-in, approval, cancellation, rejection of unsupported workspace contexts, refresh,
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
   route is configured with `DING_OPENAI_APPS_CHALLENGE` on Railway (or
   `openAIAppsChallenge` in private server JSON). Leave it unset until the portal
   supplies the token; the endpoint then returns 404. It serves one exact token
   as plain text on the canonical host, with no JSON wrapper or newline.
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
