package service

import (
	"context"
	"fmt"
)

// PlanStatusCounts counts plans by lifecycle status, for the metrics refresher.
//
// Status is not in Postgres: it lives in data/plan_state/{dialogID}.json and
// defaults to draft when the file is absent. So this is one query plus one file
// read per plan, which is why the refresher polls it on an interval instead of
// computing it on every scrape. Reusing ReadPlanStates keeps the draft default
// and the status validation identical to what the dialog list shows.
func (s *ChatService) PlanStatusCounts(ctx context.Context) (map[string]int, error) {
	ids, err := s.dialogRepo.ListPlanDialogIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("plan status counts: %w", err)
	}

	counts := make(map[string]int, len(actionPlanStatuses))
	for _, state := range s.ReadPlanStates(ids) {
		counts[string(state.Status)]++
	}
	return counts, nil
}
