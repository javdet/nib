package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/javdet/nib/internal/includedtools"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/executor"
	"github.com/javdet/nib/internal/llm"
	"github.com/javdet/nib/internal/mcpclient"
	"github.com/javdet/nib/internal/mcpconfig"
	"github.com/javdet/nib/internal/mode"
	"github.com/javdet/nib/internal/repository"
	"github.com/javdet/nib/internal/rules"
	"github.com/javdet/nib/internal/skills"
	"github.com/javdet/nib/internal/systemprompts"
	"github.com/javdet/nib/internal/toolcatalog"
	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultSystemPrompt       = "You are a helpful assistant."
	defaultMaxIterations        = 10
	mcpDiscoveryCacheTTL        = 60 * time.Second
	toolCategoryNamesCacheTTL   = 60 * time.Second
)

type localToolHandler func(ctx context.Context, args map[string]any) (string, error)

type toolCategoryLister interface {
	List(ctx context.Context) ([]toolcatalog.CategoryWithPatterns, error)
	ListToolsByCategory(ctx context.Context, name string) ([]toolcatalog.CatalogTool, error)
}

type toolRoute struct {
	connID    uuid.UUID
	serverURL string
	headers   map[string]string
	// secrets holds the values expanded from ${NAME} references in mcp.json.
	// Transport errors embed the request URL, so they are stripped before the
	// error reaches a log line or the model.
	secrets []string
}

// redactRouteError hides values expanded from mcp.json variables in err.
func redactRouteError(route toolRoute, err error) error {
	if err == nil || len(route.secrets) == 0 {
		return err
	}
	msg := mcpconfig.RedactValues(err.Error(), route.secrets)
	if msg == err.Error() {
		return err
	}
	return errors.New(msg)
}

type toolCatalog struct {
	tools         []llm.ToolDef
	mcpRoutes     map[string]toolRoute
	localHandlers map[string]localToolHandler
}

func newToolCatalog() *toolCatalog {
	return &toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}
}

// addMCPTool registers a routable MCP tool. It reports false when the name is
// already served by a local handler or another route.
func (c *toolCatalog) addMCPTool(def llm.ToolDef, route toolRoute) bool {
	if _, exists := c.localHandlers[def.Name]; exists {
		return false
	}
	if _, exists := c.mcpRoutes[def.Name]; exists {
		return false
	}
	c.mcpRoutes[def.Name] = route
	c.tools = append(c.tools, def)
	return true
}

// ChatService delegates user messages to an LLM provider and returns responses.
type ChatService struct {
	provider         llm.Provider
	systemPromptsSvc *systemprompts.Service
	variableRepo     repository.VariableRepository
	selection        *SelectionStore
	dialogRepo       repository.DialogRepository
	attachmentRepo   repository.AttachmentRepository
	mcpSvc           *MCPService
	mcpConfigSvc     *mcpconfig.Service
	includedToolsSvc *includedtools.Service
	knowledgeSvc     *KnowledgeService
	toolSearchSvc    *ToolSearchService
	toolCategorySvc  toolCategoryLister
	secretSvc        *SecretService
	executorSvc      *executor.Service
	skillsSvc        *skills.Service
	rulesSvc         *rules.Service
	llmBaseURL       string
	allowToolsDir    string
	dagsDir          string
	summariesDir     string
	actionPlansDir   string
	planContractsDir string
	planFanoutDir    string
	planStateDir     string
	subagentsDir     string
	attachmentsDir   string
	maxIterations    int
	fanout           PlanFanoutConfig
	actionExec       ActionExecConfig
	activity         *ActivityBroker

	mcpDiscoveryMu    sync.RWMutex
	mcpDiscoveryCache []mcpDiscoveryResult
	mcpDiscoveryUntil time.Time

	toolCategoryNamesMu    sync.RWMutex
	toolCategoryNamesCache []string
	toolCategoryNamesUntil time.Time

	// planWriteMu serialises the read-modify-write of an action plan file,
	// keyed by the dialog that owns it. Stage subagents write the same plan
	// concurrently, and atomicfile only makes each individual write atomic.
	planWriteMu sync.Map

	// fanoutWriteMu guards the fan-out run record, which stage subagents append
	// blockers to while the runner updates their status.
	fanoutWriteMu sync.Map

	// fanoutCancels holds the cancel func of each in-flight fan-out, keyed by the
	// dialog it plans, so an operator can stop a run that is going nowhere.
	fanoutCancels sync.Map

	// transcriptWriteMu serialises appends to one dialog's transcript, keyed by
	// that dialog. An agent round writes its assistant row and the tool results
	// answering it as separate inserts, and anything appended between the two
	// breaks the pairing every later replay depends on.
	transcriptWriteMu sync.Map

	// execLeaseMu guards execLease. The lease is the claim that keeps a second
	// agent off live infrastructure, so it is taken before any work starts and
	// released by whatever ends it -- the sub-agent goroutine, the agent-runner
	// webhook, or a force stop.
	execLeaseMu sync.Mutex
	// execLease is the one execution allowed at a time, across every plan, or
	// nil when nothing is running. See ExecutionLease for why it is one.
	execLease *ExecutionLease

	// subagentClaims holds a mutex per orchestrator subagent, keyed by root
	// dialog and subagent name, so two concurrent turns on one plan cannot run
	// the same subagent twice over the same transcript.
	subagentClaims sync.Map
}

