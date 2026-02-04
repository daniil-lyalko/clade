# Pacer User Guide

Pacer is a Go CLI that manages git worktrees and context for AI coding sessions. Named after biological pacers (branching groups sharing common ancestry) - perfect metaphor for worktree branches.

**Core insight:** Claude Code already has hooks, sessions, MCP. Pacer doesn't replace these - it orchestrates worktrees and context so Claude's built-in features work better.

**Three problems it solves:**

1. **Worktree friction** - `git worktree add ../some-path -b some-branch` is verbose. You forget the syntax. Paths are manual. You end up just stashing or making messy commits instead of properly isolating work.

2. **Multi-repo coordination** - You're building a feature across 3 repos. No native way to create matching branches, unified workspace, coordinated context. You're juggling terminal tabs and losing track.

3. **Context loss** - You're deep in a session, context built up over hours. You switch tasks or go home. Tomorrow, Claude has no idea where you left off. You waste 15 minutes re-explaining what you were doing.

Pacer fixes all three.

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
go install github.com/daniil-lyalko/pacer/cmd/pacer@latest
```

### First Run

On first use, pacer prompts you to choose your AI coding tool:

```
  Welcome to pacer! Let's set up your preferences.

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
pacer repo add ~/repos/my-api

# Create a worktree
cd ~/repos/my-api
pacer try-redis -t spike

# See what's active
pacer list

# Resume work
pacer resume try-redis

# Clean up when done
pacer cleanup try-redis
```

---

## For Cursor Users

If you're primarily a **Cursor IDE** user, pacer integrates seamlessly with Cursor's hook system to inject context into your AI conversations.

### Cursor-First Setup

```bash
# Install pacer
go install github.com/daniil-lyalko/pacer/cmd/pacer@latest

# First run - select "Cursor (IDE)"
pacer
#   What AI coding tool do you use?
#     Claude Code (terminal)
#   > Cursor (IDE)            <-- Select this
#     Both Claude Code and Cursor
#     Neither (just worktree management)
```

This configures pacer with:
- `editor`: `"cursor"` - Opens Cursor when creating/resuming worktrees
- `agent`: `""` (empty) - No separate terminal agent needed

### How It Works with Cursor

When you run `pacer init` in a repo, it creates **both** hook configurations:

```
.cursor/
  hooks.json              # Cursor's native hook system

.claude/
  settings.json           # Also created (Cursor reads these too)
  commands/
    drop.md               # /drop command works in Cursor
```

**Cursor's `sessionStart` hook** calls `pacer inject-context`, which:
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
pacer try-redis -t spike
# -> Creates worktree
# -> Opens Cursor in that directory
# -> Cursor's AI has full context injected

# Resume work tomorrow
pacer resume try-redis
# -> Opens Cursor
# -> SessionStart hook fires
# -> AI sees your DROPBAG from yesterday

# Or open in Cursor explicitly
pacer resume try-redis -o cursor
```

### Cursor + Claude Code Together

Some developers use both - Cursor for visual editing, Claude Code for terminal tasks. Select "Both" in the wizard:

```bash
pacer
#   What AI coding tool do you use?
#     Claude Code (terminal)
#     Cursor (IDE)
#   > Both Claude Code and Cursor  <-- Select this
```

This sets:
- `editor`: `"cursor"` - Opens Cursor first
- `agent`: `"claude"` - Then launches Claude Code in terminal

**Deduplication:** If both Cursor and Claude Code fire their hooks, pacer detects this and skips the second injection within 3 seconds.

### Cursor-Specific Config

```json
// ~/.config/pacer/config.json
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
echo '{}' | pacer inject-context
# {"additional_context":"# Session Context\n\n## Git Status\n..."}

# Check .cursor/hooks.json exists
cat .cursor/hooks.json
# {
#   "hooks": {
#     "sessionStart": [{
#       "command": "pacer inject-context"
#     }]
#   }
# }
```

### Cursor Troubleshooting

