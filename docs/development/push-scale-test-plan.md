# Performance improvement and database comparison tests

This is P0/P6 of the [performance plan](push-scale-plan.md). Use focused tests
while changes progress, then run the complete matrix against the selected
implementation. The current local matrix is deferred; the Cloud soak runs
independently.

## T1 Repair measurement qualifications before changing the runtime

- Add graceful stop-after-current and cancellation recording to the launcher.
  Preserve partial ledgers and owned database volumes, mark cancelled counts
  unknown, and prevent the next queued case from starting.
- Record actual body sizes, seeded groups/receipts, applied configuration, schema
  and application/harness revisions. Recalculate comparison gates under one
  documented harness version; historical `stable` flags remain historical.
- Detect host sleep/clock divergence and classify affected capacity runs as
  inconclusive. Reconcile push-produced observations separately from internal
  clock/health observations, including the actual receipt-span contract. Keep
  clock recovery tests as deliberate local correctness cases.
- Keep profiling off for capacity; run profiles separately on a reproduced
  bottleneck. Qualify the separate generator for each offered byte rate, batch
  size and arrival rate, rather than only a small-body no-op test.
- Verify the start barrier captures accounting before measurement; validate
  resource samples and phase transitions after cancellation/restart.

Repeat the sleep-affected 100,000-receipt control and any focused comparison that
lacks a qualified baseline on an awake, idle host. Add a separate test in which
receipts expire without making the machine wait 24 hours. Synthetic setup must
preserve actual real-service acceptance for throughput measurements.

## T2 Correctness gates for every increment

| Boundary | Required tests |
| --- | --- |
| Acceptance | All-or-nothing batches; exact receipt/usage; identical replay; changed-byte conflict; lost-response reconciliation; rejection without partial writes |
| Authorization | Two actual users/workspaces; roles; watch grants; revocation/rotation/expiry during acceptance; workspace deletion; reused connection scope |
| Evaluation | Typed grouping/matching; repeated group in a batch; source bootstrap/fanout/recovery; unknown values; missing-data timers; rolling windows; revision/generation changes |
| Storage | Existing-store migration; counter backfill/repair; close/read/write races; consistent backup; retention pins and resumable cleanup; restore quarantine |
| Delivery | Ordered streams; unrelated-stream fairness; slow/outage recovery; lease fencing; worker death; stable IDs under duplicate remote delivery |
| PostgreSQL | Real transactions and isolation; lock ordering; deadlock/serialization retries; rollback sequence allocation; two processes; pooler compatibility; restart/reconnect |

Use deterministic barriers/fault hooks for the races, and randomized model tests
for counters/retention. Run targeted tests after each coherent change. Run Go
race tests for changed concurrency boundaries, then repository Go/vet, generated
contracts and affected Console/MCP integration gates before merging the phase.
Keep external egress and provider-native adapters as their existing release gates.

## T3 Focused performance comparisons

| Change | Before and after measurements |
| --- | --- |
| R1 group reads | 1/100/1,000/10,000 groups; repeated keys; 1/18/100 outputs; healthy input versus source-health fanout |
| R2 receipt accounting | 0/1,000/10,000/100,000 active receipts; expiry/churn; exact count/query cost and end-to-end latency |
| R3 control reads | 1/10/100 workspaces; multiple credentials; pool sizes 1/2/4/8; shared pool wait and revocation fences |
| R4 workspace reads | One hot workspace; multiple watches; ten/100 workspaces; authenticated read load; WAL and checkpoint pressure |
| R5 delivery | Fast/slow/outage receivers; single ordered stream and many independent streams; pending age, attempt rate and recovery drain |
| R6/R7 retention/storage | Prepopulated history plus new writes; bounded cleanup; pinned evidence; retained/discarded padding; live pages, WAL and physical growth |
| G1–G6 PostgreSQL | The same inputs, reads, output counts and correctness checks; one/two processes; direct/pooler; row-lock and connection waits |

Screen for 30 seconds, then run each meaningful boundary for 180 seconds three
times. Alternate old/new controls where practical and run them sequentially.
Revert or revise changes with unexplained correctness failures, a persistent
reference-workload regression or gains smaller than observed variance. Each PR
states its expected improvement and acceptance criteria before its results.

## T4 Use explicit total resource budgets

Compare Linux servers on the same machine and storage class. SQLite runs inside
the Go service; PostgreSQL and a pooler are additional processes whose resources
must be counted. Record each component's configured and actual usage.

| Comparison family | Budget and interpretation |
| --- | --- |
| Equal total resources | Start with 1 CPU and 1 GiB total for app plus database/pooler. Declare the split; test a small allocation sweep if one component is starved. SQLite receives the same total. Sum component limits or use a verified common resource group. |
| Expanded resources | Repeat at 2 CPU/2 GiB and 4 CPU/4 GiB total for both architectures. Report every component and total; distinguish added resources from database improvements. |
| Multiple app processes | Keep the total budget fixed for one versus two processes first. Then report a separate scale-out allocation, including the database and pooler. |
| Native exploration | Whole-laptop runs diagnose overhead and larger ceilings; report separately from constrained Linux results. |

