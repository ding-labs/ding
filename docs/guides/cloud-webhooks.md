# Native webhook connections

Native GitHub, Stripe and WorkOS adapters are implemented behind `push.nativeWebhooks`. They remain **disabled by default** until real provider qualification is complete. Generic scripts and GitHub Actions can use the [bearer push service](push.md) independently.

These adapters currently have an administrator REST API. A connection belongs to one workspace and one existing push watch lifetime. It stores an encrypted signing secret and explicit repository/account scope. The public connection ID is not a credential: signature verification authenticates every request over the exact received body. Provider connections do not expose their secrets in list responses.

## Administration

Use an owner/administrator browser session with its ordinary CSRF header and workspace selection. Producer bearer tokens cannot call these operations. Do not paste signing secrets into chat or commit them in example files.

| Method / route | Body / result |
| --- | --- |
| `GET /v1/webhooks` | Safe connection metadata in the current workspace |
| `POST /v1/webhooks` | Settings below; returns `connection` and its public `url` |
| `POST /v1/webhooks/{id}/rotate` | `secret`, `overlapSeconds` (0–86400) |
| `DELETE /v1/webhooks/{id}` | Disconnect; old signatures immediately lose access |

GitHub settings:

```json
{"name":"Repository CI","provider":"github","watch":"ci-results","repositories":["123456"],"secret":"SUPPLY_PRIVATELY"}
```

Stripe test-mode settings:

```json
{"name":"Billing events","provider":"stripe","watch":"billing-events","account":"acct_CONFIGURED_ACCOUNT","live":false,"events":["invoice.payment_failed"],"secret":"SUPPLY_PRIVATELY"}
```

Select real immutable IDs; repository names are descriptive. Secrets must be at least 16 bytes. New connections and secret rotation should be reconciled in the metadata list if a response is lost. Deletion/recreation of a watch requires a new connection.

## GitHub

Configure the returned `https://cloud.ding.ing/v1/hooks/github/{connectionId}` URL, JSON content type, matching webhook secret, and **Workflow runs** events in each allowed repository. Verification uses HMAC-SHA256 (`X-Hub-Signature-256`) over untouched bytes, and the signed payload's repository ID must be allowed.

Completed `workflow_run` events normalize to:

```json
{"repo":"owner/api","repositoryId":"123456","workflow":"test","runId":"456","attempt":1,"eventId":"123456:456:1","conclusion":"failure","failed":true}
```

Success produces `failed:false`. Failure, timeout, action-required, and startup-failure produce `failed:true`. Canceled, skipped, other nonterminal outcomes, and in-progress events get an explicit 200 `ignored:true, observations:0`; they cannot recover an incident. A valid repository ping is also acknowledged without observation. Unknown event types are rejected.

The delivery header supplies the receipt key, with a digest of the signed raw body. GitHub does not sign delivery/event headers; the adapter does not treat them as authenticated resource identity or a universal replay defense. Use normalized `eventId` with `new-event` when longer semantic deduplication is needed. Arrival order remains significant for transition watches.

