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

	// Action execution events. A per-action subagent runs in its own dialog, so
	// like the fan-out kinds these are published against the decompose dialog
	// that owns the plan and the run record.
	ActivityActionExecStarted AgentActivityKind = "action_exec_started"
	ActivityActionExecDone    AgentActivityKind = "action_exec_done"
	ActivityActionExecFailed  AgentActivityKind = "action_exec_failed"

	// ActivityReportUpdated says a plan's closing report has been written. Like
	// the kinds above it is published against the plan dialog rather than the
	// subagent's own, because the report belongs to the plan and the plan page
	// is what listens.
	ActivityReportUpdated AgentActivityKind = "report_updated"
)

type AgentActivity struct {
	Kind  AgentActivityKind `json:"kind"`
	Round int               `json:"round,omitempty"`
	Count int               `json:"count,omitempty"`
	// Stage names the plan stage a fan-out event belongs to.
	Stage string `json:"stage,omitempty"`
	// Action is the row key of the plan item an action-execution event belongs
	// to. The operator-facing number is derived from position, so the key is
	// what stays correct across a reorder.
	Action string `json:"action,omitempty"`
	// Status carries the outcome of a stage or of a whole fan-out run.
	Status string `json:"status,omitempty"`
}
