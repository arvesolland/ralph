package board

import (
	"fmt"
	"testing"
)

func TestMockBoard_ImplementsBoard(t *testing.T) {
	// Compile-time check is in mock.go (var _ Board = (*MockBoard)(nil)),
	// but this test explicitly verifies all process registry methods exist
	// and are callable on the mock.
	mock := NewMockBoard()
	var b Board = mock
	_ = b // ensure it compiles as Board interface
}

func TestMockBoard_RegisterProcess(t *testing.T) {
	mock := NewMockBoard()

	reg := &ProcessRegistration{
		ProcessID: "test-id",
		Hostname:  "test-host",
		PID:       1234,
		Mode:      "worker",
		State:     "running",
		LogFile:   "/tmp/test.log",
	}

	result, err := mock.RegisterProcess(reg)
	if err != nil {
		t.Fatalf("RegisterProcess() error: %v", err)
	}
	if result != reg {
		t.Error("default RegisterProcess should return the input registration")
	}

	// Verify call was recorded
	if len(mock.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.Calls))
	}
	if mock.Calls[0].Method != "RegisterProcess" {
		t.Errorf("Method = %q, want %q", mock.Calls[0].Method, "RegisterProcess")
	}
}

func TestMockBoard_RegisterProcess_WithFunc(t *testing.T) {
	mock := NewMockBoard()
	mock.RegisterProcessFunc = func(reg *ProcessRegistration) (*ProcessRegistration, error) {
		return nil, fmt.Errorf("registration failed")
	}

	_, err := mock.RegisterProcess(&ProcessRegistration{})
	if err == nil {
		t.Error("expected error from custom RegisterProcessFunc")
	}
}

func TestMockBoard_HeartbeatProcess(t *testing.T) {
	mock := NewMockBoard()

	planID := 42
	result, err := mock.HeartbeatProcess("proc-1", "running", &planID)
	if err != nil {
		t.Fatalf("HeartbeatProcess() error: %v", err)
	}
	if result.ProcessID != "proc-1" {
		t.Errorf("ProcessID = %q, want %q", result.ProcessID, "proc-1")
	}
	if result.State != "running" {
		t.Errorf("State = %q, want %q", result.State, "running")
	}
	if result.PlanID == nil || *result.PlanID != 42 {
		t.Errorf("PlanID = %v, want 42", result.PlanID)
	}

	// Verify call was recorded
	if len(mock.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.Calls))
	}
	if mock.Calls[0].Method != "HeartbeatProcess" {
		t.Errorf("Method = %q, want %q", mock.Calls[0].Method, "HeartbeatProcess")
	}
}

func TestMockBoard_HeartbeatProcess_WithFunc(t *testing.T) {
	mock := NewMockBoard()
	mock.HeartbeatProcessFunc = func(processID, state string, planID *int) (*ProcessRegistration, error) {
		return nil, fmt.Errorf("heartbeat failed")
	}

	_, err := mock.HeartbeatProcess("proc-1", "running", nil)
	if err == nil {
		t.Error("expected error from custom HeartbeatProcessFunc")
	}
}

func TestMockBoard_DeregisterProcess(t *testing.T) {
	mock := NewMockBoard()

	err := mock.DeregisterProcess("proc-1")
	if err != nil {
		t.Fatalf("DeregisterProcess() error: %v", err)
	}

	// Verify call was recorded
	if len(mock.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.Calls))
	}
	if mock.Calls[0].Method != "DeregisterProcess" {
		t.Errorf("Method = %q, want %q", mock.Calls[0].Method, "DeregisterProcess")
	}
}

func TestMockBoard_DeregisterProcess_WithFunc(t *testing.T) {
	mock := NewMockBoard()
	mock.DeregisterProcessFunc = func(processID string) error {
		return fmt.Errorf("deregister failed")
	}

	err := mock.DeregisterProcess("proc-1")
	if err == nil {
		t.Error("expected error from custom DeregisterProcessFunc")
	}
}

func TestMockBoard_ProcessRegistryLifecycle(t *testing.T) {
	// Tests the full lifecycle through the mock: register -> heartbeat -> deregister
	mock := NewMockBoard()

	reg := &ProcessRegistration{
		ProcessID: "lifecycle-test",
		Hostname:  "test-host",
		PID:       9999,
		Mode:      "run",
		State:     "running",
		LogFile:   "/tmp/lifecycle.log",
	}

	// Register
	_, err := mock.RegisterProcess(reg)
	if err != nil {
		t.Fatalf("RegisterProcess: %v", err)
	}

	// Heartbeat
	planID := 5
	_, err = mock.HeartbeatProcess("lifecycle-test", "running", &planID)
	if err != nil {
		t.Fatalf("HeartbeatProcess: %v", err)
	}

	// Deregister
	err = mock.DeregisterProcess("lifecycle-test")
	if err != nil {
		t.Fatalf("DeregisterProcess: %v", err)
	}

	// Verify all 3 calls in order
	if len(mock.Calls) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(mock.Calls))
	}
	expectedMethods := []string{"RegisterProcess", "HeartbeatProcess", "DeregisterProcess"}
	for i, expected := range expectedMethods {
		if mock.Calls[i].Method != expected {
			t.Errorf("Calls[%d].Method = %q, want %q", i, mock.Calls[i].Method, expected)
		}
	}
}
