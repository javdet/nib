package service

import (
	"testing"
	"time"

	"github.com/javdet/nib/internal/domain"
	"github.com/google/uuid"
)

func TestActivityBrokerFanOut(t *testing.T) {
	broker := NewActivityBroker()
	dialogID := uuid.New()

	ch1, unsub1 := broker.Subscribe(dialogID)
	defer unsub1()
	ch2, unsub2 := broker.Subscribe(dialogID)
	defer unsub2()

	ev := domain.AgentActivity{Kind: domain.ActivityToolsStart, Round: 1, Count: 2}
	broker.Publish(dialogID, ev)

	got1 := <-ch1
	got2 := <-ch2
	if got1 != ev {
		t.Fatalf("subscriber 1: got %+v, want %+v", got1, ev)
	}
	if got2 != ev {
		t.Fatalf("subscriber 2: got %+v, want %+v", got2, ev)
	}
}

func TestActivityBrokerUnsubscribe(t *testing.T) {
	broker := NewActivityBroker()
	dialogID := uuid.New()

	ch, unsub := broker.Subscribe(dialogID)
	unsub()

	broker.Publish(dialogID, domain.AgentActivity{Kind: domain.ActivityToolsEnd})

	select {
	case _, open := <-ch:
		if open {
			t.Fatal("expected closed channel after unsubscribe")
		}
	default:
		t.Fatal("expected channel to be closed after unsubscribe")
	}
}

func TestActivityBrokerPublishNonBlocking(t *testing.T) {
	broker := NewActivityBroker()
	dialogID := uuid.New()

	ch, unsub := broker.Subscribe(dialogID)
	defer unsub()

	ev := domain.AgentActivity{Kind: domain.ActivityToolsStart}
	for i := 0; i < activityChannelBuffer+10; i++ {
		broker.Publish(dialogID, ev)
	}

	done := make(chan struct{})
	go func() {
		broker.Publish(dialogID, ev)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Publish blocked on full subscriber buffer")
	}

	// Drain at least one event so the test does not leak goroutines.
	<-ch
}
