package mode

// Modes lists predefined chat modes exposed by the API.
//
// main comes first because it is the entry point: it is the mode a new plan
// starts in, and the only one that knows the others can be launched as
// sub-agents.
var Modes = []string{"main", "decompose", "plan", "execute", "discuss", "incident"}

// IsValid reports whether name is a predefined mode.
func IsValid(name string) bool {
	for _, m := range Modes {
		if m == name {
			return true
		}
	}
	return false
}
