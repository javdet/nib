package handler

import (
	"strings"
	"testing"
)

func TestPlanBundleFileName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		title string
		want  string
	}{
		{title: "Upgrade Postgres to 17", want: "upgrade-postgres-to-17.nib"},
		{title: `  "quoted"; name/with\slashes `, want: "quoted-name-with-slashes.nib"},
		{title: "Обновить кластер", want: "plan.nib"},
		{title: "", want: "plan.nib"},
		{title: strings.Repeat("a", 100), want: strings.Repeat("a", 80) + ".nib"},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			t.Parallel()
			if got := planBundleFileName(tt.title); got != tt.want {
				t.Fatalf("planBundleFileName(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}
