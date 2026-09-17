package service

// The impact fields say what carrying an action out costs the people using the
// system. They sit on a stage step and on a rollback entry alike, and unlike
// `categories` they are operator-facing: each one is rendered as a label above
// the action with its value as the tooltip.
const (
	stepDowntimeKey = "downtime"
	stepDegradedKey = "degraded"
)

const stepDowntimeDescription = "Set only when performing this action makes a request the system normally serves " +
	"fail or be refused: the component is stopped, scaled to zero replicas, write-locked, restarted, or something it " +
	"depends on is down for the duration. One sentence naming exactly what becomes unavailable, for whom, and for " +
	"roughly how long. It is shown to the operator as the tooltip of a red downtime label on this action, so a vague " +
	`"brief downtime" is worse than nothing. Omit the field entirely when the action causes no outage, and never set ` +
	"it together with `degraded`."

const stepDegradedDescription = "Set only when the system keeps serving during this action but measurably worse: " +
	"higher latency, lower throughput, lost redundancy or quorum, resource contention, a backlog building up. One " +
	"sentence naming exactly what gets slower or weaker and for roughly how long. It is shown to the operator as the " +
	"tooltip of an amber degraded label on this action. Omit the field entirely when nothing is affected, and use " +
	"`downtime` instead whenever requests actually fail -- the two are never set on the same action."

// setStepImpactProperties describes the downtime and degraded fields on one
// step-shaped schema object.
//
// Neither is added to `required`, and that is the point: a required string would
// make the model send "" on every action, and a label that appears on every row
// tells the operator nothing. `categories` takes the opposite treatment for the
// opposite reason -- an omitted array there silently costs the executor its
// tools, so the model is made to state the empty case.
//
// A property the schema already describes is left alone. The wording may be an
// operator's, and unlike the category enum there is nothing here that has to
// follow a runtime value.
func setStepImpactProperties(schema map[string]any) {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return
	}

	setStepImpactProperty(props, stepDowntimeKey, stepDowntimeDescription)
	setStepImpactProperty(props, stepDegradedKey, stepDegradedDescription)
}

func setStepImpactProperty(props map[string]any, key, description string) {
	if _, ok := props[key].(map[string]any); ok {
		return
	}
	props[key] = map[string]any{
		"type":        "string",
		"description": description,
	}
}
