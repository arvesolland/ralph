// Package log provides structured logging with level filtering and color support.
package log

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Level represents the severity of a log message.
type Level int

const (
	// LevelDebug is for detailed debugging information.
	LevelDebug Level = iota
	// LevelInfo is for general informational messages.
	LevelInfo
	// LevelSuccess is for success messages (custom level, treated as Info priority).
	LevelSuccess
	// LevelWarn is for warning messages.
	LevelWarn
	// LevelError is for error messages.
	LevelError
)

// String returns the string representation of the log level.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelSuccess:
		return "SUCCESS"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger defines the interface for logging operations.
type Logger interface {
	// Debug logs a debug message.
	Debug(format string, args ...interface{})
	// Info logs an informational message.
	Info(format string, args ...interface{})
	// Success logs a success message.
	Success(format string, args ...interface{})
	// Warn logs a warning message.
	Warn(format string, args ...interface{})
	// Error logs an error message.
	Error(format string, args ...interface{})

	// SetLevel sets the minimum log level to output.
	SetLevel(level Level)
	// SetOutput sets the output writer.
	SetOutput(w io.Writer)
	// SetColorEnabled enables or disables color output.
	SetColorEnabled(enabled bool)
}

// ConsoleLogger implements Logger with console output.
type ConsoleLogger struct {
	mu           sync.Mutex
	level        Level
	output       io.Writer
	colorEnabled bool
}

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorGray   = "\033[90m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
)

// levelColors maps log levels to their color codes.
var levelColors = map[Level]string{
	LevelDebug:   colorGray,
	LevelInfo:    "", // default color
	LevelSuccess: colorGreen,
	LevelWarn:    colorYellow,
	LevelError:   colorRed,
}

// NewConsoleLogger creates a new ConsoleLogger with default settings.
// Colors are enabled if stderr is a TTY.
func NewConsoleLogger() *ConsoleLogger {
	return &ConsoleLogger{
		level:        LevelInfo,
		output:       os.Stderr,
		colorEnabled: isTerminal(os.Stderr),
	}
}

// isTerminal checks if the given file is a terminal.
func isTerminal(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// SetLevel sets the minimum log level to output.
func (l *ConsoleLogger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// SetOutput sets the output writer.
func (l *ConsoleLogger) SetOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.output = w
}

// SetColorEnabled enables or disables color output.
func (l *ConsoleLogger) SetColorEnabled(enabled bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.colorEnabled = enabled
}

// log writes a log message if the level is at or above the current threshold.
func (l *ConsoleLogger) log(level Level, format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Level filtering: Success is treated same as Info for filtering purposes
	minLevel := l.level
	effectiveLevel := level
	if effectiveLevel == LevelSuccess {
		effectiveLevel = LevelInfo
	}
	if minLevel == LevelSuccess {
		minLevel = LevelInfo
	}
	if effectiveLevel < minLevel {
		return
	}

	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, args...)

	var output string
	if l.colorEnabled {
		color := levelColors[level]
		if color != "" {
			output = fmt.Sprintf("%s[%s] [%s] %s%s\n", color, timestamp, level.String(), message, colorReset)
		} else {
			output = fmt.Sprintf("[%s] [%s] %s\n", timestamp, level.String(), message)
		}
	} else {
		output = fmt.Sprintf("[%s] [%s] %s\n", timestamp, level.String(), message)
	}

	fmt.Fprint(l.output, output)
}

// Debug logs a debug message.
func (l *ConsoleLogger) Debug(format string, args ...interface{}) {
	l.log(LevelDebug, format, args...)
}

// Info logs an informational message.
func (l *ConsoleLogger) Info(format string, args ...interface{}) {
	l.log(LevelInfo, format, args...)
}

// Success logs a success message.
func (l *ConsoleLogger) Success(format string, args ...interface{}) {
	l.log(LevelSuccess, format, args...)
}

// Warn logs a warning message.
func (l *ConsoleLogger) Warn(format string, args ...interface{}) {
	l.log(LevelWarn, format, args...)
}

// Error logs an error message.
func (l *ConsoleLogger) Error(format string, args ...interface{}) {
	l.log(LevelError, format, args...)
}

// Default logger instance
var defaultLogger Logger = NewConsoleLogger()

// SetDefault sets the default logger instance.
func SetDefault(logger Logger) {
	defaultLogger = logger
}

// Default returns the default logger instance.
func Default() Logger {
	return defaultLogger
}

// Package-level functions that use the default logger

// Debug logs a debug message using the default logger.
func Debug(format string, args ...interface{}) {
	defaultLogger.Debug(format, args...)
}

// Info logs an informational message using the default logger.
func Info(format string, args ...interface{}) {
	defaultLogger.Info(format, args...)
}

// Success logs a success message using the default logger.
func Success(format string, args ...interface{}) {
	defaultLogger.Success(format, args...)
}

// Warn logs a warning message using the default logger.
func Warn(format string, args ...interface{}) {
	defaultLogger.Warn(format, args...)
}

// Error logs an error message using the default logger.
func Error(format string, args ...interface{}) {
	defaultLogger.Error(format, args...)
}
