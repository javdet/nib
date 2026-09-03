package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

// TestResolveRootDialogID covers the lineage shapes the orchestrator introduces
// and the two malformed ones parent_id has no constraint against.
func TestResolveRootDialogID(t *testing.T) {
	t.Parallel()

	root := uuid.New()
	child := uuid.New()
	grandchild := uuid.New()
	selfParented := uuid.New()
	loopA, loopB := uuid.New(), uuid.New()

	parent := func(id uuid.UUID) *uuid.UUID { return &id }

	repo := &actionListDialogRepo{dialogs: map[uuid.UUID]domain.Dialog{
		root:         {ID: root, Mode: mainDialogMode},
		child:        {ID: child, Mode: "decompose", ParentID: parent(root)},
		grandchild:   {ID: grandchild, Mode: "execute", ParentID: parent(child)},
		selfParented: {ID: selfParented, ParentID: parent(selfParented)},
		loopA:        {ID: loopA, ParentID: parent(loopB)},
		loopB:        {ID: loopB, ParentID: parent(loopA)},
	}}
	svc := &ChatService{dialogRepo: repo}

	for name, tc := range map[string]struct {
		in   uuid.UUID
		want uuid.UUID
	}{
		"a root resolves to itself": {root, root},
		"one hop":                   {child, root},
		"two hops":                  {grandchild, root},
		"a self-parented row":       {selfParented, selfParented},
		"a mutually-parented pair":  {loopA, loopA},
	} {
		got, err := svc.resolveRootDialogID(context.Background(), tc.in)
		if err != nil {
			t.Fatalf("%s: resolveRootDialogID err = %v", name, err)
		}
		if got != tc.want {
			t.Errorf("%s: resolveRootDialogID = %v, want %v", name, got, tc.want)
		}
	}
}

// A deeper lineage than the cap must still terminate rather than spin: the point
// of the cap is that a malformed tree cannot hang a turn.
func TestResolveRootDialogIDStopsAtTheDepthCap(t *testing.T) {
	t.Parallel()

	dialogs := make(map[uuid.UUID]domain.Dialog)
	ids := make([]uuid.UUID, maxDialogAncestorDepth+3)
	for i := range ids {
		ids[i] = uuid.New()
	}
	for i, id := range ids {
		d := domain.Dialog{ID: id}
		if i+1 < len(ids) {
			next := ids[i+1]
			d.ParentID = &next
		}
		dialogs[id] = d
	}

	svc := &ChatService{dialogRepo: &actionListDialogRepo{dialogs: dialogs}}
	got, err := svc.resolveRootDialogID(context.Background(), ids[0])
	if err != nil {
		t.Fatalf("resolveRootDialogID err = %v", err)
	}
	if got != ids[maxDialogAncestorDepth] {
		t.Errorf("resolveRootDialogID = %v, want the dialog at the cap %v", got, ids[maxDialogAncestorDepth])
	}
}

// dialogToolBinding is what puts a subagent's plan artifacts on the root: the
// transcript stays its own, the plan does not.
func TestDialogToolBindingSplitsTranscriptFromPlan(t *testing.T) {
	t.Parallel()

	root := uuid.New()
	child := uuid.New()
	repo := &actionListDialogRepo{dialogs: map[uuid.UUID]domain.Dialog{
		root:  {ID: root, Mode: mainDialogMode},
		child: {ID: child, Mode: "plan", ParentID: &root},
	}}
	svc := &ChatService{dialogRepo: repo}

	b := svc.dialogToolBinding(context.Background(), repo.dialogs[child])
	if b.dialogID != child {
		t.Errorf("dialogID = %v, want the subagent's own dialog %v", b.dialogID, child)
	}
	if b.planID != root {
		t.Errorf("planID = %v, want the plan root %v", b.planID, root)
	}

	rootBinding := svc.dialogToolBinding(context.Background(), repo.dialogs[root])
	if rootBinding.dialogID != root || rootBinding.planID != root {
		t.Errorf("a root turn binding = %+v, want both ids to be %v", rootBinding, root)
	}
}
