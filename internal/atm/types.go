// Package atm provides a client for the ATM task management API via the atm-cli binary.
package atm

import (
	"regexp"
	"strings"
)

// Plan status constants.
const (
	PlanStatusDraft    = "draft"
	PlanStatusReady    = "ready"
	PlanStatusActive   = "active"
	PlanStatusComplete = "complete"
	PlanStatusBlocked  = "blocked"
)

// Task status constants.
const (
	TaskStatusTodo    = "todo"
	TaskStatusClaimed = "claimed"
	TaskStatusDoing   = "doing"
	TaskStatusDone    = "done"
	TaskStatusBlocked = "blocked"
	TaskStatusSkipped = "skipped"
)

// Project represents an ATM project.
type Project struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	Description    string `json:"description"`
	SlackChannelID string `json:"slack_channel_id"`
	GithubRepo     string `json:"github_repo"`
	PlansCount     int    `json:"plans_count,omitempty"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// Plan represents an ATM plan.
type Plan struct {
	ID             int        `json:"id"`
	ProjectID      int        `json:"project_id"`
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	Status         string     `json:"status"`
	SlackThreadURL string     `json:"slack_thread_url"`
	FeatureBranch  string     `json:"feature_branch"`
	FeedbackCount  int        `json:"feedback_count,omitempty"`
	ProgressCount  int        `json:"progress_count,omitempty"`
	TasksCount     int        `json:"tasks_count,omitempty"`
	Project        *Project   `json:"project,omitempty"`
	Tasks          []Task     `json:"tasks,omitempty"`
	Feedback       []Feedback `json:"feedback,omitempty"`
	Progress       []Progress `json:"progress,omitempty"`
	CreatedAt      string     `json:"created_at"`
	UpdatedAt      string     `json:"updated_at"`
}

// BranchName returns the feature branch for this plan.
// If FeatureBranch is set, it's returned as-is.
// Otherwise, a branch name is generated from the plan title: "feat/<slugified-title>".
func (p *Plan) BranchName() string {
	if p.FeatureBranch != "" {
		return p.FeatureBranch
	}
	return "feat/" + slugify(p.Title)
}

// slugify converts a string to a git-safe branch name component.
// "AKB -> ATM Integration" becomes "akb-atm-integration".
var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "->", "")
	s = nonAlphanumRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// Task represents an ATM task.
type Task struct {
	ID                 int         `json:"id"`
	PlanID             int         `json:"plan_id"`
	ParentID           *int        `json:"parent_id"`
	Title              string      `json:"title"`
	Description        string      `json:"description"`
	Status             string      `json:"status"`
	Assignee           string      `json:"assignee"`
	Position           int         `json:"position"`
	StartedAt          string      `json:"started_at"`
	CompletedAt        string      `json:"completed_at"`
	Children           []Task      `json:"children,omitempty"`
	AcceptanceCriteria []Criterion `json:"acceptance_criteria,omitempty"`
	BlockedBy          []Task      `json:"blocked_by,omitempty"`
	CreatedAt          string      `json:"created_at"`
	UpdatedAt          string      `json:"updated_at"`
}

// Criterion represents an acceptance criterion for a task.
type Criterion struct {
	ID          int    `json:"id"`
	TaskID      int    `json:"task_id"`
	Description string `json:"description"`
	IsChecked   bool   `json:"is_checked"`
	Reason      string `json:"reason"`
	Position    int    `json:"position"`
	CheckedAt   string `json:"checked_at"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// Progress represents a progress entry on a plan.
type Progress struct {
	ID        int    `json:"id"`
	PlanID    int    `json:"plan_id"`
	Author    string `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

// Feedback represents a feedback entry on a plan.
type Feedback struct {
	ID        int    `json:"id"`
	PlanID    int    `json:"plan_id"`
	Author    string `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

// Stats holds task status counts from the agent context endpoint.
type Stats struct {
	TotalTasks int `json:"total_tasks"`
	Done       int `json:"done"`
	Doing      int `json:"doing"`
	Claimed    int `json:"claimed"`
	Blocked    int `json:"blocked"`
	Available  int `json:"available"`
	Skipped    int `json:"skipped"`
}

// AgentContext is the composite response from the project agent-context endpoint.
type AgentContext struct {
	Project        Project    `json:"project"`
	Plan           Plan       `json:"plan"`
	RecentProgress []Progress `json:"recent_progress"`
	RecentFeedback []Feedback `json:"recent_feedback"`
	AvailableTasks []Task     `json:"available_tasks"`
	BlockedTasks   []Task     `json:"blocked_tasks"`
	Stats          Stats      `json:"stats"`
}

// TaskListOpts holds optional filters for listing tasks.
type TaskListOpts struct {
	Status    string // Filter by task status (e.g., "todo", "doing").
	Available bool   // Show only available (unblocked todo) tasks.
}
