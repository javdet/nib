package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/atomicfile"
	"github.com/javdet/nib/internal/domain"
)

const incidentDialogMode = "incident"

var (
	// ErrNotIncidentDialog is returned when an incident-only operation names a
	// dialog that is not the root of an incident.
	ErrNotIncidentDialog = errors.New("dialog is not an incident")
	// ErrInvalidPostmortemPatch is returned when an operator edit names an
	// unknown field, list or item, or carries a value the field cannot hold.
	ErrInvalidPostmortemPatch = errors.New("invalid post-mortem edit")
)

// Incident statuses, in the order an incident moves through them.
const (
	IncidentStatusOngoing   = "ongoing"
	IncidentStatusMitigated = "mitigated"
	IncidentStatusResolved  = "resolved"
	IncidentStatusClosed    = "closed"
)

// Corrective action statuses. in_plan is set only by the handoff to a plan,
// never by an agent or a plain edit, because it is what links the item to the
// plan that carries it out.
const (
	PostmortemActionTodo    = "todo"
	PostmortemActionInPlan  = "in_plan"
	PostmortemActionDone    = "done"
	PostmortemActionDropped = "dropped"
)

// incidentRole says which part of the post-mortem a writer owns. The operator
// owns all of it; every agent role owns a slice, so two agents running side by
// side never write the same section.
type incidentRole string

const (
	incidentRoleOperator     incidentRole = "operator"
	incidentRoleOrchestrator incidentRole = "orchestrator"
	incidentRoleDigger       incidentRole = "digger"
	incidentRoleSynthesis    incidentRole = "synthesis"
	incidentRoleImpact       incidentRole = "impact"
	incidentRoleClosure      incidentRole = "closure"
)

// Postmortem is the incident document, filled in while the investigation runs.
type Postmortem struct {
	// Version is incremented by every write, so the page can ignore a refetch
	// that lost the race with a newer one.
	Version     int    `json:"version"`
	Status      string `json:"status"`
	Severity    string `json:"severity"`
	Environment string `json:"environment"`
	StartedAt   string `json:"startedAt"`
	DetectedAt  string `json:"detectedAt"`
	MitigatedAt string `json:"mitigatedAt"`
	ResolvedAt  string `json:"resolvedAt"`
	ClosedAt    string `json:"closedAt"`

	Summary    string `json:"summary"`
	Hypothesis string `json:"hypothesis"`
	Impact     string `json:"impact"`
	Closure    string `json:"closure"`

	RootCauses  []PostmortemItem       `json:"rootCauses"`
	Impacted    []PostmortemImpacted   `json:"impacted"`
	Timeline    []PostmortemEvent      `json:"timeline"`
	ShortTerm   []PostmortemAction     `json:"shortTerm"`
	LongTerm    []PostmortemAction     `json:"longTerm"`
	Findings    []PostmortemFinding    `json:"findings"`
	LinkedPlans []PostmortemLinkedPlan `json:"linkedPlans"`

	// Locks lists the scalar fields the operator has edited. An agent cannot
	// overwrite one until the operator hands it back.
	Locks []string `json:"locks"`

	// NextID numbers list items across the whole document, so an id is never
	// reused after a delete and a plan link cannot land on the wrong item.
	NextID    int   `json:"nextId"`
	UpdatedAt int64 `json:"updatedAt"`
}

// PostmortemItem is a root cause.
type PostmortemItem struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Operator bool   `json:"operator,omitempty"`
}

// PostmortemImpacted is a system or area the incident affected.
type PostmortemImpacted struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Environment string `json:"environment,omitempty"`
	Description string `json:"description,omitempty"`
	Severity    string `json:"severity,omitempty"`
	Operator    bool   `json:"operator,omitempty"`
}

// PostmortemEvent is one timeline entry. At is RFC 3339 in UTC when it could
// be parsed and kept as written otherwise: an approximate time is still worth
// recording.
type PostmortemEvent struct {
	ID       string `json:"id"`
	At       string `json:"at"`
	Event    string `json:"event"`
	Source   string `json:"source,omitempty"`
	Operator bool   `json:"operator,omitempty"`
}

