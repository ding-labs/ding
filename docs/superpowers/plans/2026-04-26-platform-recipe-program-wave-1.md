# Platform Recipe Program — Wave 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up `docs/recipes/` infrastructure (template + index + mkdocs nav) and ship Wave 1's four CI/CD recipes (GitLab CI, CircleCI, Jenkins, Buildkite) per the spec at `docs/superpowers/specs/2026-04-26-platform-recipe-program-design.md`.

**Architecture:** Documentation-only. New `docs/recipes/` directory with a canonical `_template.md`, an `index.md` status table, and four platform recipe pages. `mkdocs.yml` gains a "Recipes" nav section. Cross-links added from `README.md` and `docs/configuration.md`. **Zero changes** to `internal/` Go code — recipes leverage DING's existing `ding run` + `internal/runctx/runctx.go` runner auto-detection (verified at `internal/runctx/runctx.go:64-89`).

**Tech Stack:** Markdown (mkdocs Material theme), YAML (mkdocs config + DING config examples in recipes), Python (yaml-parsing fallback when `mkdocs` CLI is unavailable).

---

## Reference: spec and source files

- Spec: `docs/superpowers/specs/2026-04-26-platform-recipe-program-design.md`
- Auto-detected runner env vars: `internal/runctx/runctx.go:52-89`
- Existing notifier docs to cross-link: `docs/configuration.md`
- mkdocs nav structure: `mkdocs.yml`

---

## Task 1: Plumbing — template, index, mkdocs nav

**Files:**
- Create: `docs/recipes/_template.md`
- Create: `docs/recipes/index.md`
- Modify: `mkdocs.yml`

- [ ] **Step 1: Create the canonical recipe template**

Write `docs/recipes/_template.md` with this exact content:

```markdown
# Using DING with <Platform>

<!--
This is the canonical template every recipe in docs/recipes/ must conform to.
When writing a new recipe: copy this file, rename it `<platform>.md` (lowercase,
hyphens), and replace the placeholders. Keep the section structure verbatim —
the program's value comes from every recipe being judgeable against the same
rubric.
-->

> **Two-sentence framing.** What is this platform? Why does co-mortal observability fit?

## Prerequisites

- DING binary `>= v0.3.0` — see [install](../install.md)
- <Platform-specific requirements: account, runtime version, etc.>
- A notifier endpoint (Slack webhook URL, custom webhook, etc.)

## Minimal example

The shortest config that produces a working alert when the workload exits non-zero.

```yaml
# ding.yaml
notifiers:
  slack:
    type: slack
    url: $SLACK_WEBHOOK_URL

rules:
  - name: job_failed
    match:
      metric: run.exit
    condition: value != 0
    mode: end-of-run
    message: "Job failed (exit {{ .exit_code }})"
    alert:
      - notifier: slack
```

```yaml
# Platform-specific config — wrap your job command in `ding run`
# <platform-specific YAML / shell snippet here>
```

## What you get

A textual or screenshot rendering of the alert as actually delivered. Include the auto-captured labels visible in the alert.

## Configuration

`runctx` auto-detects this platform and captures these labels:

| Label | Env var on the platform |
|---|---|
| `run_id` | `<env var>` |
| `runner` | `"<platform-slug>"` (set by runctx) |
| `repo` | `<env var>` |
| `branch` | `<env var>` |
| `commit` | `<env var>` |
| `job` | `<env var>` |

Use these as label keys in `match.labels` or `message` template variables.

## Verification

1. Run `ding validate --config ding.yaml` locally to confirm the rule parses.
2. Trigger a job that exits 0 — confirm no alert fires.
3. Trigger a job that exits non-zero — confirm the alert fires within `drain_timeout` of job exit.

## Tradeoffs / known limitations

- <Honest list of what the recipe doesn't solve.>
- <Especially: anything the platform forces a workaround for.>

## Escalation criteria

This recipe is **<Tier 1 | a Tier 2 candidate>** by the program's standard rubric:

- **Setup commands required:** `<N>` — <under / over> threshold of 5
- **Boilerplate lines:** `<N>` — <under / over> threshold of 50
- **"Gotcha" callouts:** `<N>` — <under / over> threshold of 2
- **End-to-end runnable:** <yes / no — reason>

<If Tier 2 candidate: one-paragraph note on what abstraction (`ding-<platform>` repo, Helm chart, plugin, etc.) would collapse the friction.>
```

