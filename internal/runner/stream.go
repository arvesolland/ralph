// Package runner handles Claude CLI execution.
package runner

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"sync"
)

// StreamEvent represents a parsed event from Claude CLI stream-json output.
type StreamEvent struct {
	Type    string `json:"type"`
	Message struct {
		Content []ContentBlock `json:"content"`
	} `json:"message"`
	Result string `json:"result"`
}

// ContentBlock represents a content block within a message.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// StreamParser parses Claude CLI streaming JSON output line-by-line.
type StreamParser struct {
	mu sync.Mutex

	// buffer holds incomplete line data
	buffer strings.Builder

	// fullOutput accumulates all raw output
	fullOutput strings.Builder

	// textContent accumulates all extracted text content
	textContent strings.Builder

	// hasResult tracks whether we've seen a result event
	hasResult bool

	// resultContent holds the final result
	resultContent string

	// OnText is called for each text chunk extracted from the stream
	OnText func(text string)

	// OnResult is called when a result event is received
	OnResult func(result string)

	// OnError is called when a parse error occurs (non-fatal, for logging)
	OnError func(err error, line string)
}

// NewStreamParser creates a new parser with optional callbacks.
func NewStreamParser() *StreamParser {
	return &StreamParser{}
}

// Parse processes a chunk of data from the stream.
// It handles partial lines by buffering until a complete line is received.
func (p *StreamParser) Parse(data []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Append to full output
	p.fullOutput.Write(data)

	// Append to buffer for line parsing
	p.buffer.Write(data)

	// Process complete lines
	p.processLines()
}

// ParseReader reads from an io.Reader and parses all content.
// This is useful for processing complete output.
func (p *StreamParser) ParseReader(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	// Increase buffer size for potentially long JSON lines
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		p.mu.Lock()
		p.fullOutput.WriteString(line)
		p.fullOutput.WriteString("\n")
		p.parseLine(line)
		p.mu.Unlock()
	}

	return scanner.Err()
}

// processLines extracts and processes complete lines from the buffer.
func (p *StreamParser) processLines() {
	content := p.buffer.String()

	for {
		idx := strings.Index(content, "\n")
		if idx < 0 {
			break
		}

		line := content[:idx]
		content = content[idx+1:]

		p.parseLine(line)
	}

	// Keep remaining partial line in buffer
	p.buffer.Reset()
	p.buffer.WriteString(content)
}

// parseLine parses a single line of JSON output.
func (p *StreamParser) parseLine(line string) {
	// Skip empty lines
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	// Skip non-JSON lines (error messages, etc.)
	if !strings.HasPrefix(line, "{") {
		return
	}

	var event StreamEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		if p.OnError != nil {
			p.OnError(err, line)
		}
		return
	}

	switch event.Type {
	case "assistant":
		// Extract text content from assistant messages
		for _, block := range event.Message.Content {
			if block.Type == "text" && block.Text != "" {
				p.textContent.WriteString(block.Text)
				if p.OnText != nil {
					p.OnText(block.Text)
				}
			}
		}

	case "result":
		p.hasResult = true
		p.resultContent = event.Result
		if p.OnResult != nil {
			p.OnResult(event.Result)
		}
	}
}

// FullOutput returns all raw output received.
func (p *StreamParser) FullOutput() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.fullOutput.String()
}

// TextContent returns all extracted text content.
func (p *StreamParser) TextContent() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.textContent.String()
}

// HasResult returns true if a result event was received.
func (p *StreamParser) HasResult() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.hasResult
}

// ResultContent returns the result content if a result event was received.
func (p *StreamParser) ResultContent() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.resultContent
}

// Reset clears the parser state.
func (p *StreamParser) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.buffer.Reset()
	p.fullOutput.Reset()
	p.textContent.Reset()
	p.hasResult = false
	p.resultContent = ""
}