// PostmortemAction is a corrective action, short-term or long-term.
type PostmortemAction struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Status   string `json:"status"`
	PlanID   string `json:"planId,omitempty"`
	Operator bool   `json:"operator,omitempty"`
}

// PostmortemFinding is what one Digger established about one system.
type PostmortemFinding struct {
	System         string   `json:"system"`
	Environment    string   `json:"environment,omitempty"`
	Status         string   `json:"status"`
	Summary        string   `json:"summary"`
	Before         string   `json:"before,omitempty"`
	During         string   `json:"during,omitempty"`
	Changes        []string `json:"changes,omitempty"`
	SuspectedCause string   `json:"suspectedCause,omitempty"`
	Confidence     string   `json:"confidence,omitempty"`
	Assumptions    string   `json:"assumptions,omitempty"`
	DialogID       string   `json:"dialogId,omitempty"`
	UpdatedAt      int64    `json:"updatedAt"`
}

// PostmortemLinkedPlan records a plan created from the incident's actions.
type PostmortemLinkedPlan struct {
	PlanID    string   `json:"planId"`
	ActionIDs []string `json:"actionIds"`
	CreatedAt int64    `json:"createdAt"`
}

// PostmortemPatch is one write to the document.
//
// Agents send Set and Lists: a list they send replaces the items they own and
// leaves the operator's alone. The operator sends Set, Ops and Unlock -- item
// operations rather than whole lists, so an edit cannot drop an item an agent
// added after the page last loaded.
type PostmortemPatch struct {
	Set    map[string]string  `json:"set,omitempty"`
	Lists  *PostmortemLists   `json:"lists,omitempty"`
	Ops    []PostmortemItemOp `json:"ops,omitempty"`
	Unlock []string           `json:"unlock,omitempty"`
}

// PostmortemLists carries agent list replacements. A nil list is left alone; an
// empty one removes every item the agent owns.
type PostmortemLists struct {
	RootCauses []PostmortemItem     `json:"rootCauses,omitempty"`
	Impacted   []PostmortemImpacted `json:"impacted,omitempty"`
	Timeline   []PostmortemEvent    `json:"timeline,omitempty"`
	ShortTerm  []PostmortemAction   `json:"shortTerm,omitempty"`
	LongTerm   []PostmortemAction   `json:"longTerm,omitempty"`
}

// PostmortemItemOp is one operator edit of a list item. Fields carries the
// whole item for add and update; which of its fields apply depends on List.
type PostmortemItemOp struct {
	List   string               `json:"list"`
	Op     string               `json:"op"`
	ID     string               `json:"id,omitempty"`
	Fields PostmortemItemFields `json:"fields"`
}

// PostmortemItemFields is the union of every list item's editable fields.
type PostmortemItemFields struct {
	Text        string `json:"text,omitempty"`
	Name        string `json:"name,omitempty"`
	Environment string `json:"environment,omitempty"`
	Description string `json:"description,omitempty"`
	Severity    string `json:"severity,omitempty"`
	At          string `json:"at,omitempty"`
	Event       string `json:"event,omitempty"`
	Source      string `json:"source,omitempty"`
	Status      string `json:"status,omitempty"`
}

const (
	pmListRootCauses = "rootCauses"
	pmListImpacted   = "impacted"
	pmListTimeline   = "timeline"
	pmListShortTerm  = "shortTerm"
	pmListLongTerm   = "longTerm"
)

const (
	pmFieldStatus      = "status"
	pmFieldSeverity    = "severity"
	pmFieldEnvironment = "environment"
	pmFieldStartedAt   = "startedAt"
	pmFieldDetectedAt  = "detectedAt"
	pmFieldMitigatedAt = "mitigatedAt"
	pmFieldResolvedAt  = "resolvedAt"
	pmFieldClosedAt    = "closedAt"
	pmFieldSummary     = "summary"
	pmFieldHypothesis  = "hypothesis"
	pmFieldImpact      = "impact"
	pmFieldClosure     = "closure"
)

