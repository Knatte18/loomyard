// template.go — shuttle.yaml template accessor.
//
// Provides the default YAML template for shuttle configuration, embedded directly from
// template.yaml at build time, mirroring reedengine's ${env:VAR:-default} config pattern.

package shuttleengine

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for shuttle configuration.
// The template uses ${env:VAR:-default} syntax for run_dir, allowing an environment-based override while preserving the default when not set.
// The remaining keys (poll_interval_ms, liveness_every_n_polls, run_timeout_min, startup_timeout_s, deny_python and the other knobs) are plain literals.
func ConfigTemplate() string {
	return configTemplate
}
