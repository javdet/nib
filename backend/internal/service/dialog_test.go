package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/rules"
	"github.com/google/uuid"
)

type stubDialogRepo struct {
	dialog     domain.Dialog
	subjects   []string
	categories []string
}

func (s *stubDialogRepo) CreateDialog(context.Context, string, string, *uuid.UUID) (domain.Dialog, error) {
	return domain.Dialog{}, nil
}
func (s *stubDialogRepo) ListRecentDialogs(context.Context, int, int) ([]domain.Dialog, error) {
	return nil, nil
}
func (s *stubDialogRepo) CountDialogs(context.Context) (int, error) { return 0, nil }
func (s *stubDialogRepo) ListDialogsByMode(context.Context, string, int, int) ([]domain.Dialog, error) {
	return nil, nil
}
func (s *stubDialogRepo) CountDialogsByMode(context.Context, string) (int, error) { return 0, nil }
func (s *stubDialogRepo) ListAllRecentDialogs(context.Context, int, int) ([]domain.Dialog, error) {
	return nil, nil
}
func (s *stubDialogRepo) CountAllDialogs(context.Context) (int, error) { return 0, nil }
func (s *stubDialogRepo) SearchDialogs(context.Context, string, string, int, int) ([]domain.Dialog, error) {
	return nil, nil
}
func (s *stubDialogRepo) CountDialogsSearch(context.Context, string, string) (int, error) {
	return 0, nil
}
func (s *stubDialogRepo) ListChildren(context.Context, uuid.UUID) ([]domain.Dialog, error) {
	return nil, nil
}
func (s *stubDialogRepo) ListPlanChildren(context.Context, []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	return map[uuid.UUID]uuid.UUID{}, nil
}
func (s *stubDialogRepo) GetDialog(context.Context, uuid.UUID) (domain.Dialog, error) {
	return s.dialog, nil
}
func (s *stubDialogRepo) UpdateTitle(context.Context, uuid.UUID, string) error { return nil }
func (s *stubDialogRepo) SetDialogTaskID(context.Context, uuid.UUID, *string) error {
	return nil
}
func (s *stubDialogRepo) SetDialogSubjects(_ context.Context, _ uuid.UUID, subjects []string) error {
	s.subjects = subjects
	s.dialog.Subjects = subjects
	return nil
}
func (s *stubDialogRepo) SetDialogCategories(_ context.Context, _ uuid.UUID, categories []string) error {
	s.categories = categories
	s.dialog.Categories = categories
	return nil
}
func (s *stubDialogRepo) SetDialogPinned(context.Context, uuid.UUID, bool) error { return nil }
func (s *stubDialogRepo) ListPinnedDialogs(context.Context) ([]domain.Dialog, error) {
	return nil, nil
}
func (s *stubDialogRepo) DeleteDialog(context.Context, uuid.UUID) error { return nil }
func (s *stubDialogRepo) ListMessages(context.Context, uuid.UUID) ([]domain.DialogMessage, error) {
	return nil, nil
}
func (s *stubDialogRepo) AppendMessage(context.Context, uuid.UUID, domain.DialogMessage) (domain.DialogMessage, error) {
	return domain.DialogMessage{}, nil
}
func (s *stubDialogRepo) DeleteMessagesAfterSeq(context.Context, uuid.UUID, int) error {
	return nil
}

type stubVariableRepo struct {
	vars map[string]map[string]any
	list []domain.PromptVariable
}

func (s *stubVariableRepo) List(context.Context) ([]domain.PromptVariable, error) {
	if s.list == nil {
		return nil, nil
	}
	return s.list, nil
}
func (s *stubVariableRepo) GetByID(context.Context, uuid.UUID) (domain.PromptVariable, error) {
	return domain.PromptVariable{}, nil
}
func (s *stubVariableRepo) Create(context.Context, domain.PromptVariable) (domain.PromptVariable, error) {
	return domain.PromptVariable{}, nil
}
func (s *stubVariableRepo) Update(context.Context, uuid.UUID, domain.PromptVariable) (domain.PromptVariable, error) {
	return domain.PromptVariable{}, nil
}
func (s *stubVariableRepo) Delete(context.Context, uuid.UUID) error { return nil }
func (s *stubVariableRepo) EnsureExists(context.Context, domain.PromptVariable) error { return nil }
func (s *stubVariableRepo) EnsureBuiltin(context.Context, domain.PromptVariable) error { return nil }
func (s *stubVariableRepo) Upsert(context.Context, domain.PromptVariable) (domain.PromptVariable, error) {
	return domain.PromptVariable{}, nil
}
func (s *stubVariableRepo) LoadAll(context.Context) (map[string]map[string]any, error) {
	return s.vars, nil
}

