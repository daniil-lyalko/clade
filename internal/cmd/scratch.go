package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/daniil-lyalko/clade/internal/config"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var scratchAgentFlag string

var scratchCmd = &cobra.Command{
	Use:   "scratch [name]",
	Short: "Create a no-git scratch folder for documents or analysis",
	Long: `Create a scratch folder without git for quick document analysis or ad-hoc work.

Unlike experiments, scratch folders:
  - Have no git repository or worktree
  - Are for temporary document analysis, file sharing, etc.
  - Still get .claude/ config for hooks and context

Examples:
  clade scratch doc-analysis       # Quick scratch folder
  clade scratch PROJ-1234          # Ticket investigation (no code)
  clade scratch meeting-notes      # Temporary workspace`,
	Args: cobra.MaximumNArgs(1),
	RunE: runScratch,
}

func init() {
	rootCmd.AddCommand(scratchCmd)
	scratchCmd.Flags().StringVarP(&scratchAgentFlag, "agent", "a", "", "Agent to launch (overrides config)")
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
			return launchAgent(cfg, existing.Path, scratchAgentFlag)
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

	// Initialize .claude/ configuration
	ui.Info("Initializing .claude/ configuration...")
	if err := InitRepo(scratchPath); err != nil {
		ui.Warn("Failed to initialize .claude/: %v", err)
	}

	// Create .clade.json metadata
	ticket := extractTicketFromName(scratchName)
	cladeMetadata := map[string]interface{}{
		"type":    "scratch",
		"name":    scratchName,
		"ticket":  ticket,
		"created": time.Now().Format(time.RFC3339),
	}
	if err := writeScratchJSON(filepath.Join(scratchPath, ".clade.json"), cladeMetadata); err != nil {
		ui.Warn("Failed to write .clade.json: %v", err)
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

	// Launch agent
	return launchAgent(cfg, scratchPath, scratchAgentFlag)
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

func writeScratchJSON(path string, data interface{}) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}