func NewChatService(
	provider llm.Provider,
	systemPromptsSvc *systemprompts.Service,
	variableRepo repository.VariableRepository,
	selection *SelectionStore,
	dialogRepo repository.DialogRepository,
	attachmentRepo repository.AttachmentRepository,
	mcpSvc *MCPService,
	mcpConfigSvc *mcpconfig.Service,
	includedToolsSvc *includedtools.Service,
	knowledgeSvc *KnowledgeService,
	toolSearchSvc *ToolSearchService,
	toolCategorySvc toolCategoryLister,
	secretSvc *SecretService,
	executorSvc *executor.Service,
	skillsSvc *skills.Service,
	rulesSvc *rules.Service,
	llmBaseURL string,
	dataDir string,
	allowToolsDir string,
	maxIterations int,
	fanout PlanFanoutConfig,
	actionExec ActionExecConfig,
) *ChatService {
	if maxIterations <= 0 {
		maxIterations = defaultMaxIterations
	}
	fanout = fanout.withDefaults()
	actionExec = actionExec.withDefaults()
	return &ChatService{
		provider:         provider,
		systemPromptsSvc: systemPromptsSvc,
		variableRepo:     variableRepo,
		selection:        selection,
		dialogRepo:       dialogRepo,
		attachmentRepo:   attachmentRepo,
		mcpSvc:           mcpSvc,
		mcpConfigSvc:     mcpConfigSvc,
		includedToolsSvc: includedToolsSvc,
		knowledgeSvc:     knowledgeSvc,
		toolSearchSvc:    toolSearchSvc,
		toolCategorySvc:  toolCategorySvc,
		secretSvc:        secretSvc,
		executorSvc:      executorSvc,
		skillsSvc:        skillsSvc,
		rulesSvc:         rulesSvc,
		llmBaseURL:       strings.TrimSpace(llmBaseURL),
		allowToolsDir:    mode.ResolveDir(dataDir, allowToolsDir),
		dagsDir:          filepath.Join(dataDir, "dags"),
		summariesDir:     filepath.Join(dataDir, "summaries"),
		actionPlansDir:   filepath.Join(dataDir, "action_plans"),
		planContractsDir: filepath.Join(dataDir, "plan_contracts"),
		planFanoutDir:    filepath.Join(dataDir, "plan_fanout"),
		planStateDir:     filepath.Join(dataDir, "plan_state"),
		subagentsDir:     filepath.Join(dataDir, "subagents"),
		attachmentsDir:   filepath.Join(dataDir, "attachments"),
		maxIterations:    maxIterations,
		fanout:           fanout,
		actionExec:       actionExec,
		activity:         NewActivityBroker(),
	}
}

func (s *ChatService) SubscribeActivity(dialogID uuid.UUID) (<-chan domain.AgentActivity, func()) {
	return s.activity.Subscribe(dialogID)
}

func (s *ChatService) Send(ctx context.Context, message, modeName string) (domain.ChatResponse, error) {
	sysPrompt, err := s.resolveSystemPrompt(ctx, modeName)
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("chat send: resolve system prompt: %w", err)
	}

	catalog := newToolCatalog()
	if mode.IsValid(modeName) {
		allow, err := s.resolveAllowSet(modeName)
		if err != nil {
			return domain.ChatResponse{}, fmt.Errorf("chat send: load allow-tools: %w", err)
		}
		catalog, err = s.buildToolCatalog(ctx, allow, newToolBinding(uuid.Nil))
		if err != nil {
			return domain.ChatResponse{}, fmt.Errorf("chat send: %w", err)
		}
	}

	slog.Info("chat send", "mode", modeName)

	text, err := s.runAgentLoop(ctx, sysPrompt, message, modeName, catalog)
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("chat send: %w", err)
	}
	return domain.ChatResponse{Response: text}, nil
}

