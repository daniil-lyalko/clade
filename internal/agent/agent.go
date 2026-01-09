package agent

import (
	"os"
	"os/exec"
	"strings"
)

// LaunchOptions contains options for launching an agent
type LaunchOptions struct {
	AddDirs []string // Additional directories (for multi-repo projects)
	Flags   []string // Extra flags to pass to the agent
}

// Agent defines the interface for AI coding agents
type Agent interface {
	Launch(workdir string, opts LaunchOptions) error
	Name() string
}

// ClaudeAgent implements Agent for Claude Code
type ClaudeAgent struct{}

// Name returns the agent name
func (c *ClaudeAgent) Name() string {
	return "claude"
}

// Launch starts Claude Code in the given directory
func (c *ClaudeAgent) Launch(workdir string, opts LaunchOptions) error {
	args := []string{}

	// Add additional directories
	for _, dir := range opts.AddDirs {
		args = append(args, "--add-dir", dir)
	}

	// Add extra flags
	args = append(args, opts.Flags...)

	cmd := exec.Command("claude", args...)
	cmd.Dir = workdir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// GenericAgent implements Agent for any command-based agent
type GenericAgent struct {
	Command string // e.g., "cursor .", "code ."
}

// Name returns the command being used
func (g *GenericAgent) Name() string {
	return g.Command
}

// Launch starts the generic agent in the given directory
func (g *GenericAgent) Launch(workdir string, opts LaunchOptions) error {
	parts := strings.Fields(g.Command)
	if len(parts) == 0 {
		parts = []string{"claude"}
	}

	var args []string
	if len(parts) > 1 {
		args = parts[1:]
	}

	// Replace "." with the actual workdir path (some editors don't respect cmd.Dir)
	for i, arg := range args {
		if arg == "." {
			args[i] = workdir
		}
	}

	cmd := exec.Command(parts[0], args...)
	cmd.Dir = workdir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// NewAgent creates an agent based on the configured command
func NewAgent(agentCmd string) Agent {
	if agentCmd == "claude" || agentCmd == "" {
		return &ClaudeAgent{}
	}
	return &GenericAgent{Command: agentCmd}
}
