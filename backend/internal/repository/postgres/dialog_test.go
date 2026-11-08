package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
)

func mustCreateDialog(t *testing.T, repo *DialogRepo, mode, title string, parent *uuid.UUID) domain.Dialog {
	t.Helper()
	d, err := repo.CreateDialog(context.Background(), mode, title, parent)
	if err != nil {
		t.Fatalf("CreateDialog(%s, %q): %v", mode, title, err)
	}
	return d
}

func dialogIDs(ds []domain.Dialog) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.ID)
	}
	return out
}

func sortedIDs(ids []uuid.UUID) []uuid.UUID {
	out := slices.Clone(ids)
	slices.SortFunc(out, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	return out
}

func TestDialogRepoCreateAndGet(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewDialogRepo(pool)

	root := mustCreateDialog(t, repo, "main", "deploy api", nil)
	if root.ParentID != nil || root.Pinned || root.TaskID != nil {
		t.Fatalf("new root = %+v, want no parent, unpinned, no task", root)
	}
	child := mustCreateDialog(t, repo, "execute", "step 1", &root.ID)
	if child.ParentID == nil || *child.ParentID != root.ID {
		t.Fatalf("child.ParentID = %v, want %s", child.ParentID, root.ID)
	}

	got, err := repo.GetDialog(ctx, root.ID)
	if err != nil {
		t.Fatalf("GetDialog: %v", err)
	}
	if got.ID != root.ID || got.Title != "deploy api" || got.Mode != "main" {
		t.Fatalf("GetDialog = %+v, want the created root", got)
	}
	if _, err := repo.GetDialog(ctx, uuid.New()); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("GetDialog unknown = %v, want ErrNotFound", err)
	}

	// The schema refuses a mode the backend does not know.
	if _, err := repo.CreateDialog(ctx, "bogus", "x", nil); err == nil {
		t.Fatal("CreateDialog with an unknown mode succeeded, want the mode CHECK to refuse it")
	}
}

// The plan listings agree on one definition of a plan: a root dialog whose mode
// carries one. Children, discuss chats and incidents must stay out of them.
func TestDialogRepoPlanListings(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewDialogRepo(pool)

	plan := mustCreateDialog(t, repo, "main", "plan", nil)
	pinned := mustCreateDialog(t, repo, "plan", "pinned plan", nil)
	mustCreateDialog(t, repo, "discuss", "chat", nil)
	incident := mustCreateDialog(t, repo, "incident", "outage", nil)
	child := mustCreateDialog(t, repo, "execute", "child", &plan.ID)

	if err := repo.SetDialogPinned(ctx, pinned.ID, true); err != nil {
		t.Fatalf("SetDialogPinned: %v", err)
	}

	recent, err := repo.ListRecentDialogs(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListRecentDialogs: %v", err)
	}
	if got, want := dialogIDs(recent), []uuid.UUID{plan.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ListRecentDialogs = %v, want only the unpinned plan %v", got, want)
	}
	if n, err := repo.CountDialogs(ctx); err != nil || n != 1 {
		t.Fatalf("CountDialogs = %d, %v, want 1", n, err)
	}

	pins, err := repo.ListPinnedDialogs(ctx)
	if err != nil {
		t.Fatalf("ListPinnedDialogs: %v", err)
	}
	if got, want := dialogIDs(pins), []uuid.UUID{pinned.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ListPinnedDialogs = %v, want %v", got, want)
	}
	if !pins[0].Pinned {
		t.Fatal("pinned dialog scanned with Pinned = false")
	}

	ids, err := repo.ListPlanDialogIDs(ctx)
	if err != nil {
		t.Fatalf("ListPlanDialogIDs: %v", err)
	}
	if got, want := sortedIDs(ids), sortedIDs([]uuid.UUID{plan.ID, pinned.ID}); !reflect.DeepEqual(got, want) {
		t.Fatalf("ListPlanDialogIDs = %v, want %v", got, want)
	}

	byMode, err := repo.ListDialogsByMode(ctx, "incident", 10, 0)
	if err != nil {
		t.Fatalf("ListDialogsByMode: %v", err)
	}
	if got, want := dialogIDs(byMode), []uuid.UUID{incident.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ListDialogsByMode(incident) = %v, want %v", got, want)
	}
	// A child in a plan mode is not a root of that mode.
	if n, err := repo.CountDialogsByMode(ctx, "execute"); err != nil || n != 0 {
		t.Fatalf("CountDialogsByMode(execute) = %d, %v, want 0", n, err)
	}
	if n, err := repo.CountDialogsByMode(ctx, "discuss"); err != nil || n != 1 {
		t.Fatalf("CountDialogsByMode(discuss) = %d, %v, want 1", n, err)
	}

	if n, err := repo.CountAllDialogs(ctx); err != nil || n != 5 {
		t.Fatalf("CountAllDialogs = %d, %v, want 5", n, err)
	}
	all, err := repo.ListAllRecentDialogs(ctx, 2, 1)
	if err != nil {
		t.Fatalf("ListAllRecentDialogs: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("ListAllRecentDialogs(limit 2) returned %d rows, want 2", len(all))
	}

	children, err := repo.ListChildren(ctx, plan.ID)
	if err != nil {
		t.Fatalf("ListChildren: %v", err)
	}
	if got, want := dialogIDs(children), []uuid.UUID{child.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ListChildren = %v, want %v", got, want)
	}

	if err := repo.SetDialogPinned(ctx, pinned.ID, false); err != nil {
		t.Fatalf("SetDialogPinned(false): %v", err)
	}
	if n, err := repo.CountDialogs(ctx); err != nil || n != 2 {
		t.Fatalf("CountDialogs after unpin = %d, %v, want 2", n, err)
	}
	if pins, err := repo.ListPinnedDialogs(ctx); err != nil || len(pins) != 0 {
		t.Fatalf("ListPinnedDialogs after unpin = %v, %v, want none", pins, err)
	}
	if err := repo.SetDialogPinned(ctx, uuid.New(), true); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("SetDialogPinned unknown = %v, want ErrNotFound", err)
	}
}

