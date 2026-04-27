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
