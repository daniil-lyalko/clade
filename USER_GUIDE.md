# Clade User Guide

Clade is a Go CLI that manages git worktrees and context for AI coding sessions. Named after biological clades (branching groups sharing common ancestry) - perfect metaphor for worktree branches.

**Core insight:** Claude Code already has hooks, sessions, MCP. Clade doesn't replace these - it orchestrates worktrees and context so Claude's built-in features work better.

**Three problems it solves:**

1. **Worktree friction** - `git worktree add ../some-path -b some-branch` is verbose. You forget the syntax. Paths are manual. You end up just stashing or making messy commits instead of properly isolating work.

2. **Multi-repo coordination** - You're building a feature across 3 repos. No native way to create matching branches, unified workspace, coordinated context. You're juggling terminal tabs and losing track.

3. **Context loss** - You're deep in a session, context built up over hours. You switch tasks or go home. Tomorrow, Claude has no idea where you left off. You waste 15 minutes re-explaining what you were doing.

Clade fixes all three.

---

## Table of Contents

- [Getting Started](#getting-started)
- [Directory Structure](#directory-structure)
- [Commands](#commands)
- [Configuration](#configuration)
- [Workflows](#workflows)
- [Quick Reference](#quick-reference)

---

## Getting Started

### Installation

```bash
go install github.com/daniil-lyalko/clade/cmd/clade@latest
```

### First Run

On first use, clade prompts you to choose your AI coding tool:

```
  Welcome to clade! Let's set up your preferences.

  What AI coding tool do you use?
  > Claude Code (terminal)
    Cursor (IDE)
    Both Claude Code and Cursor
    Neither (just worktree management)
```

This sets `agent` and `editor` appropriately. You can always edit the config later.

### Quick Start

```bash
# Register a repo
clade repo add ~/repos/my-api

# Create a worktree
cd ~/repos/my-api
clade try-redis -t spike

# See what's active
clade list

# Resume work
clade resume try-redis

# Clean up when done
clade cleanup try-redis
```

---

## For Cursor Users

If you're primarily a **Cursor IDE** user, clade integrates seamlessly with Cursor's hook system to inject context into your AI conversations.

### Cursor-First Setup

```bash
# Install clade
go install github.com/daniil-lyalko/clade/cmd/clade@latest

# First run - select "Cursor (IDE)"
clade
#   What AI coding tool do you use?
#     Claude Code (terminal)
#   > Cursor (IDE)            <-- Select this
#     Both Claude Code and Cursor
#     Neither (just worktree management)
```

This configures clade with:
- `editor`: `"cursor"` - Opens Cursor when creating/resuming worktrees
- `agent`: `""` (empty) - No separate terminal agent needed

### How It Works with Cursor

When you run `clade init` in a repo, it creates **both** hook configurations:

```
.cursor/
  hooks.json              # Cursor's native hook system

.claude/
  settings.json           # Also created (Cursor reads these too)
  commands/
    drop.md               # /drop command works in Cursor
```

**Cursor's `sessionStart` hook** calls `clade inject-context`, which:
1. Detects it's being called by Cursor (piped stdin)
2. Returns JSON: `{"additional_context": "..."}`
3. Cursor prepends this to the system prompt

Your AI in Cursor automatically sees:
- Last session's DROPBAG notes
- Git status and recent commits
- Open TODOs in the codebase
- Ticket info if detected

### Cursor Workflow

```bash
# Create a worktree - Cursor opens automatically
cd ~/repos/my-api
clade try-redis -t spike
# -> Creates worktree
# -> Opens Cursor in that directory
# -> Cursor's AI has full context injected

# Resume work tomorrow
clade resume try-redis
# -> Opens Cursor
# -> SessionStart hook fires
# -> AI sees your DROPBAG from yesterday

# Or open in Cursor explicitly
clade resume try-redis -o cursor
```

### Cursor + Claude Code Together

Some developers use both - Cursor for visual editing, Claude Code for terminal tasks. Select "Both" in the wizard:

```bash
clade
#   What AI coding tool do you use?
#     Claude Code (terminal)
#     Cursor (IDE)
#   > Both Claude Code and Cursor  <-- Select this
```

This sets:
- `editor`: `"cursor"` - Opens Cursor first
- `agent`: `"claude"` - Then launches Claude Code in terminal

**Deduplication:** If both Cursor and Claude Code fire their hooks, clade detects this and skips the second injection within 3 seconds.

### Cursor-Specific Config

```json
// ~/.config/clade/config.json
{
  "editor": "cursor",
  "agent": "",
  "auto_init": true,
  "repos": {
    "my-api": "~/repos/my-api"
  }
}
```

| Field | Cursor-First Value | Description |
|-------|-------------------|-------------|
| `editor` | `"cursor"` | Opens Cursor on create/resume |
| `agent` | `""` (empty) | No terminal agent (Cursor has built-in AI) |

### Testing Cursor Integration

```bash
# Verify hook output format (should be JSON)
echo '{}' | clade inject-context
# {"additional_context":"# Session Context\n\n## Git Status\n..."}

# Check .cursor/hooks.json exists
cat .cursor/hooks.json
# {
#   "hooks": {
#     "sessionStart": [{
#       "command": "clade inject-context"
#     }]
#   }
# }
```

### Cursor Troubleshooting

**Hook not firing?**
1. Ensure `.cursor/hooks.json` exists: `clade init`
2. Restart Cursor after modifying hooks
3. Check Cursor settings: hooks must be enabled

**Context not appearing?**
1. Test manually: `echo '{}' | clade inject-context`
2. Should return JSON with `additional_context` field
3. If plain text, clade isn't detecting Cursor mode

**Using Cursor's "Third-party hooks" feature?**
Cursor may also read `.claude/settings.json`. Clade handles this - if both hooks fire, the second is silently skipped (3-second dedup window).

---

## Directory Structure

### Clade's Home: `~/clade/`

```
~/clade/                                    # Everything clade creates lives here
+-- repos/                                  # Repo-centric worktrees (v0.3+)
|   +-- my-api/                             # Grouped by repository
|   |   +-- try-redis/                      # label: spike
|   |   +-- PROJ-1234/                      # label: bug
|   |   +-- new-feature/                    # label: feature
|   +-- my-frontend/
|       +-- ui-redesign/                    # label: feature
|
+-- projects/                               # Multi-repo workspaces
|   +-- api-integration/                    # Project name
|       +-- backend/                        # Worktree: my-api
|       +-- package/                        # Worktree: my-package
|       +-- admin-ui/                       # Worktree: my-frontend
|
+-- scratch/                                # No-git scratch folders
|   +-- doc-review/                         # Document analysis
|   +-- meeting-notes/                      # Temporary workspace
|
+-- state.json                              # Clade's state tracking
```

### Config: `~/.config/clade/`

```
~/.config/clade/
+-- config.json                             # User configuration
+-- hooks.yaml                              # Global lifecycle hooks
+-- trusted-repos.json                      # Hook trust registry
```

### Per-Worktree: `.claude/` (Generated by clade init)

```
{any-worktree}/
+-- .claude/
|   +-- settings.json                       # Claude Code hooks
|   +-- commands/
|   |   +-- drop.md                         # /drop command template
|   +-- dropbags/                           # Session handoff archives (gitignored)
|       +-- DROPBAG-2026-01-26-1430.md      # Timestamped session notes
|       +-- DROPBAG-2026-01-25-0915.md      # Previous session
+-- .cursor/
|   +-- hooks.json                          # Cursor IDE hooks
|   +-- commands/
|       +-- drop.md                         # /drop command for Cursor
+-- .clade/
|   +-- metadata.json                       # Clade metadata (ticket, label, etc.)
+-- CLAUDE.md                               # Project context
```

### Hook Integration (Claude Code + Cursor)

Clade supports both Claude Code CLI and Cursor IDE through their respective hook systems:

**Claude Code** (`.claude/settings.json`):
- Hook: `SessionStart` -> runs `clade inject-context`
- Output: Plain text to stdout
- Context injected into conversation

**Cursor** (`.cursor/hooks.json`):
- Hook: `sessionStart` -> runs `clade inject-context`
- Output: JSON with `{"additional_context": "..."}` (auto-detected)
- Context prepended to system prompt

Both receive:
- Most recent DROPBAG from last session (with age + staleness warnings)
- Git status and recent commits
- Open TODOs in the codebase
- Ticket information from .clade.json

**Format auto-detection:** `inject-context` detects Cursor vs Claude Code by
checking if stdin is a pipe (Cursor) or a terminal (Claude Code). No `--json`
flag is needed.

**Deduplication:** Cursor may fire both `.cursor/hooks.json` and
`.claude/settings.json` hooks (via its "Third-party hooks" feature). To prevent
double injection, the second call within 3 seconds for the same directory is
silently skipped.

**Verifying hooks work:**
```bash
# Test Claude Code format (plain text)
clade inject-context

# Test Cursor format (simulates piped stdin -> JSON output)
echo '{}' | clade inject-context
```

**DROPBAG Archives:**
- `/drop` creates timestamped files: `.clade/dropbags/DROPBAG-2026-01-26-1430.md`
- Most recent file is auto-injected at SessionStart
- Staleness warning appears if >2 days old
- Full session history preserved (no auto-deletion)

---

## Commands

### `clade init`

**Purpose:** Setup a repo for clade (generates .claude/ config with hooks)

```bash
cd ~/repos/my-api
clade init
```

**Creates:**
```
.claude/
+-- settings.json         # SessionStart hook
+-- commands/
    +-- drop.md           # /drop command

.cursor/
+-- hooks.json            # sessionStart hook
+-- commands/
    +-- drop.md           # /drop command

.clade/
+-- hooks.yaml.example    # Template for lifecycle hooks

# Also appends to .gitignore:
.clade/
```

---

### `clade repo add <path>`

**Purpose:** Register a repo for quick access from anywhere.

```bash
clade repo add ~/repos/my-api
clade repo add ~/repos/my-frontend
clade repo add ~/repos/my-package
```

**What happens:**
- Validates path is a git repo
- Adds to `~/.config/clade/config.json` repos list
- Uses directory name as short name (or specify with `--name`)

```bash
clade repo add ~/repos/my-api --name backend
# Now you can use: clade try-redis -r backend
```

---

### `clade repo list`

**Purpose:** Show registered repos.

```bash
$ clade repo list

Registered repos:
  my-api         ~/repos/my-api (last used)
  my-frontend    ~/repos/my-frontend
  my-package     ~/repos/my-package
```

---

### `clade repo remove <name>`

**Purpose:** Unregister a repo.

```bash
clade repo remove my-frontend
```

---

### Repo Selection Logic

When you run `clade work` that needs a repo:

```
1. Did you specify --pick/-p flag?
   YES -> Force interactive picker
   NO  -> Continue to step 2

2. Are you in a git repo?
   YES -> Use current repo (most common case)
   NO  -> Continue to step 3

3. Did you specify --repo/-r flag?
   YES -> Use that repo (by path or registered name)
   NO  -> Continue to step 4

4. Are there registered repos?
   YES -> Interactive picker (last used at top)
   NO  -> Error: "Not in a git repo. Register repos with: clade repo add <path>"
```

**Examples:**

```bash
# In a repo - just works
cd ~/repos/my-api
clade work try-redis -t spike

# Force picker even in a repo
clade work try-redis -t spike -p

# Anywhere - specify repo
clade work try-redis -t spike -r my-api
clade work try-redis --repo ~/repos/my-api

# Anywhere - interactive picker
$ clade work try-redis
Select repo:
  > my-api (last used)
    my-frontend
    my-package
```

---

### `clade [name]` or `clade work [name]`

**Purpose:** Create a new worktree for isolated development.

**Simplest usage:** `clade foo` creates a worktree with branch name `foo` (no prefix).

Use `-t/--type` to add a branch prefix:

| Type | Branch Prefix | Merge Expected? |
|------|---------------|-----------------|
| (none) | (no prefix) | Depends on use |
| `spike` | `spike/` | No (throwaway) |
| `feature` | `feat/` | Yes |
| `bug` | `fix/` | Yes |
| `chore` | `chore/` | Yes |
| `hotfix` | `hotfix/` | Yes |
| `docs` | `docs/` | Yes |

**Examples:**

```bash
clade new-api                      # branch: new-api (no prefix)
clade try-redis -t spike           # branch: spike/try-redis
clade PROJ-123 -t bug              # branch: fix/PROJ-123
clade cleanup -t chore             # branch: chore/cleanup
clade foo -o cursor                # Open in Cursor IDE
clade foo --no-agent               # Skip launching agent
clade foo -a claude                # Override agent
```

**What happens:**
1. Creates `~/clade/repos/{repo}/{name}/`
2. Branch: `{name}` or `{prefix}/{name}` (with `-t`)
3. Writes `.clade.json` with metadata and label
4. Copies `.claude/` from main repo (or generates if missing)
5. Runs `on_create` hooks
6. Opens editor (if configured)
7. Launches agent (if configured)

---

### `clade` / `clade work` Flags

| Flag | Description |
|------|-------------|
| `-t`, `--type` | Type/prefix (spike, feature, bug, chore, hotfix, docs) |
| `-r`, `--repo` | Repository path or registered name |
| `-p`, `--pick` | Force repo picker even if in a git repo |
| `-b`, `--branch` | Custom branch name (skips prompt) |
| `-o`, `--open` | Open editor/IDE (cursor, code, nvim) |
| `-a`, `--agent` | Override configured agent |
| `--no-agent` | Skip launching the AI agent |
| `--no-editor` | Skip opening the editor |

---

### `clade project [name]` (EXPERIMENTAL)

> **Experimental:** Requires `--experimental` flag. May be removed in future versions.
> For most use cases, consider creating separate worktrees with the same branch name instead.

**Purpose:** Create multi-repo workspace with unified branch.

```bash
clade --experimental project api-integration
```

**Interactive prompts:**
```
Project name: api-integration
Branch name: feat/PROJ-5678/api-integration

Add repos (full path, blank when done):
  Repo: ~/repos/my-api
    Folder name [my-api]: backend
  Repo: ~/repos/my-package
    Folder name [my-package]: package
  Repo: ~/repos/my-frontend
    Folder name [my-frontend]: admin-ui
  Repo:

Creating project...
  + backend
  + package
  + admin-ui

Project ready: ~/clade/projects/api-integration
Launching claude...
```

---

### `clade list`

**Purpose:** Show all active worktrees, projects, and scratches.

```bash
$ clade list

my-api (3 worktrees)
  try-redis        spike         2h ago
  PROJ-1234        bug           3d ago  (stale)
  new-feature      feature       1d ago

my-frontend (1 worktree)
  ui-redesign      feature       1d ago

Projects
  api-integration (3 repos)      1d ago

Scratch
  doc-review                     5h ago
```

Worktrees are grouped by repository. Shows label, age, and stale marker (>7 days).
An `*` appears after names with uncommitted changes.

---

### `clade status`

**Purpose:** Show context for current directory.

```bash
$ cd ~/clade/repos/my-api/try-redis
$ clade status

Worktree: try-redis
  Repo: my-api
  Branch: spike/try-redis
  Type: spike

Context Files:
  + CLAUDE.md (project context)
  + DROPBAG.md (handoff notes from yesterday)
  - TICKET.md (no ticket linked)

Git Status:
  3 files changed, 47 insertions

Recent Sessions:
  2 hours ago - "implementing redis cache layer"
  yesterday - "initial setup"
```

---

### `clade resume [name]`

**Purpose:** Find worktree, cd there, launch agent. Hook handles context injection.

```bash
clade resume                    # Interactive picker
clade resume try-redis          # Specific worktree
clade resume api-integration    # Project
clade resume my-exp -o cursor   # Open in Cursor too
clade resume my-exp --no-agent  # Skip launching Claude
```

**Flags:**
| Flag | Description |
|------|-------------|
| `-r`, `--repo` | Repository for adopting orphaned branches |
| `-b`, `--branch` | Exact branch name to adopt |
| `-o`, `--open` | Open editor/IDE (cursor, code, nvim) |
| `--no-agent` | Skip launching the AI agent |
| `--no-editor` | Skip opening the editor |

**What happens:**
1. If tracked: finds worktree by name
2. If not tracked: searches for `exp/{name}` and `feat/{name}` branches
3. Changes to that directory
4. Opens editor (if configured)
5. Launches agent
6. SessionStart hook fires -> `clade inject-context`
7. Claude sees DROPBAG.md, git status, TODOs, ticket info

**Adopting orphaned branches:**
```bash
# Resume searches both exp/ and feat/ prefixes
clade resume my-feature -r my-repo

# If both exp/my-feature and feat/my-feature exist, prompts you to choose
# Or specify exact branch:
clade resume my-feature -r my-repo --branch feat/my-feature
```

---

### `clade scratch [name]`

**Purpose:** Create a no-git scratch folder for documents or analysis.

```bash
clade scratch doc-review
clade scratch PROJ-1234          # Ticket investigation (no code)
clade scratch meeting-notes      # Temporary workspace
```

**Unlike worktrees, scratch folders:**
- Have no git repository or worktree
- Are for temporary document analysis, file sharing, etc.
- Still get `.claude/` config for hooks and context

**What happens:**
1. Creates `~/clade/scratch/{name}/`
2. Writes `.clade.json` with metadata
3. Initializes `.claude/` configuration
4. Launches agent (default: claude)

---

### `clade cleanup [name]`

**Purpose:** Remove worktree and optionally delete branch.

```bash
clade cleanup try-redis
```

**What happens:**
1. Checks for uncommitted changes (warns if present)
2. Checks if branch is merged (via git or gh CLI)
3. Removes worktree
4. Prompts to delete branch
5. Updates state.json

```bash
$ clade cleanup try-redis

Worktree: try-redis
  Path: ~/clade/repos/my-api/try-redis
  Branch: spike/try-redis
  Label: spike

!  Uncommitted changes detected:
  M src/cache.ts
  A src/redis-client.ts

Discard changes and continue? [y/N]: y

Running on_remove hooks...
Removing worktree... done
Delete branch spike/try-redis? [y/N]: y
Deleting branch... done

Cleaned up worktree 'try-redis'
```

---

### `clade batch <tickets...>`

**Purpose:** Process multiple Jira tickets in parallel, each in its own worktree with an autonomous Claude agent.

```bash
clade batch PROJ-123 PROJ-456                   # Direct ticket IDs
clade batch --file tickets.csv                  # From CSV
clade batch --file tickets.txt                  # From text (one ID per line)
clade batch --jira-label triage-bug             # Fetched from Jira by label
clade batch --jira-label bug --jira-project SALESPRO,LEAP
```

**How it works:**
1. Clade gathers ticket IDs from all provided sources (args, `--file`, `--jira-label`), merges, and deduplicates.
2. For each ticket, a worktree is created on a per-ticket branch.
3. Each worktree runs `claude -p <prompt> --permission-mode bypassPermissions` autonomously. The default prompt instructs Claude to fetch the ticket (via Jira MCP), implement the change, run tests, push, and open a PR.
4. Workers run in parallel (configurable). Logs and state land in `~/.config/clade/batches/{id}/`.

**Input sources:**

| Source | Flag | Notes |
|---|---|---|
| Positional args | `clade batch A-1 A-2` | Space-separated ticket IDs. |
| CSV / text file | `--file / -F` | CSV supports `ticket`, `id`, `key`, `ticket_id` headers plus optional `repos` column (multi-repo per ticket). One-per-line text works too. |
| Jira label query | `--jira-label` | Comma-separated labels. Delegated to the Atlassian Jira MCP — clade does not talk to Jira directly. |
| Jira project scope | `--jira-project` | Comma-separated project keys. Optional; requires `--jira-label`. |

All sources are merged and deduplicated — feel free to combine positional args, a CSV, and a label query in one invocation.

**Flags:**

| Flag | Default | Purpose |
|---|---|---|
| `-F, --file` | — | CSV or text file of ticket IDs (optionally with per-ticket repos). |
| `-n, --concurrency` | `2` | Number of parallel workers. |
| `-r, --repo` | first registered | Default repo to work in (registered repo name or path). |
| `-t, --type` | `feature` | Branch type (`feature`, `bug`, `spike`, `chore`, `hotfix`, `docs`, or config-defined). |
| `-f, --from` | origin default | Base branch to create each ticket's branch from. |
| `--budget` | — | Max USD per ticket passed to `claude --max-budget-usd`. |
| `--prompt` | — | Override the default agent prompt. Placeholders: `{TICKET_ID}`, `{BRANCH}`. |
| `--project` | off | Create a clade multi-repo workspace per ticket instead of a single worktree. |
| `--jira-label` | — | Comma-separated Jira labels to fetch tickets for. |
| `--jira-project` | — | Comma-separated Jira project keys scoping the label search. Requires `--jira-label`. |
| `--dry-run` | off | Print what would be created (including resolved label IDs); run nothing. |

**Subcommands:**

```bash
clade batch status               # List all batches
clade batch status <batch-id>    # Detailed status for one batch
clade batch logs <batch-id> <ticket-id>   # Full log for a ticket's agent run
```

**CSV format:**

```csv
ticket,repos
PROJ-100,my-api
PROJ-200,"my-api,my-frontend"
PROJ-300,
```

Headers are optional (if absent, one ticket ID per line). The `repos` column is optional — when present, a ticket can span multiple repos, each getting a worktree on the shared branch.

**Example — label fetch with dry-run:**

```bash
$ clade batch --jira-label triage-bug --jira-project SALESPRO --dry-run

Fetching tickets from Jira for labels: triage-bug

Dry run - no changes will be made

Default repo: leap-360 (~/repos/leap-360)
Tickets: 4
Concurrency: 2
Jira labels: triage-bug
Jira projects: SALESPRO
Resolved from labels: 4

Would create:
  SALESPRO-6493 → branch fix/SALESPRO-6493
  SALESPRO-6501 → branch fix/SALESPRO-6501
  SALESPRO-6510 → branch fix/SALESPRO-6510
  SALESPRO-6514 → branch fix/SALESPRO-6514
```

**Prerequisites for label fetching:**
- The Atlassian Jira MCP must be configured and authenticated in the Claude CLI used by the current shell.
- Clade does not store or handle Jira credentials directly — the MCP owns that.

**Label semantics:**
- Multiple labels are combined with **AND** — a ticket must carry every label to match.
- Multiple `--jira-project` values are OR-scoped (tickets in any of the listed projects).
- To OR labels (match any), run clade once per label.

**Routing tickets to repos via labels:**

Clade does not inspect ticket content to choose a repo. Label fetching
assigns every resolved ticket to the default repo (`-r`), same as
positional args. If you have multiple repos and want each ticket to land
in the right one, a convention-based approach works well:

1. In Jira, tag every agent-ready ticket with two labels — a "pick me" label
   like `for-agents` and a repo-name label matching the registered clade
   repo (e.g. `leap_one_server`).
2. Run one clade batch per repo:
   ```bash
   clade batch --jira-label for-agents,leap_one_server -r leap_one_server
   clade batch --jira-label for-agents,salespro       -r salespro
   ```

Because label semantics are AND, each command returns only the tickets
meant for that repo.

For richer per-ticket repo metadata (pulling repo info from a Jira custom
field, or carrying multi-repo assignments), use the CSV path with a
`repos` column. See the "CSV format" section above.

---

### `clade migrate`

**Purpose:** Migrate existing experiments from v1 to v2 repo-centric structure.

```bash
clade migrate              # Show what would be migrated (dry-run)
clade migrate --dry-run    # Explicit dry-run
clade migrate --force      # Actually perform the migration
```

---

### `clade doctor`

**Purpose:** Diagnose common configuration issues.

```bash
$ clade doctor

Clade Doctor

  ✓ Config file (config.json)
      agent: claude
  ✓ State file
      v2, 3 worktree(s), 0 project(s), 0 scratch(es)
  ✓ Base directory
      /Users/user/clade
  ✓ Git
      git version 2.43.0
  ✓ Agent
      claude found at /usr/local/bin/claude
  ⚠ Repo: my-api
      path does not exist: ~/repos/my-api
  ✓ Trust registry
      2 trusted repo(s)

⚠ All checks passed with 1 warning(s)
```

---

### `clade inject-context`

**Purpose:** Called by SessionStart hook. Outputs context to stdout for the AI agent.

**Not user-facing** - the hook calls this automatically. Output format is auto-detected:
- **Terminal (Claude Code):** plain Markdown text
- **Piped stdin (Cursor):** JSON with `additional_context` field

---

## Configuration

### `~/.config/clade/config.json`

```json
{
  "base_dir": "~/clade",
  "agent": "claude",
  "agent_flags": ["--dangerously-skip-permissions"],
  "editor": "cursor",
  "auto_init": true,
  "repos": {
    "my-api": "~/repos/my-api",
    "my-frontend": "~/repos/my-frontend",
    "my-package": "~/repos/my-package"
  },
  "last_repo": "my-api",
  "copy_files": ["config/local.json"],
  "custom_labels": {
    "perf": { "branch_prefix": "perf", "merge_expected": true },
    "research": { "branch_prefix": "research", "merge_expected": false }
  }
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `base_dir` | `~/clade` | Where worktrees/projects are stored |
| `agent` | (from wizard) | AI agent command (has hooks, context injection) |
| `agent_flags` | `[]` | Flags to pass to agent |
| `editor` | (from wizard) | Editor/IDE to open before agent (cursor, code, nvim) |
| `auto_init` | `true` | Auto-run `clade init` on new worktrees |
| `repos` | `{}` | Registered repos (name -> path mapping) |
| `last_repo` | `null` | Last used repo (for picker default) |
| `copy_files` | `[]` | Extra files to copy to worktrees (beyond built-in defaults) |
| `custom_labels` | `{}` | Custom labels for `clade -t <label>` |

**Agent vs Editor:**
- **Agent** = AI assistant (e.g., `claude`). Gets SessionStart hooks, context injection
- **Editor** = IDE (e.g., `cursor`, `code`, `nvim`). Opens before agent launches

Both can be used together. Editor opens first, then agent takes over the terminal.

---

### `~/.config/clade/hooks.yaml` (Lifecycle Hooks)

Define scripts to run on clade operations:

```yaml
hooks:
  on_create:
    - npm install --prefer-offline
    - cp .env.example .env 2>/dev/null || true
  on_resume:
    - direnv allow 2>/dev/null || true
  on_remove:
    - echo "Cleaning up..."
```

| Event | When | Example Use Case |
|-------|------|------------------|
| `on_create` | After worktree created | `npm install`, copy `.env.example` |
| `on_resume` | Entering via `clade resume` | `direnv allow`, activate venv |
| `on_remove` | Before worktree deletion | Cleanup temp files, notify |

**Per-repo hooks:** Place `.clade/hooks.yaml` in the repo root. Per-repo hooks run AFTER global hooks.

**Environment variables available to hooks:**
```bash
CLADE_TYPE=spike|feature|bug|chore|hotfix|docs
CLADE_NAME=try-redis
CLADE_PATH=/Users/user/pacer/repos/my-api/try-redis
CLADE_REPO_NAME=my-api
CLADE_REPO_PATH=/Users/user/repos/my-api
CLADE_BRANCH=spike/try-redis
CLADE_TICKET=PROJ-1234  # if detected
```

**Automatic File Copying:**

When creating a worktree, clade automatically copies common config files from the source repo (if they exist). These are files that are typically gitignored but needed to run the project:

```
.env, .env.local, .env.development, .env.test, etc.
.npmrc, .yarnrc, .yarnrc.yml
.tool-versions, .nvmrc, .python-version, .ruby-version, .node-version, .go-version
.vscode/settings.json
```

No prompting - files from the default list are copied automatically and silently.

**Adding Extra Files:**

To copy additional files beyond the defaults, add them to your config:

```json
{
  "copy_files": ["config/local.json", ".env.staging"],

  "repo_settings": {
    "/path/to/specific/repo": {
      "copy_files": ["secrets/dev.json", "terraform.tfvars"]
    }
  }
}
```

---

### State Files

**`~/clade/state.json`** tracks all worktrees, projects, and scratches. See [ARCHITECTURE.md](ARCHITECTURE.md) for the v2 format specification.

**`.clade/metadata.json`** (per-worktree) stores metadata about the worktree:

```json
{
  "type": "worktree",
  "label": "bug",
  "name": "PROJ-1234",
  "ticket": "PROJ-1234",
  "repo": "my-api",
  "branch": "fix/PROJ-1234",
  "created": "2025-01-03T09:00:00Z"
}
```

> **Migration note**: Legacy `.clade.json` files are automatically migrated to `.clade/metadata.json` on first access.

---

## Workflows

### Scenario 1: Quick Experiment

```bash
# You're in your main repo, want to try something
$ cd ~/repos/my-api
$ clade try-redis -t spike

Creating spike: try-redis
  Path: ~/clade/repos/my-api/try-redis
  Branch: spike/try-redis
Launching claude...

# Claude starts, SessionStart hook fires, you see:
# "Session context loaded from clade"

# You work for a few hours...
# Before stopping:

> /drop

# Claude writes DROPBAG.md with your current state
# You exit claude, go home

# Next morning:
$ clade resume try-redis

Resuming: try-redis
  Path: ~/clade/repos/my-api/try-redis
Launching claude...

# SessionStart hook fires, Claude sees:
# - Your DROPBAG.md from yesterday
# - Git status showing your changes
# - Any TODOs in code
# You're immediately back in context

# Experiment worked! Clean up:
$ clade cleanup try-redis --merge

# Or it failed:
$ clade cleanup try-redis  # Just deletes everything
```

---

### Scenario 2: Ticket Investigation

```bash
$ clade PROJ-1234 -t bug

Creating bug: PROJ-1234
  Path: ~/clade/repos/my-api/PROJ-1234
  Branch: fix/PROJ-1234
  Ticket: PROJ-1234 (will prompt Claude to fetch)
Launching claude...

# Claude starts, sees in context:
# "Ticket PROJ-1234 detected. Please fetch from JIRA and save to TICKET.md"

# Claude uses JIRA MCP to fetch ticket, saves it
# Now you have full ticket context

# Investigate, fix, done:
$ clade cleanup PROJ-1234
```

---

### Scenario 3: Multi-Repo Feature (Experimental)

> Note: For most use cases, just create separate worktrees with the same branch name.

```bash
$ clade --experimental project api-integration

Project name: api-integration
Branch: feat/PROJ-5678/api-integration

Add repos:
  ~/repos/my-api -> backend
  ~/repos/my-package -> package
  ~/repos/my-frontend -> admin-ui

Creating project...
Launching claude...

# Claude sees all three repos in ~/clade/projects/api-integration/
# You work across backend/, package/, admin-ui/

# End of day:
> /drop

# Next day:
$ clade resume api-integration

# All context restored, all three repos ready
```

---

### Scenario 4: See What's Active

```bash
$ clade list

my-api (3 worktrees)
  try-redis        spike         2h ago
  PROJ-1234        bug           3d ago  (stale)

Projects:
  api-integration - 1 day ago

# Oh right, I forgot about that old investigation
$ clade cleanup PROJ-1234
```

---

### Scenario 5: Batch-process tickets by Jira label (with per-repo routing)

You have multiple repos and want to dispatch the agent-ready tickets for each repo. Tag every agent-ready ticket in Jira with two labels:

- `for-agents` — a shared "pick me up" label.
- `<repo-name>` — matches the registered clade repo name (e.g. `leap_one_server`, `salespro`).

Then run one batch per repo. Because labels are AND'd, each command pulls only the tickets meant for that repo.

```bash
# Preview what you'd get for the leap_one_server repo
$ clade batch --jira-label for-agents,leap_one_server -r leap_one_server --dry-run

# Launch it for real: 3 workers, $5 cap per ticket
$ clade batch --jira-label for-agents,leap_one_server -r leap_one_server \
    --concurrency 3 --type bug --budget 5.00

# Then the salespro repo
$ clade batch --jira-label for-agents,salespro -r salespro --concurrency 3

# Check progress across all running batches
$ clade batch status

# Drill into one ticket's agent log
$ clade batch logs 2026-04-20-0930 SALESPRO-6493
```

Each ticket gets its own worktree, its own autonomous Claude session, and a PR (or a "needs more detail" comment back on the ticket, if the description was too vague). Clade does not mutate Jira directly; the agents do that via the Jira MCP.

---

## Quick Reference

```bash
# Simplified workflow (v0.4+)
clade foo                         # Create worktree (branch: foo)
clade foo -t spike                # Create spike (branch: spike/foo)
clade                             # Interactive dashboard
clade list                        # What's active
clade resume foo                  # Get back to work
clade cleanup foo                 # Clean up when done

# Repo management
clade repo add ~/repos/my-repo    # Register a repo
clade repo list                   # Show registered repos
clade repo remove my-repo         # Unregister

# With type prefixes
clade foo -t spike                # spike/foo (throwaway)
clade foo -t feature              # feat/foo (to merge)
clade foo -t bug                  # fix/foo (bug fix)
clade foo -t chore                # chore/foo (maintenance)
clade foo -t hotfix               # hotfix/foo (urgent)
clade foo -t docs                 # docs/foo (documentation)
clade foo -t perf                 # Custom label from config

# Other commands
clade init                        # Setup .claude/ with hooks
clade scratch doc-review          # No-git scratch folder
clade status                      # Current context
clade doctor                      # Diagnose configuration issues
clade migrate                     # Migrate v1 -> v2 structure

# Batch mode (parallel ticket processing)
clade batch PROJ-1 PROJ-2                       # From args
clade batch --file tickets.csv                  # From CSV/text
clade batch --jira-label bug                    # Fetched from Jira by label
clade batch --jira-label bug --jira-project PROJ,OTHER --concurrency 3
clade batch status                              # List all batches
clade batch status <batch-id>                   # Detailed batch status
clade batch logs <batch-id> <ticket-id>         # Ticket log

# Useful flags
clade foo -p                      # Force repo picker
clade foo -r my-api               # Use specific repo
clade foo -b custom/name          # Custom branch name
clade foo -o cursor               # Open in Cursor IDE
clade foo -a claude               # Override agent
clade foo --no-agent              # Skip launching agent
clade foo --no-editor             # Skip opening editor
clade resume foo -o code          # Open VS Code on resume
clade --verbose list              # Enable debug output

# Experimental
clade --experimental project foo  # Multi-repo workspace
```

---

## See Also

- [ARCHITECTURE.md](ARCHITECTURE.md) - Go project structure, design patterns, implementation details
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) - Common issues and solutions
- [README.md](README.md) - Quick overview and installation
