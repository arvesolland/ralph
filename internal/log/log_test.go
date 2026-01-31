package log

import (
	"bytes"
	"strings"
	"testing"
)

func TestLevelString(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelSuccess, "SUCCESS"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{Level(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.level.String(); got != tt.expected {
				t.Errorf("Level.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestConsoleLogger_LevelFiltering(t *testing.T) {
	tests := []struct {
		name      string
		setLevel  Level
		logLevel  Level
		shouldLog bool
	}{
		{"Debug at Debug level", LevelDebug, LevelDebug, true},
		{"Info at Debug level", LevelDebug, LevelInfo, true},
		{"Warn at Debug level", LevelDebug, LevelWarn, true},
		{"Error at Debug level", LevelDebug, LevelError, true},

		{"Debug at Info level", LevelInfo, LevelDebug, false},
		{"Info at Info level", LevelInfo, LevelInfo, true},
		{"Success at Info level", LevelInfo, LevelSuccess, true},
		{"Warn at Info level", LevelInfo, LevelWarn, true},
		{"Error at Info level", LevelInfo, LevelError, true},

		{"Debug at Warn level", LevelWarn, LevelDebug, false},
		{"Info at Warn level", LevelWarn, LevelInfo, false},
		{"Success at Warn level", LevelWarn, LevelSuccess, false},
		{"Warn at Warn level", LevelWarn, LevelWarn, true},
		{"Error at Warn level", LevelWarn, LevelError, true},

		{"Debug at Error level", LevelError, LevelDebug, false},
		{"Info at Error level", LevelError, LevelInfo, false},
		{"Warn at Error level", LevelError, LevelWarn, false},
		{"Error at Error level", LevelError, LevelError, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := NewConsoleLogger()
			logger.SetOutput(&buf)
			logger.SetLevel(tt.setLevel)
			logger.SetColorEnabled(false)

			// Log at the specified level
			switch tt.logLevel {
			case LevelDebug:
				logger.Debug("test message")
			case LevelInfo:
				logger.Info("test message")
			case LevelSuccess:
				logger.Success("test message")
			case LevelWarn:
				logger.Warn("test message")
			case LevelError:
				logger.Error("test message")
			}

			output := buf.String()
			if tt.shouldLog && output == "" {
				t.Errorf("expected log output, got none")
			}
			if !tt.shouldLog && output != "" {
				t.Errorf("expected no log output, got %q", output)
			}
		})
	}
}

func TestConsoleLogger_MessageFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := NewConsoleLogger()
	logger.SetOutput(&buf)
	logger.SetColorEnabled(false)

	logger.Info("hello %s", "world")

	output := buf.String()
	// Format: [HH:MM:SS] [LEVEL] message
	if !strings.Contains(output, "[INFO]") {
		t.Errorf("output should contain [INFO], got %q", output)
	}
	if !strings.Contains(output, "hello world") {
		t.Errorf("output should contain formatted message, got %q", output)
	}
	if !strings.HasSuffix(output, "\n") {
		t.Errorf("output should end with newline, got %q", output)
	}
}

func TestConsoleLogger_ColorOutput(t *testing.T) {
	tests := []struct {
		name          string
		level         Level
		colorEnabled  bool
		shouldHaveANSI bool
	}{
		{"Debug with color", LevelDebug, true, true},
		{"Info with color", LevelInfo, true, false}, // Info has no color
		{"Success with color", LevelSuccess, true, true},
		{"Warn with color", LevelWarn, true, true},
		{"Error with color", LevelError, true, true},
		{"Debug without color", LevelDebug, false, false},
		{"Warn without color", LevelWarn, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := NewConsoleLogger()
			logger.SetOutput(&buf)
			logger.SetLevel(LevelDebug)
			logger.SetColorEnabled(tt.colorEnabled)

			switch tt.level {
			case LevelDebug:
				logger.Debug("test")
			case LevelInfo:
				logger.Info("test")
			case LevelSuccess:
				logger.Success("test")
			case LevelWarn:
				logger.Warn("test")
			case LevelError:
				logger.Error("test")
			}

			output := buf.String()
			hasANSI := strings.Contains(output, "\033[")

			if tt.shouldHaveANSI && !hasANSI {
				t.Errorf("expected ANSI codes in output, got %q", output)
			}
			if !tt.shouldHaveANSI && hasANSI {
				t.Errorf("expected no ANSI codes in output, got %q", output)
			}
		})
	}
}

func TestConsoleLogger_ColorCodes(t *testing.T) {
	tests := []struct {
		name      string
		level     Level
		colorCode string
	}{
		{"Debug is gray", LevelDebug, colorGray},
		{"Success is green", LevelSuccess, colorGreen},
		{"Warn is yellow", LevelWarn, colorYellow},
		{"Error is red", LevelError, colorRed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := NewConsoleLogger()
			logger.SetOutput(&buf)
			logger.SetLevel(LevelDebug)
			logger.SetColorEnabled(true)

			switch tt.level {
			case LevelDebug:
				logger.Debug("test")
			case LevelSuccess:
				logger.Success("test")
			case LevelWarn:
				logger.Warn("test")
			case LevelError:
				logger.Error("test")
			}

			output := buf.String()
			if !strings.Contains(output, tt.colorCode) {
				t.Errorf("expected color code %q in output, got %q", tt.colorCode, output)
			}
			if !strings.Contains(output, colorReset) {
				t.Errorf("expected color reset in output, got %q", output)
			}
		})
	}
}

func TestPackageLevelFunctions(t *testing.T) {
	var buf bytes.Buffer
	logger := NewConsoleLogger()
	logger.SetOutput(&buf)
	logger.SetLevel(LevelDebug)
	logger.SetColorEnabled(false)
	SetDefault(logger)

	Debug("debug msg")
	Info("info msg")
	Success("success msg")
	Warn("warn msg")
	Error("error msg")

	output := buf.String()
	if !strings.Contains(output, "debug msg") {
		t.Error("expected Debug output")
	}
	if !strings.Contains(output, "info msg") {
		t.Error("expected Info output")
	}
	if !strings.Contains(output, "success msg") {
		t.Error("expected Success output")
	}
	if !strings.Contains(output, "warn msg") {
		t.Error("expected Warn output")
	}
	if !strings.Contains(output, "error msg") {
		t.Error("expected Error output")
	}
}

func TestDefault(t *testing.T) {
	logger := Default()
	if logger == nil {
		t.Error("Default() should not return nil")
	}
}
