package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/atomicfile"
)

// CodeFixRun is the record of one ad-hoc code fix: a code change the operator
// asked for in the orchestrator's chat rather than through a row of the plan.
//
// It exists because a fix has no action row to be recorded on, and the
// agent-runner webhook needs something durable to recognise the run by: the
// container outlives the process that started it, so a restart between the
// launch and the callback must not turn the result into an orphan message in a
// chat nobody is watching. It is a file rather than a table for the same reason
// a sub-agent pause is: no migration.
type CodeFixRun struct {
	// Status reuses the action statuses -- running, done, failed, cancelled --
	// because a fix ends in exactly the same ways a code action does.
	Status ActionExecStatus `json:"status"`
	// Repository is what the operator named, before the git base URL was joined
	// onto it.
	Repository string `json:"repository"`
	// Branch is what the agent was told to commit to, which is also how the
	// operator finds the work: an existing branch was continued, a generated one
	// is new.
	Branch string `json:"branch"`
	// JobName, ContainerID and Namespace identify the container, and are the
	// only way to reach it once this process is gone.
	JobName     string `json:"jobName,omitempty"`
	ContainerID string `json:"containerId,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
	PRURL       string `json:"prUrl,omitempty"`
	StartedAt   int64  `json:"startedAt"`
	FinishedAt  int64  `json:"finishedAt,omitempty"`
	Error       string `json:"error,omitempty"`
}

// Active reports whether the container is still expected to report back.
func (r CodeFixRun) Active() bool {
	return r.Status == ActionExecRunning
}

// CodeFixRuns maps a fix's own dialog id to its run. The dialog is the key
// because it is what the container is given as CHAT_ID and what comes back on
// the webhook.
type CodeFixRuns map[string]CodeFixRun

func (s *ChatService) codeFixMutex(rootID uuid.UUID) *sync.Mutex {
	mu, _ := s.subagentClaims.LoadOrStore("codefix|"+rootID.String(), &sync.Mutex{})
	return mu.(*sync.Mutex)
}

func (s *ChatService) codeFixPath(rootID uuid.UUID) string {
	return filepath.Join(s.codeFixesDir, rootID.String()+".json")
}

// ReadCodeFixRuns returns the fixes recorded against a chat. A missing file is
// not an error: most conversations never ask for one.
func (s *ChatService) ReadCodeFixRuns(rootID uuid.UUID) (CodeFixRuns, error) {
	b, err := os.ReadFile(s.codeFixPath(rootID))
	if errors.Is(err, os.ErrNotExist) {
		return CodeFixRuns{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read code fix runs: %w", err)
	}

	var runs CodeFixRuns
	if err := json.Unmarshal(b, &runs); err != nil {
		return nil, fmt.Errorf("unmarshal code fix runs: %w", err)
	}
	if runs == nil {
		return CodeFixRuns{}, nil
	}
	return runs, nil
}

func (s *ChatService) writeCodeFixRuns(rootID uuid.UUID, runs CodeFixRuns) error {
	if runs == nil {
		runs = CodeFixRuns{}
	}
	if err := os.MkdirAll(s.codeFixesDir, 0o755); err != nil {
		return fmt.Errorf("create code_fixes directory: %w", err)
	}
	data, err := json.Marshal(runs)
	if err != nil {
		return fmt.Errorf("marshal code fix runs: %w", err)
	}
	if err := atomicfile.Write(s.codeFixPath(rootID), data); err != nil {
		return fmt.Errorf("write code fix runs: %w", err)
	}
	return nil
}

// recordCodeFixRun stores a fresh fix under its dialog id.
func (s *ChatService) recordCodeFixRun(rootID, dialogID uuid.UUID, run CodeFixRun) error {
	mu := s.codeFixMutex(rootID)
	mu.Lock()
	defer mu.Unlock()

	runs, err := s.ReadCodeFixRuns(rootID)
	if err != nil {
		return err
	}
	runs[dialogID.String()] = run
	return s.writeCodeFixRuns(rootID, runs)
}

// updateCodeFixRun applies mutate to one fix and stores the result, reporting
// whether there was a fix to apply it to.
//
// "Not found" is the ordinary case for a container that is not a fix at all:
// run_executor reports through the same webhook and has no record here.
func (s *ChatService) updateCodeFixRun(
	rootID, dialogID uuid.UUID,
	mutate func(*CodeFixRun),
) (CodeFixRun, bool, error) {
	mu := s.codeFixMutex(rootID)
	mu.Lock()
	defer mu.Unlock()

	runs, err := s.ReadCodeFixRuns(rootID)
	if err != nil {
		return CodeFixRun{}, false, err
	}
	run, ok := runs[dialogID.String()]
	if !ok {
		return CodeFixRun{}, false, nil
	}

	mutate(&run)
	runs[dialogID.String()] = run
	if err := s.writeCodeFixRuns(rootID, runs); err != nil {
		return CodeFixRun{}, false, err
	}
	return run, true, nil
}
