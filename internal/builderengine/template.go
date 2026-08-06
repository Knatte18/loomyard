// template.go — builder.yaml template accessor and the embedded orchestrator and implementer prompt
// templates.
//
// ConfigTemplate provides the default YAML template for builder configuration, embedded directly
// from template.yaml at build time, mirroring perchengine's and reedengine's embed-and-accessor
// pattern.
// OrchestratorTemplate provides the judgment-core prompt the long-lived orchestrator session
// receives,
// and ImplementerTemplate provides the implementer prompt one batch's implementer session receives;
// both are embedded from their own .md asset and filled via internal/stencil at spawn time
// (runlevel.go and spawn.go, respectively) — the same embed+fill+test pattern burlerengine's
// round-orchestrator-template.md and its three instruction-*-template.md assets use (see the
// discussion's "prompt templates are embedded stencils, co-versioned" decision).

package builderengine

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default builder.yaml template.
func ConfigTemplate() string {
	return configTemplate
}

//go:embed implementer-template.md
var implementerTemplate []byte

// ImplementerTemplate returns the embedded implementer prompt template's raw bytes.
func ImplementerTemplate() []byte {
	return implementerTemplate
}

//go:embed orchestrator-template.md
var orchestratorTemplate []byte

// OrchestratorTemplate returns the embedded orchestrator prompt template's raw bytes.
func OrchestratorTemplate() []byte {
	return orchestratorTemplate
}
