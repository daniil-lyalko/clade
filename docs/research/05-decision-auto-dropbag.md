# ADR-001: Auto-DROPBAG Hook Architecture

**Status:** Decided
**Date:** 2026-02-07
**Participants:** Vasquez (AI ecosystem), Tanaka (CLI design), Chen (DevOps), Sharma (UX), + Anthropic engineer research
**Decision:** Stop hook (async) for auto-DROPBAG, PreCompact for user notification only

---

## Context

Pacer's DROPBAG system provides session continuity — a markdown snapshot of "where you left off" that gets injected into the next session via the `SessionStart` hook. Currently, DROPBAGs are created manually via the `/drop` command. Users forget to `/drop` before closing sessions, losing context.

The question: which Claude Code hook should auto-generate DROPBAGs?

## Options Considered

### Option A: `Stop` hook only
Write DROPBAG after every Claude response (async, debounced).

### Option B: `PreCompact` hook only
Write DROPBAG right before context window compression.

### Option C: Both hooks, same job
Both write DROPBAGs at different moments.

### Option D: Both hooks, different jobs (chosen)
`Stop` writes DROPBAGs. `PreCompact` notifies the user. Different responsibilities.

## Decision

**Option D: Stop writes, PreCompact notifies.**

### The Hook Architecture

```
Hook            Responsibility              Visibility     Async
─────────────   ──────────────────────────  ───────────    ─────
Stop            Write DROPBAG.md to disk    Silent         Yes
PreCompact      Notify user, suggest /drop  Calm stderr    No
SessionStart    Read DROPBAG + state        Injected       No
Manual /drop    Human-curated checkpoint    Explicit       No
```

### Stop Hook: The Write Mechanism

```json
{
  "hooks": {
    "Stop": [{
      "hooks": [{
        "type": "command",
        "command": "pacer auto-dropbag",
        "async": true,
        "timeout": 30
      }]
    }]
  }
}
```

The `pacer auto-dropbag` command:
1. Reads `transcript_path` from stdin JSON (provided by Claude Code)
2. Parses the transcript JSONL for: files modified, decisions made, errors, current task state
3. Debounces: skips if last write was <60 seconds ago AND no git changes detected
4. Writes structured `DROPBAG.md` to `.pacer/dropbags/`
5. Exits silently on any error (safety net, not blocking operation)

