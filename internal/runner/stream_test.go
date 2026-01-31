package runner

import (
	"strings"
	"testing"
)

// Sample Claude CLI stream-json output for testing
const sampleAssistantEvent = `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello, world!"}]}}`
const sampleResultEvent = `{"type":"result","result":"Task completed successfully"}`
const sampleToolUseEvent = `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"bash","input":{"command":"ls"}}]}}`
const sampleMixedContent = `{"type":"assistant","message":{"content":[{"type":"text","text":"Running command: "},{"type":"tool_use","name":"bash"},{"type":"text","text":" done"}]}}`

func TestStreamParser_ParseAssistantEvent(t *testing.T) {
	p := NewStreamParser()

	p.Parse([]byte(sampleAssistantEvent + "\n"))

	if text := p.TextContent(); text != "Hello, world!" {
		t.Errorf("expected text content 'Hello, world!', got: %s", text)
	}
}

func TestStreamParser_ParseResultEvent(t *testing.T) {
	p := NewStreamParser()

	p.Parse([]byte(sampleResultEvent + "\n"))

	if !p.HasResult() {
		t.Error("expected HasResult() to be true")
	}
	if result := p.ResultContent(); result != "Task completed successfully" {
		t.Errorf("expected result 'Task completed successfully', got: %s", result)
	}
}

func TestStreamParser_ParseMultipleEvents(t *testing.T) {
	p := NewStreamParser()

	input := strings.Join([]string{
		`{"type":"assistant","message":{"content":[{"type":"text","text":"First "}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Second "}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Third"}]}}`,
		sampleResultEvent,
	}, "\n") + "\n"

	p.Parse([]byte(input))

	expected := "First Second Third"
	if text := p.TextContent(); text != expected {
		t.Errorf("expected text content '%s', got: '%s'", expected, text)
	}

	if !p.HasResult() {
		t.Error("expected HasResult() to be true")
	}
}

func TestStreamParser_SkipsNonJSONLines(t *testing.T) {
	p := NewStreamParser()

	input := `Some error message
{"type":"assistant","message":{"content":[{"type":"text","text":"Hello"}]}}
Another error message
`
	p.Parse([]byte(input))

	if text := p.TextContent(); text != "Hello" {
		t.Errorf("expected text content 'Hello', got: '%s'", text)
	}
}

func TestStreamParser_SkipsEmptyLines(t *testing.T) {
	p := NewStreamParser()

	input := `
{"type":"assistant","message":{"content":[{"type":"text","text":"Hello"}]}}

{"type":"assistant","message":{"content":[{"type":"text","text":" World"}]}}

`
	p.Parse([]byte(input))

	if text := p.TextContent(); text != "Hello World" {
		t.Errorf("expected text content 'Hello World', got: '%s'", text)
	}
}

func TestStreamParser_HandlesPartialLines(t *testing.T) {
	p := NewStreamParser()

	// Send partial line
	p.Parse([]byte(`{"type":"assistant","message":{"content":[{"type":"text"`))

	// Text should not be extracted yet
	if text := p.TextContent(); text != "" {
		t.Errorf("expected empty text content, got: '%s'", text)
	}

	// Complete the line
	p.Parse([]byte(`,"text":"Hello"}]}}` + "\n"))

	if text := p.TextContent(); text != "Hello" {
		t.Errorf("expected text content 'Hello', got: '%s'", text)
	}
}

func TestStreamParser_HandlesInvalidJSON(t *testing.T) {
	var errorCalled bool
	var errorLine string

	p := NewStreamParser()
	p.OnError = func(err error, line string) {
		errorCalled = true
		errorLine = line
	}

	p.Parse([]byte("{invalid json}\n"))

	if !errorCalled {
		t.Error("expected OnError to be called")
	}
	if errorLine != "{invalid json}" {
		t.Errorf("expected error line '{invalid json}', got: '%s'", errorLine)
	}
}

func TestStreamParser_OnTextCallback(t *testing.T) {
	var texts []string

	p := NewStreamParser()
	p.OnText = func(text string) {
		texts = append(texts, text)
	}

	input := strings.Join([]string{
		`{"type":"assistant","message":{"content":[{"type":"text","text":"First"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Second"}]}}`,
	}, "\n") + "\n"

	p.Parse([]byte(input))

	if len(texts) != 2 {
		t.Errorf("expected 2 OnText calls, got: %d", len(texts))
	}
	if texts[0] != "First" {
		t.Errorf("expected first text 'First', got: '%s'", texts[0])
	}
	if texts[1] != "Second" {
		t.Errorf("expected second text 'Second', got: '%s'", texts[1])
	}
}

func TestStreamParser_OnResultCallback(t *testing.T) {
	var resultCalled bool
	var resultContent string

	p := NewStreamParser()
	p.OnResult = func(result string) {
		resultCalled = true
		resultContent = result
	}

	p.Parse([]byte(sampleResultEvent + "\n"))

	if !resultCalled {
		t.Error("expected OnResult to be called")
	}
	if resultContent != "Task completed successfully" {
		t.Errorf("expected result 'Task completed successfully', got: '%s'", resultContent)
	}
}

