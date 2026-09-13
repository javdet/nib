package llm

import "testing"

func TestCostFromUsageJSON(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want *float64
	}{
		{name: "openrouter reports a cost", raw: `{"prompt_tokens":10,"cost":0.00012}`, want: ptr(0.00012)},
		{name: "cost below the old numeric(12,6) scale", raw: `{"cost":0.0000002}`, want: ptr(0.0000002)},
		{name: "explicit zero is a real report", raw: `{"cost":0}`, want: ptr(0.0)},
		{name: "openai usage carries no cost", raw: `{"prompt_tokens":10,"completion_tokens":5}`},
		{name: "empty raw json", raw: ``},
		{name: "malformed json", raw: `{`},
		{name: "null cost", raw: `{"cost":null}`},
		{name: "cost as a string is not trusted", raw: `{"cost":"0.1"}`},
		{name: "negative cost is not trusted", raw: `{"cost":-1}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := costFromUsageJSON(tt.raw)
			switch {
			case tt.want == nil && got != nil:
				t.Fatalf("costFromUsageJSON(%q) = %v, want nil", tt.raw, *got)
			case tt.want != nil && got == nil:
				t.Fatalf("costFromUsageJSON(%q) = nil, want %v", tt.raw, *tt.want)
			case tt.want != nil && *got != *tt.want:
				t.Fatalf("costFromUsageJSON(%q) = %v, want %v", tt.raw, *got, *tt.want)
			}
		})
	}
}

func ptr(f float64) *float64 { return &f }
