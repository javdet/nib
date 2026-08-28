package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/javdet/nib/internal/llm"
	"github.com/javdet/nib/internal/textutil"
	"github.com/google/shlex"
)

const (
	ExecuteCommandToolName         = "execute_command"
	executeCommandTimeout          = 60 * time.Second
	executeCommandMaxOutputBytes   = 1 << 20 // 1 MiB
	executeCommandTruncationSuffix = "\n... [output truncated]"
)

var executeCommandParameters = json.RawMessage(`{
  "type": "object",
  "required": ["command"],
  "properties": {
    "command": {
      "type": "string",
      "description": "Command name and optional arguments to run on the backend host. Example: date or base64 --help. Shell features like pipes, redirects, and && are not supported."
    }
  }
}`)

// runCommand executes a command with the given argv. Overridden in tests.
var runCommand = defaultRunCommand

// ExecuteCommandToolDef returns the LLM tool definition for the local execute_command handler.
func ExecuteCommandToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name: ExecuteCommandToolName,
		Description: "Run a simple command locally on the backend host. " +
			"Provide the command name and optional arguments (e.g. date or base64 --help). " +
			"Shell features like pipes, redirects, and && are not supported. " +
			"Returns JSON with exit_code, output (combined stdout/stderr), and command_not_found.",
		Parameters: executeCommandParameters,
	}
}

// executeCommandResult is the JSON payload returned to the LLM.
type executeCommandResult struct {
	ExitCode          int    `json:"exit_code"`
	Output            string `json:"output"`
	CommandNotFound   bool   `json:"command_not_found"`
}

// ExecuteCommand splits command into argv, runs it without a shell, and returns JSON result.
// Validation failures are returned as tool output text (not Go errors) so the agent loop can continue.
func ExecuteCommand(ctx context.Context, args map[string]any) (string, error) {
	command := strings.TrimSpace(stringArg(args, "command"))
	if command == "" {
		return "command is empty", nil
	}

	argv, err := parseCommand(command)
	if err != nil {
		return fmt.Sprintf("invalid command: %v", err), nil
	}
	if len(argv) == 0 {
		return "command is empty", nil
	}

	return runCommand(ctx, argv)
}

func parseCommand(command string) ([]string, error) {
	return shlex.Split(command)
}

func defaultRunCommand(ctx context.Context, argv []string) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, executeCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, argv[0], argv[1:]...)
	out, err := cmd.CombinedOutput()
	result := executeCommandResult{
		Output:          capExecuteCommandOutput(string(out)),
		CommandNotFound: false,
	}

	if err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			result.ExitCode = -1
			result.Output = result.Output + "\n[error: command timed out]"
			return marshalExecuteCommandResult(result)
		}
		if errors.Is(err, exec.ErrNotFound) {
			result.ExitCode = -1
			result.CommandNotFound = true
			if result.Output == "" {
				result.Output = fmt.Sprintf("command not found: %s", argv[0])
			}
			return marshalExecuteCommandResult(result)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
			return marshalExecuteCommandResult(result)
		}
		return "", fmt.Errorf("execute command: %w", err)
	}

	result.ExitCode = 0
	return marshalExecuteCommandResult(result)
}

func marshalExecuteCommandResult(result executeCommandResult) (string, error) {
	b, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshal execute command result: %w", err)
	}
	return string(b), nil
}

func capExecuteCommandOutput(s string) string {
	return textutil.TruncateBytes(s, executeCommandTruncationSuffix, executeCommandMaxOutputBytes)
}
