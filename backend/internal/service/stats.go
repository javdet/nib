package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/mode"
	"github.com/javdet/nib/internal/repository"
)

// planStatusSnapshotTTL caches the by-status counts for a moment.
//
// PlanStatusCounts is one query plus one plan_state file read per plan, which is
// exactly why the Prometheus refresher polls it on an interval instead of
// computing it per scrape. The stats endpoint pays the same cost, so it gets the
// same treatment: stale by up to half a minute is fine for a dashboard, and it
// stops a held-down refresh from re-reading every plan file.
const planStatusSnapshotTTL = 30 * time.Second

// topPlansLimit and modelLimit bound two group-bys whose cardinality nib does
// not control -- model is free text from the provider, and plans grow forever.
const (
	topPlansLimit = 10
	modelLimit    = 20
)

// PlanStatusSnapshotter is the current by-status count. Implemented by
// ChatService; declared here so StatsService depends on the behaviour rather
// than on all of ChatService.
type PlanStatusSnapshotter interface {
	PlanStatusCounts(ctx context.Context) (map[string]int, error)
}

// StatsService answers the statistics dashboard.
//
// It joins a repository with the file-backed plan status snapshot, which is the
// reason it exists at all: the HTTP layer takes services and never a
// repository, so a join of two collaborators belongs in a service.
type StatsService struct {
	repo  repository.StatsReader
	plans PlanStatusSnapshotter

	snapshotMu    sync.RWMutex
	snapshot      map[string]int
	snapshotAt    time.Time
	snapshotUntil time.Time
}

func NewStatsService(repo repository.StatsReader, plans PlanStatusSnapshotter) *StatsService {
	return &StatsService{repo: repo, plans: plans}
}

// Report is the whole dashboard in one response. It is a single endpoint
// because the page loads as a unit and the frontend has no query cache that
// would make several requests cheap.
type Report struct {
	Range     ReportRange   `json:"range"`
	Plans     PlanStats     `json:"plans"`
	Usage     UsageStats    `json:"usage"`
	AgentRuns AgentRunStats `json:"agentRuns"`
}

type ReportRange struct {
	From   time.Time `json:"from"`
	To     time.Time `json:"to"`
	Bucket string    `json:"bucket"`
}

type PlanStats struct {
	Total          int `json:"total"`
	CreatedInRange int `json:"createdInRange"`
	// AsOf marks CurrentByStatus as a snapshot of now, not of the range: plan
	// status has no history before the transitions table, so the breakdown
	// cannot honour from/to. The UI must label it or it contradicts the picker.
	AsOf             time.Time          `json:"asOf"`
	CurrentByStatus  []KeyCount         `json:"currentByStatus"`
	ByMode           []KeyCount         `json:"byMode"`
	CreatedSeries    []CountPoint       `json:"createdSeries"`
	TransitionSeries []StatusCountPoint `json:"transitionSeries"`
}

type UsageStats struct {
	Totals      UsageTotalsDTO `json:"totals"`
	Series      []UsagePoint   `json:"series"`
	ByModel     []UsageKeyDTO  `json:"byModel"`
	ByMode      []UsageKeyDTO  `json:"byMode"`
	ByOperation []UsageKeyDTO  `json:"byOperation"`
	TopPlans    []PlanUsageDTO `json:"topPlans"`
}

type AgentRunStats struct {
	Totals   AgentRunTotalsDTO `json:"totals"`
	ByStatus []KeyCount        `json:"byStatus"`
	Series   []AgentRunPoint   `json:"series"`
}

type KeyCount struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

type CountPoint struct {
	TS    time.Time `json:"ts"`
	Count int       `json:"count"`
}

type StatusCountPoint struct {
	TS     time.Time `json:"ts"`
	Status string    `json:"status"`
	Count  int       `json:"count"`
}

