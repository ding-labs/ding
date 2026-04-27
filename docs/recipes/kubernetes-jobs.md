# Using DING with Kubernetes Jobs and CronJobs

> Kubernetes Jobs and CronJobs are the canonical primitives for ephemeral, run-to-completion workloads. Grafana watches the cluster; DING ships with the work — `ding run` wraps your container's command, evaluates rules in-Pod, and alerts when the Pod exits, automatically tagging each alert with namespace, pod, node, and Job name.

## Prerequisites

- DING binary `>= v0.5.0` — see [install](../install.md). The recipe pulls the official container image `ghcr.io/zuchka/ding:v0.5.0` (multi-arch, scratch base) into your Pod via an initContainer; no need to bake DING into your workload image.
- A Kubernetes cluster `>= 1.21` for the primary wrapper pattern below. The sidecar alternative documented in [Configuration](#sidecar-alternative-k8s-129) requires `>= 1.29` for native sidecar lifecycle.
- `kubectl` access to a namespace where you can create Jobs, ConfigMaps, and Secrets.
- A notifier endpoint (Slack webhook URL or custom webhook) you can store in a Kubernetes Secret.

## Minimal example

The shortest configuration that produces a working alert when a Job exits non-zero. The pattern: `ding run` wraps your command in the main container; an initContainer copies `/ding` from the published image into a shared `emptyDir`; a second initContainer renders the `ding.yaml` template by substituting environment variables sourced from a Secret.

Save the YAML below as `ding-job.yaml` and apply with `kubectl apply -f ding-job.yaml`:

```yaml
---
# Notifier credential — replace with your real Slack webhook URL.
apiVersion: v1
kind: Secret
metadata:
  name: ding-secrets
type: Opaque
stringData:
  slack-webhook: https://hooks.slack.com/services/T.../B.../...
---
# DING config template. ${SLACK_WEBHOOK_URL} is rendered at Pod startup
# by the render-config initContainer below.
apiVersion: v1
kind: ConfigMap
metadata:
  name: ding-config
data:
  ding.yaml.tpl: |
    server:
      drain_timeout: 30s
    notifiers:
      slack:
        type: slack
        url: ${SLACK_WEBHOOK_URL}
    rules:
      - name: job_failed
        match:
          metric: run.exit
        condition: value != 0
        mode: end-of-run
        message: "{{ .pod }} (Job {{ .job_name }}) failed with exit {{ .exit_code }}"
        alert:
          - notifier: slack
---
# Job — replace `image:` and the workload `command:` with your real workload.
# The example below intentionally exits 1 so the failure path can be observed.
apiVersion: batch/v1
kind: Job
metadata:
  name: my-job
spec:
  backoffLimit: 0
  template:
    spec:
      restartPolicy: Never
      terminationGracePeriodSeconds: 60
      volumes:
        - name: ding-bin
          emptyDir: {}
        - name: ding-rendered
          emptyDir: {}
        - name: ding-tpl
          configMap:
            name: ding-config
      initContainers:
        - name: install-ding
          image: ghcr.io/zuchka/ding:v0.5.0
          command: ["/bin/sh", "-c", "cp /ding /shared/ding"]
          volumeMounts:
            - { name: ding-bin, mountPath: /shared }
        - name: render-config
          image: alpine:3
          command:
            - /bin/sh
            - -c
            - apk add --no-cache gettext && envsubst < /tpl/ding.yaml.tpl > /rendered/ding.yaml
          env:
            - name: SLACK_WEBHOOK_URL
              valueFrom:
                secretKeyRef: { name: ding-secrets, key: slack-webhook }
          volumeMounts:
            - { name: ding-tpl, mountPath: /tpl }
            - { name: ding-rendered, mountPath: /rendered }
      containers:
        - name: workload
          image: alpine:3
          command:
            - /shared/ding
            - run
            - --config
            - /rendered/ding.yaml
            - --
            - /bin/sh
            - -c
            - echo running; sleep 1; exit 1
          env:
            # Downward API surfaces Pod metadata as env vars; runctx auto-labels alerts with these.
            - name: POD_UID
              valueFrom: { fieldRef: { fieldPath: metadata.uid } }
            - name: POD_NAME
              valueFrom: { fieldRef: { fieldPath: metadata.name } }
            - name: POD_NAMESPACE
              valueFrom: { fieldRef: { fieldPath: metadata.namespace } }
            - name: NODE_NAME
              valueFrom: { fieldRef: { fieldPath: spec.nodeName } }
            - name: JOB_NAME
              valueFrom: { fieldRef: { fieldPath: "metadata.labels['job-name']" } }
          volumeMounts:
            - { name: ding-bin, mountPath: /shared }
            - { name: ding-rendered, mountPath: /rendered }
```

For a CronJob, wrap the same `template:` block in a `jobTemplate:`:

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: nightly-batch
spec:
  schedule: "0 2 * * *"        # 02:00 UTC daily
  concurrencyPolicy: Forbid
  jobTemplate:
    spec:
      backoffLimit: 0
      template:
        # Identical to the Job's spec.template above:
        # restartPolicy, terminationGracePeriodSeconds, volumes,
        # initContainers, containers, downward-API env block.
        ...
```

This is the wedge's headline use case in K8s: silent CronJob failure is the most universal observability pain in the platform, and DING surfaces it as an alert the moment the Pod exits non-zero — no scrape interval, no Pushgateway, no separate backend.

## What you get

- An alert delivered on every non-zero Pod exit, automatically tagged with `namespace`, `pod`, `node`, `job_name`, and `exit_code`.
- A `runner=kubernetes` label so rules can dispatch K8s alerts differently from CI alerts.
- Proper SIGTERM handling: when the Pod is deleted (manually or by the scheduler), `ding run` forwards the signal to the workload, drains in-flight notifier deliveries, then exits — within `terminationGracePeriodSeconds`.
- Exit code propagation: the workload container reports the workload's true exit code, so the Job condition (`Complete` vs `Failed`) reflects reality.

## Configuration

`runctx` auto-detects Kubernetes via the presence of `KUBERNETES_SERVICE_HOST` (kubelet-injected on every Pod with a service account, which is the default) and captures these labels:

| Label | Source |
|---|---|
| `run_id` | `POD_UID` (downward API `metadata.uid`); falls back to `POD_NAME` |
| `runner` | `"kubernetes"` (set by runctx) |
| `namespace` | `POD_NAMESPACE` (downward API `metadata.namespace`) |
| `pod` | `POD_NAME` (downward API `metadata.name`) |
| `node` | `NODE_NAME` (downward API `spec.nodeName`) |
| `job_name` | `JOB_NAME` (downward API `metadata.labels['job-name']` — auto-injected by the Job controller) |

For CronJobs, `job_name` is the spawned Job's randomized name (e.g. `nightly-batch-1737848400`); the parent CronJob's name isn't directly available via downward API. If you want the CronJob name in alerts, surface it explicitly via a label on `jobTemplate.spec.template.metadata.labels` plus an additional `fieldRef` on the env block.

A self-hosted CI runner (GitHub Actions, GitLab CI, etc.) deployed on Kubernetes will set both its CI env vars *and* `KUBERNETES_SERVICE_HOST`. In that case `runctx` reports the CI platform — its labels are richer for alerting purposes — and the K8s labels are skipped. See [Configuration](../configuration.md) for the full notifier reference.

### `drain_timeout` and `terminationGracePeriodSeconds`

Kubernetes sends SIGTERM on Pod deletion, then waits up to `terminationGracePeriodSeconds` (default 30) before force-killing with SIGKILL. DING's `ding run` traps SIGTERM, forwards it to the child, then drains queued notifier deliveries before exiting — but only up to `server.drain_timeout` (default 5s).

The defaults are unsafe in practice. With Slack/PagerDuty's default `initial_backoff: 1s` and `max_attempts: 3`, a full retry cycle takes ~7s — which the default 5s drain truncates silently. The recipe sets `drain_timeout: 30s` and `terminationGracePeriodSeconds: 60` so a SIGTERM-initiated graceful shutdown completes the retry cycle without truncation. Tune higher if you have notifiers with longer retry policies.

### Sidecar alternative (K8s 1.29+)

The wrapper pattern above requires modifying the workload container's `command:`. If you can't (e.g. third-party image with a fixed entrypoint), run DING as a **native sidecar** instead:

```yaml
spec:
  template:
    spec:
      initContainers:
        - name: ding
          image: ghcr.io/zuchka/ding:v0.5.0
          restartPolicy: Always       # native sidecar — K8s 1.29+
          command: ["/ding", "serve", "--config", "/etc/ding/ding.yaml"]
          # ...volumeMounts for config + downward-API env block
      containers:
        - name: workload
          image: third-party/image:tag
          # ...workload posts events to http://localhost:8080/events
```

The workload sends events to DING over the Pod's loopback. DING runs as a long-lived `serve` process; native sidecar lifecycle (initContainer with `restartPolicy: Always`) ensures the sidecar is auto-killed when the workload exits, so the Job can complete. **This pattern requires Kubernetes 1.29 or later** — earlier versions hit the long-standing Job-completion deadlock where sidecars never exit on their own.

The sidecar pattern is heavier (separate container, IPC over HTTP, no `ding run` lifecycle semantics) so use it only when the wrapper pattern can't apply.

## Verification

1. Locally: `ding validate --config <(envsubst < ding.yaml.tpl)` — confirms the rule parses.
2. Apply the manifests: `kubectl apply -f ding-job.yaml`.
3. Wait for the Job: `kubectl wait --for=condition=failed job/my-job --timeout=60s`. With the example's `exit 1`, the Job should reach `Failed` quickly.
4. Confirm the alert: a Slack message tagged with `pod`, `namespace`, `node`, `job_name`, and `exit_code`. Check `kubectl logs job/my-job -c workload` for DING's drain output.
5. Trigger the happy path: change the workload `command:` last line from `exit 1` to `exit 0`, reapply, wait for `Complete`. Confirm no alert fires.
6. CronJob path: replace `kind: Job` with the CronJob example, wait for the first spawned Job to complete, verify the same alert behavior on a forced failure.

If the alert doesn't fire, common issues: the `render-config` initContainer failed (check `kubectl logs job/my-job -c render-config`), the Secret wasn't readable (RBAC on the default ServiceAccount), or `terminationGracePeriodSeconds` was too tight for the Pod's actual deletion path.

## Tradeoffs / known limitations

- **Wrapper pattern requires modifying `command`.** If the workload's entrypoint is fixed (third-party image), use the [sidecar alternative](#sidecar-alternative-k8s-129) — but it requires K8s 1.29+ for native sidecar lifecycle.
- **No K8s API access by default.** DING ships alerts to external notifiers (Slack/Discord/PagerDuty/etc.), not as K8s Events visible to `kubectl describe pod` or `kubectl get events`. A native `type: kubernetes_event` notifier is a flagged Tier-2 candidate.
- **YAML credentials need an envsubst initContainer.** DING's YAML parser doesn't yet expand `${VAR}` references natively, so the recipe runs an `alpine + envsubst` initContainer to render `ding.yaml` from `ding.yaml.tpl` at Pod startup. A native parser feature is a flagged Tier-2 candidate that would simplify this.
- **No CronJob-name auto-label.** The Job controller injects `job-name`, but the parent CronJob's name has to be surfaced manually via a label on `jobTemplate.spec.template.metadata.labels` plus an additional `fieldRef`.
- **Pre-1.29 sidecar pattern needs lifecycle workarounds.** If you must support older clusters, the historical pattern (an emptyDir flag file plus a `pkill` in a lifecycle hook) is documented in upstream K8s docs; it's outside this recipe's scope.

## Escalation criteria

This recipe is **a Tier-2 candidate** by the program's standard rubric:

- **Setup commands required:** 1 (`kubectl apply`) — under threshold of 5
- **Boilerplate lines:** ~95 (single-document YAML for Secret + ConfigMap + Job) — over threshold of 50 → **Tier-2 candidate**
- **"Gotcha" callouts:** 4 (envsubst initContainer, drain/terminationGracePeriod pairing, sidecar gates on K8s 1.29+, no CronJob-name auto-label) — over threshold of 2 → **Tier-2 candidate**
- **End-to-end runnable:** yes (kind / minikube are free and self-installable in a few minutes)

**Tier-2 candidate.** The boilerplate count is the structural problem — the manifest is mostly mechanical plumbing (volumes, initContainers, downward API env block) that every K8s user will copy verbatim. A `ding-k8s-job` Helm chart (separate repo, mirroring the [`ding-action`](https://github.com/zuchka/ding-action) pattern) that templates the wrapper-pattern manifest behind `helm install ding-k8s-job ... --set image=my-app --set command='python train.py'` would collapse the recipe to a one-line install. Defer until 2+ users ask, OR until the related Tier-2 candidate (`${VAR}` substitution in the YAML parser) ships and changes the cost calculus.
