package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestExecuteCommandToolDef(t *testing.T) {
	t.Parallel()

	def := ExecuteCommandToolDef()
	if def.Name != ExecuteCommandToolName {
		t.Fatalf("Name = %q, want %q", def.Name, ExecuteCommandToolName)
	}
	if strings.TrimSpace(def.Description) == "" {
		t.Fatal("Description is empty")
	}
	if len(def.Parameters) == 0 {
		t.Fatal("Parameters is empty")
	}
}

func TestParseCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command string
		want    []string
		wantErr bool
	}{
		{
			name:    "simple command",
			command: "date",
			want:    []string{"date"},
		},
		{
			name:    "command with args",
			command: "base64 --help",
			want:    []string{"base64", "--help"},
		},
		{
			name:    "quoted argument",
			command: `echo "hello world"`,
			want:    []string{"echo", "hello world"},
		},
		{
			name:    "unclosed quote",
			command: `echo "hello`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseCommand(tt.command)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseCommand() expected error, got argv=%v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCommand() error = %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseCommand() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("parseCommand()[%d] = %q, want %q (full: %v)", i, got[i], tt.want[i], got)
				}
			}
		})
	}
}

func TestCapExecuteCommandOutput(t *testing.T) {
	t.Parallel()

	short := strings.Repeat("a", 100)
	if got := capExecuteCommandOutput(short); got != short {
		t.Fatalf("capExecuteCommandOutput(short) changed output")
	}

	long := strings.Repeat("b", executeCommandMaxOutputBytes+100)
	got := capExecuteCommandOutput(long)
	if len(got) > executeCommandMaxOutputBytes {
		t.Fatalf("capExecuteCommandOutput() len = %d, want <= %d", len(got), executeCommandMaxOutputBytes)
	}
	if !strings.HasSuffix(got, executeCommandTruncationSuffix) {
		t.Fatalf("capExecuteCommandOutput() missing truncation suffix")
	}
	if !utf8.ValidString(got) {
		t.Fatalf("capExecuteCommandOutput() returned invalid UTF-8")
	}

	cyrillic := strings.Repeat("А", executeCommandMaxOutputBytes/2+100)
	gotCyrillic := capExecuteCommandOutput(cyrillic)
	if len(gotCyrillic) > executeCommandMaxOutputBytes {
		t.Fatalf("capExecuteCommandOutput(cyrillic) len = %d, want <= %d", len(gotCyrillic), executeCommandMaxOutputBytes)
	}
	if !utf8.ValidString(gotCyrillic) {
		t.Fatalf("capExecuteCommandOutput(cyrillic) returned invalid UTF-8")
	}
}

func TestExecuteCommandValidation(t *testing.T) {
	t.Parallel()

	out, err := ExecuteCommand(context.Background(), map[string]any{"command": "  "})
	if err != nil {
		t.Fatalf("ExecuteCommand() error = %v", err)
	}
	if out != "command is empty" {
		t.Fatalf("ExecuteCommand() = %q, want %q", out, "command is empty")
	}

	out, err = ExecuteCommand(context.Background(), map[string]any{"command": `echo "broken`})
	if err != nil {
		t.Fatalf("ExecuteCommand() error = %v", err)
	}
	if !strings.HasPrefix(out, "invalid command:") {
		t.Fatalf("ExecuteCommand() = %q, want invalid command prefix", out)
	}
}

func TestExecuteCommandRunsCommand(t *testing.T) {
	orig := runCommand
	t.Cleanup(func() { runCommand = orig })

	var gotArgv []string
	runCommand = func(ctx context.Context, argv []string) (string, error) {
		gotArgv = append([]string(nil), argv...)
		return `{"exit_code":0,"output":"ok","command_not_found":false}`, nil
	}

	out, err := ExecuteCommand(context.Background(), map[string]any{
		"command": `base64 --help`,
	})
	if err != nil {
		t.Fatalf("ExecuteCommand() error = %v", err)
	}
	if out != `{"exit_code":0,"output":"ok","command_not_found":false}` {
		t.Fatalf("ExecuteCommand() = %q", out)
	}

	want := []string{"base64", "--help"}
	if len(gotArgv) != len(want) {
		t.Fatalf("argv = %v, want %v", gotArgv, want)
	}
	for i := range want {
		if gotArgv[i] != want[i] {
			t.Fatalf("argv[%d] = %q, want %q", i, gotArgv[i], want[i])
		}
	}
}

func TestExecuteCommandNotFound(t *testing.T) {
	orig := runCommand
	t.Cleanup(func() { runCommand = orig })

	runCommand = func(ctx context.Context, argv []string) (string, error) {
		result := executeCommandResult{
			ExitCode:        -1,
			Output:          "command not found: missing-binary",
			CommandNotFound: true,
		}
		return marshalExecuteCommandResult(result)
	}

	out, err := ExecuteCommand(context.Background(), map[string]any{
		"command": "missing-binary",
	})
	if err != nil {
		t.Fatalf("ExecuteCommand() error = %v", err)
	}

	var parsed executeCommandResult
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !parsed.CommandNotFound {
		t.Fatalf("command_not_found = false, want true")
	}
	if parsed.ExitCode != -1 {
		t.Fatalf("exit_code = %d, want -1", parsed.ExitCode)
	}
}

func TestExecuteCommandNonZeroExitCode(t *testing.T) {
	orig := runCommand
	t.Cleanup(func() { runCommand = orig })

	runCommand = func(ctx context.Context, argv []string) (string, error) {
		result := executeCommandResult{
			ExitCode:        1,
			Output:          "error output",
			CommandNotFound: false,
		}
		return marshalExecuteCommandResult(result)
	}

	out, err := ExecuteCommand(context.Background(), map[string]any{
		"command": "false",
	})
	if err != nil {
		t.Fatalf("ExecuteCommand() error = %v", err)
	}

	var parsed executeCommandResult
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if parsed.ExitCode != 1 {
		t.Fatalf("exit_code = %d, want 1", parsed.ExitCode)
	}
	if parsed.Output != "error output" {
		t.Fatalf("output = %q, want %q", parsed.Output, "error output")
	}
}

func TestExecuteCommandPropagatesUnexpectedError(t *testing.T) {
	orig := runCommand
	t.Cleanup(func() { runCommand = orig })

	runCommand = func(ctx context.Context, argv []string) (string, error) {
		return "", errors.New("internal failure")
	}

	_, err := ExecuteCommand(context.Background(), map[string]any{
		"command": "date",
	})
	if err == nil {
		t.Fatal("ExecuteCommand() expected error from runCommand")
	}
}
