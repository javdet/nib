package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/atomicfile"
)

var (
	// ErrNoActionsSelected is returned when a plan is requested from an
	// incident without naming any of its corrective actions.
	ErrNoActionsSelected = errors.New("select at least one corrective action")
	// ErrUnknownPostmortemItem is returned when a request names an action the
	// post-mortem does not have, typically one deleted since the page loaded.
	ErrUnknownPostmortemItem = errors.New("the post-mortem has no such action")
	// ErrPostmortemActionPlanned is returned when an action is already in a plan.
	ErrPostmortemActionPlanned = errors.New("the action is already in a plan")
)

// IncidentPlan is the plan created from an incident's corrective actions.
type IncidentPlan struct {
	PlanID uuid.UUID `json:"planId"`
	// Message is the first message of the plan's chat. The web interface sends
	// it, so the orchestrator's first turn runs as any other turn does: under a
	// request, with the chat showing it is busy.
	Message string `json:"message"`
}

// PlanOrigin records the incident a plan was created from.
type PlanOrigin struct {
	IncidentID string   `json:"incidentId"`
	ActionIDs  []string `json:"actionIds"`
	CreatedAt  int64    `json:"createdAt"`
}

func (s *ChatService) planOriginPath(planID uuid.UUID) string {
	return filepath.Join(s.planOriginDir, planID.String()+".json")
}

// ReadPlanOrigin returns the incident a plan came from, or false for a plan
// created any other way.
func (s *ChatService) ReadPlanOrigin(planID uuid.UUID) (PlanOrigin, bool, error) {
	b, err := os.ReadFile(s.planOriginPath(planID))
	if errors.Is(err, os.ErrNotExist) {
		return PlanOrigin{}, false, nil
	}
	if err != nil {
		return PlanOrigin{}, false, fmt.Errorf("read plan origin: %w", err)
	}
	var origin PlanOrigin
	if err := json.Unmarshal(b, &origin); err != nil {
		return PlanOrigin{}, false, fmt.Errorf("decode plan origin: %w", err)
	}
	return origin, true, nil
}

func (s *ChatService) writePlanOrigin(planID uuid.UUID, origin PlanOrigin) error {
	b, err := json.MarshalIndent(origin, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.planOriginDir, 0o755); err != nil {
		return fmt.Errorf("create plan origin directory: %w", err)
	}
	return atomicfile.Write(s.planOriginPath(planID), b)
}

// CreatePlanFromIncident hands the chosen corrective actions to the change
// management track: one new plan carrying all of them, linked both ways.
//
// It runs inside the post-mortem's write lock, so a second press of the button
// finds the actions already in_plan and is refused rather than creating a
// second plan for them.
func (s *ChatService) CreatePlanFromIncident(ctx context.Context, incidentID uuid.UUID, actionIDs []string) (IncidentPlan, error) {
	incident, err := s.resolveIncidentRoot(ctx, incidentID)
	if err != nil {
		return IncidentPlan{}, err
	}
	ids := uniqueNonEmpty(actionIDs)
	if len(ids) == 0 {
		return IncidentPlan{}, ErrNoActionsSelected
	}

	var plan IncidentPlan
	_, err = s.updatePostmortem(incidentID, func(doc *Postmortem) error {
		var shortTerm, longTerm []PostmortemAction
		for _, id := range ids {
			action := doc.findAction(id)
			if action == nil {
				return fmt.Errorf("%w: %s", ErrUnknownPostmortemItem, id)
			}
			if action.Status == PostmortemActionInPlan || action.PlanID != "" {
				return fmt.Errorf("%w: %s", ErrPostmortemActionPlanned, action.Text)
			}
			if slices.ContainsFunc(doc.LongTerm, func(a PostmortemAction) bool { return a.ID == id }) {
				longTerm = append(longTerm, *action)
			} else {
				shortTerm = append(shortTerm, *action)
			}
		}

		title := strings.TrimSpace(incident.Title)
		planTitle := "Incident remediation"
		if title != "" {
			planTitle = "Remediate: " + title
		}
		created, err := s.dialogRepo.CreateDialog(ctx, mainDialogMode, planTitle, nil)
		if err != nil {
			return fmt.Errorf("create plan: %w", err)
		}

		// The summary is what the plan page shows before its first turn has
		// run; decomposition replaces it with its own.
		if err := s.WriteSummary(created.ID, incidentPlanSummary(title, shortTerm, longTerm)); err != nil {
			return fmt.Errorf("write plan summary: %w", err)
		}
		now := time.Now().Unix()
		if err := s.writePlanOrigin(created.ID, PlanOrigin{IncidentID: incidentID.String(), ActionIDs: ids, CreatedAt: now}); err != nil {
			return fmt.Errorf("write plan origin: %w", err)
		}

		for _, id := range ids {
			action := doc.findAction(id)
			action.Status = PostmortemActionInPlan
			action.PlanID = created.ID.String()
		}
		doc.LinkedPlans = append(doc.LinkedPlans, PostmortemLinkedPlan{
			PlanID: created.ID.String(), ActionIDs: ids, CreatedAt: now,
		})

		plan = IncidentPlan{PlanID: created.ID, Message: incidentPlanMessage(title, incidentID, doc, shortTerm, longTerm)}
		return nil
	})
	if err != nil {
		return IncidentPlan{}, err
	}
	return plan, nil
}

func uniqueNonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

func incidentPlanSummary(title string, shortTerm, longTerm []PostmortemAction) string {
	var b strings.Builder
	if title != "" {
		fmt.Fprintf(&b, "Corrective actions from incident «%s».", title)
	} else {
		b.WriteString("Corrective actions from an incident.")
	}
	writeActions(&b, "Immediate", shortTerm)
	writeActions(&b, "Long-term", longTerm)
	return b.String()
}

func writeActions(b *strings.Builder, heading string, actions []PostmortemAction) {
	if len(actions) == 0 {
		return
	}
	fmt.Fprintf(b, "\n\n**%s:**", heading)
	for _, a := range actions {
		fmt.Fprintf(b, "\n- %s", a.Text)
	}
}

// incidentPlanMessage is the request the orchestrator receives. It carries the
// incident context decomposition needs, because no agent of the plan can read
// the incident's own chat.
func incidentPlanMessage(title string, incidentID uuid.UUID, doc *Postmortem, shortTerm, longTerm []PostmortemAction) string {
	var b strings.Builder
	name := title
	if name == "" {
		name = incidentID.String()
	}
	fmt.Fprintf(&b, "Plan the corrective actions from incident «%s».\n", name)

	b.WriteString("\n## Incident\n")
	if doc.Summary != "" {
		b.WriteString(doc.Summary + "\n")
	}
	var facts []string
	if doc.Environment != "" {
		facts = append(facts, "Environment: "+doc.Environment)
	}
	if doc.Severity != "" {
		facts = append(facts, "Severity: "+doc.Severity)
	}
	if doc.StartedAt != "" {
		facts = append(facts, "Started: "+doc.StartedAt)
	}
	if len(facts) > 0 {
		b.WriteString("\n" + strings.Join(facts, " · ") + "\n")
	}
	if len(doc.RootCauses) > 0 {
		b.WriteString("\n## Root cause\n")
		for _, rc := range doc.RootCauses {
			fmt.Fprintf(&b, "- %s\n", rc.Text)
		}
	}
	if len(doc.Impacted) > 0 {
		b.WriteString("\n## Impacted\n")
		for _, im := range doc.Impacted {
			line := im.Name
			if im.Environment != "" {
				line += " (" + im.Environment + ")"
			}
			if im.Description != "" {
				line += ": " + im.Description
			}
			fmt.Fprintf(&b, "- %s\n", line)
		}
	}

	b.WriteString("\n## Actions to plan\n")
	for _, group := range []struct {
		heading string
		actions []PostmortemAction
	}{{"Immediate (here and now)", shortTerm}, {"Long-term", longTerm}} {
		if len(group.actions) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n### %s\n", group.heading)
		for i, a := range group.actions {
			fmt.Fprintf(&b, "%d. %s\n", i+1, a.Text)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// PlanOriginIncident is what the plan page shows about where a plan came from.
type PlanOriginIncident struct {
	PlanOrigin
	IncidentTitle string `json:"incidentTitle"`
}

// PlanOriginForDialog serves a plan's origin with the incident's current title.
// The bool is false for a plan that did not come from an incident.
func (s *ChatService) PlanOriginForDialog(ctx context.Context, planID uuid.UUID) (PlanOriginIncident, bool, error) {
	origin, ok, err := s.ReadPlanOrigin(planID)
	if err != nil || !ok {
		return PlanOriginIncident{}, ok, err
	}
	out := PlanOriginIncident{PlanOrigin: origin}
	if id, err := uuid.Parse(origin.IncidentID); err == nil && s.dialogRepo != nil {
		// A deleted incident leaves the plan's origin behind; the page then
		// shows the link without a title rather than failing the plan.
		if d, err := s.dialogRepo.GetDialog(ctx, id); err == nil {
			out.IncidentTitle = d.Title
		}
	}
	return out, true, nil
}
