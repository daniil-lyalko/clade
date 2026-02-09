# Clade DX Review -- Priya Sharma, Principal UX Engineer

## Executive Summary

Clade solves a real and growing problem. As parallel AI-agent workflows become standard in 2026, git worktree management is now a daily necessity rather than a niche concern. The tool has solid bones: smart defaults, good error messages, and a thoughtful hook architecture. But the first five minutes are rough, the concept surface area is too wide for the value delivered, and the UI layer (promptui) is holding back a tool that wants to feel like lazygit-for-worktrees.

This review covers seven areas, grounded in the actual code I read.

---

## 1. First 5 Minutes Audit

### The Happy Path (as documented)

```
brew install daniil-lyalko/clade/clade   # 1. Install
clade foo                                 # 2. First run - wizard fires
                                          # 3. Pick AI tool
                                          # 4. FAIL: "Not in a git repository"
```

### Where It Breaks Down

**Step-by-step reality for a new user:**

1. User installs clade via Homebrew. Smooth.

2. User runs `clade foo` from their home directory (as the README suggests). The first-run wizard fires (lines 169-218 of `/Users/dlyalko/daniil/pacer/internal/config/config.go`), asks a single question about their AI tool, saves config. Good.

3. Then `runWork` executes. It calls `resolveRepoWithPick`. There are no registered repos. The user is **not** in a git repo. They get:
   ```
   Not in a git repository. Register repos with: clade repo add <path>
   ```

4. Now the user must:
   - Navigate to a git repo or know a path
   - Run `clade repo add ~/repos/my-api`
   - Run `clade foo` again
   - Get prompted for branch name (another interruption)
   - Worktree gets created

**That is 4-5 steps and 2 context switches before the user sees any value.** The README's Quick Start shows `clade foo` as step one, but if the user is not already `cd`-ed into a git repo, it fails immediately after the wizard completes.

**Specific code issue (root.go line 97-106):** The root command delegates straight to `runWork` if any args are present, but the first-run wizard only asks about the AI tool. It does NOT ask "Where are your repos?" -- so the user walks through setup and then immediately hits a wall.

```go
// root.go line 89-110
func runRoot(cmd *cobra.Command, args []string) error {
    if versionFlag { ... }
    if len(args) > 0 {
        // Goes straight to runWork -- no check for "is this the very first command ever?"
        return runWork(cmd, args)
    }
    return runInteractiveDashboard(cmd, args)
}
```

**The branch name prompt is unnecessary friction.** In `worktree.go` lines 76-87, even when the user passes `clade foo -t spike`, they still get a `promptui.Prompt` asking to confirm the branch name `spike/foo`. For 90% of cases the default is correct. This should be auto-accepted unless the user passes `--branch` or an explicit interactive flag.

### Minimum Path to Value

Currently: install -> run -> wizard -> fail -> repo add -> run again -> branch prompt -> done (5+ interactions, 2+ commands)

Should be: install -> cd to any git repo -> run -> done (2 interactions, 1 command)

### Recommendation: Auto-detect, then ask

```
$ clade try-redis -t spike

  Welcome to clade! Let's set up your preferences.
  What AI coding tool do you use?
  > Claude Code (terminal)

  Detected git repo: my-api (/Users/you/repos/my-api)
  Register it for quick access? [Y/n]

  Creating spike: try-redis
    Repo: my-api
    Path: ~/clade/repos/my-api/try-redis
    Branch: spike/try-redis
  Launching claude...
```

This is achievable because the detection code already exists in `root.go` lines 736-758 (`detectCurrentRepo`). It just needs to be wired into the first-run flow.

---

## 2. The Setup Problem

### Current Setup Surface Area

Clade has **four** separate setup/init concepts:

| Command | What it does | When needed |
|---------|-------------|-------------|
| **First-run wizard** (`config.go:169`) | Sets agent/editor preference | Auto on first run |
| **`clade init`** (`init.go`) | Creates `.clade/hooks.yaml.example`, updates `.gitignore` | Per-repo, manual |
| **`clade setup`** (`setup.go`) | Installs global hooks to `~/.claude/`, `~/.cursor/` | One-time, manual |
| **`clade repo add`** | Registers a repo in config | Per-repo, manual |