func TestDialogService_MatchedRules_rendersTemplateVariables(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	rulesDir := filepath.Join(dataDir, "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatalf("mkdir rules: %v", err)
	}
	ruleContent := "Company: {{ .global.CompanyName }}, project: {{ .builtin.Project }}"
	if err := os.WriteFile(filepath.Join(rulesDir, "postgres.md"), []byte(ruleContent), 0o644); err != nil {
		t.Fatalf("write rule: %v", err)
	}

	rulesSvc := rules.NewService(dataDir, "")
	selection := NewSelectionStore()
	selection.Set(domain.Selection{Project: "demo", Environment: "any", Cloud: "any", Location: "any"})

	dialogID := uuid.New()
	svc := NewDialogService(
		&stubDialogRepo{dialog: domain.Dialog{ID: dialogID, Subjects: []string{"postgres"}}},
		rulesSvc,
		&stubVariableRepo{vars: map[string]map[string]any{
			"global": {"CompanyName": "AutomagicOps"},
		}},
		selection,
	)

	matched, err := svc.MatchedRules(context.Background(), dialogID)
	if err != nil {
		t.Fatalf("MatchedRules: %v", err)
	}
	if len(matched) != 1 {
		t.Fatalf("matched rules = %d, want 1", len(matched))
	}
	want := "Company: AutomagicOps, project: demo"
	if matched[0].Content != want {
		t.Fatalf("rendered content = %q, want %q", matched[0].Content, want)
	}
}

func TestDialogService_MatchedRules_plainContentUnchanged(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	rulesDir := filepath.Join(dataDir, "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatalf("mkdir rules: %v", err)
	}
	const plain = "- Postgres must listen on all interfaces"
	if err := os.WriteFile(filepath.Join(rulesDir, "postgres.md"), []byte(plain), 0o644); err != nil {
		t.Fatalf("write rule: %v", err)
	}

	rulesSvc := rules.NewService(dataDir, "")
	dialogID := uuid.New()
	svc := NewDialogService(
		&stubDialogRepo{dialog: domain.Dialog{ID: dialogID, Subjects: []string{"postgres"}}},
		rulesSvc,
		nil,
		nil,
	)

	matched, err := svc.MatchedRules(context.Background(), dialogID)
	if err != nil {
		t.Fatalf("MatchedRules: %v", err)
	}
	if len(matched) != 1 || matched[0].Content != plain {
		t.Fatalf("matched = %+v, want plain content %q", matched, plain)
	}
}

func TestNormalizeTagList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   []string
		want    []string
		wantErr error
	}{
		{
			name:  "empty",
			input: nil,
			want:  []string{},
		},
		{
			name:  "single word",
			input: []string{"Cassandra"},
			want:  []string{"cassandra"},
		},
		{
			name:  "multi word entry splits",
			input: []string{"postgres nginx"},
			want:  []string{"postgres", "nginx"},
		},
		{
			name:  "comma separated",
			input: []string{"cloud, k8s"},
			want:  []string{"cloud", "k8s"},
		},
		{
			name:  "dedupe",
			input: []string{"cloud", "Cloud", "cloud"},
			want:  []string{"cloud"},
		},
		{
			name:  "trim and drop empty",
			input: []string{"  ", "nginx", ""},
			want:  []string{"nginx"},
		},
		{
			name:    "too many tags",
			input:   []string{"a b c d e f g h i j k l m n o p q r s t u v w x y z aa ab ac ad ae af ag ah ai aj ak al am an ao ap aq ar as at au av aw ax ay az ba"},
			wantErr: ErrTooManyDialogTags,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeTagList(tt.input)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("normalizeTagList() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeTagList() unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("normalizeTagList() = %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("normalizeTagList() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestDialogService_SetSubjects(t *testing.T) {
	t.Parallel()

	repo := &stubDialogRepo{dialog: domain.Dialog{ID: uuid.New()}}
	svc := NewDialogService(repo, nil, nil, nil)

	if err := svc.SetSubjects(context.Background(), repo.dialog.ID, []string{"Postgres", "nginx"}); err != nil {
		t.Fatalf("SetSubjects: %v", err)
	}
	want := []string{"postgres", "nginx"}
	if len(repo.subjects) != len(want) {
		t.Fatalf("subjects = %v, want %v", repo.subjects, want)
	}
	for i := range want {
		if repo.subjects[i] != want[i] {
			t.Fatalf("subjects = %v, want %v", repo.subjects, want)
		}
	}
}

func TestDialogService_SetCategories(t *testing.T) {
	t.Parallel()

	repo := &stubDialogRepo{dialog: domain.Dialog{ID: uuid.New()}}
	svc := NewDialogService(repo, nil, nil, nil)

	if err := svc.SetCategories(context.Background(), repo.dialog.ID, []string{"Cloud", "k8s"}); err != nil {
		t.Fatalf("SetCategories: %v", err)
	}
	want := []string{"cloud", "k8s"}
	if len(repo.categories) != len(want) {
		t.Fatalf("categories = %v, want %v", repo.categories, want)
	}
	for i := range want {
		if repo.categories[i] != want[i] {
			t.Fatalf("categories = %v, want %v", repo.categories, want)
		}
	}
}
