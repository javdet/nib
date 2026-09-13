package llm

import "encoding/json"

// costFromUsageJSON reads a provider-priced cost off the raw usage object.
//
// OpenRouter puts "cost" there; the OpenAI platform does not, so nil is the
// common answer and has to survive all the way to a NULL column -- nib prices
// nothing itself, and a zero would read as "this call was free" rather than
// "nobody said". Anything unexpected (a string, a null, malformed JSON) is
// treated the same way, because a wrong number here is worse than no number.
func costFromUsageJSON(raw string) *float64 {
	if raw == "" {
		return nil
	}
	var probe struct {
		Cost *float64 `json:"cost"`
	}
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return nil
	}
	if probe.Cost == nil || *probe.Cost < 0 {
		return nil
	}
	return probe.Cost
}
