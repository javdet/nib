package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/hostenv"
	"github.com/javdet/nib/internal/repository"
)

// fakeHostVarRepo is a variable repository that remembers what was written, so
// a test can tell an insert from a refresh from a value left alone.
type fakeHostVarRepo struct {
	rows    map[string]domain.PromptVariable
	upserts []string
}

func newFakeHostVarRepo(values map[string]string) *fakeHostVarRepo {
	rows := make(map[string]domain.PromptVariable, len(values))
	for name, value := range values {
		rows[name] = domain.PromptVariable{
			Scope: defaultVariableScope, Name: name, Value: value,
			Kind: VariableKindString, Deletable: false,
		}
	}
	return &fakeHostVarRepo{rows: rows}
}

func (r *fakeHostVarRepo) List(context.Context) ([]domain.PromptVariable, error) {
	out := make([]domain.PromptVariable, 0, len(r.rows))
	for _, v := range r.rows {
		out = append(out, v)
	}
	return out, nil
}

func (r *fakeHostVarRepo) Upsert(_ context.Context, v domain.PromptVariable) (domain.PromptVariable, error) {
	r.upserts = append(r.upserts, v.Name)
	r.rows[v.Name] = v
	return v, nil
}

func (r *fakeHostVarRepo) GetByID(context.Context, uuid.UUID) (domain.PromptVariable, error) {
	return domain.PromptVariable{}, repository.ErrNotFound
}
func (r *fakeHostVarRepo) Create(context.Context, domain.PromptVariable) (domain.PromptVariable, error) {
	return domain.PromptVariable{}, nil
}
func (r *fakeHostVarRepo) Update(context.Context, uuid.UUID, domain.PromptVariable) (domain.PromptVariable, error) {
	return domain.PromptVariable{}, nil
}
func (r *fakeHostVarRepo) Delete(context.Context, uuid.UUID) error                    { return nil }
func (r *fakeHostVarRepo) EnsureExists(context.Context, domain.PromptVariable) error  { return nil }
func (r *fakeHostVarRepo) EnsureBuiltin(context.Context, domain.PromptVariable) error { return nil }
func (r *fakeHostVarRepo) LoadAll(context.Context) (map[string]map[string]any, error) {
	return nil, nil
}

func dockerHost() hostenv.Info {
	return hostenv.Info{
		GOOS: "linux", GOARCH: "arm64",
		Distro: "Debian GNU/Linux 12 (bookworm)", Kernel: "6.10.14-linuxkit",
		Shell: "/bin/bash", WorkDir: "/app", User: "root", UID: 0,
		Runtime: hostenv.RuntimeDocker,
		OnPath:  []string{"bash", "curl", "jq"},
	}
}

func TestHostEnvEnsureDefaultsFirstBoot(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	repo := newFakeHostVarRepo(nil)
	svc := NewHostEnvService(repo, dataDir)

	result, err := svc.EnsureDefaults(context.Background(), dockerHost())
	if err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}

	if !slices.Equal(result.Created, HostVariableNames()) {
		t.Fatalf("Created = %v, want every host variable %v", result.Created, HostVariableNames())
	}
	if len(result.Refreshed) != 0 || len(result.Overridden) != 0 {
		t.Fatalf("first boot reported refreshed=%v overridden=%v, want neither", result.Refreshed, result.Overridden)
	}

	for _, name := range HostVariableNames() {
		row, ok := repo.rows[name]
		if !ok {
			t.Fatalf("%s was not written", name)
		}
		// The Environment block of four prompts renders these, so they must not
		// be deletable -- but their value stays editable.
		if row.Deletable {
			t.Fatalf("%s was written as deletable", name)
		}
		if row.Value == "" {
			t.Fatalf("%s was written empty, which renders as a dangling label", name)
		}
		if row.Description == "" {
			t.Fatalf("%s has no description for the Variables page", name)
		}
	}
	if got := repo.rows["hostRuntime"].Value; got != "Docker container" {
		t.Fatalf("hostRuntime = %q, want %q", got, "Docker container")
	}
	if got := repo.rows["hostDataDir"].Value; got != dataDir {
		t.Fatalf("hostDataDir = %q, want %q", got, dataDir)
	}

	marker := readMarker(t, dataDir)
	if marker["hostRuntime"] != "Docker container" {
		t.Fatalf("marker = %v, want it to record what detection wrote", marker)
	}
}

func TestHostEnvEnsureDefaultsSecondBootIsQuiet(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	repo := newFakeHostVarRepo(nil)
	svc := NewHostEnvService(repo, dataDir)

	if _, err := svc.EnsureDefaults(context.Background(), dockerHost()); err != nil {
		t.Fatalf("first EnsureDefaults: %v", err)
	}
	repo.upserts = nil

	result, err := svc.EnsureDefaults(context.Background(), dockerHost())
	if err != nil {
		t.Fatalf("second EnsureDefaults: %v", err)
	}
	if len(repo.upserts) != 0 {
		t.Fatalf("an unchanged host rewrote %v", repo.upserts)
	}
	if len(result.Created)+len(result.Refreshed)+len(result.Overridden) != 0 {
		t.Fatalf("second boot reported %+v, want nothing", result)
	}
}

