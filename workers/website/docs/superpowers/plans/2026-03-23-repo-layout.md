# Repo Layout Match Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure `ding-website` to match the filetree of `zuchka/mja-dot-com` — moving `index.html` into a `site/` subdirectory and adding `Staticfile` and `railway.toml` at the root.

**Architecture:** Move existing HTML to `site/index.html`, add a `Staticfile` telling the static host to serve from `site/`, and add a `railway.toml` for Railway deployment. No content changes to the HTML.

**Tech Stack:** Static HTML, Railway (RAILPACK builder), Staticfile convention.

---

### File Map

| Action | Path |
|--------|------|
| Create | `site/index.html` (content from current `index.html`) |
| Delete | `index.html` |
| Create | `Staticfile` |
| Create | `railway.toml` |

---

### Task 1: Move index.html into site/ subdirectory

**Files:**
- Create: `site/index.html`
- Delete: `index.html`

- [ ] **Step 1: Create site/ directory and move the file with git**

```bash
mkdir site
git mv index.html site/index.html
```

- [ ] **Step 2: Verify the move**

Run:
```bash
ls site/
git status
```
Expected output from `ls site/`: `index.html`
Expected `git status`: shows `renamed: index.html -> site/index.html`

---

### Task 2: Add Staticfile

**Files:**
- Create: `Staticfile`

- [ ] **Step 1: Create Staticfile**

```bash
printf 'root: site\n' > Staticfile
```

- [ ] **Step 2: Verify content**

```bash
cat Staticfile
```
Expected output: `root: site`

---

### Task 3: Add railway.toml

**Files:**
- Create: `railway.toml`

- [ ] **Step 1: Create railway.toml**

```bash
cat > railway.toml << 'EOF'
[build]
builder = "RAILPACK"

[deploy]
numReplicas = 1
sleepApplication = false
restartPolicyType = "ON_FAILURE"
restartPolicyMaxRetries = 10
EOF
```

- [ ] **Step 2: Verify content**

```bash
cat railway.toml
```
Expected: matches the content above exactly.

---

### Task 4: Verify final structure and commit

- [ ] **Step 1: Verify full file tree**

```bash
find . -not -path './.git/*' -not -path './.claude/*' -not -path './docs/*' -not -path './node_modules/*' | sort
```

Expected output (order may vary):
```
.
./Staticfile
./railway.toml
./site
./site/index.html
```

- [ ] **Step 2: Commit all changes**

```bash
git add Staticfile railway.toml site/index.html
git commit -m "$(cat <<'EOF'
Restructure repo layout to match mja-dot-com

Move index.html into site/ subdirectory, add Staticfile (root: site)
and railway.toml for Railway deployment.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 3: Verify clean working tree**

```bash
git status
```
Expected: `nothing to commit, working tree clean`
