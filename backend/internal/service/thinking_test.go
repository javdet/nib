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

// TestStripThinkTag covers the `<think>` spelling DeepSeek and Qwen emit.
// Unlike `<thinking>`, nothing in the prompts asks for it -- the models
// produce it unprompted -- so before it was matched it leaked verbatim into
// transcripts and into the notes a finished action hands to the next one.
func TestStripThinkTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "closed think block",
			in:   "<think>weighing options</think>The answer is 4.",
			want: "The answer is 4.",
		},
		{
			name: "unclosed think block keeps nothing after it",
			in:   "Partial answer.\n<think>cut off mid-nar",
			want: "Partial answer.",
		},
		{
			name: "mixed case",
			in:   "<THINK>noise</THINK>real",
			want: "real",
		},
		{
			name: "mismatched open and close still cancels",
			in:   "<think>noise</thinking>real",
			want: "real",
		},
		{
			name: "thinking still works",
			in:   "<thinking>noise</thinking>real",
			want: "real",
		},
		{
			name: "both spellings in one reply",
			in:   "<think>a</think>keep<thinking>b</thinking>",
			want: "keep",
		},
		{
			name: "prose containing the word think is untouched",
			in:   "I think this is correct, and <thinkers> is not a tag we emit.",
			want: "I think this is correct, and <thinkers> is not a tag we emit.",
		},
		{
			name: "no tags at all",
			in:   "plain answer",
			want: "plain answer",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := stripThinking(tc.in); got != tc.want {
				t.Errorf("stripThinking(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestAnswerWithoutThinkTag covers the final-answer path, where emptiness
// costs more than a stray tag: a reply that was entirely narration is
// unwrapped rather than dropped.
func TestAnswerWithoutThinkTag(t *testing.T) {
	t.Parallel()

	if got := answerWithoutThinking("<think>all of it</think>"); got != "all of it" {
		t.Errorf("answerWithoutThinking() = %q, want %q", got, "all of it")
	}
	if got := answerWithoutThinking("<think>noise</think>answer"); got != "answer" {
		t.Errorf("answerWithoutThinking() = %q, want %q", got, "answer")
	}
}
