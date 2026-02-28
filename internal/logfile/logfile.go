// Package logfile provides log file streaming for Ralph.
// It tees stdout and stderr to a log file while preserving normal terminal output.
package logfile

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// LogFile manages tee-ing output to a log file alongside stdout/stderr.
type LogFile struct {
	file     *os.File
	path     string
	origOut  *os.File
	origErr  *os.File
	outPipeR *os.File
	outPipeW *os.File
	errPipeR *os.File
	errPipeW *os.File
	outDone  chan struct{}
	errDone  chan struct{}
}

// Options configures log file creation.
type Options struct {
	// LogDir is the base directory for log files (e.g., .ralph/logs).
	LogDir string

	// Prefix is the log file name prefix (e.g., "worker", "plan-56").
	Prefix string

	// CustomPath overrides the default path (--log-file flag).
	CustomPath string

	// MaxFiles is the maximum number of log files to keep per prefix.
	// Files beyond this limit are removed (oldest first).
	// Default: 10.
	MaxFiles int
}

// DefaultMaxFiles is the default number of log files to retain per prefix.
const DefaultMaxFiles = 10

// New creates a new log file and returns a LogFile that tees stdout/stderr.
// Call Close() when done to flush and close the log file.
func New(opts Options) (*LogFile, error) {
	if opts.MaxFiles == 0 {
		opts.MaxFiles = DefaultMaxFiles
	}

	// Determine log file path
	logPath := opts.CustomPath
	if logPath == "" {
		if opts.LogDir == "" {
			return nil, fmt.Errorf("LogDir is required when CustomPath is not set")
		}
		if opts.Prefix == "" {
			return nil, fmt.Errorf("Prefix is required when CustomPath is not set")
		}
		// Ensure logs directory exists
		if err := os.MkdirAll(opts.LogDir, 0755); err != nil {
			return nil, fmt.Errorf("creating log directory: %w", err)
		}
		timestamp := time.Now().Format("20060102-150405")
		logPath = filepath.Join(opts.LogDir, fmt.Sprintf("%s-%s.log", opts.Prefix, timestamp))
	} else {
		// Ensure parent directory exists for custom path
		dir := filepath.Dir(logPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("creating log directory for custom path: %w", err)
		}
	}

	// Open log file
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening log file: %w", err)
	}

	lf := &LogFile{
		file:    f,
		path:    logPath,
		origOut: os.Stdout,
		origErr: os.Stderr,
		outDone: make(chan struct{}),
		errDone: make(chan struct{}),
	}

	// Create pipe for stdout: anything written to os.Stdout goes to pipe writer,
	// which we read and tee to both original stdout and the file.
	outR, outW, err := os.Pipe()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}
	lf.outPipeR = outR
	lf.outPipeW = outW

	// Create pipe for stderr
	errR, errW, err := os.Pipe()
	if err != nil {
		f.Close()
		outR.Close()
		outW.Close()
		return nil, fmt.Errorf("creating stderr pipe: %w", err)
	}
	lf.errPipeR = errR
	lf.errPipeW = errW

	// Replace os.Stdout and os.Stderr
	os.Stdout = outW
	os.Stderr = errW

	// Start goroutines to copy from pipes to both original outputs and file
	go func() {
		_, _ = io.Copy(io.MultiWriter(lf.origOut, f), outR)
		close(lf.outDone)
	}()
	go func() {
		_, _ = io.Copy(io.MultiWriter(lf.origErr, f), errR)
		close(lf.errDone)
	}()

	// Run cleanup for old log files (non-blocking)
	if opts.CustomPath == "" && opts.Prefix != "" {
		go Cleanup(opts.LogDir, opts.Prefix, opts.MaxFiles)
	}

	return lf, nil
}

// Path returns the path to the log file.
func (lf *LogFile) Path() string {
	return lf.path
}

// Close restores os.Stdout and os.Stderr and closes the log file.
func (lf *LogFile) Close() error {
	// Restore original stdout/stderr
	os.Stdout = lf.origOut
	os.Stderr = lf.origErr

	// Close pipe writers to signal EOF to tee goroutines
	lf.outPipeW.Close()
	lf.errPipeW.Close()

	// Wait for both goroutines to finish draining
	<-lf.outDone
	<-lf.errDone

	// Close pipe readers
	lf.outPipeR.Close()
	lf.errPipeR.Close()

	return lf.file.Close()
}

// Cleanup removes old log files, keeping only the most recent maxFiles.
// It matches files with the given prefix in the log directory.
func Cleanup(logDir, prefix string, maxFiles int) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}

	// Collect matching log files
	matchPrefix := prefix + "-"
	var logFiles []os.DirEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), matchPrefix) && strings.HasSuffix(e.Name(), ".log") {
			logFiles = append(logFiles, e)
		}
	}

	if len(logFiles) <= maxFiles {
		return
	}

	// Sort by name (which includes timestamp, so alphabetical = chronological)
	sort.Slice(logFiles, func(i, j int) bool {
		return logFiles[i].Name() < logFiles[j].Name()
	})

	// Remove oldest files
	toRemove := len(logFiles) - maxFiles
	for i := 0; i < toRemove; i++ {
		path := filepath.Join(logDir, logFiles[i].Name())
		_ = os.Remove(path)
	}
}

// PlanPrefix returns the log file prefix for a plan run.
func PlanPrefix(planID int) string {
	return fmt.Sprintf("plan-%d", planID)
}

// WorkerPrefix returns the log file prefix for a worker run.
func WorkerPrefix() string {
	return "worker"
}
