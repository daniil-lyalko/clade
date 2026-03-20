# DevOps Engineering Review: Clade Hook & Automation Architecture

**Reviewer:** Marcus Chen, Staff DevOps Engineer
**Date:** 2026-02-06
**Scope:** Hook architecture, configuration model, automation opportunities, CI/CD integration, doctor diagnostics, and configuration evolution

---

## Executive Summary

Clade is a well-architected worktree management CLI with a clean separation of concerns: it manages worktrees and context, while Claude Code / Cursor handle the AI agent lifecycle. The hook system is functional but underutilizes the full lifecycle of modern AI coding tools. The biggest immediate wins are in environment bootstrapping automation (direnv/mise integration), expanding the doctor command into a comprehensive preflight system, and adding hooks for the full Claude Code lifecycle (12 events now exist, Clade only uses 1).

---

## 1. Hook Architecture

### Current State

Clade has **two distinct hook systems** that operate independently:

**Clade Lifecycle Hooks** (`/Users/dlyalko/daniil/clade/internal/hooks/hooks.go`):
- Three events: `on_create`, `on_resume`, `on_remove`
- Configured in YAML (`~/.config/clade/hooks.yaml` global, `.clade/hooks.yaml` per-repo)
- Executed via `sh -c` with 30-second timeout
- Environment variables: `CLADE_TYPE`, `CLADE_NAME`, `CLADE_PATH`, `CLADE_REPO_NAME`, `CLADE_REPO_PATH`, `CLADE_BRANCH`, `CLADE_TICKET`
- Global hooks run first, then per-repo hooks
- Failure in one hook does not block subsequent hooks
- Per-repo hooks use a TOFU (Trust On First Use) model with SHA-256 hash verification (`/Users/dlyalko/daniil/clade/internal/config/trust.go`)

**Agent Hooks** (`/Users/dlyalko/daniil/clade/internal/cmd/setup.go`, `/Users/dlyalko/daniil/clade/internal/cmd/inject.go`):
- Single event: Claude Code `SessionStart` / Cursor `sessionStart`
- Calls `clade inject-context` which gathers: DROPBAG, git status, recent commits, TODOs, ticket metadata
- Auto-detects output format: plain text for Claude Code (checks `CLAUDE_PROJECT_DIR` env var), JSON for Cursor
- 3-second dedup window prevents double injection when Cursor fires both `.cursor/hooks.json` and `.claude/settings.json`

### How They Should Interact

Currently, these two systems are completely decoupled -- which is mostly correct. But there is a gap: Clade lifecycle hooks (on_create, on_resume) run *before* the agent launches, and agent hooks (SessionStart) run *after* the agent launches. This means:

- If `on_create` fails (e.g., `npm install` errors), the agent still launches with no awareness of the failure
- There is no mechanism for `on_create` results to feed into the SessionStart context

**Recommendation:** Add an optional `on_create` / `on_resume` results file (e.g., `.clade/last-hook-results.json`) that `inject-context` reads and includes in the session context. This way Claude sees "WARNING: npm install failed with exit code 1" at session start rather than discovering it 10 minutes into the conversation.

### Missing Hooks

