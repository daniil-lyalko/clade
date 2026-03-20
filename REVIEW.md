# Clade Repository Review

Independent review by a senior developer with no prior context on the project.

---

## 1. First Impressions (30 seconds)

### README Clarity: B-

The README opens well: *"A CLI that manages git worktrees and context for AI coding sessions."* I understood the concept within 15 seconds. The three-bullet "Why Clade?" section is effective.

However, the README undersells the product. The Quick Start section jumps straight into commands without establishing *what happens* when you run them. A new reader sees `clade foo` and thinks "why wouldn't I just use `git worktree add`?" The context injection story -- the actual differentiator -- is buried under "How It Works" halfway down the page. Leading with the hook-based session continuity would be stronger.

### Installation Friction: A-

Two options: `go install` one-liner or `git clone && make install`. Standard Go toolchain. No Docker, no NPM, no exotic deps. Reasonable. One concern: requires Go 1.24.3 (per go.mod) which is very recent. Many developers won't have it.

### Value Proposition: C+

Here's the problem: the value proposition is split across two documents. The README gives a thin explanation. The real pitch lives in CLAUDE.md, which is a 900-line internal architecture document that *also* doubles as user documentation, design spec, and implementation roadmap. A potential user would need to read hundreds of lines of CLAUDE.md to understand what clade actually does for them.

The three problems clade solves are legitimate, but only the third one (context loss across sessions) is genuinely novel. Worktree creation convenience alone doesn't justify a new tool.

---

## 2. Technical Review

### Code Quality and Organization: 6.5/10

**Strengths:**

- Clean package separation. `git/`, `config/`, `context/`, `hooks/`, `agent/`, `ui/` are logically isolated with clear responsibilities. No circular imports.
- The Cobra command structure is well-organized. Commands are individual files with consistent patterns.
- Error propagation is generally done correctly with `fmt.Errorf("...%w", err)` wrapping.
- The agent interface (`agent/agent.go:16-19`) is well-designed for extensibility.
- Backward compatibility is handled gracefully: v1-to-v2 state migration, deprecated commands that still function.

**Issues:**

1. **Zero test files.** Not a single `_test.go` in the entire codebase. This is the most significant technical concern. A CLI that manages git worktrees and state files without test coverage is fragile. The git operations, state serialization, hook execution, and migration logic are all untested.

2. **Bubble sort everywhere.** `root.go:227-233`, `root.go:439-445`, `root.go:457-463`, `list.go:110-112`, `resume.go` -- every sorting operation is a hand-rolled O(n^2) bubble sort. Go's standard library provides `sort.Slice`. The data sizes are small enough that performance doesn't matter, but it signals unfamiliarity with the standard library.

3. **Broken relative time formatting.** `context/dropbag.go:90`:
   ```go
   return string(rune('0'+mins/10)) + string(rune('0'+mins%10)) + " minutes ago"
   ```
   For minutes 2-9, this produces "02 minutes ago", "03 minutes ago" with spurious leading zeros. Same bug for hours 2-9 on line 97. For days (line 104), `string(rune('0'+days))` only handles single digits 0-6 -- but the guard `days < 7` saves it. All of this should just be `fmt.Sprintf("%d minutes ago", mins)`.

4. **TICKET.md path bug.** `context/format.go:153` checks `filepath.Join(".", "TICKET.md")` -- a relative path from the process working directory rather than the worktree directory being analyzed. Should use the `dir` parameter that was passed to `GatherContext`.

5. **Function registration hack.** `worktree.go:351-356` uses `RegisterCopyGitignoredFiles` to wire up a function from `exp.go` via an init function. The comment says "workaround for avoiding circular imports" but the actual reason is that file-copying logic is stuck in the deprecated `exp.go` file. The fix is to extract it to a proper package, not to use global function pointers.

6. **Global mutable flags.** Multiple files define package-level `var xxxFlag` variables (`exp.go:20-27`, `work.go:13-22`, `resume.go:18-24`). These are mutated by the dashboard's interactive `runInteractiveNew` on line 411: `workTypeFlag = types[idx]` with a deferred reset. This is fragile and would break under concurrency (even though a CLI is single-threaded, this is poor practice that makes testing harder).

