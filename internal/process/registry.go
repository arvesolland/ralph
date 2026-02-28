// Package process manages Ralph's lifecycle in the Board process registry.
// It handles registration on startup, heartbeats per iteration, and
// deregistration on shutdown.
package process

import (
	"crypto/rand"
	"fmt"
	"os"
	"sync"

	"github.com/arvesolland/ralph/internal/board"
	"github.com/arvesolland/ralph/internal/log"
)

// Registry manages a single Ralph process registration with Board.
// All methods are safe for concurrent use.
type Registry struct {
	board     board.Board
	processID string
	mode      string // "worker" or "run"
	logFile   string

	mu         sync.Mutex
	registered bool
}

// New creates a new Registry for the given mode ("worker" or "run").
// logFile is the path to the current log file (may be empty).
func New(b board.Board, mode, logFile string) *Registry {
	return &Registry{
		board:     b,
		processID: generateID(),
		mode:      mode,
		logFile:   logFile,
	}
}

// Register registers this process with Board. If registration fails, it logs
// a warning but does not return an error (non-fatal).
func (r *Registry) Register() {
	hostname, _ := os.Hostname()

	reg := &board.ProcessRegistration{
		ProcessID: r.processID,
		Hostname:  hostname,
		PID:       os.Getpid(),
		Mode:      r.mode,
		State:     "running",
		LogFile:   r.logFile,
	}

	_, err := r.board.RegisterProcess(reg)
	if err != nil {
		log.Warn("Failed to register process with Board: %v (continuing without registry)", err)
		return
	}

	r.mu.Lock()
	r.registered = true
	r.mu.Unlock()

	log.Info("Registered with Board process registry (id: %s)", r.processID)
}

// Heartbeat sends a heartbeat to Board with the current state and optional plan ID.
// Silently ignores errors (non-fatal).
func (r *Registry) Heartbeat(state string, planID *int) {
	r.mu.Lock()
	registered := r.registered
	r.mu.Unlock()

	if !registered {
		return
	}

	_, err := r.board.HeartbeatProcess(r.processID, state, planID)
	if err != nil {
		log.Debug("Process heartbeat failed: %v", err)
	}
}

// Deregister removes this process from the Board registry.
// Silently ignores errors (best-effort on shutdown).
func (r *Registry) Deregister() {
	r.mu.Lock()
	registered := r.registered
	r.registered = false
	r.mu.Unlock()

	if !registered {
		return
	}

	if err := r.board.DeregisterProcess(r.processID); err != nil {
		log.Debug("Process deregister failed: %v", err)
	} else {
		log.Info("Deregistered from Board process registry")
	}
}

// ProcessID returns the unique ID of this process registration.
func (r *Registry) ProcessID() string {
	return r.processID
}

// generateID generates a random hex ID (16 bytes = 32 hex chars).
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
