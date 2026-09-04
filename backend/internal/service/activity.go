package service

import (
	"log/slog"
	"sync"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/metrics"
	"github.com/google/uuid"
)

const activityChannelBuffer = 32

type ActivityBroker struct {
	mu          sync.Mutex
	nextID      uint64
	subscribers map[uuid.UUID]map[uint64]chan domain.AgentActivity
}

func NewActivityBroker() *ActivityBroker {
	return &ActivityBroker{
		subscribers: make(map[uuid.UUID]map[uint64]chan domain.AgentActivity),
	}
}

func (b *ActivityBroker) Subscribe(dialogID uuid.UUID) (<-chan domain.AgentActivity, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.nextID++
	id := b.nextID
	ch := make(chan domain.AgentActivity, activityChannelBuffer)

	if b.subscribers[dialogID] == nil {
		b.subscribers[dialogID] = make(map[uint64]chan domain.AgentActivity)
	}
	b.subscribers[dialogID][id] = ch
	metrics.IncSSESubscribers()

	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()

		subs, ok := b.subscribers[dialogID]
		if !ok {
			return
		}
		if c, exists := subs[id]; exists {
			delete(subs, id)
			close(c)
			// Decremented inside the existence check, not at the top: the
			// closure is idempotent by design and Events defers it, so counting
			// unconditionally would drive the gauge negative.
			metrics.DecSSESubscribers()
		}
		if len(subs) == 0 {
			delete(b.subscribers, dialogID)
		}
	}

	return ch, unsubscribe
}

func (b *ActivityBroker) Publish(dialogID uuid.UUID, ev domain.AgentActivity) {
	b.mu.Lock()
	subs := b.subscribers[dialogID]
	channels := make([]chan domain.AgentActivity, 0, len(subs))
	for _, ch := range subs {
		channels = append(channels, ch)
	}
	b.mu.Unlock()

	for _, ch := range channels {
		select {
		case ch <- ev:
			metrics.RecordSSEEvent(string(ev.Kind))
		default:
			metrics.RecordSSEEventDropped(string(ev.Kind))
			slog.Warn("activity event dropped, slow subscriber", "dialog_id", dialogID, "kind", ev.Kind)
		}
	}
}
