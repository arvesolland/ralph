// Package plan handles plan parsing and queue management.
package plan

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Queue manages the plan queue lifecycle: pending → current → complete.
type Queue struct {
	// BaseDir is the base directory containing the queue subdirectories.
	// Typically "plans/" containing pending/, current/, complete/ subdirectories.
	BaseDir string
}

// QueueStatus contains counts for each queue state.
type QueueStatus struct {
	// PendingCount is the number of plans waiting to be processed.
	PendingCount int

	// CurrentCount is the number of plans currently being processed (0 or 1).
	CurrentCount int

	// CompleteCount is the number of plans that have been completed.
	CompleteCount int

	// PendingPlans contains the names of pending plans.
	PendingPlans []string

	// CurrentPlan is the name of the current plan, if any.
	CurrentPlan string
}

var (
	// ErrQueueFull is returned when trying to activate a plan while current/ is not empty.
	ErrQueueFull = errors.New("queue full: current directory already has a plan")

	// ErrNoCurrent is returned when trying to complete or reset but no current plan exists.
	ErrNoCurrent = errors.New("no current plan")

	// ErrPlanNotInPending is returned when trying to activate a plan that's not in pending/.
	ErrPlanNotInPending = errors.New("plan is not in pending directory")

	// ErrPlanNotInCurrent is returned when trying to complete a plan that's not in current/.
	ErrPlanNotInCurrent = errors.New("plan is not in current directory")
)

// NewQueue creates a new Queue with the given base directory.
func NewQueue(baseDir string) *Queue {
	return &Queue{BaseDir: baseDir}
}

// pendingDir returns the path to the pending/ directory.
func (q *Queue) pendingDir() string {
	return filepath.Join(q.BaseDir, "pending")
}

// currentDir returns the path to the current/ directory.
func (q *Queue) currentDir() string {
	return filepath.Join(q.BaseDir, "current")
}

// completeDir returns the path to the complete/ directory.
func (q *Queue) completeDir() string {
	return filepath.Join(q.BaseDir, "complete")
}

// resolvePath resolves a path to its absolute form with symlinks evaluated.
// Returns the original path on error for graceful degradation.
func resolvePath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// Path may not exist yet, return absolute path
		return abs
	}
	return resolved
}

// planDir returns the directory containing the plan.
// For bundles: returns BundleDir (the plan IS the directory)
// For flat files: returns filepath.Dir(Path) (the directory containing the plan.md)
func planDir(p *Plan) string {
	if p.IsBundle() {
		return p.BundleDir
	}
	return filepath.Dir(p.Path)
}

// uniqueCompleteName generates a unique name for completed bundles.
// Format: name-YYYYMMDD or name-YYYYMMDD-N for collisions.
// Example: "my-plan-20260201" or "my-plan-20260201-2"
func (q *Queue) uniqueCompleteName(name string) string {
	dateSuffix := time.Now().Format("20060102")
	baseName := name + "-" + dateSuffix

	// Check if base name is available
	candidatePath := filepath.Join(q.completeDir(), baseName)
	if _, err := os.Stat(candidatePath); os.IsNotExist(err) {
		return baseName
	}

	// Add counter suffix for collisions
	for i := 2; ; i++ {
		candidateName := fmt.Sprintf("%s-%d", baseName, i)
		candidatePath := filepath.Join(q.completeDir(), candidateName)
		if _, err := os.Stat(candidatePath); os.IsNotExist(err) {
			return candidateName
		}
	}
}

// Pending returns all plans in the pending/ directory, sorted by name.
func (q *Queue) Pending() ([]*Plan, error) {
	return q.listPlans(q.pendingDir())
}

// Current returns the plan in current/, or nil if empty.
// Returns an error if there are multiple plans in current/ (shouldn't happen).
func (q *Queue) Current() (*Plan, error) {
	plans, err := q.listPlans(q.currentDir())
	if err != nil {
		return nil, err
	}

	if len(plans) == 0 {
		return nil, nil
	}

	if len(plans) > 1 {
		return nil, fmt.Errorf("multiple plans in current directory: found %d", len(plans))
	}

	return plans[0], nil
}

// Activate moves a plan from pending/ to current/.
// For bundles: moves the entire directory.
// For flat files: moves just the .md file.
// Returns ErrQueueFull if current/ already has a plan.
// Returns ErrPlanNotInPending if the plan is not in pending/.
func (q *Queue) Activate(plan *Plan) error {
	// Check if current/ is empty
	current, err := q.Current()
	if err != nil {
		return fmt.Errorf("checking current queue: %w", err)
	}
	if current != nil {
		return ErrQueueFull
	}

	// Verify plan is in pending/
	pDir := planDir(plan)
	parentDir := resolvePath(filepath.Dir(pDir))
	pendingDir := resolvePath(q.pendingDir())

	// For flat files, the parent is the pending dir
	// For bundles, the bundle itself is in pending, so parent == pending
	if plan.IsBundle() {
		if parentDir != pendingDir {
			return ErrPlanNotInPending
		}
	} else {
		resolvedPDir := resolvePath(pDir)
		if resolvedPDir != pendingDir {
			return ErrPlanNotInPending
		}
	}

	if plan.IsBundle() {
		// Move entire bundle directory
		newBundleDir := filepath.Join(q.currentDir(), filepath.Base(plan.BundleDir))
		if err := os.Rename(plan.BundleDir, newBundleDir); err != nil {
			return fmt.Errorf("moving bundle to current: %w", err)
		}
		// Update plan's paths
		plan.BundleDir = newBundleDir
		plan.Path = filepath.Join(newBundleDir, "plan.md")
	} else {
		// Move just the .md file (legacy flat file)
		newPath := filepath.Join(q.currentDir(), filepath.Base(plan.Path))
		if err := os.Rename(plan.Path, newPath); err != nil {
			return fmt.Errorf("moving plan to current: %w", err)
		}
		plan.Path = newPath
	}

	return nil
}

