package toolcatalog

import (
	"testing"
)

func TestMatchesPattern(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pattern string
		tool    string
		want    bool
	}{
		{"kubernetes_get_pods", "kubernetes_get_pods", true},
		{"kubernetes_get_pods", "Kubernetes_Get_Pods", true},
		{"kubernetes_*", "kubernetes_get_pods", true},
		{"kubernetes_*", "kubernetes_list_nodes", true},
		{"kubernetes_*", "grafana_query", false},
		{"*", "anything", true},
		{"exact", "exactly", false},
		{"", "tool", false},
		{"tool", "", false},
	}

	for _, tt := range tests {
		got := MatchesPattern(tt.pattern, tt.tool)
		if got != tt.want {
			t.Errorf("MatchesPattern(%q, %q) = %v, want %v", tt.pattern, tt.tool, got, tt.want)
		}
	}
}
