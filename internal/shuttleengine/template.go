// template.go — shuttle.yaml template accessor.
//
// Provides the default YAML template for shuttle configuration, embedded
// directly from template.yaml at build time, mirroring muxengine's
// ${env:VAR:-default} config pattern.

package shuttleengine

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for shuttle configuration.
// The template uses ${env:VAR:-default} syntax for the run_dir and claude
// executable path, allowing environment-based overrides while preserving
// defaults when not set; the remaining keys (poll_interval_ms,
// liveness_every_n_polls, run_timeout_min, startup_timeout_s,
// claude_deny_agent_tool, claude_deny_ask_user_question) are plain literals.
func ConfigTemplate() string {
	return configTemplate
}
