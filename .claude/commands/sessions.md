Show all active Claude Code sessions and offer management actions.

1. Run the sessions dashboard command:
   ```bash
   clade sessions --json
   ```

2. Parse the JSON output and present it in a readable format:
   - For each session, show: status, project name, age, and summary
   - Color-code: active = working now, idle = paused recently, stale = forgotten
   - Show total counts

3. Offer actions:
   - **Resume**: "Want me to resume session N?" -> Run `clade sessions` interactively, or provide the `claude --resume <session_id>` command
   - **Clean**: "Want me to archive stale sessions?" -> Run `clade sessions --clean`
   - **Details**: If asked about a specific session, read its dropbag from `~/.clade/sessions/<session_id>.md`

4. If no sessions exist, explain that sessions are tracked automatically via hooks and suggest running `clade setup --force` if hooks aren't configured.

Example output format:

```
Found 3 sessions:

1. [active] leap-complete (2h) - API gateway debug, posted instructions
2. [idle 4h] document-engine - Lambda fix DONE, needs PR
3. [stale 1d] chezmoi - Dotfiles tmux config

What would you like to do?
```
