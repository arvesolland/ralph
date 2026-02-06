// Package state manages structured plan state via state.yaml.
package state

import (
	"time"

	"gopkg.in/yaml.v3"
)

// PlanStatus represents the lifecycle status of a plan.
type PlanStatus string

const (
	PlanStatusDraft    PlanStatus = "draft"
	PlanStatusReady    PlanStatus = "ready"
	PlanStatusActive   PlanStatus = "active"
	PlanStatusBlocked  PlanStatus = "blocked"
	PlanStatusComplete PlanStatus = "complete"
)

// IsValid returns true if the PlanStatus is a recognized value.
func (s PlanStatus) IsValid() bool {
	switch s {
	case PlanStatusDraft, PlanStatusReady, PlanStatusActive, PlanStatusBlocked, PlanStatusComplete:
		return true
	}
	return false
}

// TaskStatus represents the lifecycle status of a task.
type TaskStatus string

const (
	TaskStatusTodo    TaskStatus = "todo"
	TaskStatusClaimed TaskStatus = "claimed"
	TaskStatusDoing   TaskStatus = "doing"
	TaskStatusBlocked TaskStatus = "blocked"
	TaskStatusDone    TaskStatus = "done"
	TaskStatusSkipped TaskStatus = "skipped"
)

// IsValid returns true if the TaskStatus is a recognized value.
func (s TaskStatus) IsValid() bool {
	switch s {
	case TaskStatusTodo, TaskStatusClaimed, TaskStatusDoing, TaskStatusBlocked, TaskStatusDone, TaskStatusSkipped:
		return true
	}
	return false
}

// PlanState is the root structured state for a plan bundle.
type PlanState struct {
	ID        string     `yaml:"id"         json:"id"`
	Title     string     `yaml:"title"      json:"title"`
	Status    PlanStatus `yaml:"status"     json:"status"`
	CreatedAt time.Time  `yaml:"created_at" json:"created_at"`
	Tasks     []TaskState `yaml:"tasks"     json:"tasks"`
	Feedback  []Feedback  `yaml:"feedback,omitempty"  json:"feedback,omitempty"`
}

// TaskState represents a single task within a plan.
type TaskState struct {
	ID          string     `yaml:"id"            json:"id"`
	Title       string     `yaml:"title"         json:"title"`
	Description string     `yaml:"description,omitempty" json:"description,omitempty"`
	Status      TaskStatus `yaml:"status"        json:"status"`
	Requires    []string   `yaml:"requires,omitempty"    json:"requires,omitempty"`
	Criteria    []Criterion `yaml:"criteria,omitempty"   json:"criteria,omitempty"`
	Notes       []string   `yaml:"notes,omitempty"       json:"notes,omitempty"`
	Artifacts   Artifacts  `yaml:"artifacts,omitempty"   json:"artifacts,omitempty"`
	StartedAt   *time.Time `yaml:"started_at,omitempty"  json:"started_at,omitempty"`
	DoneAt      *time.Time `yaml:"done_at,omitempty"     json:"done_at,omitempty"`
}

// Criterion represents a single acceptance criterion for a task.
type Criterion struct {
	Text   string     `yaml:"text"              json:"text"`
	Done   bool       `yaml:"done"              json:"done"`
	DoneAt *time.Time `yaml:"done_at,omitempty" json:"done_at,omitempty"`
}

// UnmarshalYAML handles both string and struct formats for Criterion.
// LLMs frequently output criteria as plain strings instead of {text, done} objects.
func (c *Criterion) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		c.Text = value.Value
		c.Done = false
		return nil
	}
	type rawCriterion Criterion
	var raw rawCriterion
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*c = Criterion(raw)
	return nil
}

// Artifacts tracks what a task produced.
type Artifacts struct {
	Commits      []string `yaml:"commits,omitempty"       json:"commits,omitempty"`
	FilesTouched []string `yaml:"files_touched,omitempty" json:"files_touched,omitempty"`
}

// Feedback represents a feedback entry scoped to a plan or specific task.
type Feedback struct {
	ID         string     `yaml:"id"                    json:"id"`
	Scope      string     `yaml:"scope"                 json:"scope"`
	Author     string     `yaml:"author"                json:"author"`
	Message    string     `yaml:"message"               json:"message"`
	Resolved   bool       `yaml:"resolved"              json:"resolved"`
	ResolvedAt *time.Time `yaml:"resolved_at,omitempty" json:"resolved_at,omitempty"`
	CreatedAt  time.Time  `yaml:"created_at"            json:"created_at"`
}
