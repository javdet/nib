package metrics

import "github.com/prometheus/client_golang/prometheus"

type sseMetrics struct {
	subscribers prometheus.Gauge
	published   *prometheus.CounterVec
	dropped     *prometheus.CounterVec
}

func newSSEMetrics() sseMetrics {
	return sseMetrics{
		subscribers: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: "sse",
			Name:      "subscribers",
			Help:      "Open activity-stream subscriptions.",
		}),
		published: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "sse",
			Name:      "events_published_total",
			Help:      "Activity events delivered to a subscriber, by kind.",
		}, []string{"kind"}),
		// A dropped event means the UI silently diverges from server state:
		// the plan view stops updating with nothing but a log line to say so.
		dropped: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "sse",
			Name:      "events_dropped_total",
			Help:      "Activity events dropped because a subscriber was too slow, by kind.",
		}, []string{"kind"}),
	}
}

func (s sseMetrics) collectors() []prometheus.Collector {
	return []prometheus.Collector{s.subscribers, s.published, s.dropped}
}

// IncSSESubscribers records a new activity subscription.
func IncSSESubscribers() {
	if m := active(); m != nil {
		m.sse.subscribers.Inc()
	}
}

// DecSSESubscribers records an activity subscription closing.
func DecSSESubscribers() {
	if m := active(); m != nil {
		m.sse.subscribers.Dec()
	}
}

// RecordSSEEvent records one event delivered to one subscriber.
func RecordSSEEvent(kind string) {
	if m := active(); m != nil {
		m.sse.published.WithLabelValues(kind).Inc()
	}
}

// RecordSSEEventDropped records one event a slow subscriber never received.
func RecordSSEEventDropped(kind string) {
	if m := active(); m != nil {
		m.sse.dropped.WithLabelValues(kind).Inc()
	}
}