Use isolated local volumes, pin images, record filesystem/runtime settings and
keep comparable dataset populations, indexes, retention, admission configuration
and nonbinding performance-test quotas. Test binding quota behavior separately.
Do not compare cached/tuned PostgreSQL against an artificially cold SQLite control.

SQLite keeps WAL/FULL. PostgreSQL keeps `fsync`, `full_page_writes` and
`synchronous_commit` on, with settings verified at runtime. Record checkpoint,
autovacuum and buffer configuration. Pooling and durability experiments remain
separate. A Docker restart or process kill does not qualify physical power loss.
[PostgreSQL WAL durability](https://www.postgresql.org/docs/current/runtime-config-wal.html).

## T5 Rerun the full workload matrix

Use three application variants: the preserved SQLite control, optimized SQLite,
and optimized Go with direct PostgreSQL. Add pooled PostgreSQL after T2/G6 pass.
The current Go lab must gain backend selection and portable accounting/health
checks; hardcoded workspace files and SQLite checks cannot stand in for that work.

| Dimension | Required cases |
| --- | --- |
| Arrival rate | Small-body ramps through 100/250/500/1,000/2,500 planned requests/s; qualify the generator before any higher rate |
| Tenant distribution | 1/10/100 workspaces, multiple watches in one workspace, independent credentials, and a noisy workspace with quiet peers |
| Output count | 1/18/100 observations per accepted request; report both requests/s and observations/s |
| Input bytes and shape | Small, 10 KB, 100 KB, 600 KB and 1 MiB; ignored versus retained padding; bounded realistic nested/array JSON, including an 18-item cloud-soak-shaped batch |
| State population | 1/100/1,000/10,000 real groups; repeated groups; mature one-minute windows |
| History | Fresh and 100,000+ receipts; ordinary and pinned history; receipt expiry and retention churn |
| Reads | Authenticated inspection at a recorded aggregate rate, plus per-workspace fairness; large entity/outbox pages |
| Delivery | Fast, 100 ms slow and outage/recovery; single versus independent streams; separate receiver resources |
| Recovery | Graceful reopen; local app/database/pooler process termination; uncertain commit response; counter repair; lease reclaim |
| Admission | Operator defaults and raised finite lab limits; storage/receipt/pending/pool exhaustion; malformed/deep/oversized input and credential floods |

Expected policy/adversarial rejections use their own assertions; they are not a
valid-input capacity failure. Keep eventful workloads separate from healthy
state updates. Prepopulation and clock-controlled expiry logic tests are labelled
setup/correctness work; measured throughput uses real time and the actual service.

Screen the whole matrix first. Extend qualified operating-range and boundary
cases to 180 seconds, repeating the meaningful boundary three times. Expected
overload screens can stop once their cause and recovery are established. Run the
complete fixed comparison set on the final candidate without profiling.

Then resume the deferred thirty-minute tests at rates established as sustainable:
one hot workspace and many workspaces with reads and a final reopen/reconcile.
Choose each backend's qualified operating range and include a common fixed rate
for direct comparison. The old arbitrary sustained rates are not guaranteed to
remain appropriate. Add aged-store churn and the separate delivery recovery test
to the final qualification when their changed paths require sustained evidence.

## T6 Decide from correctness and repeatability

The valid-input local target remains at least 99.9% first-attempt acknowledgement,
acceptance p99 below 250 ms, zero missed generator arrivals, unexpected rejections,
uncertain responses or read errors, correct durable reconciliation and verified
postchecks. Delivery cases additionally need the declared backlog/drain target;
an intentionally overloaded single stream can pass its overload/recovery test
without being classified as sustainably drained during input.

Require three qualified boundary repeats and sustained evidence before claiming
a supported rate. Report p50/p95/p99, accepted fraction, explicit pressure/error
codes, runtime/heap/RSS, CPU, connection and row-lock waits, queue age, logical and
physical storage, WAL/checkpoints and restart/recovery outcomes. Reconcile every
offered key, including uncertain responses, against durable receipts and actual
usage. A resolved lost response still fails the first-attempt latency/service target.

Publish a sanitized comparison with workload/configuration, exact revisions,
backend/driver versions, resource splits, missing/cancelled/inconclusive cases and
untested deployment gates. Preserve private raw evidence. Recommend production
PostgreSQL based on repeatable gains or demonstrated multi-process/availability
needs plus recovery and cost, rather than a pooling microbenchmark.

## T7 Keep implementation and evidence reviewable

For R1–R8 and G1–G7, use the source/API preparation, associated regression tests,
runtime switch/migration and before/after evidence as separate focused commits
where they are independently coherent. Do not combine a schema port, worker
coordination, pooler and deployment into one commit or PR. Preserve every small
commit when pushing.

The release sequence is optimized SQLite, experimental PostgreSQL parity,
distributed/pooler qualification, final matrix, architecture decision, and tested
cloud cutover. Leave the existing Cloud soak's worker, credentials, production
revision and schedule intact throughout local work. Its eventual completion and
cleanup remain governed by the separate soak runbook.
