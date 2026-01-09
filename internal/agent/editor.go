package agent

import (
	"fmt"
	"os"
	"os/exec"
)

// EditorOptions contains options for launching an editor
type EditorOptions struct {
	TmuxSplitDirection string // "horizontal" or "vertical"
}

// OpenEditor opens an editor/IDE alongside the agent session
// Returns an error if the editor cannot be launched
func OpenEditor(workdir string, editor string, opts EditorOptions) error {
	switch editor {
	case "cursor":
		return openCursor(workdir)
	case "code":
		return openVSCode(workdir)
	case "nvim", "neovim", "vim":
		return openNvim(workdir, opts)
	case "":
		return nil // No editor configured
	default:
		// Try to run as a generic command
		return openGeneric(workdir, editor)
	}
}

// openCursor opens Cursor IDE in the background
func openCursor(workdir string) error {
	cmd := exec.Command("cursor", workdir)
	cmd.Dir = workdir
	// Don't attach stdin/stdout - run in background
	return cmd.Start()
}

// openVSCode opens VS Code in the background
func openVSCode(workdir string) error {
	cmd := exec.Command("code", workdir)
	cmd.Dir = workdir
	return cmd.Start()
}

// openNvim opens neovim - in tmux split if available, otherwise warns
func openNvim(workdir string, opts EditorOptions) error {
	if !inTmux() {
		return fmt.Errorf("nvim requires tmux for split view. Use --open cursor or --open code instead")
	}

	// Determine split flag based on direction
	splitFlag := "-h" // horizontal (side by side) is default
	if opts.TmuxSplitDirection == "vertical" {
		splitFlag = "-v" // vertical (stacked)
	}

	// Create new pane with nvim
	// -c starts the pane in the specified directory
	// The new pane opens on the right/bottom, current pane (Claude) stays in focus
	cmd := exec.Command("tmux", "split-window", splitFlag, "-c", workdir, "nvim", ".")
	cmd.Dir = workdir
	return cmd.Run()
}

// openGeneric tries to open an arbitrary editor command
func openGeneric(workdir string, editor string) error {
	cmd := exec.Command(editor, workdir)
	cmd.Dir = workdir
	return cmd.Start()
}

// inTmux checks if we're running inside a tmux session
func inTmux() bool {
	return os.Getenv("TMUX") != ""
}
