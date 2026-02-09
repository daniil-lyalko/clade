package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var setupForceFlag bool

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Install global hooks for Claude Code and Cursor (one-time)",
	Long: `Install clade hooks globally so every project benefits automatically.

This merges hooks into:
  - ~/.claude/settings.json  (SessionStart hook)
  - ~/.claude/commands/drop.md  (/drop command)
  - ~/.cursor/hooks.json  (sessionStart hook)
  - ~/.cursor/commands/drop.md  (/drop command)

Safe to run multiple times — existing settings are preserved and hooks
are not duplicated. Use --force to overwrite hook entries.

Both Claude Code and Cursor hooks are additive across scopes, so
per-repo hooks (if any) will still work alongside these global hooks.`,
	RunE: runSetup,
}

func init() {
	rootCmd.AddCommand(setupCmd)
	setupCmd.Flags().BoolVarP(&setupForceFlag, "force", "f", false, "Overwrite existing hook entries")
}

// setupAction describes a planned modification that setup will make
type setupAction struct {
	description string       // e.g. "Add SessionStart hook"
	filePath    string       // e.g. "~/.claude/settings.json"
	actionType  string       // "add", "migrate", "create", "skip", "error"
	errorMsg    string       // only set when actionType == "error"
	apply       func() error // closure to execute the change
}

func runSetup(cmd *cobra.Command, args []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	// Phase 1: Scan what needs changing (no mutations)
	actions := planSetupActions(homeDir, setupForceFlag)

	// Phase 2: Display preview
	ui.Header("Global hooks setup")
	fmt.Println()

	var pending []setupAction
	var skipped []setupAction
	var errored []setupAction

	for _, a := range actions {
		switch a.actionType {
		case "skip":
			skipped = append(skipped, a)
		case "error":
			errored = append(errored, a)
		default:
			pending = append(pending, a)
		}
	}

	// Show errors
	for _, a := range errored {
		ui.Error("%s: %s", a.description, a.errorMsg)
	}

	if len(pending) == 0 {
		if len(skipped) > 0 {
			ui.Info("Everything already configured — nothing to do")
			for _, a := range skipped {
				ui.Detail("  %s  %s", ui.Dim(abbreviatePath(a.filePath, homeDir)), a.description)
			}
		}
		return nil
	}

	// Show what will be modified/created
	for _, a := range pending {
		label := "modify"
		if a.actionType == "create" {
			label = "create"
		} else if a.actionType == "migrate" {
			label = "migrate"
		}
		fmt.Printf("  %s %s  %s\n", ui.Yellow("["+label+"]"), abbreviatePath(a.filePath, homeDir), a.description)
	}

	if len(skipped) > 0 {
		fmt.Println()
		ui.Info("Already configured:")
		for _, a := range skipped {
			fmt.Printf("  %s  %s\n", ui.Dim(abbreviatePath(a.filePath, homeDir)), a.description)
		}
	}

	fmt.Println()

	// Phase 3: Confirm (skip if --force)
	if !setupForceFlag {
		prompt := promptui.Prompt{
			Label:     "Proceed with setup",
			IsConfirm: true,
			Default:   "y",
		}
		if _, err := prompt.Run(); err != nil {
			ui.Info("Setup cancelled")
			return nil
		}
		fmt.Println()
	}

	// Phase 4: Apply
	for _, a := range pending {
		if err := a.apply(); err != nil {
			ui.Error("%s: %v", a.description, err)
		} else {
			ui.Success("%s  %s", a.description, ui.Dim(abbreviatePath(a.filePath, homeDir)))
		}
	}

	fmt.Println()
	ui.Success("Setup complete!")
	ui.Detail("Hooks will run automatically in every project")

	return nil
}

// planSetupActions scans the system and returns what setup would do, without modifying anything
func planSetupActions(homeDir string, force bool) []setupAction {
	var actions []setupAction

	// 1. Claude hook in ~/.claude/settings.json
	claudeSettingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	actions = append(actions, planClaudeHookAction(claudeSettingsPath, force))

	// 2. Claude /drop command
	claudeDropPath := filepath.Join(homeDir, ".claude", "commands", "drop.md")
	actions = append(actions, planDropCommandAction(claudeDropPath, force, "Claude /drop command"))

	// 3. Cursor hook in ~/.cursor/hooks.json
	cursorHooksPath := filepath.Join(homeDir, ".cursor", "hooks.json")
	actions = append(actions, planCursorHookAction(cursorHooksPath, force))

	// 4. Cursor /drop command
	cursorDropPath := filepath.Join(homeDir, ".cursor", "commands", "drop.md")
	actions = append(actions, planDropCommandAction(cursorDropPath, force, "Cursor /drop command"))

	return actions
}

