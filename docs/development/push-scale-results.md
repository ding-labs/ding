# Push performance and database results

October 15, 2026. Local qualification is complete. Qualified, failed and
inconclusive results are recorded below, including preserved interrupted attempts.
No cloud application deployment or database change is part of this experiment.

## What was implemented

SQLite remains the default. The shared Go server can select PostgreSQL explicitly
for a fresh self-hosted installation; see [database operations](../operate/databases.md).
Both backends use the same Go authorization, evaluation, receipt, quota, retention
and delivery logic, with separate SQL and migrations. Configuration does not
silently fall back to SQLite if PostgreSQL is unavailable.

The runtime changes fetch only touched group state on healthy input, account for
active receipts transactionally, separate credential and workspace snapshot
reads from serialized writes, schedule independent delivery streams fairly and
bound retention batches with persistent evidence pins. Source-health transitions
retain their broader group fanout. Timing and pool metrics use bounded labels.
No speculative compression or condition-plan cache was introduced.

PostgreSQL has a bounded query pool and one additional direct ownership session,
forced tenant row security, tenant constraints and transactional authorization
fences. A second server cannot operate concurrently against the same database.
Native encrypted backup and quarantined restore are implemented. Cross-backend
data transfer, transaction poolers and horizontal operation need separate work.

## Reproducible method

The [harness instructions](../../benchmarks/push/README.md) describe how to repeat
individual cases. The full producer journal, raw profiles, private fixtures and
database snapshots stay outside the repository. The published aggregate contains
sanitized measurements and relative evidence identifiers.

| Setting | Measured configuration |
| --- | --- |
| Host | Apple M3, 8 CPU cores, 16 GiB memory; Docker Desktop Linux arm64, 8 CPU and approximately 7.65 GiB available |
| Drivers | modernc.org/sqlite v1.60.1; pgx/v5 v5.11.0 via database/sql |
| PostgreSQL image | 17.11 bookworm, pinned digest in `benchmarks/push/postgres.compose.yml`; fsync, full_page_writes and synchronous_commit enabled |
| Default service budget | 1 CPU and 1 GiB total for application plus database |
| SQLite allocation | Entire service budget to Go application and embedded SQLite |
| PostgreSQL allocation | Half the service CPU and memory to Go; half to PostgreSQL 17.11, unless the row explicitly specifies a CPU-share sweep |
| Expanded budgets | Explicitly labelled 2 or 4 CPU and GiB total; same default PostgreSQL split |
| Producer | Separate Linux container, 2 CPU / 512 MiB; Linux loopback TLS, outside service budget |
| Delivery receiver | Separate 0.5 CPU / 256 MiB container for delivery capacity cases |
| Durability | Real authenticated HTTP, durable acceptance, SQLite WAL/FULL or PostgreSQL durable commits; no producer retries |
| Workload limits | Most lab cases raise finite admission limits, use a 4 GiB store envelope and one million receipts; deployed defaults have a separate case |
| Arrival and latency | Fixed planned arrival schedule; acceptance latency includes queue and producer journal delay |
| Measurement | Profiles off; 30-second screens, three 180-second trials at selected boundaries, then selected 1,800-second cases |
| Inspection | One aggregate sequential 100 ms ticker, targeting 10 authenticated reads/s across the configured workspaces; actual completed reads are reported |
| Recovery check | Graceful worker/store close and reopen; this does not simulate power loss |

The candidate executable used for final qualification is from
`6f7b3b4f2387a1d7b791d2bc54956514cf3b57de`; its SHA-256 and full source revision are recorded per run. The matched
SQLite control executable is from `95737a02d2a45a249493ac14ac3dc32034001553`.
Earlier screens retain their own source revisions. Later controller changes make
any HTTP 429 disqualify capacity, including rows originally reported stable.
Host-forwarded historical runs and Linux-loopback results are separate families.

A qualified point needs at least 99.9% first-attempt acknowledgements, zero
unexpected rejections (including throttles), uncertain responses, missed arrivals,
read errors or clock discontinuities; acceptance p99 below 250 ms; and successful
receipt, sequence, identity, usage, reopen and postcheck reconciliation. Delivery
cases additionally need their backlog drained and receiver evidence reconciled.
An overloaded point can preserve every acknowledged input and still fail capacity.
Missing observer/final report evidence is inconclusive, even if an offline audit
later finds every acknowledged receipt.

