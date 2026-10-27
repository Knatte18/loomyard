// twill.go renders the twill lens review directive of the Discussion-Write and Plan-Write prompts, filled with the host's own values and the lenses of its fan.

package loomengine

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lenses"
	"github.com/Knatte18/loomyard/internal/lensreport"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/twilldirective"
)

const (
	// planTwillSpawnPoint is the Plan-Write host's spawn point, rendered verbatim into the twill directive.
	// It overrides the plan template's opening line, which says the producer has no review logic of its own.
	planTwillSpawnPoint = "This lens review is the one review step you run on your own plan: it overrides this prompt's opening line, which says you have no review logic of your own.\n" +
		"Once fork0's plan is read and `lym loom validate-plan` passes per the fork0 steps below, run this procedure once, before you end your turn.\n" +
		"Add this line to every lens fork's prompt, after the fork boilerplate and before its lens text, as the one addition to it: \"Report defects only, never a passed check. Give each finding's evidence as the `path:line` or glyph read.\""

	// planTwillSeverityRule is the Plan-Write host's severity rule, rendered verbatim into the twill directive.
	planTwillSeverityRule = "a severity (`BLOCKING` when the plan as written fails its verify or ships a wrong behavior, `MEDIUM` when a decision, criterion or branch goes unreached or untested, `LOW` otherwise)"
)

// twillLenses returns the lenses of the named fan, resolved against lenses.yaml's layer stack under dirs.
// key is the loom.yaml key that names the fan, which the errors carry:
// an unreadable lenses.yaml, a fan that is undefined or holds no lenses, and a fan naming an undefined lens are each refused naming the key and the way forward.
func twillLenses(dirs configengine.Dirs, fan, key string) ([]twilldirective.Lens, error) {
	lensesConfig, err := lenses.Load(dirs)
	if err != nil {
		return nil, fmt.Errorf("loom config key %q: lenses.yaml unreadable for the %q fan: %w; fix lenses.yaml, or set the key to \"\" to turn the lens forks off", key, fan, err)
	}
	resolved, err := lenses.ResolveFan(lensesConfig, fan)
	if err != nil {
		return nil, fmt.Errorf("loom config key %q: fan %q unresolvable through lenses.yaml: %w; define the fan and its lenses in lenses.yaml, or set the key to \"\" to turn the lens forks off", key, fan, err)
	}
	fanLenses := make([]twilldirective.Lens, len(resolved))
	for i, lens := range resolved {
		fanLenses[i] = twilldirective.Lens{Name: lens.Name, Text: lens.Text}
	}
	return fanLenses, nil
}

// planLensDirective returns the twill directive for the Plan-Write lead's prompt, filled with the plan host's values and the lenses of the named fan, resolved through lenses.yaml's layer stack under dirs.
// The forks review the overview and every card file its Card Index names, and the writer records the lens review in the plan directory's lens review file and saves each fork's report beside it, in the format of the spec deployed under specsDir.
// A fan that does not resolve is refused naming the plan_lenses key.
func planLensDirective(layout *lymcwd.Location, dirs configengine.Dirs, stencilsDir, specsDir, fan string) (string, error) {
	fanLenses, err := twillLenses(dirs, fan, planLensesKey)
	if err != nil {
		return "", err
	}
	anchorPath := layout.AnchorPath()
	lensReviewPath := filepath.Join(planparser.PlanDir(anchorPath), planparser.LensReviewFileName)
	return twilldirective.Directive(stencilsDir, twilldirective.Values{
		ReviewedFiles:         "`" + planparser.PlanOverview(anchorPath) + "` and every card file its Card Index names",
		SpawnPoint:            planTwillSpawnPoint,
		SeverityRule:          planTwillSeverityRule,
		RubricPath:            stencilstore.Path(stencilsDir, "loom-rubric-plan-review"),
		Lenses:                fanLenses,
		FormatStep:            "Step 3 and Step 4",
		SelfCheckStep:         "Step 5",
		SelfCheckCommand:      "lym loom validate-plan",
		FinalSelfCheckCommand: "lym loom validate-plan --lens-review",
		DispositionPlace:      "`" + lensReviewPath + "`",
		LensReportPath:        lensreport.PathPattern(lensReviewPath),
		LensReportSpecPath:    stencilstore.Path(specsDir, lensreport.SpecName),
		CriticalJudge: &twilldirective.CriticalJudge{
			GoverningInput:  "the decision record",
			DisputeEvidence: "a `path:line` or glyph, a `PATTERN` entry, a test or caller the change would break, or the `lym loom validate-plan` output",
		},
		WriterFork: &twilldirective.WriterFork{
			ExploreStep: "Step 2",
			InputRecord: "the decision record",
			LastFile:    "`" + planparser.PlanOverview(anchorPath) + "`",
			FilesDir:    "`" + planparser.PlanDir(anchorPath) + "`",
		},
	})
}

// discussionLensDirective returns the twill directive for the Discussion-Write prompt.
// The lenses come from the named fan, resolved against lenses.yaml's layer stack under dirs.
// The writer saves each fork's report beside the support log, in the format of the spec deployed under specsDir.
// An unreadable lenses.yaml, or a fan that does not resolve, is refused naming the discussion_lenses key and the way forward.
func discussionLensDirective(layout *lymcwd.Location, dirs configengine.Dirs, stencilsDir, specsDir, fan string) (string, error) {
	fanLenses, err := twillLenses(dirs, fan, discussionLensesKey)
	if err != nil {
		return "", err
	}
	return twilldirective.Directive(stencilsDir, twilldirective.Values{
		ReviewedFiles:         "`" + DiscussionDecisionRecord(layout) + "` and `" + DiscussionSupportLog(layout) + "`",
		RelaunchFiles:         "`" + DiscussionDecisionRecord(layout) + "`, `" + DiscussionSupportLog(layout) + "` and `" + DiscussionRelevant(layout) + "`",
		SpawnPoint:            "Once you have written all three output files per Step 5 and `lym loom validate-discussion` passes per Step 6, run this procedure once, before you end your turn:",
		RejectionRule:         "Fix the record, or reject the finding with a one-line reason.",
		SeverityRule:          "a severity (`BLOCKING`, `MEDIUM` or `LOW`)",
		RubricPath:            stencilstore.Path(stencilsDir, "loom-rubric-discussion-review"),
		Lenses:                fanLenses,
		FormatStep:            "Step 5",
		SelfCheckStep:         "Step 6",
		SelfCheckCommand:      "lym loom validate-discussion",
		FinalSelfCheckCommand: "lym loom validate-discussion",
		DispositionPlace:      "the last turn of the support log's `## Interview` section",
		LensReportPath:        lensreport.PathPattern(DiscussionSupportLog(layout)),
		LensReportSpecPath:    stencilstore.Path(specsDir, lensreport.SpecName),
	})
}