func planClaudeHookAction(path string, force bool) setupAction {
	const cladeCommand = "clade inject-context"
	const legacyPacerCommand = "pacer inject-context"

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return setupAction{
				description: "Add SessionStart hook",
				filePath:    path,
				actionType:  "create",
				apply:       func() error { _, err := mergeClaudeSettingsHooks(path, force); return err },
			}
		}
		return setupAction{
			description: "Claude settings",
			filePath:    path,
			actionType:  "error",
			errorMsg:    err.Error(),
		}
	}

	content := string(data)
	if strings.Contains(content, cladeCommand) && !force {
		return setupAction{
			description: "SessionStart hook",
			filePath:    path,
			actionType:  "skip",
		}
	}
	if strings.Contains(content, legacyPacerCommand) {
		return setupAction{
			description: "Migrate pacer → clade hook",
			filePath:    path,
			actionType:  "migrate",
			apply:       func() error { _, err := mergeClaudeSettingsHooks(path, force); return err },
		}
	}
	return setupAction{
		description: "Add SessionStart hook",
		filePath:    path,
		actionType:  "add",
		apply:       func() error { _, err := mergeClaudeSettingsHooks(path, force); return err },
	}
}

func planCursorHookAction(path string, force bool) setupAction {
	const cladeCommand = "clade inject-context"
	const legacyPacerCommand = "pacer inject-context"

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return setupAction{
				description: "Add sessionStart hook",
				filePath:    path,
				actionType:  "create",
				apply:       func() error { _, err := mergeCursorHooksJSON(path, force); return err },
			}
		}
		return setupAction{
			description: "Cursor hooks",
			filePath:    path,
			actionType:  "error",
			errorMsg:    err.Error(),
		}
	}

	content := string(data)
	if strings.Contains(content, cladeCommand) && !force {
		return setupAction{
			description: "sessionStart hook",
			filePath:    path,
			actionType:  "skip",
		}
	}
	if strings.Contains(content, legacyPacerCommand) {
		return setupAction{
			description: "Migrate pacer → clade hook",
			filePath:    path,
			actionType:  "migrate",
			apply:       func() error { _, err := mergeCursorHooksJSON(path, force); return err },
		}
	}
	return setupAction{
		description: "Add sessionStart hook",
		filePath:    path,
		actionType:  "add",
		apply:       func() error { _, err := mergeCursorHooksJSON(path, force); return err },
	}
}

func planDropCommandAction(path string, force bool, description string) setupAction {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return setupAction{
				description: description,
				filePath:    path,
				actionType:  "skip",
			}
		}
	}
	return setupAction{
		description: description,
		filePath:    path,
		actionType:  "create",
		apply:       func() error { _, err := writeDropCommandFile(path, force); return err },
	}
}

// abbreviatePath replaces the home directory prefix with ~
func abbreviatePath(path, homeDir string) string {
	if strings.HasPrefix(path, homeDir) {
		return "~" + path[len(homeDir):]
	}
	return path
}

// globalHooksInstalled checks if at least one AI tool has clade hooks configured
func globalHooksInstalled() bool {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return true // Can't check, assume OK
	}
	claude := hasCladeHook(filepath.Join(homeDir, ".claude", "settings.json"), "claude")
	cursor := hasCladeHook(filepath.Join(homeDir, ".cursor", "hooks.json"), "cursor")
	return claude || cursor
}

