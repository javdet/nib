package handler

import (
	"github.com/javdet/nib/internal/includedtools"
	"github.com/javdet/nib/internal/executor"
	"github.com/javdet/nib/internal/mcpconfig"
	"github.com/javdet/nib/internal/repository"
	"github.com/javdet/nib/internal/rules"
	"github.com/javdet/nib/internal/service"
	"github.com/javdet/nib/internal/skills"
	"github.com/javdet/nib/internal/systemprompts"
	"github.com/javdet/nib/internal/toolcatalog"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter builds the chi router with all middleware and routes.
func NewRouter(
	allowedOrigins []string,
	projectSvc *service.ProjectService,
	envSvc *service.EnvironmentService,
	cloudSvc *service.CloudService,
	locationSvc *service.LocationService,
	knowledgeSvc *service.KnowledgeService,
	mcpSvc *service.MCPService,
	chatSvc *service.ChatService,
	systemPromptsSvc *systemprompts.Service,
	rulesSvc *rules.Service,
	mcpConfigSvc *mcpconfig.Service,
	includedToolsSvc *includedtools.Service,
	toolCatalogStore *toolcatalog.Store,
	executorSvc *executor.Service,
	skillSvc *skills.Service,
	variableRepo repository.VariableRepository,
	variableSvc *service.VariableService,
	secretSvc *service.SecretService,
	companySvc *service.CompanyService,
	dialogSvc *service.DialogService,
	selectionStore *service.SelectionStore,
	toolCategorySvc *service.ToolCategoryService,
	frontendBaseURL string,
	agentWebhookToken string,
) chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware(allowedOrigins))

	agentRun := writeDeadline(agentWriteTimeout)

	projects := NewProjectHandler(projectSvc)
	environments := NewEnvironmentHandler(envSvc)
	clouds := NewCloudHandler(cloudSvc)
	locations := NewLocationHandler(locationSvc)
	knowledge := NewKnowledgeHandler(knowledgeSvc)
	mcpHandler := NewMCPHandler(mcpSvc, frontendBaseURL)
	chat := NewChatHandler(chatSvc)
	systemTools := NewSystemToolsHandler(chatSvc)
	systemPrompts := NewSystemPromptsHandler(systemPromptsSvc)
	rulesHandler := NewRulesHandler(rulesSvc)
	mcpConfig := NewMCPConfigHandler(mcpConfigSvc)
	includedTools := NewIncludedToolsHandler(includedToolsSvc, toolCatalogStore, chatSvc)
	executorHandler := NewExecutorHandler(executorSvc)
	skills := NewSkillHandler(skillSvc, variableRepo, selectionStore)
	variables := NewVariableHandler(variableSvc)
	secrets := NewSecretHandler(secretSvc)
	company := NewCompanyHandler(companySvc)
	dialogs := NewDialogHandler(dialogSvc, chatSvc)
	selection := NewSelectionHandler(selectionStore)
	toolCategories := NewToolCategoriesHandler(toolCategorySvc)
	agentWebhook := NewAgentWebhookHandler(chatSvc, agentWebhookToken)
	execution := NewExecutionHandler(chatSvc)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", HealthCheck())
		r.Get("/version", VersionInfo())
		r.Get("/modes", ListModes())
		r.Get("/system/tools", systemTools.List())

		// The execution routes deliberately take no write deadline: a force stop
		// is a docker or kubernetes call, not an agent run, and it has to work
		// while the agent loop holding the lease is wedged -- which is exactly
		// when it is reached for.
		r.Route("/execution", func(r chi.Router) {
			r.Get("/", execution.Get())
			r.Delete("/", execution.Stop())
		})
		r.Post("/agent-runner/webhook", agentWebhook.Receive())

		r.Route("/selection", func(r chi.Router) {
			r.Get("/", selection.Get())
			r.Put("/", selection.Set())
		})

		r.Route("/projects", func(r chi.Router) {
			r.Get("/", projects.List())
			r.Post("/", projects.Create())

			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", projects.GetByID())
				r.Put("/", projects.Update())
				r.Delete("/", projects.Delete())

				r.Route("/environments", func(r chi.Router) {
					r.Get("/", environments.List())
					r.Post("/", environments.Create())

					r.Route("/{envID}", func(r chi.Router) {
						r.Get("/", environments.GetByID())
						r.Put("/", environments.Update())
						r.Delete("/", environments.Delete())
					})
				})

				r.Route("/clouds", func(r chi.Router) {
					r.Get("/", clouds.List())
					r.Post("/", clouds.Create())

					r.Route("/{cloudID}", func(r chi.Router) {
						r.Get("/", clouds.GetByID())
						r.Put("/", clouds.Update())
						r.Delete("/", clouds.Delete())

						r.Route("/locations", func(r chi.Router) {
							r.Get("/", locations.List())
							r.Post("/", locations.Create())

							r.Route("/{locationID}", func(r chi.Router) {
								r.Get("/", locations.GetByID())
								r.Put("/", locations.Update())
								r.Delete("/", locations.Delete())
							})
						})
					})
				})
			})
		})

		r.Route("/knowledge", func(r chi.Router) {
			r.Get("/connection", knowledge.GetConnection())
			r.Put("/connection", knowledge.UpdateConnection())
			r.Get("/collections", knowledge.ListCollections())
			r.Get("/documents", knowledge.GetDocument())
			r.Post("/documents", knowledge.UploadDocument())
			r.Get("/status", knowledge.GetStatus())
		})

		r.With(agentRun).Post("/chat", chat.Send())

		r.Route("/system-prompts/{name}", func(r chi.Router) {
			r.Get("/", systemPrompts.Get())
			r.Put("/", systemPrompts.Update())
			r.Delete("/", systemPrompts.Reset())
		})

		r.Route("/skills", func(r chi.Router) {
			r.Get("/", skills.List())
			r.Post("/", skills.Create())

			r.Route("/{name}", func(r chi.Router) {
				r.Get("/rendered", skills.GetRendered())
				r.Get("/", skills.Get())
				r.Put("/", skills.Update())
				r.Delete("/", skills.Delete())
			})
		})

		r.Route("/rules", func(r chi.Router) {
			r.Get("/", rulesHandler.List())
			r.Post("/", rulesHandler.Create())

			r.Route("/{name}", func(r chi.Router) {
				r.Get("/", rulesHandler.Get())
				r.Put("/", rulesHandler.Update())
				r.Delete("/", rulesHandler.Delete())
			})
		})

		r.Route("/variables", func(r chi.Router) {
			r.Get("/", variables.List())
			r.Post("/", variables.Create())

			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", variables.Get())
				r.Put("/", variables.Update())
				r.Delete("/", variables.Delete())
			})
		})

		r.Route("/secrets", func(r chi.Router) {
			r.Get("/", secrets.List())
			r.Post("/", secrets.Create())

			r.Route("/{id}", func(r chi.Router) {
				r.Put("/", secrets.Update())
				r.Delete("/", secrets.Delete())
			})
		})

		r.Route("/company", func(r chi.Router) {
			r.Get("/", company.Get())
			r.Put("/", company.Save())
		})

		r.Route("/dialogs", func(r chi.Router) {
			r.Get("/", dialogs.List())
			r.Post("/", dialogs.Create())

			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", dialogs.Get())
				r.Put("/title", dialogs.UpdateTitle())
				r.Put("/subjects", dialogs.UpdateSubjects())
				r.Put("/categories", dialogs.UpdateCategories())
				r.Put("/pin", dialogs.Pin())
				r.Get("/children", dialogs.ListChildren())
				r.Get("/dag", dialogs.DAG())
				r.Get("/summary", dialogs.Summary())
				r.Put("/summary", dialogs.UpdateSummary())
				r.Get("/rules", dialogs.Rules())
				r.Get("/events", dialogs.Events())
				r.Get("/plan-state", dialogs.PlanState())
				r.Put("/plan-state/schedule", dialogs.SetPlanSchedule())
				r.Put("/plan-state/status", dialogs.SetPlanStatus())
				r.Get("/action-plan", dialogs.ActionPlan())
				r.Put("/action-plan", dialogs.UpdateActionPlan())
				r.Put("/action-plan/reorder", dialogs.ReorderActionPlan())
				r.Put("/action-plan/checks", dialogs.SetActionPlanChecks())
				r.Put("/action-plan/comments", dialogs.SetActionPlanComments())
				r.Post("/action-plan/execute", dialogs.ExecuteActionPlanAction())
				r.Get("/action-plan/exec", dialogs.ActionPlanExecRuns())
				// The fan-out answers 202 and runs on past this request, so it
				// deliberately does not take the agentRun write deadline.
				r.Post("/plan-fanout", dialogs.StartPlanFanout())
				r.Get("/plan-fanout", dialogs.PlanFanout())
				r.Delete("/plan-fanout", dialogs.CancelPlanFanout())
				r.Delete("/", dialogs.Delete())
				r.With(agentRun).Post("/tool-results", dialogs.SubmitToolResult())

				r.Route("/messages", func(r chi.Router) {
					r.Get("/", dialogs.ListMessages())
					r.With(agentRun).Post("/", dialogs.SendMessage())
					r.With(agentRun).Post("/retry", dialogs.Retry())
				})

				r.Route("/attachments", func(r chi.Router) {
					r.Get("/", dialogs.ListAttachments())
					r.Post("/", dialogs.UploadAttachment())

					r.Route("/{attachmentId}", func(r chi.Router) {
						r.Get("/", dialogs.GetAttachment())
						r.Delete("/", dialogs.DeleteAttachment())
					})
				})
			})
		})

		r.Route("/included-tools", func(r chi.Router) {
			r.Get("/{mode}", includedTools.Get())
			r.Put("/{mode}", includedTools.Set())
		})

		r.Get("/mcp/catalog-tools", includedTools.ListCatalogTools())

		r.Route("/tool-categories", func(r chi.Router) {
			r.Get("/", toolCategories.List())
			r.Get("/uncategorized/tools", toolCategories.ListUncategorizedTools())

			r.Route("/{name}", func(r chi.Router) {
				r.Get("/tools", toolCategories.ListTools())
				r.Put("/patterns", toolCategories.SetPatterns())
			})
		})

		r.Route("/mcp/servers", func(r chi.Router) {
			r.Get("/", mcpConfig.List())
			r.Post("/", mcpConfig.Create())

			r.Route("/{name}", func(r chi.Router) {
				r.Get("/tools", mcpConfig.ListTools())
				r.Get("/", mcpConfig.Get())
				r.Put("/", mcpConfig.Update())
				r.Delete("/", mcpConfig.Delete())
			})
		})

		r.Route("/mcp/config/raw", func(r chi.Router) {
			r.Get("/", mcpConfig.GetRaw())
			r.Put("/", mcpConfig.SetRaw())
		})

		r.Route("/executor/config", func(r chi.Router) {
			r.Get("/", executorHandler.GetConfig())
			r.Put("/", executorHandler.UpdateConfig())
		})

		r.Route("/mcp/connections", func(r chi.Router) {
			r.Get("/", mcpHandler.List())
			r.Post("/", mcpHandler.Create())

			r.Route("/{type}/auth", func(r chi.Router) {
				r.Get("/", mcpHandler.InitiateOAuth())
			})

			r.Route("/{type}/callback", func(r chi.Router) {
				r.Get("/", mcpHandler.OAuthCallback())
			})

		r.Route("/{id}", func(r chi.Router) {
			r.Put("/", mcpHandler.Update())
			r.Delete("/", mcpHandler.Delete())

			r.Route("/tools", func(r chi.Router) {
				r.Get("/", mcpHandler.ListTools())
				r.Post("/{toolName}", mcpHandler.CallTool())
			})
		})
		})
	})

	return r
}
