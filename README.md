[![CI](https://github.com/daniil-lyalko/clade/actions/workflows/ci.yml/badge.svg)](https://github.com/daniil-lyalko/clade/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

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

# Scratch folders (no git)
clade scratch doc-review       # Create scratch for documents/analysis
clade scratch PROJ-1234        # Ticket investigation without code

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
| `-f`, `--from` | Base branch to create from (default: main/master) |
| `-r`, `--repo` | Use specific repo |
| `-b`, `--branch` | Custom branch name (override default) |
| `-p`, `--pick` | Force repo picker |
| `-o`, `--open` | Open editor (cursor, code, nvim) |
| `--no-agent` | Skip launching agent |
| `--dry-run` | Preview what would be created without making changes |

### Advanced Usage

Combine flags for complex workflows:

```bash
# Branch from develop instead of main
clade PROJ-123 -t feature -f develop

# Full example: spike from develop in specific repo
clade DEVOPS-5700-research -t spike -f develop -r my-api

# Custom branch name (ignore type prefix)
clade foo -b custom/branch-name

# Create worktree but open in different editor
clade foo -t feature -o code

# Dry run to preview without creating
clade foo -t spike --dry-run
```

## How It Works

When you run `clade init`, it creates hook configurations:

```
.claude/settings.json       # Claude Code SessionStart hook
.claude/commands/drop.md    # /drop command for Claude Code
.cursor/hooks.json          # Cursor sessionStart hook
.cursor/commands/drop.md    # /drop command for Cursor
.clade/hooks.yaml.example   # Template for lifecycle hooks
.gitignore                  # Auto-updated with .clade/ entries
```

On session start, the hook injects:
- **Previous session notes** (from `.clade/dropbags/`) with staleness warnings
- **Git status** and recent commits
- **Open TODOs** in code

> **Note**: If clade isn't installed, hooks fail silently and sessions continue normally.
> Team members without clade can still work in the repo.

### The `/drop` Command

Before stopping work, run `/drop` in Claude Code or Cursor:
1. Creates timestamped file in `.clade/dropbags/DROPBAG-YYYY-MM-DD-HHMM.md`
2. Auto-injected on next `clade resume` with staleness warnings (>2 days old)
3. Full session history preserved for reference

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

## Shell Completions

Homebrew installs completions automatically. For manual installs:

<details>
<summary>Zsh</summary>

```bash
echo 'source <(clade completion zsh)' >> ~/.zshrc
source ~/.zshrc
```
</details>

<details>
<summary>Bash</summary>

```bash
echo 'source <(clade completion bash)' >> ~/.bashrc
source ~/.bashrc
```
</details>

<details>
<summary>Fish</summary>

```bash
clade completion fish > ~/.config/fish/completions/clade.fish
```
</details>

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