func (s *ChatService) resolveSystemPrompt(ctx context.Context, modeName string) (string, error) {
	// modeName arrives unvalidated from POST /chat and from stored dialogs; only
	// the predefined modes have a system prompt compiled into the binary.
	content := defaultSystemPrompt
	if mode.IsValid(modeName) && s.systemPromptsSvc != nil {
		p, err := s.systemPromptsSvc.Get(modeName)
		if err != nil {
			return "", fmt.Errorf("resolve system prompt %q: %w", modeName, err)
		}
		content = p.Content
	}

	rendered, err := RenderTemplateVariables(ctx, content, s.variableRepo, s.selection)
	if err != nil {
		return "", err
	}

	if modeName == "discuss" && s.skillsSvc != nil {
		section, err := s.buildIncludedSkillsSection()
		if err != nil {
			return "", fmt.Errorf("build included skills section: %w", err)
		}
		if section != "" {
			rendered = rendered + "\n\n" + section
		}
	}

	return rendered, nil
}

// resolveAllowSet returns the union of system tools (from data/tools/{mode}.json)
// and user-selected MCP tools (from mcp-included.json) for the given mode.
func (s *ChatService) resolveAllowSet(modeName string) (map[string]struct{}, error) {
	systemAllow, err := mode.LoadAllowList(s.allowToolsDir, modeName)
	if err != nil {
		return nil, err
	}

	out := make(map[string]struct{})
	for name := range systemAllow {
		out[name] = struct{}{}
	}

	if s.includedToolsSvc != nil {
		mcpTools, err := s.includedToolsSvc.Get(modeName)
		if err != nil {
			return nil, err
		}
		for _, name := range mcpTools {
			out[name] = struct{}{}
		}
	}

	return out, nil
}

// resolveDialogAllowSet returns the mode allow set plus, for plan dialogs, every
// tool belonging to the categories chosen during decomposition.
func (s *ChatService) resolveDialogAllowSet(ctx context.Context, d domain.Dialog) (map[string]struct{}, error) {
	allow, err := s.resolveAllowSet(d.Mode)
	if err != nil {
		return nil, err
	}
	if d.Mode != "plan" || s.toolCategorySvc == nil {
		return allow, nil
	}

	for _, name := range s.planDialogCategories(ctx, d) {
		tools, err := s.toolCategorySvc.ListToolsByCategory(ctx, name)
		if err != nil {
			slog.Warn("plan allow set: list tools by category", "category", name, "error", err)
			continue
		}
		for _, t := range tools {
			allow[t.Name] = struct{}{}
		}
	}
	return allow, nil
}

func (s *ChatService) planDialogCategories(ctx context.Context, d domain.Dialog) []string {
	if len(d.Categories) > 0 {
		return d.Categories
	}
	if d.ParentID == nil || s.dialogRepo == nil {
		return nil
	}
	// The categories are chosen once, during decomposition, and stored on the
	// dialog that owns the plan -- which is the root of the lineage, not
	// necessarily this dialog's immediate parent.
	rootID, err := s.resolveRootDialogID(ctx, d.ID)
	if err != nil {
		slog.Warn("plan allow set: resolve plan root", "dialog_id", d.ID, "error", err)
		return nil
	}
	root, err := s.dialogRepo.GetDialog(ctx, rootID)
	if err != nil {
		slog.Warn("plan allow set: load plan root categories", "root_id", rootID, "error", err)
		return nil
	}
	return root.Categories
}

// dialogToolBinding is the binding for a turn in d: the transcript is d's own,
// while every plan artifact belongs to the root of its lineage.
//
// They are the same dialog for a root turn and differ when an operator types
// into a subagent's transcript, which would otherwise start a second plan under
// the subagent's id. It is also what keeps the orchestrator's own tools off a
// subagent transcript: they are registered only when the two match.
func (s *ChatService) dialogToolBinding(ctx context.Context, d domain.Dialog) toolBinding {
	if d.ParentID == nil || s.dialogRepo == nil {
		return newToolBinding(d.ID)
	}
	rootID, err := s.resolveRootDialogID(ctx, d.ID)
	if err != nil {
		slog.Warn("tool binding: resolve plan root", "dialog_id", d.ID, "error", err)
		return newToolBinding(d.ID)
	}
	return toolBinding{dialogID: d.ID, planID: rootID}
}

