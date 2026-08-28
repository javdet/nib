package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestParseCurlFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		flags   string
		want    []string
		wantErr bool
	}{
		{
			name:  "simple get",
			flags: "-s https://example.com",
			want:  []string{"-s", "https://example.com"},
		},
		{
			name:  "quoted header",
			flags: `-X POST https://api.example.com -H "Content-Type: application/json"`,
			want:  []string{"-X", "POST", "https://api.example.com", "-H", "Content-Type: application/json"},
		},
		{
			name:  "json body in single quotes",
			flags: `-d '{"k":1}' https://api.example.com`,
			want:  []string{"-d", `{"k":1}`, "https://api.example.com"},
		},
		{
			name:  "strip leading curl",
			flags: `curl -s https://example.com`,
			want:  []string{"-s", "https://example.com"},
		},
		{
			name:  "strip leading CURL case insensitive",
			flags: `CURL -s https://example.com`,
			want:  []string{"-s", "https://example.com"},
		},
		{
			name:    "unclosed quote",
			flags:   `-H "Content-Type`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseCurlFlags(tt.flags)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseCurlFlags() expected error, got argv=%v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCurlFlags() error = %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseCurlFlags() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("parseCurlFlags()[%d] = %q, want %q (full: %v)", i, got[i], tt.want[i], got)
				}
			}
		})
	}
}

func TestCapAPICallOutput(t *testing.T) {
	t.Parallel()

	short := strings.Repeat("a", 100)
	if got := capAPICallOutput(short); got != short {
		t.Fatalf("capAPICallOutput(short) changed output")
	}

	long := strings.Repeat("b", apiCallMaxOutputBytes+100)
	got := capAPICallOutput(long)
	if len(got) > apiCallMaxOutputBytes {
		t.Fatalf("capAPICallOutput() len = %d, want <= %d", len(got), apiCallMaxOutputBytes)
	}
	if !strings.HasSuffix(got, apiCallTruncationSuffix) {
		t.Fatalf("capAPICallOutput() missing truncation suffix")
	}
	if !utf8.ValidString(got) {
		t.Fatalf("capAPICallOutput() returned invalid UTF-8")
	}

	cyrillic := strings.Repeat("А", apiCallMaxOutputBytes/2+100)
	gotCyrillic := capAPICallOutput(cyrillic)
	if len(gotCyrillic) > apiCallMaxOutputBytes {
		t.Fatalf("capAPICallOutput(cyrillic) len = %d, want <= %d", len(gotCyrillic), apiCallMaxOutputBytes)
	}
	if !utf8.ValidString(gotCyrillic) {
		t.Fatalf("capAPICallOutput(cyrillic) returned invalid UTF-8")
	}
}

func TestExecuteAPICallValidation(t *testing.T) {
	t.Parallel()

	out, err := ExecuteAPICall(context.Background(), map[string]any{"flags": "  "})
	if err != nil {
		t.Fatalf("ExecuteAPICall() error = %v", err)
	}
	if out != "flags is empty" {
		t.Fatalf("ExecuteAPICall() = %q, want %q", out, "flags is empty")
	}

	out, err = ExecuteAPICall(context.Background(), map[string]any{"flags": `-H "broken`})
	if err != nil {
		t.Fatalf("ExecuteAPICall() error = %v", err)
	}
	if !strings.HasPrefix(out, "invalid flags:") {
		t.Fatalf("ExecuteAPICall() = %q, want invalid flags prefix", out)
	}
}

func TestExecuteAPICallRunsCurl(t *testing.T) {
	orig := runCurl
	t.Cleanup(func() { runCurl = orig })

	var gotArgv []string
	runCurl = func(ctx context.Context, argv []string) (string, error) {
		gotArgv = append([]string(nil), argv...)
		return `{"ok":true}`, nil
	}

	out, err := ExecuteAPICall(context.Background(), map[string]any{
		"flags": `-s -X GET https://api.example.com -H "Authorization: Bearer token"`,
	})
	if err != nil {
		t.Fatalf("ExecuteAPICall() error = %v", err)
	}
	if out != `{"ok":true}` {
		t.Fatalf("ExecuteAPICall() = %q, want %q", out, `{"ok":true}`)
	}

	want := []string{"-s", "-X", "GET", "https://api.example.com", "-H", "Authorization: Bearer token"}
	if len(gotArgv) != len(want) {
		t.Fatalf("argv = %v, want %v", gotArgv, want)
	}
	for i := range want {
		if gotArgv[i] != want[i] {
			t.Fatalf("argv[%d] = %q, want %q", i, gotArgv[i], want[i])
		}
	}
}

func TestExecuteAPICallCurlErrorIncludedInOutput(t *testing.T) {
	orig := runCurl
	t.Cleanup(func() { runCurl = orig })

	runCurl = func(ctx context.Context, argv []string) (string, error) {
		return "curl: (6) Could not resolve host\n[exit error: exit status 6]", nil
	}

	out, err := ExecuteAPICall(context.Background(), map[string]any{
		"flags": "-s https://missing.example",
	})
	if err != nil {
		t.Fatalf("ExecuteAPICall() error = %v", err)
	}
	if !strings.Contains(out, "Could not resolve host") {
		t.Fatalf("ExecuteAPICall() = %q, want curl error in output", out)
	}
}

func TestDefaultRunCurlPropagatesUnexpectedError(t *testing.T) {
	orig := runCurl
	t.Cleanup(func() { runCurl = orig })

	runCurl = func(ctx context.Context, argv []string) (string, error) {
		return "", errors.New("internal failure")
	}

	_, err := ExecuteAPICall(context.Background(), map[string]any{
		"flags": "-s https://example.com",
	})
	if err == nil {
		t.Fatal("ExecuteAPICall() expected error from runCurl")
	}
}
