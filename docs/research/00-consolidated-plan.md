# Pacer Improvement Plan — Consolidated from Research Team

**Date:** 2026-02-06
**Branch:** `feat/global-setup`
**Sources:** 4 research reports from Vasquez (AI ecosystem), Tanaka (CLI design), Chen (DevOps/hooks), Sharma (UX/onboarding)

---

## Strategic North Star

> Pacer's moat is **context orchestration** — the DROPBAG/inject-context/SessionStart chain. Worktree management alone is commoditized (Worktrunk, gwq, gtr). Nobody else builds session-aware, cross-worktree, multi-agent context orchestration. **Deepen the moat first, then widen.**
>
> — Cross-referenced from all 4 reports

---

## Phase 0: Done (this branch)

| Item | Status | Report |
|------|--------|--------|
| `pacer setup` — global hooks installer | Done | — |
| Preview → Confirm → Apply flow for setup | Done | Sharma P1 |
| Dashboard prompts setup when hooks missing | Done | Sharma P0 |
| Clade → Pacer migration in hook files | Done | — |

---

## Phase 1: Quick Wins (next sprint)

High impact, low effort. These are the P0s that all reviewers flagged independently.

### 1.1 Remove branch name confirmation prompt
**Reports:** Tanaka (Unix violation #1), Sharma (P0)
**File:** `internal/cmd/worktree.go:76-87`
**Change:** Auto-accept the branch name when it can be derived from name + type. Only prompt when `--branch` is explicitly set or name is ambiguous. This is the #1 friction point on the critical path.

### 1.2 First-run wizard: auto-detect + register repo
**Reports:** Sharma (P0), Tanaka (Unix violation #2)
**File:** `internal/config/config.go:169-218`
**Change:** After the AI tool question, check if the user is in a git repo (`detectCurrentRepo` already exists in root.go). If yes, offer to register it. This eliminates the "fail → repo add → retry" loop.

### 1.3 Auto-DROPBAG via `Stop` hook + `PreCompact` notification
**Reports:** Vasquez (immediate), Chen (P0), Sharma (UX), Tanaka (CLI), Anthropic engineering research
**Decision doc:** [05-decision-auto-dropbag.md](05-decision-auto-dropbag.md) (ADR-001)
**Files:** New `internal/cmd/auto_dropbag.go`, `internal/cmd/context_warning.go`, updates to `setup.go`
**Change:** Three-hook architecture:
- `Stop` (async) → `pacer auto-dropbag` writes DROPBAG.md (debounced: 60s + meaningful changes only)
- `PreCompact` → `pacer context-warning` prints calm notification to stderr suggesting `/drop` or fresh restart
- `SessionStart` → existing `pacer inject-context` reads DROPBAG + state (labels auto vs manual source)
- Manual `/drop` remains the gold standard for human-curated checkpoints
**Rationale:** PreCompact cannot inject into post-compaction context (no mechanism exists, known bugs). Stop is the only reliable persistence point — most sessions end before compaction fires. Anthropic's own `claude-progress.txt` pattern validates this approach.

### 1.4 Surface hook failures in inject-context
**Reports:** Chen (P0)
**File:** `internal/cmd/inject.go`
**Change:** Check for `.pacer/last-hook-results.json` (written by lifecycle hooks) and include failures in SessionStart output. Claude sees "WARNING: npm install failed" at session start instead of discovering it mid-session.

### 1.5 Fix `-v` flag convention
**Reports:** Tanaka (flag design)
**File:** `internal/cmd/root.go:66-69`
**Change:** Swap: `-v` → verbose (matches every Unix tool), `--version` → long-form only. Current `-v` = version breaks muscle memory.

### 1.6 Kill "experiment" from user-facing text
**Reports:** Sharma (P1)
**Files:** `internal/cmd/root.go` (dashboard), `internal/cmd/resume.go` (picker)
**Change:** Replace "Legacy experiments" with "Legacy worktrees" or just show them as regular worktrees with a `(v1)` tag. The word "experiment" confuses new users.

### 1.7 Add `pacer path <name>` command
**Reports:** Tanaka (composability)
**Change:** Print worktree path to stdout and exit. Enables `cd $(pacer path foo)` and `code $(pacer path foo)`. Currently requires parsing `pacer list --json | jq`.

---

## Phase 2: Moat Deepening (next month)

Medium effort. These strengthen context management and improve the CLI for power users.

### 2.1 `--json` on all commands
**Reports:** Tanaka (Unix violation #3)
**Files:** `status.go`, `resume.go`, `cleanup.go`, `doctor.go`, `work.go`
**Change:** Every command that produces output gets `--json`. Human output to stderr, structured output to stdout. Table stakes for composability.

### 2.2 `--quiet` mode
**Reports:** Tanaka (Unix violation #4)
**Change:** Global `-q` / `--quiet` flag. Zero output on success, stderr only on error. Essential for hooks and CI.

### 2.3 direnv + runtime version activation
**Reports:** Chen (P1)
**File:** Hooks system / worktree.go
**Change:** Auto-detect `.envrc` → run `direnv allow`. Auto-detect `.nvmrc`/`.tool-versions` → run activation. Saves 30+ seconds per worktree creation.

### 2.4 Convert `/drop` to a proper skill
**Reports:** Vasquez (short-term)
**File:** `.claude/commands/drop.md` → `.claude/skills/pacer-drop/SKILL.md`
**Change:** Add YAML frontmatter with description for auto-detection. Claude can auto-invoke the drop skill when it detects session-end context.

### 2.5 Expand doctor checks
**Reports:** Chen (P1), Sharma
**File:** `internal/cmd/doctor.go`
**New checks:** Git version >= 2.20, disk space in base_dir, agent version (not just existence), direnv/mise installed, `.gitignore` includes `.pacer/`, inject-context execution time.

### 2.6 `PreToolUse` write validation hook
**Reports:** Chen (P2)
**Change:** Register a `PreToolUse` hook (matcher: `Write`, `Edit`) that validates the target file is inside the worktree path, not the source repo. Prevents accidental edits to wrong directory.

### 2.7 Context-sensitive dashboard
**Reports:** Sharma (P1)
**File:** `internal/cmd/root.go`
**Change:** If 0 repos and 0 worktrees, show simplified onboarding flow (2 options, not 9). Progressive disclosure — show complexity as the user grows.

### 2.8 `SessionEnd` hook for project memory
**Reports:** Vasquez (short-term)
**Change:** On session end, update `~/pacer/repos/{repo}/memory.md` with key decisions. Accumulates knowledge across worktrees. Addresses the "spike knowledge is lost by day 2 of feature" problem.

### 2.9 Cross-worktree context import
**Reports:** Vasquez (medium-term)
**Change:** `pacer context import try-redis` — inject another worktree's DROPBAG, key commits, and changed files into the current session. Bridges the spike-to-feature gap.

### 2.10 Stack templates for `pacer init`
**Reports:** Chen (P2)
**Change:** `pacer init --template node` / `go` / `python`. Pre-populates `.pacer/hooks.yaml` with stack-appropriate automation (npm ci, go mod download, etc.).

---

## Phase 3: Vision (next quarter)

High effort, transformative impact. These are the big bets.

### 3.1 Bubbletea TUI dashboard
**Reports:** Sharma (P2), Tanaka
**Change:** Replace `promptui` with Bubbletea for the dashboard only. Multi-column layout, single-key shortcuts, inline context preview, real-time updates. The "k9s for AI coding" vision. Incremental — all CLI commands keep working.

### 3.2 `pacer team` — agent teams + worktrees
**Reports:** Vasquez (medium-term, transformative)
**Change:** `pacer team api-migration --repos my-api,my-frontend` creates matching worktrees across repos, spawns an agent team with one teammate per worktree, injects per-worktree context. Bridges Pacer's physical isolation (worktrees) with Claude's logical coordination (tasks, messaging).

### 3.3 Contextual memory graph
**Reports:** Vasquez (bold bet #2)
**Change:** Replace flat DROPBAGs with structured knowledge graph per repo. Nodes = decisions, learnings, patterns. Updated via `Stop` hook. Queried by relevance at `SessionStart`. Start simple (tagged JSON entries), graduate to embeddings later.

### 3.4 Plugin distribution
**Reports:** Vasquez (immediate strategic move)
**Change:** Package Pacer's hook integration as a Claude Code plugin. `/plugin install pacer` instead of `pacer init` + `pacer setup`. Hooks, skills, context injection in one installable unit.

### 3.5 Parallel git operations
**Reports:** Tanaka (performance)
**File:** `internal/git/`, `internal/cmd/root.go`
**Change:** Parallelize git status checks using goroutines. Cache git status with 30s TTL. Make fetch lazy/background. Advisory file locking on state.json. Critical at scale (50+ repos, 200+ worktrees).

### 3.6 Shell integration wrapper
**Reports:** Tanaka (composability)
**Change:** Ship a shell function via `pacer shell-init` that wraps `pacer resume` to also `cd` the parent shell into the worktree. Add `pacer exec <name> -- <cmd>` for running commands inside worktrees without cd.

### 3.7 `pacer cleanup --pr`
**Reports:** Chen (P3)
**Change:** Integrate with `gh pr create` in cleanup flow. Creates PR, links JIRA ticket, assigns reviewers. Closes the branch lifecycle loop.

### 3.8 Autonomous worktree swarms
**Reports:** Vasquez (bold bet #1)
**Change:** `pacer swarm PROJ-123 --plan` — analyzes a JIRA ticket, decomposes into subtasks, creates worktrees, launches agent team, monitors progress. Depends on Phase 3.2 (team command) and agent team maturity.

---

## Bug Fixes (from code review)

| Bug | Report | File | Priority |
|-----|--------|------|----------|
| `GetRecentCommits` uses `string(rune('0'+count))` — fails for count >= 10 | Tanaka | `internal/git/status.go:75` | P1 |
| Typo guard threshold of 3 may be too generous for short commands | Tanaka | `internal/cmd/root.go:115` | P2 |
| `CreateWorktreeNew` fetches synchronously — blocks on slow connections | Tanaka | `internal/git/branch.go:136` | P2 |
| `cleanup` doesn't check for `git worktree lock` before removing | Tanaka | cleanup.go | P2 |
| Global hooks.yaml has no integrity check (unlike per-repo TOFU) | Chen | hooks system | P3 |
| Documentation mismatch: README says `pacer init` creates `.claude/settings.json` | Sharma | README.md | P1 |

---

## Cross-Reference Matrix

Shows which reports independently identified the same issue. Items flagged by 3+ reports are highest confidence.

| Recommendation | Vasquez | Tanaka | Chen | Sharma | Count |
|----------------|---------|--------|------|--------|-------|
| `Stop` hook for auto-DROPBAG | X | | X | | 2 |
| Remove branch prompt friction | | X | | X | 2 |
| `--json` on all commands | | X | | | 1 |
| First-run repo detection | | | | X | 1 |
| Context is the moat (strategy) | X | X | | X | 3 |
| Bubbletea TUI | | X | | X | 2 |
| direnv integration | | | X | | 1 |
| Plugin distribution | X | | | | 1 |
| Agent teams + worktrees | X | | X | X | 3 |
| Expand doctor | | | X | X | 2 |
| Progressive disclosure | | | | X | 1 |
| Parallel git ops (perf) | | X | | | 1 |
| `pacer path` command | | X | | | 1 |
| Cross-worktree context | X | | | | 1 |
| Memory graph | X | | | X | 2 |
| Kill "experiment" naming | | | | X | 1 |

---

## Decision Points

### Resolved

1. **Auto-DROPBAG hook architecture** — Decided: Stop (async) for writes, PreCompact for notification only. See [ADR-001](05-decision-auto-dropbag.md).

### Open (require your input)

2. **`-v` swap**: Changing `-v` from version to verbose is a breaking change. Do it now while the user base is small, or deprecate gradually?

3. **"Workspace" vs "Worktree"**: Sharma suggests user-facing copy should say "workspace" instead of "worktree." The underlying git concept is worktree, but the user experience is "an isolated workspace." Worth the rename?

4. **`project` command**: Currently `--experimental`. Either commit to multi-repo support (which feeds into `pacer team`) or remove it to reduce concept count. Tanaka and Sharma both flagged concept bloat.

5. **Bubbletea migration**: Large effort but high impact. Worth starting in Phase 2 (dashboard only) or defer to Phase 3?

6. **Plugin distribution**: Requires understanding Claude Code's plugin packaging format. Investigate now or after Phase 2 stabilizes?

---

## Reading Order

If reviewing the full reports:

1. **[05-decision-auto-dropbag.md](05-decision-auto-dropbag.md)** — The first architectural decision. Stop vs PreCompact debate with full rationale.
2. **[04-sharma-ux-onboarding.md](04-sharma-ux-onboarding.md)** — First 5 minutes audit reveals the most actionable fixes.
3. **[02-tanaka-cli-design.md](02-tanaka-cli-design.md)** — Unix philosophy violations and composability gaps. The technical backbone.
4. **[03-chen-devops-hooks.md](03-chen-devops-hooks.md)** — Hook architecture, automation, doctor expansion. The operational view.
5. **[01-vasquez-ai-ecosystem.md](01-vasquez-ai-ecosystem.md)** — Strategic vision and bold bets. The 90-day outlook.