The problem: `init` and `setup` serve different purposes but are named confusingly. "Init" sounds like the first thing you run (it is not). "Setup" sounds like a per-project thing (it is not). And `repo add` is a third step that could be automated.

Additionally, `clade init` (in `init.go` lines 35-79) now only creates `.clade/hooks.yaml.example` and updates `.gitignore` -- it no longer creates `.claude/settings.json` or `.cursor/hooks.json`. The global hooks are in `setup`. But the README at line 97 still says "clade init -- Setup hooks in current repo" and the CLAUDE.md flow diagram says "clade init" creates `.claude/settings.json`. This documentation mismatch will confuse users.

**The doctor command reveals the gap.** In `doctor.go` lines 757-858, `checkGlobalHooks()` checks for Claude/Cursor hooks at `~/.claude/settings.json` and `~/.cursor/hooks.json`. When missing, it says "not found -- run 'clade setup'". But a new user who just ran through the wizard has no idea `clade setup` exists. The wizard does not mention it. The dashboard does not prompt for it.

### How to Collapse This

**Merge init, setup, and repo-add into the first-run wizard.**

When the wizard detects the user is in a git repo:

```
  Welcome to clade!

  1/3  What AI coding tool do you use?
       > Claude Code (terminal)

  2/3  Detected repo: my-api (/Users/you/repos/my-api)
       Register it? [Y/n]

  3/3  Install global hooks for context injection?
       This sets up SessionStart hooks in:
         ~/.claude/settings.json
         ~/.cursor/hooks.json
       [Y/n]

  Done! Run `clade foo` to create your first worktree.
```

That collapses four commands into one interactive flow. Keep `init`, `setup`, and `repo add` as standalone commands for users who need them, but the happy path should not require knowing they exist.

---

## 3. Dashboard and TUI Vision

### Current State

The dashboard in `root.go` lines 163-595 uses `promptui` for all interaction. This is a functional but limited library -- it provides basic select lists and text prompts with no support for:

- Multi-column layouts
- Real-time updates
- Keyboard shortcuts
- Split panes
- Search/filter in lists

The dashboard shows a list of worktrees (max 5), legacy experiments (max 3), scratches (max 3), then an action picker. The action picker is a flat list of 8-9 items with no grouping or keyboard shortcuts.

From `root.go` lines 482-595:
```go
actions := []action{}
// Resume, New, Scratch, List, Register repo, Clean up, Config, Doctor, Setup, Exit
// ... all in a flat promptui.Select
```

### What a Bubbletea-Powered TUI Could Look Like

