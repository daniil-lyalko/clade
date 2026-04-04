Create a session summary that saves to TWO places for cross-session awareness:

## Step 1: Determine session ID and today's date

```bash
# Get today's date for inbox
TODAY=$(date +%Y-%m-%d)
TIMESTAMP=$(date +%Y-%m-%d-%H%M)
TIME_DISPLAY=$(date +"%l:%M %p" | sed 's/^ //')
```

## Step 2: Write session dropbag to ~/.clade/sessions/

If the CLAUDE_SESSION_ID env var is available, write to `~/.clade/sessions/$CLAUDE_SESSION_ID.md`.
Otherwise, write to the legacy location `.clade/dropbags/DROPBAG-$TIMESTAMP.md`.

```bash
mkdir -p ~/.clade/sessions
cat > ~/.clade/sessions/${CLAUDE_SESSION_ID:-dropbag-$TIMESTAMP}.md <<'EOF'
[your content here]
EOF
```

## Step 3: Append to daily inbox for cross-session broadcast

```bash
mkdir -p ~/.clade/inbox
cat >> ~/.clade/inbox/$TODAY.md <<EOF

### $TIME_DISPLAY | $PROJECT_NAME | handoff
[One-line summary of what was accomplished and what's next]
EOF
```

## Content format for the dropbag:

### Summary
What we accomplished this session. Be specific about changes made.

### Current State
What's working, what's broken, what's partially implemented.

### Next Steps
Exact actions to continue (be specific - file names, function names, etc.).

### Key Files
Files to look at first when resuming. Include line numbers if relevant.

### Open Questions
Anything unresolved or decisions that need to be made.

---

After saving both files, confirm:
1. The session dropbag was created in ~/.clade/sessions/
2. The inbox entry was appended to ~/.clade/inbox/$TODAY.md