The host is shared. Docker's earlier engine interruption and host-forwarding
failure are preserved separately. At 13:30:06 UTC, the owner authorized restoration
of another project's stopped container during the first SQLite sustained run.
Both that container and its independent retry used about 2% CPU at the initial
sample. Container quotas do not eliminate shared host or storage interference.

At 15:37 UTC, Docker exhausted its predefined network address pool while creating
the next fixture, after 78 completed qualification cases. The queue paused before
new measurement. Six unattached networks from owned, stopped Ding fixtures were
released with their volumes, containers and evidence preserved. Remaining cases
resumed with the same cached application and producer binaries. This setup
failure supplies no application capacity result.

Resource samples report container working memory, not process RSS or exact CPU
seconds. PostgreSQL quota bytes are logical usage; table/index/TOAST allocation
and WAL are separate physical costs. Repeated padding can compress heavily, so
high-entropy retained-padding cases are reported separately. Per-case database
high-water growth and WAL deltas can include setup and final checks.

## Completed boundary and control measurements

These rows use 1 CPU / 1 GiB total and profiles off. A three-minute trial is
still a bounded local experiment, not a production service-level guarantee.

| Workload | First-attempt acknowledgements per 180 s trial | Acceptance p99 per trial | Qualified trials |
| --- | --- | --- | --- |
| SQLite, one workspace, 500 requests/s | 90,000; 89,774; 89,810 of 90,000 | 117; 191; 420 ms | 1/3; the latter trials throttled 226 and 190 requests |
| SQLite, one workspace, 750 requests/s | 134,239; 133,589; 134,335 of 135,000 | 188; 249; 239 ms | 0/3; all throttled |
| PostgreSQL, one workspace, 25 requests/s, pool 8 | 4,500 of 4,500 each | 230; 113; 41 ms | 3/3 |
| PostgreSQL, one workspace, 50 requests/s, pool 8 | 9,000 of 9,000 each | 402; 316; 587 ms | 0/3; latency exceeds target |
| PostgreSQL, 10 workspaces plus inspection, 25 requests/s, pool 2 | 4,500 of 4,500 each | 38; 28; 29 ms | 3/3; read p99 10; 8; 8 ms |

The three matched 10,000-group trials offered 25 requests/s with one observation
per request. Control p99 was **112 / 133 / 192 ms**, candidate
**24 / 25 / 26 ms**. Every trial acknowledged all 4,500 inputs and reconciled.
This is a repeatable improvement for that healthy grouped-input workload.

Other controls do not support a universal speedup: the 30-second one-workspace
250/s pair measured control 93 ms versus candidate 270 ms; the ten-workspace
500/s pair measured 10 ms versus 579 ms. Both acknowledged every request, but
those candidate screens failed the latency target. Earlier candidate screens were
faster, so these the longer repeated controls below resolve the screening concern.
The 100,000-receipt paired screen similarly measured 15 ms versus 433 ms, while
the primary candidate screen measured 15 ms. Its longer paired check is recorded below.
The approximately 1.23 ms to 7.3–7.7 microsecond receipt-count query improvement
is a query microbenchmark; it cannot establish an end-to-end throughput gain.

A larger connection pool did not consistently improve PostgreSQL: at 25/s across
10 workspaces, 30-second pool 1 / 2 / 4 / 8 screens measured ingestion p99
26 / 28 / 181 / 1,210 ms. Pool 2 then passed the three repeated trials above.
The larger-budget 100-workspace screens passed at 50/s on both 2 CPU / 2 GiB and
4 CPU / 4 GiB, with p99 132 and 18 ms respectively. These are separate budgets;
they do not qualify the 1 CPU configuration.

The 1 CPU PostgreSQL 100-workspace cases could not finish their live postchecks
within the bounded test timeout. They remain inconclusive for capacity. Offline
audits recovered all 828, 816 and 737 acknowledged receipts in the three preserved
attempts and reconciled identities, digests, observation provenance and durable
usage. One additional committed request had lost its HTTP response. These audits
do not replace the failed live observer or upgrade the capacity verdict.

## Payload and batching screens

All rows below are 30-second screens across 10 workspaces with the same
1 CPU / 1 GiB total budget. Each cell gives first acknowledgements / planned
requests, then acceptance p99. They are not sustained rate recommendations.

