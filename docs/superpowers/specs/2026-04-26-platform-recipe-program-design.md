# Platform Recipe Program — Tier-1 Coverage of the Co-Mortal Map

## Context

DING shipped its v0.3.0 ephemeral-first wedge — single binary, `ding run -- <cmd>` subcommand, run-context auto-capture, end-of-run rules, several notifier channels. The natural next question is: **where else does DING ride along?** The original structural plan (`/Users/zuchka/.claude/plans/1-i-love-the-mighty-cocke.md` §3) reserved a slot for *"integrations live in separate repos — `ding-action`, `ding-mlflow`, `ding-ray`, `ding-k8s-job`."* Only `ding-action` has shipped.

A naive reading of "do the rest" is to start scaffolding `ding-mlflow`, `ding-ray`, `ding-k8s-job`, and ten more repos. That's a maintenance trap for a one-developer project: ~50 platforms × ongoing per-repo upkeep. The strategic plan's deferred-items list already gives us the better answer:

> *Document as recipes in `docs/` first; separate repos only when customer-dev confirms the persona.*

This spec operationalizes that instruction. It defines a **tiered integration program** and produces a **comprehensive platform map** as Tier-1 recipes — leaving the higher tiers conditional on what the recipes themselves expose.

The customer-dev validation gates from the wedge plan have been **explicitly waived** by user override (matching the precedent set on 2026-04-25 for the wedge MVP). The recipe-first approach is justified independently of validation: it minimizes per-platform cost, doubles every artifact as DevRel content, and lets the platforms self-select for higher-tier work via a written rubric.

## Scope

**In scope** for this spec:

- A complete map of platforms where DING fits the co-mortal frame, organized by category.
- A three-tier integration model with promotion criteria.
- A standard recipe template every Tier-1 page conforms to.
- A self-judging escalation rubric every recipe applies to itself.
- A sequenced rollout across three waves.
- File-organization plumbing (`docs/recipes/`, `mkdocs.yml`).

**Out of scope:**

- Building any Tier-2 integration repos (`ding-mlflow`, `ding-k8s-job`, etc.). Those happen only if a recipe self-promotes via the rubric.
- Tier-3 work (operators, CRDs, callback SDKs).
- Long-running platform integrations (developer environments, dashboards, IDE plugins) — these are off-wedge by definition.
- Notifier-channel additions. Slack/Teams/PagerDuty/Telegram are already in flight; new notifier work is a separate spec.

## The platform map

Filter applied: **ephemeral execution environments where a workload runs and DING can ride along inside its lifecycle.** Long-running services are explicitly survivor-territory and excluded.

### Category 1 — CI/CD runners
The native home for the wedge. Same shape as `ding-action`: wraps a step, captures output, fires on the runner's notification surface.

| Platform | Status |
|---|---|
| GitHub Actions | ✅ shipped (`ding-action`) |
| GitLab CI | `runctx` already detects env vars; needs artifact-writing notifier |
| CircleCI | `runctx` detects env vars; orb integration |
| Jenkins | `runctx` detects env vars; plugin or pipeline step |
| Buildkite | `runctx` detects env vars; `buildkite-agent annotate` analogue to step summary |
| Drone CI / Woodpecker | Plugin-shaped |
| Tekton | Task wrapping `ding run` |
| Argo Workflows | Workflow step; k8s-native |
| AWS CodeBuild / GCP Cloud Build | buildspec / step integration |
| Bitbucket Pipelines, TeamCity, Travis | Long tail |

### Category 2 — ML training & experiment tracking
The "underserved persona" bet from the wedge plan.

| Platform | Notes |
|---|---|
| MLflow | `ding-mlflow` named in plan; MLflow callback or wraps `mlflow run` |
| Ray Train / Tune | `ding-ray` named in plan; Ray callback during training |
| Weights & Biases | Wraps `wandb agent` runs; alerts on training divergence |
| Kubeflow Pipelines | Component step |
| SageMaker / Vertex AI | Managed training jobs (boxed runtime — harder) |
| Modal / RunPod / Replicate | Modern serverless GPU compute (ephemeral by design) |
| Determined AI, HF AutoTrain | Long tail |

