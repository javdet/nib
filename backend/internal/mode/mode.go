package mode

// Modes lists predefined chat modes exposed by the API.
var Modes = []string{"decompose", "plan", "execute", "discuss", "incident"}

// IsValid reports whether name is a predefined mode.
func IsValid(name string) bool {
	for _, m := range Modes {
		if m == name {
			return true
		}
	}
	return false
}
