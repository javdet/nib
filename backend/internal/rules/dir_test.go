package rules

import (
	"path/filepath"
	"testing"
)

func TestResolveDir(t *testing.T) {
	t.Parallel()

	data := "/var/data"
	tests := []struct {
		name     string
		dataDir  string
		rulesDir string
		want     string
	}{
		{
			name:     "empty uses data rules",
			dataDir:  data,
			rulesDir: "",
			want:     filepath.Join(data, "rules"),
		},
		{
			name:     "relative under data",
			dataDir:  data,
			rulesDir: "custom/rules",
			want:     filepath.Join(data, "custom/rules"),
		},
		{
			name:     "absolute unchanged",
			dataDir:  data,
			rulesDir: "/opt/rules",
			want:     "/opt/rules",
		},
		{
			name:     "whitespace treated as empty",
			dataDir:  data,
			rulesDir: "  \t ",
			want:     filepath.Join(data, "rules"),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ResolveDir(tt.dataDir, tt.rulesDir); got != tt.want {
				t.Errorf("ResolveDir() = %q, want %q", got, tt.want)
			}
		})
	}
}