**Hook not firing?**
1. Ensure `.cursor/hooks.json` exists: `pacer init`
2. Restart Cursor after modifying hooks
3. Check Cursor settings: hooks must be enabled

**Context not appearing?**
1. Test manually: `echo '{}' | pacer inject-context`
2. Should return JSON with `additional_context` field
3. If plain text, pacer isn't detecting Cursor mode

**Using Cursor's "Third-party hooks" feature?**
Cursor may also read `.claude/settings.json`. Pacer handles this - if both hooks fire, the second is silently skipped (3-second dedup window).

---

## Directory Structure

### Pacer's Home: `~/pacer/`

```
~/pacer/                                    # Everything pacer creates lives here
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
+-- state.json                              # Pacer's state tracking
```

### Config: `~/.config/pacer/`

```
~/.config/pacer/
+-- config.json                             # User configuration
+-- hooks.yaml                              # Global lifecycle hooks
+-- trusted-repos.json                      # Hook trust registry
```

### Per-Worktree: `.claude/` (Generated by pacer init)

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
+-- .pacer/
|   +-- metadata.json                       # Pacer metadata (ticket, label, etc.)
+-- CLAUDE.md                               # Project context
```

### Hook Integration (Claude Code + Cursor)

Pacer supports both Claude Code CLI and Cursor IDE through their respective hook systems:

**Claude Code** (`.claude/settings.json`):
- Hook: `SessionStart` -> runs `pacer inject-context`
- Output: Plain text to stdout
- Context injected into conversation

**Cursor** (`.cursor/hooks.json`):
- Hook: `sessionStart` -> runs `pacer inject-context`
- Output: JSON with `{"additional_context": "..."}` (auto-detected)
- Context prepended to system prompt

Both receive:
- Most recent DROPBAG from last session (with age + staleness warnings)
- Git status and recent commits
- Open TODOs in the codebase
- Ticket information from .pacer.json

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
pacer inject-context

# Test Cursor format (simulates piped stdin -> JSON output)
echo '{}' | pacer inject-context
```

**DROPBAG Archives:**
- `/drop` creates timestamped files: `.pacer/dropbags/DROPBAG-2026-01-26-1430.md`
- Most recent file is auto-injected at SessionStart
- Staleness warning appears if >2 days old
- Full session history preserved (no auto-deletion)

---

## Commands

### `pacer init`

**Purpose:** Setup a repo for pacer (generates .claude/ config with hooks)

```bash
cd ~/repos/my-api
pacer init
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

.pacer/
+-- hooks.yaml.example    # Template for lifecycle hooks

# Also appends to .gitignore:
.pacer/
```

---

### `pacer repo add <path>`

**Purpose:** Register a repo for quick access from anywhere.

```bash
pacer repo add ~/repos/my-api
pacer repo add ~/repos/my-frontend
pacer repo add ~/repos/my-package
```

**What happens:**
- Validates path is a git repo
- Adds to `~/.config/pacer/config.json` repos list
- Uses directory name as short name (or specify with `--name`)

```bash
pacer repo add ~/repos/my-api --name backend
# Now you can use: pacer try-redis -r backend
```

---

### `pacer repo list`

**Purpose:** Show registered repos.

```bash
$ pacer repo list

Registered repos:
  my-api         ~/repos/my-api (last used)
  my-frontend    ~/repos/my-frontend
  my-package     ~/repos/my-package
```

---

### `pacer repo remove <name>`

**Purpose:** Unregister a repo.

```bash
pacer repo remove my-frontend
```

---

### Repo Selection Logic

When you run `pacer work` that needs a repo:

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
   NO  -> Error: "Not in a git repo. Register repos with: pacer repo add <path>"
```

**Examples:**

```bash
# In a repo - just works
cd ~/repos/my-api
pacer work try-redis -t spike

# Force picker even in a repo
pacer work try-redis -t spike -p

