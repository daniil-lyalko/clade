package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/manifoldco/promptui"
)

// TrustEntry represents a trusted repo's hook configuration
type TrustEntry struct {
	Hash      string    `json:"hash"`       // SHA-256 hash of hooks.yaml content
	TrustedAt time.Time `json:"trusted_at"` // When the user approved
}

// TrustRegistry holds trust information for repos with hooks
type TrustRegistry struct {
	Repos map[string]TrustEntry `json:"repos"` // repo path -> trust entry
}

// TrustRegistryPath returns the path to the trust registry file
func TrustRegistryPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".config", "clade", "trusted-repos.json"), nil
}

// LoadTrustRegistry loads the trust registry from disk
func LoadTrustRegistry() (*TrustRegistry, error) {
	registryPath, err := TrustRegistryPath()
	if err != nil {
		return nil, err
	}

	registry := &TrustRegistry{
		Repos: make(map[string]TrustEntry),
	}

	data, err := os.ReadFile(registryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return registry, nil // Empty registry
		}
		return nil, err
	}

	if err := json.Unmarshal(data, registry); err != nil {
		return nil, err
	}

	if registry.Repos == nil {
		registry.Repos = make(map[string]TrustEntry)
	}

	return registry, nil
}

// Save writes the trust registry to disk
func (t *TrustRegistry) Save() error {
	registryPath, err := TrustRegistryPath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(registryPath), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}

	// Security: Trust registry should be owner-only
	return os.WriteFile(registryPath, data, 0600)
}

// HashFile computes SHA-256 hash of a file's contents
func HashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// IsTrusted checks if a repo's hooks are trusted and unchanged
func (t *TrustRegistry) IsTrusted(repoPath, hooksPath string) (bool, error) {
	entry, exists := t.Repos[repoPath]
	if !exists {
		return false, nil
	}

	// Check if the file still exists
	if _, err := os.Stat(hooksPath); os.IsNotExist(err) {
		return true, nil // No hooks file = nothing to distrust
	}

	// Verify hash matches
	currentHash, err := HashFile(hooksPath)
	if err != nil {
		return false, err
	}

	return entry.Hash == currentHash, nil
}

// Trust marks a repo's hooks as trusted
func (t *TrustRegistry) Trust(repoPath, hooksPath string) error {
	hash, err := HashFile(hooksPath)
	if err != nil {
		return err
	}

	t.Repos[repoPath] = TrustEntry{
		Hash:      hash,
		TrustedAt: time.Now(),
	}

	return t.Save()
}

// Revoke removes trust for a repo
func (t *TrustRegistry) Revoke(repoPath string) error {
	delete(t.Repos, repoPath)
	return t.Save()
}

// EnsureRepoHooksTrusted checks if repo hooks are trusted, prompting if not
// Returns error if user declines or an error occurs
//
// Set CLADE_TRUST_REPO_HOOKS=1 (or legacy PACER_TRUST_REPO_HOOKS=1) to auto-trust all repo hooks (for testing/CI).
func EnsureRepoHooksTrusted(repoPath string) error {
	hooksPath := filepath.Join(repoPath, ".clade", "hooks.yaml")

	// Check if hooks file exists
	if _, err := os.Stat(hooksPath); os.IsNotExist(err) {
		return nil // No hooks = nothing to trust
	}

	// Allow bypassing trust for testing/CI environments (accept both new and legacy env var)
	if os.Getenv("CLADE_TRUST_REPO_HOOKS") == "1" || os.Getenv("PACER_TRUST_REPO_HOOKS") == "1" {
		return nil
	}

	registry, err := LoadTrustRegistry()
	if err != nil {
		return fmt.Errorf("failed to load trust registry: %w", err)
	}

	trusted, err := registry.IsTrusted(repoPath, hooksPath)
	if err != nil {
		return fmt.Errorf("failed to check trust status: %w", err)
	}

	if trusted {
		return nil // Already trusted
	}

	// Not trusted - show hooks and ask for consent
	return promptForTrust(registry, repoPath, hooksPath)
}

// promptForTrust displays hooks and asks the user for consent
func promptForTrust(registry *TrustRegistry, repoPath, hooksPath string) error {
	// Read hooks content
	content, err := os.ReadFile(hooksPath)
	if err != nil {
		return fmt.Errorf("failed to read hooks file: %w", err)
	}

	// Display warning
	fmt.Println()
	fmt.Printf("  %s\n", "Repository hooks detected!")
	fmt.Printf("  %s: %s\n", "Repo", repoPath)
	fmt.Printf("  %s: %s\n", "File", hooksPath)
	fmt.Println()
	fmt.Println("  These hooks will run shell commands when you create, resume, or remove worktrees.")
	fmt.Println("  Please review them carefully:")
	fmt.Println()
	fmt.Println("  ---")
	fmt.Println(string(content))
	fmt.Println("  ---")
	fmt.Println()

	// Prompt for trust
	prompt := promptui.Prompt{
		Label:     "Trust these hooks",
		IsConfirm: true,
		Default:   "n",
	}

	_, err = prompt.Run()
	if err != nil {
		return fmt.Errorf("hooks not trusted - aborting")
	}

	// User approved - save trust
	if err := registry.Trust(repoPath, hooksPath); err != nil {
		return fmt.Errorf("failed to save trust: %w", err)
	}

	fmt.Println("  Hooks trusted.")
	fmt.Println()

	return nil
}