var incidentSeverities = []string{"", "sev1", "sev2", "sev3", "sev4"}

var incidentStatuses = []string{IncidentStatusOngoing, IncidentStatusMitigated, IncidentStatusResolved, IncidentStatusClosed}

// pmTimeFields are the scalars normalised to RFC 3339 in UTC.
var pmTimeFields = []string{pmFieldStartedAt, pmFieldDetectedAt, pmFieldMitigatedAt, pmFieldResolvedAt, pmFieldClosedAt}

// pmRoleFields is the slice of the document each role may write. The operator
// is not listed: it may write everything.
var pmRoleFields = map[incidentRole][]string{
	incidentRoleOrchestrator: {
		pmFieldStatus, pmFieldSeverity, pmFieldEnvironment,
		pmFieldStartedAt, pmFieldDetectedAt, pmFieldMitigatedAt, pmFieldResolvedAt,
		pmFieldSummary, pmFieldHypothesis,
		pmListRootCauses, pmListImpacted, pmListTimeline, pmListShortTerm, pmListLongTerm,
	},
	incidentRoleSynthesis: {
		pmFieldSeverity, pmFieldSummary, pmFieldHypothesis,
		pmListRootCauses, pmListTimeline, pmListShortTerm, pmListLongTerm,
	},
	incidentRoleImpact:  {pmFieldImpact, pmListImpacted},
	incidentRoleClosure: {pmFieldClosure},
}

func roleMayWrite(role incidentRole, field string) bool {
	if role == incidentRoleOperator {
		return true
	}
	return slices.Contains(pmRoleFields[role], field)
}

// newPostmortem is the document an incident starts with.
func newPostmortem() Postmortem {
	return Postmortem{Status: IncidentStatusOngoing}
}

// normalize replaces nil slices with empty ones, so the page and the agent read
// [] rather than null.
func (p *Postmortem) normalize() {
	if p.Status == "" {
		p.Status = IncidentStatusOngoing
	}
	if p.RootCauses == nil {
		p.RootCauses = []PostmortemItem{}
	}
	if p.Impacted == nil {
		p.Impacted = []PostmortemImpacted{}
	}
	if p.Timeline == nil {
		p.Timeline = []PostmortemEvent{}
	}
	if p.ShortTerm == nil {
		p.ShortTerm = []PostmortemAction{}
	}
	if p.LongTerm == nil {
		p.LongTerm = []PostmortemAction{}
	}
	if p.Findings == nil {
		p.Findings = []PostmortemFinding{}
	}
	if p.LinkedPlans == nil {
		p.LinkedPlans = []PostmortemLinkedPlan{}
	}
	if p.Locks == nil {
		p.Locks = []string{}
	}
}

func (p *Postmortem) newItemID(prefix string) string {
	p.NextID++
	return fmt.Sprintf("%s-%d", prefix, p.NextID)
}

func (p *Postmortem) locked(field string) bool {
	return slices.Contains(p.Locks, field)
}

func (p *Postmortem) lock(field string) {
	if !p.locked(field) {
		p.Locks = append(p.Locks, field)
	}
}

// scalar returns a pointer to the named scalar field, or nil for an unknown name.
func (p *Postmortem) scalar(field string) *string {
	switch field {
	case pmFieldStatus:
		return &p.Status
	case pmFieldSeverity:
		return &p.Severity
	case pmFieldEnvironment:
		return &p.Environment
	case pmFieldStartedAt:
		return &p.StartedAt
	case pmFieldDetectedAt:
		return &p.DetectedAt
	case pmFieldMitigatedAt:
		return &p.MitigatedAt
	case pmFieldResolvedAt:
		return &p.ResolvedAt
	case pmFieldClosedAt:
		return &p.ClosedAt
	case pmFieldSummary:
		return &p.Summary
	case pmFieldHypothesis:
		return &p.Hypothesis
	case pmFieldImpact:
		return &p.Impact
	case pmFieldClosure:
		return &p.Closure
	}
	return nil
}

