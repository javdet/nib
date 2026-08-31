package service

import (
	"github.com/google/uuid"
	"github.com/javdet/nib/internal/toolcatalog"
)

// toolBinding carries everything a local tool registrar needs beyond the catalog
// and the allow set.
//
// dialogID and planID are the same for an ordinary turn, and differ only for a
// plan stage subagent: its transcript lives in its own dialog while the action
// plan it writes belongs to the decompose dialog that spawned the fan-out.
type toolBinding struct {
	// dialogID owns the transcript: chat_name, ask_question and attachments.
	dialogID uuid.UUID
	// planID owns the action plan and the DAG it must match.
	planID uuid.UUID
	// stage locks update_action_plan to a single DAG stage. Empty for an
	// ordinary turn, which may write any stage.
	stage string
	// categoryNames enumerates the tool categories set_category may choose from.
	// It is passed per call rather than read from ChatService so concurrent
	// catalog builds cannot race on shared state.
	categoryNames []string
}

// newToolBinding returns a binding for an ordinary turn, where the dialog that
// holds the transcript also owns the action plan.
func newToolBinding(dialogID uuid.UUID) toolBinding {
	return toolBinding{dialogID: dialogID, planID: dialogID}
}

// localToolRegistrar registers a single local tool into the catalog when enabled.
type localToolRegistrar func(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{})

// localToolRegistrars preserves the historical tool registration order.
var localToolRegistrars = []localToolRegistrar{
	registerChatNameTool,
	registerKnowledgeSearchTool,
	registerToolSearchTool,
	registerGetSkillTool,
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
	registerCreateSubjectsTool,
	registerSetCategoryTool,
	registerCreateActionPlanTool,
	registerUpdateActionPlanTool,
	registerGetActionListTool,
	registerGetKBDocumentTool,
	registerUpdateKBTool,
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
	if b.dialogID == uuid.Nil || !localToolAllowed(allow, CreateDAGToolName) {
		return
	}
	catalog.localHandlers[CreateDAGToolName] = s.createDAGHandler(b.dialogID)
	catalog.tools = append(catalog.tools, CreateDAGToolDef())
}

func registerCreateSummaryTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.dialogID == uuid.Nil || !localToolAllowed(allow, CreateSummaryToolName) {
		return
	}
	catalog.localHandlers[CreateSummaryToolName] = s.createSummaryHandler(b.dialogID)
	catalog.tools = append(catalog.tools, CreateSummaryToolDef())
}

func registerCreateTableTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.dialogID == uuid.Nil || !localToolAllowed(allow, CreateTableToolName) {
		return
	}
	catalog.localHandlers[CreateTableToolName] = s.createTableHandler(b.dialogID)
	catalog.tools = append(catalog.tools, CreateTableToolDef())
}

func registerCreateSubjectsTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.dialogID == uuid.Nil || !localToolAllowed(allow, CreateSubjectsToolName) {
		return
	}
	catalog.localHandlers[CreateSubjectsToolName] = s.createSubjectsHandler(b.dialogID)
	catalog.tools = append(catalog.tools, CreateSubjectsToolDef())
}

func registerSetCategoryTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.dialogID == uuid.Nil || s.toolCategorySvc == nil || !localToolAllowed(allow, SetCategoryToolName) {
		return
	}
	catalog.localHandlers[SetCategoryToolName] = s.setCategoryHandler(b.dialogID)
	catalog.tools = append(catalog.tools, SetCategoryToolDef(b.categoryNames))
}

func registerCreateActionPlanTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.planID == uuid.Nil || !localToolAllowed(allow, CreateActionPlanToolName) {
		return
	}
	catalog.localHandlers[CreateActionPlanToolName] = s.createActionPlanHandler(b.planID)
	catalog.tools = append(catalog.tools, CreateActionPlanToolDef(s.allowToolsDir))
}

func registerUpdateActionPlanTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.planID == uuid.Nil || !localToolAllowed(allow, UpdateActionPlanToolName) {
		return
	}
	catalog.localHandlers[UpdateActionPlanToolName] = s.updateActionPlanHandler(b.planID, b.stage)
	catalog.tools = append(catalog.tools, UpdateActionPlanToolDef(s.allowToolsDir))
}

func registerGetActionListTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.planID == uuid.Nil || s.dialogRepo == nil || !localToolAllowed(allow, GetActionListToolName) {
		return
	}
	catalog.localHandlers[GetActionListToolName] = s.getActionListHandler(b.planID)
	catalog.tools = append(catalog.tools, GetActionListToolDef())
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

func registerCreatePlanContractTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.dialogID == uuid.Nil || !localToolAllowed(allow, CreatePlanContractToolName) {
		return
	}
	catalog.localHandlers[CreatePlanContractToolName] = s.createPlanContractHandler(b.dialogID)
	catalog.tools = append(catalog.tools, CreatePlanContractToolDef())
}

// registerReportBlockerTool is stage-scoped: only a fan-out subagent has a stage
// to report against, and only it lacks ask_question.
func registerReportBlockerTool(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	if b.planID == uuid.Nil || b.stage == "" || !localToolAllowed(allow, ReportBlockerToolName) {
		return
	}
	catalog.localHandlers[ReportBlockerToolName] = s.reportBlockerHandler(b.planID, b.stage)
	catalog.tools = append(catalog.tools, ReportBlockerToolDef())
}
