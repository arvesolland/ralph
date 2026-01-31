package notify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNewThreadTracker(t *testing.T) {
	t.Run("creates new tracker with empty file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "threads.json")

		tracker, err := NewThreadTracker(filePath)
		if err != nil {
			t.Fatalf("NewThreadTracker() error = %v", err)
		}

		if tracker == nil {
			t.Fatal("NewThreadTracker() returned nil")
		}

		// Verify no threads exist
		if len(tracker.List()) != 0 {
			t.Errorf("expected 0 threads, got %d", len(tracker.List()))
		}
	})

	t.Run("loads existing data", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "threads.json")

		// Create existing data
		existingData := map[string]*ThreadInfo{
			"test-plan": {
				PlanName:  "test-plan",
				ThreadTS:  "1234567890.123456",
				ChannelID: "C123456",
			},
		}
		data, _ := json.Marshal(existingData)
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			t.Fatalf("failed to write test data: %v", err)
		}

		tracker, err := NewThreadTracker(filePath)
		if err != nil {
			t.Fatalf("NewThreadTracker() error = %v", err)
		}

		info := tracker.Get("test-plan")
		if info == nil {
			t.Fatal("expected thread info for test-plan")
		}
		if info.ThreadTS != "1234567890.123456" {
			t.Errorf("expected ThreadTS 1234567890.123456, got %s", info.ThreadTS)
		}
	})

	t.Run("handles invalid JSON gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "threads.json")

		if err := os.WriteFile(filePath, []byte("invalid json"), 0644); err != nil {
			t.Fatalf("failed to write test data: %v", err)
		}

		_, err := NewThreadTracker(filePath)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("handles empty file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "threads.json")

		if err := os.WriteFile(filePath, []byte{}, 0644); err != nil {
			t.Fatalf("failed to write test data: %v", err)
		}

		tracker, err := NewThreadTracker(filePath)
		if err != nil {
			t.Fatalf("NewThreadTracker() error = %v", err)
		}

		if len(tracker.List()) != 0 {
			t.Errorf("expected 0 threads for empty file, got %d", len(tracker.List()))
		}
	})
}

func TestThreadTrackerPath(t *testing.T) {
	tests := []struct {
		name      string
		configDir string
		want      string
	}{
		{
			name:      "standard path",
			configDir: "/home/user/.ralph",
			want:      "/home/user/.ralph/slack_threads.json",
		},
		{
			name:      "relative path",
			configDir: ".ralph",
			want:      ".ralph/slack_threads.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ThreadTrackerPath(tt.configDir)
			if got != tt.want {
				t.Errorf("ThreadTrackerPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestThreadTracker_Get(t *testing.T) {
	t.Run("returns nil for non-existent plan", func(t *testing.T) {
		tmpDir := t.TempDir()
		tracker, _ := NewThreadTracker(filepath.Join(tmpDir, "threads.json"))

		info := tracker.Get("non-existent")
		if info != nil {
			t.Error("expected nil for non-existent plan")
		}
	})

	t.Run("returns copy of data", func(t *testing.T) {
		tmpDir := t.TempDir()
		tracker, _ := NewThreadTracker(filepath.Join(tmpDir, "threads.json"))

		err := tracker.Set("test-plan", &ThreadInfo{
			ThreadTS:         "1234567890.123456",
			ChannelID:        "C123456",
			NotifiedBlockers: []string{"abc123"},
		})
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		info := tracker.Get("test-plan")

		// Modify the returned copy
		info.ThreadTS = "modified"
		info.NotifiedBlockers[0] = "modified"

		// Original should be unchanged
		original := tracker.Get("test-plan")
		if original.ThreadTS == "modified" {
			t.Error("Get() should return a copy, not the original")
		}
		if original.NotifiedBlockers[0] == "modified" {
			t.Error("Get() should return a copy of NotifiedBlockers")
		}
	})
}

func TestThreadTracker_Set(t *testing.T) {
	t.Run("saves new thread info", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "threads.json")
		tracker, _ := NewThreadTracker(filePath)

		err := tracker.Set("test-plan", &ThreadInfo{
			ThreadTS:  "1234567890.123456",
			ChannelID: "C123456",
		})
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		// Verify in memory
		info := tracker.Get("test-plan")
		if info == nil {
			t.Fatal("expected thread info after Set")
		}
		if info.ThreadTS != "1234567890.123456" {
			t.Errorf("expected ThreadTS 1234567890.123456, got %s", info.ThreadTS)
		}

		// Verify persisted to file
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}

		var loaded map[string]*ThreadInfo
		if err := json.Unmarshal(data, &loaded); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if loaded["test-plan"] == nil {
			t.Error("expected test-plan in persisted data")
		}
	})

	t.Run("sets timestamps", func(t *testing.T) {
		tmpDir := t.TempDir()
		tracker, _ := NewThreadTracker(filepath.Join(tmpDir, "threads.json"))

		before := time.Now()
		err := tracker.Set("test-plan", &ThreadInfo{
			ThreadTS:  "1234567890.123456",
			ChannelID: "C123456",
		})
		after := time.Now()

		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		info := tracker.Get("test-plan")
		if info.CreatedAt.Before(before) || info.CreatedAt.After(after) {
			t.Error("CreatedAt not set correctly")
		}
		if info.UpdatedAt.Before(before) || info.UpdatedAt.After(after) {
			t.Error("UpdatedAt not set correctly")
		}
	})

	t.Run("preserves CreatedAt on update", func(t *testing.T) {
		tmpDir := t.TempDir()
		tracker, _ := NewThreadTracker(filepath.Join(tmpDir, "threads.json"))

		// First set
		tracker.Set("test-plan", &ThreadInfo{
			ThreadTS:  "1234567890.123456",
			ChannelID: "C123456",
		})

		firstInfo := tracker.Get("test-plan")
		firstCreated := firstInfo.CreatedAt

		time.Sleep(10 * time.Millisecond)

		// Update with existing CreatedAt
		err := tracker.Set("test-plan", &ThreadInfo{
			ThreadTS:  "9999999999.999999",
			ChannelID: "C123456",
			CreatedAt: firstCreated,
		})
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		updated := tracker.Get("test-plan")
		if !updated.CreatedAt.Equal(firstCreated) {
			t.Error("CreatedAt should be preserved on update")
		}
		if !updated.UpdatedAt.After(firstCreated) {
			t.Error("UpdatedAt should be after CreatedAt on update")
		}
	})

	t.Run("creates parent directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "nested", "dir", "threads.json")
		tracker, _ := NewThreadTracker(filePath)

		err := tracker.Set("test-plan", &ThreadInfo{
			ThreadTS:  "1234567890.123456",
			ChannelID: "C123456",
		})
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Error("expected file to be created")
		}
	})
}

