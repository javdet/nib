package templates

import _ "embed"

// JobYAML is the default Kubernetes Job template for agent action runs.
//
//go:embed job.yaml.tmpl
var JobYAML string
