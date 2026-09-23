package skills

import (
	"strings"

	"github.com/javdet/nib/internal/filestore"
)

// Access says how a skill may reach the agent. It is the skill file's `access`
// frontmatter key.
type Access string

const (
	// AccessEnabled lists the skill in the prompt catalog, so the agent may load
	// it on its own judgement as well as on request.
	AccessEnabled Access = "enabled"
	// AccessExplicit keeps the skill out of the catalog; get_skill serves it only
	// once the operator has typed `/name` in the chat.
	AccessExplicit Access = "explicit"
	// AccessDisabled keeps the skill away from the agent entirely.
	AccessDisabled Access = "disabled"
)

// accessKey is the frontmatter key the setting is stored under.
const accessKey = "access"

// ParseAccess reads the access setting from skill content. A missing or
// unrecognised value is enabled: that is what every skill written before the
// setting existed has to keep meaning.
func ParseAccess(content string) Access {
	return normalizeAccess(filestore.ParseFrontmatterFields(content)[accessKey])
}

func normalizeAccess(value string) Access {
	switch Access(strings.ToLower(strings.TrimSpace(value))) {
	case AccessExplicit:
		return AccessExplicit
	case AccessDisabled:
		return AccessDisabled
	default:
		return AccessEnabled
	}
}
