package service

import (
	"github.com/google/uuid"
	"github.com/javdet/nib/internal/toolcatalog"
)

// toolBinding carries everything a local tool registrar needs beyond the catalog
// and the allow set.
//
// dialogID and planID are the same for a root turn and differ for a subagent:
// its transcript lives in its own dialog while every plan artifact -- the
// summary, the DAG, the stage contract, the categories and the
// action plan -- belongs to the root dialog that spawned it.
//
// The two matching is also what marks a turn as the orchestrator's own, which is
// how the tools that launch and stop subagents stay off a subagent's transcript.
type toolBinding struct {
	// dialogID owns the transcript: chat_name, ask_question and attachments.
	dialogID uuid.UUID
	// planID owns every plan artifact and the DAG the plan must match.
	planID uuid.UUID
	// stage locks update_action_plan to a single DAG stage. Empty for an
	// ordinary turn, which may write any stage.
	stage string
	// kind says which part of the plan a fan-out subagent owns, so the blockers
	// it reports come back to it rather than to a stage of the same name.
	kind FanoutStageKind
	// categoryNames enumerates the tool categories set_category may choose from.
	// It is passed per call rather than read from ChatService so concurrent
	// catalog builds cannot race on shared state.
	categoryNames []string
	// mode is the chat mode the turn runs in. It scopes the tools a single mode
	// owns, so an operator adding one of them to another mode's allow list
	// cannot hand it out.
	mode string
}

// newToolBinding returns a binding for an ordinary turn, where the dialog that
// holds the transcript also owns the action plan.
func newToolBinding(dialogID uuid.UUID) toolBinding {
	return toolBinding{dialogID: dialogID, planID: dialogID}
}

// localToolRegistrar registers a single local tool into the catalog when enabled.
type localToolRegistrar func(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{})

// localToolRegistrars preserves the historical tool registration order.
//
// It is filled in init rather than in its own declaration because execute_action
// closes the loop: registering it reaches StartActionAgent, which builds a tool
// catalog for the subagent, which reads this list. That is a genuine cycle to the
// compiler even though nothing recurses at run time -- a subagent never has
// execute_action in its allow set. Do not fold this back into the var.
var localToolRegistrars []localToolRegistrar

func init() {
	localToolRegistrars = []localToolRegistrar{
		registerChatNameTool,
		registerKnowledgeSearchTool,
		registerToolSearchTool,
		registerGetSkillTool,
		registerGetRuleTool,
		registerAPICallTool,
		registerExecuteCommandTool,
		registerGetSecretsTool,
		registerListVariablesTool,
		registerRunExecutorTool,
		registerAskQuestionTool,
		registerReportBlockerTool,
		registerCreateDAGTool,
		registerCreatePlanContractTool,
		registerCreateSummaryTool,
		registerCreateTableTool,
		registerSetCategoryTool,
		registerCreateActionPlanTool,
		registerUpdateActionPlanTool,
		registerUpdateRollbackPlanTool,
		registerGetActionListTool,
		registerGetDAGTool,
		registerExecuteActionTool,
		registerGetKBDocumentTool,
		registerUpdateKBTool,
		registerRunSubagentTool,
		registerStopExecutionTool,
		registerUpdateToolCategoryTool,
	}
}

func (s *ChatService) addLocalTools(catalog *toolCatalog, allow map[string]struct{}, b toolBinding) {
	for _, register := range localToolRegistrars {
		register(s, catalog, b, allow)
	}
}

func registerChatNameTool(s *ChatService, catalog *toolCatalog, b toolBinding, _ map[string]struct{}) {
	if b.dialogID == uuid.Nil {
		return
	}
	catalog.localHandlers[ChatNameToolName] = s.chatNameHandler(b.dialogID)
	catalog.tools = append(catalog.tools, ChatNameToolDef())
}

func registerKnowledgeSearchTool(s *ChatService, catalog *toolCatalog, _ toolBinding, allow map[string]struct{}) {
	if s.knowledgeSvc == nil || !localToolAllowed(allow, KnowledgeSearchToolName) {
		return
	}
	catalog.localHandlers[KnowledgeSearchToolName] = s.knowledgeSvc.ExecuteKnowledgeSearch
	catalog.tools = append(catalog.tools, KnowledgeSearchToolDef())
}

func registerToolSearchTool(s *ChatService, catalog *toolCatalog, _ toolBinding, allow map[string]struct{}) {
	if s.toolSearchSvc == nil || !localToolAllowed(allow, toolcatalog.ToolSearchToolName) {
		return
	}
	catalog.localHandlers[toolcatalog.ToolSearchToolName] = s.toolSearchHandler(catalog)
	catalog.tools = append(catalog.tools, ToolSearchToolDef())
}

func registerGetSkillTool(s *ChatService, catalog *toolCatalog, _ toolBinding, allow map[string]struct{}) {
	if s.skillsSvc == nil || !localToolAllowed(allow, GetSkillToolName) {
		return
	}
	catalog.localHandlers[GetSkillToolName] = s.ExecuteGetSkill
	catalog.tools = append(catalog.tools, GetSkillToolDef())
}

