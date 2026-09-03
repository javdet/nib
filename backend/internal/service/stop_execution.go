package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/javdet/nib/internal/llm"
)

const StopExecutionToolName = "stop_execution"

var stopExecutionParameters = json.RawMessage(`{"type":"object","properties":{}}`)

// StopExecutionToolDef returns the LLM tool definition for force-stopping the
// running execution.
func StopExecutionToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name: StopExecutionToolName,
		Description: "Force-stop the execution running right now. " +
			"Use it when the operator asks to stop, abort or cancel what is executing, or when they " +
			"want to run something else and the running item is in the way. " +
			"Reports what was stopped, or that nothing was running.",
		Parameters: stopExecutionParameters,
	}
}

// stopExecutionHandler stops whatever holds the execution lease.
//
// Nothing running is an answer, not an error: the operator asking to stop
// something that already finished needs to be told that, not shown a failure.
func (s *ChatService) stopExecutionHandler() localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		lease, err := s.CancelExecution(ctx)
		if errors.Is(err, ErrNoExecutionRunning) {
			return "nothing is executing right now, so there was nothing to stop", nil
		}

		number := lease.Number
		if number == "" {
			number = lease.Key
		}
		if err != nil {
			// The lease is released whatever happened, so execution is
			// unblocked; the leftover container is what the operator needs to
			// hear about.
			return fmt.Sprintf("stopped %s and freed the execution slot, but the container may still be running: %s",
				number, err.Error()), nil
		}
		return fmt.Sprintf("stopped %s; the execution slot is free and its result was recorded as cancelled", number), nil
	}
}
