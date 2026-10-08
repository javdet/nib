package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/llm"
)

const (
	RunInvestigationToolName = "run_investigation"
	ReportFindingToolName    = "report_finding"
)

// RunInvestigationToolDef returns the incident orchestrator's tool for
// launching Diggers. categories is the enum a system's categories are chosen
// from, the same list set_category offers.
func RunInvestigationToolDef(categories []string) llm.ToolDef {
	categoryItems := map[string]any{"type": "string"}
	if len(categories) > 0 {
		categoryItems["enum"] = categories
	}
	params := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"brief": map[string]any{
				"type": "string",
				"description": "Self-contained description of the incident for agents that cannot read this chat: " +
					"the symptom, the alert and its labels verbatim, what is known so far and what is not.",
			},
			"window_start": map[string]any{
				"type":        "string",
				"description": "Start of the window to investigate, RFC 3339 UTC. Set it before the problem began.",
			},
			"window_end": map[string]any{
				"type":        "string",
				"description": "End of the window, RFC 3339 UTC. Omit for an ongoing incident.",
			},
			"systems": map[string]any{
				"type":        "array",
				"description": fmt.Sprintf("The systems to investigate, one Digger each, in parallel. 1 to %d.", maxInvestigatedSystems),
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":        map[string]any{"type": "string", "description": "The system, as the knowledge base names it."},
						"environment": map[string]any{"type": "string"},
						"categories": map[string]any{
							"type":        "array",
							"description": "Tool categories the Digger needs for this system: metrics, logs, cluster access and so on.",
							"items":       categoryItems,
						},
						"focus": map[string]any{"type": "string", "description": "What to look for in this system in particular."},
					},
					"required": []string{"name"},
				},
			},
			"impact_only": map[string]any{
				"type":        "boolean",
				"description": "Reassess only the impact, over the findings already made. `systems` then only supplies categories.",
			},
		},
		"required": []string{"brief"},
	}
	raw, _ := json.Marshal(params)
	return llm.ToolDef{
		Name: RunInvestigationToolName,
		Description: "Investigate the incident in parallel: one Digger sub-agent per system examines its state before and during the incident " +
			"and reports a finding, then a synthesis agent writes the hypothesis, root causes, timeline and corrective actions and an impact agent the impact. " +
			"Everything lands in the post-mortem. Returns at once with `started`; the outcome is posted into this chat when the run ends. " +
			"Refused while a run is going.",
		Parameters: raw,
	}
}

type runInvestigationArgs struct {
	Brief       string           `json:"brief"`
	WindowStart string           `json:"window_start"`
	WindowEnd   string           `json:"window_end"`
	Systems     []IncidentSystem `json:"systems"`
	ImpactOnly  any              `json:"impact_only"`
}

func (s *ChatService) runInvestigationHandler(rootID uuid.UUID) localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		raw, err := json.Marshal(args)
		if err != nil {
			return "", err
		}
		var in runInvestigationArgs
		if err := json.Unmarshal(raw, &in); err != nil {
			return "Arguments do not match the schema: " + err.Error(), nil
		}
		if strings.TrimSpace(in.Brief) == "" {
			return "brief is required: the Diggers cannot read this chat.", nil
		}

		run, err := s.StartInvestigation(ctx, rootID, InvestigationRequest{
			Brief:       in.Brief,
			WindowStart: in.WindowStart,
			WindowEnd:   in.WindowEnd,
			Systems:     in.Systems,
			ImpactOnly:  argBool(in.ImpactOnly),
		})
		switch {
		case errors.Is(err, ErrInvestigationInProgress), errors.Is(err, ErrIncidentClosed), errors.Is(err, ErrInvalidInvestigation):
			return marshalToolResult(map[string]any{"status": "refused", "reason": err.Error()})
		case err != nil:
			return "", err
		}

		var names []string
		for _, st := range run.Steps {
			if st.Kind == IncidentStepDigger {
				names = append(names, st.System)
			}
		}
		next := "The Diggers run in the background; their findings, the synthesis and the impact land in the post-mortem and the outcome is posted here when the run ends. " +
			"Tell the operator briefly what is being investigated and end your turn — do not wait, poll or launch it again."
		return marshalToolResult(map[string]any{
			"status":  "started",
			"run_id":  run.RunID,
			"systems": names,
			"next":    next,
		})
	}
}

