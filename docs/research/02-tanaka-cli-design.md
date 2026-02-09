# Clade Technical Review

**Reviewer:** Koji Tanaka
**Date:** 2026-02-06
**Subject:** Clade v0.4 -- CLI design, git worktree usage, competitive landscape, architectural assessment
**Classification:** Honest, opinionated review. No punches pulled.

---

## 1. CLI Design Audit

### What Works

The command surface is compact and memorable. The root shortcut (`clade foo` = `clade work foo`) is the right call. It follows the git mental model where the most common operation is the shortest path. The command taxonomy is clean:

```
clade <name>           # create (primary action)
clade list             # query
clade resume <name>    # navigate
clade cleanup <name>   # destroy
clade repo add/list/remove  # manage
clade init / doctor / status  # meta
```

This is close to the `noun verb` or `verb noun` patterns that feel natural. The `-t/--type` flag for branch prefix is a clean unification of what was previously 6 separate commands (`exp`, `feat`, `bug`, etc.). The evolution from v0.1 to v0.4 shows good instinct -- the API surface contracted, not expanded.

Shell completion support via Cobra's `ValidArgsFunction` is implemented in `/Users/dlyalko/daniil/pacer/internal/cmd/resume.go` (line 646) and `/Users/dlyalko/daniil/pacer/internal/cmd/cleanup.go` (line 569). This is essential infrastructure that many tools forget.

The `--dry-run` flag on both `work` and `cleanup` commands follows a well-established Unix convention. Good.

Branch name validation in `/Users/dlyalko/daniil/pacer/internal/git/branch.go` (lines 17-46) rejects shell metacharacters, path traversal, and control characters. This is security-conscious for a tool that constructs `exec.Command` calls.

### Unix Philosophy Violations

**Violation 1: Interactive prompts on the critical path.** The biggest problem. In `/Users/dlyalko/daniil/pacer/internal/cmd/worktree.go` (lines 76-87), creating a worktree prompts for branch name confirmation -- even when the default is obvious. When I type `clade foo -t spike`, I should get `spike/foo` without being asked. The prompt should only fire when the name is ambiguous or when no name is provided. This breaks piping. This breaks scripting. This breaks automation.

The design philosophy stated in ARCHITECTURE.md says "Speed First -- `clade exp` should take <2 seconds." Interactive prompts are the enemy of speed.

**Violation 2: The dashboard-as-default.** When you type `clade` with no arguments, it shows an interactive dashboard with a `promptui.Select` picker (see `/Users/dlyalko/daniil/pacer/internal/cmd/root.go`, lines 163-179). This is hostile to scripting. It means `clade` cannot be called from a script at all -- it blocks on stdin. A bare `clade` should print a short usage/status summary to stdout and exit 0, not enter an interactive mode. Interactive mode should be opt-in, e.g., `clade --interactive` or `clade tui`.

Compare: `git` with no args prints usage. `docker` with no args prints usage. `kubectl` with no args prints usage. None of them enter an interactive menu. There is precedent for this in `lazygit` and `tig`, but those are explicitly TUI tools, not workflow CLIs.

