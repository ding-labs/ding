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

A Slack message when the build exits non-zero, tagged with the Jenkins job name and build number. Successful builds produce no notification.

## Configuration

`runctx` auto-detects Jenkins via the presence of `JENKINS_URL` and captures these labels:

| Label | Jenkins env var |
|---|---|
| `run_id` | `BUILD_TAG` |
| `runner` | `"jenkins"` (set by runctx) |
| `job` | `JOB_NAME` |
| `build` | `BUILD_NUMBER` |

Note: Jenkins doesn't expose `repo`, `branch`, or `commit` as universal env vars (those depend on which SCM plugin is in use). To capture them, add explicit env vars in your Jenkinsfile from the SCM step's metadata. See [Configuration](../configuration.md) for the full notifier reference.

## Verification

1. Locally: `ding validate --config ding.yaml` — confirms the rule parses.
2. Trigger the job. Confirm a successful build produces no alert.
3. Force a failure (`exit 1` in `run-tests.sh`). Confirm the alert fires in Slack within ~5 seconds of build exit.

If the alert doesn't fire, check the Jenkins build console for `ding` output. Common issues: webhook credential not exposed to the job (`withCredentials` block missing or wrong `credentialsId`), or `drain_timeout` shorter than the notifier retry window — see [Configuration](../configuration.md).

## Tradeoffs / known limitations

- **No SCM-aware labels by default.** Unlike GitHub Actions / GitLab CI / CircleCI, Jenkins doesn't have a single `BRANCH` env var that works across all SCM plugins. You'll need to surface `GIT_BRANCH` / `GIT_COMMIT` (Git plugin) or equivalent yourself.
- **Binary download per job.** Cache DING in a Docker agent image, or as a [Tool Installation](https://www.jenkins.io/doc/book/managing/tools/) configuration on the controller.
- **No native plugin (yet).** A Jenkins plugin would expose alerts in the build console UI alongside DING's stdout. That's the most likely Tier-2 abstraction.

## Escalation criteria

This recipe is **a Tier-2 candidate** by the program's standard rubric:

- **Setup commands required:** 1 (`curl | tar`) — under threshold of 5
- **Boilerplate lines:** ~33 — under threshold of 50
- **"Gotcha" callouts:** 3 (no SCM-aware labels, binary download, no plugin) — over threshold of 2 → **Tier-2 candidate**
- **End-to-end runnable:** yes (Jenkins is free; self-hostable in 5 minutes via Docker)

**Tier-2 candidate.** The "no SCM-aware labels" friction is the structural problem — every Jenkins user wants `branch` and `commit` in their alerts, and the recipe leaves that as homework. A Jenkins plugin (`ding-jenkins-plugin`) that pulls SCM metadata into runctx labels would collapse this. Defer until 2+ users ask.