func registerGetRuleTool(s *ChatService, catalog *toolCatalog, _ toolBinding, allow map[string]struct{}) {
	if s.rulesSvc == nil || !localToolAllowed(allow, GetRuleToolName) {
		return
	}
	catalog.localHandlers[GetRuleToolName] = s.ExecuteGetRule
	catalog.tools = append(catalog.tools, GetRuleToolDef())
}

func registerAPICallTool(s *ChatService, catalog *toolCatalog, _ toolBinding, allow map[string]struct{}) {
	if !localToolAllowed(allow, APICallToolName) {
		return
	}
	catalog.localHandlers[APICallToolName] = ExecuteAPICall
	catalog.tools = append(catalog.tools, APICallToolDef())
}

func registerExecuteCommandTool(s *ChatService, catalog *toolCatalog, _ toolBinding, allow map[string]struct{}) {
	if !localToolAllowed(allow, ExecuteCommandToolName) {
		return
	}
	catalog.localHandlers[ExecuteCommandToolName] = ExecuteCommand
	catalog.tools = append(catalog.tools, ExecuteCommandToolDef())
}

func registerGetSecretsTool(s *ChatService, catalog *toolCatalog, _ toolBinding, allow map[string]struct{}) {
	if s.secretSvc == nil || !localToolAllowed(allow, GetSecretsToolName) {
		return
	}
	catalog.localHandlers[GetSecretsToolName] = s.secretSvc.ExecuteGetSecrets
	catalog.tools = append(catalog.tools, GetSecretsToolDef())
}

func registerListVariablesTool(s *ChatService, catalog *toolCatalog, _ toolBinding, allow map[string]struct{}) {
	if s.variableRepo == nil || !localToolAllowed(allow, ListVariablesToolName) {
		return
	}
	catalog.localHandlers[ListVariablesToolName] = s.ExecuteListVariables
	catalog.tools = append(catalog.tools, ListVariablesToolDef())
}

func registerRunExecutorTool(s *ChatService, catalog *toolCatalog, _ toolBinding, allow map[string]struct{}) {
	if s.executorSvc == nil || !localToolAllowed(allow, RunExecutorToolName) {
		return
	}
	catalog.localHandlers[RunExecutorToolName] = s.ExecuteRunExecutor
	catalog.tools = append(catalog.tools, RunExecutorToolDef())
}

func registerAskQuestionTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.dialogID == uuid.Nil || !localToolAllowed(allow, AskQuestionToolName) {
		return
	}
	catalog.tools = append(catalog.tools, AskQuestionToolDef())
}

func registerCreateDAGTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.planID == uuid.Nil || !localToolAllowed(allow, CreateDAGToolName) {
		return
	}
	catalog.localHandlers[CreateDAGToolName] = s.createDAGHandler(b.planID)
	catalog.tools = append(catalog.tools, CreateDAGToolDef())
}

func registerCreateSummaryTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.planID == uuid.Nil || !localToolAllowed(allow, CreateSummaryToolName) {
		return
	}
	catalog.localHandlers[CreateSummaryToolName] = s.createSummaryHandler(b.planID)
	catalog.tools = append(catalog.tools, CreateSummaryToolDef())
}

func registerCreateTableTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.dialogID == uuid.Nil || !localToolAllowed(allow, CreateTableToolName) {
		return
	}
	catalog.localHandlers[CreateTableToolName] = s.createTableHandler(b.dialogID)
	catalog.tools = append(catalog.tools, CreateTableToolDef())
}

func registerSetCategoryTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.planID == uuid.Nil || s.toolCategorySvc == nil || !localToolAllowed(allow, SetCategoryToolName) {
		return
	}
	catalog.localHandlers[SetCategoryToolName] = s.setCategoryHandler(b.planID)
	catalog.tools = append(catalog.tools, SetCategoryToolDef(b.categoryNames))
}

func registerCreateActionPlanTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.planID == uuid.Nil || !localToolAllowed(allow, CreateActionPlanToolName) {
		return
	}
	catalog.localHandlers[CreateActionPlanToolName] = s.createActionPlanHandler(b.planID)
	catalog.tools = append(catalog.tools, CreateActionPlanToolDef(s.allowToolsDir, b.categoryNames))
}

func registerUpdateActionPlanTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.planID == uuid.Nil || !localToolAllowed(allow, UpdateActionPlanToolName) {
		return
	}
	catalog.localHandlers[UpdateActionPlanToolName] = s.updateActionPlanHandler(b.planID, b.stage)
	catalog.tools = append(catalog.tools, UpdateActionPlanToolDef(s.allowToolsDir, b.categoryNames))
}

func registerUpdateRollbackPlanTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.planID == uuid.Nil || !localToolAllowed(allow, UpdateRollbackPlanToolName) {
		return
	}
	catalog.localHandlers[UpdateRollbackPlanToolName] = s.updateRollbackPlanHandler(b.planID)
	catalog.tools = append(catalog.tools, UpdateRollbackPlanToolDef(s.allowToolsDir, b.categoryNames))
}

func registerGetActionListTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.planID == uuid.Nil || s.dialogRepo == nil || !localToolAllowed(allow, GetActionListToolName) {
		return
	}
	catalog.localHandlers[GetActionListToolName] = s.getActionListHandler(b.planID)
	catalog.tools = append(catalog.tools, GetActionListToolDef())
}

func registerGetDAGTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.planID == uuid.Nil || s.dialogRepo == nil || !localToolAllowed(allow, GetDAGToolName) {
		return
	}
	catalog.localHandlers[GetDAGToolName] = s.getDAGHandler(b.planID)
	catalog.tools = append(catalog.tools, GetDAGToolDef())
}

func registerGetKBDocumentTool(s *ChatService, catalog *toolCatalog, _ toolBinding, allow map[string]struct{}) {
	if s.knowledgeSvc == nil || !localToolAllowed(allow, GetKBDocumentToolName) {
		return
	}
	catalog.localHandlers[GetKBDocumentToolName] = s.knowledgeSvc.ExecuteGetKBDocument
	catalog.tools = append(catalog.tools, GetKBDocumentToolDef())
}

func registerUpdateKBTool(s *ChatService, catalog *toolCatalog, _ toolBinding, allow map[string]struct{}) {
	if s.knowledgeSvc == nil || !localToolAllowed(allow, UpdateKBToolName) {
		return
	}
	catalog.localHandlers[UpdateKBToolName] = s.knowledgeSvc.ExecuteUpdateKB
	catalog.tools = append(catalog.tools, UpdateKBToolDef())
}

// registerExecuteActionTool gives a conversation that owns an action plan the
// ability to carry one of its actions out. A subagent executing an action has it
// deleted from its allow set: it holds one row and must not hand out more.
func registerExecuteActionTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.planID == uuid.Nil || s.dialogRepo == nil || !localToolAllowed(allow, ExecuteActionToolName) {
		return
	}
	catalog.localHandlers[ExecuteActionToolName] = s.executeActionHandler(b)
	catalog.tools = append(catalog.tools, ExecuteActionToolDef())
}

func registerCreatePlanContractTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.planID == uuid.Nil || !localToolAllowed(allow, CreatePlanContractToolName) {
		return
	}
	catalog.localHandlers[CreatePlanContractToolName] = s.createPlanContractHandler(b.planID)
	catalog.tools = append(catalog.tools, CreatePlanContractToolDef())
}

// registerReportBlockerTool is stage-scoped: only a fan-out subagent has a stage
// to report against, and only it lacks ask_question.
func registerReportBlockerTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.planID == uuid.Nil || b.stage == "" || !localToolAllowed(allow, ReportBlockerToolName) {
		return
	}
	catalog.localHandlers[ReportBlockerToolName] = s.reportBlockerHandler(b.planID, b.stage, b.kind)
	catalog.tools = append(catalog.tools, ReportBlockerToolDef())
}

// registerRunSubagentTool is the orchestrator's alone.
//
// The binding is the recursion guard: a sub-agent runs with dialogID != planID
// -- its transcript is its own while the plan belongs to the root -- so only a
// root turn ever sees this tool. stripSubagentTools is the second guard, for the
// day some sub-agent runs on a root dialog.
func registerRunSubagentTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.planID == uuid.Nil || b.planID != b.dialogID || s.dialogRepo == nil {
		return
	}
	if !localToolAllowed(allow, RunSubagentToolName) {
		return
	}
	catalog.localHandlers[RunSubagentToolName] = s.runSubagentHandler(b)
	catalog.tools = append(catalog.tools, RunSubagentToolDef())
}

// registerStopExecutionTool is global, like the lease it releases: bound to no
// plan, and offered only on a root turn so a sub-agent cannot stop itself.
func registerStopExecutionTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.planID == uuid.Nil || b.planID != b.dialogID || !localToolAllowed(allow, StopExecutionToolName) {
		return
	}
	catalog.localHandlers[StopExecutionToolName] = s.stopExecutionHandler()
	catalog.tools = append(catalog.tools, StopExecutionToolDef())
}

// registerUpdateToolCategoryTool is discuss mode's alone. The binding is the
// first guard and enforceModeToolLimits, which drops the name from every other
// mode's allow set, is the second: SeedAllowLists never takes a tool out of a
// list on disk, so a list that once carried it would keep offering it.
func registerUpdateToolCategoryTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.mode != discussDialogMode || s.toolCategorySvc == nil {
		return
	}
	if !localToolAllowed(allow, UpdateToolCategoryToolName) {
		return
	}
	catalog.localHandlers[UpdateToolCategoryToolName] = s.updateToolCategoryHandler()
	catalog.tools = append(catalog.tools, UpdateToolCategoryToolDef(b.categoryNames))
}
