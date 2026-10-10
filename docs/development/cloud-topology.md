# C0: fenced single-host beta implementation

Decision: build a single-host, capped-cohort implementation first. Do not provision
it or claim production capacity from the measurements below. Public launch still
requires the local release gates, a confirmed budget, real identity integration,
representative sustained workloads, backup/restore and laptop-off transfer tests.

One process holds an OS lock for the control directory for its entire lifetime.
It owns a SQLite control database and one independently locked SQLite watch store
per workspace on the same persistent volume. The control database maps a verified
issuer + immutable subject to a random workspace ID. No user-supplied path or
watch ID selects another tenant's database. Losing the process releases ownership;
an expiring distributed lease is not used to authorize a replacement writer.

The cohort is capped at 100 accounts. Each tenant has one acquisition and one
delivery worker, with a one-second scheduler poll; shared HTTP admission caps
active outbound requests at 32. Public egress validates DNS answers and dials only
the validated IP, rejects mixed private/public answers and nonstandard ports,
does not use environment proxies, and rejects redirects. Credentials use
workspace-derived encryption keys and authenticated workspace/name binding.

The beta policy allows three public HTTP watches, three destinations, five-minute
minimum intervals, ten-second source timeouts, 64 KiB bodies, ten entities/outputs,
100 samples, 100 pending deliveries, 64 MiB live store limits, and seven-day
ordinary retention. Referenced evidence stays pinned and counts against limits.
Monthly outbound limits are 30,000 checks, 1,000 delivery attempts and 128 MiB of
metered application traffic per account. Conservative reservations prevent
parallel requests from overbooking byte limits; crash reservations remain charged.
These are implementation hypotheses, not advertised entitlements. TLS overhead,
backups and operations need separate infrastructure accounting.

## Reproducible initial result

On October 10, 2026, macOS ARM64 development machine:

```sh
go run ./cmd/ding-cloud-bench --accounts 100 --duration 30s
```

| Measurement | Result |
| --- | ---: |
| Accounts with first observation | 100 / 100 |
| Synthetic checks | 100 |
| p95 time from apply to first observation | 1,104.718 ms |
| Sampled Go heap allocation | 5,212,536 bytes |
| Go runtime reserved memory | 32,332,056 bytes |
| Goroutines | 304 |
| Live data including WAL | 38,033,160 bytes |
| Data after orderly worker close | 23,087,560 bytes |
| Worker shutdown | 88.827 ms |

This uses one watch per account, five-minute cadence, 100 ms synthetic healthy
responses, 1 KiB bodies, real SQLite writes, and no external network. Memory is
Go allocation, not process RSS. A 30-second run measures initial acquisition;
it does not establish steady-state throughput, CPU, DNS/TLS cost, noisy-tenant
fairness, 72-hour reliability, seven-day retention growth, or provider pricing.
The benchmark removes only its own temporary databases afterward. Use durations
longer than five minutes for repeated acquisitions and independently capture RSS,
CPU, I/O, and filesystem behavior on the proposed deployment machine.

## Operational tradeoff and next gate

A controlled restart or operator-assisted recovery is the intended first failure
mode. This design has no cross-host automatic failover or availability SLA.
Moving the volume requires proving the previous owner has stopped. Keep a separate
observer for the service's own outages. Restore encrypted backups onto a fresh
host before enabling replacement execution; never run both copies simultaneously.

The initial result justifies continuing implementation without a PostgreSQL
rewrite. It does not establish that this architecture is best at 1,000 or 10,000
accounts. Revisit placement/storage when representative measurements justify it.
Provider selection, current quotations, an approved monthly budget, and named
operational ownership remain deployment decisions. No infrastructure was created
or paid service enabled by this implementation.