// findAction returns the action with id from either list.
func (p *Postmortem) findAction(id string) *PostmortemAction {
	for i := range p.ShortTerm {
		if p.ShortTerm[i].ID == id {
			return &p.ShortTerm[i]
		}
	}
	for i := range p.LongTerm {
		if p.LongTerm[i].ID == id {
			return &p.LongTerm[i]
		}
	}
	return nil
}

// normalizeIncidentTime turns a parseable time into RFC 3339 UTC and keeps
// anything else as written.
func normalizeIncidentTime(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02 15:04"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return v
}

// validateScalar checks and normalises a value for field. The error text is
// written for whoever sent it, the model or the operator.
func validateScalar(field, value string) (string, error) {
	value = strings.TrimSpace(value)
	switch {
	case field == pmFieldStatus:
		value = strings.ToLower(value)
		if !slices.Contains(incidentStatuses, value) {
			return "", fmt.Errorf("status must be one of %s", strings.Join(incidentStatuses, ", "))
		}
	case field == pmFieldSeverity:
		value = strings.ToLower(value)
		if !slices.Contains(incidentSeverities, value) {
			return "", fmt.Errorf("severity must be sev1, sev2, sev3, sev4 or empty")
		}
	case slices.Contains(pmTimeFields, field):
		value = normalizeIncidentTime(value)
		if value != "" {
			if _, err := time.Parse(time.RFC3339, value); err != nil {
				return "", fmt.Errorf("%s must be an RFC 3339 time, such as 2026-10-07T14:05:00Z", field)
			}
		}
	}
	return value, nil
}

func validActionStatus(status string, allowInPlan bool) bool {
	switch status {
	case PostmortemActionTodo, PostmortemActionDone, PostmortemActionDropped:
		return true
	case PostmortemActionInPlan:
		return allowInPlan
	}
	return false
}

// applyPostmortemPatch applies one write by role to doc.
//
// An agent's write is applied field by field: a field it does not own, or one
// the operator has locked, is refused and named in the returned list while the
// rest still lands, so one stale opinion does not cost it a whole update. The
// operator's write is all or nothing and fails with ErrInvalidPostmortemPatch.
func applyPostmortemPatch(doc *Postmortem, patch PostmortemPatch, role incidentRole) ([]string, error) {
	doc.normalize()
	operator := role == incidentRoleOperator
	var refusals []string
	refuse := func(format string, args ...any) error {
		msg := fmt.Sprintf(format, args...)
		if operator {
			return fmt.Errorf("%w: %s", ErrInvalidPostmortemPatch, msg)
		}
		refusals = append(refusals, msg)
		return nil
	}

	if !operator && doc.Status == IncidentStatusClosed && role != incidentRoleClosure {
		return []string{"the incident is closed; the post-mortem can no longer be changed by an agent"}, nil
	}

	for _, field := range patch.Unlock {
		if !operator {
			if err := refuse("only the operator can unlock %s", field); err != nil {
				return nil, err
			}
			continue
		}
		doc.Locks = slices.DeleteFunc(doc.Locks, func(f string) bool { return f == field })
	}

	fields := make([]string, 0, len(patch.Set))
	for f := range patch.Set {
		fields = append(fields, f)
	}
	sort.Strings(fields)
	for _, field := range fields {
		target := doc.scalar(field)
		if target == nil {
			if err := refuse("%s is not a field of the post-mortem", field); err != nil {
				return nil, err
			}
			continue
		}
		if !roleMayWrite(role, field) {
			if err := refuse("%s is not yours to write", field); err != nil {
				return nil, err
			}
			continue
		}
		if !operator && doc.locked(field) {
			_ = refuse("%s is owned by the operator; propose the change in chat instead", field)
			continue
		}
		value, err := validateScalar(field, patch.Set[field])
		if err != nil {
			if err := refuse("%s", err.Error()); err != nil {
				return nil, err
			}
			continue
		}
		if field == pmFieldStatus && value == IncidentStatusClosed && !operator {
			_ = refuse("only Close incident can close it")
			continue
		}
		*target = value
		if operator {
			doc.lock(field)
		}
	}

	if patch.Lists != nil {
		if operator {
			return nil, fmt.Errorf("%w: the operator edits list items with ops", ErrInvalidPostmortemPatch)
		}
		refusals = append(refusals, applyAgentLists(doc, *patch.Lists, role)...)
	}

	if len(patch.Ops) > 0 {
		if !operator {
			refusals = append(refusals, "item operations are the operator's; send whole lists instead")
		} else {
			for _, op := range patch.Ops {
				if err := applyItemOp(doc, op); err != nil {
					return nil, err
				}
			}
		}
	}

	sortTimeline(doc.Timeline)
	return refusals, nil
}

