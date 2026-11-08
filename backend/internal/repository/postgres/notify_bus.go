package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NotifyBus is a message bus over Postgres LISTEN/NOTIFY: Publish is a
// pg_notify on any connection of the publish pool, and Run holds one
// connection of the listen pool LISTENing on every channel, reconnecting when
// it drops. Publishing goes through the ordinary pool rather than the one
// locks pin, so a busy lock pool never stalls an event.
//
// NOTIFY is delivered at most once and only to sessions listening at the time,
// so a message published while the listener is reconnecting is lost to this
// process. Listening says when that is the case, so a caller can fall back to
// delivering its own messages locally.
type NotifyBus struct {
	publish   *pgxpool.Pool
	listen    *pgxpool.Pool
	channels  []string
	listening atomic.Bool
	// pingChannel is this bus's own: every process pings only itself.
	pingChannel string
	lastPing    atomic.Int64
}

// Liveness of the listening session. A half-open connection -- a failover, a
// NAT that dropped it idle -- blocks WaitForNotification forever while
// reporting nothing wrong, so the bus pings itself and reconnects when its own
// pings stop coming back.
const (
	busPingInterval = 15 * time.Second
	busPingTimeout  = 3 * busPingInterval
)

func NewNotifyBus(publish, listen *pgxpool.Pool, channels ...string) *NotifyBus {
	return &NotifyBus{
		publish:     publish,
		listen:      listen,
		channels:    channels,
		pingChannel: "nib_bus_ping_" + strconv.FormatInt(time.Now().UnixNano(), 36),
	}
}

func (b *NotifyBus) Publish(ctx context.Context, channel string, payload []byte) error {
	if _, err := b.publish.Exec(ctx, `SELECT pg_notify($1, $2)`, channel, string(payload)); err != nil {
		return fmt.Errorf("notify %s: %w", channel, err)
	}
	return nil
}

func (b *NotifyBus) Listening() bool {
	return b.listening.Load()
}

// Run delivers every notification to handle until ctx ends. handle runs on the
// listener's own goroutine, so it must not block.
func (b *NotifyBus) Run(ctx context.Context, handle func(channel string, payload []byte)) {
	const (
		minBackoff = time.Second
		maxBackoff = 30 * time.Second
	)
	backoff := minBackoff
	for ctx.Err() == nil {
		err := b.listenOnce(ctx, handle, func() { backoff = minBackoff })
		b.listening.Store(false)
		if ctx.Err() != nil {
			return
		}
		slog.Warn("message bus: listener disconnected, reconnecting", "error", err, "in", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(2*backoff, maxBackoff)
	}
}

func (b *NotifyBus) listenOnce(ctx context.Context, handle func(string, []byte), connected func()) error {
	pooled, err := b.listen.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire listener connection: %w", err)
	}
	// Taken out of the pool for good: a LISTENing session must not be handed
	// to an ordinary query, and it is closed rather than returned.
	conn := pooled.Hijack()
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), unlockTimeout)
		defer cancel()
		_ = conn.Close(closeCtx)
	}()

	for _, ch := range append([]string{b.pingChannel}, b.channels...) {
		if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{ch}.Sanitize()); err != nil {
			return fmt.Errorf("listen %s: %w", ch, err)
		}
	}
	b.listening.Store(true)
	connected()
	slog.Info("message bus: listening", "channels", b.channels)

	// Cancelling the wait closes the session, which is what a dead one needs.
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	b.lastPing.Store(time.Now().UnixNano())
	go b.watchLiveness(sessionCtx, cancel)

	for {
		n, err := conn.WaitForNotification(sessionCtx)
		if err != nil {
			if ctx.Err() == nil && sessionCtx.Err() != nil {
				return fmt.Errorf("no ping came back within %s", busPingTimeout)
			}
			return err
		}
		if n.Channel == b.pingChannel {
			b.lastPing.Store(time.Now().UnixNano())
			continue
		}
		handle(n.Channel, []byte(n.Payload))
	}
}

// watchLiveness pings the listening session through the publish pool and
// ends it when the pings stop arriving.
func (b *NotifyBus) watchLiveness(ctx context.Context, kill context.CancelFunc) {
	tick := time.NewTicker(busPingInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		pingCtx, cancel := context.WithTimeout(ctx, busPingInterval)
		if err := b.Publish(pingCtx, b.pingChannel, nil); err != nil {
			slog.Warn("message bus: ping", "error", err)
		}
		cancel()
		if time.Since(time.Unix(0, b.lastPing.Load())) > busPingTimeout {
			b.listening.Store(false)
			kill()
			return
		}
	}
}
