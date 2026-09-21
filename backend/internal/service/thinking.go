package service

import (
	"regexp"
	"strings"
)

// The plan and decompose prompts ask the model to open a turn with a
// `<thinking>` block, and some models emit one unprompted. It is scratch
// reasoning addressed to nobody: a provider that has a reasoning channel keeps
// it out of the content, but one that does not leaves it in the reply, where it
// lands in the transcript and in the notes a finished action hands to the next.
var (
	// A closed block, tolerant of attributes and of casing.
	thinkingBlock = regexp.MustCompile(`(?is)<thinking\b[^>]*>.*?</thinking\s*>`)
	// A block the model opened and never closed -- a reply cut off mid-narration
	// would otherwise keep its whole tail.
	thinkingOpen = regexp.MustCompile(`(?is)<thinking\b[^>]*>.*\z`)
	// The markers alone, for the unwrap fallback below.
	thinkingTags = regexp.MustCompile(`(?is)</?thinking\b[^>]*>`)
)

// stripThinking removes thinking narration from an assistant reply. A reply
// that was nothing but narration comes back empty, which is what the rounds
// carrying tool calls want: the content there is commentary on calls the
// operator can already see.
func stripThinking(content string) string {
	if !hasThinking(content) {
		return content
	}
	out := thinkingBlock.ReplaceAllString(content, "")
	out = thinkingOpen.ReplaceAllString(out, "")
	return strings.TrimSpace(out)
}

// answerWithoutThinking is stripThinking for a turn's final answer, where
// emptiness costs more than a stray tag: the answer is what the operator reads
// and what an action sub-agent records as the note later actions inherit. So a
// reply the model wrapped entirely in `<thinking>` is unwrapped instead of
// dropped.
func answerWithoutThinking(content string) string {
	if !hasThinking(content) {
		return content
	}
	if stripped := stripThinking(content); stripped != "" {
		return stripped
	}
	return strings.TrimSpace(thinkingTags.ReplaceAllString(content, ""))
}

func hasThinking(content string) bool {
	return strings.Contains(strings.ToLower(content), "<thinking")
}