// mergeClaudeSettingsHooks safely merges the clade SessionStart hook into
// an existing ~/.claude/settings.json, preserving all other keys.
// Returns (changed, error).
func mergeClaudeSettingsHooks(path string, force bool) (bool, error) {
	const cladeCommand = "clade inject-context"
	const legacyPacerCommand = "pacer inject-context"

	// Read existing file
	var root map[string]json.RawMessage
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return false, fmt.Errorf("failed to read %s: %w", path, err)
		}
		// File doesn't exist — create from scratch
		root = make(map[string]json.RawMessage)
	} else {
		// Parse top-level keys (preserving everything)
		if err := json.Unmarshal(data, &root); err != nil {
			return false, fmt.Errorf("invalid JSON in %s: %w (will not overwrite)", path, err)
		}
	}

	// Parse the hooks object
	var hooksObj map[string]json.RawMessage
	if raw, ok := root["hooks"]; ok {
		if err := json.Unmarshal(raw, &hooksObj); err != nil {
			return false, fmt.Errorf("invalid hooks in %s: %w", path, err)
		}
	} else {
		hooksObj = make(map[string]json.RawMessage)
	}

	// Parse SessionStart array
	type hookEntry struct {
		Matcher string `json:"matcher,omitempty"`
		Hooks   []struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		} `json:"hooks,omitempty"`
	}

	var sessionStart []hookEntry
	if raw, ok := hooksObj["SessionStart"]; ok {
		if err := json.Unmarshal(raw, &sessionStart); err != nil {
			return false, fmt.Errorf("invalid SessionStart in %s: %w", path, err)
		}
	}

	// Check if clade hook already present
	for i, entry := range sessionStart {
		for j, h := range entry.Hooks {
			if h.Command == cladeCommand {
				if !force {
					return false, nil // Already configured
				}
				// Force: continue to ensure it's there (it is)
				_ = i
				_ = j
				return false, nil
			}
			if h.Command == legacyPacerCommand {
				// Migration: replace pacer with clade
				sessionStart[i].Hooks[j].Command = cladeCommand
				goto write
			}
		}
	}

	// Not found — add it
	sessionStart = append(sessionStart, hookEntry{
		Matcher: "*",
		Hooks: []struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		}{
			{Type: "command", Command: cladeCommand},
		},
	})

write:
	// Marshal back SessionStart
	sessionStartJSON, err := json.Marshal(sessionStart)
	if err != nil {
		return false, err
	}
	hooksObj["SessionStart"] = sessionStartJSON

	// Handle Stop hook (async auto-dropbag)
	type asyncHook struct {
		Type    string `json:"type"`
		Command string `json:"command"`
		Async   bool   `json:"async"`
		Timeout int    `json:"timeout"`
	}
	type asyncHookEntry struct {
		Matcher string      `json:"matcher,omitempty"`
		Hooks   []asyncHook `json:"hooks,omitempty"`
	}

	var stopHooks []asyncHookEntry
	if raw, ok := hooksObj["Stop"]; ok {
		if err := json.Unmarshal(raw, &stopHooks); err == nil {
			// Check if clade auto-dropbag already present
			stopNeedsUpdate := true
			for _, entry := range stopHooks {
				for _, h := range entry.Hooks {
					if h.Command == "clade auto-dropbag" {
						stopNeedsUpdate = false
						break
					}
				}
			}

			if stopNeedsUpdate {
				// Add to existing Stop hooks
				stopHooks = append(stopHooks, asyncHookEntry{
					Matcher: "*",
					Hooks: []asyncHook{
						{
							Type:    "command",
							Command: "clade auto-dropbag",
							Async:   true,
							Timeout: 30,
						},
					},
				})
			}
		}
	} else {
		// No Stop hooks yet - create from scratch
		stopHooks = []asyncHookEntry{
			{
				Matcher: "*",
				Hooks: []asyncHook{
					{
						Type:    "command",
						Command: "clade auto-dropbag",
						Async:   true,
						Timeout: 30,
					},
				},
			},
		}
	}

	stopHooksJSON, _ := json.Marshal(stopHooks)
	hooksObj["Stop"] = stopHooksJSON

	// Handle PreCompact hook (context warning)
	var preCompactHooks []hookEntry
	if raw, ok := hooksObj["PreCompact"]; ok {
		if err := json.Unmarshal(raw, &preCompactHooks); err == nil {
			// Check if clade context-warning already present
			preCompactNeedsUpdate := true
			for _, entry := range preCompactHooks {
				for _, h := range entry.Hooks {
					if h.Command == "clade context-warning" {
						preCompactNeedsUpdate = false
						break
					}
				}
			}

			if preCompactNeedsUpdate {
				// Add to existing PreCompact hooks
				preCompactHooks = append(preCompactHooks, hookEntry{
					Matcher: "*",
					Hooks: []struct {
						Type    string `json:"type"`
						Command string `json:"command"`
					}{
						{Type: "command", Command: "clade context-warning"},
					},
				})
			}
		}
	} else {
		// No PreCompact hooks yet - create from scratch
		preCompactHooks = []hookEntry{
			{
				Matcher: "*",
				Hooks: []struct {
					Type    string `json:"type"`
					Command string `json:"command"`
				}{
					{Type: "command", Command: "clade context-warning"},
				},
			},
		}
	}

	preCompactJSON, _ := json.Marshal(preCompactHooks)
	hooksObj["PreCompact"] = preCompactJSON

	hooksJSON, err := json.Marshal(hooksObj)
	if err != nil {
		return false, err
	}
	root["hooks"] = hooksJSON

	output, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return false, err
	}

	if err := os.WriteFile(path, append(output, '\n'), 0644); err != nil {
		return false, err
	}
	return true, nil
}