| Input | Offered requests/s | SQLite | Direct PostgreSQL, pool 8 |
| --- | --- | --- | --- |
| 10,000 bytes, one output | 500 | 15,000 / 15,000; 9 ms | 1,425 / 15,000; 9,615 ms |
| 100,000 bytes, one output | 100 | 3,000 / 3,000; 11 ms | 1,900 / 3,000; 9,158 ms |
| 600,000 bytes, one output | 100 | 3,000 / 3,000; 104 ms | 1,349 / 3,000; 10,458 ms |
| 3,115 bytes, 100 distinct-group outputs | 100 | 3,000 / 3,000; 25 ms | 92 / 3,000; 20,190 ms |
| Nested 600,000 bytes, 18 distinct-group outputs | 25 | 750 / 750; 50 ms | 750 / 750; 2,010 ms |

The SQLite 100-output screen represents 100 HTTP requests/s × 100 outputs/request
= 10,000 projected observations/s while input lasts. Incoming padding is discarded
unless a retained-padding case explicitly projects it. PostgreSQL's later
600 KB / 25/s single-output retry passed all 750 inputs at p99 189 ms after the
timeout error-classification fix; its original failed attempt remains preserved.
The measured adapter provides no evidence that a database swap alone improves
throughput under this small shared app/database budget.

## Repeated ten-workspace controls

The six 180-second trials used the same Linux producer, 1 CPU / 1 GiB and
500 planned requests/s across 10 workspaces. Each trial planned 90,000 requests.

| Variant | First acknowledgements by trial | Acceptance p99 | HTTP 429 by trial | Qualified trials |
| --- | --- | --- | --- | --- |
| Control | 89,940 / 89,650 / 89,424 | 481 / 702 / 939 ms | 60 / 350 / 576 | 0/3 |
| Candidate | 89,838 / 90,000 / 89,895 | 467 / 208 / 632 ms | 162 / 0 / 105 | 1/3 |

Every acknowledged input reconciled and postchecks passed. The short-screen
latency difference did not reproduce as a uniform candidate regression. Both
variants fail to qualify a dependable 500/s operating point on this budget.

## Sustained measurements

The first 1,800-second run used one SQLite workspace, 250 planned requests/s,
concurrent inspection and final reopen. It acknowledged **449,929 / 450,000**
requests (99.9842%), with p95 **12 ms**, p99 **163 ms**, 17,978 completed reads
and read p99 **6 ms**. Every acknowledged input reconciled; there were no read or
clock errors, and reopen/postchecks passed.

It **did not qualify** under the zero-rejection rule: 70 requests were throttled
at 13:30 UTC and one returned temporary `acceptance_uncertain` at 13:40 UTC.
The uncertain response had no durable receipt; reconciliation found no missing
acknowledged input. All throttles occurred about 24 seconds after the other
project's container restart. That temporal overlap is a confounder, not proof of
causation. This run cannot establish an uncontended sustained 250/s service rate.

The SQLite allocated database grew by approximately **660 MB** over the accepted
449,929 requests, roughly 1.47 KB per request for this tiny-body workload including
its durable receipt/state/index costs. The run uses the raised lab storage cap;
the production default 128 MiB envelope would bind much earlier.

The 100-workspace SQLite run **qualified** on **4 CPU / 4 GiB total** at
250 aggregate requests/s: **450,000 / 450,000** first acknowledgements, p95
**11 ms**, p99 **65 ms**, zero rejections/uncertain responses/missed arrivals or
clock errors. All receipts and usage reconciled, 17,948 authenticated reads
completed without errors (read p99 **7 ms**), and reopen/postchecks passed.
Its three preceding 180-second trials all acknowledged 45,000 inputs, with p99
47 / 33 / 16 ms. Lower-budget 250/s trials did not pass consistently; this result
does not qualify that rate on a 1 CPU or 2 CPU configuration.

PostgreSQL's one-workspace run **qualified** at **50 requests/s**, using a query
pool of **2** and **1 CPU / 1 GiB total**, split equally between Go and PostgreSQL.
It acknowledged and reconciled **90,000 / 90,000** requests over 1,800 seconds,
with p95 **15 ms**, p99 **117 ms**, read p99 **10 ms**, zero rejections, uncertain
responses, missed arrivals or clock errors, and successful reopen/postchecks.
The three preceding 180-second trials all acknowledged 9,000 inputs, with p99
34 / 32 / 111 ms. This configuration includes concurrent inspection; the earlier
pool-of-8 boundary trials had a different inspection configuration.

No 100-workspace PostgreSQL point passed the repeatability gate. At 25 aggregate
requests/s, 2 CPU / 2 GiB measured p99 607 ms; 4 CPU / 4 GiB measured 55 / 51 /
319 ms, qualifying two of three trials. At 10/s, 2 CPU measured 554 ms and 4 CPU
604 ms. All **21,600 acknowledged inputs** across those six trials reconciled,
and postchecks/reopen passed, but the **30-minute cohort run was skipped** because
no candidate qualified. These variable latency failures do not establish a simple
maximum request rate. They are a gate before recommending this adapter for a
large production cohort.