Based on the [full Claude Code hooks lifecycle](https://code.claude.com/docs/en/hooks) (12 events as of January 2026), Clade only leverages `SessionStart`. The following hooks would add real value:

| Hook | Use Case | Time Savings |
|------|----------|-------------|
| `Stop` | Auto-run `/drop` equivalent (write DROPBAG on session end) | Eliminates forgetting to `/drop` before closing |
| `PreToolUse` (Write) | Validate that modified files are inside the worktree path, not the source repo | Prevents accidental edits to the wrong directory |
| `PostToolUse` (Bash) | Auto-detect when `npm install`, `go mod tidy`, etc. runs and update `.clade/metadata.json` with dependency state | Context continuity |
| `SubagentStop` | For multi-agent workflows, track which sub-agents completed which tasks | Project coordination |
| `Setup` hooks (Jan 2026) | Run `clade inject-context` as a setup hook instead of SessionStart for earlier context availability | Faster initial context |

On the Clade lifecycle side:

| Missing Hook | When | Use Case |
|--------------|------|----------|
| `on_stale` | When `clade list` detects >7 day old worktrees | Auto-notification, auto-archive |
| `on_merge` | When cleanup detects branch is merged | Post-merge cleanup scripts, JIRA transition |
| `pre_create` | Before worktree creation | Validate preconditions (disk space, branch conflicts, required tools) |
| `on_switch` | When user `cd`s between worktrees | direnv-style environment activation |

### Security Model Assessment

The TOFU trust model in `/Users/dlyalko/daniil/clade/internal/config/trust.go` is solid:
- SHA-256 hash verification catches modified hooks
- Interactive prompt shows hook content before trust
- `CLADE_TRUST_REPO_HOOKS=1` bypass for CI environments
- Trust registry uses 0600 permissions

One gap: the global `~/.config/clade/hooks.yaml` has no trust verification at all. If someone modifies it (e.g., via a compromised dotfiles sync), those hooks run silently. Consider adding an integrity check for global hooks as well, or at least logging when they change.

---

## 2. Global vs Per-Repo vs Per-Worktree Configuration

### Current Precedence Model

```
~/.config/clade/hooks.yaml          (global lifecycle hooks)
~/.config/clade/config.json         (global config: agent, editor, repos)
{repo}/.clade/hooks.yaml            (per-repo lifecycle hooks)
{worktree}/.clade/metadata.json     (per-worktree metadata)
~/.claude/settings.json             (global agent hooks)
{repo}/.claude/settings.json        (per-repo agent hooks -- not managed by clade)
```

Global runs first, then per-repo. Per-worktree is metadata only (no hooks). This is a two-layer model.

### Comparison with Industry Standards

**Git config** uses a three-layer model: system -> global -> local (repo). Each layer can override or extend the previous. Git also supports `includeIf` for conditional config based on directory patterns.

**Cursor rules** (as of 2026) use `.cursor/rules/` with `.mdc` files, supporting project-level and user-level rules that are merged. The legacy `.cursorrules` is deprecated in favor of modular rules.

**.github/** uses a single repo-level config with no inheritance. Actions workflows are per-repo only.

**direnv** uses a strict per-directory model where `.envrc` files are loaded when you `cd` into a directory. Each directory's `.envrc` can optionally source its parent.

### Assessment

The current two-layer model (global + per-repo) is appropriate for Clade's scope. A per-worktree hook layer would add complexity without clear value -- worktrees are ephemeral, and their behavior should derive from the repo they came from.

**What IS missing is conditional configuration.** Consider a developer who works on both Node.js and Go repos. Their global `on_create` should not run `npm install` for Go repos or `go mod download` for Node repos. Options:

1. **Matcher patterns** (like Claude Code's hooks): `"matcher": "*.json"` but for repo names or paths
2. **Per-repo config in the global file** (like git's `includeIf`):
   ```yaml
   hooks:
     on_create:
       - match: "*/node-*"
         commands:
           - npm install
       - match: "*/go-*"
         commands:
           - go mod download
   ```
3. **Just use per-repo hooks** -- this already works, but requires `.clade/hooks.yaml` in every repo

Option 3 is the pragmatic choice today. Option 1 would be the right evolution.

---

## 3. Automation Opportunities

These are ranked by real time savings per invocation multiplied by frequency.

### High Impact (saves 30+ seconds per worktree creation)

**1. Runtime version management (nvm/mise/asdf):**
The current `copy_files` list in config already copies `.nvmrc`, `.tool-versions`, `.python-version`, `.ruby-version`, `.node-version`, `.go-version`. But copying the file does not activate the runtime. Add built-in support for:
```yaml
hooks:
  on_create:
    - mise install 2>/dev/null || true
    - nvm use 2>/dev/null || true
```
Or better: detect which version manager files exist and auto-run the right activation. The [git-prole](https://github.com/9999years/git-prole) worktree manager already does `direnv allow` on worktree creation -- Clade should match this.

**2. direnv integration:**
`direnv allow` is the single most impactful automation. Without it, every new worktree requires manual `direnv allow` because the `.envrc` path changes. This is already in the hooks.yaml.example template but should be a built-in behavior when `.envrc` is detected.

**3. Dependency installation:**
Detect lockfile type and auto-install:
- `package-lock.json` -> `npm ci --prefer-offline`
- `yarn.lock` -> `yarn install --frozen-lockfile`
- `pnpm-lock.yaml` -> `pnpm install --frozen-lockfile`
- `go.sum` -> `go mod download`
- `Gemfile.lock` -> `bundle install`
- `requirements.txt` -> `pip install -r requirements.txt`

Using `--prefer-offline` / `--frozen-lockfile` flags means this is fast (local cache) and deterministic.

### Medium Impact (saves 10-30 seconds)

**4. docker-compose bootstrapping:**
If `docker-compose.yml` exists in the repo root, optionally run `docker compose up -d` on `on_create`. This is repo-specific and should be a per-repo hook, not a global default.

**5. Pre-flight checks:**
Before creating a worktree, verify:
- Enough disk space (worktrees with node_modules can be 500MB+)
- No conflicting branch name
- Source repo is on expected default branch
- No uncommitted changes in source repo that would be lost

**6. Git configuration inheritance:**
Copy repo-specific git config (like `.gitattributes`, merge strategies) to the worktree. Currently only `.env*` and tool version files are copied.

### Lower Impact (quality of life)

**7. IDE workspace generation:**
When creating a worktree with `--open cursor`, generate a `.code-workspace` file that includes the worktree path and any related directories.

**8. Notification on long-running hooks:**
The 30-second timeout in `runSingleHook` (`/Users/dlyalko/daniil/clade/internal/hooks/hooks.go`, line 158) is good, but there is no progress feedback. A simple "Running on_create hooks..." with elapsed time would improve UX for hooks like `npm install` that take 5-15 seconds.

---

## 4. CI/CD Integration

### Parallel Test Runs Across Worktrees

This is where Clade has an unexploited architectural advantage. Each worktree is an isolated copy of the repo -- they share the `.git` directory via git's worktree mechanism, but the working directory is independent. This means:

**Opportunity 1: Parallel test matrix across worktrees**
```bash
# Create worktrees for parallel testing
clade test-unit -t spike --no-agent
clade test-integration -t spike --no-agent
clade test-e2e -t spike --no-agent

# Run tests in parallel
parallel --jobs 3 ::: \
  "cd ~/clade/repos/my-api/test-unit && npm run test:unit" \
  "cd ~/clade/repos/my-api/test-integration && npm run test:integration" \
  "cd ~/clade/repos/my-api/test-e2e && npm run test:e2e"
```

A `clade test` subcommand that creates ephemeral worktrees, runs tests in parallel, and cleans up would be a genuine productivity multiplier. This is similar to what [Earthly](https://earthly.dev/) and [Nx](https://nx.dev/) do for monorepos.

**Opportunity 2: Branch-per-PR workflow**
Clade already creates branches with standard prefixes (`feat/`, `fix/`, `spike/`). Integrating with `gh pr create` in the cleanup flow would close the loop:

```bash
clade cleanup PROJ-123 --pr
# Creates PR, assigns reviewers based on CODEOWNERS, links JIRA ticket
```

**Opportunity 3: CI feedback in context injection**
The `inject-context` command (`/Users/dlyalko/daniil/clade/internal/cmd/inject.go`) currently gathers git status, commits, TODOs, and DROPBAGs. Adding CI status for the current branch would give the AI immediate visibility into build state:

```
## CI Status
Branch: feat/PROJ-123
Pipeline: PASSED (2m ago)
Coverage: 87.3% (+0.5%)
```

This requires a network call, which violates the "no network on hot path" design principle. But it could be an opt-in feature, and the result could be cached in `.clade/ci-status.json` by a background hook.

---

## 5. The Doctor Opportunity

### Current Doctor Checks

The existing `clade doctor` (`/Users/dlyalko/daniil/clade/internal/cmd/doctor.go`) checks:

1. Config file validity
2. State file validity
3. State file location (legacy migration)
4. Base directory existence and writability
5. Git availability
6. Agent availability
7. Registered repo paths and git status
8. Trust registry
9. Global hooks (Claude + Cursor)
10. Orphaned worktrees (state entry but no directory)
11. Untracked worktrees (directory exists but not in state)
12. Prunable git worktrees

This is a strong foundation. The `--fix` flag for auto-remediation and `--json` for machine-readable output are excellent design choices.

### What a Comprehensive Dev Environment Health Check Should Include

**Tier 1: Environment Fundamentals**
- [ ] Git version >= 2.20 (required for worktree improvements)
- [ ] Shell (zsh/bash) with required features
- [ ] Disk space available in base_dir (warn if <1GB)
- [ ] File descriptor limits (ulimit -n, relevant for large repos)
- [ ] Agent version check (`claude --version`, not just existence)

**Tier 2: Tool Chain Validation**
- [ ] direnv installed and shell hook configured
- [ ] mise/nvm/asdf installed (if `.tool-versions`/`.nvmrc` found in any registered repo)
- [ ] Docker daemon running (if any registered repo has `docker-compose.yml`)
- [ ] GitHub CLI (`gh`) installed and authenticated
- [ ] JIRA MCP configured (if any worktrees reference tickets)

**Tier 3: Configuration Consistency**
- [ ] All registered repos have `clade init` run (`.clade/` exists)
- [ ] Global hooks match between Claude and Cursor (currently both are checked independently)
- [ ] `.gitignore` in all repos includes `.clade/`
- [ ] No duplicate branch names across repos
- [ ] Worktree branches exist in remote (detect force-deleted remote branches)

**Tier 4: Performance**
- [ ] Warn if >10 active worktrees (context switching overhead)
- [ ] Warn if total worktree disk usage >5GB
- [ ] Check git gc status for repos with many worktrees
- [ ] Measure `inject-context` execution time (should be <500ms)

**Tier 5: Security**
- [ ] Config file permissions are 0600 (already enforced on write, but check existing)
- [ ] Trust registry not world-readable
- [ ] No secrets in `.clade/hooks.yaml` (scan for common patterns)
- [ ] `--dangerously-skip-permissions` not in agent_flags (warn if present)

The `--fix` pattern already established is the right model. Each new check should include a `fixFunc` where auto-remediation is possible.

### Doctor as a Pre-Flight System

Beyond diagnostics, `clade doctor` could serve as a pre-flight check that runs automatically before `clade work` (opt-in). A lightweight subset (checks 1-3 from Tier 1) running in <200ms would catch common issues before they waste time:

```bash
clade foo -t feature
# Pre-flight: disk space OK, git OK, agent OK
# Creating worktree...
```

---

## 6. Configuration Model

### Current State

- **Format:** JSON (`config.json`, `state.json`, `metadata.json`, `trusted-repos.json`)
- **Lifecycle hooks:** YAML (`hooks.yaml`)
- **Agent hooks:** JSON (`.claude/settings.json`, `.cursor/hooks.json`)

This mixed format (JSON for data, YAML for hooks) is pragmatic. YAML supports comments, which is important for hook configuration that users edit manually. JSON is better for machine-generated config.

### How Config Should Evolve

**Keep JSON for machine-managed files.** `state.json`, `metadata.json`, `trusted-repos.json` should stay JSON. Humans rarely edit these.

**Keep YAML for human-edited hooks.** Comments in hooks.yaml are essential for documentation.

**Consider YAML for `config.json`.** The config file is human-edited and would benefit from comments:
```yaml
# Clade configuration
base_dir: ~/clade
agent: claude
editor: cursor
auto_init: true

# Registered repositories
repos:
  my-api: ~/repos/my-api
  my-frontend: ~/repos/my-frontend

# Custom labels beyond built-in (feature, bug, spike, chore, hotfix, docs)
custom_labels:
  perf:
    branch_prefix: perf
    merge_expected: true
```

However, this is a breaking change and the current JSON is fine. It is not worth the migration cost unless other features require YAML.

**What IS worth adding:**

1. **Config inheritance / includes:**
   ```json
   {
     "extends": "~/.config/clade/base-config.json",
     "repos": { ... }
   }
   ```
   Useful for teams sharing a base configuration.

2. **Repo-scoped hook config in the global config file:**
   Currently `repo_settings` only supports `copy_files`. Extending it to include `hooks` would let power users centralize all configuration:
   ```json
   {
     "repo_settings": {
       "~/repos/my-api": {
         "copy_files": ["config/local.json"],
         "on_create": ["npm ci --prefer-offline"],
         "on_resume": ["direnv allow"]
       }
     }
   }
   ```

3. **Environment-aware defaults:**
   Detect `CI=true` and disable interactive prompts, agent launching, and editor opening. The `CLADE_TRUST_REPO_HOOKS=1` bypass in `trust.go` is a good start; extend this pattern to all interactive features.

4. **Templates for hooks:**
   Instead of just `hooks.yaml.example`, provide templates for common stacks:
   ```bash
   clade init --template node
   clade init --template go
   clade init --template python
   ```
   Each template pre-populates `.clade/hooks.yaml` with stack-appropriate automation.

---

## Summary of Recommendations (Priority Order)

| Priority | Recommendation | Effort | Impact |
|----------|---------------|--------|--------|
| P0 | Surface `on_create` hook failures in `inject-context` output | Small | Prevents wasted AI sessions |
| P0 | Add `Stop` hook integration to auto-save DROPBAG | Medium | Eliminates "forgot to /drop" entirely |
| P1 | Built-in direnv allow + runtime version activation on create | Small | Saves 30s per worktree |
| P1 | Auto-detect lockfile and run dependency install | Medium | Saves 30-60s per worktree |
| P1 | Expand doctor with disk space, tool chain, and permissions checks | Medium | Catches issues before they waste time |
| P2 | Add `PreToolUse` hook to validate writes are inside worktree | Small | Prevents accidental source repo edits |
| P2 | CI status in `inject-context` (opt-in, cached) | Medium | Immediate build awareness |
| P2 | Stack templates for `clade init` (node, go, python) | Small | Better onboarding |
| P3 | Parallel test runner across worktrees | Large | Genuine CI/CD innovation |
| P3 | `clade cleanup --pr` integration with `gh` CLI | Medium | Closes the branch lifecycle loop |
| P3 | Config inheritance / extends | Medium | Team standardization |

---

## Sources

- [Claude Code Hooks Reference](https://code.claude.com/docs/en/hooks)
- [Claude Code Hooks Guide](https://code.claude.com/docs/en/hooks-guide)
- [Claude Code Hooks: Complete Guide to All 12 Lifecycle Events](https://claudefa.st/blog/tools/hooks/hooks-guide)
- [Claude Code Setup Hooks (Jan 2026)](https://claudefa.st/blog/tools/hooks/claude-code-setup-hooks)
- [The 2 Minute Inner Loop: Revolutionizing Local Development in 2026](https://dev.to/meena_nukala/the-2-minute-inner-loop-revolutionizing-local-development-in-2026-570f)
- [Lefthook vs Husky: Which Git Hooks Tool is Better? 2026](https://www.edopedia.com/blog/lefthook-vs-husky/)
- [Lefthook - Fast and powerful Git hooks manager](https://github.com/evilmartians/lefthook)
- [Development Containers Specification](https://containers.dev/overview.html)
- [How to Create Dev Containers for Development Environments](https://oneuptime.com/blog/post/2026-01-27-dev-containers-development/view)
- [Cursor Rules Documentation](https://cursor.com/docs/context/rules)
- [Guide to Cursor Rules - Engineering Context, Speed, and the Token Tax (Jan 2026)](https://medium.com/@peakvance/guide-to-cursor-rules-engineering-context-speed-and-the-token-tax-16c0560a686a)
- [6 Software Development and DevOps Trends Shaping 2026](https://dzone.com/articles/software-devops-trends-shaping-2026)
- [git-prole: A git-worktree(1) manager with direnv integration](https://github.com/9999years/git-prole)
- [direnv and mise-en-place integration](https://mise.jdx.dev/direnv.html)
- [Four Methods to Automate Development Environment Setup](https://dzone.com/articles/4-methods-automate-development)
- [The Three Developer Loops: A New Framework for AI-Assisted Coding](https://itrevolution.com/articles/the-three-developer-loops-a-new-framework-for-ai-assisted-coding/)