func (s *ChatService) cachedToolCategoryNames(ctx context.Context) []string {
	if s.toolCategorySvc == nil {
		return nil
	}

	now := time.Now()

	s.toolCategoryNamesMu.RLock()
	if now.Before(s.toolCategoryNamesUntil) && s.toolCategoryNamesCache != nil {
		cached := s.toolCategoryNamesCache
		s.toolCategoryNamesMu.RUnlock()
		return cached
	}
	s.toolCategoryNamesMu.RUnlock()

	cats, err := s.toolCategorySvc.List(ctx)
	if err != nil {
		slog.Warn("tool categories: list for cache", "error", err)
		return nil
	}

	names := make([]string, 0, len(cats))
	for _, cat := range cats {
		name := strings.TrimSpace(cat.Name)
		if name != "" {
			names = append(names, name)
		}
	}

	s.toolCategoryNamesMu.Lock()
	s.toolCategoryNamesCache = names
	s.toolCategoryNamesUntil = now.Add(toolCategoryNamesCacheTTL)
	s.toolCategoryNamesMu.Unlock()

	return names
}

func (s *ChatService) buildToolCatalog(ctx context.Context, allow map[string]struct{}, b toolBinding) (*toolCatalog, error) {
	catalog := newToolCatalog()

	// Resolved per call rather than cached on the service: concurrent stage
	// subagents each build their own catalog, so shared mutable state here
	// would be a data race.
	b.categoryNames = s.cachedToolCategoryNames(ctx)
	s.addLocalTools(catalog, allow, b)

	conns, err := s.mcpSvc.ListConnections(ctx)
	if err != nil {
		return catalog, fmt.Errorf("list mcp connections: %w", err)
	}

	var servers []mcpconfig.Server
	if s.mcpConfigSvc != nil {
		servers, err = s.mcpConfigSvc.ListServers()
		if err != nil {
			return catalog, fmt.Errorf("list mcp.json servers: %w", err)
		}
	}

	// Discover tools from every source concurrently so one slow or unreachable
	// MCP server cannot serialize behind the others and blow the request write
	// deadline. Results are appended in a deterministic order afterwards.
	for _, r := range s.cachedMCPToolDiscovery(ctx, conns, servers) {
		appendMCPTools(catalog, r.tools, allow, r.route, r.routeKey, r.routeValue)
	}

	logMissingAllowTools(allow, *catalog)
	return catalog, nil
}

// mcpDiscoveryResult holds the tools discovered from a single MCP source plus the
// route metadata needed to append them to a catalog.
type mcpDiscoveryResult struct {
	tools      []mcpclient.ToolInfo
	route      toolRoute
	routeKey   string
	routeValue string
}

// discoverMCPTools lists tools from all connected MCP connections and configured
// mcp.json servers concurrently. Results are returned in a deterministic order
// (connections first in listing order, then mcp.json servers) so catalog
// duplicate resolution stays stable. Per-source failures are logged and yield an
// empty tool set instead of aborting discovery.
func (s *ChatService) discoverMCPTools(
	ctx context.Context,
	conns []domain.MCPConnection,
	servers []mcpconfig.Server,
	perSourceTimeout time.Duration,
) []mcpDiscoveryResult {
	if perSourceTimeout <= 0 {
		perSourceTimeout = mcpclient.DefaultListToolsTimeout
	}
	discoveryClient := &http.Client{Timeout: perSourceTimeout}

	results := make([]mcpDiscoveryResult, 0, len(conns)+len(servers))
	for _, conn := range conns {
		if conn.Status != "connected" {
			continue
		}
		results = append(results, mcpDiscoveryResult{
			route:      toolRoute{connID: conn.ID},
			routeKey:   "connectionID",
			routeValue: conn.ID.String(),
		})
	}
	for _, srv := range servers {
		if strings.TrimSpace(srv.URL) == "" {
			continue
		}
		// A server whose ${NAME} references cannot be resolved is dropped so the
		// rest of the catalog stays usable.
		resolved, err := s.mcpConfigSvc.ResolveServer(ctx, srv)
		if err != nil {
			slog.Warn("resolve mcp.json server variables failed", "server", srv.Name, "error", err)
			continue
		}
		results = append(results, mcpDiscoveryResult{
			route: toolRoute{
				serverURL: resolved.URL,
				headers:   cloneHeaderMap(resolved.Headers),
				secrets:   resolved.Values(),
			},
			routeKey:   "mcpServer",
			routeValue: srv.Name,
		})
	}

	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := &results[i]
			srcCtx, cancel := context.WithTimeout(ctx, perSourceTimeout)
			defer cancel()
			if r.routeKey == "connectionID" {
				tools, err := s.mcpSvc.ListTools(srcCtx, r.route.connID)
				if err != nil {
					slog.Warn("list mcp tools failed", "connectionID", r.routeValue, "error", err)
					return
				}
				r.tools = tools
				return
			}
			tools, err := mcpclient.ListToolsFromURLWithHTTPClient(
				srcCtx,
				r.route.serverURL,
				r.route.headers,
				discoveryClient,
			)
			if err != nil {
				slog.Warn("list mcp.json server tools failed",
					"server", r.routeValue, "error", redactRouteError(r.route, err))
				return
			}
			r.tools = tools
		}(i)
	}
	wg.Wait()

	return results
}