7. **File permissions.** Config (`config.go:231`) and state files are written with `0644` (world-readable). These contain repo paths, branch names, and ticket IDs. Should be `0600`.

8. **Code duplication.** The deprecated `exp.go` (416 lines) duplicates significant logic that now lives in `worktree.go` and `work.go`. The deprecation warnings are good, but the dead code adds maintenance burden and confusion.

### Error Handling: Mixed

- Good: Most functions wrap errors with context. Non-critical failures (hook failures, editor open failures) are logged as warnings and don't block the main workflow.
- Bad: `files/copy.go:34-36` silently returns nil on git command failures. `context/format.go:153` uses the wrong working directory. Multiple places in `status.go` suppress errors with `_`. `migrate.go:316` silently ignores WriteFile errors.

### Dependencies: Lean and Justified

```
cobra v1.8.0        - CLI framework (standard choice)
fatih/color v1.16.0 - Terminal colors (standard choice)
promptui v0.9.0     - Interactive prompts (reasonable)
yaml.v3              - YAML parsing for hooks config (necessary)
```

Four direct dependencies. All are well-maintained, widely-used Go libraries. No bloat. This is done correctly.

One note: everything in `go.mod` is marked `// indirect`, which means the `require` block wasn't generated by `go mod tidy` in the standard way. This suggests manual editing of go.mod. Not harmful, but unusual.

### Cross-Platform Considerations

- **macOS and Linux:** Explicitly targeted. Config path migration from macOS's `~/Library/Application Support/` to `~/.config/` is handled (`config.go:127-163`).
- **Windows:** Explicitly a non-goal per CLAUDE.md. However, the code uses `sh -c` for hook execution (`hooks.go:152`) which would fail on Windows. There's no build constraint preventing compilation on Windows, which would produce a binary that partially works and silently fails on hooks. A build tag or runtime check would be appropriate.
- **Path handling:** Uses `filepath.Join` consistently. No hardcoded `/` separators.

---

## 3. Competitive Landscape

### Direct Competitors

**git-worktree itself**: The raw `git worktree add/remove/list` commands do the core job. Clade's convenience wrapper (`clade foo` vs `git worktree add ~/clade/repos/myrepo/foo -b foo`) is real but incremental.

**git-town, git-branchless**: Workflow tools that manage branches and worktrees with opinionated conventions. More mature, tested, and documented. However, they don't address AI context injection.

**tmux/tmuxinator/direnv**: Developers already use session management tools. Many have shell aliases or scripts that accomplish the worktree-creation-and-cd workflow in 2-3 lines.

### The Actual Differentiator

The *only* feature that existing tools don't provide is the **Claude Code/Cursor SessionStart hook integration** -- automatically injecting DROPBAG notes, git status, TODOs, and ticket context when an AI session starts. This is genuinely useful and novel.

Everything else (worktree creation, branch naming conventions, project management, scratch folders) is table stakes that could be replaced by shell aliases or existing tools.

### Is This a Real Pain Point?

**Context loss across AI sessions: Yes, definitely.** Anyone who uses Claude Code or Cursor heavily has experienced the "explain everything again" problem. The DROPBAG + inject-context pattern is a clever, practical solution.

**Worktree friction: Marginally.** `git worktree add` is verbose but infrequent. Most developers create a worktree a few times a week at most. The friction saved is real but small.

**Multi-repo coordination: Niche.** The `project` command is experimental and incomplete. Most teams use monorepos or have custom tooling for this.

---

## 4. Adoption Barriers

### What Would Stop a Developer From Using This?

1. **Requires Go toolchain to install.** No Homebrew tap, no prebuilt binaries, no curl-to-install script. The target audience (Claude Code / Cursor users) may not have Go installed. This is the biggest practical barrier.

2. **Opinionated directory structure.** Everything goes in `~/clade/`. Developers who already have worktree conventions or who use a different home directory layout are forced to adapt. The `base_dir` config exists but the directory hierarchy (`repos/{repo}/{name}/`) is rigid.