// The whole point of the marker: a deployment that moves from Docker to
// Kubernetes must stop telling the model it is in Docker.
func TestHostEnvEnsureDefaultsRefreshesWhatDetectionOwns(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	repo := newFakeHostVarRepo(nil)
	svc := NewHostEnvService(repo, dataDir)

	if _, err := svc.EnsureDefaults(context.Background(), dockerHost()); err != nil {
		t.Fatalf("first EnsureDefaults: %v", err)
	}
	repo.upserts = nil

	moved := dockerHost()
	moved.Runtime = hostenv.RuntimeKubernetes
	moved.KubeNamespace = "nib"

	result, err := svc.EnsureDefaults(context.Background(), moved)
	if err != nil {
		t.Fatalf("EnsureDefaults after the move: %v", err)
	}
	if !slices.Equal(result.Refreshed, []string{"hostRuntime"}) {
		t.Fatalf("Refreshed = %v, want [hostRuntime]", result.Refreshed)
	}
	if got := repo.rows["hostRuntime"].Value; got != "Kubernetes pod (namespace nib)" {
		t.Fatalf("hostRuntime = %q, want the new runtime", got)
	}
}

func TestHostEnvEnsureDefaultsLeavesAnOperatorEditAlone(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	repo := newFakeHostVarRepo(nil)
	svc := NewHostEnvService(repo, dataDir)

	if _, err := svc.EnsureDefaults(context.Background(), dockerHost()); err != nil {
		t.Fatalf("first EnsureDefaults: %v", err)
	}

	edited := repo.rows["hostCommands"]
	edited.Value = "bash, curl, jq, kubectl (installed by hand)"
	repo.rows["hostCommands"] = edited
	repo.upserts = nil

	result, err := svc.EnsureDefaults(context.Background(), dockerHost())
	if err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}
	if !slices.Equal(result.Overridden, []string{"hostCommands"}) {
		t.Fatalf("Overridden = %v, want [hostCommands]", result.Overridden)
	}
	if len(repo.upserts) != 0 {
		t.Fatalf("the operator's edit was overwritten: %v", repo.upserts)
	}
	if got := repo.rows["hostCommands"].Value; got != edited.Value {
		t.Fatalf("hostCommands = %q, want the operator's value %q", got, edited.Value)
	}
}

// Without a marker nothing can be proved to be ours, so every existing row is
// treated as the operator's. That loses one refresh and never loses an edit.
func TestHostEnvEnsureDefaultsWithoutAMarkerTreatsRowsAsOperatorOwned(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		marker string
	}{
		{name: "no marker at all"},
		{name: "corrupt marker", marker: "{not json"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dataDir := t.TempDir()
			if tt.marker != "" {
				if err := os.WriteFile(filepath.Join(dataDir, hostEnvMarkerFile), []byte(tt.marker), 0o644); err != nil {
					t.Fatalf("write marker: %v", err)
				}
			}

			repo := newFakeHostVarRepo(map[string]string{"hostRuntime": "something an operator wrote"})
			svc := NewHostEnvService(repo, dataDir)

			result, err := svc.EnsureDefaults(context.Background(), dockerHost())
			if err != nil {
				t.Fatalf("EnsureDefaults: %v", err)
			}
			if !slices.Contains(result.Overridden, "hostRuntime") {
				t.Fatalf("Overridden = %v, want it to contain hostRuntime", result.Overridden)
			}
			if got := repo.rows["hostRuntime"].Value; got != "something an operator wrote" {
				t.Fatalf("hostRuntime = %q, want the existing value untouched", got)
			}
			// The rows that were absent are still created.
			if !slices.Contains(result.Created, "hostOS") {
				t.Fatalf("Created = %v, want it to contain hostOS", result.Created)
			}
		})
	}
}

// A sparse snapshot is what a probe that could not answer looks like; the rows
// must still say something rather than render as an empty bullet.
func TestHostEnvEnsureDefaultsNeverWritesBlanks(t *testing.T) {
	t.Parallel()

	repo := newFakeHostVarRepo(nil)
	svc := NewHostEnvService(repo, t.TempDir())

	if _, err := svc.EnsureDefaults(context.Background(), hostenv.Info{}); err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}
	for name, row := range repo.rows {
		if row.Value == "" {
			t.Fatalf("%s was written empty from an empty snapshot", name)
		}
	}
}

func TestHostEnvEnsureDefaultsReportsAMarkerItCannotWrite(t *testing.T) {
	t.Parallel()

	// A file where the data directory should be: MkdirAll fails, and the caller
	// has to hear about it rather than silently lose the marker.
	blocked := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}

	svc := NewHostEnvService(newFakeHostVarRepo(nil), blocked)
	if _, err := svc.EnsureDefaults(context.Background(), dockerHost()); err == nil {
		t.Fatal("EnsureDefaults with an unwritable data directory = nil, want error")
	}
}

func readMarker(t *testing.T, dataDir string) map[string]string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(dataDir, hostEnvMarkerFile))
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	var marker map[string]string
	if err := json.Unmarshal(data, &marker); err != nil {
		t.Fatalf("marker is not valid JSON: %v", err)
	}
	return marker
}
