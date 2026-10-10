# Local-first implementation progress

Started October 10, 2026. Implements the [roadmap](local-first-roadmap.md).
Work is committed in small changes by behavior. A checked implementation is not
automatically a qualified public release.

## Work ledger

| Area | State | Evidence / next step |
| --- | --- | --- |
| L0 installation ownership and release channels | In progress | Private ownership record, signed metadata and hash-pinned installer generation implemented; preserve legacy stable until native package qualification |
| L1 service lifecycle and setup | In progress | User startup definitions for three OSes; native macOS install/start/restart/stop/uninstall passed; login/boot and other OS qualification open |
| L2 offline status and repair | In progress | Offline status, instance/version, waiting/overdue observations, source/delivery health and bounded logs implemented; broader fault/repair qualification open |
| L3 first watch and notifications | In progress | Real-daemon browser onboarding, notification confirmation gate, native transports and disposable real scheduler demo implemented; visible notification/pilot gates open |
| L4 safe updates | In progress | Signed metadata/archive checks, backups, journal recovery, same-schema rollback and CLI implemented; packaged N→N+1, platform installers and automatic scheduling remain |
| L5 local MCP / self-hosting | Pending | Real host and always-on machine qualification |
| M marketplace proof package | Prepared | [Desktop proof](../integrations/desktop-proof.md) and inquiry draft prepared; no outreach sent; public eligibility unresolved |
| C0 cloud benchmark and topology | Initial evidence | [100-workspace synthetic result](cloud-topology.md); single-host fencing implemented; representative sustained load/cost gates open; no infrastructure provisioned |
| C1 cloud identity and isolation | In progress | Encrypted tenant credentials, hashed sessions, one-use PKCE/nonce sign-in, guarded hosted API and cross-tenant negative tests; real GitHub broker configuration remains |
| C2 hosted execution | In progress | Isolated Go engines, guarded DNS/dial, shared network bound, durable monthly budgets and restart test; hosted Console/entrypoint and real delivery-test flow implemented; backups and sustained qualification remain |
| C3 reversible transfer and funnel | In progress | Offline preflight, durable source/target holds, cloud test proof, CLI move/move-back and availability choices; round-trip/lost-response tests pass; native client/user qualification remains |
| C4 public MCP | In progress | Official Go SDK endpoint, exact resource audience, signed client identity, explicit scoped bindings and revocation; cross-tenant tests pass; real provider/ChatGPT qualification remains |
| C5 beta operations | Pending | Budget, sustained soak and real-user retention gates |

## External release gates

Signing/notarization identities, live GitHub OAuth registration, public-domain
configuration, publisher eligibility/review, physical reboot qualification, human
usability testing, and elapsed-time soak/retention results require real external
evidence. Keep these open until they actually pass. This implementation request
authorizes repository work; it does not manufacture platform approval or a budget.
