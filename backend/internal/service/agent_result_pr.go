package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/javdet/nib/internal/domain"
	"github.com/google/uuid"
)

// attachActionPullRequest records the PR opened by an action run on the action
// itself, so the plan shows it without opening the execute chat.
func (s *ChatService) attachActionPullRequest(ctx context.Context, exec domain.Dialog, prURL string) error {
	prURL = strings.TrimSpace(prURL)
	if prURL == "" || exec.ParentID == nil {
		return nil
	}

	planID := *exec.ParentID

	runs, err := s.ReadActionPlanRuns(planID)
	if err != nil {
		return fmt.Errorf("attach action pull request: read runs: %w", err)
	}

	key := findRunKeyByExecID(runs, exec.ID)
	if key == "" {
		return nil
	}

	raw, found, err := s.ReadActionPlan(planID)
	if err != nil {
		return fmt.Errorf("attach action pull request: read plan: %w", err)
	}
	if !found {
		return nil
	}

	var plan map[string]any
	if err := json.Unmarshal(raw, &plan); err != nil {
		return fmt.Errorf("attach action pull request: unmarshal plan: %w", err)
	}

	changed, err := setActionPlanStepPRURL(plan, key, prURL)
	if err != nil {
		return fmt.Errorf("attach action pull request: %w", err)
	}
	if !changed {
		return nil
	}

	planData, err := json.Marshal(plan)
	if err != nil {
		return fmt.Errorf("attach action pull request: marshal plan: %w", err)
	}
	if _, err := s.WriteActionPlan(planID, planData); err != nil {
		return fmt.Errorf("attach action pull request: write plan: %w", err)
	}

	s.activity.Publish(planID, domain.AgentActivity{Kind: domain.ActivityActionPlanUpdated})
	return nil
}

func findRunKeyByExecID(runs map[string]string, execID uuid.UUID) string {
	execStr := execID.String()
	for key, val := range runs {
		if val == execStr {
			return key
		}
	}
	return ""
}

func setActionPlanStepPRURL(plan map[string]any, key, prURL string) (bool, error) {
	step, ok := getActionPlanStepMap(plan, key)
	if !ok {
		return false, nil
	}

	existing, _ := step["pr_url"].(string)
	if strings.TrimSpace(existing) == prURL {
		return false, nil
	}

	step["pr_url"] = prURL
	return true, nil
}

func getActionPlanStepMap(plan map[string]any, key string) (map[string]any, bool) {
	if key == "" {
		return nil, false
	}

	if strings.HasPrefix(key, "rollback.") {
		idx := atoi(strings.TrimPrefix(key, "rollback."))
		rollback, ok := plan["rollback"].([]any)
		if !ok || idx < 0 || idx >= len(rollback) {
			return nil, false
		}
		step, ok := rollback[idx].(map[string]any)
		return step, ok
	}

	m := actionPlanKeyPattern.FindStringSubmatch(key)
	if m == nil || m[2] != "step" {
		return nil, false
	}

	stageIdx := atoi(m[1])
	stepIdx := atoi(m[3])

	stages, ok := plan["stages"].([]any)
	if !ok || stageIdx < 0 || stageIdx >= len(stages) {
		return nil, false
	}
	stage, ok := stages[stageIdx].(map[string]any)
	if !ok {
		return nil, false
	}
	steps, ok := stage["steps"].([]any)
	if !ok || stepIdx < 0 || stepIdx >= len(steps) {
		return nil, false
	}
	step, ok := steps[stepIdx].(map[string]any)
	return step, ok
}

// warnAttachActionPullRequest logs attach failures without failing the webhook.
func (s *ChatService) warnAttachActionPullRequest(ctx context.Context, exec domain.Dialog, prURL string) {
	if err := s.attachActionPullRequest(ctx, exec, prURL); err != nil {
		slog.Warn("attach action pull request",
			"exec_dialog_id", exec.ID,
			"plan_dialog_id", exec.ParentID,
			"error", err,
		)
	}
}
