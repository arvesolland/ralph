package atm

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"github.com/arvesolland/ralph/internal/retry"
)

// ExecTimeout is the timeout for atm-cli commands.
const ExecTimeout = 30 * time.Second

// ClientConfig holds configuration for creating a new Client.
type ClientConfig struct {
	BinPath  string // Path to atm-cli binary. Defaults to "atm-cli".
	APIURL   string // Base URL for the ATM API (--api-url flag).
	APIToken string // Bearer token for authentication (--api-token flag).
}

// Compile-time check that Client implements ATM.
var _ ATM = (*Client)(nil)

// Client shells out to the atm-cli binary to interact with the ATM API.
type Client struct {
	binPath  string
	apiURL   string
	apiToken string
	retrier  *retry.Retrier
}

// NewClient creates a new ATM client with the given configuration.
func NewClient(cfg ClientConfig) *Client {
	bin := cfg.BinPath
	if bin == "" {
		bin = "atm-cli"
	}
	return &Client{
		binPath:  bin,
		apiURL:   cfg.APIURL,
		apiToken: cfg.APIToken,
		retrier: retry.NewRetrier(retry.RetryConfig{
			MaxRetries:   3,
			InitialDelay: 2 * time.Second,
			MaxDelay:     30 * time.Second,
			JitterFactor: 0.25,
		}),
	}
}

// ProjectContext returns the full agent context for a project, including the
// active plan, available tasks, recent progress, and recent feedback.
func (c *Client) ProjectContext(slug string) (*AgentContext, error) {
	data, err := c.exec("project", "context", slug)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data AgentContext `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing agent context response: %w", err)
	}
	return &resp.Data, nil
}

// PlanContext returns the agent context for a specific plan (compact JSON).
// Used for programmatic checks like completion detection.
func (c *Client) PlanContext(planID int) (*AgentContext, error) {
	data, err := c.exec("plan", "context", strconv.Itoa(planID))
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data AgentContext `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing plan context response: %w", err)
	}
	return &resp.Data, nil
}

// PlanContextText returns the agent context for a plan as structured text
// optimized for LLM consumption. This is injected directly into the prompt.
func (c *Client) PlanContextText(planID int) (string, error) {
	data, err := c.exec("plan", "context", strconv.Itoa(planID), "--format", "text")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ListPlans returns plans for a project, optionally filtered by status.
func (c *Client) ListPlans(projectSlug, status string) ([]Plan, error) {
	args := []string{"plan", "list", projectSlug}
	if status != "" {
		args = append(args, "--status", status)
	}
	data, err := c.exec(args...)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data []Plan `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing plans response: %w", err)
	}
	return resp.Data, nil
}

// GetPlan returns a single plan by ID.
func (c *Client) GetPlan(id int) (*Plan, error) {
	data, err := c.exec("plan", "show", strconv.Itoa(id))
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data Plan `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing plan response: %w", err)
	}
	return &resp.Data, nil
}

// UpdatePlanStatus transitions a plan to a new status.
func (c *Client) UpdatePlanStatus(id int, status string) (*Plan, error) {
	data, err := c.exec("plan", "status", strconv.Itoa(id), "--status", status)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data Plan `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing plan response: %w", err)
	}
	return &resp.Data, nil
}

// ListTasks returns tasks for a plan, optionally filtered by status or availability.
func (c *Client) ListTasks(planID int, opts *TaskListOpts) ([]Task, error) {
	args := []string{"task", "list", strconv.Itoa(planID)}
	if opts != nil {
		if opts.Status != "" {
			args = append(args, "--status", opts.Status)
		}
		if opts.Available {
			args = append(args, "--available")
		}
	}
	data, err := c.exec(args...)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data []Task `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing tasks response: %w", err)
	}
	return resp.Data, nil
}