// pmList describes how one list's items are identified and protected.
type pmList[T any] struct {
	prefix string
	id     func(*T) *string
	key    func(*T) string
	// pinned reports whether an agent must leave the item alone.
	pinned func(*T) bool
	// adopt copies onto an agent's restatement of an item what the agent does
	// not own, such as the plan it was handed to.
	adopt func(dst, existing *T)
}

func pmKey(parts ...string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.Join(parts, " ")), " "))
}

var (
	rootCauseList = pmList[PostmortemItem]{
		prefix: "rc",
		id:     func(it *PostmortemItem) *string { return &it.ID },
		key:    func(it *PostmortemItem) string { return pmKey(it.Text) },
		pinned: func(it *PostmortemItem) bool { return it.Operator },
	}
	impactedList = pmList[PostmortemImpacted]{
		prefix: "im",
		id:     func(it *PostmortemImpacted) *string { return &it.ID },
		key:    func(it *PostmortemImpacted) string { return pmKey(it.Name, it.Environment) },
		pinned: func(it *PostmortemImpacted) bool { return it.Operator },
	}
	timelineList = pmList[PostmortemEvent]{
		prefix: "tl",
		id:     func(it *PostmortemEvent) *string { return &it.ID },
		key:    func(it *PostmortemEvent) string { return pmKey(it.At, it.Event) },
		pinned: func(it *PostmortemEvent) bool { return it.Operator },
	}
)

func actionList(prefix string) pmList[PostmortemAction] {
	return pmList[PostmortemAction]{
		prefix: prefix,
		id:     func(it *PostmortemAction) *string { return &it.ID },
		key:    func(it *PostmortemAction) string { return pmKey(it.Text) },
		pinned: func(it *PostmortemAction) bool {
			return it.Operator || it.PlanID != "" || it.Status == PostmortemActionInPlan
		},
		adopt: func(dst, existing *PostmortemAction) {
			if dst.Status == "" {
				dst.Status = existing.Status
			}
		},
	}
}

// replaceAgentItems is an agent's write of a whole list: it keeps every pinned
// item, gives an item the agent restates the id it already had -- so ticks and
// plan links stay attached -- and drops the unpinned items the agent no longer
// sends.
func replaceAgentItems[T any](doc *Postmortem, l pmList[T], existing, incoming []T) []T {
	byKey := make(map[string]int, len(existing))
	for i := range existing {
		byKey[l.key(&existing[i])] = i
	}
	used := make([]bool, len(existing))
	seen := make(map[string]struct{}, len(incoming))
	out := make([]T, 0, len(incoming)+len(existing))
	for _, it := range incoming {
		k := l.key(&it)
		if k == "" {
			continue
		}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		if i, ok := byKey[k]; ok {
			used[i] = true
			if l.pinned(&existing[i]) {
				out = append(out, existing[i])
				continue
			}
			*l.id(&it) = *l.id(&existing[i])
			if l.adopt != nil {
				l.adopt(&it, &existing[i])
			}
			out = append(out, it)
			continue
		}
		*l.id(&it) = doc.newItemID(l.prefix)
		out = append(out, it)
	}
	for i := range existing {
		if !used[i] && l.pinned(&existing[i]) {
			out = append(out, existing[i])
		}
	}
	return out
}