3. **Trust concerns.** The default agent flags include `--dangerously-skip-permissions` (`config.go:192`). Setting this as a default for new users is aggressive. This should require explicit opt-in.

4. **No tests = no confidence.** An OSS tool that manages git worktrees (destructive operations), executes shell hooks (security surface), and manipulates state files (data integrity) with zero test coverage is a hard sell for cautious developers.

5. **Claude/Cursor coupling.** The tool bills itself as a general worktree manager, but the unique value is tightly coupled to two specific AI tools. If you're not using Claude Code or Cursor, the value drops to "slightly nicer git worktree aliases."

6. **No CI/CD.** No GitHub Actions, no release pipeline, no automated testing, no linting in CI. The Makefile has `test` and `lint` targets but they don't run in automation.

### Documentation Gaps

- No `man` page or `--help` examples beyond the built-in Cobra help text.
- CLAUDE.md mixes architecture documentation with user documentation. These should be separate.
- No troubleshooting guide. What happens when a worktree creation fails mid-way? How do you recover corrupt state?
- The lifecycle hooks feature has no examples beyond the YAML snippet in CLAUDE.md.
- No changelog or release notes.

### Missing Features That Would Be Expected

- **`clade cd <name>`**: Quickly navigate to a worktree. `clade open` exists but requires `cd $(clade open foo)` which is awkward.
- **Tab completion for worktree names**: The Makefile installs zsh completions but it's unclear if subcommand arguments (worktree names) are completed.
- **Undo/rollback**: No way to recover from a failed `clade cleanup` or interrupted worktree creation.
- **Dry-run mode for destructive operations**: `cleanup` should support `--dry-run`.
- **`clade config`**: No way to view or edit config without manually opening the JSON file.

---

## 5. Honest Verdict

### Would I Use This?

**Not in its current state.** I use Claude Code daily and the context injection problem is real. But I'd solve it with a 50-line shell script that writes a `.claude/settings.json` with a SessionStart hook pointing to a simple context gatherer. I don't need the worktree management, branch naming conventions, or project orchestration to get the core value.

If clade were packaged as a Homebrew formula with prebuilt binaries, had test coverage, and the context injection could be used *without* the worktree management (i.e., `clade init` in any directory without requiring the full ~/clade/ structure), I'd reconsider.

### Would I Recommend It to My Team?

**No.** Zero test coverage for a tool that manipulates git worktrees and executes shell hooks is a non-starter for team adoption. The `--dangerously-skip-permissions` default is a red flag I'd have to explain. The Go toolchain requirement adds friction for frontend-heavy teams.

### Rating: **Niche Useful**

The core idea (session context injection for AI coding tools) is sound and addresses a genuine pain point. The implementation has reasonable architecture but lacks the polish needed for distribution: no tests, no CI, no prebuilt binaries, and too much surface area for the problem it's solving.

### Recommendations for Distribution Readiness

**Must-do:**
1. Add tests. At minimum: state serialization, git operations, context formatting, hook execution. Aim for 60%+ coverage on `config/`, `git/`, `context/`, and `hooks/` packages.
2. Set up CI with GitHub Actions (build, test, lint).
3. Create a GoReleaser config or equivalent for prebuilt binaries and Homebrew tap.
4. Remove `--dangerously-skip-permissions` from defaults. Let users opt in.
5. Fix the TICKET.md path bug and the relative time formatting issues.
6. Write config and state files with `0600` permissions.

**Should-do:**
7. Decouple context injection from worktree management. Let `clade init` work standalone in any repo.
8. Replace bubble sorts with `sort.Slice`.
9. Delete or properly isolate deprecated code in `exp.go`.
10. Split CLAUDE.md into architecture docs (for contributors) and user docs (for users).
11. Add a `--dry-run` flag to `cleanup`.

**Nice-to-have:**
12. Homebrew formula.
13. Shell function for `clade cd` (prints path, user wraps in `cd`).
14. Verbose/debug logging flag.
15. Configuration validation on load.
