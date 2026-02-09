package cli

import (
	"strings"
	"testing"
)

func TestRunCmd_HelpOutput(t *testing.T) {
	if runCmd == nil {
		t.Fatal("runCmd not initialized")
	}

	if !strings.Contains(runCmd.Use, "plan") {
		t.Errorf("Use should mention plan: %s", runCmd.Use)
	}

	if !strings.Contains(runCmd.Short, "iteration loop") {
		t.Errorf("Short should mention iteration loop: %s", runCmd.Short)
	}
}

func TestRunCmd_FlagsRegistered(t *testing.T) {
	// Verify --plan flag exists
	planFlag := runCmd.Flags().Lookup("plan")
	if planFlag == nil {
		t.Fatal("--plan flag not registered")
	}

	// Verify --max flag exists
	maxFlag := runCmd.Flags().Lookup("max")
	if maxFlag == nil {
		t.Fatal("--max flag not registered")
	}
	if maxFlag.DefValue != "100" {
		t.Errorf("--max default should be 100, got %s", maxFlag.DefValue)
	}

	// Verify --completion-mode flag exists
	modeFlag := runCmd.Flags().Lookup("completion-mode")
	if modeFlag == nil {
		t.Fatal("--completion-mode flag not registered")
	}

	// Verify --push flag exists
	pushFlag := runCmd.Flags().Lookup("push")
	if pushFlag == nil {
		t.Fatal("--push flag not registered")
	}
}

