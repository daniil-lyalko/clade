package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/daniil-lyalko/pacer/internal/config"
	"github.com/daniil-lyalko/pacer/internal/hooks"
	"github.com/daniil-lyalko/pacer/internal/ui"
	"github.com/daniil-lyalko/pacer/internal/util"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var (
	scratchEditorFlag   string
	scratchAgentFlag    string
	scratchNoAgentFlag  bool
	scratchNoEditorFlag bool
)

var scratchCmd = &cobra.Command{
	Use:   "scratch [name]",
	Short: "Create a no-git scratch folder for documents or analysis",
	Long: `Create a scratch folder without git for quick document analysis or ad-hoc work.

Unlike experiments, scratch folders:
  - Have no git repository or worktree
  - Are for temporary document analysis, file sharing, etc.
  - Still get .claude/ config for hooks and context

Examples:
  pacer scratch doc-analysis       # Quick scratch folder
  pacer scratch PROJ-1234          # Ticket investigation (no code)
  pacer scratch meeting-notes      # Temporary workspace
  pacer scratch foo -o cursor      # Open Cursor IDE
  pacer scratch foo --no-agent     # Skip launching Claude`,
	Args: cobra.MaximumNArgs(1),
	RunE: runScratch,
}

func init() {
	rootCmd.AddCommand(scratchCmd)
	scratchCmd.Flags().StringVarP(&scratchEditorFlag, "open", "o", "", "Open editor/IDE (cursor, code, nvim)")
	scratchCmd.Flags().StringVarP(&scratchAgentFlag, "agent", "a", "", "Override configured agent")
	scratchCmd.Flags().BoolVar(&scratchNoAgentFlag, "no-agent", false, "Skip launching the AI agent")
	scratchCmd.Flags().BoolVar(&scratchNoEditorFlag, "no-editor", false, "Skip opening the editor")
}

func runScratch(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Get scratch name
	var scratchName string
	if len(args) > 0 {
		scratchName = args[0]
	} else {
		prompt := promptui.Prompt{
			Label: "Scratch folder name",
		}
		scratchName, err = prompt.Run()
		if err != nil {
			return err
		}
	}

	// Validate scratch name
	if !isValidScratchName(scratchName) {
		return fmt.Errorf("invalid scratch name: use alphanumeric, hyphens, underscores only")
	}

	scratchPath := filepath.Join(cfg.ScratchDir(), scratchName)

	// Check if scratch already exists
	state, err := config.LoadState(cfg)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	if existing := state.GetScratch(scratchName); existing != nil {
		ui.Warn("Scratch '%s' already exists", scratchName)
		ui.KeyValue("Path", existing.Path)

		prompt := promptui.Prompt{
			Label:     "Resume existing scratch",
			IsConfirm: true,
		}
		_, err := prompt.Run()
		if err == nil {
			// User wants to resume
			return launchSession(cfg, existing.Path, scratchEditorFlag, scratchNoAgentFlag, scratchNoEditorFlag)
		}
		return nil
	}

	// Create scratch directory
	ui.Header("Creating scratch: %s", scratchName)
	ui.KeyValue("Path", scratchPath)

	// Ensure scratch directory exists
	if err := os.MkdirAll(scratchPath, 0755); err != nil {
		return fmt.Errorf("failed to create scratch directory: %w", err)
	}

	// Initialize .claude/ configuration (scratch-specific, no git assumptions)
	ui.Info("Initializing .claude/ configuration...")
	if err := initScratchConfig(scratchPath); err != nil {
		ui.Warn("Failed to initialize .claude/: %v", err)
	}

	// Create .pacer/metadata.json
	ticket := extractTicketFromName(scratchName)
	pacerMetadata := map[string]interface{}{
		"type":    "scratch",
		"name":    scratchName,
		"ticket":  ticket,
		"created": time.Now().Format(time.RFC3339),
	}
	metadataPath := filepath.Join(scratchPath, ".pacer", "metadata.json")
	if err := os.MkdirAll(filepath.Dir(metadataPath), 0755); err != nil {
		ui.Warn("Failed to create .pacer directory: %v", err)
	}
	if err := util.WriteJSON(metadataPath, pacerMetadata); err != nil {
		ui.Warn("Failed to write metadata.json: %v", err)
	}

	// Update state
	scratch := &config.Scratch{
		Name:     scratchName,
		Path:     scratchPath,
		Ticket:   ticket,
		Created:  time.Now(),
		LastUsed: time.Now(),
	}
	state.AddScratch(scratch)
	if err := state.Save(cfg); err != nil {
		ui.Warn("Failed to save state: %v", err)
	}

	ui.Success("Scratch folder created!")

	// Run on_create hooks (global only for scratches)
	if hooks.HasHooks(hooks.OnCreate, "") {
		ui.Info("Running on_create hooks...")
		hookEnv := &hooks.Env{
			Type:   "scratch",
			Name:   scratchName,
			Path:   scratchPath,
			Ticket: ticket,
		}
		results := hooks.RunHooks(hooks.OnCreate, hookEnv)
		for _, r := range results {
			if r.Error != nil {
				ui.Warn("Hook failed: %s - %v", r.Command, r.Error)
			}
		}
	}

	// Launch editor and/or agent
	return launchWorktreeSession(cfg, "", scratchPath, WorktreeOptions{
		EditorFlag:   scratchEditorFlag,
		AgentFlag:    scratchAgentFlag,
		NoAgentFlag:  scratchNoAgentFlag,
		NoEditorFlag: scratchNoEditorFlag,
	})
}

func isValidScratchName(name string) bool {
	if name == "" {
		return false
	}
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`, name)
	return matched
}

// extractTicketFromName extracts a JIRA-style ticket ID from the scratch name
func extractTicketFromName(name string) string {
	// Match patterns like PROJ-1234, ABC-123, etc.
	re := regexp.MustCompile(`^([A-Z]+-\d+)`)
	matches := re.FindStringSubmatch(strings.ToUpper(name))
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// initScratchConfig initializes .claude/ for scratch folders (no git assumptions)
func initScratchConfig(scratchPath string) error {
	claudeDir := filepath.Join(scratchPath, ".claude")
	commandsDir := filepath.Join(claudeDir, "commands")

	// Ensure directories exist
	if err := os.MkdirAll(commandsDir, 0755); err != nil {
		return err
	}

	// Write settings.json (same as regular init)
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		if err := writeSettingsJSON(settingsPath); err != nil {
			return err
		}
	}

	// Write drop.md command
	dropPath := filepath.Join(commandsDir, "drop.md")
	if _, err := os.Stat(dropPath); os.IsNotExist(err) {
		if err := writeDropCommand(dropPath); err != nil {
			return err
		}
	}

	// Cursor hooks and commands
	cursorDir := filepath.Join(scratchPath, ".cursor")
	cursorCommandsDir := filepath.Join(cursorDir, "commands")
	if err := os.MkdirAll(cursorCommandsDir, 0755); err != nil {
		return err
	}
	cursorHooksPath := filepath.Join(cursorDir, "hooks.json")
	if _, err := os.Stat(cursorHooksPath); os.IsNotExist(err) {
		if err := writeCursorHooksJSON(cursorHooksPath); err != nil {
			return err
		}
	}
	cursorDropPath := filepath.Join(cursorCommandsDir, "drop.md")
	if _, err := os.Stat(cursorDropPath); os.IsNotExist(err) {
		if err := writeCursorDropCommand(cursorDropPath); err != nil {
			return err
		}
	}

	// Note: No .gitignore update for scratches (they're not git repos)
	return nil
}
