package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/javdet/nib/internal/domain"
)

const sseHeartbeatInterval = 25 * time.Second

type flushWriter interface {
	http.ResponseWriter
	Flush()
}

func (h *DialogHandler) Events() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		rc := http.NewResponseController(w)
		if err := rc.SetWriteDeadline(time.Time{}); err != nil {
			writeError(w, http.StatusInternalServerError, "cannot configure event stream")
			return
		}

		flusher, ok := w.(flushWriter)
		if !ok {
			writeError(w, http.StatusInternalServerError, "streaming not supported")
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		events, unsubscribe := h.chatSvc.SubscribeActivity(id)
		defer unsubscribe()

		heartbeat := time.NewTicker(sseHeartbeatInterval)
		defer heartbeat.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case ev, open := <-events:
				if !open {
					return
				}
				if err := writeSSEEvent(flusher, ev); err != nil {
					return
				}
			case <-heartbeat.C:
				if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}

func writeSSEEvent(w flushWriter, ev domain.AgentActivity) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	w.Flush()
	return nil
}