func (s *ChatService) cachedMCPToolDiscovery(
	ctx context.Context,
	conns []domain.MCPConnection,
	servers []mcpconfig.Server,
) []mcpDiscoveryResult {
	now := time.Now()

	s.mcpDiscoveryMu.RLock()
	if now.Before(s.mcpDiscoveryUntil) && s.mcpDiscoveryCache != nil {
		cached := s.mcpDiscoveryCache
		s.mcpDiscoveryMu.RUnlock()
		return cached
	}
	s.mcpDiscoveryMu.RUnlock()

	results := s.discoverMCPTools(ctx, conns, servers, 0)

	s.mcpDiscoveryMu.Lock()
	s.mcpDiscoveryCache = results
	s.mcpDiscoveryUntil = now.Add(mcpDiscoveryCacheTTL)
	s.mcpDiscoveryMu.Unlock()

	return results
}

// InvalidateMCPToolDiscoveryCache clears cached MCP tool discovery results.
func (s *ChatService) InvalidateMCPToolDiscoveryCache() {
	s.mcpDiscoveryMu.Lock()
	s.mcpDiscoveryCache = nil
	s.mcpDiscoveryUntil = time.Time{}
	s.mcpDiscoveryMu.Unlock()
}

func appendMCPTools(catalog *toolCatalog, tools []mcpclient.ToolInfo, allow map[string]struct{}, route toolRoute, routeKey, routeValue string) {
	for _, t := range tools {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			continue
		}
		if allow != nil {
			if _, ok := allow[name]; !ok {
				continue
			}
		}
		if _, exists := catalog.localHandlers[name]; exists {
			continue
		}
		if _, exists := catalog.mcpRoutes[name]; exists {
			slog.Warn("duplicate mcp tool name, skipping", "name", name, routeKey, routeValue)
			continue
		}
		catalog.addMCPTool(llm.ToolDef{
			Name:        name,
			Description: t.Description,
			Parameters:  toolParametersJSON(t.InputSchema),
		}, route)
	}
}

func logMissingAllowTools(allow map[string]struct{}, catalog toolCatalog) {
	if allow == nil {
		return
	}
	present := make(map[string]struct{}, len(catalog.tools))
	for _, t := range catalog.tools {
		present[t.Name] = struct{}{}
	}
	for name := range allow {
		if _, ok := present[name]; !ok {
			slog.Warn("allow-tools: name not found among discovered tools", "name", name)
		}
	}
}

func cloneHeaderMap(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		out[k] = v
	}
	return out
}

func localToolAllowed(allow map[string]struct{}, name string) bool {
	if allow == nil {
		return true
	}
	_, ok := allow[name]
	return ok
}

func toolParametersJSON(schema json.RawMessage) json.RawMessage {
	if len(schema) == 0 || string(schema) == "null" {
		return json.RawMessage(`{"type":"object"}`)
	}
	return schema
}