## Mature rolling windows

Each backend passed three 180-second trials of a healthy one-minute average
condition in one workspace, on 1 CPU / 1 GiB total. SQLite offered 100/s and
acknowledged all 18,000 inputs per trial, with p99 **158 / 27 / 38 ms**.
PostgreSQL used pool 2 at 25/s and acknowledged all 4,500 per trial, with p99
**48 / 125 / 107 ms**. Every trial reconciled and passed postchecks with no clock
errors. Real time advanced beyond the window; source-health fanout and missing-data
transitions at large group counts remain separate performance gates.

## Aged-store paired check

The 180-second SQLite pair used 1 CPU / 1 GiB, 100 requests/s and real receipt
history. Starting durable request counts were **100,559 control / 100,560
candidate**. Both acknowledged all **18,000** measured inputs and passed
reconciliation/postchecks. Control p99 was **118 ms**, candidate **26 ms**.
This pair improved the observed tail, while the earlier short controls varied
substantially. It does not establish a uniform aged-store speedup or a higher
supported throughput rate; the receipt-count query microbenchmark is separate.

## Delivery screens

At 25 requests/s for 30 seconds, both backends acknowledged every input with a
separate slow local receiver. One ordered stream still had 2 SQLite / 7 PostgreSQL
pending deliveries after the one-minute drain bound. Four independent streams
with four workers drained completely in about one second after input ended
(SQLite p99 23 ms; PostgreSQL p99 156 ms). Receiver journals reconciled durable
delivery identities, including warmup and postcheck deliveries. Fast and local
outage/recovery screens also drained on both backends. These are local webhook
experiments; external provider delivery remains a separate release gate.

## Storage and remaining costs

Retaining arbitrary JSON needs a separate storage budget from parsing and then
discarding padding. In the 30-second retained-payload screens, PostgreSQL's
observed database high-water growth was:

| Incoming body and offered rate | Repeated padding | High-entropy padding |
| --- | --- | --- |
| 100,000 bytes, 25 requests/s | 13.7 MB | 82.8 MB |
| 600,000 bytes, 5 requests/s | 11.9 MB | 93.7 MB |

These are sampled whole-case allocation changes including setup/checks, not exact
per-tenant retained byte totals. All requests acknowledged and reconciled; the
repeated 100 KB PostgreSQL screen nevertheless failed latency at p99 276 ms.
Logical quota usage grew similarly within each repeated/entropy pair, while
physical growth differed substantially. Compressible fixture strings must not
stand in for arbitrary user data. Sampled PostgreSQL WAL reached approximately
185 MB in the entropy cases and needs separate reserve.

SQLite's final allocated database files were approximately 89–91 MB for the
100 KB cases and 105 MB for the 600 KB cases, plus roughly 4.3–4.5 MB WAL. Those
are final footprints including warmup and checks, rather than the PostgreSQL
growth statistic above. The aggregate retains both accounting categories.

Separate 60-second diagnostics found SQLite parser/statement execution among the
remaining CPU costs. PostgreSQL Go profiles spent substantial time in system
calls and waits; they do not contain PostgreSQL backend CPU stacks. Query round
trips, ownership checks, tenant worker overhead and database waits warrant a
focused follow-up before changing the transaction design. Pool sweeps already
show that more connections alone do not solve the bottleneck. No exact
observations-per-CPU-second figure was collected.

A read-only counter checkpoint wholly inside the third 100-workspace PostgreSQL
25/s trial on 4 CPU / 4 GiB observed **35,723 database transactions in 55.18 s**
(about **647/s**) alongside the input and target 10 inspection reads/s. This
includes authorization, inspection and background work; it does not attribute all
extra transactions to one subsystem. Read snapshots normally end with rollback,
so the rollback counter is not a count of failed writes. No database block reads
were recorded during that window; WAL flush latency was not collected. Tagged
query counts and worker scheduling measurements are the next attribution step.
That trial still missed latency at p99 319 ms after earlier trials at 55 and 51 ms.

## Noisy workspace and quiet peers

The 30-second raised-limit screen offered 500 requests/s, approximately 90% to
one of 10 workspaces. The accepted-response journal gives:

| Backend | Noisy workspace acknowledgements | Quiet peers combined | Noisy p99 | Per-quiet-workspace p99 range |
| --- | --- | --- | --- | --- |
| SQLite | 13,482 / 13,515 | 1,481 / 1,485 | 544 ms | 102–407 ms |
| PostgreSQL | 445 / 13,515 | 71 / 1,485 | 15,152 ms | 669–2,360 ms |

These are overloaded screens, with few successful PostgreSQL samples per quiet
peer. They expose shared pressure under relaxed lab admission settings; they do
not qualify isolated tenant service levels. The
[per-workspace aggregate](../../benchmarks/push/results/2026-10-15-noisy-workspaces.csv)
retains offered counts, pressure responses and accepted-response medians/tails.
Workspace indices identify synthetic fixtures, with no account credentials.

## Correctness and release checks

Implementation verification included the full local tagged Go race suite
(`go test -race -tags console,mcpui -count=1 ./...`), vet, real PostgreSQL
store/control/HTTP/evaluator contracts, concurrency and ownership-loss tests,
native encrypted backup/restore and CLI roundtrips, both frontend builds and
44 frontend unit tests. Later surgical fixes passed focused race/real-database
regressions; CI passed whole-repository race tests and its PostgreSQL contract job
on the current production Go code. Final report/data commits receive CI separately.

Pressure tests exposed two error-classification issues that were fixed:
projection cancellation/deadline errors and unavailable credential storage now
produce retryable 503 responses. Malformed JSON, invalid credentials and revoked
access retain their appropriate rejection behavior. Original failed attempts and
source revisions remain in the aggregate. The PostgreSQL 600 KB retry and the
actual canceled-authorization regression verified the corrected paths.

SQLite remains the default. PostgreSQL is an explicit option for fresh self-hosted
installations with one active server and direct connections. These experiments
do not qualify populated SQLite-to-PostgreSQL transfer, transaction poolers,
horizontal servers, physical power-loss recovery, managed PITR or a cloud cutover.
Native encrypted restore/quarantine tests cover their documented local path.
The live cloud soak uses one account and no notification destination; production
multi-user deployment and external delivery still need their own release gates.
Its actual producer sends 18 events in roughly 599–611 bytes each minute, with
replay/conflict probes; large payloads belong to this independent local matrix.

## Evidence and conclusion

The [sanitized aggregate](../../benchmarks/push/results/2026-10-15-qualification.csv)
contains **191 case records**, including generator controls, diagnostics and
preserved failed attempts. **187 have final reports**, with zero reconciliation
errors in completed service cases. Three missing-report cases have offline audits
finding every acknowledged receipt; one failed setup never began measurement.
Unknown and backend-inapplicable cells are blank.

`reported_stable` preserves the original result; `stable` applies the strict
zero-rejection gate. Generator-only results have zero projected observations,
and profiled diagnostics cannot establish capacity. `resource_window` distinguishes
measurement samples from entire attempts whose report is missing.
`ledger_p50_ms` is derived from accepted-response journal timestamps, rounded to
three decimals; those timestamps follow the runtime histogram observation.
Runtime p95/p99 use millisecond bins. All 190 available journals matched their
recorded acceptance counts. No credentials or raw payload journals are published.

SQLite completed all 46 catalog screens. PostgreSQL completed 43 distinct catalog
screens; its first 100-workspace 1,000/s setup lacked a report, and subsequent
bounded retries could not complete live postchecks. The higher 2,500/s and original
500/s cohort screens were deferred in favor of lower rates and larger budgets.
The 80-case main stage, eight supplemental cases and six repeated controls are
finished. Three 30-minute runs completed; the PostgreSQL cohort long run was
skipped because no tested point met its preceding qualification gate.

The repeatable healthy 10,000-group gain, bounded receipt accounting, concurrent
reads and tested recovery support the focused Go improvements. SQLite remains
the default. Explicit PostgreSQL self-hosting is available for its documented
configuration, with further cohort performance, pooler/distributed coordination,
recovery and migration work needed before a production cloud database decision.

Review order: [plan #52](https://github.com/ding-labs/ding/pull/52),
[SQLite #53](https://github.com/ding-labs/ding/pull/53),
[PostgreSQL store #54](https://github.com/ding-labs/ding/pull/54),
[shared server #55](https://github.com/ding-labs/ding/pull/55),
[operations #56](https://github.com/ding-labs/ding/pull/56),
[qualification #57](https://github.com/ding-labs/ding/pull/57).
Preserve the focused commit history. The independent cloud soak continues on its
recorded deployment; its existing missing samples and telemetry failure remain
part of that separate experiment's evidence.
