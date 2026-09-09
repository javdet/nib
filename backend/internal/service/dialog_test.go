package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

type stubDialogRepo struct {
	dialog     domain.Dialog
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

func (s *stubDialogRepo) ListPlanDialogIDs(_ context.Context) ([]uuid.UUID, error) {
	return nil, nil
}
func (s *stubDialogRepo) GetDialog(context.Context, uuid.UUID) (domain.Dialog, error) {
	return s.dialog, nil
}
func (s *stubDialogRepo) UpdateTitle(context.Context, uuid.UUID, string) error { return nil }
func (s *stubDialogRepo) SetDialogTaskID(context.Context, uuid.UUID, *string) error {
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
func (s *stubVariableRepo) Delete(context.Context, uuid.UUID) error                    { return nil }
func (s *stubVariableRepo) EnsureExists(context.Context, domain.PromptVariable) error  { return nil }
func (s *stubVariableRepo) EnsureBuiltin(context.Context, domain.PromptVariable) error { return nil }
func (s *stubVariableRepo) Upsert(context.Context, domain.PromptVariable) (domain.PromptVariable, error) {
	return domain.PromptVariable{}, nil
}

// LoadAll fills in the host* variables unless the test set its own. Every mode
// prompt renders them and they are reconciled on every boot in production, so a
// fixture that omits them would fail on missingkey=error for a reason that has
// nothing to do with what it is testing.
func (s *stubVariableRepo) LoadAll(context.Context, map[string]string) (map[string]map[string]any, error) {
	out := make(map[string]map[string]any, len(s.vars)+1)
	for scope, names := range s.vars {
		copied := make(map[string]any, len(names))
		for name, value := range names {
			copied[name] = value
		}
		out[scope] = copied
	}
	if out[defaultVariableScope] == nil {
		out[defaultVariableScope] = map[string]any{}
	}
	for _, name := range HostVariableNames() {
		if _, ok := out[defaultVariableScope][name]; !ok {
			out[defaultVariableScope][name] = "stub-" + name
		}
	}
	return out, nil
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

func TestDialogService_SetCategories(t *testing.T) {
	t.Parallel()

	repo := &stubDialogRepo{dialog: domain.Dialog{ID: uuid.New()}}
	svc := NewDialogService(repo)

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
