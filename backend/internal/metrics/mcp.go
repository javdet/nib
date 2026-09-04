package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type mcpMetrics struct {
	discoveryDuration prometheus.Histogram
	discoveryErrors   *prometheus.CounterVec
	toolsDiscovered   *prometheus.GaugeVec
}

func newMCPMetrics() mcpMetrics {
	return mcpMetrics{
		discoveryDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: Namespace,
			Subsystem: "mcp",
			Name:      "tool_discovery_duration_seconds",
			Help:      "Duration of a full MCP tool discovery pass across every source.",
			Buckets:   llmBuckets,
		}),
		discoveryErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "mcp",
			Name:      "tool_discovery_errors_total",
			Help:      "MCP sources that failed to list their tools, by server.",
		}, []string{"server"}),
		toolsDiscovered: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: "mcp",
			Name:      "tools_discovered",
			Help:      "Tools most recently discovered from each MCP server.",
		}, []string{"server"}),
	}
}

func (m mcpMetrics) collectors() []prometheus.Collector {
	return []prometheus.Collector{m.discoveryDuration, m.discoveryErrors, m.toolsDiscovered}
}

// ObserveMCPDiscovery records how long a discovery pass took. Only the uncached
// passes reach here, so the rate of this metric is the cache miss rate.
func ObserveMCPDiscovery(d time.Duration) {
	if m := active(); m != nil {
		m.mcp.discoveryDuration.Observe(d.Seconds())
	}
}

// RecordMCPSource records one source's discovery result. server names come from
// operator-configured connections and mcp.json entries, so the label space is
// bounded by the install.
func RecordMCPSource(server string, tools int, err error) {
	m := active()
	if m == nil || server == "" {
		return
	}
	if err != nil {
		m.mcp.discoveryErrors.WithLabelValues(server).Inc()
	}
	m.mcp.toolsDiscovered.WithLabelValues(server).Set(float64(tools))
}
