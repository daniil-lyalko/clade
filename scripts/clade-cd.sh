#!/bin/bash
# clade-cd.sh - Shell helper for changing to clade worktrees
#
# Installation:
#   # Via Homebrew (automatic):
#   source $(brew --prefix)/opt/clade/scripts/clade-cd.sh
#
#   # Manual installation:
#   source /path/to/clade/scripts/clade-cd.sh
#
# Usage:
#   ccd <name>      # Jump to worktree by name
#   ccd             # Interactive picker

function ccd() {
  # Get the worktree path from clade
  local path

  if [ $# -eq 0 ]; then
    # No args - show interactive picker
    path=$(clade resume --no-agent --no-editor --print-path 2>/dev/null)
  else
    # Name provided - resolve directly
    path=$(clade resume "$1" --no-agent --no-editor --print-path 2>/dev/null)
  fi

  local exit_code=$?

  if [ $exit_code -ne 0 ]; then
    echo "Error: clade resume failed" >&2
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
  # Zsh completion - delegate to clade's completion
  compdef ccd=clade
elif [ -n "$BASH_VERSION" ]; then
  # Bash completion - delegate to clade's completion
  complete -F _clade ccd 2>/dev/null || true
fi

# Export the function so it's available in subshells
if [ -n "$ZSH_VERSION" ]; then
  # Zsh doesn't need export -f
  :
elif [ -n "$BASH_VERSION" ]; then
  export -f ccd
fi