func applyAgentLists(doc *Postmortem, lists PostmortemLists, role incidentRole) []string {
	var refusals []string
	allowed := func(list string, present bool) bool {
		if !present {
			return false
		}
		if !roleMayWrite(role, list) {
			refusals = append(refusals, list+" is not yours to write")
			return false
		}
		return true
	}

	if allowed(pmListRootCauses, lists.RootCauses != nil) {
		items := make([]PostmortemItem, 0, len(lists.RootCauses))
		for _, it := range lists.RootCauses {
			items = append(items, PostmortemItem{Text: strings.TrimSpace(it.Text)})
		}
		doc.RootCauses = replaceAgentItems(doc, rootCauseList, doc.RootCauses, items)
	}
	if allowed(pmListImpacted, lists.Impacted != nil) {
		items := make([]PostmortemImpacted, 0, len(lists.Impacted))
		for _, it := range lists.Impacted {
			items = append(items, PostmortemImpacted{
				Name:        strings.TrimSpace(it.Name),
				Environment: strings.TrimSpace(it.Environment),
				Description: strings.TrimSpace(it.Description),
				Severity:    strings.TrimSpace(it.Severity),
			})
		}
		doc.Impacted = replaceAgentItems(doc, impactedList, doc.Impacted, items)
	}
	if allowed(pmListTimeline, lists.Timeline != nil) {
		items := make([]PostmortemEvent, 0, len(lists.Timeline))
		for _, it := range lists.Timeline {
			items = append(items, PostmortemEvent{
				At:     normalizeIncidentTime(it.At),
				Event:  strings.TrimSpace(it.Event),
				Source: strings.TrimSpace(it.Source),
			})
		}
		doc.Timeline = replaceAgentItems(doc, timelineList, doc.Timeline, items)
	}
	for _, l := range []struct {
		name     string
		prefix   string
		incoming []PostmortemAction
		target   *[]PostmortemAction
	}{
		{pmListShortTerm, "st", lists.ShortTerm, &doc.ShortTerm},
		{pmListLongTerm, "lt", lists.LongTerm, &doc.LongTerm},
	} {
		if !allowed(l.name, l.incoming != nil) {
			continue
		}
		items := make([]PostmortemAction, 0, len(l.incoming))
		for _, it := range l.incoming {
			status := strings.ToLower(strings.TrimSpace(it.Status))
			if status != "" && !validActionStatus(status, false) {
				refusals = append(refusals, fmt.Sprintf("%s: status %q ignored; use todo, done or dropped", l.name, it.Status))
				status = ""
			}
			items = append(items, PostmortemAction{Text: strings.TrimSpace(it.Text), Status: status})
		}
		merged := replaceAgentItems(doc, actionList(l.prefix), *l.target, items)
		for i := range merged {
			if merged[i].Status == "" {
				merged[i].Status = PostmortemActionTodo
			}
		}
		*l.target = merged
	}
	return refusals
}

