package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

// Labels stamped on every local agent container. The plan label is what lets a
// cancelled plan find its containers: the job name is random, and an exited
// action container is kept on purpose (see runActionLocal), so without it a
// plan's containers would pile up on the host with nothing tying them back.
const (
	containerLabelAgent = "io.nib.agent"
	containerLabelPlan  = "io.nib.plan"
)

func agentContainerLabels(planID string) map[string]string {
	labels := map[string]string{containerLabelAgent: "true"}
	if id := strings.TrimSpace(planID); id != "" {
		labels[containerLabelPlan] = id
	}
	return labels
}

// planContainersClient is the slice of the Docker client the plan cleanup needs.
type planContainersClient interface {
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
	ContainerRemove(ctx context.Context, container string, options container.RemoveOptions) error
}

// RemovePlanContainersRequest names the plan whose agent containers go.
type RemovePlanContainersRequest struct {
	PlanID string
	// Targets are container ids or names the plan's own records hold. They
	// reach containers started before containers carried the plan label.
	Targets []string
}

// RemovePlanContainers force-removes every local agent container of a plan,
// running or exited, and returns how many it removed.
//
// It is a no-op on any executor but local Docker: the setting that asks for it
// is a local one, and a Kubernetes Job is collected by its own TTL. A failure
// on one container does not stop the rest.
func (s *Service) RemovePlanContainers(ctx context.Context, req RemovePlanContainersRequest) (int, error) {
	if strings.TrimSpace(req.PlanID) == "" {
		return 0, ErrPlanIDRequired
	}

	cfg, err := s.config.Get()
	if err != nil {
		return 0, err
	}
	if cfg.Type != TypeLocal {
		return 0, nil
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return 0, fmt.Errorf("create docker client: %w", err)
	}
	defer cli.Close()

	return removePlanContainers(ctx, cli, req)
}

func removePlanContainers(ctx context.Context, cli planContainersClient, req RemovePlanContainersRequest) (int, error) {
	listed, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("label", containerLabelPlan+"="+strings.TrimSpace(req.PlanID))),
	})
	if err != nil {
		return 0, fmt.Errorf("list plan containers: %w", err)
	}

	seen := make(map[string]struct{}, len(listed)+len(req.Targets))
	targets := make([]string, 0, len(listed)+len(req.Targets))
	add := func(target string) {
		target = strings.TrimSpace(target)
		if target == "" {
			return
		}
		if _, ok := seen[target]; ok {
			return
		}
		seen[target] = struct{}{}
		targets = append(targets, target)
	}
	for _, c := range listed {
		add(c.ID)
	}
	for _, t := range req.Targets {
		add(t)
	}

	removed := 0
	var errs []error
	for _, target := range targets {
		// Force kills a running container outright: the operator cancelled the
		// plan, so there is no run left whose outcome a graceful stop would save.
		err := cli.ContainerRemove(ctx, target, container.RemoveOptions{Force: true})
		switch {
		case err == nil:
			removed++
		case client.IsErrNotFound(err):
			// Already gone -- auto-removed, or a recorded target removed earlier.
		default:
			errs = append(errs, fmt.Errorf("remove container %q: %w", target, err))
		}
	}
	return removed, errors.Join(errs...)
}
