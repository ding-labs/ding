# Plan: confirm Ding's desktop-only marketplace route

Proposed October 10, 2026. Part of the [local-first roadmap](local-first-roadmap.md). **Ding's public local-MCP eligibility is unresolved. No publisher inquiry or submission has been sent as part of this plan.**

## Question to resolve

Can a new publisher distribute Ding through the official ChatGPT plugin directory as a desktop-only plugin, running a native local Go MCP adapter against a user-owned background Ding service, with no Ding account and no public Ding endpoint?

A successful local configuration or private plugin installation does not answer this distribution question. Neither does the existence of desktop-only plugins from other publishers.

## Evidence as of October 10, 2026

| Finding | What it establishes | What remains unproven |
| --- | --- | --- |
| OpenAI documents a shared public plugin directory and desktop-only plugins that are discoverable on the web but installed/used on desktop | Desktop-only is a real distribution category | Whether Ding's package is eligible and which desktop surfaces can invoke its local tools |
| Public MCP packaging instructions use a remote HTTPS submission route and direct developers needing local MCP support to an OpenAI contact | Local support requires a specific publisher investigation | General self-service acceptance of Ding's bundled executable/service setup |
| Public-directory packages cannot contain lifecycle hooks under the documented packaging rules | Service installation must use an accepted setup mechanism | Whether an onboarding flow may install Ding, or must connect to an independently installed Ding |
| Remote review requires a public endpoint; template endpoints are restricted to established trusted publishers | A standard remote submission and an arbitrary self-hosted URL are different routes | Permission for Ding to use customer-owned endpoints as its public listing |
| Secure MCP Tunnel explicitly excludes public submission/distribution | It can be useful for eligible private development connections | It cannot serve as Ding's public-directory distribution route |

