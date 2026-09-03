package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

// mainDialogMode is the orchestrator mode. It is the only mode that knows
// sub-agents exist, and the only one a plan starts in.
const mainDialogMode = "main"

// SubagentName is what the orchestrator addresses a specialist by.
//
// It is deliberately not a mode name: "plan" launches a fan-out of many
// plan-mode agents plus the one that derives the rollback, not a single agent in
// plan mode.
type SubagentName string

const (
	SubagentDecompose SubagentName = "decompose"
	SubagentPlan      SubagentName = "plan"
	SubagentExecute   SubagentName = "execute"
)

// SubagentRequest is run_subagent's arguments after parsing. Every field is read
// by exactly one specialist; the tool schema says which, and the handler checks
// it.
type SubagentRequest struct {
	Name SubagentName
	// Task is decompose's brief: the operator's request verbatim, or their
	// answer to a question decompose raised.
	Task string
	// Stages restricts a planning run to those DAG stages, named by title or by
	// the number the operator counted off the diagram. Empty with Rollback set
	// redoes the rollback on its own.
	Stages []string
	// Rollback overrides whether a planning run ends with the rollback agent.
	// Nil means "as usual": the rollback follows whatever the stages say.
	Rollback *bool
	// Item is the plan row to execute, as the operator sees it: 1.1, R1.
	Item string
	// Rerun replaces an attempt already running on Item.
	Rerun bool
}

// SubagentStatus is how a launch ended, from the orchestrator's point of view.
type SubagentStatus string

const (
	// SubagentCompleted means the sub-agent finished and Summary is its answer.
	SubagentCompleted SubagentStatus = "completed"
	// SubagentAwaitingInput means it is suspended on Questions, which the
	// orchestrator must put to the operator with ask_question.
	SubagentAwaitingInput SubagentStatus = "awaiting_input"
	// SubagentStarted means the work runs in the background and reports into the
	// orchestrator's chat on its own.
	SubagentStarted SubagentStatus = "started"
	// SubagentRefused means a precondition failed in a way the operator can fix.
	// Summary says which; it is not an error.
	SubagentRefused SubagentStatus = "refused"
)

// SubagentResult is what a launch hands back to the orchestrator's turn.
type SubagentResult struct {
	Status    SubagentStatus
	Summary   string
	Questions []domain.Question
	// Next says what to do with this result. It rides on the tool output rather
	// than the system prompt because it is specific to the status: a prompt
	// cannot say "this one is still running" only sometimes.
	Next string
}

// SubagentSpec is one entry of the registry: what the orchestrator may launch,
// how the tool schema describes it, and what launching it does.
type SubagentSpec struct {
	Name        SubagentName
	Description string
	// Params names the run_subagent arguments this specialist reads. The schema
	// is generated from it, so a new specialist needs no schema edit.
	Params []string
	// Required names the params a launch cannot go ahead without.
	Required []string
	Launch   func(s *ChatService, ctx context.Context, rootID uuid.UUID, req SubagentRequest) (SubagentResult, error)
}

// subagentSpecs is the only place the set of sub-agents is written down.
//
// It is filled in init for the same reason localToolRegistrars is: a Launch body
// reaches StartActionAgent, which builds a tool catalog, which walks
// localToolRegistrars, one of which reads this list. Nothing recurses at run
// time -- run_subagent is stripped from every sub-agent's allow set -- but it is
// a genuine cycle to the compiler. Do not fold this back into the var.
var subagentSpecs []SubagentSpec

func init() {
	subagentSpecs = []SubagentSpec{
		{
			Name:     SubagentDecompose,
			Params:   []string{"task"},
			Required: []string{"task"},
			Description: "Works a task out into stages: finds it in the task tracker, decides the " +
				"subjects, the action and the location, writes the summary, the DAG and the " +
				"cross-stage contract, and picks the tool categories the plan will need. Launch it " +
				"when the operator asks to plan a task or to work on one, and again for every " +
				"change to the DAG afterwards. It runs to completion inside this call, and may come " +
				"back asking for a decision you relay.",
			Launch: func(s *ChatService, ctx context.Context, rootID uuid.UUID, req SubagentRequest) (SubagentResult, error) {
				return s.launchDecomposeSubagent(ctx, rootID, req.Task)
			},
		},
		{
			Name:   SubagentPlan,
			Params: []string{"stages", "rollback"},
			Description: "Works the DAG out into an action plan: one planning agent per stage, running " +
				"in parallel wave by wave, then one more that derives the rollback from every stage " +
				"once they are written. Name stages to replan only those. Returns as soon as the run " +
				"starts; each stage reports into this chat as it lands.",
			Launch: func(s *ChatService, ctx context.Context, rootID uuid.UUID, req SubagentRequest) (SubagentResult, error) {
				return s.launchPlanSubagent(ctx, rootID, req.Stages, req.Rollback)
			},
		},
		{
			Name:     SubagentExecute,
			Params:   []string{"item", "rerun"},
			Required: []string{"item"},
			Description: "Carries out one item of the action plan, named by the number the operator sees " +
				"beside it. A code action goes to the coding agent in a container and opens a pull " +
				"request; anything else goes to an agent that executes it and posts its result into " +
				"this chat. Returns as soon as the work starts. Only one execution runs at a time.",
			Launch: func(s *ChatService, ctx context.Context, rootID uuid.UUID, req SubagentRequest) (SubagentResult, error) {
				return s.launchExecuteSubagent(ctx, rootID, req.Item, req.Rerun)
			},
		},
	}
}