### Category 3 — Kubernetes Jobs / CronJobs
Probably the strongest single pain point in the space — "silent cron failure" is a persona universal.

| Platform | Notes |
|---|---|
| Kubernetes Jobs | Sidecar or init-container pattern; `ding run` wraps the main container |
| Kubernetes CronJobs | Same pattern, recurring |
| Argo CronWorkflows | k8s-native cron |

### Category 4 — Cloud batch (managed K8s-Job equivalents)

| Platform | Notes |
|---|---|
| AWS Batch | Docker runtime, fits cleanly |
| GCP Batch | Equivalent |
| Azure Batch | Equivalent |
| AWS Step Functions | Task callbacks |
| Nomad batch / periodic jobs | HashiCorp's scheduler |

### Category 5 — Workflow orchestrators (ephemeral DAG runs)

| Platform | Notes |
|---|---|
| Airflow | Task callback hooks; the legacy giant |
| Dagster | Modern; ephemeral runs are first-class |
| Prefect | Modern; "flow run" is the unit |
| Kestra | Newer event-driven |
| Temporal | Activities are ephemeral (workflows often aren't) |

### Category 6 — Data pipelines (subset, distinct persona)

| Platform | Notes |
|---|---|
| dbt Cloud / dbt Core | `ding run -- dbt build` |
| Apache Spark batch | Driver wrapper |
| Apache Beam batch | — |
| Fivetran / Airbyte / Singer / Meltano | Sync runs |

### Category 7 — Deploys & GitOps

| Platform | Notes |
|---|---|
| ArgoCD sync hooks | k8s GitOps |
| Flux | Equivalent |
| Helm release lifecycle hooks | — |
| Terraform Cloud / Atlantis | IaC apply runs |
| Spinnaker | Canary releases as ephemeral runs |

### Category 8 — Game servers & realtime matches
The "match-as-job" angle from the wedge plan.

| Platform | Notes |
|---|---|
| Agones | k8s dedicated game servers |
| GameLift | AWS managed |
| PlayFab Multiplayer | Microsoft |
| Bare Unreal/Unity dedicated server | `ding run -- UnrealServer` |
| Open Match | Matchmaking |

### Category 9 — Serverless / per-invocation ephemeral
Edge of the wedge — granularity issues.

| Platform | Notes |
|---|---|
| AWS Lambda | Too granular for general use; fits batch-invocation patterns |
| GCP Cloud Run Jobs | Better fit than Cloud Run services |
| Cloudflare Workers Cron Triggers | — |
| Vercel / Netlify cron functions | — |
| Fly.io Machines | Start-stop containers |

### Category 10 — Test runners & load tests
Could be wrapped via `ding run` as either a runner or an alerting target.

| Platform | Notes |
|---|---|
| pytest / Jest / Go test | Wrap with `ding run` for flaky-test alerting |
| k6 / Locust | Load tests |
| Playwright / Cypress | e2e runs |
| Lighthouse CI | — |

### Excluded — long-running dev environments
Tilt / Skaffold dev loops, GitHub Codespaces, Coder / Daytona — long-running, off-wedge. Explicitly not covered by this program.

## Tier model

| Tier | Form | Cost / platform | Realistic count |
|---|---|---|---|
| **1 — Recipe** | A `docs/recipes/<platform>.md` page showing how to use the existing `ding` binary on that platform. Zero new code in DING core. | Writing time + verification run | All 10 categories, ~30+ pages |
| **2 — Thin integration** | Shell wrapper, plugin manifest, or small adapter, `ding-action`-shaped. Lives in its own repo. Built only when the recipe self-promotes via the rubric. | Small repo + ongoing version maintenance | 3–5 ever |
| **3 — Operator / SDK** | Real code: K8s operator, callback library, CRD. | Substantial; long-term commitment | 1–2 ever, if any |

The discipline this enforces: **most platforms get a docs page and stop there.** Engineering investment escalates only when the rubric proves it's needed.

## Recipe template

Every `docs/recipes/<platform>.md` page conforms to this structure:

```markdown
# Using DING with <Platform>

<2-sentence framing — what the platform is, why co-mortal observability fits>

## Prerequisites
- DING binary version (`>= v0.3.0`)
- Platform-specific requirements (account, runtime version, etc.)

## Minimal example
<5–15 line copy-paste config showing `ding run` + a representative rule>

## What you get
<Screenshot or text rendering of the alert as actually delivered>

## Configuration
<Key options for this platform — env-var capture handled by `runctx`,
notifier choice, any platform-specific gotchas>

## Verification
<How to confirm it's working: `ding validate` + a test command>

## Tradeoffs / known limitations
<Honest list of what the recipe doesn't solve>

## Escalation criteria
<Boilerplate rubric, evaluated per-recipe — see below>
```

The page should be readable on its own without prior DING knowledge — assume the reader arrived via SEO for "alerting in <platform>."

## Escalation rubric

Every recipe ends with the same self-evaluation. This is the **systematic test** the program runs against itself:

> **Promote to Tier 2 if any of these hold:**
> - Setup is **>5 commands** (i.e., the user has to run more than five things to get a working alert)
> - Boilerplate is **>50 lines** of platform-specific YAML/code copy-paste
> - The recipe needs **>2 "gotcha" callouts** to avoid common breakage
> - You **can't actually run it end-to-end** — e.g., requires a paid SaaS without a free tier, or a runtime the recipe author can't reproduce

A recipe that fails any of these flags itself in a "Tier 2 candidate" admonition at the bottom of the page. The full set of Tier-2 candidates becomes the input to a future `ding-<platform>` repo decision — no separate research pass needed.

The rubric is intentionally permissive. The point is to surface real friction, not to perfectionism-gate every recipe.

## Sequencing — three waves

### Wave 1 — Free wins (~1 hour each)

Platforms `runctx` already detects. The recipe is essentially "use `ding run`, here are the auto-detected labels." Confirmed via `internal/runctx/runctx.go` line 49 onwards:

- **GitLab CI**
- **CircleCI**
- **Jenkins**
- **Buildkite**

**Why first:** zero engineering risk, fastest path to a complete CI/CD coverage story, easiest to verify end-to-end.

### Wave 2 — Persona pain (~3–4 hours each)

Higher-substance recipes; likely Tier-2 promotion candidates after self-judging:

- **Kubernetes Jobs / CronJobs** (sidecar / init-container pattern)
- **Argo Workflows**
- **MLflow**
- **Ray (Train / Tune)**

**Why second:** these are the wedge plan's named integration repo slots (`ding-k8s-job`, `ding-mlflow`, `ding-ray`). The recipe pass tells us whether to actually build the repo.

### Wave 3 — Category creation / niche / long tail (~half-day each)

Each is blog-post-length and doubles as DevRel content. Subdivided into milestones to give the implementation plan natural checkpoints; within a milestone, ordering is at author discretion:

**3a. Modern ML / serverless GPU compute**
- **Modal**, **RunPod**, **Replicate**

**3b. Workflow orchestrators**
- **Airflow**, **Dagster**, **Prefect**, **Kestra**

**3c. Cloud batch & remaining CI/CD**
- **AWS Batch**, **GCP Batch**, **AWS Step Functions**
- **Tekton**, **Drone CI**, **Bitbucket Pipelines**, **AWS CodeBuild**, **GCP Cloud Build**

**3d. Data pipelines**
- **dbt Cloud / dbt Core**
- (Spark / Beam / Fivetran / Airbyte deferred to a 3e tail unless demand surfaces)

**3e. Game servers & realtime**
- **Agones**, bare Unreal/Unity dedicated servers

**3f. Serverless ephemeral / per-invocation**
- **GCP Cloud Run Jobs**, **Cloudflare Workers Cron Triggers**, **Fly.io Machines**

**3g. Test runners**
- pytest, Go test, k6, Playwright

**Total scope:** ~30 recipes across all three waves. At ~2–3 hours average, ~1–2 months of part-time DevRel content production. Each recipe is independently shippable; milestones exist for planning checkpoints, not gating.

## File organization

- **New directory:** `docs/recipes/` — flat layout, one markdown file per platform. File naming: lowercase, hyphenated, no category prefix (e.g., `gitlab-ci.md`, `kubernetes-jobs.md`, `mlflow.md`).
- **`mkdocs.yml`:** add a top-level `Recipes:` nav section between Examples and CLI Reference. Within the section, group by category (CI/CD, ML, Kubernetes, etc.) for navigation, even though files live flat on disk.
- **Cross-linking:**
  - `README.md` — link to `docs/recipes/` index from the existing notifier section.
  - `docs/configuration.md` — each notifier section gets a "see also" pointer to recipes that use it.
  - Each recipe links back to `docs/configuration.md` for notifier reference.
- **Index page:** `docs/recipes/index.md` — table of all recipes grouped by category, with a status column (`shipped`, `Tier-2 candidate`, `pending`).

## How "test systematically" works

The user's framing: *"go through and test each one systematically and see if we need to escalate to tier two or three."* Concrete process per recipe:

1. **Write the recipe** to template.
2. **Run it end-to-end** on the actual platform. If you can't (paid-only SaaS, unavailable runtime), the rubric's last criterion auto-flags it as Tier-2.
3. **Apply the rubric in writing** at the bottom of the page. Don't soften — the program's value comes from honest self-assessment.
4. **Mark in the index** as `shipped` or `Tier-2 candidate`.
5. **Commit.** Don't batch — each recipe is a standalone shippable unit.

Wave-level checkpoints: at the end of each wave, review the Tier-2-candidate list. If <30% of recipes flagged, the program is working; if >50%, the rubric may be too strict and worth tuning.

## Success criteria

This program is "successfully executed" when:

1. All 10 in-scope categories have at least one recipe published.
2. Wave 1 is shipped end-to-end (4 recipes).
3. Each shipped recipe has been verified with an actual end-to-end run (or is explicitly flagged as Tier-2 because end-to-end wasn't possible).
4. The index page accurately tracks Tier-2 candidates.

(Earlier draft included "≥2 recipes promoted to blog posts" as criterion 5; reviewer noted that depends on external editorial decisions. Demoted to a stretch goal — see Open Questions on cross-promotion.)

This spec is "executed correctly" — not the program itself — when:

- The directory structure exists, mkdocs nav updated.
- The recipe template is documented in a `CONTRIBUTING`-style file or `docs/recipes/_template.md`.
- Wave 1 ships first.
- The escalation rubric appears verbatim in every recipe's last section.

## Critical files

- **New:**
  - `docs/recipes/` (directory)
  - `docs/recipes/index.md` (status table)
  - `docs/recipes/_template.md` (canonical template)
  - One file per Wave 1 platform: `gitlab-ci.md`, `circleci.md`, `jenkins.md`, `buildkite.md`
- **Modified:**
  - `mkdocs.yml` — new `Recipes:` nav section
  - `README.md` — link to recipes index
  - `docs/configuration.md` — cross-links to recipes
- **Not modified:** `internal/` — zero core code changes. The whole program leans on existing `ding run` + `runctx` + notifier infrastructure.

## Open questions

Non-blocking; resolve during Wave-1 drafting:

- **Recipe author voice:** matching DING's existing docs voice (terse, technical) or a more tutorial-friendly voice for SEO traffic? Recommendation: matching existing voice for consistency; SEO concerns addressed via title/H1 wording, not body register.
- **Index status badges:** plain-text `shipped` / `Tier-2 candidate` / `pending`, or visual badge images? Recommendation: plain text — badges become broken-link liabilities.
- **Cross-promotion to blog:** every recipe automatically becomes a blog post draft, or curated subset? Recommendation: curated. Recipe and blog post serve different readers; not all recipes earn a 1500-word piece.
- **Tier-2 promotion authority:** can a single Tier-2 candidate trigger a repo, or does it need 2+ failing recipes to confirm a pattern? Recommendation: 2+ for confidence, with override for "obvious" cases (e.g., K8s Jobs needs a sidecar abstraction regardless).

## Verification (of this spec, not the program)

This spec is executed correctly when, before any recipe is written:

1. The recipe template (`docs/recipes/_template.md`) exists and matches §"Recipe template" above verbatim.
2. The directory structure exists with an empty `docs/recipes/index.md` placeholder.
3. `mkdocs.yml` is updated with the new nav section.
4. Wave 1's four recipes are scoped as explicit implementation tasks — but not yet written. Writing happens in the implementation plan that follows this spec.

The program is "executed correctly" when the success criteria above are all met across all three waves.