- [ ] **Step 2: Create the index page**

Write `docs/recipes/index.md` with this content:

```markdown
# Recipes

Concrete configurations for using DING with specific platforms. Every recipe shows the minimal config needed to get an alert firing when a workload exits, then enumerates the auto-captured labels and any platform-specific tradeoffs.

Recipes are organized into three waves matching the [program spec](../superpowers/specs/2026-04-26-platform-recipe-program-design.md). Each recipe self-judges against the program's escalation rubric — recipes flagged as **Tier-2 candidates** indicate where a future `ding-<platform>` integration repo would be useful.

## Status

| Recipe | Category | Status |
|---|---|---|
| [GitHub Actions](https://github.com/zuchka/ding-action) | CI/CD | shipped — separate repo (`ding-action`) |
| [GitLab CI](gitlab-ci.md) | CI/CD | pending |
| [CircleCI](circleci.md) | CI/CD | pending |
| [Jenkins](jenkins.md) | CI/CD | pending |
| [Buildkite](buildkite.md) | CI/CD | pending |

## Template

The canonical recipe shape lives in [`_template.md`](_template.md). All recipes conform to it.
```

- [ ] **Step 3: Update mkdocs.yml nav**

Edit `mkdocs.yml`. Find the existing nav block:

```yaml
nav:
  - Home: index.md
  - Install: install.md
  - Configuration: configuration.md
  - HTTP API: api.md
  - Examples: examples.md
  - CLI Reference:
    - ding: cli/ding.md
    - serve: cli/ding_serve.md
    - validate: cli/ding_validate.md
    - version: cli/ding_version.md
```

Replace with:

```yaml
nav:
  - Home: index.md
  - Install: install.md
  - Configuration: configuration.md
  - HTTP API: api.md
  - Examples: examples.md
  - Recipes:
    - Overview: recipes/index.md
    - GitLab CI: recipes/gitlab-ci.md
    - CircleCI: recipes/circleci.md
    - Jenkins: recipes/jenkins.md
    - Buildkite: recipes/buildkite.md
  - CLI Reference:
    - ding: cli/ding.md
    - serve: cli/ding_serve.md
    - validate: cli/ding_validate.md
    - version: cli/ding_version.md
```