func marshalToolResult(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

var reportFindingParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "status": {"type": "string", "enum": ["root_cause_candidate", "degraded", "healthy", "unknown"],
      "description": "root_cause_candidate: this system's change or failure plausibly caused the incident. degraded: affected, but as a symptom. healthy: nothing wrong in the window. unknown: you could not tell."},
    "summary": {"type": "string", "description": "What you established, in Markdown, citing the evidence: which tool showed what."},
    "before": {"type": "string", "description": "The system's state in the baseline window."},
    "during": {"type": "string", "description": "Its state during the incident."},
    "changes": {"type": "array", "items": {"type": "string"}, "description": "Changes in or just before the window: deploys, config, traffic, infrastructure, security, provider. Each with its time."},
    "suspected_cause": {"type": "string"},
    "confidence": {"type": "string", "enum": ["low", "medium", "high"]},
    "assumptions": {"type": "string", "description": "What you assumed rather than verified."},
    "timeline": {"type": "array", "items": {"type": "object", "properties": {
      "at": {"type": "string", "description": "RFC 3339 UTC."}, "event": {"type": "string"}
    }, "required": ["at", "event"]}, "description": "Notable events this system shows, for the incident timeline."}
  },
  "required": ["status", "summary"]
}`)

// ReportFindingToolDef returns the Digger's tool for filing what it found.
func ReportFindingToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name: ReportFindingToolName,
		Description: "File your finding about the system you were given. Call it once, at the end; calling it again replaces the finding. " +
			"It is what the synthesis reads -- anything not in it is lost.",
		Parameters: reportFindingParameters,
	}
}

type reportFindingArgs struct {
	Status         string            `json:"status"`
	Summary        string            `json:"summary"`
	Before         string            `json:"before"`
	During         string            `json:"during"`
	Changes        []string          `json:"changes"`
	SuspectedCause string            `json:"suspected_cause"`
	Confidence     string            `json:"confidence"`
	Assumptions    string            `json:"assumptions"`
	Timeline       []PostmortemEvent `json:"timeline"`
}

var findingStatuses = map[string]bool{"root_cause_candidate": true, "degraded": true, "healthy": true, "unknown": true}

func (s *ChatService) reportFindingHandler(rootID, dialogID uuid.UUID, system string) localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		raw, err := json.Marshal(args)
		if err != nil {
			return "", err
		}
		var in reportFindingArgs
		if err := json.Unmarshal(raw, &in); err != nil {
			return "Arguments do not match the schema: " + err.Error(), nil
		}
		status := strings.ToLower(strings.TrimSpace(in.Status))
		if !findingStatuses[status] {
			return "status must be root_cause_candidate, degraded, healthy or unknown", nil
		}
		if strings.TrimSpace(in.Summary) == "" {
			return "summary is required", nil
		}

		doc, err := s.updatePostmortem(rootID, func(doc *Postmortem) error {
			environment := ""
			if run, ok, _ := s.ReadIncidentRun(rootID); ok {
				for _, st := range run.Steps {
					if st.DialogID == dialogID.String() {
						environment = st.Environment
					}
				}
			}
			upsertFinding(doc, PostmortemFinding{
				System:         system,
				Environment:    environment,
				Status:         status,
				Summary:        strings.TrimSpace(in.Summary),
				Before:         strings.TrimSpace(in.Before),
				During:         strings.TrimSpace(in.During),
				Changes:        uniqueNonEmpty(in.Changes),
				SuspectedCause: strings.TrimSpace(in.SuspectedCause),
				Confidence:     strings.ToLower(strings.TrimSpace(in.Confidence)),
				Assumptions:    strings.TrimSpace(in.Assumptions),
				DialogID:       dialogID.String(),
				UpdatedAt:      time.Now().Unix(),
			})
			appendTimeline(doc, in.Timeline, system)
			return nil
		})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Finding for %s filed (post-mortem version %d). Your investigation is complete; end your turn.", system, doc.Version), nil
	}
}

// appendTimeline adds a Digger's events to the timeline. It only adds: the
// timeline is shared by every Digger, and a replace would take a sibling's
// events with it. Merging and deduplicating them is the synthesis agent's job.
func appendTimeline(doc *Postmortem, events []PostmortemEvent, source string) {
	existing := make(map[string]struct{}, len(doc.Timeline))
	for i := range doc.Timeline {
		existing[timelineList.key(&doc.Timeline[i])] = struct{}{}
	}
	for _, ev := range events {
		item := PostmortemEvent{At: normalizeIncidentTime(ev.At), Event: strings.TrimSpace(ev.Event), Source: source}
		k := timelineList.key(&item)
		if item.Event == "" {
			continue
		}
		if _, dup := existing[k]; dup {
			continue
		}
		existing[k] = struct{}{}
		item.ID = doc.newItemID(timelineList.prefix)
		doc.Timeline = append(doc.Timeline, item)
	}
	sortTimeline(doc.Timeline)
}

// registerRunInvestigationTool is the incident orchestrator's alone: the root
// turn of an incident, never a sub-agent's transcript, which is also what keeps
// a Digger from launching Diggers.
func registerRunInvestigationTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.mode != incidentDialogMode || b.incidentRole != incidentRoleOrchestrator {
		return
	}
	if b.planID == uuid.Nil || b.planID != b.dialogID || s.dialogRepo == nil {
		return
	}
	if !localToolAllowed(allow, RunInvestigationToolName) {
		return
	}
	catalog.localHandlers[RunInvestigationToolName] = s.runInvestigationHandler(b.planID)
	catalog.tools = append(catalog.tools, RunInvestigationToolDef(b.categoryNames))
}

// registerReportFindingTool exists only for a Digger, whose binding names the
// system the finding is filed under.
func registerReportFindingTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.mode != incidentDialogMode || b.system == "" || b.planID == uuid.Nil {
		return
	}
	if !localToolAllowed(allow, ReportFindingToolName) {
		return
	}
	catalog.localHandlers[ReportFindingToolName] = s.reportFindingHandler(b.planID, b.dialogID, b.system)
	catalog.tools = append(catalog.tools, ReportFindingToolDef())
}
