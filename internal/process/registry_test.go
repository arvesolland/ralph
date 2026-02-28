package process

import (
	"fmt"
	"testing"

	"github.com/arvesolland/ralph/internal/board"
)

func TestNew(t *testing.T) {
	mock := board.NewMockBoard()
	r := New(mock, "worker", "/tmp/ralph.log")

	if r.mode != "worker" {
		t.Errorf("mode = %q, want %q", r.mode, "worker")
	}
	if r.logFile != "/tmp/ralph.log" {
		t.Errorf("logFile = %q, want %q", r.logFile, "/tmp/ralph.log")
	}
	if r.processID == "" {
		t.Error("processID should not be empty")
	}
	if r.registered {
		t.Error("should not be registered initially")
	}
}

func TestRegister_Success(t *testing.T) {
	mock := board.NewMockBoard()
	var captured *board.ProcessRegistration
	mock.RegisterProcessFunc = func(reg *board.ProcessRegistration) (*board.ProcessRegistration, error) {
		captured = reg
		return reg, nil
	}

	r := New(mock, "worker", "/tmp/ralph.log")
	r.Register()

	if !r.registered {
		t.Error("should be registered after successful Register()")
	}
	if captured == nil {
		t.Fatal("RegisterProcess was not called")
	}
	if captured.Mode != "worker" {
		t.Errorf("Mode = %q, want %q", captured.Mode, "worker")
	}
	if captured.State != "running" {
		t.Errorf("State = %q, want %q", captured.State, "running")
	}
	if captured.LogFile != "/tmp/ralph.log" {
		t.Errorf("LogFile = %q, want %q", captured.LogFile, "/tmp/ralph.log")
	}
	if captured.PID == 0 {
		t.Error("PID should be set")
	}
}

func TestRegister_Failure_NonFatal(t *testing.T) {
	mock := board.NewMockBoard()
	mock.RegisterProcessFunc = func(reg *board.ProcessRegistration) (*board.ProcessRegistration, error) {
		return nil, fmt.Errorf("connection refused")
	}

	r := New(mock, "run", "")
	r.Register() // should not panic

	if r.registered {
		t.Error("should not be registered after failed Register()")
	}
}

func TestHeartbeat_Sends_WhenRegistered(t *testing.T) {
	mock := board.NewMockBoard()
	mock.RegisterProcessFunc = func(reg *board.ProcessRegistration) (*board.ProcessRegistration, error) {
		return reg, nil
	}

	heartbeatCalls := 0
	mock.HeartbeatProcessFunc = func(processID, state string, planID *int) (*board.ProcessRegistration, error) {
		heartbeatCalls++
		if state != "running" {
			t.Errorf("state = %q, want %q", state, "running")
		}
		if planID == nil || *planID != 42 {
			t.Errorf("planID = %v, want 42", planID)
		}
		return &board.ProcessRegistration{}, nil
	}

	r := New(mock, "worker", "")
	r.Register()

	pid := 42
	r.Heartbeat("running", &pid)

	if heartbeatCalls != 1 {
		t.Errorf("heartbeat calls = %d, want 1", heartbeatCalls)
	}
}

func TestHeartbeat_Skipped_WhenNotRegistered(t *testing.T) {
	mock := board.NewMockBoard()
	heartbeatCalls := 0
	mock.HeartbeatProcessFunc = func(processID, state string, planID *int) (*board.ProcessRegistration, error) {
		heartbeatCalls++
		return &board.ProcessRegistration{}, nil
	}

	r := New(mock, "worker", "")
	// Don't register

	r.Heartbeat("running", nil)

	if heartbeatCalls != 0 {
		t.Errorf("heartbeat calls = %d, want 0 (not registered)", heartbeatCalls)
	}
}

func TestHeartbeat_Skipped_AfterRegisterFailure(t *testing.T) {
	mock := board.NewMockBoard()
	mock.RegisterProcessFunc = func(reg *board.ProcessRegistration) (*board.ProcessRegistration, error) {
		return nil, fmt.Errorf("connection refused")
	}

	heartbeatCalls := 0
	mock.HeartbeatProcessFunc = func(processID, state string, planID *int) (*board.ProcessRegistration, error) {
		heartbeatCalls++
		return &board.ProcessRegistration{}, nil
	}

	r := New(mock, "worker", "")
	r.Register() // fails
	r.Heartbeat("running", nil)

	if heartbeatCalls != 0 {
		t.Errorf("heartbeat calls = %d, want 0 (registration failed)", heartbeatCalls)
	}
}

func TestDeregister_Success(t *testing.T) {
	mock := board.NewMockBoard()
	mock.RegisterProcessFunc = func(reg *board.ProcessRegistration) (*board.ProcessRegistration, error) {
		return reg, nil
	}

	deregisterCalls := 0
	mock.DeregisterProcessFunc = func(processID string) error {
		deregisterCalls++
		return nil
	}

	r := New(mock, "worker", "")
	r.Register()
	r.Deregister()

	if deregisterCalls != 1 {
		t.Errorf("deregister calls = %d, want 1", deregisterCalls)
	}
	if r.registered {
		t.Error("should not be registered after Deregister()")
	}
}