func subagentSpec(name SubagentName) (SubagentSpec, bool) {
	for _, spec := range subagentSpecs {
		if spec.Name == name {
			return spec, true
		}
	}
	return SubagentSpec{}, false
}

func subagentNames() []string {
	out := make([]string, 0, len(subagentSpecs))
	for _, spec := range subagentSpecs {
		out = append(out, string(spec.Name))
	}
	return out
}

// stripSubagentTools removes the tools that belong to the orchestrator alone,
// whatever an operator's edit to a mode allow list says.
//
// run_subagent: a sub-agent spawning sub-agents has no chat to report into.
// stop_execution: a sub-agent holding the execution lease would be stopping
// itself. execute_action: handing out more work while the orchestrator holds the
// one execution slot for the row this sub-agent was given.
//
// This is code rather than JSON because SeedAllowLists never takes a name out of
// a list already on a data volume, so every upgraded install still has
// execute_action in data/tools/decompose.json.
func stripSubagentTools(allow map[string]struct{}) map[string]struct{} {
	delete(allow, RunSubagentToolName)
	delete(allow, StopExecutionToolName)
	delete(allow, ExecuteActionToolName)
	return allow
}

// subagentSystemPrompt is a mode prompt plus the overlay that narrows it to one
// sub-agent's job.
func (s *ChatService) subagentSystemPrompt(ctx context.Context, modeName, overlayName string) (string, error) {
	base, err := s.resolveSystemPrompt(ctx, modeName)
	if err != nil {
		return "", err
	}
	if s.systemPromptsSvc == nil {
		return base, nil
	}

	overlay, err := s.systemPromptsSvc.Get(overlayName)
	if err != nil {
		return "", fmt.Errorf("resolve %s prompt: %w", overlayName, err)
	}
	rendered, err := RenderTemplateVariables(ctx, overlay.Content, s.variableRepo, s.selection)
	if err != nil {
		return "", err
	}
	return base + "\n\n" + rendered, nil
}

// subagentClaim serialises launches of one sub-agent of one plan, keyed by root
// dialog and name. Two run_subagent calls in a single round already run in
// order, since the loop executes tool calls sequentially, so this guards two
// concurrent turns on the same plan.
func (s *ChatService) subagentClaim(rootID uuid.UUID, name SubagentName) *tryMutex {
	key := rootID.String() + "|" + string(name)
	mu, _ := s.subagentClaims.LoadOrStore(key, &tryMutex{ch: make(chan struct{}, 1)})
	return mu.(*tryMutex)
}

// tryMutex is a lock that can be tested rather than waited on. A launch blocked
// on a claim would hold the whole orchestrator turn, so a busy claim is reported
// to the model as a refusal instead.
type tryMutex struct {
	ch chan struct{}
}

func (m *tryMutex) tryLock() bool {
	select {
	case m.ch <- struct{}{}:
		return true
	default:
		return false
	}
}

func (m *tryMutex) unlock() {
	select {
	case <-m.ch:
	default:
	}
}

// resolveOrchestratorRoot checks that rootID really is an orchestrator root and
// returns it. Every launch goes through it, so a sub-agent transcript that
// somehow reached the tool cannot start work against somebody else's plan.
func (s *ChatService) resolveOrchestratorRoot(ctx context.Context, rootID uuid.UUID) (domain.Dialog, error) {
	if s.dialogRepo == nil {
		return domain.Dialog{}, fmt.Errorf("launch sub-agent: dialog repository is not configured")
	}
	d, err := s.dialogRepo.GetDialog(ctx, rootID)
	if err != nil {
		return domain.Dialog{}, fmt.Errorf("launch sub-agent: %w", err)
	}
	if d.ParentID != nil || !strings.EqualFold(d.Mode, mainDialogMode) {
		return domain.Dialog{}, ErrNotMainDialog
	}
	return d, nil
}
