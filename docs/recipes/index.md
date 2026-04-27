# Recipes

Concrete configurations for using DING with specific platforms. Every recipe shows the minimal config needed to get an alert firing when a workload exits, then enumerates the auto-captured labels and any platform-specific tradeoffs.

Recipes follow a three-tier integration program:

- **Tier 1 — Recipe** (this directory): a docs page using the existing `ding` binary on a specific platform. Most platforms stop here.
- **Tier 2 — Thin integration**: a separate repo (e.g. [`ding-action`](https://github.com/zuchka/ding-action) for GitHub Actions). Built only when a recipe self-promotes via the escalation rubric printed at the bottom of each recipe.
- **Tier 3 — Operator / SDK**: real code (CRD, callback library). Reserved for the few platforms where Tier 2 isn't enough.

Recipes marked **Tier-2 candidate** in the table below have self-evaluated as exceeding the rubric thresholds; promotion to a real Tier-2 repo happens when 2+ users confirm the friction.

## Status

| Recipe | Category | Tier | Status |
|---|---|---|---|
| [GitHub Actions](https://github.com/zuchka/ding-action) | CI/CD | Tier 2 (separate repo) | shipped |
| [GitLab CI](gitlab-ci.md) | CI/CD | Tier 1 | shipped |
| [CircleCI](circleci.md) | CI/CD | Tier 1 | shipped (Tier-2 candidate) |
| [Jenkins](jenkins.md) | CI/CD | Tier 1 | shipped (Tier-2 candidate) |
| [Buildkite](buildkite.md) | CI/CD | Tier 1 | shipped (Tier-2 candidate) |

## Template

The canonical recipe shape lives in [`_template.md`](_template.md). All recipes conform to it.
