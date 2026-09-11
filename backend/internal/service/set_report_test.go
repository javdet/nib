package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

func TestSetReportToolDef(t *testing.T) {
	t.Parallel()
	def := SetReportToolDef()
	if def.Name != SetReportToolName {
		t.Fatalf("name = %q, want %q", def.Name, SetReportToolName)
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(def.Parameters, &schema); err != nil {
		t.Fatalf("unmarshal parameters: %v", err)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "report" {
		t.Fatalf("required = %#v, want [report]", schema.Required)
	}
}

func TestSetReportHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	planID := uuid.New()
	dir := t.TempDir()

	svc := &ChatService{reportsDir: dir, activity: NewActivityBroker()}
	handler := svc.setReportHandler(planID)
	wantRel := "Report saved to " + filepath.Join("reports", planID.String()+".md")

	tests := []struct {
		name    string
		args    map[string]any
		wantOut string
		wantFil string
	}{
		{
			name:    "writes the report",
			args:    map[string]any{"report": "## Outcome\n\nEverything landed."},
			wantOut: wantRel,
			wantFil: "## Outcome\n\nEverything landed.",
		},
		{
			name:    "overwrites an earlier report",
			args:    map[string]any{"report": "## Outcome\n\nSecond pass."},
			wantOut: wantRel,
			wantFil: "## Outcome\n\nSecond pass.",
		},
		{
			name:    "missing report",
			args:    map[string]any{},
			wantOut: "report is required",
			wantFil: "## Outcome\n\nSecond pass.",
		},
		{
			name:    "blank report",
			args:    map[string]any{"report": "   \n "},
			wantOut: "report is required",
			wantFil: "## Outcome\n\nSecond pass.",
		},
	}

	// Sequential on purpose: each case asserts what the file holds after the ones
	// before it, which is how "a refused call changes nothing" is checked.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := handler(ctx, tt.args)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if out != tt.wantOut {
				t.Fatalf("out = %q, want %q", out, tt.wantOut)
			}
			data, err := os.ReadFile(filepath.Join(dir, planID.String()+".md"))
			if err != nil {
				t.Fatalf("read file: %v", err)
			}
			if string(data) != tt.wantFil {
				t.Fatalf("content = %q, want %q", string(data), tt.wantFil)
			}
		})
	}
}

func TestSetReportHandlerPublishesActivity(t *testing.T) {
	t.Parallel()
	planID := uuid.New()
	svc := &ChatService{reportsDir: t.TempDir(), activity: NewActivityBroker()}

	events, unsubscribe := svc.SubscribeActivity(planID)
	defer unsubscribe()

	if _, err := svc.setReportHandler(planID)(context.Background(), map[string]any{
		"report": "## Outcome\n\nDone.",
	}); err != nil {
		t.Fatalf("handler err = %v", err)
	}

	select {
	case ev := <-events:
		if ev.Kind != domain.ActivityReportUpdated {
			t.Fatalf("kind = %q, want %q", ev.Kind, domain.ActivityReportUpdated)
		}
	default:
		t.Fatal("expected a report_updated activity event")
	}
}

func TestReadReport(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := &ChatService{reportsDir: dir}

	t.Run("missing report is not an error", func(t *testing.T) {
		content, found, err := svc.ReadReport(uuid.New())
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if found {
			t.Fatalf("found = true, want false")
		}
		if content != "" {
			t.Fatalf("content = %q, want empty", content)
		}
	})

	t.Run("round-trips what was written", func(t *testing.T) {
		id := uuid.New()
		if err := svc.WriteReport(id, "## Outcome\n\nAll good."); err != nil {
			t.Fatalf("WriteReport err = %v", err)
		}
		content, found, err := svc.ReadReport(id)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if !found {
			t.Fatal("found = false, want true")
		}
		if content != "## Outcome\n\nAll good." {
			t.Fatalf("content = %q", content)
		}
	})
}

func TestAddLocalTools_includesSetReportForPlan(t *testing.T) {
	t.Parallel()
	svc := &ChatService{}
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}

	svc.addLocalTools(&catalog, map[string]struct{}{SetReportToolName: {}}, newToolBinding(uuid.New()))

	if _, ok := catalog.localHandlers[SetReportToolName]; !ok {
		t.Fatal("expected set_report handler")
	}
}

func TestAddLocalTools_excludesSetReportWithoutAllowList(t *testing.T) {
	t.Parallel()
	svc := &ChatService{}
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}

	svc.addLocalTools(&catalog, map[string]struct{}{"other_tool": {}}, newToolBinding(uuid.New()))

	if _, ok := catalog.localHandlers[SetReportToolName]; ok {
		t.Fatal("did not expect set_report handler when not in the allow list")
	}
}
