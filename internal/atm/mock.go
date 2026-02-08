package atm

// MockCall records a method call on MockATM.
type MockCall struct {
	Method string
	Args   []interface{}
}

// MockATM is a test double for the ATM interface.
// Each method has a corresponding XxxFunc field; if set, it is called.
// Otherwise a sensible zero-value is returned (nil/empty, nil error).
// All calls are recorded in Calls.
type MockATM struct {
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
}

// Compile-time check that MockATM implements ATM.
var _ ATM = (*MockATM)(nil)

// NewMockATM returns a new MockATM with no hooks set.
func NewMockATM() *MockATM {
	return &MockATM{}
}

func (m *MockATM) record(method string, args ...interface{}) {
	m.Calls = append(m.Calls, MockCall{Method: method, Args: args})
}

func (m *MockATM) ProjectContext(slug string) (*AgentContext, error) {
	m.record("ProjectContext", slug)
	if m.ProjectContextFunc != nil {
		return m.ProjectContextFunc(slug)
	}
	return &AgentContext{}, nil
}

func (m *MockATM) PlanContext(planID int) (*AgentContext, error) {
	m.record("PlanContext", planID)
	if m.PlanContextFunc != nil {
		return m.PlanContextFunc(planID)
	}
	return &AgentContext{}, nil
}

func (m *MockATM) PlanContextText(planID int) (string, error) {
	m.record("PlanContextText", planID)
	if m.PlanContextTextFunc != nil {
		return m.PlanContextTextFunc(planID)
	}
	return "", nil
}

func (m *MockATM) ListPlans(projectSlug, status string) ([]Plan, error) {
	m.record("ListPlans", projectSlug, status)
	if m.ListPlansFunc != nil {
		return m.ListPlansFunc(projectSlug, status)
	}
	return nil, nil
}

func (m *MockATM) GetPlan(id int) (*Plan, error) {
	m.record("GetPlan", id)
	if m.GetPlanFunc != nil {
		return m.GetPlanFunc(id)
	}
	return nil, nil
}

func (m *MockATM) UpdatePlanStatus(id int, status string) (*Plan, error) {
	m.record("UpdatePlanStatus", id, status)
	if m.UpdatePlanStatusFunc != nil {
		return m.UpdatePlanStatusFunc(id, status)
	}
	return nil, nil
}

func (m *MockATM) ListTasks(planID int, opts *TaskListOpts) ([]Task, error) {
	m.record("ListTasks", planID, opts)
	if m.ListTasksFunc != nil {
		return m.ListTasksFunc(planID, opts)
	}
	return nil, nil
}

func (m *MockATM) GetTask(id int) (*Task, error) {
	m.record("GetTask", id)
	if m.GetTaskFunc != nil {
		return m.GetTaskFunc(id)
	}
	return nil, nil
}

func (m *MockATM) ClaimTask(id int, assignee string) (*Task, error) {
	m.record("ClaimTask", id, assignee)
	if m.ClaimTaskFunc != nil {
		return m.ClaimTaskFunc(id, assignee)
	}
	return nil, nil
}

func (m *MockATM) StartTask(id int) (*Task, error) {
	m.record("StartTask", id)
	if m.StartTaskFunc != nil {
		return m.StartTaskFunc(id)
	}
	return nil, nil
}

func (m *MockATM) CompleteTask(id int) (*Task, error) {
	m.record("CompleteTask", id)
	if m.CompleteTaskFunc != nil {
		return m.CompleteTaskFunc(id)
	}
	return nil, nil
}

func (m *MockATM) BlockTask(id int, reason string) (*Task, error) {
	m.record("BlockTask", id, reason)
	if m.BlockTaskFunc != nil {
		return m.BlockTaskFunc(id, reason)
	}
	return nil, nil
}

func (m *MockATM) SkipTask(id int, reason string) (*Task, error) {
	m.record("SkipTask", id, reason)
	if m.SkipTaskFunc != nil {
		return m.SkipTaskFunc(id, reason)
	}
	return nil, nil
}

func (m *MockATM) AddProgress(planID int, author, body string) (*Progress, error) {
	m.record("AddProgress", planID, author, body)
	if m.AddProgressFunc != nil {
		return m.AddProgressFunc(planID, author, body)
	}
	return nil, nil
}

func (m *MockATM) AddFeedback(planID int, author, body string) (*Feedback, error) {
	m.record("AddFeedback", planID, author, body)
	if m.AddFeedbackFunc != nil {
		return m.AddFeedbackFunc(planID, author, body)
	}
	return nil, nil
}

func (m *MockATM) CheckCriterion(id int) (*Criterion, error) {
	m.record("CheckCriterion", id)
	if m.CheckCriterionFunc != nil {
		return m.CheckCriterionFunc(id)
	}
	return nil, nil
}

func (m *MockATM) UncheckCriterion(id int) (*Criterion, error) {
	m.record("UncheckCriterion", id)
	if m.UncheckCriterionFunc != nil {
		return m.UncheckCriterionFunc(id)
	}
	return nil, nil
}
