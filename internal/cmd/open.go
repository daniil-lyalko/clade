package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/daniil-lyalko/clade/internal/config"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var openCmd = &cobra.Command{
	Use:   "open [name]",
	Short: "Print path to experiment/project/scratch (use with cd)",
	Long: `Print the path to a worktree for use with cd.

Examples:
  cd $(clade open try-redis)
  cd $(clade open)              # Interactive picker

Tip: Add a shell alias for convenience:
  alias cdo='cd $(clade open)'`,
	Args:              cobra.MaximumNArgs(1),
	RunE:              runOpen,
	ValidArgsFunction: completeResumableNames,
}

func init() {
	rootCmd.AddCommand(openCmd)
}

func runOpen(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	state, err := config.LoadState(cfg)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	// If no args, show picker
	if len(args) == 0 {
		return openInteractive(cfg, state)
	}

	name := args[0]

	// Check experiments
	for _, exp := range state.Experiments {
		if exp.Name == name {
			return openPath(cfg, state, exp.Path, "experiment", name)
		}
	}

	// Check projects
	for _, proj := range state.Projects {
		if proj.Name == name {
			return openPath(cfg, state, proj.Path, "project", name)
		}
	}

	// Check scratches
	for _, scratch := range state.Scratches {
		if scratch.Name == name {
			return openPath(cfg, state, scratch.Path, "scratch", name)
		}
	}

	return fmt.Errorf("not found: %s", name)
}

func openInteractive(cfg *config.Config, state *config.State) error {
	if len(state.Experiments) == 0 && len(state.Projects) == 0 && len(state.Scratches) == 0 {
		return fmt.Errorf("no experiments, projects, or scratch folders")
	}

	type pickItem struct {
		Name     string
		Path     string
		Type     string
		LastUsed time.Time
		Display  string
	}

	var items []pickItem

	for _, exp := range state.Experiments {
		age := formatAge(exp.LastUsed)
		items = append(items, pickItem{
			Name:     exp.Name,
			Path:     exp.Path,
			Type:     "experiment",
			LastUsed: exp.LastUsed,
			Display:  fmt.Sprintf("%s [exp] (%s)", exp.Name, age),
		})
	}

	for _, proj := range state.Projects {
		age := formatAge(proj.LastUsed)
		items = append(items, pickItem{
			Name:     proj.Name,
			Path:     proj.Path,
			Type:     "project",
			LastUsed: proj.LastUsed,
			Display:  fmt.Sprintf("%s [project] (%s)", proj.Name, age),
		})
	}

	for _, scratch := range state.Scratches {
		age := formatAge(scratch.LastUsed)
		items = append(items, pickItem{
			Name:     scratch.Name,
			Path:     scratch.Path,
			Type:     "scratch",
			LastUsed: scratch.LastUsed,
			Display:  fmt.Sprintf("%s [scratch] (%s)", scratch.Name, age),
		})
	}

	// Sort by last used
	for i := 0; i < len(items)-1; i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].LastUsed.After(items[i].LastUsed) {
				items[i], items[j] = items[j], items[i]
			}
		}
	}

	var displayItems []string
	for _, item := range items {
		displayItems = append(displayItems, item.Display)
	}

	prompt := promptui.Select{
		Label:  "Select to open",
		Items:  displayItems,
		Size:   10,
		Stdout: os.Stderr, // Use stderr for prompt so stdout is clean for path
	}

	idx, _, err := prompt.Run()
	if err != nil {
		return err
	}

	selected := items[idx]
	return openPath(cfg, state, selected.Path, selected.Type, selected.Name)
}

func openPath(cfg *config.Config, state *config.State, path, itemType, name string) error {
	// Verify path exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("path no longer exists: %s", path)
	}

	// Update last used timestamp
	switch itemType {
	case "experiment":
		for _, exp := range state.Experiments {
			if exp.Name == name {
				exp.LastUsed = time.Now()
				state.Experiments[config.ExperimentKey(exp.Repo, exp.Name)] = exp
				break
			}
		}
	case "project":
		if proj := state.Projects[name]; proj != nil {
			proj.LastUsed = time.Now()
		}
	case "scratch":
		if scratch := state.Scratches[name]; scratch != nil {
			scratch.LastUsed = time.Now()
		}
	}
	state.Save(cfg)

	// Print path to stdout (clean, no decoration)
	fmt.Println(path)
	return nil
}
