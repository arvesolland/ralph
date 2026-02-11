// Package main implements a fake board binary for integration testing.
// It reads/writes a JSON state file specified by FAKEBOARD_STATE_PATH,
// producing the same CLI interface and output format as the real board.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func main() {
	statePath := os.Getenv("FAKEBOARD_STATE_PATH")
	if statePath == "" {
		fatal("FAKEBOARD_STATE_PATH environment variable is required")
	}

	args := os.Args[1:]

	// Strip global flags (--api-url and --api-token) that the real CLI accepts.
	args = stripGlobalFlags(args)

	if len(args) < 2 {
		fatal("usage: fakeboard <group> <action> [args...] [--flags]")
	}

	group := args[0]
	action := args[1]
	rest := args[2:]

	// Parse flags and positional args from rest.
	positional, flags := parseArgs(rest)

	var (
		result   any
		err      error
		rawText  bool
		textData string
	)

	err = WithState(statePath, func(s *State) error {
		switch group {
		case "project":
			switch action {
			case "context":
				result, err = handleProjectContext(s, positional)
			default:
				return fmt.Errorf("unknown project action: %s", action)
			}

		case "plan":
			switch action {
			case "context":
				formatText := flags["format"] == "text"
				var raw any
				raw, rawText, err = handlePlanContext(s, positional, formatText)
				if rawText {
					textData = raw.(string)
				} else {
					result = raw
				}
			case "list":
				result, err = handlePlanList(s, positional, flags["status"])
			case "show":
				result, err = handlePlanShow(s, positional)
			case "status":
				result, err = handlePlanStatus(s, positional, flags["status"])
			default:
				return fmt.Errorf("unknown plan action: %s", action)
			}

		case "task":
			switch action {
			case "list":
				_, avail := flags["available"]
				result, err = handleTaskList(s, positional, flags["status"], avail)
			case "show":
				result, err = handleTaskShow(s, positional)
			case "claim":
				result, err = handleTaskClaim(s, positional, flags["assignee"])
			case "start":
				result, err = handleTaskStart(s, positional)
			case "complete":
				result, err = handleTaskComplete(s, positional)
			case "block":
				result, err = handleTaskBlock(s, positional, flags["reason"])
			case "skip":
				result, err = handleTaskSkip(s, positional, flags["reason"])
			default:
				return fmt.Errorf("unknown task action: %s", action)
			}

		case "criteria":
			switch action {
			case "check":
				result, err = handleCriteriaCheck(s, positional)
			case "uncheck":
				result, err = handleCriteriaUncheck(s, positional)
			default:
				return fmt.Errorf("unknown criteria action: %s", action)
			}

		case "progress":
			switch action {
			case "add":
				result, err = handleProgressAdd(s, positional, flags["author"], flags["body"])
			default:
				return fmt.Errorf("unknown progress action: %s", action)
			}

		case "feedback":
			switch action {
			case "add":
				result, err = handleFeedbackAdd(s, positional, flags["author"], flags["body"])
			default:
				return fmt.Errorf("unknown feedback action: %s", action)
			}

		default:
			return fmt.Errorf("unknown group: %s", group)
		}
		return err
	})

	if err != nil {
		fatal(err.Error())
	}

	// Output: raw text for --format text, JSON envelope for everything else.
	if rawText {
		fmt.Print(textData)
	} else {
		envelope := map[string]any{"data": result}
		out, err := json.Marshal(envelope)
		if err != nil {
			fatal("failed to marshal response: " + err.Error())
		}
		fmt.Println(string(out))
	}
}

// fatal prints an error to stderr and exits with code 1.
func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "error: "+msg)
	os.Exit(1)
}

// stripGlobalFlags removes --api-url and --api-token (and their values) from args.
func stripGlobalFlags(args []string) []string {
	var result []string
	skip := false
	for i, arg := range args {
		if skip {
			skip = false
			continue
		}
		if arg == "--api-url" || arg == "--api-token" {
			// Skip this flag and its value.
			if i+1 < len(args) {
				skip = true
			}
			continue
		}
		if strings.HasPrefix(arg, "--api-url=") || strings.HasPrefix(arg, "--api-token=") {
			continue
		}
		result = append(result, arg)
	}
	return result
}

// parseArgs separates positional arguments from --flag values.
// Boolean flags (like --available) are stored with value "" in the map.
// Supports both --key value and --key=value formats.
func parseArgs(args []string) ([]string, map[string]string) {
	var positional []string
	flags := make(map[string]string)

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") {
			key := strings.TrimPrefix(arg, "--")
			if idx := strings.Index(key, "="); idx >= 0 {
				flags[key[:idx]] = key[idx+1:]
			} else if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				flags[key] = args[i+1]
				i++
			} else {
				// Boolean flag.
				flags[key] = ""
			}
		} else {
			positional = append(positional, arg)
		}
	}
	return positional, flags
}
