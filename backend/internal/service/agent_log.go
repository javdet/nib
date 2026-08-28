package service

import (
	"fmt"
	"log/slog"

	"github.com/javdet/nib/internal/llm"
	"github.com/google/uuid"
)

const logTruncateRunes = 2048

type agentLogCtx struct {
	dialogID *uuid.UUID
	mode     string
	round    int
}

func newAgentLogCtx(dialogID *uuid.UUID, mode string) agentLogCtx {
	return agentLogCtx{dialogID: dialogID, mode: mode}
}

func (c agentLogCtx) withRound(round int) agentLogCtx {
	c.round = round
	return c
}

func (c agentLogCtx) attrs(extra ...any) []any {
	out := make([]any, 0, len(extra)+6)
	if c.dialogID != nil {
		out = append(out, "dialog_id", c.dialogID.String())
	}
	if c.mode != "" {
		out = append(out, "mode", c.mode)
	}
	if c.round > 0 {
		out = append(out, "round", c.round)
	}
	out = append(out, extra...)
	return out
}

func truncateForLog(s string, max int) string {
	if max <= 0 {
		max = logTruncateRunes
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + fmt.Sprintf("…(%d runes truncated)", len(r)-max)
}

func toolNamesFromCalls(calls []llm.ToolCall) []string {
	if len(calls) == 0 {
		return nil
	}
	names := make([]string, len(calls))
	for i, tc := range calls {
		names[i] = tc.Name
	}
	return names
}

func logAgentRoundStart(round, messageCount, toolCount int, ctx agentLogCtx) {
	slog.Info("agent round start", ctx.withRound(round).attrs(
		"message_count", messageCount,
		"available_tool_count", toolCount,
	)...)
}

func logSendingCompletionRequest(round int, ctx agentLogCtx) {
	slog.Info("sending chat completion request", ctx.withRound(round).attrs()...)
}

func logCompletionParsed(round int, calls []llm.ToolCall, ctx agentLogCtx) {
	attrs := []any{
		"has_tool_calls", len(calls) > 0,
		"tool_call_count", len(calls),
	}
	if len(calls) > 0 {
		attrs = append(attrs, "tools", toolNamesFromCalls(calls))
	}
	slog.Info("chat completion parsed", ctx.withRound(round).attrs(attrs...)...)
}

func logCompletionError(round int, err error, ctx agentLogCtx) {
	slog.Error("llm completion failed", ctx.withRound(round).attrs("err", err)...)
}

func logActionPlanReminder(round int, ctx agentLogCtx) {
	slog.Warn("plan turn ended without an action plan; reminding the model to finish it",
		ctx.withRound(round).attrs()...)
}

func logMaxIterationsWithPendingTools(max, round int, ctx agentLogCtx) {
	slog.Error("agent max iterations exceeded with pending tool_calls; not executing tools",
		ctx.withRound(round).attrs("max", max)...)
}

func logToolBatch(round int, calls []llm.ToolCall, ctx agentLogCtx) {
	slog.Info("executing tool batch", ctx.withRound(round).attrs(
		"tool_count", len(calls),
		"tools", toolNamesFromCalls(calls),
	)...)
}

func logCallTool(name, toolCallID, arguments string, ctx agentLogCtx) {
	slog.Info("CallTool", ctx.attrs(
		"name", name,
		"tool_call_id", toolCallID,
		"arguments", truncateForLog(arguments, logTruncateRunes),
	)...)
}

func logCallToolResult(toolCallID, content string, ctx agentLogCtx) {
	slog.Info("CallTool result", ctx.attrs(
		"tool_call_id", toolCallID,
		"content", truncateForLog(content, logTruncateRunes),
	)...)
}

func logCallToolFailed(name, toolCallID string, err error, ctx agentLogCtx) {
	slog.Error("CallTool failed", ctx.attrs(
		"name", name,
		"tool_call_id", toolCallID,
		"err", err,
	)...)
}
