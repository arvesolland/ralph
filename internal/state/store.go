package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// StateFilename is the name of the state file within a plan bundle.
const StateFilename = "state.yaml"

// StatePath returns the path to state.yaml within a plan bundle directory.
func StatePath(bundleDir string) string {
	return filepath.Join(bundleDir, StateFilename)
}

// LoadState reads and parses state.yaml from a plan bundle directory.
// Returns (nil, nil) if the file does not exist (backward compat — plan has no structured state yet).
// Returns a parse error if the file exists but contains invalid YAML.
func LoadState(bundleDir string) (*PlanState, error) {
	path := StatePath(bundleDir)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var state PlanState
	if err := yaml.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	return &state, nil
}

// SaveState writes the state atomically to state.yaml in the plan bundle directory.
// It writes to a temp file first, then renames for crash safety.
func SaveState(state *PlanState, bundleDir string) error {
	data, err := yaml.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	path := StatePath(bundleDir)

	// Ensure parent directory exists
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write to temp file first for atomic save
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp state file: %w", err)
	}

	// Rename temp file to target path (atomic on POSIX)
	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath) // Clean up temp file on failure
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	return nil
}
