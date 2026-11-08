package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/atomicfile"
	"github.com/javdet/nib/internal/repository"
)

// runStateTimeout bounds one run-state read or write. The accessors predate the
// store and take no context -- they are called from goroutines that outlive any
// request -- so each call gets a deadline of its own instead of none at all.
const runStateTimeout = 15 * time.Second

// migratedSuffix is appended to a run-state file once its content is in the
// database, which keeps it out of every later glob without deleting it.
const migratedSuffix = ".migrated"

// SetRunStateStore moves run state off DATA_DIR and into store. Without it the
// service keeps the per-dialog JSON files, which is what every service test
// relies on.
func (s *ChatService) SetRunStateStore(store repository.RunStateRepository) {
	s.runState = store
}

func (s *ChatService) runStateStore() repository.RunStateRepository {
	if s.runState != nil {
		return s.runState
	}
	return fileRunState{s: s}
}

// loadRunState decodes the document of kind into v and reports whether there
// was one.
func (s *ChatService) loadRunState(kind repository.RunStateKind, dialogID uuid.UUID, v any) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), runStateTimeout)
	defer cancel()

	b, err := s.runStateStore().GetRunState(ctx, kind, dialogID)
	if errors.Is(err, repository.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return false, fmt.Errorf("unmarshal %s: %w", kind, err)
	}
	return true, nil
}

func (s *ChatService) saveRunState(kind repository.RunStateKind, dialogID uuid.UUID, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", kind, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), runStateTimeout)
	defer cancel()
	return s.runStateStore().PutRunState(ctx, kind, dialogID, data)
}

func (s *ChatService) dropRunState(kind repository.RunStateKind, dialogID uuid.UUID) error {
	ctx, cancel := context.WithTimeout(context.Background(), runStateTimeout)
	defer cancel()
	return s.runStateStore().DeleteRunState(ctx, kind, dialogID)
}

// dropRunStateOrWarn removes the documents of kinds, for a caller with nowhere
// to report a failure to.
func (s *ChatService) dropRunStateOrWarn(dialogID uuid.UUID, kinds ...repository.RunStateKind) {
	for _, kind := range kinds {
		if err := s.dropRunState(kind, dialogID); err != nil {
			slog.Warn("remove run state", "dialog_id", dialogID, "kind", kind, "error", err)
		}
	}
}

// listRunStates returns every document of kind. A sweep reads them all, so the
// deadline is longer than a single document's.
func (s *ChatService) listRunStates(kind repository.RunStateKind) ([]repository.RunStateEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*runStateTimeout)
	defer cancel()
	return s.runStateStore().ListRunStates(ctx, kind)
}

// listRunningRunStates returns every document of kind that may hold a running
// record.
func (s *ChatService) listRunningRunStates(kind repository.RunStateKind) ([]repository.RunStateEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*runStateTimeout)
	defer cancel()
	return s.runStateStore().ListRunningRunStates(ctx, kind)
}

// ImportRunStateFiles copies every run-state file still on DATA_DIR into store
// and renames each to *.migrated, so an upgrade keeps what the previous release
// had on disk. A document already in store wins: the file is older than
// anything written since the store was switched on.
//
// A file whose dialog no longer exists cannot be stored (the table references
// chat_dialogs) and nothing would ever read it, so it is renamed all the same.
func (s *ChatService) ImportRunStateFiles(ctx context.Context, store repository.RunStateRepository) (int, error) {
	files := fileRunState{s: s}
	imported := 0
	for _, kind := range repository.RunStateKinds {
		entries, paths, err := files.list(kind)
		if err != nil {
			return imported, err
		}
		for i, e := range entries {
			if !json.Valid(e.Data) {
				slog.Warn("run state import: skipping a file that is not JSON", "path", paths[i])
				continue
			}
			ok, err := store.InsertRunState(ctx, kind, e.DialogID, e.Data)
			if err != nil {
				if !errors.Is(err, repository.ErrNotFound) {
					return imported, fmt.Errorf("import %s: %w", paths[i], err)
				}
				slog.Warn("run state import: dialog no longer exists, leaving its file behind",
					"path", paths[i], "kind", kind)
			} else if ok {
				imported++
			}
			// A process booting beside this one may have moved it first.
			if err := os.Rename(paths[i], paths[i]+migratedSuffix); err != nil && !errors.Is(err, os.ErrNotExist) {
				return imported, fmt.Errorf("mark %s migrated: %w", paths[i], err)
			}
		}
	}
	return imported, nil
}

// fileRunState is the run-state store a service without a database keeps:
// the per-dialog JSON files, at the paths each kind always had. It takes no
// locks; the callers' plan and record locks are what serialise a
// read-modify-write.
type fileRunState struct {
	s *ChatService
}