(`_template.md` is intentionally NOT in nav — it's authoring scaffolding, not user-facing content.)

- [ ] **Step 4: Verify mkdocs.yml parses**

Run:

```bash
python3 -c "import yaml; yaml.safe_load(open('mkdocs.yml'))" && echo "OK: mkdocs.yml parses as valid YAML"
```

Expected: `OK: mkdocs.yml parses as valid YAML`

If `mkdocs` CLI is available (optional, uncommon on dev machines), also run `mkdocs build --strict` and confirm no errors. Otherwise the YAML parse is sufficient — broken navigation references will be caught by GitHub Pages CI on commit.

- [ ] **Step 5: Commit plumbing**

```bash
git add docs/recipes/_template.md docs/recipes/index.md mkdocs.yml
git commit -m "docs(recipes): scaffold platform recipe program

Adds docs/recipes/ with canonical template and index page; wires Recipes
into mkdocs nav with placeholder entries for Wave 1 platforms.

Refs: docs/superpowers/specs/2026-04-26-platform-recipe-program-design.md"
```

---

## Task 2: Wave 1 Recipe — GitLab CI

**Files:**
- Create: `docs/recipes/gitlab-ci.md`
- Modify: `docs/recipes/index.md` (mark status `shipped`)

- [ ] **Step 1: Write the recipe**

Write `docs/recipes/gitlab-ci.md` with this exact content:

```markdown
# Using DING with GitLab CI

> GitLab CI is GitLab's native CI/CD runner. DING's `ding run` subcommand wraps a CI job, captures its output, evaluates rules during the run, and fires alerts when the job exits — without any backend infrastructure.

## Prerequisites

- DING binary `>= v0.3.0` — see [install](../install.md)
- A GitLab project with CI enabled (gitlab.com or self-hosted)
- A notifier endpoint (Slack webhook URL, custom webhook, etc.) accessible from the runner

## Minimal example

`.gitlab-ci.yml`:

```yaml
test_with_ding:
  image: alpine:latest
  before_script:
    - apk add --no-cache curl tar
    - curl -sSL https://github.com/zuchka/ding/releases/latest/download/ding_linux_amd64.tar.gz | tar -xz
  script:
    - ./ding run --config ding.yaml -- ./run-tests.sh
```

`ding.yaml` (committed alongside `.gitlab-ci.yml`):

```yaml
notifiers:
  slack:
    type: slack
    url: $SLACK_WEBHOOK_URL

rules:
  - name: ci_job_failed
    match:
      metric: run.exit
    condition: value != 0
    mode: end-of-run
    message: "Pipeline {{ .branch }} failed (exit {{ .exit_code }}, {{ .duration_seconds }}s)"
    alert:
      - notifier: slack
```

Set `SLACK_WEBHOOK_URL` as a [protected CI/CD variable](https://docs.gitlab.com/ci/variables/) in your project settings.

## What you get

A Slack message when the pipeline exits non-zero, automatically tagged with `repo`, `branch`, `commit`, `job` pulled from GitLab's environment. Successful runs produce no notification.

## Configuration

`runctx` auto-detects GitLab CI via the `GITLAB_CI=true` environment variable and captures these labels:

| Label | GitLab env var |
|---|---|
| `run_id` | `CI_PIPELINE_ID` |
| `runner` | `"gitlab-ci"` (set by runctx) |
| `repo` | `CI_PROJECT_PATH` |
| `branch` | `CI_COMMIT_REF_NAME` |
| `commit` | `CI_COMMIT_SHA` |
| `job` | `CI_JOB_NAME` |

Use these in `match.labels` for selective rules, or in `message` templates as `{{ .branch }}`, `{{ .commit }}`, etc. See [Configuration](../configuration.md) for the full notifier reference.

## Verification

1. Locally: `ding validate --config ding.yaml` — confirms the rule parses.
2. Push a commit. Confirm the pipeline runs and that a successful job produces no alert.
3. Force a failure: change `run-tests.sh` to `exit 1`. Confirm the alert fires in Slack within ~5 seconds of job exit.

If the alert doesn't fire, check the GitLab CI job log for `ding` output. Common issues: webhook URL not exposed to the job (mark the variable as not "Protected" if testing on a non-protected branch), or `drain_timeout` shorter than the notifier retry window — see [Configuration → drain_timeout](../configuration.md).

## Tradeoffs / known limitations

- **No native step-summary surface.** GitHub Actions has `$GITHUB_STEP_SUMMARY`; GitLab does not. Alerts go to your notifier of choice (Slack, webhook, etc.), not into the GitLab UI itself. Surfacing alerts back into GitLab would require a future Tier-2 abstraction (an artifact-writing notifier) — see escalation criteria below.
- **Binary download per job.** The minimal example downloads DING from GitHub Releases each run (~4MB tarball, plus an `apk add curl tar` round-trip on `alpine:latest` — typically 5–10s of preamble cold). For high-frequency pipelines, bake DING and its dependencies into your CI image instead.

## Escalation criteria

This recipe is **Tier 1** by the program's standard rubric:

- **Setup commands required:** 2 (`apk add`, `curl | tar`) — under threshold of 5
- **Boilerplate lines:** ~24 across `.gitlab-ci.yml` and `ding.yaml` — under threshold of 50
- **"Gotcha" callouts:** 2 (no step-summary surface, binary download per job) — at threshold of 2
- **End-to-end runnable:** yes (gitlab.com has a free tier sufficient for evaluation; minutes allotment varies — see [GitLab pricing](https://about.gitlab.com/pricing/))

The "no step-summary surface" callout is the only structural friction. If users start asking for GitLab-native alert surfacing, that's the trigger to promote this to Tier 2 with an artifact-writing notifier (`type: gitlab_artifact` or similar).
```

- [ ] **Step 2: Update index status**

In `docs/recipes/index.md`, change the GitLab CI row from `pending` to `shipped`:

```markdown
| [GitLab CI](gitlab-ci.md) | CI/CD | shipped |
```

- [ ] **Step 3: Verify mkdocs.yml still parses**

```bash
python3 -c "import yaml; yaml.safe_load(open('mkdocs.yml'))" && echo "OK"
```

Expected: `OK`

- [ ] **Step 4: Commit**

```bash
git add docs/recipes/gitlab-ci.md docs/recipes/index.md
git commit -m "docs(recipes): GitLab CI recipe (Wave 1)

Tier 1 by self-evaluation: 3 setup commands, ~25 boilerplate lines, 2
gotchas (no step-summary surface, binary download per job), end-to-end
runnable on gitlab.com free tier.

Refs: docs/superpowers/specs/2026-04-26-platform-recipe-program-design.md"
```

---

## Task 3: Wave 1 Recipe — CircleCI

**Files:**
- Create: `docs/recipes/circleci.md`
- Modify: `docs/recipes/index.md`

- [ ] **Step 1: Write the recipe**

Write `docs/recipes/circleci.md` with this exact content:

```markdown
# Using DING with CircleCI

> CircleCI is a hosted CI/CD platform built around YAML-defined workflows. DING's `ding run` wraps a job step, evaluates rules, and fires alerts on exit — leveraging CircleCI's environment to auto-tag every alert with project, branch, and commit context.

## Prerequisites

- DING binary `>= v0.3.0` — see [install](../install.md)
- A CircleCI project (free tier covers most personal/OSS use)
- A notifier endpoint (Slack webhook URL, custom webhook, etc.)

## Minimal example

`.circleci/config.yml`:

```yaml
version: 2.1

jobs:
  test:
    docker:
      - image: cimg/base:current
    steps:
      - checkout
      - run:
          name: Install DING
          command: |
            curl -sSL https://github.com/zuchka/ding/releases/latest/download/ding_linux_amd64.tar.gz | tar -xz
      - run:
          name: Run tests with DING
          command: ./ding run --config ding.yaml -- ./run-tests.sh

workflows:
  test_workflow:
    jobs:
      - test
```

`ding.yaml`:

```yaml
notifiers:
  slack:
    type: slack
    url: $SLACK_WEBHOOK_URL

rules:
  - name: ci_job_failed
    match:
      metric: run.exit
    condition: value != 0
    mode: end-of-run
    message: "{{ .repo }}@{{ .branch }} failed (exit {{ .exit_code }}, {{ .duration_seconds }}s)"
    alert:
      - notifier: slack
```

Set `SLACK_WEBHOOK_URL` as an [environment variable](https://circleci.com/docs/env-vars/) in your CircleCI project settings.

## What you get

A Slack message when the job exits non-zero, automatically tagged with `repo`, `branch`, `commit`, `job` from CircleCI's environment.

## Configuration

`runctx` auto-detects CircleCI via the `CIRCLECI=true` environment variable and captures these labels:

| Label | CircleCI env var |
|---|---|
| `run_id` | `CIRCLE_BUILD_NUM` |
| `runner` | `"circleci"` (set by runctx) |
| `repo` | `CIRCLE_PROJECT_REPONAME` |
| `branch` | `CIRCLE_BRANCH` |
| `commit` | `CIRCLE_SHA1` |
| `job` | `CIRCLE_JOB` |

Use these in `match.labels` or `message` templates.

## Verification

1. Locally: `ding validate --config ding.yaml` — confirms the rule parses.
2. Push a commit. Confirm a successful job produces no alert.
3. Force a failure (`exit 1` in `run-tests.sh`). Confirm the alert fires in Slack within ~5 seconds of job exit.

## Tradeoffs / known limitations

- **No native annotation surface.** CircleCI doesn't have a step-summary equivalent to GitHub Actions' `$GITHUB_STEP_SUMMARY`. Alerts go to your notifier; CircleCI's UI shows DING's stdout.
- **Binary download per job.** Bake DING into a [custom Docker image](https://circleci.com/docs/custom-images/) for high-frequency workflows.
- **Orb not provided.** A CircleCI orb (a packaged config wrapper) would collapse the install step into one line — that's the most likely Tier-2 promotion target.

## Escalation criteria

This recipe is **Tier 1** by the program's standard rubric:

- **Setup commands required:** 1 (`curl | tar`) — under threshold of 5
- **Boilerplate lines:** ~28 across `.circleci/config.yml` and `ding.yaml` — under threshold of 50
- **"Gotcha" callouts:** 3 (no annotation surface, binary download per job, no orb) — over threshold of 2 → **Tier-2 candidate**
- **End-to-end runnable:** yes (CircleCI free tier covers ~6,000 build minutes/month for OSS projects)

**Tier-2 candidate.** Three callouts cross the rubric threshold. The natural Tier-2 abstraction is a CircleCI orb (`zuchka/ding`) that exposes a `ding/run` step — collapsing the install + invoke pattern into one line. Defer until 2+ users ask for it (per spec §"Open Questions" promotion authority).
```

- [ ] **Step 2: Update index status**

In `docs/recipes/index.md`, change the CircleCI row:

```markdown
| [CircleCI](circleci.md) | CI/CD | shipped (Tier-2 candidate) |
```

- [ ] **Step 3: Verify mkdocs.yml still parses**

```bash
python3 -c "import yaml; yaml.safe_load(open('mkdocs.yml'))" && echo "OK"
```

Expected: `OK`

- [ ] **Step 4: Commit**

```bash
git add docs/recipes/circleci.md docs/recipes/index.md
git commit -m "docs(recipes): CircleCI recipe (Wave 1, Tier-2 candidate)

Self-evaluated as Tier-2 candidate: 3 gotcha callouts crosses rubric
threshold of 2. Natural Tier-2 abstraction is a CircleCI orb (zuchka/ding)
collapsing install + invoke into one step. Deferred until 2+ user requests.

Refs: docs/superpowers/specs/2026-04-26-platform-recipe-program-design.md"
```

---

## Task 4: Wave 1 Recipe — Jenkins

**Files:**
- Create: `docs/recipes/jenkins.md`
- Modify: `docs/recipes/index.md`

- [ ] **Step 1: Write the recipe**

Write `docs/recipes/jenkins.md` with this exact content:

```markdown
# Using DING with Jenkins

> Jenkins is the long-standing self-hosted CI server. DING's `ding run` wraps a pipeline step, evaluates rules during execution, and fires alerts on exit — automatically tagging each alert with the Jenkins job name and build number.

## Prerequisites

- DING binary `>= v0.3.0` — see [install](../install.md)
- A Jenkins controller (any version supporting Pipeline DSL — most do)
- A notifier endpoint (Slack webhook URL, custom webhook, etc.) reachable from the Jenkins agent

## Minimal example

Declarative `Jenkinsfile`:

```groovy
pipeline {
    agent any
    stages {
        stage('Install DING') {
            steps {
                sh '''
                    curl -sSL https://github.com/zuchka/ding/releases/latest/download/ding_linux_amd64.tar.gz | tar -xz
                '''
            }
        }
        stage('Test') {
            steps {
                withCredentials([string(credentialsId: 'slack-webhook', variable: 'SLACK_WEBHOOK_URL')]) {
                    sh './ding run --config ding.yaml -- ./run-tests.sh'
                }
            }
        }
    }
}
```

`ding.yaml`:

```yaml
notifiers:
  slack:
    type: slack
    url: $SLACK_WEBHOOK_URL

rules:
  - name: ci_job_failed
    match:
      metric: run.exit
    condition: value != 0
    mode: end-of-run
    message: "{{ .job }} build {{ .build }} failed (exit {{ .exit_code }})"
    alert:
      - notifier: slack
```

Add the Slack webhook as a [secret text credential](https://www.jenkins.io/doc/book/using/using-credentials/) named `slack-webhook`.

## What you get

A Slack message when the build exits non-zero, tagged with the Jenkins job name and build number.

## Configuration

`runctx` auto-detects Jenkins via the presence of `JENKINS_URL` and captures these labels:

| Label | Jenkins env var |
|---|---|
| `run_id` | `BUILD_TAG` |
| `runner` | `"jenkins"` (set by runctx) |
| `job` | `JOB_NAME` |
| `build` | `BUILD_NUMBER` |

Note: Jenkins doesn't expose `repo`, `branch`, or `commit` as universal env vars (those depend on which SCM plugin is in use). To capture them, add explicit env vars in your Jenkinsfile from the SCM step's metadata.

## Verification

1. Locally: `ding validate --config ding.yaml` — confirms the rule parses.
2. Trigger the job. Confirm a successful build produces no alert.
3. Force a failure (`exit 1` in `run-tests.sh`). Confirm the alert fires in Slack within ~5 seconds of build exit.

## Tradeoffs / known limitations

- **No SCM-aware labels by default.** Unlike GitHub Actions / GitLab CI / CircleCI, Jenkins doesn't have a single `BRANCH` env var that works across all SCM plugins. You'll need to surface `GIT_BRANCH` / `GIT_COMMIT` (Git plugin) or equivalent yourself.
- **Binary download per job.** Cache DING in a Docker agent image, or as a [Tool Installation](https://www.jenkins.io/doc/book/managing/tools/) configuration on the controller.
- **No native plugin (yet).** A Jenkins plugin would expose alerts in the build console UI alongside DING's stdout. That's the most likely Tier-2 abstraction.

## Escalation criteria

This recipe is **a Tier-2 candidate** by the program's standard rubric:

- **Setup commands required:** 1 (`curl | tar`) — under threshold of 5
- **Boilerplate lines:** ~28 — under threshold of 50
- **"Gotcha" callouts:** 3 (no SCM-aware labels, binary download, no plugin) — over threshold of 2 → **Tier-2 candidate**
- **End-to-end runnable:** yes (Jenkins is free; self-hostable in 5 minutes via Docker)

**Tier-2 candidate.** The "no SCM-aware labels" friction is the structural problem — every Jenkins user wants `branch` and `commit` in their alerts, and the recipe leaves that as homework. A Jenkins plugin (`ding-jenkins-plugin`) that pulls SCM metadata into runctx labels would collapse this. Defer until 2+ users ask.
```

- [ ] **Step 2: Update index status**

In `docs/recipes/index.md`:

```markdown
| [Jenkins](jenkins.md) | CI/CD | shipped (Tier-2 candidate) |
```

- [ ] **Step 3: Verify mkdocs.yml still parses**

```bash
python3 -c "import yaml; yaml.safe_load(open('mkdocs.yml'))" && echo "OK"
```

Expected: `OK`

- [ ] **Step 4: Commit**

```bash
git add docs/recipes/jenkins.md docs/recipes/index.md
git commit -m "docs(recipes): Jenkins recipe (Wave 1, Tier-2 candidate)

Self-evaluated as Tier-2 candidate: SCM-aware label capture is the missing
abstraction. Tier-2 target is a Jenkins plugin (ding-jenkins-plugin)
surfacing GIT_BRANCH/GIT_COMMIT into runctx labels.

Refs: docs/superpowers/specs/2026-04-26-platform-recipe-program-design.md"
```

---

## Task 5: Wave 1 Recipe — Buildkite

**Files:**
- Create: `docs/recipes/buildkite.md`
- Modify: `docs/recipes/index.md`

- [ ] **Step 1: Write the recipe**

Write `docs/recipes/buildkite.md` with this exact content:

```markdown
# Using DING with Buildkite

> Buildkite is a hosted CI control plane that runs jobs on your own infrastructure. DING's `ding run` wraps a step, evaluates rules during the step, and fires alerts on exit — and Buildkite's `buildkite-agent annotate` API lets DING surface alerts directly back into the build UI.

## Prerequisites

- DING binary `>= v0.3.0` — see [install](../install.md)
- A Buildkite organization with at least one agent ([free trial available](https://buildkite.com/pricing))
- A notifier endpoint (Slack webhook URL, custom webhook, etc.)

## Minimal example

`.buildkite/pipeline.yml`:

```yaml
steps:
  - label: "Test with DING"
    command: |
      curl -sSL https://github.com/zuchka/ding/releases/latest/download/ding_linux_amd64.tar.gz | tar -xz
      ./ding run --config ding.yaml -- ./run-tests.sh
```

`ding.yaml`:

```yaml
notifiers:
  slack:
    type: slack
    url: $SLACK_WEBHOOK_URL

rules:
  - name: ci_step_failed
    match:
      metric: run.exit
    condition: value != 0
    mode: end-of-run
    message: "{{ .repo }}@{{ .branch }} failed (exit {{ .exit_code }}, {{ .duration_seconds }}s)"
    alert:
      - notifier: slack
```

Set `SLACK_WEBHOOK_URL` as an [environment hook](https://buildkite.com/docs/agent/v3/hooks) on your agent or as a pipeline-level environment variable.

## What you get

A Slack message when the step exits non-zero, automatically tagged with `repo`, `branch`, `commit` from Buildkite's environment.

## Configuration

`runctx` auto-detects Buildkite via the `BUILDKITE=true` environment variable and captures these labels:

| Label | Buildkite env var |
|---|---|
| `run_id` | `BUILDKITE_BUILD_ID` |
| `runner` | `"buildkite"` (set by runctx) |
| `repo` | `BUILDKITE_PIPELINE_SLUG` |
| `branch` | `BUILDKITE_BRANCH` |
| `commit` | `BUILDKITE_COMMIT` |

Use these in `match.labels` or `message` templates.

## Verification

1. Locally: `ding validate --config ding.yaml` — confirms the rule parses.
2. Trigger a build. Confirm a successful step produces no alert.
3. Force a failure (`exit 1` in `run-tests.sh`). Confirm the alert fires in Slack within ~5 seconds of step exit.

## Tradeoffs / known limitations

- **No `job` label by default.** runctx captures Buildkite's pipeline-level identifiers but not step-level (`BUILDKITE_STEP_KEY`). Add explicit `match.labels` if you need per-step rules.
- **Binary download per step.** Bake DING into your agent image, or use a [`pre-command` hook](https://buildkite.com/docs/agent/v3/hooks#available-hooks) to install it once per agent.
- **Annotation surface unused.** Buildkite has `buildkite-agent annotate`, the analogue of GHA's `$GITHUB_STEP_SUMMARY`. The minimal recipe doesn't use it; surfacing alerts back into the build UI would be a Tier-2 abstraction (`type: buildkite_annotate` notifier).

## Escalation criteria

This recipe is **a Tier-2 candidate** by the program's standard rubric:

- **Setup commands required:** 1 (`curl | tar`) — under threshold of 5
- **Boilerplate lines:** ~24 — under threshold of 50
- **"Gotcha" callouts:** 3 (no `job` label, binary download, no annotation surface) — over threshold of 2 → **Tier-2 candidate**
- **End-to-end runnable:** yes (Buildkite has a free trial; the underlying agent is OSS and self-hostable indefinitely)

**Tier-2 candidate.** The structural friction is "annotations not used" — Buildkite users expect alerts to land in the build UI, not just Slack. A `type: buildkite_annotate` notifier (calling `buildkite-agent annotate --style error --context ding`) is the natural Tier-2 abstraction. Sequence it after GitLab CI's artifact notifier (similar shape).
```

- [ ] **Step 2: Update index status**

In `docs/recipes/index.md`:

```markdown
| [Buildkite](buildkite.md) | CI/CD | shipped (Tier-2 candidate) |
```

- [ ] **Step 3: Verify mkdocs.yml still parses**

```bash
python3 -c "import yaml; yaml.safe_load(open('mkdocs.yml'))" && echo "OK"
```

Expected: `OK`

- [ ] **Step 4: Commit**

```bash
git add docs/recipes/buildkite.md docs/recipes/index.md
git commit -m "docs(recipes): Buildkite recipe (Wave 1, Tier-2 candidate)

Self-evaluated as Tier-2 candidate: missing buildkite-agent annotate
integration is structural. Tier-2 target is type:buildkite_annotate
notifier, similar shape to deferred GitLab CI artifact notifier.

Refs: docs/superpowers/specs/2026-04-26-platform-recipe-program-design.md"
```

---

## Task 6: Cross-links — README and configuration.md

**Files:**
- Modify: `README.md`
- Modify: `docs/configuration.md`

- [ ] **Step 1: Read current README to confirm anchor point**

```bash
grep -n "## " README.md | head -20
```

The expected anchor is just after the existing `## Notifiers` section (currently around line 214). If line numbers have shifted significantly, place the new section adjacent to Notifiers regardless — that's the highest-traffic section for users picking a configuration.

- [ ] **Step 2: Add recipes link to README**

Insert a new `## Recipes` section in `README.md` immediately after the `## Notifiers` section (and before `## Beyond CI — long-running mode`):

```markdown
## Recipes

Looking for a config that works on your specific platform? See **[docs/recipes/](docs/recipes/index.md)** for platform-specific guides:

- **CI/CD:** [GitHub Actions](https://github.com/zuchka/ding-action) · [GitLab CI](docs/recipes/gitlab-ci.md) · [CircleCI](docs/recipes/circleci.md) · [Jenkins](docs/recipes/jenkins.md) · [Buildkite](docs/recipes/buildkite.md)
- More platforms (K8s Jobs, MLflow, Ray, Argo Workflows, dbt, Modal, …) coming in subsequent waves.
```

- [ ] **Step 3: Cross-link from docs/configuration.md**

```bash
grep -n "## " docs/configuration.md | head -20
```

The current top-level section order is: Full example → `server` → `notifiers` → `rules` → `persistence` → `alert_log` → Duration format. Add the new `## Platform-specific examples` section as the **last** top-level section (after `## Duration format`):

```markdown
## Platform-specific examples

See [Recipes](recipes/index.md) for end-to-end configurations on specific CI/CD platforms (GitLab CI, CircleCI, Jenkins, Buildkite). Each recipe shows the auto-captured labels and the minimal `ding.yaml` for that platform.
```

(The reference docs above are field-by-field; the recipes are platform-by-platform — they complement each other and the platform-recipes section reads naturally as the closing material.)

- [ ] **Step 4: Verify mkdocs.yml still parses**

```bash
python3 -c "import yaml; yaml.safe_load(open('mkdocs.yml'))" && echo "OK"
```

Expected: `OK`

- [ ] **Step 5: Commit**

```bash
git add README.md docs/configuration.md
git commit -m "docs: cross-link platform recipes from README and configuration

Surfaces the new docs/recipes/ index from the project's two highest-traffic
docs pages. Wave 1 platforms linked individually; future waves will add
themselves to the recipes index."
```

---

## Task 7: Final verification

**Files:** none (read-only checks)

- [ ] **Step 1: Verify all files committed**

```bash
git status
```

Expected: `nothing to commit, working tree clean`

- [ ] **Step 2: Verify mkdocs.yml is valid and references all new files**

```bash
python3 -c "
import yaml
nav = yaml.safe_load(open('mkdocs.yml'))['nav']
def collect(items, out):
    for x in items:
        if isinstance(x, dict):
            for v in x.values():
                if isinstance(v, list):
                    collect(v, out)
                elif isinstance(v, str):
                    out.append(v)
files = []
collect(nav, files)
import os
missing = [f for f in files if not os.path.exists(f'docs/{f}')]
print('Missing:', missing or 'none')
"
```

Expected: `Missing: none`

- [ ] **Step 3: Verify recipe template integrity**

```bash
ls -la docs/recipes/
```

Expected output includes:
- `_template.md` (canonical template, NOT in nav)
- `index.md` (status table)
- `gitlab-ci.md`
- `circleci.md`
- `jenkins.md`
- `buildkite.md`

- [ ] **Step 4: Confirm Wave 1 status in index**

```bash
grep -E "(GitLab CI|CircleCI|Jenkins|Buildkite)" docs/recipes/index.md
```

All four lines should show `shipped` (with or without "Tier-2 candidate" suffix). None should still say `pending`.

- [ ] **Step 5: Final commit (if anything still untracked)**

```bash
git status && git log --oneline -10
```

Confirm 6 new commits since the spec commit (`fd3a490`):
1. plumbing
2. gitlab-ci
3. circleci
4. jenkins
5. buildkite
6. cross-links

---

## Out of scope for this plan

- **Wave 2 recipes** (K8s Jobs, MLflow, Ray, Argo Workflows). Separate plan.
- **Wave 3 recipes** (Modal, Airflow, Dagster, etc.). Separate plan(s) per milestone.
- **Tier-2 abstractions** (CircleCI orb, Jenkins plugin, Buildkite annotate notifier, GitLab artifact notifier). Each is its own future plan, gated on rubric promotion + 2+ user requests per spec §"Open Questions".
- **End-to-end CI runs.** Verifying each recipe by actually pushing a commit to a real GitLab/CircleCI/Jenkins/Buildkite project is valuable but not required for "plan complete." If the user has access to one or more of these platforms, end-to-end verification can happen as a follow-up.
- **Screenshots in recipes.** Text rendering of alerts is sufficient per the template. Screenshots can be added incrementally once recipes are live.

## Success criteria for this plan

The plan is complete when:

1. `docs/recipes/_template.md`, `docs/recipes/index.md`, and all four Wave 1 recipes exist on `main`.
2. `mkdocs.yml` is updated and parses as valid YAML.
3. README and `docs/configuration.md` have cross-links to recipes.
4. The index page accurately reflects each recipe's Tier status from its self-evaluation.
5. `git status` is clean; six commits exist since spec commit `fd3a490`.

The next plan (Wave 2) starts after these criteria are met.
