package main

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/arvesolland/ralph/internal/atm"
)

// State is the JSON state file that persists all ATM data for the fake CLI.
type State struct {
	Projects []atm.Project  `json:"projects"`
	Plans    []atm.Plan     `json:"plans"`
	Tasks    []atm.Task     `json:"tasks"`
	Criteria []atm.Criterion `json:"criteria"`
	Progress []atm.Progress `json:"progress"`
	Feedback []atm.Feedback `json:"feedback"`

	// Auto-increment counters for each entity type.
	NextProjectID  int `json:"next_project_id"`
	NextPlanID     int `json:"next_plan_id"`
	NextTaskID     int `json:"next_task_id"`
	NextCriterionID int `json:"next_criterion_id"`
	NextProgressID int `json:"next_progress_id"`
	NextFeedbackID int `json:"next_feedback_id"`
}

// LoadState reads the state file at path. If the file does not exist, returns
// an empty state with counters starting at 1.
func LoadState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{
				NextProjectID:   1,
				NextPlanID:      1,
				NextTaskID:      1,
				NextCriterionID: 1,
				NextProgressID:  1,
				NextFeedbackID:  1,
			}, nil
		}
		return nil, fmt.Errorf("reading state file: %w", err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing state file: %w", err)
	}
	return &s, nil
}

// SaveState writes the state to the given path atomically.
func SaveState(path string, s *State) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing state file: %w", err)
	}
	return nil
}

// WithState performs a read-modify-write operation on the state file with
// file-level locking to handle concurrent access from the Go orchestrator
// and the Claude agent.
func WithState(path string, fn func(*State) error) error {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("opening state file: %w", err)
	}
	defer f.Close()

	// Acquire exclusive lock.
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("locking state file: %w", err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

	// Read current state.
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading state file: %w", err)
	}

	var s State
	if len(data) > 0 {
		if err := json.Unmarshal(data, &s); err != nil {
			return fmt.Errorf("parsing state file: %w", err)
		}
	} else {
		s = State{
			NextProjectID:   1,
			NextPlanID:      1,
			NextTaskID:      1,
			NextCriterionID: 1,
			NextProgressID:  1,
			NextFeedbackID:  1,
		}
	}

	// Apply mutation.
	if err := fn(&s); err != nil {
		return err
	}

	// Write back.
	out, err := json.MarshalIndent(&s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}
	if err := f.Truncate(0); err != nil {
		return fmt.Errorf("truncating state file: %w", err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		return fmt.Errorf("seeking state file: %w", err)
	}
	if _, err := f.Write(out); err != nil {
		return fmt.Errorf("writing state file: %w", err)
	}

	return nil
}

// now returns the current time as an RFC3339 string.
func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// Seed helpers for test setup.

// SeedProject adds a project to the state and returns it with its assigned ID.
func SeedProject(s *State, name, slug string) atm.Project {
	p := atm.Project{
		ID:        s.NextProjectID,
		Name:      name,
		Slug:      slug,
		CreatedAt: now(),
		UpdatedAt: now(),
	}
	s.NextProjectID++
	s.Projects = append(s.Projects, p)
	return p
}

// SeedPlan adds a plan to the state and returns it with its assigned ID.
func SeedPlan(s *State, projectID int, title, status, branch string) atm.Plan {
	p := atm.Plan{
		ID:            s.NextPlanID,
		ProjectID:     projectID,
		Title:         title,
		Status:        status,
		FeatureBranch: branch,
		CreatedAt:     now(),
		UpdatedAt:     now(),
	}
	s.NextPlanID++
	s.Plans = append(s.Plans, p)
	return p
}

// SeedTask adds a task to the state and returns it with its assigned ID.
func SeedTask(s *State, planID int, title, description string, deps []int) atm.Task {
	t := atm.Task{
		ID:          s.NextTaskID,
		PlanID:      planID,
		Title:       title,
		Description: description,
		Status:      atm.TaskStatusTodo,
		Position:    len(tasksForPlan(s, planID)) + 1,
		CreatedAt:   now(),
		UpdatedAt:   now(),
	}
	// Store dependency info as BlockedBy references (IDs only).
	for _, depID := range deps {
		t.BlockedBy = append(t.BlockedBy, atm.Task{ID: depID})
	}
	s.NextTaskID++
	s.Tasks = append(s.Tasks, t)
	return t
}

// SeedCriterion adds a criterion to the state and returns it with its assigned ID.
func SeedCriterion(s *State, taskID int, description string) atm.Criterion {
	c := atm.Criterion{
		ID:          s.NextCriterionID,
		TaskID:      taskID,
		Description: description,
		Position:    len(criteriaForTask(s, taskID)) + 1,
		CreatedAt:   now(),
		UpdatedAt:   now(),
	}
	s.NextCriterionID++
	s.Criteria = append(s.Criteria, c)
	return c
}

// Helper lookups.

func findProject(s *State, slug string) *atm.Project {
	for i := range s.Projects {
		if s.Projects[i].Slug == slug {
			return &s.Projects[i]
		}
	}
	return nil
}