GitHub expects a response within ten seconds and does not automatically retry failed deliveries. Inspect the delivery response/request ID, fix the issue, and request a provider redelivery within the receipt horizon. Do not automate provider API access without configuring its separate credentials and permissions. [GitHub delivery guidance](https://docs.github.com/en/webhooks/using-webhooks/handling-failed-webhook-deliveries)

## Stripe

Configure `https://cloud.ding.ing/v1/hooks/stripe/{connectionId}` for **snapshot events**, copy its endpoint signing secret privately, and select event types explicitly. The maintained Stripe Go SDK verifies `Stripe-Signature` against untouched bytes with its default five-minute timestamp tolerance. Clock synchronization is required.

The adapter checks `livemode`, configured event types, and `account` when supplied. For direct-account events that omit `account`, the endpoint signing secret is the account binding; use a secret from that configured account only. A mismatched mode/account is rejected. Thin events lacking `data.object` are rejected because this adapter does not make a provider API fetch. Unselected event types receive an explicit ignored response.

The original snapshot JSON is projected by the watch; for example `fields: {id: id, customer: data.object.customer}` and `condition: {field: id, operator: new-event, dedupFor: 24h}`. Use level policy with `interval: 0s`. Do not group on optional snapshot fields unless every selected event type supplies them.

Event ID is the receipt key. Exact redeliveries reconcile; a changed signed body with the same ID produces a digest conflict for inspection. Events may arrive late or out of order. Snapshot format, real test-mode delivery/redelivery, and rotation still require live qualification before advertising this adapter.

## WorkOS account signup alerts

WorkOS `user.created` reports AuthKit account creation. A later Ding browser or
MCP login creates the Ding workspace; these are separate steps. This adapter
alerts on registration, including people who never finish opening Ding.

The [signup watch](../../examples/cloud-push/workos-signups.yaml) uses the existing
`slack-ding-notifications` destination. Inspect that destination in the intended
operator workspace before applying the manifest. Each distinct provider event
queues a notification; the seven-day event deduplication horizon exceeds WorkOS's
documented three-day production retry window. Remote Slack delivery remains at
least once, so an uncertain receiver acknowledgement can still produce a repeat.

On an isolated qualification deployment with this adapter and
`push.nativeWebhooks` enabled:

1. Create the push watch and its Slack destination in the operator workspace.
2. Register the Ding connection through `POST /v1/webhooks`, using the ordinary
   owner/administrator session and CSRF protection. Generate a random provisional
   signing secret of at least 32 bytes in the private operator setup:

   ```json
   {"name":"Ding account signups","provider":"workos","watch":"ding-account-signups","events":["user.created"],"secret":"PROVISIONAL_SECRET_SUPPLIED_PRIVATELY"}
   ```

   Ding returns `https://cloud.ding.ing/v1/hooks/workos/{connectionId}`.
3. Choose **Webhooks** in the correct WorkOS environment. Create an endpoint for
   **only `user.created`** at Ding's returned URL. WorkOS supplies its own
   endpoint-specific signing secret. Replace the provisional Ding secret through
   `POST /v1/webhooks/{connectionId}/rotate` with `overlapSeconds: 0` and that
   WorkOS secret. Keep both secrets in the private operator setup; never put them
   in a manifest, shell history or chat. Deliveries fail signature verification
   until the actual WorkOS secret is installed; reconcile any setup deliveries
   in the WorkOS dashboard afterward.
4. Send a labeled WorkOS test event. Inspect Ding's accepted input, `new-event`
   evidence, delivery record and actual Slack message separately. Redeliver that
   exact event and confirm there is one durable notification intent. Test a new
   signup separately from a login to an existing account.
5. Repeat on the reviewed production deployment/environment after the isolated
   checks pass. Existing account records are not backfilled by this setup.

WorkOS requests require one `WorkOS-Signature` header. Ding checks HMAC-SHA256 over
the untouched body with the literal endpoint secret, and accepts timestamps only
within five minutes of server time in either direction. WorkOS timestamps use
milliseconds. The endpoint signing secret binds the WorkOS environment; never
reuse staging and production secrets. Rotation and revocation use the same
encrypted, workspace-bound connection lifecycle as the other native adapters.

Only `eventId`, `type` and `createdAt` reach retained observations and notification
payloads. Account IDs, names, emails and metadata are discarded. Other validly
signed event types are acknowledged with zero observations; malformed signup
snapshots are rejected. Exact raw-body redeliveries use the provider event ID as
the durable receipt key. Changed bodies with the same ID conflict explicitly.

References: [WorkOS webhook delivery and verification](https://workos.com/docs/events/data-syncing/webhooks)
and [webhook endpoint API](https://workos.com/docs/reference/webhooks).
