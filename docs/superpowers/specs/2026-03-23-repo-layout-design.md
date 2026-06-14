---
name: Repo Layout Match (mja-dot-com structure)
description: Restructure ding-website to match the file layout of zuchka/mja-dot-com
type: project
---

# Repo Layout Match

## Goal

Update `ding-website` to match the filetree and file layout of `zuchka/mja-dot-com`, preserving the existing DING content.

## Target Structure

```
ding-website/
├── Staticfile          # root: site
├── railway.toml        # Railway deployment config
└── site/
    └── index.html      # existing DING content (moved from root)
```

## Changes Required

1. Create `site/index.html` — move existing root `index.html` content here (no content changes)
2. Delete root `index.html`
3. Create `Staticfile` at root with content: `root: site`
4. Create `railway.toml` at root with Railway deployment config

## `Staticfile` content

```
root: site
```

## `railway.toml` content

```toml
[build]
builder = "RAILPACK"

[deploy]
numReplicas = 1
sleepApplication = false
restartPolicyType = "ON_FAILURE"
restartPolicyMaxRetries = 10
```

## Out of Scope

- No changes to `index.html` content
- No changes to `.git`, `.claude`, or any other existing files
