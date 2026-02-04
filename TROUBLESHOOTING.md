# Pacer Troubleshooting Guide

This guide helps diagnose and resolve common issues with Pacer.

---

## Table of Contents

- [Quick Diagnostics](#quick-diagnostics)
- [Common Issues](#common-issues)
  - [Not in a git repo](#not-in-a-git-repo)
  - [Hook not firing / No context injection](#hook-not-firing--no-context-injection)
  - [Agent not found](#agent-not-found)
  - [Worktree creation fails](#worktree-creation-fails)
  - [Symlink warnings during copy](#symlink-warnings-during-copy)
  - [Hook trust prompt appears repeatedly](#hook-trust-prompt-appears-repeatedly)
  - [State file corruption](#state-file-corruption)
  - [Orphaned worktrees](#orphaned-worktrees)
  - [Branch name validation errors](#branch-name-validation-errors)
- [Diagnostic Commands](#diagnostic-commands)

---

## Quick Diagnostics

Run these commands to quickly identify issues:

```bash
# Full diagnostic check
pacer doctor

# Enable verbose output for any command
pacer --verbose list
pacer --verbose resume my-worktree

# Test hook output manually
pacer inject-context

# Test Cursor JSON format
echo '{}' | pacer inject-context
```

---

## Common Issues

### Not in a git repo

**Error:**
```
Not in a git repository. Register repos with: pacer repo add <path>
```

**Cause:** You're running pacer from a directory that isn't a git repository, and no repo was specified.

**Solutions:**

1. **Navigate to a git repo first:**
   ```bash
   cd ~/repos/my-api
   pacer try-redis -t spike
   ```

2. **Register repos for use from anywhere:**
   ```bash
   pacer repo add ~/repos/my-api
   pacer repo add ~/repos/my-frontend

   # Then use from anywhere:
   pacer try-redis -t spike -r my-api
   ```

3. **Use the repo picker:**
   ```bash
   # If you have registered repos, pacer will show a picker
   pacer try-redis -t spike
   # Select repo:
   #   > my-api (last used)
   #     my-frontend
   ```

---

### Hook not firing / No context injection

**Symptoms:**
- Claude starts but doesn't show "Session context loaded from pacer"
- No DROPBAG.md or git status in context
- `/drop` command doesn't work

**Diagnosis:**

1. **Check if .claude/ exists:**
   ```bash
   ls -la .claude/
   # Should show settings.json and commands/
   ```

2. **Initialize if missing:**
   ```bash
   pacer init
   ```

3. **Verify settings.json content:**
   ```bash
   cat .claude/settings.json
   ```

   Should contain:
   ```json
   {
     "hooks": {
       "SessionStart": [{
         "matcher": "*",
         "hooks": [{
           "type": "command",
           "command": "pacer inject-context"
         }]
       }]
     }
   }
   ```

4. **Test inject-context manually:**
   ```bash
   pacer inject-context
   ```

   Should output context in Markdown format. If it fails, check:
   - Is pacer in your PATH? (`which pacer`)
   - Are there any error messages?

5. **For Cursor users, check .cursor/hooks.json:**
   ```bash
   cat .cursor/hooks.json
   ```

**Common fixes:**
- Run `pacer init` to regenerate hooks
- Ensure pacer binary is in PATH
- Restart Claude Code / Cursor after modifying hook files

---

### Agent not found

**Error:**
```
claude not found in PATH
```
or
```
Agent not found: cursor
```

**Diagnosis:**
```bash
pacer doctor
# Look for the "Agent" check
```

**Solutions:**

1. **Verify agent is installed:**
   ```bash
   which claude    # For Claude Code
   which cursor    # For Cursor
   which code      # For VS Code
   ```

2. **Add to PATH if installed but not found:**
   ```bash
   # Add to ~/.zshrc or ~/.bashrc:
   export PATH="$PATH:/path/to/agent/binary"
   ```

3. **Update pacer config with correct path:**
   ```bash
   # Edit ~/.config/pacer/config.json
   {
     "agent": "/full/path/to/claude"
   }
   ```

4. **Or skip agent launch:**
   ```bash
   pacer try-redis -t spike --no-agent
   ```

---

### Worktree creation fails

**Common errors:**

**"fatal: 'branch-name' is already checked out"**
```
The branch is already checked out in another worktree.
```

**Solution:** Use a different branch name or clean up the existing worktree:
```bash
pacer list                    # Find existing worktree
pacer cleanup existing-name   # Remove it
pacer new-name -t spike       # Create with new name
```

**"Preparing worktree (new branch 'branch-name')... fatal: A branch named 'branch-name' already exists"**

**Solution:** The branch exists but isn't checked out anywhere. Either:
```bash
# Use the existing branch (adopt it)
pacer resume branch-name -r my-repo

# Or delete the branch and recreate
git branch -d branch-name
pacer branch-name -t spike
```

**"Invalid branch name"**

**Solution:** Branch names can only contain alphanumeric characters, `/`, `_`, `-`, and `.`. Avoid:
- Spaces
- Special characters (`;`, `&`, `|`, `$`, etc.)
- Path traversal (`..`)
- Starting with `-` or `.`

---

### Symlink warnings during copy

**Message:**
```
Skipping symlink: .env -> /etc/secrets/env
```

**Cause:** For security, pacer refuses to follow symlinks when copying files to worktrees. This prevents symlink attacks where a malicious repo could trick pacer into copying sensitive files.

**Solutions:**

1. **Replace symlink with actual file:**
   ```bash
   # In the source repo
   rm .env
   cp /etc/secrets/env .env
   ```

2. **Use hooks to handle symlinks:**
   ```yaml
   # ~/.config/pacer/hooks.yaml
   hooks:
     on_create:
       - cp -L "$PACER_REPO_PATH/.env" ./ 2>/dev/null || true
   ```
   The `-L` flag follows symlinks.

3. **Ignore the warning** if the symlinked file isn't needed in worktrees.

---

### Hook trust prompt appears repeatedly

**Symptom:** Every time you run pacer, it asks to trust repo hooks again.

**Causes:**

1. **Hook file changed:** The `.pacer/hooks.yaml` content changed, invalidating the trust hash.

   **Solution:** Review the changes and approve the new version:
   ```
   The hooks for this repository have changed.

   File: .pacer/hooks.yaml
   New hooks:
     on_create:
       - npm install
       - cp .env.example .env

   Trust these hooks? [y/N]: y
   ```

2. **Trust registry not writable:**
   ```bash
   ls -la ~/.config/pacer/trusted-repos.json
   # Check permissions
   ```

3. **CI/testing environment:** Set environment variable to skip trust:
   ```bash
   export PACER_TRUST_REPO_HOOKS=1
   pacer try-redis -t spike
   ```

   **Warning:** Only use this in trusted CI environments, never in production.

---

### State file corruption

**Symptoms:**
- `pacer list` shows incorrect worktrees
- Error parsing state.json
- Worktrees exist on disk but not in state

**Diagnosis:**
```bash
pacer doctor
# Look for "State file" check

# Inspect state file directly
cat ~/pacer/state.json | jq .
```

**Solutions:**

1. **Rebuild state from disk:**
   ```bash
   # Manual approach - inspect what exists
   ls ~/pacer/repos/*/

   # Remove state and let pacer rebuild
   rm ~/pacer/state.json
   pacer list  # Will show empty, then re-add
   ```

2. **Restore from backup (if using v1 migration):**
   ```bash
   cp ~/pacer/state.json.v1.backup ~/pacer/state.json
   ```

3. **Fix manually:**
   ```bash
   # Edit state.json to match reality
   vim ~/pacer/state.json
   ```

---

### Orphaned worktrees

**Symptoms:**
- Pacer shows worktree but directory doesn't exist
- Directory exists but `pacer list` doesn't show it
- Git worktree exists but pacer state is out of sync

**Diagnosis:**

```bash
# Compare what pacer knows vs what exists
pacer list
ls ~/pacer/repos/*/

# Compare with git's worktree tracking
cd ~/repos/my-api  # Your source repo
git worktree list
```

**Solutions:**

**Scenario 1: Pacer shows worktree but directory is missing**

The directory was deleted outside of pacer (manual `rm`, disk cleanup, etc.):

```bash
# Option A: Remove stale entry by resetting state
rm ~/pacer/state.json
pacer list  # Fresh start

# Option B: Recreate the worktree
pacer foo -t spike  # Will create it again
```

**Scenario 2: Directory exists but pacer doesn't know about it**

Worktree was created manually or state was corrupted:

```bash
# If you want to keep it, just resume it
# Pacer will "adopt" existing directories
pacer resume foo -r my-repo

# If you don't need it, remove manually
rm -rf ~/pacer/repos/my-repo/foo
git -C ~/repos/my-api worktree prune
```

**Scenario 3: Git worktree out of sync**

Git knows about worktrees that no longer exist:

```bash
# Clean up git's worktree tracking
cd ~/repos/my-api
git worktree list       # See what git thinks exists
git worktree prune      # Remove stale entries
```

**Prevention:**

Always use `pacer cleanup` instead of manually deleting worktree directories. This ensures:
1. Git worktree is properly removed
2. Pacer state is updated
3. Session context is archived for future reference

---

### Branch name validation errors

**Error:**
```
invalid branch name "my;branch": use alphanumeric, /, _, -, . only
```
or
```
invalid branch name "../../etc/passwd": cannot contain '..'
```

**Cause:** Pacer validates branch names to prevent command injection attacks. Certain characters are blocked.

**Allowed characters:**
- Letters (a-z, A-Z)
- Numbers (0-9)
- Forward slash (/) - for prefixes like `feat/`, `fix/`
- Underscore (_)
- Hyphen (-)
- Period (.)

**Not allowed:**
- Semicolon, ampersand, pipe, dollar sign, backticks
- Double dots (`..`) - prevents path traversal
- Starting with hyphen or period
- Spaces

**Solution:** Use a valid branch name:
```bash
# Bad
pacer "my feature" -t spike
pacer "test; rm -rf /" -t spike

# Good
pacer my-feature -t spike
pacer test-feature -t spike
```

---

## Diagnostic Commands

### `pacer doctor`

Comprehensive health check:

```bash
$ pacer doctor

Pacer Doctor

  ✓ Config file (config.json)
      agent: claude, editor: cursor
  ✓ State file
      v2, 5 worktree(s), 1 project(s), 2 scratch(es)
  ✓ Base directory
      /Users/user/pacer
  ✓ Git
      git version 2.43.0
  ✓ Agent
      claude found at /usr/local/bin/claude
  ✓ Repo: my-api
      /Users/user/repos/my-api
  ⚠ Repo: old-project
      path does not exist: /Users/user/repos/old-project
  ✓ Trust registry
      3 trusted repo(s)

⚠ All checks passed with 1 warning(s)
```

### `pacer --verbose`

Enable debug output for any command:

```bash
$ pacer --verbose list
  [debug] Loading config from /Users/user/.config/pacer/config.json
  [debug] Loading state from /Users/user/pacer/state.json
  [debug] Found 5 worktrees across 2 repos

my-api (3 worktrees)
  try-redis        spike         2h ago
  ...
```

### `pacer inject-context`

Test context injection manually:

```bash
# Plain text (Claude Code format)
$ pacer inject-context

# Session Context

## DROPBAG.md (from 2 hours ago)
We implemented basic caching...

## Git Status
On branch spike/try-redis
2 files modified

## Recent Commits
abc1234 - feat: add cache layer

# JSON format (Cursor format)
$ echo '{}' | pacer inject-context
{"additional_context":"# Session Context\n\n## DROPBAG.md..."}
```

### `pacer status`

Show context for current directory:

```bash
$ pacer status

Worktree: try-redis
  Repo: my-api
  Branch: spike/try-redis
  Type: spike

Context Files:
  ✓ CLAUDE.md (project context, 2 days ago)
  ✓ DROPBAG.md (session handoff notes, 2 hours ago)
  ○ TICKET.md (no ticket linked)

Git Status:
  2 uncommitted changes
    M src/cache.ts
    A src/redis-client.ts

Recent Commits:
  abc1234 - feat: add cache layer
  def5678 - chore: initial setup
```

---

## Getting Help

If you encounter an issue not covered here:

1. Run `pacer doctor` and include the output
2. Run the failing command with `--verbose`
3. Check the [GitHub Issues](https://github.com/daniil-lyalko/pacer/issues)
4. Open a new issue with:
   - Pacer version (`pacer --version`)
   - OS and version
   - Full command that failed
   - Error message or unexpected behavior
   - Doctor output
