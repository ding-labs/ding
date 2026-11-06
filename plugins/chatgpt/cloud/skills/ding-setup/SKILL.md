---
name: ding-setup
description: Connect a Ding Cloud account through ChatGPT's browser authentication flow and establish the user's cloud workspace. Use for Ding Cloud connection and onboarding.
---

Use ChatGPT's Connect flow for this plugin. The configured endpoint is Ding Cloud;
customers do not need a server address, client ID, terminal or local daemon.
Authentication and permission approval belong in the browser. Never ask for a
password, token, client secret or notification credential in the conversation.

After authentication, call `ding_get_profile` with no arguments. Its immutable
`id` identifies the Ding account; a display name or OAuth email alias does not.
Do not ask the user to choose between email aliases or reconnect a working
account. Then call `ding_list_workspaces`. Use the single approved
workspace automatically; if there are several, let the user choose by name and
retain its `workspace_id`. Call `ding_get_capabilities` to establish effective
permissions and limits, then inspect watches and destinations as useful.

An OAuth scope challenge can trigger ChatGPT's reconnect flow. A denied Ding
workspace grant or missing membership needs the account owner's attention; do
not repeatedly sign in or broaden consent. Report an unavailable service clearly.
Never claim the connection is complete until an authenticated tool call succeeds.

Offer a public service health watch: ask only for the URL and notification
channel still missing from the request. Use the [watch skill](../ding-watch/SKILL.md)
for destination reuse, protected setup, typed preview, activation and first-check
verification. Connection alone does not authorize a watch or a test notification.
Do not create a demonstration watch just to prove the connection.

For a requested first watch, list saved destinations before offering new setup.
Call `ding_get_destination_setup` for the chosen destination (or omit
`destination_ref` when none exists). Give its exact `setupURL` as a clickable link.
The protected page matches the account and workspace, lets the user reuse or save
a destination, send a labeled test, and record actual receipt. Never request
webhook URLs or secrets in chat. A provider accepting the test is not proof the
user received it. Do not claim receipt or perform confirmation on their behalf.

Ask the user to return here after setup. On return, refresh profile, workspace
and destination setup status; do not infer completion from their return or a
previous conversation. If the browser shows a different account/workspace,
explain the mismatch using the stable IDs and the selected workspace name. Do
not silently switch accounts, expand consent, or overwrite the selected target.
Only current `confirmed` status completes guided notification readiness; pending,
failed, stale or expired status needs the protected page's recovery flow.

Ding Cloud stores the authoritative watches and evidence. Requested tool results
are shared with ChatGPT. Disconnecting access does not pause or delete watches.
