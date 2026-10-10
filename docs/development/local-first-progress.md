# Local-first implementation progress

Started October 10, 2026. Implements the [roadmap](local-first-roadmap.md).
Work is committed in small changes by behavior. A checked implementation is not
automatically a qualified public release.

## Work ledger

| Area | State | Evidence / next step |
| --- | --- | --- |
| L0 installation ownership and release channels | In progress | Preserve legacy stable release until watch qualification passes |
| L1 service lifecycle and setup | Pending | Native macOS, Linux and Windows acceptance required |
| L2 offline status and repair | Pending | Separate supervisor status, source health and delivery health |
| L3 first watch and notifications | Pending | Real endpoint, reviewed activation and visible test notification |
| L4 safe updates | Pending | Signatures, package ownership, backups and schema compatibility |
| L5 local MCP / self-hosting | Pending | Real host and always-on machine qualification |
| M marketplace proof package | Pending | No outreach sent; public eligibility unresolved |
| C0 cloud benchmark and topology | Pending | No infrastructure provisioned |
| C1 cloud identity and isolation | Pending | GitHub application and issuer configuration required for live tests |
| C2 hosted execution | Pending | Isolation, egress, resource limits and restore evidence |
| C3 reversible transfer and funnel | Pending | Failures at every handoff phase |
| C4 public MCP | Pending | Real identity/provider/client qualification |
| C5 beta operations | Pending | Budget, sustained soak and real-user retention gates |

## External release gates

Signing/notarization identities, live GitHub OAuth registration, public-domain
configuration, publisher eligibility/review, physical reboot qualification, human
usability testing, and elapsed-time soak/retention results require real external
evidence. Keep these open until they actually pass. This implementation request
authorizes repository work; it does not manufacture platform approval or a budget.
