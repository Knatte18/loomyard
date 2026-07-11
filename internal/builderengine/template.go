// template.go — builder.yaml template accessor and the embedded implementer
// prompt template.
//
// ConfigTemplate provides the default YAML template for builder
// configuration, embedded directly from template.yaml at build time,
// mirroring perchengine's and muxengine's embed-and-accessor pattern.
// ImplementerTemplate provides the implementer prompt one batch's
// implementer session receives, embedded from implementer-template.md and
// filled via internal/stencil at spawn time (spawn.go) — the same
// embed+fill+test pattern burlerengine's review-prompt-template.md uses
// (see the discussion's "prompt templates are embedded stencils,
// co-versioned" decision).

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
