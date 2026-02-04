[![CI](https://github.com/daniil-lyalko/pacer/actions/workflows/ci.yml/badge.svg)](https://github.com/daniil-lyalko/pacer/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

# Pacer

A CLI that manages git worktrees and context for AI coding sessions.

## See It in Action

```bash
$ pacer try-redis -t spike
  ✓ Created worktree: ~/pacer/repos/my-api/try-redis
  ✓ Branch: spike/try-redis
  ✓ Launching claude...

$ pacer resume try-redis
  # Context auto-injected: git status, recent commits, previous session notes

$ pacer cleanup try-redis
  ✓ Archived session context
  ✓ Worktree removed
```

## Quick Start

```bash
# First run - wizard asks about your AI tool (Claude Code / Cursor)
pacer foo

# That's it! Creates worktree, launches your agent
# See what's active
pacer list

# Resume tomorrow with full context
pacer resume foo

# Done? Clean up
pacer cleanup foo
```

## Install

**Homebrew (Recommended):**
```bash
brew install daniil-lyalko/tap/pacer
```

<details>
<summary>Other installation methods</summary>

**Prebuilt Binaries:**

Download from [GitHub Releases](https://github.com/daniil-lyalko/pacer/releases):
- macOS (Intel): `pacer_VERSION_darwin_amd64.tar.gz`
- macOS (Apple Silicon): `pacer_VERSION_darwin_arm64.tar.gz`
- Linux (x64): `pacer_VERSION_linux_amd64.tar.gz`

```bash
tar -xzf pacer_*.tar.gz
sudo mv pacer /usr/local/bin/
```

**From Source** (Go 1.22+):
```bash
go install github.com/daniil-lyalko/pacer/cmd/pacer@latest
```

</details>

## Why Pacer?

1. **Worktree friction** - `git worktree add ../path -b branch` is verbose. Pacer makes it one command.
2. **Context loss** - Switch tasks, come back tomorrow, Claude has no idea where you left off. Pacer preserves context via SessionStart hooks.
3. **Works with Claude Code AND Cursor** - Both support the same hook protocol for context injection.

## Commands

```bash
# Core workflow
pacer foo                      # Create worktree (branch: foo)
pacer foo -t spike             # Throwaway experiment (branch: spike/foo)
pacer foo -t feature           # Feature branch (branch: feat/foo)
pacer foo -t bug               # Bug fix (branch: fix/foo)
pacer list                     # See all worktrees
pacer resume foo               # Resume with context
pacer cleanup foo              # Clean up (archives session)

# Repo management
pacer repo add ~/repos/my-api  # Register a repo
pacer repo list                # Show registered repos

# Setup & diagnostics
pacer init                     # Setup hooks in current repo
pacer doctor                   # Diagnose configuration issues
pacer status                   # Show context for current dir
```

### Key Flags

| Flag | Description |
|------|-------------|
| `-t`, `--type` | Type/prefix (spike, feature, bug, chore, hotfix, docs) |
| `-f`, `--from` | Base branch to create from (default: main/master) |
| `-r`, `--repo` | Use specific repo |
| `-b`, `--branch` | Custom branch name (override default) |
| `-p`, `--pick` | Force repo picker |
| `--no-agent` | Skip launching agent |
| `-o`, `--open` | Open editor (cursor, code, nvim) |

### Advanced Usage

Combine flags for complex workflows:

```bash
# Branch from develop instead of main
pacer PROJ-123 -t feature -f develop

# Full example: spike from develop in specific repo
pacer DEVOPS-5700-research -t spike -f develop -r my-api

# Custom branch name (ignore type prefix)
pacer foo -b custom/branch-name

# Create worktree but open in different editor
pacer foo -t feature -o code

# Dry run to preview without creating
pacer foo -t spike --dry-run
```

## How It Works

When you run `pacer init`, it creates hook configurations:

```
.claude/settings.json       # Claude Code SessionStart hook
.claude/commands/drop.md    # /drop command for Claude Code
.cursor/hooks.json          # Cursor sessionStart hook
.cursor/commands/drop.md    # /drop command for Cursor
.pacer/hooks.yaml.example   # Template for lifecycle hooks
.gitignore                  # Auto-updated with .pacer/ entries
```

On session start, the hook injects:
- **Previous session notes** (from `.pacer/dropbags/`) with staleness warnings
- **Git status** and recent commits
- **Open TODOs** in code

> **Note**: If pacer isn't installed, hooks fail silently and sessions continue normally.
> Team members without pacer can still work in the repo.

### The `/drop` Command

Before stopping work, run `/drop` in Claude Code or Cursor:
1. Creates timestamped file in `.pacer/dropbags/DROPBAG-YYYY-MM-DD-HHMM.md`
2. Auto-injected on next `pacer resume` with staleness warnings (>2 days old)
3. Full session history preserved for reference

## Configuration

`~/.config/pacer/config.json`:

```json
{
  "base_dir": "~/pacer",
  "agent": "claude",
  "editor": "cursor",
  "repos": {}
}
```

## Documentation

| Document | Description |
|----------|-------------|
| [USER_GUIDE.md](USER_GUIDE.md) | Complete command reference, workflows |
| [TROUBLESHOOTING.md](TROUBLESHOOTING.md) | Common issues and solutions |
| [ARCHITECTURE.md](ARCHITECTURE.md) | Technical design, Go project structure |

## Supported AI Tools

- **Claude Code** (CLI) - Full hook integration
- **Cursor** (IDE) - Full hook integration
- **Other agents** - Worktree management only

## License

MIT