func TestStreamParser_SkipsToolUseContent(t *testing.T) {
	p := NewStreamParser()

	p.Parse([]byte(sampleToolUseEvent + "\n"))

	// Tool use events should not contribute to text content
	if text := p.TextContent(); text != "" {
		t.Errorf("expected empty text content for tool_use, got: '%s'", text)
	}
}

func TestStreamParser_ExtractsMixedContent(t *testing.T) {
	p := NewStreamParser()

	p.Parse([]byte(sampleMixedContent + "\n"))

	// Should extract only text blocks, ignoring tool_use
	if text := p.TextContent(); text != "Running command:  done" {
		t.Errorf("expected text 'Running command:  done', got: '%s'", text)
	}
}

func TestStreamParser_FullOutput(t *testing.T) {
	p := NewStreamParser()

	input := "line1\nline2\nline3\n"
	p.Parse([]byte(input))

	if output := p.FullOutput(); output != input {
		t.Errorf("expected full output '%s', got: '%s'", input, output)
	}
}

func TestStreamParser_Reset(t *testing.T) {
	p := NewStreamParser()

	p.Parse([]byte(sampleAssistantEvent + "\n"))
	p.Parse([]byte(sampleResultEvent + "\n"))

	if text := p.TextContent(); text == "" {
		t.Error("expected text content before reset")
	}

	p.Reset()

	if text := p.TextContent(); text != "" {
		t.Errorf("expected empty text content after reset, got: '%s'", text)
	}
	if p.HasResult() {
		t.Error("expected HasResult() to be false after reset")
	}
	if output := p.FullOutput(); output != "" {
		t.Errorf("expected empty full output after reset, got: '%s'", output)
	}
}

func TestStreamParser_ParseReader(t *testing.T) {
	p := NewStreamParser()

	input := strings.Join([]string{
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Hello from reader"}]}}`,
		sampleResultEvent,
	}, "\n") + "\n"

	err := p.ParseReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseReader error: %v", err)
	}

	if text := p.TextContent(); text != "Hello from reader" {
		t.Errorf("expected text 'Hello from reader', got: '%s'", text)
	}
	if !p.HasResult() {
		t.Error("expected HasResult() to be true")
	}
}

func TestStreamParser_RealWorldSample(t *testing.T) {
	// Simulate real Claude CLI output with multiple events
	input := `{"type":"init","session_id":"abc123"}
{"type":"assistant","message":{"content":[{"type":"text","text":"I'll help you with that task.\n\n"}]}}
{"type":"assistant","message":{"content":[{"type":"text","text":"Let me analyze the code...\n"}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/path/to/file"}}]}}
{"type":"tool_result","content":"file contents here"}
{"type":"assistant","message":{"content":[{"type":"text","text":"Based on the code, here's my analysis:\n\n1. First point\n2. Second point"}]}}
{"type":"result","result":"Analysis complete"}
`

	p := NewStreamParser()
	p.Parse([]byte(input))

	expectedText := "I'll help you with that task.\n\nLet me analyze the code...\nBased on the code, here's my analysis:\n\n1. First point\n2. Second point"
	if text := p.TextContent(); text != expectedText {
		t.Errorf("expected text:\n%s\ngot:\n%s", expectedText, text)
	}

	if !p.HasResult() {
		t.Error("expected HasResult() to be true")
	}
	if result := p.ResultContent(); result != "Analysis complete" {
		t.Errorf("expected result 'Analysis complete', got: '%s'", result)
	}
}

func TestStreamParser_EmptyContentArray(t *testing.T) {
	p := NewStreamParser()

	// Some events might have empty content arrays
	p.Parse([]byte(`{"type":"assistant","message":{"content":[]}}` + "\n"))

	if text := p.TextContent(); text != "" {
		t.Errorf("expected empty text content, got: '%s'", text)
	}
}

func TestStreamParser_TextWithEmptyString(t *testing.T) {
	p := NewStreamParser()

	// Text block with empty string should be ignored
	p.Parse([]byte(`{"type":"assistant","message":{"content":[{"type":"text","text":""}]}}` + "\n"))

	if text := p.TextContent(); text != "" {
		t.Errorf("expected empty text content, got: '%s'", text)
	}
}

func TestStreamParser_ConcurrentAccess(t *testing.T) {
	p := NewStreamParser()

	// Ensure concurrent access is safe
	done := make(chan bool, 3)

	go func() {
		for i := 0; i < 100; i++ {
			p.Parse([]byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"a"}]}}` + "\n"))
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			_ = p.TextContent()
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			_ = p.HasResult()
		}
		done <- true
	}()

	for i := 0; i < 3; i++ {
		<-done
	}

	// Just verify it didn't panic or deadlock
	text := p.TextContent()
	if len(text) == 0 {
		t.Error("expected some text content from concurrent writes")
	}
}

func TestStreamParser_UnknownEventTypes(t *testing.T) {
	p := NewStreamParser()

	// Unknown event types should be gracefully ignored
	input := `{"type":"init","session_id":"abc"}
{"type":"tool_result","content":"result"}
{"type":"unknown_type","data":"value"}
{"type":"assistant","message":{"content":[{"type":"text","text":"Hello"}]}}
`
	p.Parse([]byte(input))

	if text := p.TextContent(); text != "Hello" {
		t.Errorf("expected text 'Hello', got: '%s'", text)
	}
}