// UsageTotalsDTO carries cost as a pointer all the way to the JSON: null means
// no call in the range was priced, which the UI renders differently from $0.
type UsageTotalsDTO struct {
	Calls            int      `json:"calls"`
	CostedCalls      int      `json:"costedCalls"`
	FailedCalls      int      `json:"failedCalls"`
	PromptTokens     int      `json:"promptTokens"`
	CachedTokens     int      `json:"cachedPromptTokens"`
	CompletionTokens int      `json:"completionTokens"`
	ReasoningTokens  int      `json:"reasoningTokens"`
	TotalTokens      int      `json:"totalTokens"`
	CostUSD          *float64 `json:"costUsd"`
	AvgDurationMS    int      `json:"avgDurationMs"`
}

type UsagePoint struct {
	TS               time.Time `json:"ts"`
	Calls            int       `json:"calls"`
	CostedCalls      int       `json:"costedCalls"`
	PromptTokens     int       `json:"promptTokens"`
	CachedTokens     int       `json:"cachedPromptTokens"`
	CompletionTokens int       `json:"completionTokens"`
	ReasoningTokens  int       `json:"reasoningTokens"`
	TotalTokens      int       `json:"totalTokens"`
	CostUSD          *float64  `json:"costUsd"`
}

type UsageKeyDTO struct {
	Key         string   `json:"key"`
	Calls       int      `json:"calls"`
	TotalTokens int      `json:"totalTokens"`
	CostUSD     *float64 `json:"costUsd"`
}

type PlanUsageDTO struct {
	PlanID      string   `json:"planId"`
	Title       string   `json:"title"`
	Calls       int      `json:"calls"`
	TotalTokens int      `json:"totalTokens"`
	CostUSD     *float64 `json:"costUsd"`
}

type AgentRunTotalsDTO struct {
	Runs          int      `json:"runs"`
	CostedRuns    int      `json:"costedRuns"`
	Turns         int      `json:"turns"`
	InputTokens   int      `json:"inputTokens"`
	OutputTokens  int      `json:"outputTokens"`
	CostUSD       *float64 `json:"costUsd"`
	AvgDurationMS int      `json:"avgDurationMs"`
}

type AgentRunPoint struct {
	TS      time.Time `json:"ts"`
	Runs    int       `json:"runs"`
	CostUSD *float64  `json:"costUsd"`
}

// Report runs the dashboard queries for one validated range.
//
// Sequentially, on purpose: these are indexed range scans worth single-digit
// milliseconds each, and an errgroup would add a dependency and a partial-failure
// mode to save nothing. The slow part is the plan status snapshot, and that is
// what the TTL cache is for.
func (s *StatsService) Report(ctx context.Context, rg domain.StatsRange) (Report, error) {
	rep := Report{Range: ReportRange{From: rg.From, To: rg.To, Bucket: rg.Bucket}}

	plans, err := s.planSection(ctx, rg)
	if err != nil {
		return Report{}, err
	}
	rep.Plans = plans

	usage, err := s.usageSection(ctx, rg)
	if err != nil {
		return Report{}, err
	}
	rep.Usage = usage

	runs, err := s.agentRunSection(ctx, rg)
	if err != nil {
		return Report{}, err
	}
	rep.AgentRuns = runs

	return rep, nil
}

func (s *StatsService) planSection(ctx context.Context, rg domain.StatsRange) (PlanStats, error) {
	total, err := s.repo.PlansTotal(ctx)
	if err != nil {
		return PlanStats{}, fmt.Errorf("stats report: %w", err)
	}
	created, err := s.repo.PlansCreatedInRange(ctx, rg)
	if err != nil {
		return PlanStats{}, fmt.Errorf("stats report: %w", err)
	}
	series, err := s.repo.PlansCreatedSeries(ctx, rg)
	if err != nil {
		return PlanStats{}, fmt.Errorf("stats report: %w", err)
	}
	byMode, err := s.repo.PlansByMode(ctx)
	if err != nil {
		return PlanStats{}, fmt.Errorf("stats report: %w", err)
	}
	transitions, err := s.repo.StatusTransitionSeries(ctx, rg)
	if err != nil {
		return PlanStats{}, fmt.Errorf("stats report: %w", err)
	}

	byStatus, asOf := s.planStatusSnapshot(ctx)

	out := PlanStats{
		Total:           total,
		CreatedInRange:  created,
		AsOf:            asOf,
		CurrentByStatus: byStatus,
		ByMode:          toKeyCounts(byMode),
		CreatedSeries:   make([]CountPoint, 0, len(series)),
	}
	for _, b := range series {
		out.CreatedSeries = append(out.CreatedSeries, CountPoint{TS: b.TS, Count: b.Count})
	}
	out.TransitionSeries = make([]StatusCountPoint, 0, len(transitions))
	for _, b := range transitions {
		out.TransitionSeries = append(out.TransitionSeries,
			StatusCountPoint{TS: b.TS, Status: b.Status, Count: b.Count})
	}
	return out, nil
}