func (s *ChatService) runAgentLoop(ctx context.Context, sysPrompt, userMessage, modeName string, catalog *toolCatalog) (string, error) {
	messages := []llm.Message{
		{Role: "system", Content: sysPrompt},
		{Role: "user", Content: userMessage},
	}

	logCtx := newAgentLogCtx(nil, modeName)
	toolFailures := 0

	for round := 0; round < s.maxIterations; round++ {
		roundNum := round + 1
		roundLog := logCtx.withRound(roundNum)

		logAgentRoundStart(roundNum, len(messages), len(catalog.tools), logCtx)

		logSendingCompletionRequest(roundNum, logCtx)
		asst, err := s.provider.CompleteWithTools(ctx, messages, catalog.tools)
		if err != nil {
			logCompletionError(roundNum, err, logCtx)
			return "", fmt.Errorf("completion round %d: %w", roundNum, err)
		}

		logCompletionParsed(roundNum, asst.ToolCalls, logCtx)

		if len(asst.ToolCalls) == 0 {
			return asst.Content, nil
		}

		if round+1 >= s.maxIterations {
			logMaxIterationsWithPendingTools(s.maxIterations, roundNum, logCtx)
			return "", fmt.Errorf("max iterations (%d) exceeded with pending tool_calls", s.maxIterations)
		}

		logToolBatch(roundNum, asst.ToolCalls, logCtx)

		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   asst.Content,
			ToolCalls: asst.ToolCalls,
			Reasoning: asst.Reasoning,
		})

		for _, tc := range asst.ToolCalls {
			logCallTool(tc.Name, tc.ID, tc.Arguments, roundLog)
			out, err := s.executeToolCall(ctx, catalog, tc)
			if err != nil {
				logCallToolFailed(tc.Name, tc.ID, err, roundLog)
				if isTurnFatalToolError(ctx, err) {
					return "", fmt.Errorf("tool %q (id %s): %w", tc.Name, tc.ID, err)
				}
				out = toolErrorPayload(tc.Name, err)
				toolFailures++
			} else {
				logCallToolResult(tc.ID, out, roundLog)
			}
			messages = append(messages, llm.Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    out,
			})
			if toolFailures > maxToolFailuresPerTurn {
				return "", fmt.Errorf("round %d: %w", roundNum, ErrTooManyToolFailures)
			}
		}
	}
	return "", fmt.Errorf("max iterations (%d) exhausted without final assistant message", s.maxIterations)
}

func (s *ChatService) executeToolCall(ctx context.Context, catalog *toolCatalog, tc llm.ToolCall) (string, error) {
	args, err := parseToolArguments(tc.Arguments)
	if err != nil {
		return "", err
	}

	if handler, ok := catalog.localHandlers[tc.Name]; ok {
		return handler(ctx, args)
	}

	route, ok := catalog.mcpRoutes[tc.Name]
	if !ok {
		def, resolved, found := s.resolveDynamicTool(ctx, tc.Name)
		if !found {
			return "", fmt.Errorf("unknown tool %q", tc.Name)
		}
		catalog.addMCPTool(def, resolved)
		slog.Info("resolved tool from catalog", "name", tc.Name)
		route = resolved
	}

	var raw any
	if route.connID != uuid.Nil {
		raw, err = s.mcpSvc.CallTool(ctx, route.connID, tc.Name, args)
	} else if strings.TrimSpace(route.serverURL) != "" {
		raw, err = mcpclient.CallToolFromURL(ctx, route.serverURL, route.headers, tc.Name, args)
	} else {
		return "", fmt.Errorf("tool %q has no MCP route", tc.Name)
	}
	if err != nil {
		return "", redactRouteError(route, err)
	}
	return stringifyToolResult(raw)
}

func parseToolArguments(argumentsJSON string) (map[string]any, error) {
	s := strings.TrimSpace(argumentsJSON)
	if s == "" {
		return map[string]any{}, nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(s), &args); err != nil {
		return nil, fmt.Errorf("parse tool arguments: %w", err)
	}
	if args == nil {
		return map[string]any{}, nil
	}
	return args, nil
}

func stringifyToolResult(raw any) (string, error) {
	switch v := raw.(type) {
	case *sdkmcp.CallToolResult:
		b, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("marshal tool result: %w", err)
		}
		return string(b), nil
	default:
		b, err := json.Marshal(raw)
		if err != nil {
			return "", fmt.Errorf("marshal tool result: %w", err)
		}
		return string(b), nil
	}
}

// ReadDAG returns the stored mermaid markdown for a dialog, or false if none exists.
func (s *ChatService) ReadDAG(dialogID uuid.UUID) (string, bool, error) {
	path := filepath.Join(s.dagsDir, dialogID.String()+".md")
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return string(b), true, nil
}

// ReadSummary returns the stored plan summary for a dialog, or false if none exists.
func (s *ChatService) ReadSummary(dialogID uuid.UUID) (string, bool, error) {
	path := filepath.Join(s.summariesDir, dialogID.String()+".txt")
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return string(b), true, nil
}