func findProjectByID(s *State, id int) *atm.Project {
	for i := range s.Projects {
		if s.Projects[i].ID == id {
			return &s.Projects[i]
		}
	}
	return nil
}

func findPlan(s *State, id int) *atm.Plan {
	for i := range s.Plans {
		if s.Plans[i].ID == id {
			return &s.Plans[i]
		}
	}
	return nil
}

func findTask(s *State, id int) *atm.Task {
	for i := range s.Tasks {
		if s.Tasks[i].ID == id {
			return &s.Tasks[i]
		}
	}
	return nil
}

func findCriterion(s *State, id int) *atm.Criterion {
	for i := range s.Criteria {
		if s.Criteria[i].ID == id {
			return &s.Criteria[i]
		}
	}
	return nil
}

func tasksForPlan(s *State, planID int) []atm.Task {
	var result []atm.Task
	for _, t := range s.Tasks {
		if t.PlanID == planID {
			result = append(result, t)
		}
	}
	return result
}

func criteriaForTask(s *State, taskID int) []atm.Criterion {
	var result []atm.Criterion
	for _, c := range s.Criteria {
		if c.TaskID == taskID {
			result = append(result, c)
		}
	}
	return result
}

func progressForPlan(s *State, planID int) []atm.Progress {
	var result []atm.Progress
	for _, p := range s.Progress {
		if p.PlanID == planID {
			result = append(result, p)
		}
	}
	return result
}

func feedbackForPlan(s *State, planID int) []atm.Feedback {
	var result []atm.Feedback
	for _, f := range s.Feedback {
		if f.PlanID == planID {
			result = append(result, f)
		}
	}
	return result
}

func plansForProject(s *State, projectID int) []atm.Plan {
	var result []atm.Plan
	for _, p := range s.Plans {
		if p.ProjectID == projectID {
			result = append(result, p)
		}
	}
	return result
}

// activePlanForProject returns the first plan with status "active" for the project.
func activePlanForProject(s *State, projectID int) *atm.Plan {
	for i := range s.Plans {
		if s.Plans[i].ProjectID == projectID && s.Plans[i].Status == atm.PlanStatusActive {
			return &s.Plans[i]
		}
	}
	return nil
}

// computeStats calculates task statistics for a plan.
func computeStats(s *State, planID int) atm.Stats {
	tasks := tasksForPlan(s, planID)
	var stats atm.Stats
	stats.TotalTasks = len(tasks)
	for _, t := range tasks {
		switch t.Status {
		case atm.TaskStatusDone:
			stats.Done++
		case atm.TaskStatusDoing:
			stats.Doing++
		case atm.TaskStatusClaimed:
			stats.Claimed++
		case atm.TaskStatusBlocked:
			stats.Blocked++
		case atm.TaskStatusSkipped:
			stats.Skipped++
		case atm.TaskStatusTodo:
			if isAvailable(s, &t) {
				stats.Available++
			}
		}
	}
	return stats
}

// isAvailable returns true if a task is todo and all its dependencies are done or skipped.
func isAvailable(s *State, t *atm.Task) bool {
	if t.Status != atm.TaskStatusTodo {
		return false
	}
	for _, dep := range t.BlockedBy {
		dt := findTask(s, dep.ID)
		if dt == nil {
			return false
		}
		if dt.Status != atm.TaskStatusDone && dt.Status != atm.TaskStatusSkipped {
			return false
		}
	}
	return true
}

// availableTasks returns all tasks for a plan that are available (todo + deps satisfied).
func availableTasks(s *State, planID int) []atm.Task {
	var result []atm.Task
	for _, t := range tasksForPlan(s, planID) {
		if isAvailable(s, &t) {
			result = append(result, t)
		}
	}
	return result
}

// blockedTasks returns all tasks for a plan with status "blocked".
func blockedTasks(s *State, planID int) []atm.Task {
	var result []atm.Task
	for _, t := range tasksForPlan(s, planID) {
		if t.Status == atm.TaskStatusBlocked {
			result = append(result, t)
		}
	}
	return result
}

// enrichTask adds acceptance_criteria and blocked_by data to a task.
func enrichTask(s *State, t atm.Task) atm.Task {
	t.AcceptanceCriteria = criteriaForTask(s, t.ID)
	// BlockedBy is already stored on the task as ID references.
	// Enrich with full task data for the response.
	var enrichedDeps []atm.Task
	for _, dep := range t.BlockedBy {
		dt := findTask(s, dep.ID)
		if dt != nil {
			enrichedDeps = append(enrichedDeps, *dt)
		}
	}
	t.BlockedBy = enrichedDeps
	return t
}

// enrichPlan adds tasks, feedback, and progress data to a plan.
func enrichPlan(s *State, p atm.Plan) atm.Plan {
	tasks := tasksForPlan(s, p.ID)
	for i := range tasks {
		tasks[i] = enrichTask(s, tasks[i])
	}
	p.Tasks = tasks
	p.TasksCount = len(tasks)
	p.Feedback = feedbackForPlan(s, p.ID)
	p.FeedbackCount = len(p.Feedback)
	p.Progress = progressForPlan(s, p.ID)
	p.ProgressCount = len(p.Progress)
	return p
}
