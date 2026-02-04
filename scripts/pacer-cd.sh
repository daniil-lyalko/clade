#!/bin/bash
# pacer-cd.sh - Shell helper for changing to pacer worktrees
#
# Installation:
#   # Via Homebrew (automatic):
#   source $(brew --prefix)/opt/pacer/scripts/pacer-cd.sh
#
#   # Manual installation:
#   source /path/to/pacer/scripts/pacer-cd.sh
#
# Usage:
#   ccd <name>      # Jump to worktree by name
#   ccd             # Interactive picker

function ccd() {
  # Get the worktree path from pacer
  local path

  if [ $# -eq 0 ]; then
    # No args - show interactive picker
    path=$(pacer resume --no-agent --no-editor --print-path 2>/dev/null)
  else
    # Name provided - resolve directly
    path=$(pacer resume "$1" --no-agent --no-editor --print-path 2>/dev/null)
  fi

  local exit_code=$?

  if [ $exit_code -ne 0 ]; then
    echo "Error: pacer resume failed" >&2
    return 1
  fi

  if [ -z "$path" ]; then
    echo "No worktree found" >&2
    return 1
  fi

  # Change to the directory
  if cd "$path"; then
    echo "Switched to: $path"
    return 0
  else
    echo "Failed to change directory to: $path" >&2
    return 1
  fi
}

# Tab completion for ccd
if [ -n "$ZSH_VERSION" ]; then
  # Zsh completion - delegate to pacer's completion
  compdef ccd=pacer
elif [ -n "$BASH_VERSION" ]; then
  # Bash completion - delegate to pacer's completion
  complete -F _pacer ccd 2>/dev/null || true
fi

# Export the function so it's available in subshells
if [ -n "$ZSH_VERSION" ]; then
  # Zsh doesn't need export -f
  :
elif [ -n "$BASH_VERSION" ]; then
  export -f ccd
fi
