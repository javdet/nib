package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/atomicfile"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/llm"
)

const SetReportToolName = "set_report"

var setReportParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "report": {
      "type": "string",
      "description": "The finished report in Markdown, with the sections your instructions name. It is stored as written and rendered in the web interface."
    }
  },
  "required": ["report"]
}`)

// SetReportToolDef returns the LLM tool definition for persisting a plan's
// closing report.
//
// It is deliberately absent from every mode allow list: the only agent that may
// call it is the one StartReportAgent launches, which is handed a catalog of two
// tools rather than a mode's. See reportAgentAllowSet.
func SetReportToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name: SetReportToolName,
		Description: "Save the closing report of a finished plan. " +
			"Call it once, with the whole report as Markdown; the text is stored and rendered in the web interface. " +
			"Finishing without calling it produces nothing.",
		Parameters: setReportParameters,
	}
}

// WriteReport persists a plan report for the given dialog.
func (s *ChatService) WriteReport(dialogID uuid.UUID, content string) error {
	if err := os.MkdirAll(s.reportsDir, 0o755); err != nil {
		return fmt.Errorf("create reports directory: %w", err)
	}

	dest := filepath.Join(s.reportsDir, dialogID.String()+".md")
	return atomicfile.WriteString(dest, content)
}

// ReadReport returns the stored plan report for a dialog, or false if none exists.
func (s *ChatService) ReadReport(dialogID uuid.UUID) (string, bool, error) {
	path := filepath.Join(s.reportsDir, dialogID.String()+".md")
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return string(b), true, nil
}

// setReportHandler returns a local tool handler bound to a specific plan.
//
// Unlike create_summary it publishes an activity event: the report is written by
// a subagent in a dialog of its own, long after the operator pressed Finish, so
// without one the plan page would only show it on a full reload.
func (s *ChatService) setReportHandler(planID uuid.UUID) localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		raw := strings.TrimSpace(argString(args["report"]))
		if raw == "" {
			return "report is required", nil
		}

		if err := s.WriteReport(planID, raw); err != nil {
			return "", err
		}
		s.activity.Publish(planID, domain.AgentActivity{Kind: domain.ActivityReportUpdated})

		return "Report saved to " + filepath.Join("reports", planID.String()+".md"), nil
	}
}
