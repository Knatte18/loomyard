// template.go — builder.yaml template accessor and the embedded orchestrator
// and implementer prompt templates.
//
// ConfigTemplate provides the default YAML template for builder
// configuration, embedded directly from template.yaml at build time,
// mirroring perchengine's and muxengine's embed-and-accessor pattern.
// OrchestratorTemplate provides the judgment-core prompt the long-lived
// orchestrator session receives, and ImplementerTemplate provides the
// implementer prompt one batch's implementer session receives; both are
// embedded from their own .md asset and filled via internal/stencil at spawn
// time (runlevel.go and spawn.go, respectively) — the same embed+fill+test
// pattern burlerengine's review-prompt-template.md uses (see the
// discussion's "prompt templates are embedded stencils, co-versioned"
// decision).

package builderengine

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for builder
// configuration: the four role model-specs (orchestrator, implementer,
// implementer_oversized, recovery) and the numeric knobs the batch loop and
// its validation gate consult.
func ConfigTemplate() string {
	return configTemplate
}

//go:embed implementer-template.md
var implementerTemplate []byte

// ImplementerTemplate returns the embedded implementer prompt template's raw
// bytes: the caller-required top-level markers are {{.batch_file}},
// {{.batch_name}}, {{.report_path}}, {{.self_fix_cap}}, and
// {{.worktree_root}} (see implementer-template.md's leading banner comment).
// SpawnBatch fills it via stencil.Fill before handing it to shuttle as the
// implementer's Prompt.
func ImplementerTemplate() []byte {
	return implementerTemplate
}

//go:embed orchestrator-template.md
var orchestratorTemplate []byte

// OrchestratorTemplate returns the embedded orchestrator prompt template's
// raw bytes: the caller-required top-level markers are {{.batch_index}},
// {{.progress}}, {{.outcome_path}}, {{.self_fix_cap}}, and
// {{.poll_wait_s}} (see orchestrator-template.md's leading banner comment).
// Run (runlevel.go) fills it via stencil.Fill before handing it to shuttle as
// the orchestrator's Prompt.
func OrchestratorTemplate() []byte {
	return orchestratorTemplate
}
