package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/javdet/nib/internal/atomicfile"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/hostenv"
	"github.com/javdet/nib/internal/repository"
)

// hostEnvMarkerFile records the values detection last wrote, so a later start
// can tell a value it owns from one the operator has since corrected. The dot
// prefix keeps it out of every {name}.md and {mode}.json lookup on the volume.
const hostEnvMarkerFile = ".host-env.json"

// hostVariable is one row of the Environment block the mode prompts render.
type hostVariable struct {
	name        string
	description string
	value       func(info hostenv.Info, dataDir string) string
}

// hostVariables are the facts about the container the backend runs in. They are
// prompt variables rather than text built at render time so the operator can see
// what the agent believes and correct it: a value nothing has touched is
// refreshed on the next start, an edited one is left alone.
var hostVariables = []hostVariable{
	{
		name:        "hostOS",
		description: "Host OS of the nib backend (auto-detected at startup; edit to override)",
		value: func(info hostenv.Info, _ string) string {
			return orElse(info.DescribeOS(), "unknown")
		},
	},
	{
		name:        "hostRuntime",
		description: "Whether the nib backend runs in Docker, Kubernetes or on a bare host (auto-detected at startup; edit to override)",
		value: func(info hostenv.Info, _ string) string {
			return info.DescribeRuntime()
		},
	},
	{
		name:        "hostShell",
		description: "Login shell present on the nib backend host (auto-detected at startup; edit to override)",
		value: func(info hostenv.Info, _ string) string {
			return orElse(info.Shell, "no login shell is installed")
		},
	},
	{
		name:        "hostWorkingDir",
		description: "Working directory of the nib backend process (auto-detected at startup; edit to override)",
		value: func(info hostenv.Info, _ string) string {
			return orElse(info.WorkDir, "unknown")
		},
	},
	{
		name:        "hostDataDir",
		description: "Directory holding nib's own file-backed state: prompts, skills, rules, tool allow lists, mcp.json",
		value: func(_ hostenv.Info, dataDir string) string {
			return orElse(dataDir, "unknown")
		},
	},
	{
		name:        "hostUser",
		description: "User the nib backend process runs as (auto-detected at startup; edit to override)",
		value: func(info hostenv.Info, _ string) string {
			return info.DescribeUser()
		},
	},
	{
		name:        "hostCommands",
		description: "Infrastructure CLIs found on the nib backend's PATH (auto-detected at startup; edit to override)",
		value: func(info hostenv.Info, _ string) string {
			return info.DescribeCommands()
		},
	},
}

// HostEnvResult reports what the startup reconcile of the host variables did,
// for the boot log.
type HostEnvResult struct {
	Created []string // inserted on this start
	// Refreshed are values detection owned and has now changed -- a deployment
	// that moved from Docker to Kubernetes shows up here.
	Refreshed []string
	// Overridden are values the operator has edited, so detection left them.
	Overridden []string
}

// HostEnvService keeps the host* prompt variables in step with the process the
// backend actually runs in, without ever overwriting an operator's edit.
type HostEnvService struct {
	repo    repository.VariableRepository
	dataDir string
}

func NewHostEnvService(repo repository.VariableRepository, dataDir string) *HostEnvService {
	return &HostEnvService{repo: repo, dataDir: dataDir}
}

// HostVariableNames returns the names of the host variables, in prompt order.
func HostVariableNames() []string {
	names := make([]string, 0, len(hostVariables))
	for _, v := range hostVariables {
		names = append(names, v.name)
	}
	return names
}

// EnsureDefaults reconciles the detected snapshot into prompt_variables.
//
// The rows are marked non-deletable, so the Environment block of every mode
// prompt can rely on them being there, while their values stay editable.
func (s *HostEnvService) EnsureDefaults(ctx context.Context, info hostenv.Info) (HostEnvResult, error) {
	var result HostEnvResult

	detected := make(map[string]string, len(hostVariables))
	for _, v := range hostVariables {
		detected[v.name] = v.value(info, s.dataDir)
	}

	previous := s.readMarker()

	stored, err := s.storedValues(ctx)
	if err != nil {
		return result, err
	}

	for _, v := range hostVariables {
		value := detected[v.name]
		current, found := stored[v.name]

		switch {
		case !found:
			if err := s.write(ctx, v, value); err != nil {
				return result, err
			}
			result.Created = append(result.Created, v.name)
		case current == value:
			// Already what we would have written.
		case current == previous[v.name]:
			// Still the value detection put there, and the host has changed
			// under it.
			if err := s.write(ctx, v, value); err != nil {
				return result, err
			}
			result.Refreshed = append(result.Refreshed, v.name)
		default:
			result.Overridden = append(result.Overridden, v.name)
		}
	}

	if err := s.writeMarker(detected); err != nil {
		return result, err
	}
	return result, nil
}

func (s *HostEnvService) write(ctx context.Context, v hostVariable, value string) error {
	_, err := s.repo.Upsert(ctx, domain.PromptVariable{
		Scope:       defaultVariableScope,
		Name:        v.name,
		Description: v.description,
		Value:       value,
		Kind:        VariableKindString,
		Deletable:   false,
	})
	if err != nil {
		return fmt.Errorf("upsert host variable %s: %w", v.name, err)
	}
	return nil
}

// storedValues reads the current global-scope values, keyed by name.
func (s *HostEnvService) storedValues(ctx context.Context) (map[string]string, error) {
	vars, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list prompt variables: %w", err)
	}

	out := make(map[string]string, len(vars))
	for _, v := range vars {
		if v.Scope == defaultVariableScope && v.ScopeName == "" {
			out[v.Name] = v.Value
		}
	}
	return out, nil
}

func (s *HostEnvService) markerPath() string {
	return filepath.Join(s.dataDir, hostEnvMarkerFile)
}

// readMarker returns the values detection last wrote. An absent or unreadable
// marker is treated as "nothing is ours yet": every existing row then counts as
// an operator's, which loses one refresh but never overwrites an edit.
func (s *HostEnvService) readMarker() map[string]string {
	if s.dataDir == "" {
		return nil
	}

	data, err := os.ReadFile(s.markerPath())
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			slog.Warn("read host environment marker", "path", s.markerPath(), "error", err)
		}
		return nil
	}

	var previous map[string]string
	if err := json.Unmarshal(data, &previous); err != nil {
		slog.Warn("host environment marker is not valid JSON; treating every value as operator-owned",
			"path", s.markerPath(), "error", err)
		return nil
	}
	return previous
}

func (s *HostEnvService) writeMarker(detected map[string]string) error {
	if s.dataDir == "" {
		return nil
	}
	if err := os.MkdirAll(s.dataDir, 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	data, err := json.MarshalIndent(detected, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal host environment marker: %w", err)
	}
	if err := atomicfile.Write(s.markerPath(), append(data, '\n')); err != nil {
		return fmt.Errorf("write host environment marker: %w", err)
	}
	return nil
}

func orElse(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