// planStatusSnapshot returns the cached by-status counts, refreshing when stale.
//
// A failure is logged and returns whatever is cached rather than failing the
// whole dashboard: the same reasoning as the metrics refresher keeping its last
// good gauges, since a transient error should not read as every plan vanishing.
func (s *StatsService) planStatusSnapshot(ctx context.Context) ([]KeyCount, time.Time) {
	if s.plans == nil {
		return nil, time.Time{}
	}

	now := time.Now()

	s.snapshotMu.RLock()
	if now.Before(s.snapshotUntil) && s.snapshot != nil {
		counts, at := s.snapshot, s.snapshotAt
		s.snapshotMu.RUnlock()
		return statusCounts(counts), at
	}
	s.snapshotMu.RUnlock()

	counts, err := s.plans.PlanStatusCounts(ctx)
	if err != nil {
		slog.Warn("stats: plan status snapshot", "error", err)
		s.snapshotMu.RLock()
		defer s.snapshotMu.RUnlock()
		return statusCounts(s.snapshot), s.snapshotAt
	}

	s.snapshotMu.Lock()
	s.snapshot, s.snapshotAt = counts, now
	s.snapshotUntil = now.Add(planStatusSnapshotTTL)
	s.snapshotMu.Unlock()

	return statusCounts(counts), now
}

// statusCounts renders the map in the canonical status order, including the
// zeroes: a status missing from a donut reads as "not a thing", not as "none".
func statusCounts(counts map[string]int) []KeyCount {
	if counts == nil {
		return nil
	}
	out := make([]KeyCount, 0, len(actionPlanStatuses))
	for _, st := range actionPlanStatuses {
		out = append(out, KeyCount{Key: string(st), Count: counts[string(st)]})
	}
	return out
}

func (s *StatsService) usageSection(ctx context.Context, rg domain.StatsRange) (UsageStats, error) {
	totals, err := s.repo.UsageTotals(ctx, rg)
	if err != nil {
		return UsageStats{}, fmt.Errorf("stats report: %w", err)
	}
	series, err := s.repo.UsageSeries(ctx, rg)
	if err != nil {
		return UsageStats{}, fmt.Errorf("stats report: %w", err)
	}
	byModel, err := s.repo.UsageByColumn(ctx, rg, "model", modelLimit)
	if err != nil {
		return UsageStats{}, fmt.Errorf("stats report: %w", err)
	}
	byMode, err := s.repo.UsageByColumn(ctx, rg, "mode", len(mode.Modes)+1)
	if err != nil {
		return UsageStats{}, fmt.Errorf("stats report: %w", err)
	}
	byOperation, err := s.repo.UsageByColumn(ctx, rg, "operation", 8)
	if err != nil {
		return UsageStats{}, fmt.Errorf("stats report: %w", err)
	}
	topPlans, err := s.repo.TopPlansByUsage(ctx, rg, topPlansLimit)
	if err != nil {
		return UsageStats{}, fmt.Errorf("stats report: %w", err)
	}

	out := UsageStats{
		Totals: UsageTotalsDTO{
			Calls: totals.Calls, CostedCalls: totals.CostedCalls, FailedCalls: totals.FailedCalls,
			PromptTokens: totals.PromptTokens, CachedTokens: totals.CachedTokens,
			CompletionTokens: totals.OutputTokens, ReasoningTokens: totals.ReasonTokens,
			TotalTokens: totals.TotalTokens, CostUSD: totals.CostUSD,
			AvgDurationMS: totals.AvgDurationMS,
		},
		Series:      make([]UsagePoint, 0, len(series)),
		ByModel:     foldUsageTail(byModel, modelLimit, totals),
		ByMode:      toUsageKeys(byMode),
		ByOperation: toUsageKeys(byOperation),
		TopPlans:    make([]PlanUsageDTO, 0, len(topPlans)),
	}
	for _, b := range series {
		out.Series = append(out.Series, UsagePoint{
			TS: b.TS, Calls: b.Calls, CostedCalls: b.CostedCalls,
			PromptTokens: b.PromptTokens, CachedTokens: b.CachedTokens,
			CompletionTokens: b.CompletionTokens, ReasoningTokens: b.ReasoningTokens,
			TotalTokens: b.TotalTokens, CostUSD: b.CostUSD,
		})
	}
	for _, p := range topPlans {
		out.TopPlans = append(out.TopPlans, PlanUsageDTO{
			PlanID: p.PlanID.String(), Title: p.Title, Calls: p.Calls,
			TotalTokens: p.TotalTokens, CostUSD: p.CostUSD,
		})
	}
	return out, nil
}