**Why async:** `async: true` means the hook never blocks user interaction and avoids the [infinite loop footgun](https://github.com/thedotmack/claude-mem/issues/987) where Stop hook output triggers another response.

**Why debounced:** Stop fires on every response. Most responses don't warrant a snapshot. The debounce (60s + meaningful-changes-only) reduces writes from dozens per session to ~5-10 meaningful checkpoints.

### PreCompact Hook: The Notification

```json
{
  "hooks": {
    "PreCompact": [{
      "hooks": [{
        "type": "command",
        "command": "pacer context-warning",
        "timeout": 5
      }]
    }]
  }
}
```

The `pacer context-warning` command:
1. Prints a calm message to stderr (shown to user):
   > "Context getting full. Auto-DROPBAG saved. Run /drop to add your notes, or `pacer resume` to start fresh with full context."
2. Does NOT write a DROPBAG (Stop hook already did)
3. Does NOT try to inject into post-compaction context (can't — no mechanism exists)
4. Completes in <1 second

### DROPBAG Priority at Session Start

When `inject-context` reads DROPBAGs, it picks the most recent by timestamp and labels the source:

```
[Developer notes] Mutex fix worked but broke idempotency...    (from /drop)
[Auto-saved] Modified handler.go, handler_test.go...           (from Stop hook)
```

Manual `/drop` DROPBAGs are always preferred when newer. Auto-DROPBAGs are the safety net.

---

## Rationale

### Why Stop is the primary write mechanism

| Argument | Source |
|----------|--------|
| Fires at natural semantic boundaries (Claude finished a coherent unit) | Vasquez |
| Fires frequently enough to capture incremental progress | Chen |
| `async: true` makes it zero-cost to user interaction | Vasquez |
| Only reliable persistence point — most sessions end before PreCompact fires | Chen |
| Anthropic's own pattern (`claude-progress.txt`) writes after each agent response | Anthropic engineering blog |

### Why PreCompact is NOT for writing DROPBAGs

| Argument | Source |
|----------|--------|
| Cannot inject stdout into post-compaction context (no mechanism exists) | Vasquez (hooks reference research) |
| Known bugs: empty `transcript_path` ([#13668](https://github.com/anthropics/claude-code/issues/13668)), not triggered on `/compact` ([#13572](https://github.com/anthropics/claude-code/issues/13572)) | Anthropic engineer research |
| Fires too rarely — once per multi-hour session, maybe never (Opus 4.6 has 1M context) | Vasquez |
| By the time it fires, context is already degraded (triggers at ~95% capacity) | Vasquez |
| If it's your only mechanism, sessions that end normally produce zero DROPBAGs | Chen |
| Creates false security — users let sessions run too long trusting the safety net | Vasquez |

### Why PreCompact IS valuable as notification

| Argument | Source |
|----------|--------|
| Silent compaction is "gaslighting as a feature" — tool degrades without user knowing | Sharma |
| A calm notification is a 3-second interruption that prevents a 30-minute recovery | Sharma |
| Gives user informed consent: keep going (accept degradation) or restart fresh (zero loss) | Sharma |
| Doesn't need to inject or write — just inform | — |

### Why manual `/drop` stays

| Argument | Source |
|----------|--------|
| The friction is the feature — 15 seconds of reflection saves 30 minutes tomorrow | Sharma |
| Captures intent, judgment, rejected approaches — things auto-DROPBAG can't know | Sharma |
| Auto-DROPBAG captures what happened; manual captures what you understood | Sharma |
| Should be faster and more templated (pre-populated with git diff, topics discussed) | Sharma |

---

## The "Avoid Compaction, Restart Fresh" Workflow

All reviewers and Anthropic's engineering blog agree: **restarting fresh with a DROPBAG is superior to letting compaction degrade the session.**

Anthropic's own guidance:
> *"Write important state to disk, not to conversation history. Context windows are ephemeral; files are persistent."*

The shift-handoff metaphor from Anthropic's ["Effective Harnesses for Long-Running Agents"](https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents):
> *"Like shift workers leaving clear handoff notes and a tidy workstation so the next person can pick up where they left off."*

**Recommended workflow:**
1. Work in focused sprints (60-90 minutes or 25-30 messages)
2. Stop hook auto-saves DROPBAGs continuously (async, debounced)
3. At natural stopping points, run `/drop` for a curated checkpoint
4. If PreCompact fires, take the hint — `/drop` and `pacer resume` for a fresh session
5. Fresh session gets clean 1M-token window + curated DROPBAG via SessionStart

**Tradeoff:** Fresh restarts lose conversational thread memory (rejected approaches, implicit preferences). Mitigated by the DROPBAG's "Open Questions" and "Decisions" sections capturing this explicitly.

---

## Implementation Plan

### New command: `pacer auto-dropbag`

Hidden command (like `inject-context`), called by the Stop hook.

```go
// Reads transcript_path from Stop hook JSON stdin
// Parses JSONL transcript for: tool calls, decisions, errors
// Writes structured DROPBAG.md
// Debounced: skips if <60s since last write AND no git changes
// Fast: must complete in <2s for the common case
```

### New command: `pacer context-warning`

Hidden command, called by the PreCompact hook.

```go
// Prints calm notification to stderr
// Does NOT write files (Stop hook handles that)
// Completes in <1s
```

### Updated `pacer setup`

Registers three hooks (currently only SessionStart):

```json
{
  "hooks": {
    "SessionStart": [{"matcher": "*", "hooks": [{"type": "command", "command": "pacer inject-context"}]}],
    "Stop": [{"hooks": [{"type": "command", "command": "pacer auto-dropbag", "async": true, "timeout": 30}]}],
    "PreCompact": [{"hooks": [{"type": "command", "command": "pacer context-warning", "timeout": 5}]}]
  }
}
```

### Updated `pacer inject-context`

Labels DROPBAG source when injecting:
- `[Developer notes]` for manual `/drop`
- `[Auto-saved]` for Stop hook auto-DROPBAGs
- Prefers manual when newer

---

## Dissenting Views

**Tanaka** argued for PreCompact as the only hook, with no Stop hook. His concern about Stop being a "noise factory" is valid but resolved by `async: true` + debouncing. His position was undermined by the technical finding that PreCompact cannot inject into post-compaction context and fires too rarely to be a reliable persistence mechanism.

**Chen** argued for PreCompact as an injection safety net (inject summary into post-compaction context). This was invalidated by Vasquez's research showing PreCompact has no injection mechanism and has known bugs with `transcript_path`.

---

## Sources

- [Effective context engineering for AI agents — Anthropic Engineering](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)
- [Effective harnesses for long-running agents — Anthropic Engineering](https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents)
- [Hooks reference — Claude Code Docs](https://code.claude.com/docs/en/hooks)
- [Automate workflows with hooks — Claude Code Docs](https://code.claude.com/docs/en/hooks-guide)
- [Claude Code power user customization: How to configure hooks](https://claude.com/blog/how-to-configure-hooks)
- [Boris Cherny on Threads — How I use Claude Code](https://www.threads.com/@boris_cherny/post/DTBVlMIkpcm)
- [GitHub Issue #13668 — PreCompact empty transcript_path](https://github.com/anthropics/claude-code/issues/13668)
- [GitHub Issue #13572 — PreCompact not triggered on /compact](https://github.com/anthropics/claude-code/issues/13572)
- [GitHub Issue #14258 — PostCompact hooks feature request](https://github.com/anthropics/claude-code/issues/14258)
- [Stop hook infinite loop — claude-mem #987](https://github.com/thedotmack/claude-mem/issues/987)
- [Continuous-Claude-v3 — community session continuity project](https://github.com/parcadei/Continuous-Claude-v3)
- [Session continuity and strategic compaction — claudecn.com](https://claudecn.com/en/docs/claude-code/workflows/session-continuity/)
