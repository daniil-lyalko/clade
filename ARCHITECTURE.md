# Clade Architecture

This document covers the internal architecture, design decisions, and implementation details of Clade. For usage instructions, see [USER_GUIDE.md](USER_GUIDE.md).

---

## Table of Contents

- [High-Level Design](#high-level-design)
- [Project Structure](#project-structure)
- [Key Packages](#key-packages)
- [State Management](#state-management)
- [Agent-Agnostic Design](#agent-agnostic-design)
- [Claude Code Integration](#claude-code-integration)
- [Dependencies](#dependencies)
- [Version History](#version-history)
- [Design Principles](#design-principles)

---

## High-Level Design

```
+-----------------------------------------------------------------+
|                           CLADE                                 |
+-----------------------------------------------------------------+
|  Worktree Management          |  Context Management             |
|  -------------------------    |  ----------------------------   |
|  o feature/bug/spike/etc      |  o inject-context (for hooks)   |
|  o project (multi-repo)       |  o Generates .claude/ config    |
|  o scratch (no-git)           |  o Tracks state                 |
|  o cleanup / list / resume    |  o Lifecycle hooks              |
+-----------------------------------------------------------------+
|  Agent (AI)                   |  Editor (IDE)                   |
|  o Default: claude            |  o cursor, code, nvim           |
|  o Has hooks, context inject  |  o Opens before agent launches  |
|  o Override with --agent      |  o Override with --open         |
+-----------------------------------------------------------------+
                                |
                                v
+-----------------------------------------------------------------+
|                    Claude Code (or other agent)                 |
+-----------------------------------------------------------------+
|  SessionStart Hook -> calls `clade inject-context`              |
|  /drop command -> writes DROPBAG.md                             |
|  JIRA MCP -> fetches ticket (if referenced)                     |
|  Sessions -> --resume, --continue                               |
+-----------------------------------------------------------------+
```

**Key principle:** Clade manages worktrees and generates context. Claude's hooks do the injection. Claude's MCP fetches external data. Clean separation.

---

## Project Structure

```
clade/
+-- cmd/
|   +-- clade/
|       +-- main.go                 # Entry point, cobra setup
+-- internal/
|   +-- cmd/                        # Command implementations
|   |   +-- init.go                 # clade init
|   |   +-- worktree.go             # Shared worktree creation logic
|   |   +-- work.go                 # clade work (primary command)
|   |   +-- helpers.go              # Shared helpers (repo resolution, session launch)
|   |   +-- repo.go                 # clade repo add/list/remove
|   |   +-- project.go              # clade project
|   |   +-- scratch.go              # clade scratch
|   |   +-- list.go                 # clade list
|   |   +-- status.go               # clade status
|   |   +-- resume.go               # clade resume
|   |   +-- cleanup.go              # clade cleanup
|   |   +-- migrate.go              # clade migrate (v1 -> v2)
|   |   +-- inject.go               # clade inject-context
|   |   +-- doctor.go               # clade doctor
|   +-- git/                        # Git operations
|   |   +-- worktree.go
|   |   +-- branch.go
|   |   +-- status.go
|   +-- context/                    # Context generation
|   |   +-- dropbag.go              # Read DROPBAG.md
|   |   +-- todos.go                # Find TODOs in code
|   |   +-- format.go               # Format context output
|   +-- config/                     # Configuration
|   |   +-- config.go               # User config (incl. custom labels)
|   |   +-- state.go                # State management (v2 format)
|   |   +-- trust.go                # Hook trust registry
|   +-- hooks/                      # Lifecycle hooks
|   |   +-- hooks.go                # Hook execution, YAML parsing
|   +-- files/                      # File operations
|   |   +-- copy.go                 # Safe file copying (symlink-aware)
|   +-- util/                       # Shared utilities
|   |   +-- util.go                 # extractTicket, writeJSON, etc.
|   +-- agent/                      # Agent launching
|   |   +-- agent.go                # Interface
|   |   +-- claude.go               # Claude-specific
|   |   +-- generic.go              # Generic (cursor, code, etc.)
|   +-- ui/                         # Terminal UI
|       +-- prompt.go               # Interactive prompts
|       +-- colors.go               # Output formatting
|       +-- spinner.go              # Progress indicators
+-- templates/                      # Generated file templates
|   +-- settings.json.tmpl          # .claude/settings.json
|   +-- drop.md.tmpl                # .claude/commands/drop.md
|   +-- gitignore.tmpl              # Lines to add to .gitignore
+-- go.mod
+-- go.sum
+-- Makefile
+-- README.md
+-- CLAUDE.md                       # AI context (this project's own)
```

---

## Key Packages

### `internal/cmd`

All Cobra command implementations. Each file typically contains:
- Command definition with `Use`, `Short`, `Long`, `RunE`
- `init()` function registering with parent command
- Private helper functions

### `internal/git`

Git operations abstracted from commands:
- **branch.go**: Branch validation, creation, checking
- **worktree.go**: Worktree creation, removal, listing
- **status.go**: Git status parsing, recent commits

**Security:** Branch validation prevents command injection by rejecting branch names with shell metacharacters or path traversal patterns (`..`).

### `internal/config`

Configuration management:
- **config.go**: User config (`~/.config/clade/config.json`), custom labels, repo settings
- **state.go**: State tracking (`~/clade/state.json`), v2 format with repo-nested worktrees
- **trust.go**: Hook trust registry, SHA-256 hash verification for per-repo hooks

### `internal/hooks`

Lifecycle hook system:
- Loads global hooks from `~/.config/clade/hooks.yaml`
- Loads per-repo hooks from `{repo}/.clade/hooks.yaml`
- Runs hooks with environment variables (`CLADE_TYPE`, `CLADE_NAME`, etc.)
- **Security:** Per-repo hooks require explicit trust (TOFU model)

### `internal/context`

Context generation for `inject-context`:
- **dropbag.go**: Finds most recent DROPBAG, formats with age
- **todos.go**: Scans code for TODO/FIXME comments
- **format.go**: Combines context sections into output

### `internal/files`

File operations with security considerations:
- **copy.go**: Symlink-aware file copying, refuses to follow symlinks
- Protects against symlink attacks during worktree setup

### `internal/ui`

Terminal output:
- **colors.go**: Colored output functions, debug mode
- **prompt.go**: Interactive prompts using promptui
- **spinner.go**: Progress indicators

---

## State Management

### State v2 Format

```json
{
  "version": 2,
  "worktrees": {
    "my-api": {
      "try-redis": {
        "name": "try-redis",
        "label": "spike",
        "branch": "spike/try-redis",
        "ticket": null,
        "created": "2025-01-06T10:00:00Z",
        "last_used": "2025-01-06T14:30:00Z"
      },
      "PROJ-1234": {
        "name": "PROJ-1234",
        "label": "bug",
        "branch": "fix/PROJ-1234",
        "ticket": "PROJ-1234",
        "created": "2025-01-03T09:00:00Z",
        "last_used": "2025-01-03T17:00:00Z"
      }
    }
  },
  "projects": {
    "api-integration": {
      "name": "api-integration",
      "path": "~/clade/projects/api-integration",
      "branch": "feat/PROJ-5678/api-integration",
      "repos": [
        {"name": "backend", "source": "~/repos/my-api"},
        {"name": "package", "source": "~/repos/my-package"},
        {"name": "admin-ui", "source": "~/repos/my-frontend"}
      ],
      "created": "2025-01-05T11:00:00Z",
      "last_used": "2025-01-06T16:00:00Z"
    }
  },
  "scratches": {
    "doc-review": {
      "name": "doc-review",
      "path": "~/clade/scratch/doc-review",
      "ticket": null,
      "created": "2025-01-07T09:00:00Z",
      "last_used": "2025-01-07T14:00:00Z"
    }
  }
}
```

**v2 changes from v1:**
- Worktrees nested by repo name (not flat map)
- Each worktree has `label` field (spike, feature, bug, etc.)
- Migration command handles v1 -> v2 upgrade

### Trust Registry

```json
{
  "repos": {
    "/Users/user/repos/my-api": {
      "hash": "sha256:abc123...",
      "trusted_at": "2025-01-06T10:00:00Z"
    }
  }
}
```

Stored at `~/.config/clade/trusted-repos.json`. Per-repo hooks require trust on first use; re-prompts if hook file hash changes.

---

## Agent-Agnostic Design

```go
// internal/agent/agent.go
type Agent interface {
    Launch(workdir string, opts LaunchOptions) error
    Name() string
}

type LaunchOptions struct {
    AddDirs []string  // Additional directories
    Flags   []string  // Extra flags
}

// internal/agent/claude.go
type ClaudeAgent struct{}

func (c *ClaudeAgent) Launch(workdir string, opts LaunchOptions) error {
    args := []string{}
    for _, dir := range opts.AddDirs {
        args = append(args, "--add-dir", dir)
    }
    args = append(args, opts.Flags...)

    cmd := exec.Command("claude", args...)
    cmd.Dir = workdir
    cmd.Stdin = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    return cmd.Run()
}

// internal/agent/generic.go
type GenericAgent struct {
    Command string  // "cursor .", "code .", etc.
}

func (g *GenericAgent) Launch(workdir string, opts LaunchOptions) error {
    parts := strings.Fields(g.Command)
    cmd := exec.Command(parts[0], parts[1:]...)
    cmd.Dir = workdir
    return cmd.Run()
}
```

Claude users get hooks + context injection. Other agents just get worktree management.

---

## Claude Code Integration

| Feature | How Clade Uses It |
|---------|-------------------|
| **SessionStart hook** | Calls `clade inject-context` |
| **Custom commands** | `/drop` -> DROPBAG.md |
| **MCP (JIRA)** | Claude fetches ticket, not clade |
| **Sessions** | Clade tracks but Claude manages |
| **--add-dir** | For multi-repo projects |

---

## Dependencies

```go
// go.mod
module github.com/daniil-lyalko/clade

go 1.22

require (
    github.com/spf13/cobra v1.8.0           // CLI framework
    github.com/fatih/color v1.16.0          // Terminal colors
    github.com/manifoldco/promptui v0.9.0   // Interactive prompts
    gopkg.in/yaml.v3 v3.0.1                 // YAML parsing for hooks
)
```

Keep it minimal. No heavy frameworks.

---

## Version History

### v0.1 (MVP)
- `clade exp` - Throwaway experiments
- `clade feat` - Features to merge
- `clade repo add/list/remove` - Register repos
- `clade list` - See what's active
- `clade cleanup` - Remove experiments
- `clade inject-context` - Makes hooks work
- `clade init` - Generate .claude/ config
- `clade resume` - Smart navigation
- `clade scratch` - No-git scratch folders
- `clade project` - Multi-repo workspaces

### v0.3
- **Repo-centric structure** - `~/clade/repos/{repo}/{name}/` instead of `experiments/`
- **Label system** - feature, bug, spike, chore, hotfix, docs + custom labels
- **Lifecycle hooks** - on_create, on_resume, on_remove via YAML
- **State v2 format** - Worktrees nested by repo
- **Migration command** - `clade migrate` for v1 -> v2
- **Deprecate exp/feat** - Use spike/feature instead
- **--agent flag** - Override configured agent

### v0.3.1
- **Unified `work` command** - Consolidated 6 label commands into one `work` command with `-t/--type` flag
- **Cleaner CLI** - `clade work foo -t spike` instead of `clade spike foo`
- **Updated deprecations** - `exp` -> `work -t spike`, `feat` -> `work`

### v0.4
- **Root shortcut** - `clade foo` = `clade work foo` (even simpler)
- **No branch prefix by default** - `clade foo` creates branch `foo`, not `feat/foo`
- **First-run wizard** - Prompts for Claude Code / Cursor / Both / Neither
- **Project hidden** - `clade project` requires `--experimental` flag
- **Cursor compatibility** - Works with both Claude Code AND Cursor (both support hooks)

### v0.5 (Planned)
- Ticket detection (JIRA pattern matching)
- Stale worktree warnings
- Session tracking (read ~/.claude/projects/)

---

## Design Principles

### 1. Speed First
`clade exp` should take <2 seconds to create worktree and launch. No network calls on the hot path.

### 2. Context Works
SessionStart hook fires, Claude sees DROPBAG.md. This is the core value proposition.

### 3. Scannable Output
`clade list` shows concise, grouped output. Not walls of text.

### 4. Handle Edge Cases
Cleanup handles uncommitted changes, merged branches. Doctor diagnoses common issues.

### 5. Zero Config Start
Works with defaults. Wizard guides first-time setup.

### 6. Security Conscious
- Branch name validation (no command injection)
- Symlink-safe file copying
- Hook trust verification (TOFU model)
- Restrictive file permissions (0600 for sensitive files)

---

## Non-Goals

- GUI or TUI (CLI only)
- MCP connections (let Claude handle it)
- Billing/usage tracking
- Replacing Claude Code
- IDE integrations beyond hooks
- Windows support (v1)

---

## See Also

- [USER_GUIDE.md](USER_GUIDE.md) - User-facing documentation
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) - Common issues and solutions
- [README.md](README.md) - Quick overview