func (s *StatsService) agentRunSection(ctx context.Context, rg domain.StatsRange) (AgentRunStats, error) {
	totals, err := s.repo.AgentRunTotals(ctx, rg)
	if err != nil {
		return AgentRunStats{}, fmt.Errorf("stats report: %w", err)
	}
	byStatus, err := s.repo.AgentRunsByStatus(ctx, rg)
	if err != nil {
		return AgentRunStats{}, fmt.Errorf("stats report: %w", err)
	}
	series, err := s.repo.AgentRunSeries(ctx, rg)
	if err != nil {
		return AgentRunStats{}, fmt.Errorf("stats report: %w", err)
	}

	out := AgentRunStats{
		Totals: AgentRunTotalsDTO{
			Runs: totals.Runs, CostedRuns: totals.CostedRuns, Turns: totals.Turns,
			InputTokens: totals.InputTokens, OutputTokens: totals.OutputTokens,
			CostUSD: totals.CostUSD, AvgDurationMS: totals.AvgDurationMS,
		},
		ByStatus: toKeyCounts(byStatus),
		Series:   make([]AgentRunPoint, 0, len(series)),
	}
	for _, b := range series {
		out.Series = append(out.Series, AgentRunPoint{TS: b.TS, Runs: b.Runs, CostUSD: b.CostUSD})
	}
	return out, nil
}

// foldUsageTail accounts for what the LIMIT cut off.
//
// model is free text chosen by the provider, so the query has to bound its
// cardinality -- but silently dropping the tail would make the breakdown add up
// to less than the totals and look like a bug. Cost is deliberately not folded:
// the tail's cost is not known from the totals alone.
func foldUsageTail(rows []domain.UsageByKey, limit int, totals domain.UsageTotals) []UsageKeyDTO {
	out := toUsageKeys(rows)
	if len(rows) < limit {
		return out
	}

	var calls, tokens int
	for _, r := range rows {
		calls += r.Calls
		tokens += r.TotalTokens
	}
	if rest := totals.Calls - calls; rest > 0 {
		out = append(out, UsageKeyDTO{
			Key:         "other",
			Calls:       rest,
			TotalTokens: max(totals.TotalTokens-tokens, 0),
		})
	}
	return out
}

func toUsageKeys(rows []domain.UsageByKey) []UsageKeyDTO {
	out := make([]UsageKeyDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, UsageKeyDTO{
			Key: r.Key, Calls: r.Calls, TotalTokens: r.TotalTokens, CostUSD: r.CostUSD,
		})
	}
	return out
}

func toKeyCounts(rows []domain.CountByKey) []KeyCount {
	out := make([]KeyCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, KeyCount{Key: r.Key, Count: r.Count})
	}
	return out
}
