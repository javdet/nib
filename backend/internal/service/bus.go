package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

// Channels the backend processes talk over.
const (
	// BusChannelActivity carries the activity events behind the SSE streams,
	// so a browser connected to one process sees the work another one does.
	BusChannelActivity = "nib_activity"
	// BusChannelControl carries what only the process holding some in-memory
	// state can act on: a cancel func, a stage waiting on an answer, a job.
	BusChannelControl = "nib_control"
)

// MessageBus delivers a payload to every backend process listening on a
// channel, this one included. Postgres LISTEN/NOTIFY implements it.
type MessageBus interface {
	Publish(ctx context.Context, channel string, payload []byte) error
	// Listening reports whether this process currently receives what is
	// published. While it does not, events are delivered locally instead of
	// being lost on the way back.
	Listening() bool
}

// busPublishTimeout bounds one publish. Activity is published from inside the
// agent loop, which must not stall on a slow database.
const busPublishTimeout = 5 * time.Second

// maxBusPayload keeps a payload under the 8000-byte NOTIFY limit with room for
// the envelope.
const maxBusPayload = 7500

// SetMessageBus routes activity and control messages through bus.
func (s *ChatService) SetMessageBus(bus MessageBus) {
	s.bus = bus
	s.activity.setRelay(func(dialogID uuid.UUID, ev domain.AgentActivity) bool {
		payload, err := json.Marshal(activityEnvelope{DialogID: dialogID, Event: ev})
		if err != nil {
			return false
		}
		ctx, cancel := context.WithTimeout(context.Background(), busPublishTimeout)
		defer cancel()
		if err := bus.Publish(ctx, BusChannelActivity, payload); err != nil {
			slog.Warn("activity relay: publish", "dialog_id", dialogID, "kind", ev.Kind, "error", err)
			return false
		}
		// Published, but only delivered back here if this process is still
		// listening; otherwise the local subscribers get it directly.
		return bus.Listening()
	})
}

type activityEnvelope struct {
	DialogID uuid.UUID            `json:"d"`
	Event    domain.AgentActivity `json:"e"`
}

// Control operations.
const (
	controlCancelLease    = "cancel_lease"
	controlCancelFanout   = "cancel_fanout"
	controlCancelIncident = "cancel_incident"
	controlFanoutAnswer   = "fanout_answer"
	controlCancelJob      = "cancel_job"
)

// controlMessage asks the process To (every process when empty) to act on
// state only it holds.
type controlMessage struct {
	Op     string    `json:"op"`
	To     string    `json:"to,omitempty"`
	ID     uuid.UUID `json:"id,omitempty"`
	CallID string    `json:"callId,omitempty"`
	Answer string    `json:"answer,omitempty"`
	Reason string    `json:"reason,omitempty"`
}

// errNoBus is relayControl without a bus: there is no other process to reach.
var errNoBus = fmt.Errorf("no message bus configured")

func (s *ChatService) relayControl(msg controlMessage) error {
	if s.bus == nil {
		return errNoBus
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("encode control message: %w", err)
	}
	// An answer is the only field of any size. Cut, not refused: the full text
	// is already in the transcript, and the planner needs the gist now. Cut in
	// a loop, since escaping makes the encoded length run ahead of the text's.
	answer := msg.Answer
	for len(payload) > maxBusPayload && answer != "" {
		answer = truncateUTF8(answer, len(answer)-(len(payload)-maxBusPayload)-64)
		msg.Answer = answer + " …(truncated)"
		if payload, err = json.Marshal(msg); err != nil {
			return fmt.Errorf("encode control message: %w", err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), busPublishTimeout)
	defer cancel()
	if err := s.bus.Publish(ctx, BusChannelControl, payload); err != nil {
		return fmt.Errorf("relay %s: %w", msg.Op, err)
	}
	return nil
}

func truncateUTF8(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if n >= len(s) {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// relayLeaseCancel asks the process running a sub-agent to cancel it. The
// lease is already dropped by then, so the slot is free whether or not the
// message arrives; the run records itself cancelled when its context ends.
func (s *ChatService) relayLeaseCancel(_ context.Context, held ExecutionLease) error {
	if err := s.relayControl(controlMessage{Op: controlCancelLease, To: held.Owner, ID: held.token}); err != nil {
		return fmt.Errorf("stop sub-agent in %s: %w", held.Owner, err)
	}
	return nil
}

// HandleBusMessage is the bus listener's callback for every channel.
func (s *ChatService) HandleBusMessage(channel string, payload []byte) {
	switch channel {
	case BusChannelActivity:
		var env activityEnvelope
		if err := json.Unmarshal(payload, &env); err != nil {
			slog.Warn("bus: undecodable activity", "error", err)
			return
		}
		s.activity.Deliver(env.DialogID, env.Event)
	case BusChannelControl:
		var msg controlMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			slog.Warn("bus: undecodable control message", "error", err)
			return
		}
		if msg.To != "" && msg.To != s.replicaID {
			return
		}
		s.handleControl(msg)
	}
}

func (s *ChatService) handleControl(msg controlMessage) {
	switch msg.Op {
	case controlCancelLease:
		if v, ok := s.leaseCancels.LoadAndDelete(msg.ID); ok {
			v.(context.CancelFunc)()
			slog.Info("sub-agent stopped at another process's request", "token", msg.ID)
		}
	case controlCancelFanout:
		if v, ok := s.fanoutRuns.LoadAndDelete(msg.ID); ok {
			v.(*fanoutRunState).cancel()
		}
	case controlCancelIncident:
		if v, ok := s.incidentRuns.Load(msg.ID); ok {
			v.(*incidentRunState).cancel()
		}
	case controlFanoutAnswer:
		if v, ok := s.fanoutWaits.LoadAndDelete(msg.CallID); ok {
			select {
			case v.(*fanoutWait).ch <- msg.Answer:
			default:
			}
		}
	case controlCancelJob:
		if s.commandJobs != nil {
			// A job that already ended is no error here: the operator's view was
			// a moment behind.
			_ = s.commandJobs.CancelByID(msg.ID, msg.Reason)
		}
	default:
		slog.Warn("bus: unknown control operation", "op", msg.Op)
	}
}

// RelayJobCancel asks the process running a command job to cancel it.
func (s *ChatService) RelayJobCancel(owner string, jobID uuid.UUID, reason string) error {
	return s.relayControl(controlMessage{Op: controlCancelJob, To: owner, ID: jobID, Reason: reason})
}

// ReplicaID names this process to the others.
func (s *ChatService) ReplicaID() string { return s.replicaID }
