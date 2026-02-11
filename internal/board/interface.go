package board

// Board defines the interface for interacting with the Board task management API.
// The concrete implementation is Client, which shells out to board-cli.
type Board interface {
	ProjectContext(slug string) (*AgentContext, error)
	PlanContext(planID int) (*AgentContext, error)
	PlanContextText(planID int) (string, error)
	ListPlans(projectSlug, status string) ([]Plan, error)
	GetPlan(id int) (*Plan, error)
	UpdatePlanStatus(id int, status string) (*Plan, error)
	ListTasks(planID int, opts *TaskListOpts) ([]Task, error)
	GetTask(id int) (*Task, error)
	ClaimTask(id int, assignee string) (*Task, error)
	StartTask(id int) (*Task, error)
	CompleteTask(id int) (*Task, error)
	BlockTask(id int, reason string) (*Task, error)
	SkipTask(id int, reason string) (*Task, error)
	AddProgress(planID int, author, body string) (*Progress, error)
	AddFeedback(planID int, author, body string) (*Feedback, error)
	CheckCriterion(id int) (*Criterion, error)
	UncheckCriterion(id int) (*Criterion, error)
	UpdatePlan(id int, fields map[string]string) (*Plan, error)
}