// applyItemOp applies one operator edit. Every item it adds or touches becomes
// the operator's, so a later agent rewrite of the list leaves it alone.
func applyItemOp(doc *Postmortem, op PostmortemItemOp) error {
	f := op.Fields
	switch op.List {
	case pmListRootCauses:
		return applyOp(doc, rootCauseList, &doc.RootCauses, op, func(it *PostmortemItem) error {
			it.Text = strings.TrimSpace(f.Text)
			it.Operator = true
			return requireField(it.Text, "text")
		})
	case pmListImpacted:
		return applyOp(doc, impactedList, &doc.Impacted, op, func(it *PostmortemImpacted) error {
			it.Name = strings.TrimSpace(f.Name)
			it.Environment = strings.TrimSpace(f.Environment)
			it.Description = strings.TrimSpace(f.Description)
			it.Severity = strings.TrimSpace(f.Severity)
			it.Operator = true
			return requireField(it.Name, "name")
		})
	case pmListTimeline:
		return applyOp(doc, timelineList, &doc.Timeline, op, func(it *PostmortemEvent) error {
			it.At = normalizeIncidentTime(f.At)
			it.Event = strings.TrimSpace(f.Event)
			it.Source = strings.TrimSpace(f.Source)
			it.Operator = true
			return requireField(it.Event, "event")
		})
	case pmListShortTerm, pmListLongTerm:
		target := &doc.ShortTerm
		l := actionList("st")
		if op.List == pmListLongTerm {
			target = &doc.LongTerm
			l = actionList("lt")
		}
		return applyOp(doc, l, target, op, func(it *PostmortemAction) error {
			it.Text = strings.TrimSpace(f.Text)
			status := strings.ToLower(strings.TrimSpace(f.Status))
			switch {
			case status == "":
				if it.Status == "" {
					it.Status = PostmortemActionTodo
				}
			case status == PostmortemActionInPlan && it.Status != PostmortemActionInPlan:
				return fmt.Errorf("%w: an action becomes in_plan only by creating a plan from it", ErrInvalidPostmortemPatch)
			case validActionStatus(status, true):
				it.Status = status
			default:
				return fmt.Errorf("%w: status must be todo, done or dropped", ErrInvalidPostmortemPatch)
			}
			it.Operator = true
			return requireField(it.Text, "text")
		})
	}
	return fmt.Errorf("%w: unknown list %q", ErrInvalidPostmortemPatch, op.List)
}

func requireField(v, name string) error {
	if v == "" {
		return fmt.Errorf("%w: %s is required", ErrInvalidPostmortemPatch, name)
	}
	return nil
}

func applyOp[T any](doc *Postmortem, l pmList[T], items *[]T, op PostmortemItemOp, fill func(*T) error) error {
	switch op.Op {
	case "add":
		var it T
		if err := fill(&it); err != nil {
			return err
		}
		*l.id(&it) = doc.newItemID(l.prefix)
		*items = append(*items, it)
		return nil
	case "update", "delete":
		idx := slices.IndexFunc(*items, func(it T) bool { return *l.id(&it) == op.ID })
		if idx < 0 {
			return fmt.Errorf("%w: no %s item %q", ErrInvalidPostmortemPatch, op.List, op.ID)
		}
		if op.Op == "delete" {
			*items = slices.Delete(*items, idx, idx+1)
			return nil
		}
		return fill(&(*items)[idx])
	}
	return fmt.Errorf("%w: unknown operation %q", ErrInvalidPostmortemPatch, op.Op)
}

// sortTimeline orders the entries with a parseable time chronologically and
// keeps the rest, in the order they came, after them.
func sortTimeline(events []PostmortemEvent) {
	sort.SliceStable(events, func(i, j int) bool {
		ti, ei := time.Parse(time.RFC3339, events[i].At)
		tj, ej := time.Parse(time.RFC3339, events[j].At)
		switch {
		case ei == nil && ej == nil:
			return ti.Before(tj)
		case ei == nil:
			return true
		default:
			return false
		}
	})
}

// upsertFinding replaces the finding for f.System, or appends it.
func upsertFinding(doc *Postmortem, f PostmortemFinding) {
	key := pmKey(f.System)
	for i := range doc.Findings {
		if pmKey(doc.Findings[i].System) == key {
			doc.Findings[i] = f
			return
		}
	}
	doc.Findings = append(doc.Findings, f)
}

func (s *ChatService) postmortemPath(rootID uuid.UUID) string {
	return filepath.Join(s.postmortemsDir, rootID.String()+".json")
}