**Violation 3: Insufficient machine-readable output.** The `clade list --json` flag exists (implemented in `/Users/dlyalko/daniil/pacer/internal/cmd/list.go`, line 27), but `--json` is missing from `status`, `resume`, `cleanup`, `doctor`, and `work`. Every command that produces output should have a `--json` or `--format` flag. This is table stakes for composability in 2026. The [Command Line Interface Guidelines](https://clig.dev/) are explicit about this.

**Violation 4: No quiet mode.** There is no `--quiet` or `-q` flag. When running hooks in CI, you want `clade foo -t spike -q` to produce zero output on success and only write to stderr on failure. The current output is all informational ("Creating spike: try-redis", "Copying config files...", "Worktree created!") -- useful for humans, noise for scripts.

**Violation 5: Exit codes are binary.** From what I can see, the tool returns 0 (success) or 1 (error). There is no distinction between "worktree already exists" (exit 2?), "branch already exists" (exit 3?), "repo not found" (exit 4?). Structured exit codes enable shell-level branching without parsing output.

### Flag Design Issues

The `-v` flag is version (`/Users/dlyalko/daniil/pacer/internal/cmd/root.go`, line 69), while verbose is `-V` (line 66). This violates convention. Almost every Unix tool uses `-v` for verbose and `--version` (long form only) for version. When someone types `clade -v`, they expect verbosity, not a version string. This will burn users.

The `work` command is registered but hidden (`workCmd.Hidden = true` in `/Users/dlyalko/daniil/pacer/internal/cmd/work.go`, line 66). This means `clade work foo` works, but `clade help` does not show it. Users who find it in docs will think it is gone. If it works, document it. If you want the shorthand to be canonical, at least show it in help with a note.

### The Typo Guard

The Levenshtein-based subcommand suggestion in `/Users/dlyalko/daniil/pacer/internal/cmd/root.go` (lines 112-125) is clever. Since `clade <name>` is the default action, a typo like `clade lsit` would silently create a worktree named "lsit" instead of running `clade list`. The edit distance check catches this. Good defensive design. But the threshold of 2 might be too generous -- "stat" is distance 2 from "list". Consider distance 1 for short commands.

---

## 2. Git Worktree Deep Dive

### What Clade Does Right

The git layer in `/Users/dlyalko/daniil/pacer/internal/git/` is thin and correct. It shells out to `git` rather than using a Go library like go-git. This is the right call. The git CLI is the canonical interface. go-git does not support worktrees well, and shelling out means you inherit git's own bug fixes and config handling.

`ListWorktreesDetailed` (line 106 of `/Users/dlyalko/daniil/pacer/internal/git/worktree.go`) uses `--porcelain` output, which is the correct machine-stable format. Good.

`RepairWorktrees` (line 278) handles the post-migration case where paths change. This is operational awareness -- most tools do not account for worktree links breaking after directory renames.

### Untapped Git Capabilities

**Sparse checkout integration.** This is the big one. When you create a worktree from a monorepo containing 50 services, you are checking out the entire tree. Git's sparse-checkout (cone mode) can limit the checkout to specific directories. This is critical at scale. Clade could accept a `--sparse <path>` flag: `clade try-redis -t spike --sparse services/api`. The combination of `git worktree add` + `git sparse-checkout set --cone services/api` could reduce checkout time from minutes to seconds on large repos. See [GitHub's sparse checkout blog post](https://github.blog/open-source/git/bring-your-monorepo-down-to-size-with-sparse-checkout/).

**Partial clones.** For large repos, `git clone --filter=blob:none` creates a "blobless" clone. Worktrees created from such a clone only fetch objects on demand. Clade should detect if the repo is a partial clone and handle the implications (e.g., some operations may need network).

**Worktree-specific git config.** Each worktree can have its own `.git/config.worktree` (via `git config --worktree`). Clade could use this to set worktree-specific hooks, user settings, or environment configs. For example, setting a different `core.hooksPath` per worktree.

**Detached HEAD worktrees.** `git worktree add --detach <path> <commit>` creates a worktree at a specific commit without a branch. This is useful for bisecting, reviewing tags, or investigating specific states. Clade currently assumes every worktree has a branch. Supporting `clade foo --detach v2.1.0` would be useful for investigation workflows.

**Lock/unlock.** `git worktree lock` prevents a worktree from being pruned. Clade's `cleanup` command should check for locks before removing. Currently it does not.

**Git 2.53 (February 2026)** added the `git maintenance is-needed` subcommand and the experimental `git replay` with automatic ref updates. `git maintenance` auto-maintenance could be triggered by clade after creating or removing worktrees. See [Git 2.53 release notes](https://news.tuxmachines.org/n/2026/02/02/Git_2_53_Released_with_New_Features_and_Performance_Improvement.shtml).

### Performance Concerns in `internal/git`

In `/Users/dlyalko/daniil/pacer/internal/git/branch.go`, `CreateWorktreeNew` (line 136) calls `Fetch(repoPath)` synchronously. This is a network call. For large repos or slow connections, this is the bottleneck. The function comment says "Ignore error - might be offline" which is the right fallback, but fetching on every creation is expensive. Consider making fetch opt-in (`--fetch`) or doing it in the background.

In `/Users/dlyalko/daniil/pacer/internal/git/status.go`, `GetRecentCommits` (line 75) has a bug:

```go
cmd := exec.Command("git", "log", "--oneline", "-n", string(rune('0'+count)))
```

`string(rune('0'+count))` works for count 1-9 but produces garbage for count >= 10 (it converts to a Unicode codepoint, not an ASCII digit). The fallback on line 78 caps at 10, so this happens to work in practice, but the code is incorrect by construction. This is the kind of thing that bites you when someone passes `count=15` in the future.

---

## 3. Composability Assessment

### Current State

Clade is currently a **human-interactive tool** that happens to have some machine-readable affordances. The `list --json` flag is the only structured output option. To make clade a first-class composable tool:

**Needed: `clade path <name>`.** A command that prints the worktree path to stdout and exits. This enables:
```bash
cd $(clade path try-redis)
code $(clade path try-redis)
```
Currently, you have to parse `clade list --json | jq ...` which is heavyweight for a common operation.

**Needed: `--json` everywhere.** Every command should support `--json` output. `clade work foo --json` should print `{"name":"foo","path":"/path","branch":"spike/foo","repo":"my-api"}` and exit. The human output goes to stderr, the machine output goes to stdout.

**Needed: stdin support for batch operations.** `clade cleanup --stdin` reading worktree names from stdin would enable:
```bash
clade list --json | jq -r '.worktrees[][] | select(.stale) | .name' | clade cleanup --stdin
```
This is how Unix tools compose.

**Needed: environment variable overrides.** `CLADE_REPO`, `CLADE_TYPE`, `CLADE_AGENT` should override config defaults. This enables container-based CI without config files:
```bash
CLADE_REPO=/workspace CLADE_AGENT=none clade my-feature -t feature
```

### Shell Integration

The `clade resume` command launches an agent, but it does not `cd` the parent shell into the worktree directory. This is a fundamental limitation of subprocess-based tools. The standard solution is a shell function wrapper:

```bash
clade() {
    if [ "$1" = "resume" ] || [ "$1" = "cd" ]; then
        dir=$(command clade path "$2" 2>/dev/null)
        [ -n "$dir" ] && cd "$dir"
    fi
    command clade "$@"
}
```

Clade should ship this as part of `clade completion zsh/bash/fish` or as a separate `clade shell-init` command. [gwq does this](https://github.com/d-kuro/gwq) with its `gwq cd` subcommand.

---

## 4. Hook Architecture

### Current Layering

Clade has three hook layers:

1. **Global hooks** (`~/.config/clade/hooks.yaml`) -- applied to all worktrees
2. **Per-repo hooks** (`{repo}/.clade/hooks.yaml`) -- repo-specific, with TOFU trust
3. **Agent hooks** (`.claude/settings.json`, `.cursor/hooks.json`) -- handled by the agent, not clade

The lifecycle events are `on_create`, `on_resume`, `on_remove`. The trust model uses SHA-256 hash verification for per-repo hooks (`/Users/dlyalko/daniil/pacer/internal/config/trust.go`).

### Assessment

The TOFU (Trust On First Use) model is appropriate for per-repo hooks. It mirrors how SSH handles host keys. The hash verification on subsequent runs prevents supply-chain attacks via modified hook files.

**Missing: per-worktree hooks.** What if I want a specific worktree to run `docker compose up` on resume but not others? Currently, hooks are global or per-repo, not per-worktree. The `.clade/metadata.json` file in each worktree could include a `hooks` key.

**Missing: hook ordering control.** The docs say "Per-repo hooks run AFTER global hooks" but there is no way to specify ordering within a hook set. A `priority` field or explicit `before`/`after` would help.

**Missing: async/background hooks.** `on_create` runs `npm install` synchronously. For large projects this blocks for 30+ seconds. A `background: true` option would let long-running setup tasks proceed while the agent launches:

```yaml
hooks:
  on_create:
    - command: npm install --prefer-offline
      background: true
    - command: cp .env.example .env 2>/dev/null || true
```

### How Other Tools Handle This

[Worktrunk](https://github.com/max-sixty/worktrunk) implements hooks as `setup_new` and `setup_existing` scripts that run automatically. Simple and predictable.

[gwq](https://github.com/d-kuro/gwq) delegates hooks to tmux integration rather than a custom hook system.

`git` itself uses `.git/hooks/` with a simple naming convention. The `core.hooksPath` config allows overriding per-repo.

The clade approach of YAML-defined hooks with environment variables is more structured than shell scripts but less flexible than, say, Makefile targets or `just` commands. The trade-off is reasonable for the target audience (developers who want things to work without thinking).

---

## 5. Competitive Landscape

### Direct Competitors

| Tool | Language | Focus | Key Differentiator |
|------|----------|-------|-------------------|
| **[Worktrunk](https://worktrunk.dev/)** | Rust | Parallel AI agents | Minimal surface, path derivation, Rust speed |
| **[gwq](https://github.com/d-kuro/gwq)** | Go | Global worktree dashboard | Fuzzy finder, tmux integration, cross-repo management |
| **[git-worktree-runner (gtr)](https://github.com/coderabbitai/git-worktree-runner)** | Bash | CodeRabbit AI integration | Shell script, no compile step, editor hooks |
| **[tree-me](https://haacked.com/archive/2025/11/21/tree-me/)** | ? | Minimal wrapper | Convention-based, thin layer over git |
| **[worktree-cli](https://github.com/fnebenfuehr/worktree-cli)** | ? | Modern CLI | Basic worktree ops with nice UX |
| **[branchlet](https://github.com/raghavpillai/branchlet)** | ? | Simple manager | Interactive CLI, minimal features |
| **[wtp](https://dev.to/satococoa/wtp-a-better-git-worktree-cli-tool-4i8l)** | ? | Path derivation | Branch-only input, auto path |

### Clade's Differentiation

Clade is the **only tool** that deeply integrates AI session context management with worktree lifecycle. The DROPBAG/inject-context/SessionStart hook chain is unique. No competitor has:

1. Session continuity (dropbags)
2. Cross-agent support (Claude Code + Cursor)
3. Ticket detection and injection
4. TODO scanning as context

Worktrunk is the closest competitor in spirit -- it is also designed for parallel AI agents and has hooks. But Worktrunk is narrower: it manages worktrees and hooks. It does not do context injection, session persistence, or multi-agent detection.

gwq is the closest competitor in features -- global dashboard, cross-repo management, status monitoring. But gwq does not do context injection or AI session management.

### What to Steal

**From gwq:** The `exec` subcommand -- run a command inside a worktree without cd-ing:
```bash
gwq exec my-feature -- npm test
```
This is enormously useful for CI and scripting.

**From gwq:** Global cross-repo status dashboard. gwq can show all worktrees across all repos in one view. Clade's `list` groups by repo but requires repos to be registered first.

**From Worktrunk:** Path derivation is automatic and convention-based. No state file needed. This is simpler and more resilient than clade's `state.json`. If the state file gets corrupted or out of sync with reality, clade breaks. Worktrunk derives state from the filesystem.

**From tmux ecosystem:** Several tools (gwq, manual workflows) use tmux sessions per worktree. Each worktree gets its own tmux session. Clade's TMUX support is limited to `TmuxSplitDirection` for editor opening. A deeper integration where `clade foo` creates a named tmux session `clade:foo` would be powerful for multi-agent workflows.

---

## 6. Performance and Scale

### Current Architecture

Clade's state management uses a single JSON file (`~/clade/state.json` described in `/Users/dlyalko/daniil/pacer/ARCHITECTURE.md`). The file is read/written atomically on every operation. At small scale (5 repos, 10 worktrees), this is fine.

### Scale Concerns

**50 repos, 200 worktrees:** The `state.json` file grows linearly. A 200-worktree state file is maybe 50KB -- not a problem for I/O. But:

1. **`clade list` is O(repos * worktrees).** For each registered repo, `ListWorktreesDetailed` shells out to `git worktree list --porcelain`. With 50 repos, that is 50 subprocess spawns. Each one does stat() calls on the filesystem. This will take 2-5 seconds.

2. **`showDashboardWorktrees` calls `HasUncommittedChanges` per worktree.** That is `git status --porcelain` per worktree. With 200 worktrees: 200 subprocess spawns. This will take 10-30 seconds on a loaded machine. The dashboard becomes unusable.

3. **Fetch on every create/resume.** `git fetch origin` is called synchronously in `CreateWorktreeNew` and `ResumeWorktree`. With large repos, each fetch can take 5-10 seconds. This is the single biggest performance bottleneck.

4. **State file locking.** There is no file lock on `state.json`. If two clade instances run concurrently (e.g., two terminal tabs), they can race on read-modify-write. With 200 worktrees, concurrent use is expected.

### Recommendations

- Parallelize git status checks using goroutines (Go makes this trivial)
- Cache git status with a short TTL (e.g., 30 seconds) to make repeated `list` calls fast
- Make `fetch` lazy or background: `clade foo --no-fetch` or fetch in a goroutine that does not block worktree creation
- Add advisory file locking to `state.json` (e.g., `flock` on Linux/macOS)
- Consider splitting state by repo: `~/clade/repos/{repo}/.clade-state.json` instead of one global file. This eliminates contention and makes state file size proportional to per-repo worktrees, not total worktrees

---

## Summary Verdict

Clade solves a real problem that I have personally felt when juggling multiple git worktrees with AI coding agents. The DROPBAG/inject-context chain is genuinely novel in this space. The code is clean Go, the security posture is reasonable (branch validation, symlink-safe copies, TOFU hooks), and the evolution from v0.1 to v0.4 shows good design instinct.

**What makes it feel like a first-class tool:** Shell completions, `--dry-run` on destructive operations, `doctor` for diagnostics, the typo guard on subcommands, auto-detection of Cursor vs Claude Code output format.

**What prevents it from being a first-class tool:** Interactive prompts blocking the fast path, no `--quiet` mode, insufficient `--json` coverage, no `path` subcommand for composability, the `-v` = version convention inversion, synchronous network fetches on the critical path, and the state file as a single point of failure at scale.

The competitive landscape is heating up fast. Worktrunk (Rust, January 2026), gwq (Go, 2025), and git-worktree-runner (Bash, CodeRabbit) are all targeting the same user. Clade's moat is context management -- the DROPBAG/inject-context/SessionStart chain. Protect that moat. Make it better. Do not try to out-feature gwq on dashboards or Worktrunk on raw speed. Focus on the thing that nobody else does: making AI agents remember where they left off.

---

**Sources:**

- [Git Worktree Documentation](https://git-scm.com/docs/git-worktree)
- [Worktrunk - Git Worktree CLI for Parallel AI Agents](https://github.com/max-sixty/worktrunk)
- [gwq - Git Worktree Manager with Fuzzy Finder](https://github.com/d-kuro/gwq)
- [git-worktree-runner (gtr)](https://github.com/coderabbitai/git-worktree-runner)
- [wtp - A Better Git Worktree CLI Tool](https://dev.to/satococoa/wtp-a-better-git-worktree-cli-tool-4i8l)
- [tree-me: Because git worktrees shouldn't be a chore](https://haacked.com/archive/2025/11/21/tree-me/)
- [Command Line Interface Guidelines](https://clig.dev/)
- [Git Worktrees + Claude Code: Parallel Development](https://medium.com/@francoisschuers/git-worktrees-claude-code-effortless-parallel-development-2a43e746c28c)
- [How Git Worktrees Changed My AI Agent Workflow (Nx Blog)](https://nx.dev/blog/git-worktrees-ai-agents)
- [GitHub Blog: Sparse Checkout for Monorepos](https://github.blog/open-source/git/bring-your-monorepo-down-to-size-with-sparse-checkout/)
- [Git 2.53 Released with New Features](https://news.tuxmachines.org/n/2026/02/02/Git_2_53_Released_with_New_Features_and_Performance_Improvement.shtml)
- [CLI Is the New MCP for AI Agents](https://oneuptime.com/blog/post/2026-02-03-cli-is-the-new-mcp/view)
- [Claude Code vs. the Unix Philosophy](https://derickschaefer.medium.com/claude-code-vs-the-unix-philosophy-e1141d9111e6)
- [Visual Studio 2026 Git Workflow Improvements](https://devblogs.microsoft.com/visualstudio/streamlining-your-git-workflow-with-visual-studio-2026/)
- [Worktrunk Open Sources Git Worktree CLI](https://ascii.co.uk/news/article/news-20260101-05a8cecc/worktrunk-open-sources-git-worktree-cli-for-parallel-ai-agen)
- [Coding-Agent-Friendly Environment: ghq x gwq x fzf](https://shunk031.me/post/ghq-gwq-fzf-worktree/)