func TestDialogRepoListingsAreNewestFirst(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewDialogRepo(pool)

	older := mustCreateDialog(t, repo, "main", "older", nil)
	newer := mustCreateDialog(t, repo, "main", "newer", nil)

	recent, err := repo.ListRecentDialogs(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListRecentDialogs: %v", err)
	}
	if got, want := dialogIDs(recent), []uuid.UUID{newer.ID, older.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ListRecentDialogs = %v, want newest first %v", got, want)
	}

	// A new message bumps updated_at, which moves the dialog to the top.
	if _, err := repo.AppendMessage(ctx, older.ID, domain.DialogMessage{Role: "user", Content: "hi"}); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	recent, err = repo.ListRecentDialogs(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListRecentDialogs: %v", err)
	}
	if got, want := dialogIDs(recent), []uuid.UUID{older.ID, newer.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ListRecentDialogs after a message = %v, want %v", got, want)
	}

	page, err := repo.ListRecentDialogs(ctx, 1, 1)
	if err != nil {
		t.Fatalf("ListRecentDialogs page: %v", err)
	}
	if got, want := dialogIDs(page), []uuid.UUID{newer.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ListRecentDialogs(limit 1, offset 1) = %v, want %v", got, want)
	}
}

// The search term is a literal: ILIKE wildcards and the escape character in it
// must match themselves, not any character.
func TestDialogRepoSearchEscapesWildcards(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewDialogRepo(pool)

	percent := mustCreateDialog(t, repo, "main", "Rollout 100% done", nil)
	mustCreateDialog(t, repo, "main", "Rollout 100 percent", nil)
	underscore := mustCreateDialog(t, repo, "main", "snake_case plan", nil)
	mustCreateDialog(t, repo, "main", "snakeXcase plan", nil)
	backslash := mustCreateDialog(t, repo, "main", `C:\deploy`, nil)
	chat := mustCreateDialog(t, repo, "discuss", "100% chat", nil)
	// A plan's child never shows up in a search, whatever its title.
	mustCreateDialog(t, repo, "execute", "100% child", &percent.ID)

	tests := []struct {
		name  string
		mode  string
		query string
		want  []uuid.UUID
	}{
		{name: "percent is literal", query: "%", want: []uuid.UUID{percent.ID}},
		{name: "underscore is literal", query: "_", want: []uuid.UUID{underscore.ID}},
		{name: "backslash is literal", query: `\`, want: []uuid.UUID{backslash.ID}},
		{name: "case-insensitive", query: "DONE", want: []uuid.UUID{percent.ID}},
		{name: "no match", query: "nothing like this", want: nil},
		{name: "mode filter", mode: "discuss", query: "%", want: []uuid.UUID{chat.ID}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.SearchDialogs(ctx, tt.mode, tt.query, 10, 0)
			if err != nil {
				t.Fatalf("SearchDialogs: %v", err)
			}
			if ids := dialogIDs(got); !slices.Equal(sortedIDs(ids), sortedIDs(tt.want)) {
				t.Fatalf("SearchDialogs(%q, %q) = %v, want %v", tt.mode, tt.query, ids, tt.want)
			}
			n, err := repo.CountDialogsSearch(ctx, tt.mode, tt.query)
			if err != nil {
				t.Fatalf("CountDialogsSearch: %v", err)
			}
			if n != len(tt.want) {
				t.Fatalf("CountDialogsSearch(%q, %q) = %d, want %d", tt.mode, tt.query, n, len(tt.want))
			}
		})
	}
}

func TestDialogRepoMutations(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewDialogRepo(pool)
	d := mustCreateDialog(t, repo, "main", "old", nil)
	unknown := uuid.New()

	if err := repo.UpdateTitle(ctx, d.ID, "new\x00 title"); err != nil {
		t.Fatalf("UpdateTitle: %v", err)
	}
	task := "OPS-1\x00"
	if err := repo.SetDialogTaskID(ctx, d.ID, &task); err != nil {
		t.Fatalf("SetDialogTaskID: %v", err)
	}
	if err := repo.SetDialogCategories(ctx, d.ID, []string{"k8s", "db\x00"}); err != nil {
		t.Fatalf("SetDialogCategories: %v", err)
	}
	got, err := repo.GetDialog(ctx, d.ID)
	if err != nil {
		t.Fatalf("GetDialog: %v", err)
	}
	if got.Title != "new title" {
		t.Errorf("Title = %q, want NUL stripped %q", got.Title, "new title")
	}
	if got.TaskID == nil || *got.TaskID != "OPS-1" {
		t.Errorf("TaskID = %v, want OPS-1", got.TaskID)
	}
	if want := []string{"k8s", "db"}; !reflect.DeepEqual(got.Categories, want) {
		t.Errorf("Categories = %q, want %q", got.Categories, want)
	}

	// nil clears both rather than failing on a NOT NULL column.
	if err := repo.SetDialogTaskID(ctx, d.ID, nil); err != nil {
		t.Fatalf("SetDialogTaskID(nil): %v", err)
	}
	if err := repo.SetDialogCategories(ctx, d.ID, nil); err != nil {
		t.Fatalf("SetDialogCategories(nil): %v", err)
	}
	got, err = repo.GetDialog(ctx, d.ID)
	if err != nil {
		t.Fatalf("GetDialog: %v", err)
	}
	if got.TaskID != nil {
		t.Errorf("TaskID after clear = %q, want nil", *got.TaskID)
	}
	if len(got.Categories) != 0 {
		t.Errorf("Categories after clear = %q, want empty", got.Categories)
	}

	for name, err := range map[string]error{
		"UpdateTitle":         repo.UpdateTitle(ctx, unknown, "x"),
		"SetDialogTaskID":     repo.SetDialogTaskID(ctx, unknown, nil),
		"SetDialogCategories": repo.SetDialogCategories(ctx, unknown, nil),
		"DeleteDialog":        repo.DeleteDialog(ctx, unknown),
	} {
		if !errors.Is(err, repository.ErrNotFound) {
			t.Errorf("%s on an unknown dialog = %v, want ErrNotFound", name, err)
		}
	}
}

func TestDialogRepoDeleteCascadesToChildrenAndMessages(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewDialogRepo(pool)

	root := mustCreateDialog(t, repo, "main", "root", nil)
	child := mustCreateDialog(t, repo, "plan", "child", &root.ID)
	if _, err := repo.AppendMessage(ctx, child.ID, domain.DialogMessage{Role: "user", Content: "x"}); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}

	if err := repo.DeleteDialog(ctx, root.ID); err != nil {
		t.Fatalf("DeleteDialog: %v", err)
	}
	if _, err := repo.GetDialog(ctx, child.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("GetDialog(child) after deleting the root = %v, want ErrNotFound", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM chat_dialog_messages WHERE dialog_id = $1`, child.ID); n != 0 {
		t.Fatalf("messages of a deleted child = %d, want 0", n)
	}
	if err := repo.DeleteDialog(ctx, root.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("second DeleteDialog = %v, want ErrNotFound", err)
	}
}

func TestDialogRepoMessages(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewDialogRepo(pool)
	d := mustCreateDialog(t, repo, "main", "chat", nil)

	before, err := repo.GetDialog(ctx, d.ID)
	if err != nil {
		t.Fatalf("GetDialog: %v", err)
	}

	user, err := repo.AppendMessage(ctx, d.ID, domain.DialogMessage{Role: "user", Content: "hello\x00 world"})
	if err != nil {
		t.Fatalf("AppendMessage(user): %v", err)
	}
	asst, err := repo.AppendMessage(ctx, d.ID, domain.DialogMessage{
		Role:      "assistant",
		ToolCalls: json.RawMessage(`[{"id":"call_1","name":"get_skill"}]`),
	})
	if err != nil {
		t.Fatalf("AppendMessage(assistant): %v", err)
	}
	tool, err := repo.AppendMessage(ctx, d.ID, domain.DialogMessage{
		Role: "tool", Content: "ok", ToolCallID: "call_1", Name: "get_skill",
	})
	if err != nil {
		t.Fatalf("AppendMessage(tool): %v", err)
	}
	if user.Seq != 0 || asst.Seq != 1 || tool.Seq != 2 {
		t.Fatalf("seqs = %d, %d, %d, want 0, 1, 2", user.Seq, asst.Seq, tool.Seq)
	}
	if user.Content != "hello world" {
		t.Errorf("Content = %q, want NUL stripped", user.Content)
	}

	// Empty optional fields are stored as NULL, not as empty strings or "null".
	if n := countRows(t, pool,
		`SELECT count(*) FROM chat_dialog_messages
		 WHERE id = $1 AND tool_calls IS NULL AND tool_call_id IS NULL AND name IS NULL`, user.ID); n != 1 {
		t.Errorf("user message optional columns are not all NULL")
	}

	after, err := repo.GetDialog(ctx, d.ID)
	if err != nil {
		t.Fatalf("GetDialog: %v", err)
	}
	if !after.UpdatedAt.After(before.UpdatedAt) {
		t.Errorf("UpdatedAt = %v, want later than %v after a message", after.UpdatedAt, before.UpdatedAt)
	}

	msgs, err := repo.ListMessages(ctx, d.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("ListMessages returned %d messages, want 3", len(msgs))
	}
	var calls []map[string]string
	if err := json.Unmarshal(msgs[1].ToolCalls, &calls); err != nil {
		t.Fatalf("ToolCalls %s: %v", msgs[1].ToolCalls, err)
	}
	if len(calls) != 1 || calls[0]["id"] != "call_1" {
		t.Errorf("ToolCalls = %v, want the one call_1", calls)
	}
	if msgs[0].ToolCalls != nil {
		t.Errorf("user ToolCalls = %s, want nil", msgs[0].ToolCalls)
	}
	if msgs[2].ToolCallID != "call_1" || msgs[2].Name != "get_skill" {
		t.Errorf("tool message = %+v, want call_1 / get_skill", msgs[2])
	}

	// Rewinding keeps everything up to and including afterSeq, and the next
	// message continues from what is left rather than from the old maximum.
	if err := repo.DeleteMessagesAfterSeq(ctx, d.ID, 0); err != nil {
		t.Fatalf("DeleteMessagesAfterSeq: %v", err)
	}
	next, err := repo.AppendMessage(ctx, d.ID, domain.DialogMessage{Role: "user", Content: "again"})
	if err != nil {
		t.Fatalf("AppendMessage after rewind: %v", err)
	}
	if next.Seq != 1 {
		t.Fatalf("seq after rewind = %d, want 1", next.Seq)
	}

	unknown := uuid.New()
	if _, err := repo.AppendMessage(ctx, unknown, domain.DialogMessage{Role: "user"}); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("AppendMessage to an unknown dialog = %v, want ErrNotFound", err)
	}
	if err := repo.DeleteMessagesAfterSeq(ctx, unknown, 0); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("DeleteMessagesAfterSeq on an unknown dialog = %v, want ErrNotFound", err)
	}
	if msgs, err := repo.ListMessages(ctx, unknown); err != nil || len(msgs) != 0 {
		t.Errorf("ListMessages of an unknown dialog = %v, %v, want none", msgs, err)
	}
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}
