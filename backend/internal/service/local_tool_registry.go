package service

import (
	"github.com/javdet/nib/internal/toolcatalog"
	"github.com/google/uuid"
)

// localToolRegistrar registers a single local tool into the catalog when enabled.
type localToolRegistrar func(s *ChatService, catalog *toolCatalog, dialogID uuid.UUID, allow map[string]struct{})

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
	registerCreateDAGTool,
	registerCreateSummaryTool,
	registerCreateTableTool,
	registerCreateSubjectsTool,
	registerSetCategoryTool,
	registerCreateActionPlanTool,
	registerGetActionListTool,
}

func (s *ChatService) addLocalTools(catalog *toolCatalog, allow map[string]struct{}, dialogID uuid.UUID) {
	for _, register := range localToolRegistrars {
		register(s, catalog, dialogID, allow)
	}
}

func registerChatNameTool(s *ChatService, catalog *toolCatalog, dialogID uuid.UUID, _ map[string]struct{}) {
	if dialogID == uuid.Nil {
		return
	}
	catalog.localHandlers[ChatNameToolName] = s.chatNameHandler(dialogID)
	catalog.tools = append(catalog.tools, ChatNameToolDef())
}

func registerKnowledgeSearchTool(s *ChatService, catalog *toolCatalog, _ uuid.UUID, allow map[string]struct{}) {
	if s.knowledgeSvc == nil || !localToolAllowed(allow, KnowledgeSearchToolName) {
		return
	}
	catalog.localHandlers[KnowledgeSearchToolName] = s.knowledgeSvc.ExecuteKnowledgeSearch
	catalog.tools = append(catalog.tools, KnowledgeSearchToolDef())
}

func registerToolSearchTool(s *ChatService, catalog *toolCatalog, _ uuid.UUID, allow map[string]struct{}) {
	if s.toolSearchSvc == nil || !localToolAllowed(allow, toolcatalog.ToolSearchToolName) {
		return
	}
	catalog.localHandlers[toolcatalog.ToolSearchToolName] = s.toolSearchHandler(catalog)
	catalog.tools = append(catalog.tools, ToolSearchToolDef())
}

func registerGetSkillTool(s *ChatService, catalog *toolCatalog, _ uuid.UUID, allow map[string]struct{}) {
	if s.skillsSvc == nil || !localToolAllowed(allow, GetSkillToolName) {
		return
	}
	catalog.localHandlers[GetSkillToolName] = s.ExecuteGetSkill
	catalog.tools = append(catalog.tools, GetSkillToolDef())
}

func registerAPICallTool(s *ChatService, catalog *toolCatalog, _ uuid.UUID, allow map[string]struct{}) {
	if !localToolAllowed(allow, APICallToolName) {
		return
	}
	catalog.localHandlers[APICallToolName] = ExecuteAPICall
	catalog.tools = append(catalog.tools, APICallToolDef())
}

func registerExecuteCommandTool(s *ChatService, catalog *toolCatalog, _ uuid.UUID, allow map[string]struct{}) {
	if !localToolAllowed(allow, ExecuteCommandToolName) {
		return
	}
	catalog.localHandlers[ExecuteCommandToolName] = ExecuteCommand
	catalog.tools = append(catalog.tools, ExecuteCommandToolDef())
}

func registerGetSecretsTool(s *ChatService, catalog *toolCatalog, _ uuid.UUID, allow map[string]struct{}) {
	if s.secretSvc == nil || !localToolAllowed(allow, GetSecretsToolName) {
		return
	}
	catalog.localHandlers[GetSecretsToolName] = s.secretSvc.ExecuteGetSecrets
	catalog.tools = append(catalog.tools, GetSecretsToolDef())
}

func registerListVariablesTool(s *ChatService, catalog *toolCatalog, _ uuid.UUID, allow map[string]struct{}) {
	if s.variableRepo == nil || !localToolAllowed(allow, ListVariablesToolName) {
		return
	}
	catalog.localHandlers[ListVariablesToolName] = s.ExecuteListVariables
	catalog.tools = append(catalog.tools, ListVariablesToolDef())
}

