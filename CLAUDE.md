# CLAUDE.md - Clade: Claude Code Workflow CLI

Clade is a Go CLI that manages git worktrees and context for AI coding sessions. Named after biological clades (branching groups sharing common ancestry) - perfect metaphor for worktree branches.

**Core insight:** Claude Code already has hooks, sessions, MCP. Clade doesn't replace these - it orchestrates worktrees and context so Claude's built-in features work better.

---

## Documentation

| Document | Purpose |
|----------|---------|
| **[USER_GUIDE.md](USER_GUIDE.md)** | Commands, configuration, workflows |
| **[ARCHITECTURE.md](ARCHITECTURE.md)** | Go project structure, design patterns |
| **[TROUBLESHOOTING.md](TROUBLESHOOTING.md)** | Common issues and solutions |
| **[README.md](README.md)** | Quick overview and installation |

---

## Quick Reference

```bash
# Create worktrees
clade foo                         # Branch: foo (no prefix)
clade foo -t spike                # Branch: spike/foo (throwaway)
clade foo -t feature              # Branch: feat/foo (to merge)
clade foo -t bug                  # Branch: fix/foo (bug fix)

# Manage work
clade list                        # What's active
clade resume foo                  # Get back to work
clade cleanup foo                 # Clean up when done
clade status                      # Current context

# Repo management
clade repo add ~/repos/my-repo    # Register a repo
clade repo list                   # Show registered repos
clade repo remove my-repo         # Unregister

# Setup & diagnostics
clade init                        # Setup .claude/ with hooks
clade doctor                      # Diagnose configuration issues

# Useful flags
clade foo -p                      # Force repo picker
clade foo -r my-api               # Use specific repo
clade foo -o cursor               # Open in Cursor IDE
clade foo --no-agent              # Skip launching agent
clade --verbose list              # Enable debug output
```

---

## Three Problems It Solves

1. **Worktree friction** - `git worktree add ../some-path -b some-branch` is verbose. You forget the syntax. Paths are manual. Clade simplifies: `clade try-redis -t spike`.

2. **Multi-repo coordination** - Building a feature across 3 repos? No native way to create matching branches. Clade's project command (experimental) creates unified workspaces.

3. **Context loss** - You're deep in a session, switch tasks or go home. Tomorrow, Claude has no idea where you left off. Clade's `/drop` command + SessionStart hook restores context automatically.

---

## How It Works

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
                                |
                                v
+-----------------------------------------------------------------+
|                    Claude Code (or other agent)                 |
+-----------------------------------------------------------------+
|  SessionStart Hook -> calls `clade inject-context`              |
|  /drop command -> writes DROPBAG.md                             |
|  JIRA MCP -> fetches ticket (if referenced)                     |
+-----------------------------------------------------------------+
```

**Key principle:** Clade manages worktrees and generates context. Claude's hooks do the injection. Claude's MCP fetches external data. Clean separation.

---

## Directory Structure

```
~/clade/                          # Everything clade creates
+-- repos/                        # Repo-centric worktrees
|   +-- my-api/
|   |   +-- try-redis/            # label: spike
|   |   +-- PROJ-1234/            # label: bug
|   +-- my-frontend/
|       +-- ui-redesign/          # label: feature
+-- projects/                     # Multi-repo workspaces
+-- scratch/                      # No-git scratch folders
+-- state.json                    # Clade's state tracking

~/.config/clade/
+-- config.json                   # User configuration
+-- hooks.yaml                    # Global lifecycle hooks
+-- trusted-repos.json            # Hook trust registry
```

---

## Getting Started

```bash
# Install
go install github.com/daniil-lyalko/clade/cmd/clade@latest

# First run wizard sets up agent preference
clade

# Or manually configure
clade repo add ~/repos/my-api
cd ~/repos/my-api
clade init
clade try-redis -t spike
```

For detailed usage, see [USER_GUIDE.md](USER_GUIDE.md).
