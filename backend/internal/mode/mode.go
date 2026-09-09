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

// PlanModes are the modes that carry an action plan. It is a whitelist so a mode
// added to Modes later is not treated as a plan until it is named here.
//
// It has to stay in step with postgres.planDialogPredicate and the plan_dialogs
// view created by migration 000025.
var PlanModes = []string{"main", "decompose", "plan", "execute"}

// CarriesPlan reports whether dialogs in this mode carry an action plan.
func CarriesPlan(name string) bool {
	for _, m := range PlanModes {
		if m == name {
			return true
		}
	}
	return false
}

// Default is the mode a dialog gets when none was asked for. It matches the
// chat_dialogs.mode column default.
const Default = "main"
