// Package plan handles plan parsing and queue management.
package plan

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	if filepath.Dir(plan.Path) != q.pendingDir() {
		absDir, _ := filepath.Abs(filepath.Dir(plan.Path))
		absPending, _ := filepath.Abs(q.pendingDir())
		if absDir != absPending {
			return ErrPlanNotInPending
		}
	}

	// Move to current/
	newPath := filepath.Join(q.currentDir(), filepath.Base(plan.Path))
	if err := os.Rename(plan.Path, newPath); err != nil {
		return fmt.Errorf("moving plan to current: %w", err)
	}

	// Update plan's path
	plan.Path = newPath

	return nil
}

// Complete moves a plan from current/ to complete/.
// Returns ErrPlanNotInCurrent if the plan is not in current/.
func (q *Queue) Complete(plan *Plan) error {
	// Verify plan is in current/
	if filepath.Dir(plan.Path) != q.currentDir() {
		absDir, _ := filepath.Abs(filepath.Dir(plan.Path))
		absCurrent, _ := filepath.Abs(q.currentDir())
		if absDir != absCurrent {
			return ErrPlanNotInCurrent
		}
	}

	// Move to complete/
	newPath := filepath.Join(q.completeDir(), filepath.Base(plan.Path))
	if err := os.Rename(plan.Path, newPath); err != nil {
		return fmt.Errorf("moving plan to complete: %w", err)
	}

	// Update plan's path
	plan.Path = newPath

	return nil
}

// Reset moves a plan from current/ back to pending/.
// Returns ErrPlanNotInCurrent if the plan is not in current/.
func (q *Queue) Reset(plan *Plan) error {
	// Verify plan is in current/
	if filepath.Dir(plan.Path) != q.currentDir() {
		absDir, _ := filepath.Abs(filepath.Dir(plan.Path))
		absCurrent, _ := filepath.Abs(q.currentDir())
		if absDir != absCurrent {
			return ErrPlanNotInCurrent
		}
	}

	// Move to pending/
	newPath := filepath.Join(q.pendingDir(), filepath.Base(plan.Path))
	if err := os.Rename(plan.Path, newPath); err != nil {
		return fmt.Errorf("moving plan to pending: %w", err)
	}

	// Update plan's path
	plan.Path = newPath

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

// listPlans lists all .md files in the given directory as plans.
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
			continue
		}

		// Only process .md files
		if filepath.Ext(entry.Name()) != ".md" {
			continue
		}

		// Skip progress and feedback files
		name := entry.Name()
		if len(name) > 12 && name[len(name)-12:] == ".progress.md" {
			continue
		}
		if len(name) > 12 && name[len(name)-12:] == ".feedback.md" {
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