// GetTask returns a single task by ID.
func (c *Client) GetTask(id int) (*Task, error) {
	data, err := c.exec("task", "show", strconv.Itoa(id))
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data Task `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing task response: %w", err)
	}
	return &resp.Data, nil
}

// ClaimTask assigns a task to the given assignee.
func (c *Client) ClaimTask(id int, assignee string) (*Task, error) {
	data, err := c.exec("task", "claim", strconv.Itoa(id), "--assignee", assignee)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data Task `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing task response: %w", err)
	}
	return &resp.Data, nil
}

// StartTask transitions a claimed task to doing.
func (c *Client) StartTask(id int) (*Task, error) {
	data, err := c.exec("task", "start", strconv.Itoa(id))
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data Task `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing task response: %w", err)
	}
	return &resp.Data, nil
}

// CompleteTask marks a task as done.
func (c *Client) CompleteTask(id int) (*Task, error) {
	data, err := c.exec("task", "complete", strconv.Itoa(id))
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data Task `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing task response: %w", err)
	}
	return &resp.Data, nil
}

// BlockTask marks a task as blocked with the given reason.
func (c *Client) BlockTask(id int, reason string) (*Task, error) {
	data, err := c.exec("task", "block", strconv.Itoa(id), "--reason", reason)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data Task `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing task response: %w", err)
	}
	return &resp.Data, nil
}

// SkipTask marks a task as skipped with an optional reason.
func (c *Client) SkipTask(id int, reason string) (*Task, error) {
	args := []string{"task", "skip", strconv.Itoa(id)}
	if reason != "" {
		args = append(args, "--reason", reason)
	}
	data, err := c.exec(args...)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data Task `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing task response: %w", err)
	}
	return &resp.Data, nil
}

// AddProgress adds a progress entry to a plan.
func (c *Client) AddProgress(planID int, author, body string) (*Progress, error) {
	data, err := c.exec("progress", "add", strconv.Itoa(planID), "--author", author, "--body", body)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data Progress `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing progress response: %w", err)
	}
	return &resp.Data, nil
}

// AddFeedback adds a feedback entry to a plan.
func (c *Client) AddFeedback(planID int, author, body string) (*Feedback, error) {
	data, err := c.exec("feedback", "add", strconv.Itoa(planID), "--author", author, "--body", body)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data Feedback `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing feedback response: %w", err)
	}
	return &resp.Data, nil
}

// CheckCriterion marks an acceptance criterion as checked.
func (c *Client) CheckCriterion(id int) (*Criterion, error) {
	data, err := c.exec("criteria", "check", strconv.Itoa(id))
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data Criterion `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing criterion response: %w", err)
	}
	return &resp.Data, nil
}

// UncheckCriterion marks an acceptance criterion as unchecked.
func (c *Client) UncheckCriterion(id int) (*Criterion, error) {
	data, err := c.exec("criteria", "uncheck", strconv.Itoa(id))
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data Criterion `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing criterion response: %w", err)
	}
	return &resp.Data, nil
}

// exec runs the atm-cli binary with global flags and the given arguments,
// returning stdout on success or an error wrapping stderr on failure.
// Commands are bounded by ExecTimeout per attempt and retried on transient failures.
func (c *Client) exec(args ...string) ([]byte, error) {
	// Prepend global flags.
	var cmdArgs []string
	if c.apiURL != "" {
		cmdArgs = append(cmdArgs, "--api-url", c.apiURL)
	}
	if c.apiToken != "" {
		cmdArgs = append(cmdArgs, "--api-token", c.apiToken)
	}
	cmdArgs = append(cmdArgs, args...)

	var stdout []byte
	err := c.retrier.Do(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), ExecTimeout)
		defer cancel()

		cmd := exec.CommandContext(ctx, c.binPath, cmdArgs...)
		out, err := cmd.Output()
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return fmt.Errorf("atm-cli %s: timed out after %v", args[0], ExecTimeout)
			}
			if exitErr, ok := err.(*exec.ExitError); ok {
				return fmt.Errorf("atm-cli %s: %s", args[0], string(exitErr.Stderr))
			}
			return fmt.Errorf("running atm-cli: %w", err)
		}
		stdout = out
		return nil
	})
	return stdout, err
}
