# CLAUDE.md - Pacer: Claude Code Workflow CLI

Pacer is a Go CLI that manages git worktrees and context for AI coding sessions. Named after biological pacers (branching groups sharing common ancestry) - perfect metaphor for worktree branches.

**Core insight:** Claude Code already has hooks, sessions, MCP. Pacer doesn't replace these - it orchestrates worktrees and context so Claude's built-in features work better.

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
pacer foo                         # Branch: foo (no prefix)
pacer foo -t spike                # Branch: spike/foo (throwaway)
pacer foo -t feature              # Branch: feat/foo (to merge)
pacer foo -t bug                  # Branch: fix/foo (bug fix)

# Scratch folders (no git)
pacer scratch doc-review          # Document analysis workspace
pacer scratch PROJ-1234           # Ticket investigation

# Manage work
pacer list                        # What's active
pacer resume foo                  # Get back to work
pacer cleanup foo                 # Clean up when done
pacer status                      # Current context

# Repo management
pacer repo add ~/repos/my-repo    # Register a repo
pacer repo list                   # Show registered repos
pacer repo remove my-repo         # Unregister

# Setup & diagnostics
pacer init                        # Setup .claude/ with hooks
pacer doctor                      # Diagnose configuration issues

# Useful flags
pacer foo -p                      # Force repo picker
pacer foo -r my-api               # Use specific repo
pacer foo -o cursor               # Open in Cursor IDE
pacer foo --no-agent              # Skip launching agent
pacer foo --dry-run               # Preview without creating
pacer --verbose list              # Enable debug output
```

---

## Three Problems It Solves

1. **Worktree friction** - `git worktree add ../some-path -b some-branch` is verbose. You forget the syntax. Paths are manual. Pacer simplifies: `pacer try-redis -t spike`.

2. **Multi-repo coordination** - Building a feature across 3 repos? No native way to create matching branches. Pacer's project command (experimental) creates unified workspaces.

3. **Context loss** - You're deep in a session, switch tasks or go home. Tomorrow, Claude has no idea where you left off. Pacer's `/drop` command + SessionStart hook restores context automatically.

---

## How It Works

```
+-----------------------------------------------------------------+
|                           PACER                                 |
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
|  SessionStart Hook -> calls `pacer inject-context`              |
|  /drop command -> writes DROPBAG.md                             |
|  JIRA MCP -> fetches ticket (if referenced)                     |
+-----------------------------------------------------------------+
```

**Key principle:** Pacer manages worktrees and generates context. Claude's hooks do the injection. Claude's MCP fetches external data. Clean separation.

---

## Directory Structure

```
~/pacer/                          # Everything pacer creates
+-- repos/                        # Repo-centric worktrees
|   +-- my-api/
|   |   +-- try-redis/            # label: spike
|   |   +-- PROJ-1234/            # label: bug
|   +-- my-frontend/
|       +-- ui-redesign/          # label: feature
+-- projects/                     # Multi-repo workspaces
+-- scratch/                      # No-git scratch folders
+-- state.json                    # Pacer's state tracking

~/.config/pacer/
+-- config.json                   # User configuration
+-- hooks.yaml                    # Global lifecycle hooks
+-- trusted-repos.json            # Hook trust registry
```

---

## Getting Started

```bash
# Install
go install github.com/daniil-lyalko/pacer/cmd/pacer@latest

# First run wizard sets up agent preference
pacer

# Or manually configure
pacer repo add ~/repos/my-api
cd ~/repos/my-api
pacer init
pacer try-redis -t spike
```

For detailed usage, see [USER_GUIDE.md](USER_GUIDE.md).
