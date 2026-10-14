package executor

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/errdefs"
)

// fakePlanContainersClient lists canned containers and records what was removed.
type fakePlanContainersClient struct {
	listed    []container.Summary
	listErr   error
	removeErr map[string]error

	listOpts []container.ListOptions
	removed  []string
}

func (f *fakePlanContainersClient) ContainerList(_ context.Context, opts container.ListOptions) ([]container.Summary, error) {
	f.listOpts = append(f.listOpts, opts)
	return f.listed, f.listErr
}

func (f *fakePlanContainersClient) ContainerRemove(_ context.Context, target string, opts container.RemoveOptions) error {
	if !opts.Force {
		return errors.New("remove without force")
	}
	f.removed = append(f.removed, target)
	return f.removeErr[target]
}

func TestAgentContainerLabels(t *testing.T) {
	t.Parallel()

	got := agentContainerLabels(" plan-1 ")
	if got[containerLabelAgent] != "true" {
		t.Errorf("agent label = %q, want true", got[containerLabelAgent])
	}
	if got[containerLabelPlan] != "plan-1" {
		t.Errorf("plan label = %q, want plan-1", got[containerLabelPlan])
	}

	// A run outside any plan carries no plan label, so no cancel can match it.
	if _, ok := agentContainerLabels("")[containerLabelPlan]; ok {
		t.Error("plan label set for an empty plan id")
	}
}

func TestRemovePlanContainers(t *testing.T) {
	t.Parallel()

	cli := &fakePlanContainersClient{
		listed: []container.Summary{{ID: "aaa"}, {ID: "bbb"}},
		removeErr: map[string]error{
			"gone":   errdefs.NotFound(errors.New("no such container")),
			"broken": errors.New("daemon said no"),
		},
	}

	removed, err := removePlanContainers(context.Background(), cli, RemovePlanContainersRequest{
		PlanID:  "plan-1",
		Targets: []string{"bbb", " ", "nib-12345678", "gone", "broken"},
	})

	if len(cli.listOpts) != 1 {
		t.Fatalf("ContainerList calls = %d, want 1", len(cli.listOpts))
	}
	opts := cli.listOpts[0]
	if !opts.All {
		t.Error("ContainerList All = false, want true: exited containers have to go too")
	}
	if got := opts.Filters.Get("label"); !slices.Equal(got, []string{"io.nib.plan=plan-1"}) {
		t.Errorf("label filter = %v, want [io.nib.plan=plan-1]", got)
	}

	wantRemoved := []string{"aaa", "bbb", "nib-12345678", "gone", "broken"}
	if !slices.Equal(cli.removed, wantRemoved) {
		t.Errorf("removed = %v, want %v", cli.removed, wantRemoved)
	}
	// "gone" was already gone and "broken" failed; neither counts.
	if removed != 3 {
		t.Errorf("removed count = %d, want 3", removed)
	}
	if err == nil {
		t.Fatal("error = nil, want the failure on broken")
	}
}

func TestRemovePlanContainers_listFailure(t *testing.T) {
	t.Parallel()

	cli := &fakePlanContainersClient{listErr: errors.New("daemon down")}
	removed, err := removePlanContainers(context.Background(), cli, RemovePlanContainersRequest{
		PlanID:  "plan-1",
		Targets: []string{"nib-12345678"},
	})
	if err == nil {
		t.Fatal("error = nil, want the list failure")
	}
	if removed != 0 || len(cli.removed) != 0 {
		t.Fatalf("removed %d (%v) after a failed list, want nothing", removed, cli.removed)
	}
}

func TestRemovePlanContainers_requiresAPlan(t *testing.T) {
	t.Parallel()

	svc := NewService(NewConfigStore(t.TempDir(), "", ""), Secrets{})
	if _, err := svc.RemovePlanContainers(context.Background(), RemovePlanContainersRequest{}); !errors.Is(err, ErrPlanIDRequired) {
		t.Fatalf("error = %v, want ErrPlanIDRequired", err)
	}
}

func TestRemovePlanContainers_onlyOnLocal(t *testing.T) {
	t.Parallel()

	// A disabled executor never reaches Docker: there is no daemon in the test
	// container, so reaching it would fail rather than return zero.
	svc := NewService(NewConfigStore(t.TempDir(), "", ""), Secrets{})
	removed, err := svc.RemovePlanContainers(context.Background(), RemovePlanContainersRequest{PlanID: "plan-1"})
	if err != nil || removed != 0 {
		t.Fatalf("RemovePlanContainers() = %d, %v; want 0, nil", removed, err)
	}
}
