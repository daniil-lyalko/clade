# Clade

A CLI that manages git worktrees and context for AI coding sessions.

Named after biological clades (branching groups sharing common ancestry) - perfect metaphor for worktree branches.

## Why Clade?

1. **Worktree friction** - `git worktree add ../path -b branch` is verbose. Clade makes it one command.
2. **Multi-repo coordination** - Building a feature across 3 repos? Clade creates matching branches and unified workspaces.
3. **Context loss** - Switch tasks, come back tomorrow, Claude has no idea where you left off. Clade preserves context via SessionStart hooks.

## Install

```bash
go install github.com/daniil-lyalko/clade/cmd/clade@latest
```

Or build from source:
```bash
git clone https://github.com/daniil-lyalko/clade.git
cd clade
make install
```

## Quick Start

```bash
# Register your repos
clade repo add ~/repos/my-project

# Create an experiment
clade exp try-redis

# Work with Claude... context is auto-injected via hooks
# Before stopping, use /drop to save session state

# Next day - resume with full context
clade resume try-redis

# Done? Clean up
clade cleanup try-redis
```

## Commands

| Command | Description |
|---------|-------------|
| `clade exp [name]` | Create isolated experiment worktree |
| `clade scratch [name]` | Create no-git scratch folder for docs/analysis |
| `clade project [name]` | Create multi-repo workspace |
| `clade init` | Setup SessionStart hooks in current repo |
| `clade list` | Show all active experiments/projects |
| `clade status` | Show context for current directory |
| `clade resume [name]` | Resume an experiment or project |
| `clade cleanup [name]` | Remove worktree and delete branch |
| `clade repo add/list/remove` | Manage registered repositories |

## How It Works

### Context Injection

When you run `clade init`, it creates `.claude/settings.json` with a SessionStart hook:

```json
{
  "hooks": {
    "SessionStart": [{
      "matcher": "*",
      "hooks": [{
        "type": "command",
        "command": "clade inject-context"
      }]
    }]
  }
}
```

When Claude starts, it automatically receives:
- **DROPBAG.md** - Your session notes from last time
- **Git status** - What's changed
- **Recent commits** - What you've done
- **TODOs** - Open tasks in code
- **Ticket info** - JIRA ticket if detected

### The /drop Command

`clade init` also creates `.claude/commands/drop.md` which tells Claude how to write a DROPBAG.md file with:
- Summary of what was accomplished
- Current state (working/broken)
- Next steps (specific actions)
- Key files to look at
- Open questions

### Directory Structure

```
~/clade/
├── experiments/              # Single-repo experiments
│   └── my-repo-try-redis/
├── scratch/                  # No-git scratch folders
│   └── doc-review/
└── projects/                 # Multi-repo workspaces
    └── api-integration/
        ├── backend/
        └── frontend/

~/.config/clade/
└── config.json               # Your preferences
```

## Configuration

`~/.config/clade/config.json`:

```json
{
  "base_dir": "~/clade",
  "agent": "claude",
  "agent_flags": [],
  "auto_init": true,
  "repos": {},
  "repo_settings": {}
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `base_dir` | `~/clade` | Where experiments/projects live |
| `agent` | `claude` | Command to launch (claude, cursor, code) |
| `agent_flags` | `[]` | Extra flags for agent |
| `auto_init` | `true` | Auto-setup .claude/ in new worktrees |
| `repos` | `{}` | Registered repos (name → path) |
| `repo_settings` | `{}` | Per-repo settings (copy_files, etc.) |

### Gitignored File Copying

When creating experiments/projects, clade detects gitignored files like `.env`, `.npmrc`, `.envrc` and prompts you to copy them:

```
Found gitignored files in source repo:
  .env
  .npmrc

Copy .env? [Y/n] y
Copy .npmrc? [Y/n] y

These preferences will be saved for future experiments from this repo.
```

Preferences are saved per-repo in `repo_settings`. Edit the config to change them.

## Multi-Repo Projects

```bash
clade project api-integration
# Prompts for:
#   - Branch name (shared across all repos)
#   - Repos to include
#   - Folder names in project

# Creates:
# ~/clade/projects/api-integration/
#   ├── backend/    (worktree from repo 1)
#   ├── frontend/   (worktree from repo 2)
#   └── shared/     (worktree from repo 3)
```

## License

MIT
