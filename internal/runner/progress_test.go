package runner

import "testing"

func TestExtractProgress(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "extracts progress tag",
			input:  "Some output\n<progress>\nAdded BoardAPIClient with adaptive polling. Criteria 45, 46 checked.\n</progress>\nMore output",
			expect: "Added BoardAPIClient with adaptive polling. Criteria 45, 46 checked.",
		},
		{
			name:   "multiline progress",
			input:  "<progress>\nImplemented auth middleware.\nHad to use custom guard due to macOS 13 target.\nCriteria 12, 13, 14 checked.\n</progress>",
			expect: "Implemented auth middleware.\nHad to use custom guard due to macOS 13 target.\nCriteria 12, 13, 14 checked.",
		},
		{
			name:   "no progress tag",
			input:  "Just some regular output without any tags",
			expect: "",
		},
		{
			name:   "empty progress tag",
			input:  "<progress>\n\n</progress>",
			expect: "",
		},
		{
			name:   "progress with completion marker",
			input:  "<progress>\nAll tasks done. Final cleanup.\n</progress>\n<promise>COMPLETE</promise>",
			expect: "All tasks done. Final cleanup.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractProgress(tt.input)
			if got != tt.expect {
				t.Errorf("ExtractProgress() = %q, want %q", got, tt.expect)
			}
		})
	}
}
