package service

import (
	"encoding/json"
	"testing"

	"github.com/javdet/nib/internal/domain"
)

func TestParseAskQuestionArgs(t *testing.T) {
	t.Parallel()

	questions, err := parseAskQuestionArgs(map[string]any{
		"questions": []any{
			map[string]any{
				"question": "Which environment?",
				"options":  []any{"staging", "production"},
			},
			map[string]any{
				"question": "Enable TLS?",
			},
		},
	})
	if err != nil {
		t.Fatalf("parseAskQuestionArgs: %v", err)
	}
	if len(questions) != 2 {
		t.Fatalf("len = %d, want 2", len(questions))
	}
	if questions[0].Question != "Which environment?" || len(questions[0].Options) != 2 {
		t.Fatalf("first question: %+v", questions[0])
	}
	if questions[1].Question != "Enable TLS?" || len(questions[1].Options) != 0 {
		t.Fatalf("second question: %+v", questions[1])
	}
}

func TestFormatAskQuestionResult(t *testing.T) {
	t.Parallel()

	questions := []domain.Question{
		{Question: "Which environment?"},
		{Question: "Enable TLS?"},
	}
	got, err := formatAskQuestionResult(questions, []string{"staging", "yes"})
	if err != nil {
		t.Fatalf("formatAskQuestionResult: %v", err)
	}

	var payload struct {
		Answers []struct {
			Question string `json:"question"`
			Answer   string `json:"answer"`
		} `json:"answers"`
	}
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(payload.Answers) != 2 {
		t.Fatalf("answers len = %d", len(payload.Answers))
	}
	if payload.Answers[0].Question != "Which environment?" || payload.Answers[0].Answer != "staging" {
		t.Fatalf("first answer: %+v", payload.Answers[0])
	}
}
