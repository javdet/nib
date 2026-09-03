package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

func TestGetDAGToolDef(t *testing.T) {
	t.Parallel()
	def := GetDAGToolDef()
	if def.Name != GetDAGToolName {
		t.Fatalf("name = %q, want %q", def.Name, GetDAGToolName)
	}
	var schema struct {
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(def.Parameters, &schema); err != nil {
		t.Fatalf("unmarshal parameters: %v", err)
	}
	if len(schema.Properties) != 0 {
		t.Fatalf("properties = %#v, want none", schema.Properties)
	}
}

func TestDAGStagesOf(t *testing.T) {
	t.Parallel()
	stages := dagStagesOf("```mermaid\nflowchart TD\n  a[Prepare cluster] --> b[Deploy Centrifugo]\n  b --> c[Wire monitoring]\n```")
	want := []dagStage{
		{Number: 1, Title: "Prepare cluster"},
		{Number: 2, Title: "Deploy Centrifugo"},
		{Number: 3, Title: "Wire monitoring"},
	}
	if len(stages) != len(want) {
		t.Fatalf("stages = %#v, want %#v", stages, want)
	}
	for i, s := range stages {
		if s != want[i] {
			t.Fatalf("stage %d = %#v, want %#v", i, s, want[i])
		}
	}
}

func TestGetDAGHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// The DAG belongs to the plan root, so a sub-agent's own dialog id has to
	// resolve up to it before anything is read.
	planID := uuid.New()
	childID := uuid.New()
	dagsDir := t.TempDir()
	summariesDir := t.TempDir()

	svc := &ChatService{
		dagsDir:      dagsDir,
		summariesDir: summariesDir,
		dialogRepo: &actionListDialogRepo{
			dialogs: map[uuid.UUID]domain.Dialog{
				planID:  {ID: planID, Mode: "main"},
				childID: {ID: childID, ParentID: &planID, Mode: "decompose"},
			},
		},
	}

	t.Run("no dag", func(t *testing.T) {
		out, err := svc.getDAGHandler(childID)(ctx, nil)
		if err != nil {
			t.Fatalf("handler: %v", err)
		}
		if out != "no DAG found for this dialog" {
			t.Fatalf("out = %q", out)
		}
	})

	mermaid := "```mermaid\nflowchart TD\n  prep[Prepare cluster] --> deploy[Deploy Centrifugo]\n```"
	if err := os.WriteFile(filepath.Join(dagsDir, planID.String()+".md"), []byte(mermaid), 0o644); err != nil {
		t.Fatalf("write dag: %v", err)
	}

	t.Run("dag without a summary", func(t *testing.T) {
		out, err := svc.getDAGHandler(childID)(ctx, nil)
		if err != nil {
			t.Fatalf("handler: %v", err)
		}
		var resp dagResponse
		if err := json.Unmarshal([]byte(out), &resp); err != nil {
			t.Fatalf("unmarshal response: %v\nout=%s", err, out)
		}
		if resp.Summary != "" {
			t.Fatalf("summary = %q, want empty", resp.Summary)
		}
		if len(resp.Stages) != 2 || resp.Stages[1].Number != 2 || resp.Stages[1].Title != "Deploy Centrifugo" {
			t.Fatalf("stages = %#v", resp.Stages)
		}
		if !strings.Contains(resp.Mermaid, "prep[Prepare cluster] --> deploy[Deploy Centrifugo]") {
			t.Fatalf("mermaid = %q", resp.Mermaid)
		}
	})

	t.Run("dag with a summary", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(summariesDir, planID.String()+".txt"), []byte("Roll Centrifugo out to staging.\n"), 0o644); err != nil {
			t.Fatalf("write summary: %v", err)
		}
		out, err := svc.getDAGHandler(planID)(ctx, nil)
		if err != nil {
			t.Fatalf("handler: %v", err)
		}
		var resp dagResponse
		if err := json.Unmarshal([]byte(out), &resp); err != nil {
			t.Fatalf("unmarshal response: %v\nout=%s", err, out)
		}
		if resp.Summary != "Roll Centrifugo out to staging." {
			t.Fatalf("summary = %q", resp.Summary)
		}
	})
}
