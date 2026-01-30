package runner

import (
	"testing"
)

func TestExtractBlocker_NoBlocker(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{"empty string", ""},
		{"no blocker tag", "Some normal output without any blockers"},
		{"partial opening tag", "<blocker Some content"},
		{"partial closing tag", "Some content</blocker>"},
		{"malformed tags", "<blocker>content<blocker>"},
		{"unclosed tag", "<blocker>content without closing tag"},
		{"empty blocker", "<blocker></blocker>"},
		{"whitespace only blocker", "<blocker>   \n  </blocker>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocker := ExtractBlocker(tt.output)
			if blocker != nil {
				t.Errorf("expected nil blocker, got %+v", blocker)
			}
		})
	}
}

func TestExtractBlocker_SimpleContent(t *testing.T) {
	output := `Some output before
<blocker>
Need human approval to continue.
</blocker>
Some output after`

	blocker := ExtractBlocker(output)
	if blocker == nil {
		t.Fatal("expected blocker, got nil")
	}

	expectedContent := "Need human approval to continue."
	if blocker.Content != expectedContent {
		t.Errorf("Content = %q, want %q", blocker.Content, expectedContent)
	}

	if blocker.Description != expectedContent {
		t.Errorf("Description = %q, want %q", blocker.Description, expectedContent)
	}

	if blocker.Action != "" {
		t.Errorf("Action = %q, want empty", blocker.Action)
	}

	if blocker.Resume != "" {
		t.Errorf("Resume = %q, want empty", blocker.Resume)
	}

	if len(blocker.Hash) != 8 {
		t.Errorf("Hash length = %d, want 8", len(blocker.Hash))
	}
}

func TestExtractBlocker_StructuredFields(t *testing.T) {
	output := `<blocker>
GitHub package visibility must be set to public via web UI.
Action: Go to https://github.com/.../packages → Settings → Change visibility to Public
Resume: Once public, I will verify anonymous pull works and complete T1.
</blocker>`

	blocker := ExtractBlocker(output)
	if blocker == nil {
		t.Fatal("expected blocker, got nil")
	}

	if blocker.Description != "GitHub package visibility must be set to public via web UI." {
		t.Errorf("Description = %q", blocker.Description)
	}

	if blocker.Action != "Go to https://github.com/.../packages → Settings → Change visibility to Public" {
		t.Errorf("Action = %q", blocker.Action)
	}

	if blocker.Resume != "Once public, I will verify anonymous pull works and complete T1." {
		t.Errorf("Resume = %q", blocker.Resume)
	}
}

func TestExtractBlocker_WithExplicitDescriptionField(t *testing.T) {
	output := `<blocker>
Description: The API key needs to be refreshed.
Action: Generate a new API key at https://api.example.com
Resume: I will update the config with the new key.
</blocker>`

	blocker := ExtractBlocker(output)
	if blocker == nil {
		t.Fatal("expected blocker, got nil")
	}

	if blocker.Description != "The API key needs to be refreshed." {
		t.Errorf("Description = %q, want %q", blocker.Description, "The API key needs to be refreshed.")
	}

	if blocker.Action != "Generate a new API key at https://api.example.com" {
		t.Errorf("Action = %q", blocker.Action)
	}
}

func TestExtractBlocker_PartialFields(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		wantDesc string
		wantAction string
		wantResume string
	}{
		{
			name: "only action",
			output: `<blocker>
Action: Do something
</blocker>`,
			wantDesc: "",
			wantAction: "Do something",
			wantResume: "",
		},
		{
			name: "only resume",
			output: `<blocker>
Resume: Will continue after
</blocker>`,
			wantDesc: "",
			wantAction: "",
			wantResume: "Will continue after",
		},
		{
			name: "desc and action only",
			output: `<blocker>
Need approval
Action: Approve the PR
</blocker>`,
			wantDesc: "Need approval",
			wantAction: "Approve the PR",
			wantResume: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocker := ExtractBlocker(tt.output)
			if blocker == nil {
				t.Fatal("expected blocker, got nil")
			}

			if blocker.Description != tt.wantDesc {
				t.Errorf("Description = %q, want %q", blocker.Description, tt.wantDesc)
			}
			if blocker.Action != tt.wantAction {
				t.Errorf("Action = %q, want %q", blocker.Action, tt.wantAction)
			}
			if blocker.Resume != tt.wantResume {
				t.Errorf("Resume = %q, want %q", blocker.Resume, tt.wantResume)
			}
		})
	}
}

func TestExtractBlocker_MultilineDescription(t *testing.T) {
	output := `<blocker>
The deployment pipeline is failing because:
1. The secret is expired
2. The token needs refresh
Action: Refresh the credentials
</blocker>`

	blocker := ExtractBlocker(output)
	if blocker == nil {
		t.Fatal("expected blocker, got nil")
	}

	expectedDesc := `The deployment pipeline is failing because:
1. The secret is expired
2. The token needs refresh`
	if blocker.Description != expectedDesc {
		t.Errorf("Description = %q, want %q", blocker.Description, expectedDesc)
	}
}

