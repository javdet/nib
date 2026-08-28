package domain

type AgentActivityKind string

const (
	ActivityToolsStart         AgentActivityKind = "tools_start"
	ActivityToolsEnd           AgentActivityKind = "tools_end"
	ActivityTurnEnd            AgentActivityKind = "turn_end"
	ActivityAgentResult        AgentActivityKind = "agent_result"
	ActivityActionPlanUpdated  AgentActivityKind = "action_plan_updated"
)

type AgentActivity struct {
	Kind  AgentActivityKind `json:"kind"`
	Round int               `json:"round,omitempty"`
	Count int               `json:"count,omitempty"`
}
