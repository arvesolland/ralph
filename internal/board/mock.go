package board

// MockCall records a method call on MockBoard.
type MockCall struct {
	Method string
	Args   []interface{}
}

// MockBoard is a test double for the Board interface.
// Each method has a corresponding XxxFunc field; if set, it is called.
// Otherwise a sensible zero-value is returned (nil/empty, nil error).
// All calls are recorded in Calls.
type MockBoard struct {
	Calls []MockCall

	ProjectContextFunc  func(slug string) (*AgentContext, error)
	PlanContextFunc     func(planID int) (*AgentContext, error)
	PlanContextTextFunc func(planID int) (string, error)
	ListPlansFunc       func(projectSlug, status string) ([]Plan, error)
	GetPlanFunc         func(id int) (*Plan, error)
	UpdatePlanStatusFunc func(id int, status string) (*Plan, error)
	ListTasksFunc       func(planID int, opts *TaskListOpts) ([]Task, error)
	GetTaskFunc         func(id int) (*Task, error)
	ClaimTaskFunc       func(id int, assignee string) (*Task, error)
	StartTaskFunc       func(id int) (*Task, error)
	CompleteTaskFunc    func(id int) (*Task, error)
	BlockTaskFunc       func(id int, reason string) (*Task, error)
	SkipTaskFunc        func(id int, reason string) (*Task, error)
	AddProgressFunc     func(planID int, author, body string) (*Progress, error)
	AddFeedbackFunc     func(planID int, author, body string) (*Feedback, error)
	CheckCriterionFunc  func(id int) (*Criterion, error)
	UncheckCriterionFunc func(id int) (*Criterion, error)
	UpdatePlanFunc      func(id int, fields map[string]string) (*Plan, error)
}

// Compile-time check that MockBoard implements Board.
var _ Board = (*MockBoard)(nil)

// NewMockBoard returns a new MockBoard with no hooks set.
func NewMockBoard() *MockBoard {
	return &MockBoard{}
}

func (m *MockBoard) record(method string, args ...interface{}) {
	m.Calls = append(m.Calls, MockCall{Method: method, Args: args})
}

func (m *MockBoard) ProjectContext(slug string) (*AgentContext, error) {
	m.record("ProjectContext", slug)
	if m.ProjectContextFunc != nil {
		return m.ProjectContextFunc(slug)
	}
	return &AgentContext{}, nil
}

func (m *MockBoard) PlanContext(planID int) (*AgentContext, error) {
	m.record("PlanContext", planID)
	if m.PlanContextFunc != nil {
		return m.PlanContextFunc(planID)
	}
	return &AgentContext{}, nil
}

func (m *MockBoard) PlanContextText(planID int) (string, error) {
	m.record("PlanContextText", planID)
	if m.PlanContextTextFunc != nil {
		return m.PlanContextTextFunc(planID)
	}
	return "", nil
}

func (m *MockBoard) ListPlans(projectSlug, status string) ([]Plan, error) {
	m.record("ListPlans", projectSlug, status)
	if m.ListPlansFunc != nil {
		return m.ListPlansFunc(projectSlug, status)
	}
	return nil, nil
}

func (m *MockBoard) GetPlan(id int) (*Plan, error) {
	m.record("GetPlan", id)
	if m.GetPlanFunc != nil {
		return m.GetPlanFunc(id)
	}
	return nil, nil
}

func (m *MockBoard) UpdatePlanStatus(id int, status string) (*Plan, error) {
	m.record("UpdatePlanStatus", id, status)
	if m.UpdatePlanStatusFunc != nil {
		return m.UpdatePlanStatusFunc(id, status)
	}
	return nil, nil
}

func (m *MockBoard) ListTasks(planID int, opts *TaskListOpts) ([]Task, error) {
	m.record("ListTasks", planID, opts)
	if m.ListTasksFunc != nil {
		return m.ListTasksFunc(planID, opts)
	}
	return nil, nil
}

func (m *MockBoard) GetTask(id int) (*Task, error) {
	m.record("GetTask", id)
	if m.GetTaskFunc != nil {
		return m.GetTaskFunc(id)
	}
	return nil, nil
}

func (m *MockBoard) ClaimTask(id int, assignee string) (*Task, error) {
	m.record("ClaimTask", id, assignee)
	if m.ClaimTaskFunc != nil {
		return m.ClaimTaskFunc(id, assignee)
	}
	return nil, nil
}

func (m *MockBoard) StartTask(id int) (*Task, error) {
	m.record("StartTask", id)
	if m.StartTaskFunc != nil {
		return m.StartTaskFunc(id)
	}
	return nil, nil
}

func (m *MockBoard) CompleteTask(id int) (*Task, error) {
	m.record("CompleteTask", id)
	if m.CompleteTaskFunc != nil {
		return m.CompleteTaskFunc(id)
	}
	return nil, nil
}

func (m *MockBoard) BlockTask(id int, reason string) (*Task, error) {
	m.record("BlockTask", id, reason)
	if m.BlockTaskFunc != nil {
		return m.BlockTaskFunc(id, reason)
	}
	return nil, nil
}

func (m *MockBoard) SkipTask(id int, reason string) (*Task, error) {
	m.record("SkipTask", id, reason)
	if m.SkipTaskFunc != nil {
		return m.SkipTaskFunc(id, reason)
	}
	return nil, nil
}

func (m *MockBoard) AddProgress(planID int, author, body string) (*Progress, error) {
	m.record("AddProgress", planID, author, body)
	if m.AddProgressFunc != nil {
		return m.AddProgressFunc(planID, author, body)
	}
	return nil, nil
}

func (m *MockBoard) AddFeedback(planID int, author, body string) (*Feedback, error) {
	m.record("AddFeedback", planID, author, body)
	if m.AddFeedbackFunc != nil {
		return m.AddFeedbackFunc(planID, author, body)
	}
	return nil, nil
}

func (m *MockBoard) CheckCriterion(id int) (*Criterion, error) {
	m.record("CheckCriterion", id)
	if m.CheckCriterionFunc != nil {
		return m.CheckCriterionFunc(id)
	}
	return nil, nil
}

func (m *MockBoard) UncheckCriterion(id int) (*Criterion, error) {
	m.record("UncheckCriterion", id)
	if m.UncheckCriterionFunc != nil {
		return m.UncheckCriterionFunc(id)
	}
	return nil, nil
}

func (m *MockBoard) UpdatePlan(id int, fields map[string]string) (*Plan, error) {
	m.record("UpdatePlan", id, fields)
	if m.UpdatePlanFunc != nil {
		return m.UpdatePlanFunc(id, fields)
	}
	return nil, nil
}
