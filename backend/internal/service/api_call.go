package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/shlex"
	"github.com/javdet/nib/internal/cmdrun"
	"github.com/javdet/nib/internal/llm"
	"github.com/javdet/nib/internal/textutil"
)

const (
	APICallToolName         = "api_call"
	apiCallTimeout          = 60 * time.Second
	apiCallMaxOutputBytes   = 1 << 20 // 1 MiB
	apiCallTruncationSuffix = "\n... [output truncated]"
)

var apiCallParameters = json.RawMessage(`{
  "type": "object",
  "required": ["flags"],
  "properties": {
    "flags": {
      "type": "string",
      "description": "curl flags and exactly one http:// or https:// URL (do not include the word curl). Example: -s -X POST https://api.example.com -H \"Content-Type: application/json\" -d '{\"k\":1}'. Shell features like pipes, redirects, and && are not supported."
    }
  }
}`)

// runCurl executes curl with the given argv. Overridden in tests.
var runCurl = defaultRunCurl

// APICallToolDef returns the LLM tool definition for the local api_call handler.
func APICallToolDef() llm.ToolDef {
	flags, valued := curlOptionSummary()
	return llm.ToolDef{
		Name: APICallToolName,
		Description: "Make one HTTP(S) request with curl from the backend host. " +
			"Provide curl flags and exactly one http:// or https:// URL (do not include the word curl). " +
			"Accepted options: " + flags + "; with a value: " + valued + ". " +
			"Anything else is refused: values cannot be read from files (@file), output cannot be written " +
			"to a file other than /dev/null, and redirects are not followed, so read the Location header " +
			"with -i and call that URL. Loopback, link-local and cloud metadata addresses are refused. " +
			"Shell features like pipes, redirects, and && are not supported. " +
			"Returns combined stdout and stderr from curl.",
		Parameters: apiCallParameters,
	}
}

// ExecuteAPICall splits flags into argv, runs curl without a shell, and returns combined output.
// Validation failures are returned as tool output text (not Go errors) so the agent loop can continue.
func ExecuteAPICall(ctx context.Context, args map[string]any) (string, error) {
	return executeAPICall(ctx, args, runCurl)
}

// apiCallHandler runs curl in the command sandbox, so it gets a uid of its own
// and none of the capabilities the backend holds, exactly like a command does.
func (s *ChatService) apiCallHandler() localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		if s.commandJobs == nil {
			return ExecuteAPICall(ctx, args)
		}
		sb := s.commandJobs.Sandbox()
		return executeAPICall(ctx, args, func(ctx context.Context, argv []string) (string, error) {
			return runCurlIn(ctx, sb, argv)
		})
	}
}

func executeAPICall(ctx context.Context, args map[string]any, run func(context.Context, []string) (string, error)) (string, error) {
	flags := strings.TrimSpace(stringArg(args, "flags"))
	if flags == "" {
		return "flags is empty", nil
	}

	argv, err := parseCurlFlags(flags)
	if err != nil {
		return fmt.Sprintf("invalid flags: %v", err), nil
	}
	if len(argv) == 0 {
		return "flags is empty", nil
	}

	vetCtx, cancel := context.WithTimeout(ctx, apiCallTimeout)
	defer cancel()
	argv, err = vetAPICall(vetCtx, argv)
	if err != nil {
		return fmt.Sprintf("rejected: %v", err), nil
	}

	return run(ctx, argv)
}

func parseCurlFlags(flags string) ([]string, error) {
	argv, err := shlex.Split(flags)
	if err != nil {
		return nil, err
	}
	if len(argv) > 0 && strings.EqualFold(argv[0], "curl") {
		argv = argv[1:]
	}
	return argv, nil
}

// defaultRunCurl runs curl unisolated, as the backend's own user. It is only
// reached before SetCommandJobs has wired a sandbox, which is tests.
func defaultRunCurl(ctx context.Context, argv []string) (string, error) {
	sb, err := cmdrun.NewSandbox(cmdrun.SandboxConfig{Isolation: cmdrun.IsolationOff})
	if err != nil {
		return "", fmt.Errorf("api_call sandbox: %w", err)
	}
	return runCurlIn(ctx, sb, argv)
}

func runCurlIn(ctx context.Context, sb *cmdrun.Sandbox, argv []string) (string, error) {
	res, err := sb.RunOnce(ctx, append([]string{"curl"}, argv...), apiCallTimeout, apiCallMaxOutputBytes)
	if err != nil {
		// A refused or failed spawn is the tool's answer, not a turn failure.
		return fmt.Sprintf("[error: %v]", err), nil
	}
	result := string(res.Output)
	if res.Truncated {
		result = capAPICallOutput(result + apiCallTruncationSuffix)
	}
	switch {
	case res.TimedOut:
		return result + "\n[error: curl timed out]", nil
	case res.ExitCode != 0 && result != "":
		return result + fmt.Sprintf("\n[exit error: exit status %d]", res.ExitCode), nil
	case res.ExitCode != 0:
		return fmt.Sprintf("[exit error: exit status %d]", res.ExitCode), nil
	}
	return result, nil
}

func capAPICallOutput(s string) string {
	return textutil.TruncateBytes(s, apiCallTruncationSuffix, apiCallMaxOutputBytes)
}
