---
name: ding-setup
description: Connect a Ding Cloud account through ChatGPT's browser authentication flow and establish the user's cloud workspace. Use for Ding Cloud connection and onboarding.
---

Use ChatGPT's Connect flow for this plugin. The configured endpoint is Ding Cloud;
customers do not need a server address, client ID, terminal or local daemon.
Authentication and permission approval belong in the browser. Never ask for a
password, token, client secret or notification credential in the conversation.

After authentication, call `ding_list_workspaces`. Use the single approved
workspace automatically; if there are several, let the user choose by name and
retain its `workspace_id`. Call `ding_get_capabilities` to establish effective
permissions and limits, then inspect watches and destinations as useful.

An OAuth scope challenge can trigger ChatGPT's reconnect flow. A denied Ding
workspace grant or missing membership needs the account owner's attention; do
not repeatedly sign in or broaden consent. Report an unavailable service clearly.
Never claim the connection is complete until an authenticated tool call succeeds.

Offer to help create the user's first watch. Connection does not require creating
a watch or configuring notifications. If there is no destination, explain that
notification setup is needed when creating a watch. Use Ding's secure browser
interface for credentials; do not promise notification setup inside ChatGPT or
future alerts in this conversation.

Ding Cloud stores the authoritative watches and evidence. Requested tool results
are shared with ChatGPT. Disconnecting access does not pause or delete watches.
