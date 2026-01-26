# Clade

A CLI that manages git worktrees and context for AI coding sessions.

Named after biological clades (branching groups sharing common ancestry) - perfect metaphor for worktree branches.

## Why Clade?

1. **Worktree friction** - `git worktree add ../path -b branch` is verbose. Clade makes it one command.
2. **Context loss** - Switch tasks, come back tomorrow, Claude has no idea where you left off. Clade preserves context via SessionStart hooks.
3. **Works with Claude Code AND Cursor** - Both support the same hook protocol for context injection.

## Install

### Homebrew (Recommended)

```bash
brew install daniil-lyalko/tap/clade
```

Homebrew installation includes shell completions automatically.

### Prebuilt Binaries

Download from [GitHub Releases](https://github.com/daniil-lyalko/clade/releases) for your platform:
- macOS (Intel): `clade_VERSION_darwin_amd64.tar.gz`
- macOS (Apple Silicon): `clade_VERSION_darwin_arm64.tar.gz`
- Linux (x64): `clade_VERSION_linux_amd64.tar.gz`
- Linux (ARM): `clade_VERSION_linux_arm64.tar.gz`

```bash
# Extract and install
tar -xzf clade_*.tar.gz
sudo mv clade /usr/local/bin/
clade --version
```

### From Source

Requires Go 1.22 or higher:

```bash
go install github.com/daniil-lyalko/clade/cmd/clade@latest
```

Or build locally:

```bash
git clone https://github.com/daniil-lyalko/clade.git
cd clade
make install
```

## Shell Completion

### Homebrew Installation
Completions are installed automatically! Just reload your shell:
```bash
exec zsh  # or: exec bash
```

### Manual Installation
If installed via other methods, generate completions:

```bash
# Zsh
clade completion zsh > ~/.zsh/completions/_clade
# Add to ~/.zshrc if needed: fpath=(~/.zsh/completions $fpath)
exec zsh

# Bash
clade completion bash > /etc/bash_completion.d/clade
exec bash

# Fish
clade completion fish > ~/.config/fish/completions/clade.fish
```

### Quick Navigation Helper (Optional)

Source the `ccd` (clade cd) helper for quick worktree switching:

```bash
# Via Homebrew
echo 'source $(brew --prefix)/opt/clade/scripts/clade-cd.sh' >> ~/.zshrc
exec zsh

# Manual
echo 'source /path/to/clade/scripts/clade-cd.sh' >> ~/.zshrc
exec zsh

# Usage
ccd foo           # Jump to worktree 'foo'
ccd               # Interactive picker
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

## Supported AI Tools

- **Claude Code** (CLI) - Full hook integration via `.claude/settings.json`
- **Cursor** (IDE) - Full hook integration via `.cursor/hooks.json`
- **Other agents** - Worktree management only (no context injection)

## How It Works

### Context Injection

When you run `clade init`, it creates hook configurations for both Claude Code and Cursor:

**Claude Code** (`.claude/settings.json`):
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

**Cursor** (`.cursor/hooks.json`):
```json
{
  "version": 1,
  "hooks": {
    "sessionStart": [{
      "command": "clade inject-context --json"
    }]
  }
}
```

When you start a session, the hook automatically injects:
- **DROPBAG** - Most recent session notes with age + staleness warnings
- **Git status** - What's changed
- **Recent commits** - What you've done
- **TODOs** - Open tasks in code

### The /drop Command

`clade init` creates `.claude/commands/drop.md` which tells the AI to write timestamped session summaries to `.clade/dropbags/DROPBAG-{timestamp}.md`. This preserves full session history and prevents context loss.

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
