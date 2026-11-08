package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/cmdrun"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
)

var (
	ErrCommandRunNotFound   = errors.New("command run not found")
	ErrCommandRunNotRunning = errors.New("the command is not running")
	ErrCommandLogGone       = errors.New("the command's output is no longer kept")
	// ErrCommandAuditUnavailable is the API on a backend wired without the
	// audit repository, which only a test does.
	ErrCommandAuditUnavailable = errors.New("command runs are not recorded on this server")
)

const (
	commandRunListLimit       = 200
	defaultCommandOutputLimit = 64 << 10
	maxCommandOutputLimit     = 256 << 10
)

// CommandRunService is what the plan page reads and cancels commands
// through. It joins the audit rows with the live jobs, which is why the
// handler does not take either directly.
type CommandRunService struct {
	repo repository.CommandRunRepository
	jobs *cmdrun.Manager
}

func NewCommandRunService(repo repository.CommandRunRepository, jobs *cmdrun.Manager) *CommandRunService {
	return &CommandRunService{repo: repo, jobs: jobs}
}

// List returns a plan's runs, newest first, optionally one action row's. A
// running row reports the bytes written so far rather than the zero it was
// inserted with.
func (s *CommandRunService) List(ctx context.Context, planID uuid.UUID, actionKey string) ([]domain.CommandRun, error) {
	if s.repo == nil {
		return nil, ErrCommandAuditUnavailable
	}
	runs, err := s.repo.ListCommandRuns(ctx, planID, strings.TrimSpace(actionKey), commandRunListLimit)
	if err != nil {
		return nil, fmt.Errorf("list command runs: %w", err)
	}
	for i := range runs {
		if runs[i].Status == domain.CommandRunRunning {
			if live, ok := s.live(runs[i].ID); ok {
				runs[i] = live
			}
		}
	}
	if runs == nil {
		runs = []domain.CommandRun{}
	}
	return runs, nil
}

// CommandOutput is one page of a run's output.
type CommandOutput struct {
	Output     string                  `json:"output"`
	Offset     int64                   `json:"offset"`
	NextOffset int64                   `json:"next_offset"`
	TotalBytes int64                   `json:"total_bytes"`
	EOF        bool                    `json:"eof"`
	Status     domain.CommandRunStatus `json:"status"`
	Capped     bool                    `json:"capped"`
}

// Output reads a page of a run's output from offset.
func (s *CommandRunService) Output(ctx context.Context, planID, runID uuid.UUID, offset, limit int64) (CommandOutput, error) {
	run, err := s.run(ctx, planID, runID)
	if err != nil {
		return CommandOutput{}, err
	}
	if limit <= 0 {
		limit = defaultCommandOutputLimit
	}
	limit = min(limit, maxCommandOutputLimit)
	offset = max(offset, 0)

	data, size, err := s.jobs.ReadLog(planID, runID, offset, limit)
	if errors.Is(err, cmdrun.ErrLogGone) {
		return CommandOutput{}, ErrCommandLogGone
	}
	if err != nil {
		return CommandOutput{}, fmt.Errorf("read command output: %w", err)
	}
	next := offset + int64(len(data))
	return CommandOutput{
		Output:     strings.ToValidUTF8(string(data), "\uFFFD"),
		Offset:     offset,
		NextOffset: next,
		TotalBytes: size,
		EOF:        next >= size,
		Status:     run.Status,
		Capped:     run.OutputCapped,
	}, nil
}

// Cancel is the operator stopping a running command. The agent that started
// it sees the job end "cancelled by the operator" and carries on.
func (s *CommandRunService) Cancel(ctx context.Context, planID, runID uuid.UUID) error {
	if _, err := s.run(ctx, planID, runID); err != nil {
		return err
	}
	if err := s.jobs.CancelByID(runID, cmdrun.ReasonOperator); err != nil {
		if errors.Is(err, cmdrun.ErrJobNotRunning) {
			return ErrCommandRunNotRunning
		}
		return fmt.Errorf("cancel command run: %w", err)
	}
	return nil
}

// ReconcileLost closes the rows a previous process left running.
func (s *CommandRunService) ReconcileLost(ctx context.Context) (int64, error) {
	if s.repo == nil {
		return 0, nil
	}
	return s.repo.MarkRunningCommandRunsLost(ctx, "the server restarted while this was running")
}

// run loads one of the plan's runs, preferring the live job's state.
func (s *CommandRunService) run(ctx context.Context, planID, runID uuid.UUID) (domain.CommandRun, error) {
	if s.repo == nil || s.jobs == nil {
		return domain.CommandRun{}, ErrCommandAuditUnavailable
	}
	run, err := s.repo.GetCommandRun(ctx, runID)
	if errors.Is(err, repository.ErrNotFound) {
		return domain.CommandRun{}, ErrCommandRunNotFound
	}
	if err != nil {
		return domain.CommandRun{}, fmt.Errorf("get command run: %w", err)
	}
	// A run of another plan is reported as missing, not as forbidden.
	if run.PlanID != planID {
		return domain.CommandRun{}, ErrCommandRunNotFound
	}
	if live, ok := s.live(runID); ok {
		return live, nil
	}
	return run, nil
}

func (s *CommandRunService) live(id uuid.UUID) (domain.CommandRun, bool) {
	if s.jobs == nil {
		return domain.CommandRun{}, false
	}
	return s.jobs.Running(id)
}
