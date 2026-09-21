package service

import "testing"

func TestStripThinking(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "no narration is left alone",
			content: "Deployed centrifugo 5.4.1.",
			want:    "Deployed centrifugo 5.4.1.",
		},
		{
			name:    "block before the answer",
			content: "<thinking>I need the chart version first.</thinking>\n\nDeployed centrifugo 5.4.1.",
			want:    "Deployed centrifugo 5.4.1.",
		},
		{
			name:    "block after the answer",
			content: "Deployed centrifugo 5.4.1.\n<thinking>done</thinking>",
			want:    "Deployed centrifugo 5.4.1.",
		},
		{
			name:    "several blocks, mixed case and attributes",
			content: "<Thinking>one</Thinking>a<thinking id=\"2\">two</thinking >b",
			want:    "ab",
		},
		{
			name:    "narration only",
			content: "<thinking>listing variables next</thinking>",
			want:    "",
		},
		{
			name:    "unclosed block takes the tail with it",
			content: "Deployed.\n<thinking>now I should check the",
			want:    "Deployed.",
		},
		{
			name:    "multiline block",
			content: "<thinking>\nunknowns:\n- version\n</thinking>\nok",
			want:    "ok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := stripThinking(tt.content); got != tt.want {
				t.Errorf("stripThinking(%q) = %q, want %q", tt.content, got, tt.want)
			}
		})
	}
}

func TestAnswerWithoutThinking(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "strips like stripThinking when an answer remains",
			content: "<thinking>checking</thinking>\nThe ACL user exists.",
			want:    "The ACL user exists.",
		},
		{
			name:    "a wholly wrapped answer is unwrapped, not dropped",
			content: "<thinking>The ACL user exists.</thinking>",
			want:    "The ACL user exists.",
		},
		{
			name:    "an unclosed wrapper is unwrapped too",
			content: "<thinking>The ACL user exists.",
			want:    "The ACL user exists.",
		},
		{
			name:    "no narration is left alone",
			content: "The ACL user exists.",
			want:    "The ACL user exists.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := answerWithoutThinking(tt.content); got != tt.want {
				t.Errorf("answerWithoutThinking(%q) = %q, want %q", tt.content, got, tt.want)
			}
		})
	}
}
