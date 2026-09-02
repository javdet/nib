package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/repository"
	"github.com/javdet/nib/internal/rules"
)

// sharedSeedSections composes the context every fan-out subagent gets, whichever
// part of the plan it owns, and hands back the contract so the caller can render
// the slice of it that is its own.
func (s *ChatService) sharedSeedSections(ctx context.Context, decomposeID uuid.UUID) ([]string, PlanContract, error) {
	var parts []string

	summary, found, err := s.ReadSummary(decomposeID)
	if err != nil {
		return nil, PlanContract{}, fmt.Errorf("read summary: %w", err)
	}
	if found && strings.TrimSpace(summary) != "" {
		parts = append(parts, "## Summary\n\n"+strings.TrimSpace(summary))
	}

	dag, found, err := s.ReadDAG(decomposeID)
	if err != nil {
		return nil, PlanContract{}, fmt.Errorf("read dag: %w", err)
	}
	if found {
		parts = append(parts, "## DAG\n\n"+strings.TrimSpace(dag))
	}

	rulesSection, err := s.matchedRulesSection(ctx, decomposeID)
	if err != nil {
		return nil, PlanContract{}, err
	}
	if rulesSection != "" {
		parts = append(parts, "## Rules\n\n"+rulesSection)
	}

	contract, _, err := s.ReadPlanContract(decomposeID)
	if err != nil {
		slog.Warn("fanout seed: read contract", "dialog_id", decomposeID, "error", err)
	}
	return parts, contract, nil
}

// buildStageSeed composes the first user message of a stage subagent: the shared
// context every stage gets, the stage it owns, and -- when the contract said this
// stage could not be planned without them -- the upstream stages already written.
func (s *ChatService) buildStageSeed(ctx context.Context, decomposeID uuid.UUID, title string) (string, error) {
	parts, contract, err := s.sharedSeedSections(ctx, decomposeID)
	if err != nil {
		return "", err
	}
	if section := stageContractSection(contract, title); section != "" {
		parts = append(parts, section)
	}

	parts = append(parts, "## Your stage\n\n"+title)

	if upstream := s.upstreamStagesSection(decomposeID, contract, title); upstream != "" {
		parts = append(parts, upstream)
	}
	if answers := s.stageAnswersSection(decomposeID, title); answers != "" {
		parts = append(parts, answers)
	}

	return strings.Join(parts, "\n\n"), nil
}

// matchedRulesSection renders the rules attached to the plan's subjects, the same
// set the workplace view shows. Rules are per subject, not per stage, so every
// stage subagent gets all of them.
func (s *ChatService) matchedRulesSection(ctx context.Context, decomposeID uuid.UUID) (string, error) {
	if s.rulesSvc == nil {
		return "", nil
	}
	d, err := s.dialogRepo.GetDialog(ctx, decomposeID)
	if err != nil {
		return "", fmt.Errorf("get dialog: %w", err)
	}

	var sections []string
	for _, subject := range d.Subjects {
		name := strings.TrimSpace(subject)
		if name == "" {
			continue
		}
		rule, err := s.rulesSvc.Get(name)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) || errors.Is(err, rules.ErrInvalidName) {
				continue
			}
			return "", fmt.Errorf("get rule %q: %w", name, err)
		}
		rendered, err := RenderTemplateVariables(ctx, rule.Content, s.variableRepo, s.selection)
		if err != nil {
			return "", fmt.Errorf("render rule %q: %w", name, err)
		}
		sections = append(sections, "### "+rule.Name+"\n\n"+rendered)
	}
	return strings.Join(sections, "\n\n"), nil
}

// stageContractSection gives the subagent the whole shared vocabulary plus its own
// obligations. The full list matters even for keys this stage does not consume:
// it is what stops the stage inventing a second name for something already named.
func stageContractSection(contract PlanContract, title string) string {
	var own any
	for name, stage := range contract.Stages {
		if normalizeStageTitle(name) == normalizeStageTitle(title) {
			own = stage
			break
		}
	}
	return contractSection(contract, own)
}

// sharedContractSection is the contract without a stage of one's own, which is
// what the rollback agent gets: it owns no stage, but every name the stages agreed
// on is a name its entries have to spell the same way.
func sharedContractSection(contract PlanContract) string {
	return contractSection(contract, nil)
}

func contractSection(contract PlanContract, own any) string {
	if len(contract.Shared) == 0 && len(contract.Stages) == 0 {
		return ""
	}

	payload := map[string]any{"shared": contract.Shared}
	if own != nil {
		payload["yourStage"] = own
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return ""
	}
	return "## Contract\n\nUse these values verbatim. Other agents are planning against the same strings right now.\n\n```json\n" +
		string(data) + "\n```"
}

// upstreamStagesSection returns the already-written stages this one was marked as
// genuinely unable to proceed without. Ordinary contract dependencies are answered
// by the values above and produce nothing here.
func (s *ChatService) upstreamStagesSection(decomposeID uuid.UUID, contract PlanContract, title string) string {
	var own PlanContractStage
	for name, stage := range contract.Stages {
		if normalizeStageTitle(name) == normalizeStageTitle(title) {
			own = stage
			break
		}
	}
	if !own.Blocking || len(own.Requires) == 0 {
		return ""
	}

	producers := make(map[string]struct{})
	for name, stage := range contract.Stages {
		if normalizeStageTitle(name) == normalizeStageTitle(title) {
			continue
		}
		for _, provided := range stage.Provides {
			for _, needed := range own.Requires {
				if provided == needed {
					producers[normalizeStageTitle(name)] = struct{}{}
				}
			}
		}
	}
	if len(producers) == 0 {
		return ""
	}

	raw, found, err := s.ReadActionPlan(decomposeID)
	if err != nil || !found {
		return ""
	}
	var plan storedActionPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return ""
	}

	var wanted []storedActionStage
	for _, stage := range plan.Stages {
		if _, ok := producers[normalizeStageTitle(stage.Title)]; ok {
			wanted = append(wanted, stage)
		}
	}
	if len(wanted) == 0 {
		return ""
	}

	data, err := json.MarshalIndent(wanted, "", "  ")
	if err != nil {
		return ""
	}
	return "## Upstream stages\n\nAlready planned and settled. Build on them; do not restate or revise them.\n\n```json\n" +
		string(data) + "\n```"
}

// stageAnswersSection replays the user's answers to questions an earlier attempt
// at this stage raised, so the replanned stage replaces its assumptions. A
// blocker that carries a kind belongs to another agent, whose title this stage
// may legitimately share.
func (s *ChatService) stageAnswersSection(decomposeID uuid.UUID, title string) string {
	run, found, err := s.ReadFanoutRun(decomposeID)
	if err != nil || !found {
		return ""
	}
	return answersSection(run.Blockers, func(b PlanBlocker) bool {
		return b.Kind == FanoutStageKindStage && normalizeStageTitle(b.Stage) == normalizeStageTitle(title)
	})
}

// answersSection renders the answered blockers a predicate selects.
func answersSection(blockers []PlanBlocker, mine func(PlanBlocker) bool) string {
	var lines []string
	for _, b := range blockers {
		if !mine(b) || strings.TrimSpace(b.Answer) == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s\n  - Answer: %s\n  - Supersedes your earlier assumption: %s",
			b.Question, b.Answer, b.Assumption))
	}
	if len(lines) == 0 {
		return ""
	}
	return "## Answers\n\nThe user has answered the questions you raised. These replace the assumptions you made.\n\n" +
		strings.Join(lines, "\n")
}