func registerRunExecutorTool(s *ChatService, catalog *toolCatalog, _ uuid.UUID, allow map[string]struct{}) {
	if s.executorSvc == nil || !localToolAllowed(allow, RunExecutorToolName) {
		return
	}
	catalog.localHandlers[RunExecutorToolName] = s.ExecuteRunExecutor
	catalog.tools = append(catalog.tools, RunExecutorToolDef())
}

func registerAskQuestionTool(s *ChatService, catalog *toolCatalog, dialogID uuid.UUID, allow map[string]struct{}) {
	if dialogID == uuid.Nil || !localToolAllowed(allow, AskQuestionToolName) {
		return
	}
	catalog.tools = append(catalog.tools, AskQuestionToolDef())
}

func registerCreateDAGTool(s *ChatService, catalog *toolCatalog, dialogID uuid.UUID, allow map[string]struct{}) {
	if dialogID == uuid.Nil || !localToolAllowed(allow, CreateDAGToolName) {
		return
	}
	catalog.localHandlers[CreateDAGToolName] = s.createDAGHandler(dialogID)
	catalog.tools = append(catalog.tools, CreateDAGToolDef())
}

func registerCreateSummaryTool(s *ChatService, catalog *toolCatalog, dialogID uuid.UUID, allow map[string]struct{}) {
	if dialogID == uuid.Nil || !localToolAllowed(allow, CreateSummaryToolName) {
		return
	}
	catalog.localHandlers[CreateSummaryToolName] = s.createSummaryHandler(dialogID)
	catalog.tools = append(catalog.tools, CreateSummaryToolDef())
}

func registerCreateTableTool(s *ChatService, catalog *toolCatalog, dialogID uuid.UUID, allow map[string]struct{}) {
	if dialogID == uuid.Nil || !localToolAllowed(allow, CreateTableToolName) {
		return
	}
	catalog.localHandlers[CreateTableToolName] = s.createTableHandler(dialogID)
	catalog.tools = append(catalog.tools, CreateTableToolDef())
}

func registerCreateSubjectsTool(s *ChatService, catalog *toolCatalog, dialogID uuid.UUID, allow map[string]struct{}) {
	if dialogID == uuid.Nil || !localToolAllowed(allow, CreateSubjectsToolName) {
		return
	}
	catalog.localHandlers[CreateSubjectsToolName] = s.createSubjectsHandler(dialogID)
	catalog.tools = append(catalog.tools, CreateSubjectsToolDef())
}

func registerSetCategoryTool(s *ChatService, catalog *toolCatalog, dialogID uuid.UUID, allow map[string]struct{}) {
	if dialogID == uuid.Nil || s.toolCategorySvc == nil || !localToolAllowed(allow, SetCategoryToolName) {
		return
	}
	catalog.localHandlers[SetCategoryToolName] = s.setCategoryHandler(dialogID)
	catalog.tools = append(catalog.tools, SetCategoryToolDef(s.toolCategoryNamesSnapshot))
}

func registerCreateActionPlanTool(s *ChatService, catalog *toolCatalog, dialogID uuid.UUID, allow map[string]struct{}) {
	if dialogID == uuid.Nil || !localToolAllowed(allow, CreateActionPlanToolName) {
		return
	}
	catalog.localHandlers[CreateActionPlanToolName] = s.createActionPlanHandler(dialogID)
	catalog.tools = append(catalog.tools, CreateActionPlanToolDef(s.allowToolsDir))
}

func registerGetActionListTool(s *ChatService, catalog *toolCatalog, dialogID uuid.UUID, allow map[string]struct{}) {
	if dialogID == uuid.Nil || s.dialogRepo == nil || !localToolAllowed(allow, GetActionListToolName) {
		return
	}
	catalog.localHandlers[GetActionListToolName] = s.getActionListHandler(dialogID)
	catalog.tools = append(catalog.tools, GetActionListToolDef())
}
