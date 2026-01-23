# Clade

A CLI that manages git worktrees and context for AI coding sessions.

Named after biological clades (branching groups sharing common ancestry) - perfect metaphor for worktree branches.

## Why Clade?

1. **Worktree friction** - `git worktree add ../path -b branch` is verbose. Clade makes it one command.
2. **Context loss** - Switch tasks, come back tomorrow, Claude has no idea where you left off. Clade preserves context via SessionStart hooks.
3. **Works with Claude Code AND Cursor** - Both support the same hook protocol for context injection.

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
# First run - wizard asks about your AI tool (Claude Code / Cursor / Both / Neither)
clade foo

# That's it! Creates:
#   ~/clade/repos/{repo}/foo/
#   Branch: foo
#   Launches your configured agent/editor

# See what's active
clade list

# Resume tomorrow with full context
clade resume foo

# Done? Clean up
clade cleanup foo
```

## Commands

```bash
# Core workflow
clade foo                      # Create worktree (branch: foo)
clade foo -t spike             # Create with prefix (branch: spike/foo)
clade                          # Interactive dashboard
clade list                     # See all worktrees
clade resume foo               # Get back to work
clade cleanup foo              # Clean up

# With type prefixes
clade foo -t spike             # spike/foo (throwaway experiment)
clade foo -t feature           # feat/foo (to merge)
clade foo -t bug               # fix/foo (bug fix)
clade foo -t chore             # chore/foo (maintenance)

# Repo management
clade repo add ~/repos/my-repo # Register a repo
clade repo list                # Show registered repos

# Other
clade init                     # Setup hooks in current repo
clade scratch notes            # No-git scratch folder
clade status                   # Show context for current dir
```

### Flags

| Flag | Description |
|------|-------------|
| `-t`, `--type` | Type/prefix (spike, feature, bug, chore, hotfix, docs) |
| `-r`, `--repo` | Use specific repo |
| `-p`, `--pick` | Force repo picker |
| `-b`, `--branch` | Custom branch name |
| `-o`, `--open` | Open editor (cursor, code, nvim) |
| `-a`, `--agent` | Override agent |
| `--no-agent` | Skip launching agent |
| `--no-editor` | Skip opening editor |

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

When Claude/Cursor starts, it automatically receives:
- **DROPBAG.md** - Your session notes from last time
- **Git status** - What's changed
- **Recent commits** - What you've done
- **TODOs** - Open tasks in code

### The /drop Command

`clade init` also creates `.claude/commands/drop.md` which tells Claude how to write a DROPBAG.md with session state before you stop working.

## Configuration

On first run, clade asks about your AI tool and sets up config accordingly.

`~/.config/clade/config.json`:

```json
{
  "base_dir": "~/clade",
  "agent": "claude",
  "agent_flags": ["--dangerously-skip-permissions"],
  "editor": "cursor",
  "auto_init": true,
  "repos": {}
}
```

| Field | Description |
|-------|-------------|
| `agent` | AI agent command (claude, or empty) |
| `editor` | Editor/IDE (cursor, code, nvim, or empty) |
| `repos` | Registered repos (name → path) |

## Directory Structure

```
~/clade/
├── repos/                    # Worktrees grouped by repo
│   └── my-api/
│       ├── try-redis/
│       └── fix-bug/
└── scratch/                  # No-git scratch folders

~/.config/clade/
├── config.json               # Your preferences
└── hooks.yaml                # Lifecycle hooks (optional)
```

## License

MIT
