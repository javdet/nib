package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/google/shlex"
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

	return runCurl(ctx, argv)
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

func defaultRunCurl(ctx context.Context, argv []string) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, apiCallTimeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "curl", argv...)
	cmd.Env = toolChildEnv()
	out, err := cmd.CombinedOutput()
	result := capAPICallOutput(string(out))

	if err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return result + "\n[error: curl timed out]", nil
		}
		if len(result) > 0 {
			return result + fmt.Sprintf("\n[exit error: %v]", err), nil
		}
		return fmt.Sprintf("[exit error: %v]", err), nil
	}

	return result, nil
}

func capAPICallOutput(s string) string {
	return textutil.TruncateBytes(s, apiCallTruncationSuffix, apiCallMaxOutputBytes)
}
