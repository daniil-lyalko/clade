# CLI Naming Decision

**Status:** Parked — sleeping on it
**Date:** 2026-02-08

---

## Background

- Originally named `clade` (your idea, biological metaphor for branching ancestry)
- Renamed to `clade` after a coworker's suggestion
- Announced publicly as `clade`, but very small user base (~1 active coworker)
- Muscle memory keeps typing `clade` — fingers fight the rename daily

## The "Too Close to Claude" Concern

**Verdict: Overblown.** `clade` is a real English word (biology: a group sharing a common ancestor). Nobody confuses `git` with `gist` or `go` with `Google`. The phonetic similarity to "Claude" is actually a subtle mnemonic — this is a Claude Code workflow tool.

## Finalists

| Name | Chars | Typing feel | Conflict risk | Metaphor |
|------|-------|-------------|---------------|----------|
| **`clade`** | 5 | Muscle memory already trained | Low — no major CLI tool | Branching ancestry = worktrees |
| **`clade`** | 5 | Fighting muscle memory | Low | Rhythm/tempo = workflow pacing |
| **`hop`** | 3 | Ultra-fast | Medium — common word | Jump between contexts |

## Analysis

### `clade` (revert)
- **Pro:** Your fingers already type it. The metaphor is more accurate (worktrees ARE a clade — branches sharing common ancestry). You invented the name, it's yours.
- **Pro:** Unique word in CLI space. Easy to search, no conflicts.
- **Con:** Feels like backtracking after public announcement. (Counter: tiny user base, normal to iterate on names.)
- **Con:** 5 characters. (Counter: `cargo`, `xcode`, `flask` — 5 chars is fine.)

### `clade` (keep)
- **Pro:** Already announced. Coworker liked it. No rename work needed.
- **Con:** Muscle memory fights it. Weaker metaphor (pacing ≠ branching). You didn't come up with it.

### `hop` (fresh start)
- **Pro:** 3 characters, ultra-fast. Action verb that describes context-switching.
- **Con:** Common word, higher conflict risk with other packages. Loses the biological identity.

## Recommendation

**Revert to `clade`.** The strongest signal is that your fingers refuse to type `clade`. Muscle memory reflects genuine affinity. The rename cost is low (small user base, one afternoon of find-replace), and the biological metaphor is more accurate.

But no rush. Sleep on it.

## If Reverting: Migration Checklist

- [ ] Rename GitHub repo (`clade` → `clade`)
- [ ] Update `go.mod` module path
- [ ] Find-replace all import paths
- [ ] Update binary name in `cmd/`
- [ ] Update all docs (README, CLAUDE.md, USER_GUIDE, etc.)
- [ ] Update Homebrew formula
- [ ] Update goreleaser config
- [ ] Notify coworker(s)
- [ ] Update any MCP/hook references that use the binary name