func TestExtractBlocker_CaseInsensitiveFields(t *testing.T) {
	output := `<blocker>
Some description
ACTION: Do the thing
RESUME: After it's done
</blocker>`

	blocker := ExtractBlocker(output)
	if blocker == nil {
		t.Fatal("expected blocker, got nil")
	}

	if blocker.Action != "Do the thing" {
		t.Errorf("Action = %q, want %q", blocker.Action, "Do the thing")
	}

	if blocker.Resume != "After it's done" {
		t.Errorf("Resume = %q, want %q", blocker.Resume, "After it's done")
	}
}

func TestExtractBlocker_Hash(t *testing.T) {
	output1 := "<blocker>Content A</blocker>"
	output2 := "<blocker>Content B</blocker>"
	output3 := "<blocker>Content A</blocker>"

	blocker1 := ExtractBlocker(output1)
	blocker2 := ExtractBlocker(output2)
	blocker3 := ExtractBlocker(output3)

	// Same content should produce same hash
	if blocker1.Hash != blocker3.Hash {
		t.Errorf("Same content should produce same hash: %q != %q", blocker1.Hash, blocker3.Hash)
	}

	// Different content should produce different hash
	if blocker1.Hash == blocker2.Hash {
		t.Errorf("Different content should produce different hash: %q == %q", blocker1.Hash, blocker2.Hash)
	}

	// Hash should be 8 characters
	if len(blocker1.Hash) != 8 {
		t.Errorf("Hash length = %d, want 8", len(blocker1.Hash))
	}
}

func TestExtractBlocker_InMiddleOfOutput(t *testing.T) {
	output := `
I've analyzed the codebase and found several issues.

<blocker>
The database migration needs to be run manually.
Action: Run 'make migrate' on the production server
Resume: Once migration is complete, I'll continue with the deployment
</blocker>

Continuing with other analysis...
`

	blocker := ExtractBlocker(output)
	if blocker == nil {
		t.Fatal("expected blocker, got nil")
	}

	if blocker.Description != "The database migration needs to be run manually." {
		t.Errorf("Description = %q", blocker.Description)
	}
}

func TestExtractBlocker_OnlyFirstMatch(t *testing.T) {
	// If there are multiple blockers, only the first should be extracted
	output := `
<blocker>First blocker</blocker>
<blocker>Second blocker</blocker>
`

	blocker := ExtractBlocker(output)
	if blocker == nil {
		t.Fatal("expected blocker, got nil")
	}

	if blocker.Content != "First blocker" {
		t.Errorf("Content = %q, want 'First blocker'", blocker.Content)
	}
}

func TestHasBlocker(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{"no blocker", "normal output", false},
		{"with blocker", "<blocker>content</blocker>", true},
		{"empty blocker", "<blocker></blocker>", true},
		{"partial tag", "<blocker>content", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasBlocker(tt.output)
			if got != tt.want {
				t.Errorf("HasBlocker() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestComputeBlockerHash(t *testing.T) {
	// Verify hash is consistent and correct format
	hash1 := computeBlockerHash("test content")
	hash2 := computeBlockerHash("test content")
	hash3 := computeBlockerHash("different content")

	if hash1 != hash2 {
		t.Errorf("Same content should produce same hash")
	}

	if hash1 == hash3 {
		t.Errorf("Different content should produce different hash")
	}

	// Verify it's a valid hex string of length 8
	if len(hash1) != 8 {
		t.Errorf("Hash length = %d, want 8", len(hash1))
	}

	for _, c := range hash1 {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("Hash contains invalid hex character: %c", c)
		}
	}
}

func TestParseBlockerFields(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantDesc string
		wantAct  string
		wantRes  string
	}{
		{
			name:     "simple description",
			content:  "Simple description only",
			wantDesc: "Simple description only",
			wantAct:  "",
			wantRes:  "",
		},
		{
			name:     "all fields",
			content:  "My description\nAction: My action\nResume: My resume",
			wantDesc: "My description",
			wantAct:  "My action",
			wantRes:  "My resume",
		},
		{
			name:     "description prefix",
			content:  "Description: Explicit description\nAction: Do something",
			wantDesc: "Explicit description",
			wantAct:  "Do something",
			wantRes:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc, act, res := parseBlockerFields(tt.content)
			if desc != tt.wantDesc {
				t.Errorf("description = %q, want %q", desc, tt.wantDesc)
			}
			if act != tt.wantAct {
				t.Errorf("action = %q, want %q", act, tt.wantAct)
			}
			if res != tt.wantRes {
				t.Errorf("resume = %q, want %q", res, tt.wantRes)
			}
		})
	}
}