func TestThreadTracker_Delete(t *testing.T) {
	t.Run("removes thread info", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "threads.json")
		tracker, _ := NewThreadTracker(filePath)

		tracker.Set("test-plan", &ThreadInfo{
			ThreadTS:  "1234567890.123456",
			ChannelID: "C123456",
		})

		err := tracker.Delete("test-plan")
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		if tracker.Get("test-plan") != nil {
			t.Error("expected nil after Delete")
		}

		// Verify persisted
		data, _ := os.ReadFile(filePath)
		var loaded map[string]*ThreadInfo
		json.Unmarshal(data, &loaded)
		if loaded["test-plan"] != nil {
			t.Error("expected test-plan to be deleted from file")
		}
	})

	t.Run("no error for non-existent plan", func(t *testing.T) {
		tmpDir := t.TempDir()
		tracker, _ := NewThreadTracker(filepath.Join(tmpDir, "threads.json"))

		err := tracker.Delete("non-existent")
		if err != nil {
			t.Errorf("Delete() error = %v, expected nil", err)
		}
	})
}

func TestThreadTracker_AddNotifiedBlocker(t *testing.T) {
	t.Run("adds new blocker hash", func(t *testing.T) {
		tmpDir := t.TempDir()
		tracker, _ := NewThreadTracker(filepath.Join(tmpDir, "threads.json"))

		tracker.Set("test-plan", &ThreadInfo{
			ThreadTS:  "1234567890.123456",
			ChannelID: "C123456",
		})

		added, err := tracker.AddNotifiedBlocker("test-plan", "abc12345")
		if err != nil {
			t.Fatalf("AddNotifiedBlocker() error = %v", err)
		}
		if !added {
			t.Error("expected added to be true for new hash")
		}

		info := tracker.Get("test-plan")
		if len(info.NotifiedBlockers) != 1 || info.NotifiedBlockers[0] != "abc12345" {
			t.Errorf("expected NotifiedBlockers to contain abc12345, got %v", info.NotifiedBlockers)
		}
	})

	t.Run("returns false for existing hash", func(t *testing.T) {
		tmpDir := t.TempDir()
		tracker, _ := NewThreadTracker(filepath.Join(tmpDir, "threads.json"))

		tracker.Set("test-plan", &ThreadInfo{
			ThreadTS:         "1234567890.123456",
			ChannelID:        "C123456",
			NotifiedBlockers: []string{"abc12345"},
		})

		added, err := tracker.AddNotifiedBlocker("test-plan", "abc12345")
		if err != nil {
			t.Fatalf("AddNotifiedBlocker() error = %v", err)
		}
		if added {
			t.Error("expected added to be false for existing hash")
		}

		info := tracker.Get("test-plan")
		if len(info.NotifiedBlockers) != 1 {
			t.Errorf("expected 1 blocker, got %d", len(info.NotifiedBlockers))
		}
	})

	t.Run("returns error for non-existent plan", func(t *testing.T) {
		tmpDir := t.TempDir()
		tracker, _ := NewThreadTracker(filepath.Join(tmpDir, "threads.json"))

		_, err := tracker.AddNotifiedBlocker("non-existent", "abc12345")
		if err == nil {
			t.Error("expected error for non-existent plan")
		}
	})
}

