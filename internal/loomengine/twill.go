// twill.go renders the two twill blocks of the Discussion-Write prompt: the lens review directive and the write-scope paragraph, filled with the Discussion host's own values and the lenses of the twill-discussion fan.

package loomengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/twilldirective"
)

// twillFan names the burler.yaml fan whose lenses the twill writer's forks review the decision record through.
const twillFan = "twill-discussion"

// twillBlocks returns the twill directive and the twill write-scope paragraph for the Discussion-Write prompt.
// The lenses come from the twillFan fan, resolved against the worktree's burler.yaml and the embedded template.
// An unreadable burler.yaml, or a fan that does not resolve, fails with an error naming the fan and burler.yaml and the way forward.
func twillBlocks(layout *lyxcwd.Location, stencilsDir string) (string, string, error) {
	burlerConfig, err := burlerengine.LoadConfig(layout.AnchorPath())
	if err != nil {
		return "", "", fmt.Errorf("Discussion-Write: burler.yaml unreadable for twill, which needs the %s fan; fix burler.yaml: %w", twillFan, err)
	}
	fan, err := burlerengine.ResolveFan(burlerConfig, twillFan)
	if err != nil {
		return "", "", fmt.Errorf("Discussion-Write: %s fan unresolvable; define %s and its lenses in burler.yaml or restore the template's: %w", twillFan, twillFan, err)
	}
	lenses := make([]twilldirective.Lens, len(fan))
	for i, lens := range fan {
		lenses[i] = twilldirective.Lens{Name: lens.Name, Text: lens.Text}
	}

	readyMarker := DiscussionReadyMarker(layout)
	directive, err := twilldirective.Directive(stencilsDir, twilldirective.Values{
		ReviewedFiles:    []string{DiscussionDecisionRecord(layout), DiscussionSupportLog(layout)},
		ReadyMarker:      readyMarker,
		RubricPath:       stencilstore.Path(stencilsDir, "loom-rubric-discussion-review"),
		Lenses:           lenses,
		FormatStep:       "Step 5",
		SelfCheckStep:    "Step 6",
		SelfCheckCommand: "lyx loom validate-discussion",
		DispositionPlace: "the last turn of the support log's `## Interview` section",
	})
	if err != nil {
		return "", "", err
	}
	writeScope, err := twilldirective.WriteScope(stencilsDir, readyMarker)
	if err != nil {
		return "", "", err
	}
	return directive, writeScope, nil
}
