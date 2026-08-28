package domain

// ChatRequest is the payload sent by the frontend to initiate an LLM exchange.
type ChatRequest struct {
	Message string `json:"message"`
	Mode    string `json:"mode"`
}

// Question is one clarifying question with optional answer options for ask_question.
type Question struct {
	Question string   `json:"question"`
	Options  []string `json:"options,omitempty"`
}

// ChatResponse carries the LLM's reply back to the frontend.
// When Status is "awaiting_input", Response is empty and Questions holds the pending form.
type ChatResponse struct {
	Response          string     `json:"response,omitempty"`
	Status            string     `json:"status,omitempty"`
	ToolCallID        string     `json:"toolCallId,omitempty"`
	Questions         []Question `json:"questions,omitempty"`
	ActionPlanUpdated bool       `json:"actionPlanUpdated,omitempty"`
}
