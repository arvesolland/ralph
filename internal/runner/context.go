// Package runner provides Claude CLI execution and iteration context management.
package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/arvesolland/ralph/internal/plan"
)

// Context represents the execution state for a plan iteration.
// It is persisted as context.json in the worktree to maintain state between iterations.
type Context struct {
	// PlanFile is the path to the plan file being executed
	PlanFile string `json:"planFile"`

	// FeatureBranch is the git branch for this plan (e.g., "feat/go-rewrite")
	FeatureBranch string `json:"featureBranch"`

	// BaseBranch is the base branch to merge into (e.g., "main")
	BaseBranch string `json:"baseBranch"`

	// Iteration is the current iteration number (1-indexed)
	Iteration int `json:"iteration"`

	// MaxIterations is the maximum allowed iterations before failure
	MaxIterations int `json:"maxIterations"`
}

// DefaultMaxIterations is the default maximum number of iterations
const DefaultMaxIterations = 30

// ContextFilename is the filename for context files in worktrees
const ContextFilename = "context.json"

// NewContext creates a new Context from a plan.
// The context is initialized for the first iteration with the specified base branch and max iterations.
func NewContext(p *plan.Plan, baseBranch string, maxIterations int) *Context {
	if maxIterations <= 0 {
		maxIterations = DefaultMaxIterations
	}
	return &Context{
		PlanFile:      p.Path,
		FeatureBranch: p.Branch,
		BaseBranch:    baseBranch,
		Iteration:     1,
		MaxIterations: maxIterations,
	}
}

// LoadContext reads a context from a JSON file.
// Returns an error if the file doesn't exist or is invalid JSON.
func LoadContext(path string) (*Context, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read context file: %w", err)
	}

	var ctx Context
	if err := json.Unmarshal(data, &ctx); err != nil {
		return nil, fmt.Errorf("failed to parse context file: %w", err)
	}

	return &ctx, nil
}

// SaveContext writes the context to a JSON file.
// The file is written atomically (write to temp, then rename).
func SaveContext(ctx *Context, path string) error {
	data, err := json.MarshalIndent(ctx, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal context: %w", err)
	}

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write to temp file first for atomic save
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp context file: %w", err)
	}

	// Rename temp file to target path (atomic on POSIX)
	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath) // Clean up temp file on failure
		return fmt.Errorf("failed to rename context file: %w", err)
	}

	return nil
}

// ContextPath returns the path to the context file within a worktree.
// The context file is stored at .ralph/context.json in the worktree root.
func ContextPath(worktreePath string) string {
	return filepath.Join(worktreePath, ".ralph", ContextFilename)
}

// Increment increments the iteration count and returns a copy of the context.
func (c *Context) Increment() *Context {
	return &Context{
		PlanFile:      c.PlanFile,
		FeatureBranch: c.FeatureBranch,
		BaseBranch:    c.BaseBranch,
		Iteration:     c.Iteration + 1,
		MaxIterations: c.MaxIterations,
	}
}

// IsMaxReached returns true if the current iteration exceeds the maximum allowed.
func (c *Context) IsMaxReached() bool {
	return c.Iteration > c.MaxIterations
}