// ReadPostmortem returns an incident's post-mortem, or the empty document an
// incident starts with when nothing has been written yet.
func (s *ChatService) ReadPostmortem(rootID uuid.UUID) (Postmortem, error) {
	doc := newPostmortem()
	b, err := os.ReadFile(s.postmortemPath(rootID))
	if errors.Is(err, os.ErrNotExist) {
		doc.normalize()
		return doc, nil
	}
	if err != nil {
		return Postmortem{}, fmt.Errorf("read post-mortem: %w", err)
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return Postmortem{}, fmt.Errorf("decode post-mortem: %w", err)
	}
	doc.normalize()
	return doc, nil
}

func (s *ChatService) postmortemMutex(rootID uuid.UUID) *sync.Mutex {
	mu, _ := s.postmortemWriteMu.LoadOrStore(rootID, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

// updatePostmortem is the one read-modify-write of the document. Agents running
// side by side and the operator all go through it, and atomicfile only makes
// each single write atomic.
func (s *ChatService) updatePostmortem(rootID uuid.UUID, mutate func(*Postmortem) error) (Postmortem, error) {
	mu := s.postmortemMutex(rootID)
	mu.Lock()
	defer mu.Unlock()

	doc, err := s.ReadPostmortem(rootID)
	if err != nil {
		return Postmortem{}, err
	}
	before, err := json.Marshal(doc)
	if err != nil {
		return Postmortem{}, fmt.Errorf("encode post-mortem: %w", err)
	}
	if err := mutate(&doc); err != nil {
		return Postmortem{}, err
	}
	doc.normalize()
	// A write that changed nothing -- every field refused, or an agent restating
	// what is already there -- is not a new version, and waking the page for it
	// would only make it refetch the same document.
	if after, err := json.Marshal(doc); err == nil && string(after) == string(before) {
		return doc, nil
	}
	doc.Version++
	doc.UpdatedAt = time.Now().Unix()
	doc.normalize()

	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return Postmortem{}, fmt.Errorf("encode post-mortem: %w", err)
	}
	if err := os.MkdirAll(s.postmortemsDir, 0o755); err != nil {
		return Postmortem{}, fmt.Errorf("create post-mortems directory: %w", err)
	}
	if err := atomicfile.Write(s.postmortemPath(rootID), b); err != nil {
		return Postmortem{}, fmt.Errorf("write post-mortem: %w", err)
	}
	if s.activity != nil {
		s.activity.Publish(rootID, domain.AgentActivity{Kind: domain.ActivityPostmortemUpdated})
	}
	return doc, nil
}

// resolveIncidentRoot checks that id is the root of an incident, the only
// dialog a post-mortem or an investigation belongs to.
func (s *ChatService) resolveIncidentRoot(ctx context.Context, id uuid.UUID) (domain.Dialog, error) {
	if s.dialogRepo == nil {
		return domain.Dialog{}, fmt.Errorf("resolve incident: dialog repository is not configured")
	}
	d, err := s.dialogRepo.GetDialog(ctx, id)
	if err != nil {
		return domain.Dialog{}, fmt.Errorf("resolve incident: %w", err)
	}
	if d.ParentID != nil || d.Mode != incidentDialogMode {
		return domain.Dialog{}, ErrNotIncidentDialog
	}
	return d, nil
}

// IncidentPostmortem serves an incident's post-mortem to the web interface.
func (s *ChatService) IncidentPostmortem(ctx context.Context, id uuid.UUID) (Postmortem, error) {
	if _, err := s.resolveIncidentRoot(ctx, id); err != nil {
		return Postmortem{}, err
	}
	return s.ReadPostmortem(id)
}

// EditIncidentPostmortem applies an operator's edit. Every scalar it sets
// becomes the operator's until they unlock it.
func (s *ChatService) EditIncidentPostmortem(ctx context.Context, id uuid.UUID, patch PostmortemPatch) (Postmortem, error) {
	if _, err := s.resolveIncidentRoot(ctx, id); err != nil {
		return Postmortem{}, err
	}
	return s.updatePostmortem(id, func(doc *Postmortem) error {
		_, err := applyPostmortemPatch(doc, patch, incidentRoleOperator)
		return err
	})
}
