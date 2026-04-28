# Using DING with Argo Workflows

> Argo Workflows is K8s-native multi-step orchestration — a controller watches `Workflow` CRs and schedules each step as a Pod. DING ships with each step's container — `ding run` wraps the step's command, evaluates rules in-Pod, and alerts when the Pod exits, automatically tagging each alert with workflow, node, pod, and namespace.

## Prerequisites

- DING binary `>= v0.6.0` — see [install](../install.md). The recipe pulls the official container image `ghcr.io/zuchka/ding:v0.6.0` (multi-arch, scratch base) into each step's Pod via an initContainer; no need to bake DING into your workload image.
- Argo Workflows controller installed in the cluster, `>= v3.5` (most users on v3.5/v3.6 LTS series).
- `kubectl` access to a namespace where you can create Workflows, ConfigMaps, and Secrets.
- `argo` CLI installed locally (ships with the controller; one-line install per [Argo docs](https://argo-workflows.readthedocs.io/en/latest/quick-start/)).
- A notifier endpoint (Slack webhook URL or custom webhook) you can store in a Kubernetes Secret.

## Minimal example

The shortest configuration that produces a working alert when an Argo Workflow step exits non-zero. The pattern: `ding run` wraps your command in the step's main container; an initContainer copies `/ding` from the published image into a shared `emptyDir`; the step's container mounts the `ding.yaml` ConfigMap directly and pulls Secret keys into its env via `envFrom`. DING expands `${SLACK_WEBHOOK_URL}` from the env at startup — no template-rendering step needed.

Save the YAML below as `ding-workflow.yaml` and apply with `kubectl apply -f ding-workflow.yaml`:

```yaml
---
# Notifier credential — replace with your real Slack webhook URL.
# The Secret key MUST be a valid env var name (uppercase, no dashes) so
# `envFrom: secretRef:` below can surface it directly into the step's
# container environment.
apiVersion: v1
kind: Secret
metadata:
  name: ding-secrets
type: Opaque
stringData:
  SLACK_WEBHOOK_URL: https://hooks.slack.com/services/T.../B.../...
---
# DING config. ${SLACK_WEBHOOK_URL} is expanded by DING itself at startup
# (see docs/configuration.md#environment-variable-substitution).
apiVersion: v1
kind: ConfigMap
metadata:
  name: ding-config
data:
  ding.yaml: |
    server:
      drain_timeout: 30s
    notifiers:
      slack:
        type: slack
        url: ${SLACK_WEBHOOK_URL}
    rules:
      - name: step_failed
        match:
          metric: run.exit
        condition: value > 0
        message: "Argo step {{ .pod }} (workflow {{ .workflow }}) failed with exit {{ .exit_code }}"
        alert:
          - notifier: slack
---
# Workflow — replace `image:` and the workload `command:` with your real workload.
# The example below intentionally exits 1 so the failure path can be observed.
apiVersion: argoproj.io/v1alpha1
kind: Workflow
metadata:
  generateName: ding-demo-
spec:
  entrypoint: main
  templates:
    - name: main
      volumes:
        - name: ding-bin
          emptyDir: {}
        - name: ding-config
          configMap:
            name: ding-config
      initContainers:
        - name: install-ding
          image: ghcr.io/zuchka/ding:v0.6.0
          # `ding install` self-copies the binary — works against the FROM-scratch
          # release image (no /bin/sh available). Added in DING v0.5.1.
          command: ["/ding", "install", "/shared/ding"]
          mirrorVolumeMounts: true
      container:
        image: alpine:3
        command:
          - /shared/ding
          - run
          - --config
          - /config/ding.yaml
          - --
          - /bin/sh
          - -c
          - "echo running; sleep 1; exit 1"
        envFrom:
          # Surfaces every key from ding-secrets as an env var inside the
          # step's container. DING expands ${SLACK_WEBHOOK_URL} from here.
          - secretRef:
              name: ding-secrets
        env:
          # Argo controller auto-injects ARGO_TEMPLATE, ARGO_WORKFLOW_UID,
          # ARGO_WORKFLOW_NAME, ARGO_NODE_ID, ARGO_POD_NAME. Only POD_NAMESPACE
          # needs explicit downward API.
          - name: POD_NAMESPACE
            valueFrom: { fieldRef: { fieldPath: metadata.namespace } }
        volumeMounts:
          - { name: ding-bin, mountPath: /shared }
          - { name: ding-config, mountPath: /config, readOnly: true }
```

This is the wedge's headline use case in Argo: silent step failure inside a multi-step DAG is exactly the observability gap the Argo controller doesn't solve on its own — it knows the workflow phase, but not why a specific step's program exited non-zero. DING surfaces that as an alert the moment the Pod exits.

## What you get

- An alert delivered on every non-zero step exit, automatically tagged with `workflow`, `node`, `pod`, `namespace`, `exit_code`, and `run_id`.
- A `runner=argo-workflows` label so rules can dispatch Argo alerts differently from bare-K8s alerts.
- Proper SIGTERM handling: when the workflow is terminated (manually via `argo terminate` or by the controller's `activeDeadlineSeconds`), `ding run` forwards the signal to the workload, drains in-flight notifier deliveries, then exits within `terminationGracePeriodSeconds`.
- Exit code propagation: the step's container exits with the workload's true exit code, so the Workflow node's `phase` (`Succeeded` vs `Failed`) reflects reality.

## Configuration

`runctx` auto-detects Argo Workflows via the presence of `ARGO_TEMPLATE` (controller-injected on every step's main container) and captures these labels:

| Label | Source |
|---|---|
| `run_id` | `ARGO_WORKFLOW_UID` (Workflow CR's K8s UID); falls back to `ARGO_WORKFLOW_NAME` |
| `runner` | `"argo-workflows"` (set by runctx) |
| `workflow` | `ARGO_WORKFLOW_NAME` (parent Workflow CR name) |
| `node` | `ARGO_NODE_ID` (per-step DAG node identifier) |
| `pod` | `ARGO_POD_NAME` (step's pod name) |
| `namespace` | `POD_NAMESPACE` (downward API `metadata.namespace` — must be declared in env block; not in Argo's auto-injected set) |

A self-hosted CI runner (GitHub Actions, GitLab CI, etc.) deployed on Argo-managed Kubernetes will set both its CI env vars *and* `ARGO_TEMPLATE`. In that case `runctx` reports the CI platform — its labels are richer for alerting purposes — and the Argo labels are skipped. See [Configuration](../configuration.md) for the full notifier reference.

### `drain_timeout` and `terminationGracePeriodSeconds`

Same constraint as the [Kubernetes Jobs recipe](kubernetes-jobs.md#drain_timeout-and-terminationgraceperiodseconds): SIGTERM-initiated graceful shutdown completes the notifier retry cycle only if `drain_timeout` is longer than `initial_backoff * 2^max_attempts` and the pod's `terminationGracePeriodSeconds` is longer than `drain_timeout`. The recipe sets `drain_timeout: 30s`; for production workflows, declare `terminationGracePeriodSeconds: 60` on the step's pod spec (`spec.templates[].podSpecPatch` or directly in the template's pod metadata). Argo additionally honors `activeDeadlineSeconds` on the Workflow CR — that's an upper bound on total runtime, not a substitute for grace period.

### Multi-step DAG

Real Argo workflows are multi-step DAGs. Define the wrapper pattern once as a reusable `dingstep` template parameterized by command, then reference it from `dag.tasks`:

```yaml
spec:
  entrypoint: pipeline
  templates:
    - name: pipeline
      dag:
        tasks:
          - name: prepare
            template: dingstep
            arguments: { parameters: [{ name: cmd, value: "echo prepare; sleep 1" }] }
          - name: train
            template: dingstep
            depends: prepare
            arguments: { parameters: [{ name: cmd, value: "echo train; sleep 1; exit 1" }] }
          - name: eval
            template: dingstep
            depends: train
            arguments: { parameters: [{ name: cmd, value: "echo eval" }] }

    - name: dingstep
      inputs:
        parameters:
          - { name: cmd }
      volumes:
        - { name: ding-bin, emptyDir: {} }
        - { name: ding-config, configMap: { name: ding-config } }
      initContainers:
        - name: install-ding
          image: ghcr.io/zuchka/ding:v0.6.0
          command: ["/ding", "install", "/shared/ding"]
          mirrorVolumeMounts: true
      container:
        image: alpine:3
        command:
          - /shared/ding
          - run
          - --config
          - /config/ding.yaml
          - --
          - /bin/sh
          - -c
          - "{{inputs.parameters.cmd}}"
        envFrom: [{ secretRef: { name: ding-secrets } }]
        env:
          - name: POD_NAMESPACE
            valueFrom: { fieldRef: { fieldPath: metadata.namespace } }
        volumeMounts:
          - { name: ding-bin, mountPath: /shared }
          - { name: ding-config, mountPath: /config, readOnly: true }
```

When `train` exits 1, DING in the `train` step's pod fires the alert; `eval` is skipped (Argo's default for failed dependencies). The Slack message contains:

- `workflow=<wf-name>` (shared across all three steps)
- `node=<wf>-train-<random>` (distinct per step)
- `pod=<wf>-train-<random>-<random>` (distinct per step; contains the step name as substring)
- `run_id=<wf-uid>` (shared across all three steps — identifies the Workflow run, not the step)

**Per-step matching is constrained.** DING's `match.labels` does exact-match comparison, and Argo's `node`/`pod` values are dynamic per Workflow run. You can't write a `match.labels: { node: my-train-step }` rule that matches "the train step." Pragmatic patterns:

- **Identify the step in the alert text**: use `{{ .pod }}` in the rule's `message` (the pod name contains the step name as substring). The user reading the Slack alert disambiguates visually.
- **Per-step rules via emitted labels**: emit during-run events from your script with an explicit `step` label — `print(json.dumps({"metric": "loss", "value": v, "labels": {"step": "train"}}))` — and write rules with `match.labels: { step: "train" }`. Works for during-run events; the synthetic `run.exit` still flows through unfiltered.

### Surfacing the template name (manual)

`runctx` does not parse `ARGO_TEMPLATE` JSON. If you want template-name labels on alerts, surface the controller-injected pod label via downward API:

```yaml
env:
  - name: TEMPLATE_NAME
    valueFrom:
      fieldRef:
        fieldPath: metadata.labels['workflows.argoproj.io/template']
```

Then your script can emit `TEMPLATE_NAME` as an event label. This is opt-in — the recipe doesn't include it in the minimal example.

## Verification

```bash
# One-time cluster setup (~3-5 min cold)
kind create cluster --name argo-smoke
kubectl create namespace argo
kubectl apply -n argo -f https://github.com/argoproj/argo-workflows/releases/download/v3.5.13/install.yaml
kubectl wait --for=condition=available --timeout=120s deployment/workflow-controller -n argo

# Smoke test 1 — single-step failure
export SLACK_WEBHOOK_URL=https://hooks.slack.com/services/...
kubectl apply -f ding-workflow.yaml         # the Minimal example manifest
argo wait @latest -n default                # blocks until completion
argo get @latest -n default                 # verify phase: Failed
argo logs @latest -n default                # verify DING drained on exit
# Verify Slack: workflow / node / pod / namespace / exit_code labels present

# Smoke test 2 — multi-step DAG (train fails, eval skipped)
kubectl apply -f ding-workflow-dag.yaml     # the Multi-step DAG manifest
argo wait @latest -n default
argo get @latest -n default                 # verify train Failed, eval Omitted

# Smoke test 3 — happy path (workload exits 0)
# Edit the minimal manifest's last command line to "exit 0"; reapply
kubectl apply -f ding-workflow-success.yaml
argo wait @latest -n default
# Verify NO Slack message arrives within 10s of completion
```

If the alert doesn't fire, common issues: the Secret wasn't readable (RBAC on the default ServiceAccount in the workflow's namespace), `SLACK_WEBHOOK_URL` was empty/missing in the Secret, or `terminationGracePeriodSeconds` was too tight for the Pod's actual deletion path.

## Tradeoffs / known limitations

- **Template name not auto-labeled.** `runctx` doesn't parse `ARGO_TEMPLATE` JSON. Surface template name manually via downward API on the `workflows.argoproj.io/template` pod label if needed (see [Surfacing the template name](#surfacing-the-template-name-manual)).
- **Per-step matching is constrained.** Parallel/fan-out steps share `run_id`; `node` and `pod` are dynamic per run, so DING's exact-match `match.labels` can't pre-target a specific DAG step's `run.exit`. Disambiguate in the message template, or emit step labels yourself for during-run rules.
- **`onExit` template alerts get a different `node`.** If you put DING in an `onExit` template instead of (or in addition to) the main step, its alerts have a different `node` label than the failed step they're reporting on, breaking per-step matching for users who copy-paste rules from the K8s recipe.
- **Workflow retries spawn new pods with same `ARGO_NODE_ID`.** A retried step gets a new `ARGO_POD_NAME` but reuses its `ARGO_NODE_ID` — alert frequency on a flapping step counts every retry; combine with rule cooldowns to dedup.
- **`sidecars:` field is an Argo-native alternative.** Argo predates K8s 1.29's native sidecar lifecycle and ships its own `sidecars:` template field. Using DING as a sidecar there works for during-run alerts but loses the `ding run` lifecycle (subprocess exit-code propagate, end-of-run drain) — not the primary recommendation.

## Escalation criteria

This recipe is **a Tier-2 candidate** by the program's standard rubric:

- **Setup commands required:** 1 (`kubectl apply`) — under threshold of 5
- **Boilerplate lines:** ~95 minimal + ~50 DAG subsection ≈ ~145 YAML — over threshold of 50 → **Tier-2 candidate**
- **"Gotcha" callouts:** 5 — over threshold of 2 → **Tier-2 candidate**
- **End-to-end runnable:** yes (kind + open-source Argo controller; ~3-5min cold)

**Tier-2 candidate.** The boilerplate count is the structural problem — both the minimal manifest and the DAG subsection are mostly mechanical plumbing (volumes, initContainers, downward API env block) that every Argo user will copy verbatim. An `argo-workflow-template` repo (separate, mirroring [`ding-k8s-job`](https://github.com/zuchka/ding-k8s-job)) that publishes a parameterized `WorkflowTemplate` to GHCR — invoked via `argo submit --from workflowtemplate/ding-step --parameter image=my-app --parameter command='python train.py' --parameter slack-url=$SLACK_WEBHOOK_URL` — would collapse the recipe to a one-line invocation. Defer the chart until 2+ users ask.
