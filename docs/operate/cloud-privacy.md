# Cloud data and deletion boundaries

This describes the optional cloud source preview. A public operator must publish
its actual domain, support contact, processors, retention and applicable terms
before accepting users. Local Ding continues without a Ding account or required
telemetry; using cloud is an explicit choice for selected watches.

| Data | Purpose and boundary |
| --- | --- |
| Verified issuer/subject and random workspace ID | Separate personal workspaces and authorize access. Minimal GitHub identity through the selected broker; no repository access is requested by Ding. |
| Selected watch definitions and destinations | Execute public HTTP checks independently of the laptop. URLs, conditions and selected response evidence live in the workspace store. |
| Source/destination credential values | Encrypted with workspace-derived keys and authenticated name/workspace binding. Never returned by Console/MCP read APIs. The worker necessarily decrypts a value when using it. |
| Browser/device sessions | Separate, hashed credentials, bounded and expiring; signing out does not stop watches. The identity broker maintains its own session and token policy. |
| Model connections | Explicit client ID, scopes, expiry and selected credential references. The model receives requested tool results/evidence under the host's own data policy. |
| Check/delivery attempts and evidence | Runtime correctness, incident investigation, durable retries and visible quotas. Normal detailed history is seven days; active/pending evidence may remain pinned, subject to store limits. |
| Monthly usage/reservations | Enforce outbound budgets across restarts. Failed attempts count; crashed reservations remain conservatively charged. Three UTC calendar months of usage metadata are retained. |
| Transfer/test receipts | Prevent uncertain requests from duplicating notifications or activating two runners. Durable receipts are bounded; reaching their limit requires explicit operational attention. |
| Operator metrics | Aggregate capacity/failure/lag counts; no watch IDs, URLs, commands, payloads, secrets or model transcripts. |

Credential encryption is not whole-database encryption. Protect the host, physical
volume, access controls and backups. The supplied backup job creates age-encrypted
snapshots, retains local copies seven days and deliberately creates a maintenance
gap; off-host copies need matching lifecycle policies. The master credential key
and backup identity are separate recovery secrets. A production operator must
measure and disclose its actual backup retention and recovery promises.

Account deletion explicitly stops hosted execution, revokes access, removes live
workspace files and credentials, and retries interrupted cleanup. It does not erase
independently retained encrypted snapshots immediately. Snapshots expire through
the disclosed policy; restoration must reapply intervening deletion requests before
reopening access. Do not claim immediate erasure of every backup. Local copies,
user exports, destination receivers and model-host records are independent systems.

Export or move watches back before deleting the cloud account. Configuration export
contains references, not credential values, and does not transfer execution
ownership. A held local watch must not be resumed merely because cloud is offline
or an account was deleted. Use the reversible move protocol while both sides can
confirm their state; unresolved ownership requires explicit recovery review.

Product analytics are not enabled. Pilot measurement uses separately consenting
participants and coarse outcome records. Do not silently associate local activity
with a subsequent GitHub identity. Operational monitoring is necessary for hosted
execution and must be disclosed separately from optional adoption measurement.
