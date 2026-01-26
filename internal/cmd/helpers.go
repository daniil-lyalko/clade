package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/daniil-lyalko/clade/internal/agent"
	"github.com/daniil-lyalko/clade/internal/config"
	"github.com/daniil-lyalko/clade/internal/git"
	"github.com/daniil-lyalko/clade/internal/ui"
	"github.com/daniil-lyalko/clade/internal/util"
	"github.com/manifoldco/promptui"
)

// Shared helper functions extracted from deprecated exp.go and feat.go
// These are used across multiple commands (work, resume, scratch, project)

// isValidExpName validates a worktree/project/scratch name
// Delegates to util.IsValidName for actual validation
func isValidExpName(name string) bool {
	return util.IsValidName(name)
}

// extractTicket extracts a JIRA-style ticket ID from a name
// Delegates to util.ExtractTicket for actual extraction
func extractTicket(name string) string {
	return util.ExtractTicket(name)
}

// resolveRepo determines which repository to use based on flags and current context
// Resolution order:
// 1. Explicit --repo flag (by name or path)
// 2. Current directory (if it's a git repo)
// 3. Registered repos (interactive picker)
func resolveRepo(cfg *config.Config, repoFlag string) (string, error) {
	// 1. Check if repo flag was provided
	if repoFlag != "" {
		// Check if it's a registered repo name
		if path, ok := cfg.Repos[repoFlag]; ok {
			return config.ExpandPath(path), nil
		}
		// Assume it's a path
		expanded := config.ExpandPath(repoFlag)
		if git.IsGitRepo(expanded) {
			return git.GetRepoRoot(expanded)
		}
		return "", fmt.Errorf("not a git repository: %s", repoFlag)
	}

	// 2. Check if we're in a git repo
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if git.IsGitRepo(cwd) {
		// Check if we're in a worktree (not main repo)
		if git.IsWorktree(cwd) {
			mainRepo, err := git.GetMainRepoRoot(cwd)
			if err != nil {
				return "", err
			}

			// Try to get context from .clade.json if it exists
			cladeFile := filepath.Join(cwd, ".clade.json")
			if data, err := os.ReadFile(cladeFile); err == nil {
				var meta map[string]interface{}
				if json.Unmarshal(data, &meta) == nil {
					name, _ := meta["name"].(string)
					label, _ := meta["label"].(string)
					repoName, _ := meta["repo"].(string)
					if repoName == "" {
						repoName = git.GetRepoName(mainRepo)
					}
					if name != "" && label != "" {
						ui.Info("In clade worktree '%s' (%s), using source repo: %s", name, label, repoName)
					} else if name != "" {
						ui.Info("In clade worktree '%s', using source repo: %s", name, repoName)
					} else {
						ui.Info("In worktree, using main repo: %s", git.GetRepoName(mainRepo))
					}
				}
			} else {
				// No .clade.json, just show basic message
				ui.Info("In worktree, using main repo: %s", git.GetRepoName(mainRepo))
			}

			return mainRepo, nil
		}
		// In main repo, just return its root
		return git.GetRepoRoot(cwd)
	}

	// 3. Check if there are registered repos
	if len(cfg.Repos) == 0 {
		return "", fmt.Errorf("not in a git repo. Register repos with: clade repo add <path>")
	}

	// 4. Interactive picker
	var repoNames []string
	// Put last used repo first
	if cfg.LastRepo != "" {
		for name, path := range cfg.Repos {
			if config.ExpandPath(path) == cfg.LastRepo {
				repoNames = append([]string{name + " (last used)"}, repoNames...)
			} else {
				repoNames = append(repoNames, name)
			}
		}
	} else {
		for name := range cfg.Repos {
			repoNames = append(repoNames, name)
		}
	}

	prompt := promptui.Select{
		Label: "Select repo",
		Items: repoNames,
	}
	_, selected, err := prompt.Run()
	if err != nil {
		return "", err
	}

	// Remove " (last used)" suffix if present
	selected = strings.TrimSuffix(selected, " (last used)")
	return config.ExpandPath(cfg.Repos[selected]), nil
}

// resolveRepoWithPick is like resolveRepo but with pick flag support
func resolveRepoWithPick(cfg *config.Config, repoFlag string, forcePick bool) (string, error) {
	// If pick flag is set, force interactive picker
	if forcePick {
		return showRepoPicker(cfg)
	}
	return resolveRepo(cfg, repoFlag)
}

// showRepoPicker shows an interactive repo picker
func showRepoPicker(cfg *config.Config) (string, error) {
	// Check if we're in a git repo - add it as an option
	var repoNames []string
	var currentRepoPath string

	cwd, err := os.Getwd()
	if err == nil && git.IsGitRepo(cwd) {
		// Use GetMainRepoRoot to handle the case where we're in a worktree
		currentRepoPath, _ = git.GetMainRepoRoot(cwd)
		repoNames = append(repoNames, "(current directory)")
	}

	// Add registered repos
	if cfg.LastRepo != "" {
		for name, path := range cfg.Repos {
			if config.ExpandPath(path) == cfg.LastRepo {
				repoNames = append(repoNames, name+" (last used)")
			} else {
				repoNames = append(repoNames, name)
			}
		}
	} else {
		for name := range cfg.Repos {
			repoNames = append(repoNames, name)
		}
	}

	if len(repoNames) == 0 {
		return "", fmt.Errorf("no repos available. Register repos with: clade repo add <path>")
	}

	prompt := promptui.Select{
		Label: "Select repo",
		Items: repoNames,
	}
	_, selected, err := prompt.Run()
	if err != nil {
		return "", err
	}

	if selected == "(current directory)" {
		return currentRepoPath, nil
	}

	// Remove " (last used)" suffix if present
	selected = strings.TrimSuffix(selected, " (last used)")
	return config.ExpandPath(cfg.Repos[selected]), nil
}

// launchSession opens editor and/or launches agent based on config and flags
func launchSession(cfg *config.Config, workdir string, editorOverride string, noAgent bool, noEditor bool) error {
	// Determine editor to use
	editor := cfg.Editor
	if editorOverride != "" {
		editor = editorOverride
	}

	// Open editor first (if configured and not disabled)
	if !noEditor && editor != "" {
		opts := agent.EditorOptions{
			TmuxSplitDirection: cfg.TmuxSplitDirection,
		}
		if err := agent.OpenEditor(workdir, editor, opts); err != nil {
			ui.Warn("Could not open editor: %s", err)
			// Continue anyway - don't block the agent launch
		} else {
			ui.Info("Opened %s", editor)
		}
	}

	// Launch agent (if configured and not disabled)
	if !noAgent && cfg.Agent != "" {
		ui.Info("Launching %s...", cfg.Agent)
		fmt.Println()

		ag := agent.NewAgent(cfg.Agent)
		opts := agent.LaunchOptions{
			Flags: cfg.AgentFlags,
		}
		return ag.Launch(workdir, opts)
	}

	return nil
}
