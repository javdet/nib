package metrics

// Bucket sets are shared and named, because the default prometheus.DefBuckets
// stops at 10 seconds and this service routinely serves requests three orders of
// magnitude slower than that -- every agent request would land in +Inf and every
// latency quantile would be a lie.
var (
	// httpBuckets spans a 5 ms health probe and a 30-minute agent request: the
	// chat, messages, retry and tool-results routes all carry
	// writeDeadline(agentWriteTimeout = 30m) from internal/handler/middleware.go.
	httpBuckets = []float64{.005, .025, .1, .25, .5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600, 1200, 1800}

	// llmBuckets is bounded by llm.timeoutSeconds, 300 in the shipped config.
	llmBuckets = []float64{.25, .5, 1, 2.5, 5, 10, 20, 30, 60, 120, 180, 300, 600}

	// runBuckets covers agent.planFanoutTimeoutMinutes (60) and
	// agent.actionExecTimeoutMinutes (30), which outlive any HTTP request.
	runBuckets = []float64{1, 5, 15, 30, 60, 120, 300, 600, 900, 1500, 1800, 2700, 3600, 5400}

	// roundBuckets covers agent.maxIterations (30) and stageMaxIterations (15):
	// how much of the round budget a turn burns before it stops.
	roundBuckets = []float64{1, 2, 3, 5, 8, 12, 16, 20, 25, 30, 40, 60}

	// turnBuckets counts agent-runner turns reported by a container.
	turnBuckets = []float64{1, 2, 5, 10, 20, 50, 100, 200}
)

// Outcome label values, shared so a dashboard can filter uniformly.
const (
	OutcomeSuccess       = "success"
	OutcomeError         = "error"
	OutcomeMaxIterations = "max_iterations"
	OutcomeToolFailures  = "tool_failures"
	OutcomeAwaitingInput = "awaiting_input"
)
