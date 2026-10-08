package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/llm"
)

const (
	GetPostmortemToolName    = "get_postmortem"
	UpdatePostmortemToolName = "update_postmortem"
)

var getPostmortemParameters = json.RawMessage(`{"type": "object", "properties": {}}`)

// GetPostmortemToolDef returns the LLM tool definition for reading an incident's
// post-mortem.
func GetPostmortemToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name: GetPostmortemToolName,
		Description: "Read the incident's post-mortem as it stands: status, times, summary, hypothesis, " +
			"root causes, impacted systems, timeline, corrective actions, per-system findings and linked plans. " +
			"`locks` names the fields the operator owns; leave them alone and propose changes in chat.",
		Parameters: getPostmortemParameters,
	}
}

var updatePostmortemParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "set": {
      "type": "object",
      "description": "Fields to overwrite. Omit a field to leave it as it is; an empty string clears it.",
      "properties": {
        "summary": {"type": "string", "description": "Brief Markdown description of the incident: what broke, where, since when, for whom."},
        "hypothesis": {"type": "string", "description": "The current working hypothesis, in Markdown."},
        "status": {"type": "string", "enum": ["ongoing", "mitigated", "resolved"]},
        "severity": {"type": "string", "enum": ["sev1", "sev2", "sev3", "sev4"]},
        "environment": {"type": "string"},
        "started_at": {"type": "string", "description": "When the problem began, RFC 3339 UTC."},
        "detected_at": {"type": "string", "description": "When it was noticed or alerted on, RFC 3339 UTC."},
        "mitigated_at": {"type": "string", "description": "RFC 3339 UTC."},
        "resolved_at": {"type": "string", "description": "RFC 3339 UTC."},
        "impact": {"type": "string", "description": "Impact report in Markdown."},
        "closure": {"type": "string", "description": "Closure summary in Markdown."}
      }
    },
    "lists": {
      "type": "object",
      "description": "Lists to restate in full. A list you send replaces every item an agent wrote before -- read get_postmortem and restate each item you want to keep. Items the operator wrote, and actions already in a plan, are kept whatever you send. Omit a list to leave it alone.",
      "properties": {
        "root_causes": {"type": "array", "items": {"type": "object", "properties": {"text": {"type": "string"}}, "required": ["text"]}},
        "impacted": {"type": "array", "items": {"type": "object", "properties": {
          "name": {"type": "string"}, "environment": {"type": "string"},
          "description": {"type": "string"}, "severity": {"type": "string"}
        }, "required": ["name"]}},
        "timeline": {"type": "array", "items": {"type": "object", "properties": {
          "at": {"type": "string", "description": "RFC 3339 UTC."},
          "event": {"type": "string"}, "source": {"type": "string", "description": "Where the fact comes from: a system, a tool, the alert."}
        }, "required": ["at", "event"]}},
        "short_term": {"type": "array", "description": "Immediate, here-and-now changes.", "items": {"type": "object", "properties": {
          "text": {"type": "string"}, "status": {"type": "string", "enum": ["todo", "done", "dropped"]}
        }, "required": ["text"]}},
        "long_term": {"type": "array", "description": "Lasting fixes.", "items": {"type": "object", "properties": {
          "text": {"type": "string"}, "status": {"type": "string", "enum": ["todo", "done", "dropped"]}
        }, "required": ["text"]}}
      }
    }
  }
}`)

// UpdatePostmortemToolDef returns the LLM tool definition for writing an
// incident's post-mortem. The schema is the same for every role; which fields a
// role may write is enforced by the handler, which names what it refused.
func UpdatePostmortemToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name: UpdatePostmortemToolName,
		Description: "Update the incident's post-mortem, which the operator watches fill in live. " +
			"Call it whenever you establish something material rather than once at the end. " +
			"Fields the operator owns, or that are not yours to write, are refused and named in the result; the rest is applied.",
		Parameters: updatePostmortemParameters,
	}
}

// postmortemToolFields maps the tool's snake_case names onto the document's.
var postmortemToolFields = map[string]string{
	"summary":      pmFieldSummary,
	"hypothesis":   pmFieldHypothesis,
	"status":       pmFieldStatus,
	"severity":     pmFieldSeverity,
	"environment":  pmFieldEnvironment,
	"started_at":   pmFieldStartedAt,
	"detected_at":  pmFieldDetectedAt,
	"mitigated_at": pmFieldMitigatedAt,
	"resolved_at":  pmFieldResolvedAt,
	"impact":       pmFieldImpact,
	"closure":      pmFieldClosure,
}

// updatePostmortemArgs mirrors the tool schema. Lists are pointers so a list
// the model left out is told apart from one it sent empty.
type updatePostmortemArgs struct {
	Set   map[string]any `json:"set"`
	Lists struct {
		RootCauses *[]PostmortemItem `json:"root_causes"`
		Impacted   *[]struct {
			Name        string `json:"name"`
			Environment string `json:"environment"`
			Description string `json:"description"`
			Severity    string `json:"severity"`
		} `json:"impacted"`
		Timeline  *[]PostmortemEvent  `json:"timeline"`
		ShortTerm *[]PostmortemAction `json:"short_term"`
		LongTerm  *[]PostmortemAction `json:"long_term"`
	} `json:"lists"`
}

// parseUpdatePostmortemArgs turns the tool's arguments into a patch. An
// unknown field is reported rather than dropped, so the model learns the name.
func parseUpdatePostmortemArgs(args map[string]any) (PostmortemPatch, []string, error) {
	raw, err := json.Marshal(args)
	if err != nil {
		return PostmortemPatch{}, nil, err
	}
	var in updatePostmortemArgs
	if err := json.Unmarshal(raw, &in); err != nil {
		return PostmortemPatch{}, nil, fmt.Errorf("arguments do not match the schema: %w", err)
	}

	var patch PostmortemPatch
	var unknown []string
	for name, v := range in.Set {
		if v == nil {
			continue
		}
		field, ok := postmortemToolFields[name]
		if !ok {
			unknown = append(unknown, name+" is not a field of the post-mortem")
			continue
		}
		if patch.Set == nil {
			patch.Set = make(map[string]string)
		}
		patch.Set[field] = argString(v)
	}

	l := in.Lists
	if l.RootCauses != nil || l.Impacted != nil || l.Timeline != nil || l.ShortTerm != nil || l.LongTerm != nil {
		lists := &PostmortemLists{}
		if l.RootCauses != nil {
			lists.RootCauses = append([]PostmortemItem{}, *l.RootCauses...)
		}
		if l.Impacted != nil {
			lists.Impacted = []PostmortemImpacted{}
			for _, it := range *l.Impacted {
				lists.Impacted = append(lists.Impacted, PostmortemImpacted{
					Name: it.Name, Environment: it.Environment, Description: it.Description, Severity: it.Severity,
				})
			}
		}
		if l.Timeline != nil {
			lists.Timeline = append([]PostmortemEvent{}, *l.Timeline...)
		}
		if l.ShortTerm != nil {
			lists.ShortTerm = append([]PostmortemAction{}, *l.ShortTerm...)
		}
		if l.LongTerm != nil {
			lists.LongTerm = append([]PostmortemAction{}, *l.LongTerm...)
		}
		patch.Lists = lists
	}
	return patch, unknown, nil
}

func (s *ChatService) getPostmortemHandler(rootID uuid.UUID) localToolHandler {
	return func(ctx context.Context, _ map[string]any) (string, error) {
		doc, err := s.ReadPostmortem(rootID)
		if err != nil {
			return "", err
		}
		b, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
}

func (s *ChatService) updatePostmortemHandler(rootID uuid.UUID, role incidentRole) localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		patch, refusals, err := parseUpdatePostmortemArgs(args)
		if err != nil {
			return err.Error(), nil
		}
		if patch.Set == nil && patch.Lists == nil {
			if len(refusals) > 0 {
				return "Nothing applied:\n- " + strings.Join(refusals, "\n- "), nil
			}
			return "Nothing to update: send `set`, `lists` or both.", nil
		}

		doc, err := s.updatePostmortem(rootID, func(doc *Postmortem) error {
			r, err := applyPostmortemPatch(doc, patch, role)
			refusals = append(refusals, r...)
			return err
		})
		if err != nil {
			return "", err
		}

		out := fmt.Sprintf("Post-mortem updated (version %d).", doc.Version)
		if len(refusals) > 0 {
			out += "\nNot applied:\n- " + strings.Join(refusals, "\n- ")
		}
		return out, nil
	}
}

// postmortemWriterRole reports whether role may call update_postmortem. A
// Digger writes through report_finding instead, and a follow-up typed into an
// agent's transcript carries no role at all.
func postmortemWriterRole(role incidentRole) bool {
	switch role {
	case incidentRoleOrchestrator, incidentRoleSynthesis, incidentRoleImpact, incidentRoleClosure:
		return true
	}
	return false
}

// registerGetPostmortemTool and registerUpdatePostmortemTool are the incident
// mode's alone, like write_rule is discuss's: the binding is the first guard and
// enforceModeToolLimits the second. Both bind to the incident, not the
// transcript, because the post-mortem belongs to the root of the lineage.
func registerGetPostmortemTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.mode != incidentDialogMode || b.planID == uuid.Nil || !localToolAllowed(allow, GetPostmortemToolName) {
		return
	}
	catalog.localHandlers[GetPostmortemToolName] = s.getPostmortemHandler(b.planID)
	catalog.tools = append(catalog.tools, GetPostmortemToolDef())
}

func registerUpdatePostmortemTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.mode != incidentDialogMode || b.planID == uuid.Nil || !postmortemWriterRole(b.incidentRole) {
		return
	}
	if !localToolAllowed(allow, UpdatePostmortemToolName) {
		return
	}
	catalog.localHandlers[UpdatePostmortemToolName] = s.updatePostmortemHandler(b.planID, b.incidentRole)
	catalog.tools = append(catalog.tools, UpdatePostmortemToolDef())
}