func TestDeregister_Skipped_WhenNotRegistered(t *testing.T) {
	mock := board.NewMockBoard()
	deregisterCalls := 0
	mock.DeregisterProcessFunc = func(processID string) error {
		deregisterCalls++
		return nil
	}

	r := New(mock, "worker", "")
	r.Deregister()

	if deregisterCalls != 0 {
		t.Errorf("deregister calls = %d, want 0 (not registered)", deregisterCalls)
	}
}

func TestDeregister_Idempotent(t *testing.T) {
	mock := board.NewMockBoard()
	mock.RegisterProcessFunc = func(reg *board.ProcessRegistration) (*board.ProcessRegistration, error) {
		return reg, nil
	}

	deregisterCalls := 0
	mock.DeregisterProcessFunc = func(processID string) error {
		deregisterCalls++
		return nil
	}

	r := New(mock, "worker", "")
	r.Register()
	r.Deregister()
	r.Deregister() // second call should be a no-op

	if deregisterCalls != 1 {
		t.Errorf("deregister calls = %d, want 1 (idempotent)", deregisterCalls)
	}
}

func TestGenerateID(t *testing.T) {
	id1 := generateID()
	id2 := generateID()

	if id1 == "" {
		t.Error("generated ID should not be empty")
	}
	if len(id1) != 32 {
		t.Errorf("generated ID length = %d, want 32 hex chars", len(id1))
	}
	if id1 == id2 {
		t.Error("generated IDs should be unique")
	}
}

func TestProcessID(t *testing.T) {
	mock := board.NewMockBoard()
	r := New(mock, "worker", "")

	if r.ProcessID() == "" {
		t.Error("ProcessID() should not be empty")
	}
	if r.ProcessID() != r.processID {
		t.Error("ProcessID() should match internal processID")
	}
}

func TestFullLifecycle_Register_Heartbeat_Deregister(t *testing.T) {
	mock := board.NewMockBoard()

	var callOrder []string

	mock.RegisterProcessFunc = func(reg *board.ProcessRegistration) (*board.ProcessRegistration, error) {
		callOrder = append(callOrder, "register")
		if reg.Mode != "worker" {
			t.Errorf("Register: Mode = %q, want %q", reg.Mode, "worker")
		}
		if reg.LogFile != "/tmp/worker.log" {
			t.Errorf("Register: LogFile = %q, want %q", reg.LogFile, "/tmp/worker.log")
		}
		return reg, nil
	}

	mock.HeartbeatProcessFunc = func(processID, state string, planID *int) (*board.ProcessRegistration, error) {
		callOrder = append(callOrder, "heartbeat")
		if state != "running" {
			t.Errorf("Heartbeat: state = %q, want %q", state, "running")
		}
		if planID == nil || *planID != 10 {
			t.Errorf("Heartbeat: planID = %v, want 10", planID)
		}
		return &board.ProcessRegistration{ProcessID: processID, State: state, PlanID: planID}, nil
	}

	mock.DeregisterProcessFunc = func(processID string) error {
		callOrder = append(callOrder, "deregister")
		return nil
	}

	r := New(mock, "worker", "/tmp/worker.log")

	// Step 1: Register
	r.Register()
	if !r.registered {
		t.Fatal("should be registered after Register()")
	}

	// Step 2: Heartbeat
	planID := 10
	r.Heartbeat("running", &planID)

	// Step 3: Deregister
	r.Deregister()
	if r.registered {
		t.Fatal("should not be registered after Deregister()")
	}

	// Verify call order
	if len(callOrder) != 3 {
		t.Fatalf("expected 3 calls, got %d: %v", len(callOrder), callOrder)
	}
	if callOrder[0] != "register" {
		t.Errorf("callOrder[0] = %q, want %q", callOrder[0], "register")
	}
	if callOrder[1] != "heartbeat" {
		t.Errorf("callOrder[1] = %q, want %q", callOrder[1], "heartbeat")
	}
	if callOrder[2] != "deregister" {
		t.Errorf("callOrder[2] = %q, want %q", callOrder[2], "deregister")
	}

	// Verify heartbeat is no-op after deregister
	r.Heartbeat("running", &planID)
	if len(callOrder) != 3 {
		t.Errorf("heartbeat should be no-op after deregister, got %d calls", len(callOrder))
	}
}

func TestDeregister_ErrorIsNonFatal(t *testing.T) {
	mock := board.NewMockBoard()
	mock.RegisterProcessFunc = func(reg *board.ProcessRegistration) (*board.ProcessRegistration, error) {
		return reg, nil
	}
	mock.DeregisterProcessFunc = func(processID string) error {
		return fmt.Errorf("network error")
	}

	r := New(mock, "run", "")
	r.Register()
	r.Deregister() // should not panic despite error

	// registered should be set to false even when deregister API call fails
	if r.registered {
		t.Error("registered flag should be cleared even on deregister error")
	}
}

func TestHeartbeat_ErrorIsNonFatal(t *testing.T) {
	mock := board.NewMockBoard()
	mock.RegisterProcessFunc = func(reg *board.ProcessRegistration) (*board.ProcessRegistration, error) {
		return reg, nil
	}
	mock.HeartbeatProcessFunc = func(processID, state string, planID *int) (*board.ProcessRegistration, error) {
		return nil, fmt.Errorf("heartbeat failed")
	}

	r := New(mock, "worker", "")
	r.Register()
	r.Heartbeat("running", nil) // should not panic despite error

	// Should still be registered (heartbeat failure doesn't change state)
	if !r.registered {
		t.Error("should still be registered after heartbeat failure")
	}
}