// mergeCursorHooksJSON safely merges the clade sessionStart hook into
// an existing ~/.cursor/hooks.json, preserving all other keys.
// Returns (changed, error).
func mergeCursorHooksJSON(path string, force bool) (bool, error) {
	const cladeCommand = "clade inject-context"
	const legacyPacerCommand = "pacer inject-context"

	// Read existing file
	var root map[string]json.RawMessage
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return false, fmt.Errorf("failed to read %s: %w", path, err)
		}
		root = make(map[string]json.RawMessage)
	} else {
		if err := json.Unmarshal(data, &root); err != nil {
			return false, fmt.Errorf("invalid JSON in %s: %w (will not overwrite)", path, err)
		}
	}

	// Ensure version key
	if _, ok := root["version"]; !ok {
		versionJSON, _ := json.Marshal(1)
		root["version"] = versionJSON
	}

	// Parse the hooks object
	var hooksObj map[string]json.RawMessage
	if raw, ok := root["hooks"]; ok {
		if err := json.Unmarshal(raw, &hooksObj); err != nil {
			return false, fmt.Errorf("invalid hooks in %s: %w", path, err)
		}
	} else {
		hooksObj = make(map[string]json.RawMessage)
	}

	// Parse sessionStart array (Cursor uses camelCase)
	type cursorHookEntry struct {
		Command string `json:"command"`
	}

	var sessionStart []cursorHookEntry
	if raw, ok := hooksObj["sessionStart"]; ok {
		if err := json.Unmarshal(raw, &sessionStart); err != nil {
			return false, fmt.Errorf("invalid sessionStart in %s: %w", path, err)
		}
	}

	// Check if clade hook already present
	for i, entry := range sessionStart {
		if entry.Command == cladeCommand {
			if !force {
				return false, nil
			}
			return false, nil
		}
		if entry.Command == legacyPacerCommand {
			sessionStart[i].Command = cladeCommand
			goto write
		}
	}

	// Not found — add it
	sessionStart = append(sessionStart, cursorHookEntry{Command: cladeCommand})

write:
	sessionStartJSON, err := json.Marshal(sessionStart)
	if err != nil {
		return false, err
	}
	hooksObj["sessionStart"] = sessionStartJSON

	hooksJSON, err := json.Marshal(hooksObj)
	if err != nil {
		return false, err
	}
	root["hooks"] = hooksJSON

	output, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return false, err
	}

	if err := os.WriteFile(path, append(output, '\n'), 0644); err != nil {
		return false, err
	}
	return true, nil
}

// writeDropCommandFile writes the /drop command file if it doesn't exist.
// Returns (changed, error).
func writeDropCommandFile(path string, force bool) (bool, error) {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return false, nil // Already exists
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return false, err
	}

	if err := os.WriteFile(path, []byte(dropCommandContent), 0644); err != nil {
		return false, err
	}
	return true, nil
}

// hasCladeHook checks if a given settings/hooks file has the clade inject-context hook.
// format should be "claude" or "cursor".
func hasCladeHook(path string, format string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	// Simple string check — good enough for detection
	return strings.Contains(string(data), "clade inject-context")
}
