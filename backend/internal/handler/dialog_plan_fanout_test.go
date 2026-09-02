package handler

import (
	"reflect"
	"testing"

	"github.com/javdet/nib/internal/service"
)

func TestStartPlanFanoutPayload_targets(t *testing.T) {
	t.Parallel()

	yes, no := true, false
	for name, tc := range map[string]struct {
		payload startPlanFanoutPayload
		want    service.FanoutTargets
	}{
		// The Process plan button sends no stages and means the whole plan.
		"no body": {
			startPlanFanoutPayload{},
			service.FanoutTargets{AllStages: true, Rollback: true},
		},
		// Named stages are a replan, and the rollback follows whatever they say.
		"named stages": {
			startPlanFanoutPayload{Stages: []string{"Verify repairs"}},
			service.FanoutTargets{Stages: []string{"Verify repairs"}, Rollback: true},
		},
		"stages without the rollback": {
			startPlanFanoutPayload{Stages: []string{"Verify repairs"}, Rollback: &no},
			service.FanoutTargets{Stages: []string{"Verify repairs"}},
		},
		// rollback is a modifier on the stage selection, not a selection of its
		// own: asking for it with no stages named is still the whole plan.
		"the rollback asked for explicitly": {
			startPlanFanoutPayload{Rollback: &yes},
			service.FanoutTargets{AllStages: true, Rollback: true},
		},
		"every stage but no rollback": {
			startPlanFanoutPayload{Rollback: &no},
			service.FanoutTargets{AllStages: true},
		},
	} {
		if got := tc.payload.targets(); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: targets = %#v, want %#v", name, got, tc.want)
		}
	}
}