The [Bubbletea framework](https://github.com/charmbracelet/bubbletea) (updated Feb 5, 2026) with [lipgloss](https://github.com/charmbracelet/lipgloss) for styling would allow something like:

```
  clade                                            v0.5.0

  ACTIVE WORKTREES                          QUICK ACTIONS
  ──────────────────────────────────────    ─────────────
  try-redis      spike  (my-api)  2h ago    n  New worktree
  PROJ-1234      bug    (my-api)  3d ago    s  Scratch folder
  ui-redesign    feat   (frontend) 1d ago   d  Run doctor

  SCRATCH FOLDERS                            RECENT REPOS
  ──────────────────────────────────────    ─────────────
  doc-review              5h ago            my-api (current)
                                            my-frontend

  [enter] resume  [n]ew  [s]cratch  [c]leanup  [?] help  [q]uit
```

Key differences:
- **Single-key shortcuts** instead of navigating a select list
- **Split layout** showing state and actions simultaneously
- **Real-time** -- no need to re-run `clade list` after cleanup
- **Inline context preview** -- select a worktree and see its DROPBAG summary, branch status, last commit

This approach is proven by tools like [lazygit](https://github.com/jesseduffield/lazygit) (zero memorization, zero context switching) and k9s. Both use the same principle: **show state and provide actions in the same view**.

### Migration Path

This does not need to be a Big Rewrite. Bubbletea programs are composed of models. You can:

1. Keep all existing CLI commands working exactly as they do now
2. Replace only the `runInteractiveDashboard` function with a Bubbletea model
3. Progressively add TUI to other interactive flows (resume picker, repo picker)

The `promptui.Select` calls scattered throughout the codebase (I count them in root.go, work.go, resume.go, config_cmd.go, worktree.go) can be replaced incrementally.

---

## 4. Progressive Disclosure

### Current Model

Clade's current approach is mostly flat. The `--help` output shows everything. The README shows everything. The dashboard shows everything (Resume, New, Scratch, List, Register repo, Clean up, Config, Doctor, Setup -- nine choices for a brand new user).

From `root.go` lines 503-563, the action picker adds ALL actions regardless of user maturity:

```go
actions = append(actions,
    action{Name: "Config", ...},
    action{Name: "Doctor", ...},
    action{Name: "Setup", ...},
)
```

### Better Layering

**Beginner (day 1):**
```
clade foo                    # Just works. Wizard handles the rest.
clade list                   # What do I have?
clade resume foo             # Get back to work
clade cleanup foo            # Done
```
Four commands. That is the entire surface area for week one.

**Intermediate (week 2+):**
```
clade foo -t spike           # Learn about types/labels
clade foo -r my-api          # Multiple repos
clade scratch notes          # No-git workspace
clade config                 # Tune settings
```

**Advanced (month 2+):**
```
clade foo -b custom/branch   # Custom branch names
clade --experimental project # Multi-repo coordination
hooks.yaml                   # Lifecycle hooks
custom_labels                # Custom label types
copy_files                   # Custom file copying
```

### How to Implement This

1. **Dashboard should be context-sensitive.** If the user has 0 repos and 0 worktrees, the dashboard should show:
   ```
   Welcome to clade!
   > Create your first worktree   (walks you through everything)
     Learn more
   ```
   Not 9 options.

2. **Help text should be tiered.** `clade --help` shows the 4 core commands. `clade --help --all` shows everything. The current help is already pretty good at grouping, but the flat list of flags on the root command is overwhelming.

3. **The work command hides itself** (`workCmd.Hidden = true` in `work.go` line 65) but its flags leak into the root command because root re-declares them. This is actually a clever pattern -- users type `clade foo` not `clade work foo` -- but the `--help` output for root shows all 10+ flags, which is intimidating for a new user.

---

## 5. The Naming Problem

### Current Concept Count

Clade asks users to understand these concepts:

| Concept | What it is | Status |
|---------|-----------|--------|
| **Worktree** | Core unit of work | Active |
| **Experiment** | Legacy name for worktree | Legacy, still in code |
| **Scratch** | No-git folder | Active |
| **Project** | Multi-repo workspace | Experimental |
| **Label/Type** | spike/feature/bug/chore/hotfix/docs | Active |
| **Repo** | Registered git repository | Active |

That is 6 concepts. For a tool whose core loop is "create branch, work, cleanup," that is too many.

**The legacy "experiment" concept is particularly problematic.** It still appears in:
- State file format
- Dashboard display (`root.go` lines 189-204: "Legacy experiments")
- Resume logic (`resume.go` lines 100-106)
- Interactive picker (`resume.go` lines 167-175)

New users will see "Legacy experiments" in the dashboard and wonder what that means. The v1-to-v2 migration has not been fully completed in the UX layer.

**"Worktree" itself is jargon.** Most developers know what a git branch is. Fewer know what a worktree is. Clade's documentation explains it well, but the moment a user hits `clade list` and sees "3 worktrees," they may not immediately map that to "3 active branches I'm working on."

### Recommendation

- **Kill "experiment" from all user-facing text.** Show them as worktrees with a "(legacy)" tag if needed, but do not use the word "experiment" in headers or labels.
- **Consider "workspace" instead of "worktree"** for user-facing copy. The underlying git concept is worktree, but the user experience is "an isolated workspace for a task."
- **Collapse "project" into the main flow** or remove it. The `--experimental` flag has kept it in limbo. Either commit to multi-repo support or remove the concept entirely. Having it sit behind a flag clutters the mental model.
- **"Scratch" is clear and good.** Keep it.

---

## 6. Error Messages and Recovery

### Strengths

Error messages are generally excellent. Examples from the code:

From `worktree.go` line 153-156:
```go
ui.Error("Branch '%s' already exists", branch)
ui.Detail("Use: clade resume %s", name)
ui.Detail("Or pick a different name")
```
This is textbook: state the problem, suggest the fix. Well done.

From `resume.go` line 527-533:
```go
ui.Error("'%s' not found in clade state", name)
ui.Detail("To adopt an existing branch, specify its full name:")
ui.Detail("  clade resume %s -r %s --branch <branch-name>", name, repoName)
```
Specific, actionable, includes the exact command to run.

### The Doctor Command is Strong

`doctor.go` is comprehensive (858 lines). It checks config, state, base dir, git, agent, repos, trust registry, global hooks, orphaned worktrees, untracked worktrees, and prunable worktrees. The `--fix` flag for auto-repair is a great pattern. The `--json` flag for scripting is thoughtful.

### Gaps

1. **The typo guard in root.go is excellent but could go further.** Lines 102-104 use Levenshtein distance to catch `clade injext-context` (a real risk given the dual role of root as both dispatcher and shorthand for `work`). But it only checks against subcommands. If a user types `clade resum` (off by one from `resume`), it would try to create a worktree named "resum" because there is no close match (edit distance 1 from "resume" which should trigger). Actually, checking the code at line 115, the threshold is `bestDist := 3` meaning it catches distance <= 2. "resum" to "resume" is distance 1, so it would be caught. Good.

2. **No "did you mean?" for flags.** If a user types `clade foo --typ spike` instead of `--type spike`, cobra will error with a generic message. Consider adding flag suggestions.

3. **Silent failures in hooks.** From `worktree.go` lines 209-228, hook failures are shown as warnings but execution continues. This is the right default, but there is no way for a user to see "hook output" after the fact. A `clade logs` or `--verbose` mode for hooks would help debugging.

4. **The branch name prompt has no validation preview.** In `worktree.go` line 76-87, the user is asked to confirm/edit the branch name, but there is no preview of whether that branch already exists until after they confirm. The check happens at line 151-157. Moving the existence check into the prompt (or at least showing "branch available" / "branch taken" inline) would prevent the annoying cycle of: pick name -> confirm -> "branch exists" -> start over.

---

## 7. Future Vision: AI-Assisted Dev Workflow 2027

### The Competitive Landscape Has Changed

When Clade was designed, it was unique. Now there are direct competitors:

- [**Worktrunk**](https://github.com/max-sixty/worktrunk) (Feb 2026): "Three core commands" (`switch`, `list`, `merge`) -- deliberately minimal, focused on parallel AI agents. Written in Rust. Already in Homebrew.
- [**wtp**](https://dev.to/satococoa/wtp-a-better-git-worktree-cli-tool-4i8l): Another worktree CLI born from daily Claude Code usage
- [**worktree-workflow**](https://github.com/forrestchang/worktree-workflow): A toolkit specifically for Claude Code parallel development

Clade's differentiator is the **context layer** -- dropbags, hook injection, session continuity. None of the competitors do this. But the worktree management piece alone is becoming commoditized.

### Where Clade Should Double Down

**Context is the moat.** In 2026, the separating factor between AI coding tools is [how well they integrate with your workflow: tool calling, project context, and predictable behavior](https://addyosmani.com/blog/ai-coding-workflow/). Clade is uniquely positioned to be the **context orchestration layer** for AI coding sessions.

Vision for 2027:

1. **Parallel agent coordination.** Worktrunk's pitch is "run 5-10 agents in parallel." Clade should support this natively: `clade foo -t spike --parallel 3` creates the worktree and launches 3 agent sessions with coordinated context. The `project` concept could evolve into this.

2. **Context-as-a-graph, not files.** DROPBAGs are flat markdown files. The next step is structured context that agents can query: "What was the decision on the Redis caching approach?" rather than dumping the entire DROPBAG. This could integrate with MCP servers.

3. **Team-aware worktrees.** "Alice is working on PROJ-1234 in my-api. Bob just merged fix/PROJ-1235." Clade already tracks state; extending it to team coordination (via shared state or git notes) would be powerful.

4. **The TUI as command center.** A Bubbletea dashboard that shows all active agents, their status, recent commits per worktree, branch merge readiness -- this is the "k9s for AI coding" vision.

5. **Zero-config onboarding.** Following the [Evil Martians principle](https://evilmartians.com/chronicles/easy-and-epiphany-4-ways-to-stop-misguided-dev-tools-users-onboarding) of "show value before asking commitment": `clade try-redis` should work from any git repo with zero prior setup. Detect the repo, create the worktree, launch the agent. Ask about preferences later.

---

## Summary of Actionable Findings

| Priority | Issue | Impact | Effort |
|----------|-------|--------|--------|
| P0 | First-run wizard does not handle repo detection/registration | Users fail immediately after wizard | Small |
| P0 | Branch name prompt is unnecessary for default cases | Friction on every create | Small |
| P1 | `init` vs `setup` naming confusion + docs mismatch | Users miss global hook setup | Medium |
| P1 | Dashboard shows too many options for new users | Cognitive overload | Medium |
| P1 | "Experiment" still visible in dashboard and picker | Confuses new users, feels unpolished | Small |
| P2 | promptui limits interactivity of dashboard | Cannot compete with lazygit/k9s UX | Large |
| P2 | No tiered help output | All complexity visible immediately | Medium |
| P3 | Multi-repo "project" concept in limbo | Mental model bloat | Decision |
| P3 | "Worktree" jargon in user-facing text | Raises cognitive load for non-git-experts | Small (copy change) |

---

## Sources

- [Evil Martians: 6 things developer tools must have in 2026](https://evilmartians.com/chronicles/six-things-developer-tools-must-have-to-earn-trust-and-adoption)
- [Evil Martians: Ease and epiphany -- 4 ways to stop misguided dev tools onboarding](https://evilmartians.com/chronicles/easy-and-epiphany-4-ways-to-stop-misguided-dev-tools-users-onboarding)
- [Addy Osmani: My LLM coding workflow going into 2026](https://addyosmani.com/blog/ai-coding-workflow/)
- [Nx Blog: How Git Worktrees Changed My AI Agent Workflow](https://nx.dev/blog/git-worktrees-ai-agents)
- [Worktrunk: Git Worktree CLI for Parallel AI Agents](https://github.com/max-sixty/worktrunk)
- [worktree-workflow: Toolkit for Claude Code parallel development](https://github.com/forrestchang/worktree-workflow)
- [wtp: A Better Git Worktree CLI Tool](https://dev.to/satococoa/wtp-a-better-git-worktree-cli-tool-4i8l)
- [Charmbracelet Bubbletea: TUI Framework for Go](https://github.com/charmbracelet/bubbletea)
- [Lazygit: Simple Terminal UI for Git](https://github.com/jesseduffield/lazygit)
- [Progressive Disclosure Matters: Applying 90s UX Wisdom to 2026 AI Agents](https://aipositive.substack.com/p/progressive-disclosure-matters)
- [IxDF: What is Progressive Disclosure?](https://www.interaction-design.org/literature/topics/progressive-disclosure)
- [Zuplo Blog: MCP or CLI? What Makes Sense for Developer Tools](https://zuplo.com/blog/cli-or-mcp)
- [Tembo: 2026 Guide to Coding CLI Tools -- 15 AI Agents Compared](https://www.tembo.io/blog/coding-cli-tools-comparison)
- [14 Great Tips to Make Amazing CLI Applications](https://dev.to/wesen/14-great-tips-to-make-amazing-cli-applications-3gp3)