# Anywhere - specify repo
pacer work try-redis -t spike -r my-api
pacer work try-redis --repo ~/repos/my-api

# Anywhere - interactive picker
$ pacer work try-redis
Select repo:
  > my-api (last used)
    my-frontend
    my-package
```

---

### `pacer [name]` or `pacer work [name]`

**Purpose:** Create a new worktree for isolated development.

**Simplest usage:** `pacer foo` creates a worktree with branch name `foo` (no prefix).

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
pacer new-api                      # branch: new-api (no prefix)
pacer try-redis -t spike           # branch: spike/try-redis
pacer PROJ-123 -t bug              # branch: fix/PROJ-123
pacer cleanup -t chore             # branch: chore/cleanup
pacer foo -o cursor                # Open in Cursor IDE
pacer foo --no-agent               # Skip launching agent
pacer foo -a claude                # Override agent
```

**What happens:**
1. Creates `~/pacer/repos/{repo}/{name}/`
2. Branch: `{name}` or `{prefix}/{name}` (with `-t`)
3. Writes `.pacer.json` with metadata and label
4. Copies `.claude/` from main repo (or generates if missing)
5. Runs `on_create` hooks
6. Opens editor (if configured)
7. Launches agent (if configured)

---

### `pacer` / `pacer work` Flags

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

### `pacer project [name]` (EXPERIMENTAL)

> **Experimental:** Requires `--experimental` flag. May be removed in future versions.
> For most use cases, consider creating separate worktrees with the same branch name instead.

**Purpose:** Create multi-repo workspace with unified branch.

```bash
pacer --experimental project api-integration
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

Project ready: ~/pacer/projects/api-integration
Launching claude...
```

---

### `pacer list`

**Purpose:** Show all active worktrees, projects, and scratches.

```bash
$ pacer list

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

### `pacer status`

**Purpose:** Show context for current directory.

```bash
$ cd ~/pacer/repos/my-api/try-redis
$ pacer status

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

### `pacer resume [name]`

**Purpose:** Find worktree, cd there, launch agent. Hook handles context injection.

```bash
pacer resume                    # Interactive picker
pacer resume try-redis          # Specific worktree
pacer resume api-integration    # Project
pacer resume my-exp -o cursor   # Open in Cursor too
pacer resume my-exp --no-agent  # Skip launching Claude
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
6. SessionStart hook fires -> `pacer inject-context`
7. Claude sees DROPBAG.md, git status, TODOs, ticket info

**Adopting orphaned branches:**
```bash
# Resume searches both exp/ and feat/ prefixes
pacer resume my-feature -r my-repo

# If both exp/my-feature and feat/my-feature exist, prompts you to choose
# Or specify exact branch:
pacer resume my-feature -r my-repo --branch feat/my-feature
```

---

### `pacer scratch [name]`

**Purpose:** Create a no-git scratch folder for documents or analysis.

```bash
pacer scratch doc-review
pacer scratch PROJ-1234          # Ticket investigation (no code)
pacer scratch meeting-notes      # Temporary workspace
```

**Unlike worktrees, scratch folders:**
- Have no git repository or worktree
- Are for temporary document analysis, file sharing, etc.
- Still get `.claude/` config for hooks and context

**What happens:**
1. Creates `~/pacer/scratch/{name}/`
2. Writes `.pacer.json` with metadata
3. Initializes `.claude/` configuration
4. Launches agent (default: claude)

---

### `pacer cleanup [name]`

**Purpose:** Remove worktree and optionally delete branch.

```bash
pacer cleanup try-redis
```

**What happens:**
1. Checks for uncommitted changes (warns if present)
2. Checks if branch is merged (via git or gh CLI)
3. Removes worktree
4. Prompts to delete branch
5. Updates state.json

```bash
$ pacer cleanup try-redis

Worktree: try-redis
  Path: ~/pacer/repos/my-api/try-redis
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

### `pacer migrate`

**Purpose:** Migrate existing experiments from v1 to v2 repo-centric structure.

