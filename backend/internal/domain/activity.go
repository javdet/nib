package domain

type AgentActivityKind string

const (
	ActivityToolsStart        AgentActivityKind = "tools_start"
	ActivityToolsEnd          AgentActivityKind = "tools_end"
	ActivityTurnEnd           AgentActivityKind = "turn_end"
	ActivityAgentResult       AgentActivityKind = "agent_result"
	ActivityActionPlanUpdated AgentActivityKind = "action_plan_updated"

	// Plan fan-out events. A stage subagent runs in its own dialog, so these are
	// published against the decompose dialog that owns the plan and the run.
	ActivityPlanStageStarted AgentActivityKind = "plan_stage_started"
	ActivityPlanStageDone    AgentActivityKind = "plan_stage_done"
	ActivityPlanStageFailed  AgentActivityKind = "plan_stage_failed"
	ActivityPlanFanoutDone   AgentActivityKind = "plan_fanout_done"
)

type AgentActivity struct {
	Kind  AgentActivityKind `json:"kind"`
	Round int               `json:"round,omitempty"`
	Count int               `json:"count,omitempty"`
	// Stage names the plan stage a fan-out event belongs to.
	Stage string `json:"stage,omitempty"`
	// Status carries the outcome of a stage or of a whole fan-out run.
	Status string `json:"status,omitempty"`
}