func TestThreadTracker_HasNotifiedBlocker(t *testing.T) {
	tmpDir := t.TempDir()
	tracker, _ := NewThreadTracker(filepath.Join(tmpDir, "threads.json"))

	tracker.Set("test-plan", &ThreadInfo{
		ThreadTS:         "1234567890.123456",
		ChannelID:        "C123456",
		NotifiedBlockers: []string{"abc12345"},
	})

	t.Run("returns true for existing hash", func(t *testing.T) {
		if !tracker.HasNotifiedBlocker("test-plan", "abc12345") {
			t.Error("expected true for existing hash")
		}
	})

	t.Run("returns false for non-existing hash", func(t *testing.T) {
		if tracker.HasNotifiedBlocker("test-plan", "xyz99999") {
			t.Error("expected false for non-existing hash")
		}
	})

	t.Run("returns false for non-existing plan", func(t *testing.T) {
		if tracker.HasNotifiedBlocker("non-existent", "abc12345") {
			t.Error("expected false for non-existing plan")
		}
	})
}

func TestThreadTracker_List(t *testing.T) {
	tmpDir := t.TempDir()
	tracker, _ := NewThreadTracker(filepath.Join(tmpDir, "threads.json"))

	tracker.Set("plan-1", &ThreadInfo{ThreadTS: "111", ChannelID: "C1"})
	tracker.Set("plan-2", &ThreadInfo{ThreadTS: "222", ChannelID: "C2"})

	list := tracker.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 threads, got %d", len(list))
	}

	// Verify it returns copies
	for _, info := range list {
		info.ThreadTS = "modified"
	}

	list2 := tracker.List()
	for _, info := range list2 {
		if info.ThreadTS == "modified" {
			t.Error("List() should return copies")
		}
	}
}

func TestThreadTracker_Reload(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "threads.json")
	tracker, _ := NewThreadTracker(filePath)

	tracker.Set("test-plan", &ThreadInfo{ThreadTS: "111", ChannelID: "C1"})

	// Modify file directly
	newData := map[string]*ThreadInfo{
		"other-plan": {ThreadTS: "222", ChannelID: "C2"},
	}
	data, _ := json.Marshal(newData)
	os.WriteFile(filePath, data, 0644)

	// Reload
	if err := tracker.Reload(); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	// Original should be gone
	if tracker.Get("test-plan") != nil {
		t.Error("expected test-plan to be gone after reload")
	}

	// New plan should exist
	if tracker.Get("other-plan") == nil {
		t.Error("expected other-plan to exist after reload")
	}
}

func TestThreadTracker_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "threads.json")

	// Create and populate tracker
	tracker1, _ := NewThreadTracker(filePath)
	tracker1.Set("test-plan", &ThreadInfo{
		ThreadTS:         "1234567890.123456",
		ChannelID:        "C123456",
		NotifiedBlockers: []string{"abc12345", "def67890"},
	})

	// Create new tracker from same file
	tracker2, err := NewThreadTracker(filePath)
	if err != nil {
		t.Fatalf("NewThreadTracker() error = %v", err)
	}

	info := tracker2.Get("test-plan")
	if info == nil {
		t.Fatal("expected thread info to be persisted")
	}
	if info.ThreadTS != "1234567890.123456" {
		t.Errorf("expected ThreadTS 1234567890.123456, got %s", info.ThreadTS)
	}
	if len(info.NotifiedBlockers) != 2 {
		t.Errorf("expected 2 notified blockers, got %d", len(info.NotifiedBlockers))
	}
}

func TestThreadTracker_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	tracker, _ := NewThreadTracker(filepath.Join(tmpDir, "threads.json"))

	var wg sync.WaitGroup
	numGoroutines := 10
	numOperations := 100

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				planName := "test-plan"
				tracker.Set(planName, &ThreadInfo{
					ThreadTS:  "1234567890.123456",
					ChannelID: "C123456",
				})
				tracker.Get(planName)
				tracker.HasNotifiedBlocker(planName, "abc")
			}
		}(i)
	}

	wg.Wait()

	// Verify no corruption
	info := tracker.Get("test-plan")
	if info == nil {
		t.Fatal("expected thread info after concurrent access")
	}
}

func TestThreadTracker_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "threads.json")
	tracker, _ := NewThreadTracker(filePath)

	// Set data
	tracker.Set("test-plan", &ThreadInfo{
		ThreadTS:  "1234567890.123456",
		ChannelID: "C123456",
	})

	// Verify temp file doesn't exist after write
	tmpPath := filePath + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("temp file should not exist after successful write")
	}

	// Verify actual file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("data file should exist after write")
	}
}
