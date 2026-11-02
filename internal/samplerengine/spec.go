// spec.go — the spec the sampler agent's interactive session starts under.

package samplerengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// Role is the agent-name role of the sampler agent's session.
const Role = "sampler"

// stencilName is the registered name of the sampler agent's opening prompt.
const stencilName = "sampler-template-agent"

// skills are the skills the sampler session loads before its prompt: it talks with the operator and writes code and tests.
var skills = []string{"scribe:prose", "scribe:conversation", "scribe:code-quality", "scribe:testing"}

// SessionInputs are the values SessionSpec is told; none is derived here.
type SessionInputs struct {
	// StencilsDir is the directory the stencils are read from.
	StencilsDir string
	// ParentName names the session the agent asks, empty for a run with no parent.
	ParentName string
	// Slug is the run's task slug.
	Slug string
	// WorktreeRoot is the task worktree the agent works in.
	WorktreeRoot string
	// DecisionRecordPath is the decision record the agent reads as the task's brief.
	DecisionRecordPath string
}

// SessionSpec builds the spec the sampler agent's interactive session starts under.
// It refuses an empty StencilsDir, Slug, WorktreeRoot or DecisionRecordPath by name, and an agent model-spec that does not resolve against reg.
// The prompt carries paths only, never file content.
func SessionSpec(in SessionInputs, cfg Config, reg modelspec.Registry) (shuttleengine.Spec, error) {
	required := []struct{ field, value string }{
		{"StencilsDir", in.StencilsDir},
		{"Slug", in.Slug},
		{"WorktreeRoot", in.WorktreeRoot},
		{"DecisionRecordPath", in.DecisionRecordPath},
	}
	for _, r := range required {
		if r.value == "" {
			return shuttleengine.Spec{}, fmt.Errorf("sampler: SessionSpec: %s must not be empty", r.field)
		}
	}

	parsed, err := modelspec.Parse(cfg.Agent)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("sampler: SessionSpec: agent model-spec: %w", err)
	}
	resolved, err := reg.Resolve(parsed)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("sampler: SessionSpec: agent model-spec: %w", err)
	}

	template, err := stencilstore.Read(in.StencilsDir, stencilName)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("sampler: SessionSpec: %w", err)
	}
	parentDirective, err := parentdirective.Directive(in.StencilsDir, in.ParentName, true)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("sampler: SessionSpec: %w", err)
	}
	editDirective, err := editdirective.Directive(in.StencilsDir)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("sampler: SessionSpec: %w", err)
	}
	prompt, err := stencil.Fill(template, map[string]string{
		"slug":                     in.Slug,
		"task_worktree":            in.WorktreeRoot,
		"decision_record_path":     in.DecisionRecordPath,
		parentdirective.MarkerName: parentDirective,
		editdirective.MarkerName:   editDirective,
	})
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("sampler: SessionSpec: fill the sampler prompt: %w", err)
	}

	return shuttleengine.Spec{
		Prompt:      string(prompt),
		ModelChoice: shuttleengine.NewModelChoice(resolved),
		Interactive: true,
		Unawaited:   true,
		DenyPublish: true,
		Role:        Role,
		Segment:     segmentcolor.Darn,
		Skills:      skills,
	}, nil
}
