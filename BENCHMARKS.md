# Watch runtime measurements

The old stream/CI comparison scripts and their results describe the legacy
runtime and are retained in the v0.14.0 Git tag. They are not capacity claims for
the watch runtime.

Current microbenchmarks:

```sh
go test ./benchmarks/go -run '^$' -bench . -benchmem
```

They measure pure evaluation, a bounded 60-sample rolling window, scalar JSON
projection, a durable push-to-event commit, and local HTTP delivery. They do not
replace the planned 24-hour capacity fixture or platform qualification. Full
workload, artifact sizes, hardware and measured results belong in the P14 report.
