# Security Policy

## Reporting Vulnerabilities

If you discover a security vulnerability in Clade, please report it responsibly:

1. **Do not** open a public issue
2. Email the maintainer directly or use GitHub's private vulnerability reporting
3. Include steps to reproduce the issue
4. Allow reasonable time for a fix before public disclosure

## Security Design

### Configuration Files

- Config files are stored with 0600 permissions (owner read/write only)
- Located at `~/.config/clade/config.json`
- Never contain credentials or secrets by design

### Trust Registry

Clade uses a trust registry to track which repositories have trusted hooks:
- Located at `~/.config/clade/trusted-repos.json`
- Identifies repos by path and commit hash
- Prevents unauthorized hook execution

### Hook Execution

- Clade generates hook configurations for Claude Code and Cursor
- Hooks are stored in `.claude/` and `.cursor/` directories within repos
- Hook scripts call `clade inject-context` which only reads local files
- No network access or arbitrary code execution

### Files Copied to Worktrees

When creating worktrees, Clade copies configuration files from the source repo. Only files matching a strict allowlist are copied:

**Allowlisted patterns:**
- `.env*` (environment templates)
- `.claude/` and `.cursor/` (hook configs)
- `.nvmrc`, `.node-version`, `.python-version` (version managers)
- `.editorconfig`, `.prettierrc*`, `.eslintrc*` (editor/linter configs)
- `CLAUDE.md` (project context)

Files like `.git`, `node_modules`, and other directories are never copied.

### Agent Launch

- Clade can optionally launch AI agents (Claude Code, Cursor)
- By default, agents are launched **without** `--dangerously-skip-permissions`
- Users must manually add this flag to config if desired

## Best Practices

When using Clade:

1. **Review hooks before trusting** - When running `clade init` in a new repo, review what will be installed
2. **Don't commit secrets** - Clade's hook system injects context but doesn't filter sensitive data
3. **Use `.gitignore`** - Ensure `.claude/` and `.cursor/` directories are in `.gitignore` if they contain local-only config
4. **Update regularly** - Keep Clade updated for security fixes

## Supported Versions

| Version | Supported |
|---------|-----------|
| 0.5.x   | Yes       |
| < 0.5   | No        |

## Threat Model

Clade is designed for local development workflows. It:

- **Does** manage local files and git operations
- **Does** launch local processes (editors, agents)
- **Does not** access networks directly
- **Does not** store or transmit credentials
- **Does not** execute arbitrary remote code

The primary security boundary is the local filesystem and user permissions.