var _ repository.RunStateRepository = fileRunState{}

// location is the directory and file suffix of kind.
func (f fileRunState) location(kind repository.RunStateKind) (dir, suffix string, err error) {
	switch kind {
	case repository.RunStatePlanState:
		return f.s.planStateDir, ".json", nil
	case repository.RunStateFanout:
		return f.s.planFanoutDir, ".json", nil
	case repository.RunStateSubagents:
		return f.s.subagentsDir, ".json", nil
	case repository.RunStateCodeFixes:
		return f.s.codeFixesDir, ".json", nil
	case repository.RunStateIncidentRun:
		return f.s.incidentRunsDir, ".json", nil
	case repository.RunStateActionExec:
		return f.s.actionPlansDir, ".exec.json", nil
	case repository.RunStateActionRuns:
		return f.s.actionPlansDir, ".runs.json", nil
	case repository.RunStateActionNotes:
		return f.s.actionPlansDir, ".notes.json", nil
	case repository.RunStateStageRun:
		return f.s.actionPlansDir, ".stagerun.json", nil
	}
	return "", "", fmt.Errorf("unknown run state kind %q", kind)
}

func (f fileRunState) path(kind repository.RunStateKind, dialogID uuid.UUID) (string, error) {
	dir, suffix, err := f.location(kind)
	if err != nil {
		return "", err
	}
	if dir == "" {
		return "", fmt.Errorf("%s: no directory configured", kind)
	}
	return filepath.Join(dir, dialogID.String()+suffix), nil
}

func (f fileRunState) GetRunState(_ context.Context, kind repository.RunStateKind, dialogID uuid.UUID) ([]byte, error) {
	path, err := f.path(kind, dialogID)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", kind, err)
	}
	return b, nil
}

func (f fileRunState) GetRunStates(ctx context.Context, kind repository.RunStateKind, dialogIDs []uuid.UUID) (map[uuid.UUID][]byte, error) {
	out := make(map[uuid.UUID][]byte, len(dialogIDs))
	for _, id := range dialogIDs {
		b, err := f.GetRunState(ctx, kind, id)
		if errors.Is(err, repository.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[id] = b
	}
	return out, nil
}

func (f fileRunState) PutRunState(_ context.Context, kind repository.RunStateKind, dialogID uuid.UUID, data []byte) error {
	path, err := f.path(kind, dialogID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s directory: %w", kind, err)
	}
	if err := atomicfile.Write(path, data); err != nil {
		return fmt.Errorf("write %s: %w", kind, err)
	}
	return nil
}

func (f fileRunState) InsertRunState(ctx context.Context, kind repository.RunStateKind, dialogID uuid.UUID, data []byte) (bool, error) {
	if _, err := f.GetRunState(ctx, kind, dialogID); err == nil {
		return false, nil
	} else if !errors.Is(err, repository.ErrNotFound) {
		return false, err
	}
	return true, f.PutRunState(ctx, kind, dialogID, data)
}

func (f fileRunState) DeleteRunState(_ context.Context, kind repository.RunStateKind, dialogID uuid.UUID) error {
	path, err := f.path(kind, dialogID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", kind, err)
	}
	return nil
}

func (f fileRunState) ListRunStates(_ context.Context, kind repository.RunStateKind) ([]repository.RunStateEntry, error) {
	entries, _, err := f.list(kind)
	return entries, err
}

// ListRunningRunStates is every document: the files are few, and a caller
// filters what it decodes anyway.
func (f fileRunState) ListRunningRunStates(ctx context.Context, kind repository.RunStateKind) ([]repository.RunStateEntry, error) {
	return f.ListRunStates(ctx, kind)
}

// list returns every document of kind with the file it came from. A file that
// vanishes between the glob and the read is skipped, and so is one whose name
// is not a dialog id.
func (f fileRunState) list(kind repository.RunStateKind) ([]repository.RunStateEntry, []string, error) {
	dir, suffix, err := f.location(kind)
	if err != nil {
		return nil, nil, err
	}
	if dir == "" {
		return nil, nil, nil
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*"+suffix))
	if err != nil {
		return nil, nil, fmt.Errorf("list %s: %w", kind, err)
	}

	var entries []repository.RunStateEntry
	var kept []string
	for _, path := range paths {
		name := strings.TrimSuffix(filepath.Base(path), suffix)
		// A bare ".json" glob also matches "{id}.exec.json" and the other side
		// files, whose stem is not a uuid; they belong to other kinds.
		id, err := uuid.Parse(name)
		if err != nil {
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				slog.Warn("run state: read record", "path", path, "error", err)
			}
			continue
		}
		entries = append(entries, repository.RunStateEntry{DialogID: id, Data: b})
		kept = append(kept, path)
	}
	return entries, kept, nil
}