// Complete moves a plan from current/ to complete/.
// For bundles: moves entire directory with date suffix (name-YYYYMMDD).
// For flat files: moves just the .md file.
// Returns ErrPlanNotInCurrent if the plan is not in current/.
func (q *Queue) Complete(plan *Plan) error {
	// Verify plan is in current/
	pDir := planDir(plan)
	parentDir := resolvePath(filepath.Dir(pDir))
	currentDir := resolvePath(q.currentDir())

	if plan.IsBundle() {
		if parentDir != currentDir {
			return ErrPlanNotInCurrent
		}
	} else {
		resolvedPDir := resolvePath(pDir)
		if resolvedPDir != currentDir {
			return ErrPlanNotInCurrent
		}
	}

	if plan.IsBundle() {
		// Move entire bundle directory with unique dated name
		uniqueName := q.uniqueCompleteName(plan.Name)
		newBundleDir := filepath.Join(q.completeDir(), uniqueName)
		if err := os.Rename(plan.BundleDir, newBundleDir); err != nil {
			return fmt.Errorf("moving bundle to complete: %w", err)
		}
		// Update plan's paths
		plan.BundleDir = newBundleDir
		plan.Path = filepath.Join(newBundleDir, "plan.md")
	} else {
		// Move just the .md file (legacy flat file)
		newPath := filepath.Join(q.completeDir(), filepath.Base(plan.Path))
		if err := os.Rename(plan.Path, newPath); err != nil {
			return fmt.Errorf("moving plan to complete: %w", err)
		}
		plan.Path = newPath
	}

	return nil
}

// Reset moves a plan from current/ back to pending/.
// For bundles: moves entire directory.
// For flat files: moves just the .md file.
// Returns ErrPlanNotInCurrent if the plan is not in current/.
func (q *Queue) Reset(plan *Plan) error {
	// Verify plan is in current/
	pDir := planDir(plan)
	parentDir := resolvePath(filepath.Dir(pDir))
	currentDir := resolvePath(q.currentDir())

	if plan.IsBundle() {
		if parentDir != currentDir {
			return ErrPlanNotInCurrent
		}
	} else {
		resolvedPDir := resolvePath(pDir)
		if resolvedPDir != currentDir {
			return ErrPlanNotInCurrent
		}
	}

	if plan.IsBundle() {
		// Move entire bundle directory back to pending
		newBundleDir := filepath.Join(q.pendingDir(), filepath.Base(plan.BundleDir))
		if err := os.Rename(plan.BundleDir, newBundleDir); err != nil {
			return fmt.Errorf("moving bundle to pending: %w", err)
		}
		// Update plan's paths
		plan.BundleDir = newBundleDir
		plan.Path = filepath.Join(newBundleDir, "plan.md")
	} else {
		// Move just the .md file (legacy flat file)
		newPath := filepath.Join(q.pendingDir(), filepath.Base(plan.Path))
		if err := os.Rename(plan.Path, newPath); err != nil {
			return fmt.Errorf("moving plan to pending: %w", err)
		}
		plan.Path = newPath
	}

	return nil
}

// Status returns the current queue status with counts and plan names.
func (q *Queue) Status() (*QueueStatus, error) {
	pending, err := q.Pending()
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("listing pending: %w", err)
	}

	current, err := q.Current()
	if err != nil {
		return nil, fmt.Errorf("getting current: %w", err)
	}

	complete, err := q.listPlans(q.completeDir())
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("listing complete: %w", err)
	}

	status := &QueueStatus{
		PendingCount:  len(pending),
		CurrentCount:  0,
		CompleteCount: len(complete),
		PendingPlans:  make([]string, len(pending)),
	}

	for i, p := range pending {
		status.PendingPlans[i] = p.Name
	}

	if current != nil {
		status.CurrentCount = 1
		status.CurrentPlan = current.Name
	}

	return status, nil
}

// listPlans lists all plans in the given directory.
// Scans for both:
// - Bundle directories (directories containing plan.md)
// - Legacy flat .md files (for backwards compatibility)
// Returns an empty slice if the directory doesn't exist.
func (q *Queue) listPlans(dir string) ([]*Plan, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Plan{}, nil
		}
		return nil, err
	}

	var plans []*Plan
	for _, entry := range entries {
		if entry.IsDir() {
			// Check if this is a bundle directory (contains plan.md)
			bundleDir := filepath.Join(dir, entry.Name())
			planPath := filepath.Join(bundleDir, "plan.md")
			if _, err := os.Stat(planPath); err == nil {
				// It's a bundle - load it
				plan, err := Load(bundleDir)
				if err != nil {
					return nil, fmt.Errorf("loading bundle %s: %w", bundleDir, err)
				}
				plans = append(plans, plan)
			}
			// Skip directories that aren't bundles
			continue
		}

		// Only process .md files (legacy flat files)
		if filepath.Ext(entry.Name()) != ".md" {
			continue
		}

		// Skip progress and feedback files
		name := entry.Name()
		if strings.HasSuffix(name, ".progress.md") {
			continue
		}
		if strings.HasSuffix(name, ".feedback.md") {
			continue
		}

		planPath := filepath.Join(dir, entry.Name())
		plan, err := Load(planPath)
		if err != nil {
			return nil, fmt.Errorf("loading plan %s: %w", planPath, err)
		}

		plans = append(plans, plan)
	}

	// Sort by name
	sort.Slice(plans, func(i, j int) bool {
		return plans[i].Name < plans[j].Name
	})

	return plans, nil
}