```bash
pacer migrate              # Show what would be migrated (dry-run)
pacer migrate --dry-run    # Explicit dry-run
pacer migrate --force      # Actually perform the migration
```

---

### `pacer doctor`

**Purpose:** Diagnose common configuration issues.

```bash
$ pacer doctor

Pacer Doctor

  ✓ Config file (config.json)
      agent: claude
  ✓ State file
      v2, 3 worktree(s), 0 project(s), 0 scratch(es)
  ✓ Base directory
      /Users/user/pacer
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

### `pacer inject-context`

**Purpose:** Called by SessionStart hook. Outputs context to stdout for the AI agent.

**Not user-facing** - the hook calls this automatically. Output format is auto-detected:
- **Terminal (Claude Code):** plain Markdown text
- **Piped stdin (Cursor):** JSON with `additional_context` field

---

## Configuration

### `~/.config/pacer/config.json`

```json
{
  "base_dir": "~/pacer",
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
| `base_dir` | `~/pacer` | Where worktrees/projects are stored |
| `agent` | (from wizard) | AI agent command (has hooks, context injection) |
| `agent_flags` | `[]` | Flags to pass to agent |
| `editor` | (from wizard) | Editor/IDE to open before agent (cursor, code, nvim) |
| `auto_init` | `true` | Auto-run `pacer init` on new worktrees |
| `repos` | `{}` | Registered repos (name -> path mapping) |
| `last_repo` | `null` | Last used repo (for picker default) |
| `copy_files` | `[]` | Extra files to copy to worktrees (beyond built-in defaults) |
| `custom_labels` | `{}` | Custom labels for `pacer -t <label>` |

**Agent vs Editor:**
- **Agent** = AI assistant (e.g., `claude`). Gets SessionStart hooks, context injection
- **Editor** = IDE (e.g., `cursor`, `code`, `nvim`). Opens before agent launches

Both can be used together. Editor opens first, then agent takes over the terminal.

---

### `~/.config/pacer/hooks.yaml` (Lifecycle Hooks)

Define scripts to run on pacer operations:

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
| `on_resume` | Entering via `pacer resume` | `direnv allow`, activate venv |
| `on_remove` | Before worktree deletion | Cleanup temp files, notify |

**Per-repo hooks:** Place `.pacer/hooks.yaml` in the repo root. Per-repo hooks run AFTER global hooks.

**Environment variables available to hooks:**
```bash
PACER_TYPE=spike|feature|bug|chore|hotfix|docs
PACER_NAME=try-redis
PACER_PATH=/Users/user/pacer/repos/my-api/try-redis
PACER_REPO_NAME=my-api
PACER_REPO_PATH=/Users/user/repos/my-api
PACER_BRANCH=spike/try-redis
PACER_TICKET=PROJ-1234  # if detected
```

**Automatic File Copying:**

When creating a worktree, pacer automatically copies common config files from the source repo (if they exist). These are files that are typically gitignored but needed to run the project:

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

**`~/pacer/state.json`** tracks all worktrees, projects, and scratches. See [ARCHITECTURE.md](ARCHITECTURE.md) for the v2 format specification.

**`.pacer/metadata.json`** (per-worktree) stores metadata about the worktree:

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

> **Migration note**: Legacy `.pacer.json` files are automatically migrated to `.pacer/metadata.json` on first access.

---

## Workflows

### Scenario 1: Quick Experiment

```bash
# You're in your main repo, want to try something
$ cd ~/repos/my-api
$ pacer try-redis -t spike

Creating spike: try-redis
  Path: ~/pacer/repos/my-api/try-redis
  Branch: spike/try-redis
Launching claude...

# Claude starts, SessionStart hook fires, you see:
# "Session context loaded from pacer"

# You work for a few hours...
# Before stopping:

> /drop

# Claude writes DROPBAG.md with your current state
# You exit claude, go home

# Next morning:
$ pacer resume try-redis

Resuming: try-redis
  Path: ~/pacer/repos/my-api/try-redis
Launching claude...

# SessionStart hook fires, Claude sees:
# - Your DROPBAG.md from yesterday
# - Git status showing your changes
# - Any TODOs in code
# You're immediately back in context

# Experiment worked! Clean up:
$ pacer cleanup try-redis --merge

# Or it failed:
$ pacer cleanup try-redis  # Just deletes everything
```

---

### Scenario 2: Ticket Investigation

```bash
$ pacer PROJ-1234 -t bug

Creating bug: PROJ-1234
  Path: ~/pacer/repos/my-api/PROJ-1234
  Branch: fix/PROJ-1234
  Ticket: PROJ-1234 (will prompt Claude to fetch)
Launching claude...

# Claude starts, sees in context:
# "Ticket PROJ-1234 detected. Please fetch from JIRA and save to TICKET.md"

# Claude uses JIRA MCP to fetch ticket, saves it
# Now you have full ticket context

# Investigate, fix, done:
$ pacer cleanup PROJ-1234
```

---

### Scenario 3: Multi-Repo Feature (Experimental)

> Note: For most use cases, just create separate worktrees with the same branch name.

```bash
$ pacer --experimental project api-integration

Project name: api-integration
Branch: feat/PROJ-5678/api-integration

Add repos:
  ~/repos/my-api -> backend
  ~/repos/my-package -> package
  ~/repos/my-frontend -> admin-ui

Creating project...
Launching claude...

# Claude sees all three repos in ~/pacer/projects/api-integration/
# You work across backend/, package/, admin-ui/

# End of day:
> /drop

# Next day:
$ pacer resume api-integration

# All context restored, all three repos ready
```

---

### Scenario 4: See What's Active

```bash
$ pacer list

my-api (3 worktrees)
  try-redis        spike         2h ago
  PROJ-1234        bug           3d ago  (stale)

Projects:
  api-integration - 1 day ago

# Oh right, I forgot about that old investigation
$ pacer cleanup PROJ-1234
```

---

## Quick Reference

```bash
# Simplified workflow (v0.4+)
pacer foo                         # Create worktree (branch: foo)
pacer foo -t spike                # Create spike (branch: spike/foo)
pacer                             # Interactive dashboard
pacer list                        # What's active
pacer resume foo                  # Get back to work
pacer cleanup foo                 # Clean up when done

# Repo management
pacer repo add ~/repos/my-repo    # Register a repo
pacer repo list                   # Show registered repos
pacer repo remove my-repo         # Unregister

# With type prefixes
pacer foo -t spike                # spike/foo (throwaway)
pacer foo -t feature              # feat/foo (to merge)
pacer foo -t bug                  # fix/foo (bug fix)
pacer foo -t chore                # chore/foo (maintenance)
pacer foo -t hotfix               # hotfix/foo (urgent)
pacer foo -t docs                 # docs/foo (documentation)
pacer foo -t perf                 # Custom label from config

# Other commands
pacer init                        # Setup .claude/ with hooks
pacer scratch doc-review          # No-git scratch folder
pacer status                      # Current context
pacer doctor                      # Diagnose configuration issues
pacer migrate                     # Migrate v1 -> v2 structure

# Useful flags
pacer foo -p                      # Force repo picker
pacer foo -r my-api               # Use specific repo
pacer foo -b custom/name          # Custom branch name
pacer foo -o cursor               # Open in Cursor IDE
pacer foo -a claude               # Override agent
pacer foo --no-agent              # Skip launching agent
pacer foo --no-editor             # Skip opening editor
pacer resume foo -o code          # Open VS Code on resume
pacer --verbose list              # Enable debug output

# Experimental
pacer --experimental project foo  # Multi-repo workspace
```

---

## See Also

- [ARCHITECTURE.md](ARCHITECTURE.md) - Go project structure, design patterns, implementation details
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) - Common issues and solutions
- [README.md](README.md) - Quick overview and installation
