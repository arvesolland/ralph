package notify

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ThreadsFilename is the name of the file that stores thread information.
const ThreadsFilename = "slack_threads.json"

// ThreadInfo contains Slack thread information for a plan.
type ThreadInfo struct {
	// PlanName is the name of the plan this thread is associated with.
	PlanName string `json:"plan_name"`

	// ThreadTS is the Slack thread timestamp (message ID).
	ThreadTS string `json:"thread_ts"`

	// ChannelID is the Slack channel ID where the thread was created.
	ChannelID string `json:"channel_id"`

	// NotifiedBlockers contains hashes of blockers that have been notified.
	// Used to prevent duplicate notifications for the same blocker.
	NotifiedBlockers []string `json:"notified_blockers,omitempty"`

	// CreatedAt is when this thread was first created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when this thread info was last updated.
	UpdatedAt time.Time `json:"updated_at"`
}

// ThreadTracker manages Slack thread information for plans.
// It provides thread-safe access and persists data to a JSON file.
type ThreadTracker struct {
	// filePath is the path to the JSON file storing thread data.
	filePath string

	// threads maps plan names to thread info.
	threads map[string]*ThreadInfo

	// mu protects concurrent access to threads.
	mu sync.RWMutex

	// fileLock is used for file-level locking.
	fileLock sync.Mutex
}

// NewThreadTracker creates a new ThreadTracker that persists to the given file path.
// If the file exists, it loads existing data.
func NewThreadTracker(filePath string) (*ThreadTracker, error) {
	t := &ThreadTracker{
		filePath: filePath,
		threads:  make(map[string]*ThreadInfo),
	}

	// Load existing data if file exists
	if err := t.load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("failed to load thread data: %w", err)
	}

	return t, nil
}

// ThreadTrackerPath returns the path to the threads file in the given config directory.
func ThreadTrackerPath(configDir string) string {
	return filepath.Join(configDir, ThreadsFilename)
}

// Get returns the thread info for a plan, or nil if not found.
func (t *ThreadTracker) Get(planName string) *ThreadInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if info, ok := t.threads[planName]; ok {
		// Return a copy to prevent external modification
		copy := *info
		copy.NotifiedBlockers = make([]string, len(info.NotifiedBlockers))
		for i, h := range info.NotifiedBlockers {
			copy.NotifiedBlockers[i] = h
		}
		return &copy
	}
	return nil
}

// Set saves thread info for a plan and persists to file.
func (t *ThreadTracker) Set(planName string, info *ThreadInfo) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Set timestamps
	now := time.Now()
	if info.CreatedAt.IsZero() {
		info.CreatedAt = now
	}
	info.UpdatedAt = now
	info.PlanName = planName

	// Make a copy to store
	copy := *info
	copy.NotifiedBlockers = make([]string, len(info.NotifiedBlockers))
	for i, h := range info.NotifiedBlockers {
		copy.NotifiedBlockers[i] = h
	}
	t.threads[planName] = &copy

	return t.saveUnlocked()
}

// Delete removes thread info for a plan and persists to file.
func (t *ThreadTracker) Delete(planName string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.threads, planName)
	return t.saveUnlocked()
}

// AddNotifiedBlocker adds a blocker hash to the list of notified blockers for a plan.
// Returns true if the hash was added (wasn't already present).
func (t *ThreadTracker) AddNotifiedBlocker(planName, blockerHash string) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	info, ok := t.threads[planName]
	if !ok {
		return false, fmt.Errorf("no thread info for plan: %s", planName)
	}

	// Check if already notified
	for _, h := range info.NotifiedBlockers {
		if h == blockerHash {
			return false, nil
		}
	}

	// Add the hash
	info.NotifiedBlockers = append(info.NotifiedBlockers, blockerHash)
	info.UpdatedAt = time.Now()

	return true, t.saveUnlocked()
}

// HasNotifiedBlocker checks if a blocker has already been notified for a plan.
func (t *ThreadTracker) HasNotifiedBlocker(planName, blockerHash string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	info, ok := t.threads[planName]
	if !ok {
		return false
	}

	for _, h := range info.NotifiedBlockers {
		if h == blockerHash {
			return true
		}
	}
	return false
}

// List returns all tracked thread infos.
func (t *ThreadTracker) List() []*ThreadInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*ThreadInfo, 0, len(t.threads))
	for _, info := range t.threads {
		copy := *info
		copy.NotifiedBlockers = make([]string, len(info.NotifiedBlockers))
		for i, h := range info.NotifiedBlockers {
			copy.NotifiedBlockers[i] = h
		}
		result = append(result, &copy)
	}
	return result
}

// load reads thread data from the file.
func (t *ThreadTracker) load() error {
	t.fileLock.Lock()
	defer t.fileLock.Unlock()

	data, err := os.ReadFile(t.filePath)
	if err != nil {
		return err
	}

	// Handle empty file
	if len(data) == 0 {
		return nil
	}

	var threads map[string]*ThreadInfo
	if err := json.Unmarshal(data, &threads); err != nil {
		return fmt.Errorf("failed to parse thread data: %w", err)
	}

	t.threads = threads
	if t.threads == nil {
		t.threads = make(map[string]*ThreadInfo)
	}

	return nil
}

// saveUnlocked saves thread data to file.
// Caller must hold the write lock.
func (t *ThreadTracker) saveUnlocked() error {
	t.fileLock.Lock()
	defer t.fileLock.Unlock()

	// Ensure parent directory exists
	dir := filepath.Dir(t.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.MarshalIndent(t.threads, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal thread data: %w", err)
	}

	// Atomic write: write to temp file, then rename
	tmpPath := t.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tmpPath, t.filePath); err != nil {
		// Clean up temp file on error
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// Reload reloads thread data from file.
// Useful when another process may have modified the file.
func (t *ThreadTracker) Reload() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.load()
}