Sources: [plugin availability](https://learn.chatgpt.com/docs/plugins), [packaging and local support](https://developers.openai.com/plugins/build/plugins), [remote review requirements](https://developers.openai.com/plugins/deploy/app-review), and [Secure MCP Tunnel](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels). Recheck these sources immediately before submission; preserve dated findings and written answers in the repository.

## M0 — prepare a reviewable proof package

Prepare this while the local installer/service work proceeds:

1. A one-page architecture: model host → local native MCP process → scoped local control API → independently managed daemon and SQLite. Include all outbound data paths and state that tool results go to the selected model provider.
2. An inventory of the existing tools, permission scopes, reviewed mutations, credential storage, revocation, and command-execution boundaries. Include negative tests and synthetic fixtures.
3. Two installation proposals: a plugin containing the adapter plus an approved setup flow for Ding; or the plugin pairing with a separately installed, signed Ding package. Ask which route is supported. Neither proposal relies on lifecycle hooks or a hosted relay.
4. A first-use storyboard: discover → install → establish daemon → pair → preview first watch → approve → observe → receive a local notification. Include offline, denied permission, update, and uninstall states.
5. Native signed package prototypes on the initially targeted OS/architecture, with a manifest using only documented/supported fields. Mark other OS targets pending until qualified.
6. A local synthetic review scenario with no production credentials. Include logs, expected results, screen recordings, version/digest, and a clean-machine reproduction.

Use the existing [integration implementation](../integrations/README.md) and [qualification evidence](../integrations/verification.md). Do not rebuild the MCP implementation to fit an imagined submission mechanism.

## M1 — obtain publisher eligibility and packaging answers

The future project owner should use a verified OpenAI publisher/support contact or the official submission channel to request a local-MCP preflight. The following draft is prepared for that step; sending it is outside this planning task.

**Draft inquiry**

> We maintain Ding, an open-source Go tool for deterministic developer alerts: https://github.com/ding-labs/ding. We want an official desktop-only ChatGPT plugin that works without a Ding account or public Ding server.
>
> A native Go MCP adapter connects to a scoped API on a local Ding daemon. Ding persists watches locally and runs under the operating system's service manager independently of the model client. The adapter provides reviewed changes, explicit grants, revocation, and optional embedded UI. We can provide signed packages and a synthetic review fixture.
>
> Your docs describe desktop-only directory plugins and direct local MCP publishers to contact OpenAI. Is this architecture eligible for a new publisher? Which packaging, installation, and review route should we use? In particular, may the approved onboarding flow install the background daemon, or must users install Ding separately? We will not rely on plugin lifecycle hooks.
>
> Please confirm the supported desktop surfaces/operating systems, local transport, native executable distribution and signing requirements, independent daemon updates, embedded UI support, and the expected review environment without a public HTTPS endpoint.

Record the exact response, date, publisher identity, supported client versions, limitations, and links to any private instructions in a private evidence record as appropriate. Keep public notes free of private correspondence or reviewer credentials. A support answer establishes the permitted path, not final acceptance of the submitted implementation.

## Questions and proof required

| Topic | Required answer or experiment |
| --- | --- |
| Publisher eligibility | Is this local-MCP architecture open to Ding, or only an existing partner program? What verification is required? |
| Surface matrix | Separately record ordinary ChatGPT chat, Work, Codex desktop, web discovery, browser execution, and mobile. Do not infer support in one surface from another. |
| OS matrix | Which operating systems and architectures accept the native package? How is the appropriate executable selected? |
| Transport | Is bundled stdio accepted? Is loopback HTTP accepted? What sandbox/network/local-process restrictions apply? |
| Daemon installation | May approved onboarding install/start an independent service? What consent and elevation UI is required? Is a separate signed installer acceptable? |
| Updates and removal | Who owns adapter updates? How does it locate a stable daemon installation? What happens on plugin uninstall versus Ding uninstall? |
| Local authorization | Is scoped local pairing sufficient without an online Ding account? How must local command capabilities be described and granted? |
| Embedded UI | Which local tool/UI flows work in the actual host, with what CSP and navigation restrictions? |
| Review | How does the reviewer install a local synthetic fixture? Is an endpoint-free review available for this package? |
| Listing quality | Can the listing explicitly promise local free use, explain desktop-only support, and link directly to the supported installation flow? |

## M2 — test the accepted route in the real host

Once the permitted route is documented, package the approved approach and test on a clean machine using the public-release client build. Record each surface/OS/version separately. Test discoverability, install, permission denial, daemon not installed, startup, tool discovery, first watch, review/approval, embedded UI, client restart, daemon restart, update, grant revocation, and uninstall. Verify the daemon continues after the chat/client closes.

Inspect the user's whole experience. Requiring secret tokens copied through a conversation, hand-edited JSON, a source build, an unofficial tunnel, or a silently installed service fails the intended public onboarding quality. A clearly presented signed installer can be acceptable if OpenAI permits it and clean-machine testing shows the combined flow is understandable.

For unsupported web/mobile surfaces, show the accurate platform limitation. Do not silently redirect a local user's watch to a cloud account to make a tool invocation work.

## M3 — review, decision, and fallback

| Evidence level | Can claim | Cannot claim |
| --- | --- | --- |
| Documentation only | Desktop-only plugins exist | Ding has approval |
| Written publisher preflight | Ding has a permitted route with stated conditions | Package has passed final review |
| Actual client qualification | Tested package works on named versions/platforms | Public listing is live |
| Accepted submission | Review approved the submitted package | Every surface is supported |
| Published listing + fresh install | Official availability on the tested listed surfaces | Browser execution of local-only tools |

Timebox preparation to a few engineering days. Plan a manual follow-up after roughly a week and record a decision checkpoint after two weeks; these are project targets, not scheduled messages or promises about OpenAI's response time. Local product work continues throughout.

If the route is approved, complete the local release gates, publisher verification, privacy/support material, and official submission. If it is denied, record the specific reason and continue shipping free standalone Ding and supported local MCP configurations. If it remains unanswered, keep the status unresolved and avoid advertising a forthcoming accepted listing. In either case, evaluate cloud separately for its actual benefit: continuous hosted execution.

Keep Claude packaging as a separate qualification track under the [original integration plan](llm-integration-plan.md). A positive result for one client does not establish another client's marketplace eligibility. The immediate investigation here is ChatGPT, as requested.

Do not build a public round-trip relay to a laptop as a fallback. Do not equate a private marketplace, a custom connector, or a skills-only listing with the requested official public MCP plugin.
