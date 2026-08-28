package domain

// Selection holds the currently active project/environment/cloud/location
// chosen in the UI. Values are stored by name; "any" means no specific choice.
type Selection struct {
	Project     string `json:"project"`
	Environment string `json:"environment"`
	Cloud       string `json:"cloud"`
	Location    string `json:"location"`
}
