// twill.go renders the twill blocks of the Discussion-Write and Plan-Write prompts: the lens review directive for either host and the Discussion write-scope paragraph, filled with the host's own values and the lenses of its fan.

package loomengine

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/twilldirective"
)

// twillFan names the burler.yaml fan whose lenses the twill writer's forks review the decision record through.
const twillFan = "twill-discussion"

// twillPlanFan names the burler.yaml fan whose lenses the twill plan writer's forks review the plan through.
const twillPlanFan = "twill-plan"

const (
	// planTwillSpawnPoint is the Plan-Write host's spawn point, rendered verbatim into the twill directive.
	// It overrides the plan template's opening line, which says the producer has no review logic of its own.
	planTwillSpawnPoint = "This lens review is the one review step you run on your own plan: it overrides this prompt's opening line, which says you have no review logic of your own.\n" +
		"Once Step 2's reading is done, run this procedure once, and before you write anything, read the rubric, state the cards you will write in a text reply, one line each with its number, slug and the decisions it carries, and spawn the forks in that same turn.\n" +
		"Write no file under the plan directory before the Agent call.\n" +
		"Add this line to every fork's prompt, after the fork boilerplate and before its lens text, as the one addition to it: \"Report defects only, never a passed check. Give each finding's evidence as the `path:line` or glyph read.\""

	// planTwillRejectionRule is the Plan-Write host's rejection rule, rendered verbatim into the twill directive.
	planTwillRejectionRule = "Fix the plan, or reject the finding. A rejection of a `BLOCKING` or `MEDIUM` finding cites the code read to reject it: a `path:line` or glyph opened in this session and what it shows against the finding. Such a finding you cannot refute from code you have read is fixed. Keeping a waiver, a deferral or an unplanned test against such a finding is a rejection and needs the same citation. A `LOW` finding is rejected with a one-line reason."

	// planTwillSeverityRule is the Plan-Write host's severity rule, rendered verbatim into the twill directive.
	planTwillSeverityRule = "a severity (`BLOCKING` when the plan as written fails its verify or ships a wrong behavior, `MEDIUM` when a decision, criterion or branch goes unreached or untested, `LOW` otherwise)"
)

// twillLenses returns the lenses of the named fan, resolved against burler.yaml's layer stack under dirs and the embedded template.
// row names the spawning row in the errors: an unreadable burler.yaml, or a fan that does not resolve, fails with an error naming the fan and burler.yaml and the way forward.
func twillLenses(dirs configengine.Dirs, fan, row string) ([]twilldirective.Lens, error) {
	burlerConfig, err := burlerengine.LoadConfig(dirs)
	if err != nil {
		return nil, fmt.Errorf("%s: burler.yaml unreadable for twill, which needs the %s fan; fix burler.yaml: %w", row, fan, err)
	}
	resolved, err := burlerengine.ResolveFan(burlerConfig, fan)
	if err != nil {
		return nil, fmt.Errorf("%s: %s fan unresolvable; define %s and its lenses in burler.yaml or restore the template's: %w", row, fan, fan, err)
	}
	lenses := make([]twilldirective.Lens, len(resolved))
	for i, lens := range resolved {
		lenses[i] = twilldirective.Lens{Name: lens.Name, Text: lens.Text}
	}
	return lenses, nil
}

// planTwillDirective returns the twill directive for the Plan-Write prompt, filled with the plan host's values and the lenses of the twillPlanFan fan, resolved through burler.yaml's layer stack under dirs.
// The forks review the overview and every card file its Card Index names, and the writer records the lens review in the plan directory's lens review file.
func planTwillDirective(layout *lyxcwd.Location, dirs configengine.Dirs, stencilsDir string) (string, error) {
	lenses, err := twillLenses(dirs, twillPlanFan, "Plan-Write")
	if err != nil {
		return "", err
	}
	anchorPath := layout.AnchorPath()
	return twilldirective.Directive(stencilsDir, twilldirective.Values{
		ReviewedFiles:         "`" + planparser.PlanOverview(anchorPath) + "` and every card file its Card Index names",
		SpawnPoint:            planTwillSpawnPoint,
		RejectionRule:         planTwillRejectionRule,
		SeverityRule:          planTwillSeverityRule,
		ReadyMarker:           PlanReadyMarker(layout),
		RubricPath:            stencilstore.Path(stencilsDir, "loom-rubric-plan-review"),
		Lenses:                lenses,
		FormatStep:            "Step 3 and Step 4",
		SelfCheckStep:         "Step 5",
		SelfCheckCommand:      "lyx loom validate-plan",
		FinalSelfCheckCommand: "lyx loom validate-plan --lens-review",
		DispositionPlace:      "`" + filepath.Join(planparser.PlanDir(anchorPath), planparser.LensReviewFileName) + "`",
	})
}

// twillBlocks returns the twill directive and the twill write-scope paragraph for the Discussion-Write prompt.
// The lenses come from the twillFan fan, resolved against burler.yaml's layer stack under dirs and the embedded template.
// An unreadable burler.yaml, or a fan that does not resolve, fails with an error naming the fan and burler.yaml and the way forward.
func twillBlocks(layout *lyxcwd.Location, dirs configengine.Dirs, stencilsDir string) (string, string, error) {
	lenses, err := twillLenses(dirs, twillFan, "Discussion-Write")
	if err != nil {
		return "", "", err
	}

	readyMarker := DiscussionReadyMarker(layout)
	directive, err := twilldirective.Directive(stencilsDir, twilldirective.Values{
		ReviewedFiles:         "`" + DiscussionDecisionRecord(layout) + "` and `" + DiscussionSupportLog(layout) + "`",
		SpawnPoint:            "When the design is settled and before you write any output file, run this procedure once:",
		RejectionRule:         "Fix the record, or reject the finding with a one-line reason.",
		SeverityRule:          "a severity (`BLOCKING`, `MEDIUM` or `LOW`)",
		ReadyMarker:           readyMarker,
		RubricPath:            stencilstore.Path(stencilsDir, "loom-rubric-discussion-review"),
		Lenses:                lenses,
		FormatStep:            "Step 5",
		SelfCheckStep:         "Step 6",
		SelfCheckCommand:      "lyx loom validate-discussion",
		FinalSelfCheckCommand: "lyx loom validate-discussion",
		DispositionPlace:      "the last turn of the support log's `## Interview` section",
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
