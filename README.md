# Clade

A CLI that manages git worktrees and context for AI coding sessions.

## See It in Action

```bash
$ clade try-redis -t spike
  ✓ Created worktree: ~/clade/repos/my-api/try-redis
  ✓ Branch: spike/try-redis
  ✓ Launching claude...

$ clade resume try-redis
  # Context auto-injected: git status, recent commits, previous session notes

$ clade cleanup try-redis
  ✓ Archived session context
  ✓ Worktree removed
```

## Quick Start

```bash
# First run - wizard asks about your AI tool (Claude Code / Cursor)
clade foo

# That's it! Creates worktree, launches your agent
# See what's active
clade list

# Resume tomorrow with full context
clade resume foo

# Done? Clean up
clade cleanup foo
```

## Install

**Homebrew (Recommended):**
```bash
brew install daniil-lyalko/tap/clade
```

<details>
<summary>Other installation methods</summary>

**Prebuilt Binaries:**

Download from [GitHub Releases](https://github.com/daniil-lyalko/clade/releases):
- macOS (Intel): `clade_VERSION_darwin_amd64.tar.gz`
- macOS (Apple Silicon): `clade_VERSION_darwin_arm64.tar.gz`
- Linux (x64): `clade_VERSION_linux_amd64.tar.gz`

```bash
tar -xzf clade_*.tar.gz
sudo mv clade /usr/local/bin/
```

**From Source** (Go 1.22+):
```bash
go install github.com/daniil-lyalko/clade/cmd/clade@latest
```

</details>

## Why Clade?

1. **Worktree friction** - `git worktree add ../path -b branch` is verbose. Clade makes it one command.
2. **Context loss** - Switch tasks, come back tomorrow, Claude has no idea where you left off. Clade preserves context via SessionStart hooks.
3. **Works with Claude Code AND Cursor** - Both support the same hook protocol for context injection.

## Commands

```bash
# Core workflow
clade foo                      # Create worktree (branch: foo)
clade foo -t spike             # Throwaway experiment (branch: spike/foo)
clade foo -t feature           # Feature branch (branch: feat/foo)
clade foo -t bug               # Bug fix (branch: fix/foo)
clade list                     # See all worktrees
clade resume foo               # Resume with context
clade cleanup foo              # Clean up (archives session)

# Repo management
clade repo add ~/repos/my-api  # Register a repo
clade repo list                # Show registered repos

# Setup & diagnostics
clade init                     # Setup hooks in current repo
clade doctor                   # Diagnose configuration issues
clade status                   # Show context for current dir
```

### Key Flags

| Flag | Description |
|------|-------------|
| `-t`, `--type` | Type/prefix (spike, feature, bug, chore, hotfix, docs) |
| `-r`, `--repo` | Use specific repo |
| `-p`, `--pick` | Force repo picker |
| `--no-agent` | Skip launching agent |
| `-o`, `--open` | Open editor (cursor, code, nvim) |

## How It Works

When you run `clade init`, it creates hook configurations:

```
.claude/settings.json    # Claude Code hook
.cursor/hooks.json       # Cursor hook
.claude/commands/drop.md # /drop command for session notes
```

On session start, the hook injects:
- **Previous session notes** (DROPBAG.md) with staleness warnings
- **Git status** and recent commits
- **Open TODOs** in code

## Configuration

`~/.config/clade/config.json`:

```json
{
  "base_dir": "~/clade",
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
