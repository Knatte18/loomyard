// render.go renders the prompt of one Weaver batch attempt.
// The template is read at call time through stencilstore and filled from the batch the attempt builds.

package weaverengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/languageconfig"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// batchTemplateStencil is the name of the batch attempt prompt's stencil.
const batchTemplateStencil = "weaver-template-batch"

// verifyFixBodyStencil is the name of the plan-level verify fixer's body stencil.
const verifyFixBodyStencil = "weaver-body-verify-fix"

// noText is what a required marker takes when it has nothing to say.
const noText = "none"

// BatchPromptInputs is everything RenderBatchPrompt fills the batch template from.
type BatchPromptInputs struct {
	// StencilsDir is the told directory the template and the directive stencils are read from.
	StencilsDir string
	// Engine names the engine the attempt launches on, whose copy of the edit directive the prompt carries.
	Engine string
	// PromptsDir is the directory the rendered prompt is written into.
	PromptsDir string
	// Label names the batch as NN-<slug>, and Attempt is the attempt number; together they name the prompt file.
	Label   string
	Attempt int
	// Cards are the batch's cards in card order.
	Cards []planparser.Card
	// PlanDirDisplay is the spelling of the plan directory the card pointers carry.
	PlanDirDisplay string
	// Gate is the hub's gate config the card gates and the batch gate render from.
	Gate gateslot.Config
	// LastBatch reports that the batch is the partition's last, whose batch gate also runs the main packages.
	LastBatch bool
	// RepoRoot is the worktree root holding PATTERN.md, which the PATTERN directive reads.
	RepoRoot string
	// DecisionRecordPath, JournalPath and ReportPath are the paths the prompt names.
	DecisionRecordPath string
	JournalPath        string
	ReportPath         string
	// FailureDigest and UncommittedPaths describe a retry; blank renders `none`.
	FailureDigest    string
	UncommittedPaths string
	// GateTiers is the hub's tier and gate-list lines; blank renders `none`.
	GateTiers string
	// RenameTools are the hub's structure-aware rename tools; empty renders `none`.
	RenameTools []string
	// ParentName renders the parent directive; blank renders its no-parent variant.
	ParentName string
	// NotePath is the friction note path, blank when friction capture is off.
	NotePath string
}

// RenderBatchPrompt fills the batch template for one attempt, writes it under in.PromptsDir and returns its bytes.
// The edit directive it carries is the copy for in.Engine, the engine the attempt launches on.
// The card pointers carry their card gates, and the batch gate carries the main packages for the last batch.
// A gate config that lacks the `card` or `batch` list refuses with the config's way forward.
// A friction directive that cannot be rendered is logged and left out.
func RenderBatchPrompt(in BatchPromptInputs) ([]byte, error) {
	cards, err := runplan.RenderCardGates(in.Cards, in.PlanDirDisplay, in.Gate)
	if err != nil {
		return nil, err
	}
	batchGate, err := runplan.RenderBatchGate(in.Cards, in.Gate, in.LastBatch)
	if err != nil {
		return nil, err
	}
	patternChanges, err := discussionparser.DeclaredPatternChanges(in.DecisionRecordPath)
	if err != nil {
		return nil, fmt.Errorf("weaver: read the declared PATTERN changes for batch %s: %w", in.Label, err)
	}
	patternDirective, err := pattern.Directive(in.RepoRoot, in.StencilsDir, pattern.RoleImplementer, patternChanges)
	if err != nil {
		return nil, fmt.Errorf("weaver: batch prompt PATTERN directive: %w", err)
	}
	parentDirective, err := parentdirective.Directive(in.StencilsDir, in.ParentName, false)
	if err != nil {
		return nil, fmt.Errorf("weaver: batch prompt parent directive: %w", err)
	}
	editDirective, err := editdirective.Directive(in.StencilsDir, in.Engine)
	if err != nil {
		return nil, fmt.Errorf("weaver: batch prompt edit directive: %w", err)
	}
	frictionDirective, err := friction.Directive(in.NotePath, in.StencilsDir, friction.RoleImplementer)
	if err != nil {
		logger.Warn("weaver: friction directive failed, continuing without one", "stencil", batchTemplateStencil, "error", err)
		frictionDirective = ""
	}

	template, err := stencilstore.Read(in.StencilsDir, batchTemplateStencil)
	if err != nil {
		return nil, fmt.Errorf("weaver: read the batch template: %w", err)
	}
	friction.WarnIfMarkerAbsent(template, batchTemplateStencil, frictionDirective)
	values := map[string]string{
		"cards":                    cards,
		"batch_gate":               batchGate,
		"decision_record":          in.DecisionRecordPath,
		"journal":                  in.JournalPath,
		"report_path":              in.ReportPath,
		"failure_digest":           textOrNone(in.FailureDigest),
		"uncommitted_paths":        textOrNone(in.UncommittedPaths),
		"gate_tiers":               textOrNone(in.GateTiers),
		"rename_tools":             languageconfig.BulletListValue(in.RenameTools),
		"pattern_directive":        patternDirective,
		friction.MarkerName:        frictionDirective,
		parentdirective.MarkerName: parentDirective,
		editdirective.MarkerName:   editDirective,
	}
	prompt, err := stencil.FillOptional(template, values, []string{"pattern_directive", friction.MarkerName, parentdirective.MarkerName})
	if err != nil {
		return nil, fmt.Errorf("weaver: fill the batch template: %w", err)
	}

	if err := os.MkdirAll(in.PromptsDir, 0o755); err != nil {
		return nil, fmt.Errorf("weaver: create prompts dir %s: %w", in.PromptsDir, err)
	}
	promptPath := filepath.Join(in.PromptsDir, fmt.Sprintf("%s-a%d.md", in.Label, in.Attempt))
	if err := os.WriteFile(promptPath, prompt, 0o644); err != nil {
		return nil, fmt.Errorf("weaver: write the batch prompt %s: %w", promptPath, err)
	}
	return prompt, nil
}

// textOrNone returns text, or `none` when it is blank.
func textOrNone(text string) string {
	if strings.TrimSpace(text) == "" {
		return noText
	}
	return text
}
