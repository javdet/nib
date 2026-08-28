package systemprompts

import (
	"path/filepath"
	"testing"
)

func TestResolveDir(t *testing.T) {
	t.Parallel()

	data := "/var/data"
	tests := []struct {
		name       string
		dataDir    string
		promptsDir string
		want       string
	}{
		{
			name:       "empty uses data prompts",
			dataDir:    data,
			promptsDir: "",
			want:       filepath.Join(data, "prompts"),
		},
		{
			name:       "relative under data",
			dataDir:    data,
			promptsDir: "custom/prompts",
			want:       filepath.Join(data, "custom/prompts"),
		},
		{
			name:       "absolute unchanged",
			dataDir:    data,
			promptsDir: "/opt/prompts",
			want:       "/opt/prompts",
		},
		{
			name:       "whitespace treated as empty",
			dataDir:    data,
			promptsDir: "  \t ",
			want:       filepath.Join(data, "prompts"),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ResolveDir(tt.dataDir, tt.promptsDir); got != tt.want {
				t.Errorf("ResolveDir() = %q, want %q", got, tt.want)
			}
		})
	}
}
