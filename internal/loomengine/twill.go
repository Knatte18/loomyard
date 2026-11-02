// twill.go renders the twill lens review directive of the Discussion-Write and Plan-Write prompts, filled with the host's own values and the lenses of its fan.

package loomengine

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lenses"
	"github.com/Knatte18/loomyard/internal/lensreport"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/twilldirective"
)

const (
	// planTwillSpawnOverride and planTwillSpawnForkLine open and close the Plan-Write host's spawn point, which the two spawn-point texts below share.
	// The opening overrides the plan template's opening line, which says the producer has no review logic of its own.
	planTwillSpawnOverride = "This lens review is the one review step you run on your own plan: it overrides this prompt's opening line, which says you have no review logic of your own.\n"
	planTwillSpawnForkLine = "Add this line to every lens fork's prompt, after the fork boilerplate and before its lens text, as the one addition to it: \"Report defects only, never a passed check. Give each finding's evidence as the `path:line` or glyph read.\""

	// planTwillSpawnPoint is the Plan-Write host's spawn point, rendered verbatim into the twill directive.
	planTwillSpawnPoint = planTwillSpawnOverride +
		"Once fork0's plan is read and `lym loom validate-plan` passes per the fork0 steps below, run this procedure once, before you end your turn.\n" +
		planTwillSpawnForkLine

	// planTwillSpawnPointGated is the Plan-Write host's spawn point when a cold fixer clears the plan gate, so the writer runs no validator before the review.
	planTwillSpawnPointGated = planTwillSpawnOverride +
		"Once fork0's plan is read per the fork0 steps below, run this procedure once, before you end your turn.\n" +
		planTwillSpawnForkLine

	// discussionTwillSpawnPoint is the Discussion-Write host's spawn point, rendered verbatim into the twill directive.
	discussionTwillSpawnPoint = "Once you have written all three output files per Step 5 and `lym loom validate-discussion` passes per Step 6, run this procedure once, before you end your turn:"

	// discussionTwillSpawnPointGated is the Discussion-Write host's spawn point when a cold fixer clears the discussion gate, so the writer runs no validator before the review.
	discussionTwillSpawnPointGated = "Once you have written all three output files per Step 5, run this procedure once, before you end your turn:"

	// planTwillSeverityRule is the Plan-Write host's severity rule, rendered verbatim into the twill directive.
	planTwillSeverityRule = "a severity (`BLOCKING` when the plan as written fails its verify or ships a wrong behavior, `MEDIUM` when a decision, criterion or branch goes unreached or untested, `LOW` otherwise)"
)

// twillLenses returns the lenses of the named fan, resolved against lenses.yaml's layer stack under dirs.
// key is the key in module's config file that names the fan, and the errors carry both:
// an unreadable lenses.yaml, a fan that is undefined or holds no lenses, and a fan naming an undefined lens are each refused naming the key and the way forward.
func twillLenses(dirs configengine.Dirs, fan, module, key string) ([]twilldirective.Lens, error) {
	lensesConfig, err := lenses.Load(dirs)
	if err != nil {
		return nil, fmt.Errorf("%s config key %q: lenses.yaml unreadable for the %q fan: %w; fix lenses.yaml, or set the key to \"\" to turn the lens forks off", module, key, fan, err)
	}
	resolved, err := lenses.ResolveFan(lensesConfig, fan)
	if err != nil {
		return nil, fmt.Errorf("%s config key %q: fan %q unresolvable through lenses.yaml: %w; define the fan and its lenses in lenses.yaml, or set the key to \"\" to turn the lens forks off", module, key, fan, err)
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
// With fixerOn set a cold fixer clears the plan gate's findings, so the writer is told to run no validator: the spawn point drops the validator's pass and the directive carries no self-check command.
func planLensDirective(layout *lymcwd.Location, dirs configengine.Dirs, stencilsDir, specsDir, fan string, fixerOn bool) (string, error) {
	fanLenses, err := twillLenses(dirs, fan, tapestryModule, planLensesKey)
	if err != nil {
		return "", err
	}
	spawnPoint, selfCheckCommand, finalSelfCheckCommand := planTwillSpawnPoint, "lym loom validate-plan", "lym loom validate-plan --lens-review"
	if fixerOn {
		spawnPoint, selfCheckCommand, finalSelfCheckCommand = planTwillSpawnPointGated, "", ""
	}
	anchorPath := layout.AnchorPath()
	lensReviewPath := filepath.Join(planparser.PlanDir(anchorPath), planparser.LensReviewFileName)
	return twilldirective.Directive(stencilsDir, twilldirective.Values{
		ReviewedFiles:         "`" + planparser.PlanOverview(anchorPath) + "` and every card file its Card Index names",
		SpawnPoint:            spawnPoint,
		SeverityRule:          planTwillSeverityRule,
		RubricPath:            stencilstore.Path(stencilsDir, "loom-rubric-plan-review"),
		Lenses:                fanLenses,
		FormatStep:            "Step 3 and Step 4",
		SelfCheckStep:         "Step 5",
		SelfCheckCommand:      selfCheckCommand,
		FinalSelfCheckCommand: finalSelfCheckCommand,
		Gated:                 fixerOn,
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
// With fixerOn set a cold fixer clears the discussion gate's findings, so the spawn point drops the validator's pass and the directive carries no final self-check.
func discussionLensDirective(layout *lymcwd.Location, dirs configengine.Dirs, stencilsDir, specsDir, fan string, fixerOn bool) (string, error) {
	fanLenses, err := twillLenses(dirs, fan, loomModule, discussionLensesKey)
	if err != nil {
		return "", err
	}
	spawnPoint, finalSelfCheckCommand := discussionTwillSpawnPoint, "lym loom validate-discussion"
	if fixerOn {
		spawnPoint, finalSelfCheckCommand = discussionTwillSpawnPointGated, ""
	}
	return twilldirective.Directive(stencilsDir, twilldirective.Values{
		ReviewedFiles:         "`" + DiscussionDecisionRecord(layout) + "` and `" + DiscussionSupportLog(layout) + "`",
		RelaunchFiles:         "`" + DiscussionDecisionRecord(layout) + "`, `" + DiscussionSupportLog(layout) + "` and `" + DiscussionRelevant(layout) + "`",
		SpawnPoint:            spawnPoint,
		RejectionRule:         "Fix the record, or reject the finding with a one-line reason.",
		SeverityRule:          "a severity (`BLOCKING`, `MEDIUM` or `LOW`)",
		RubricPath:            stencilstore.Path(stencilsDir, "loom-rubric-discussion-review"),
		Lenses:                fanLenses,
		FormatStep:            "Step 5",
		SelfCheckStep:         "Step 6",
		SelfCheckCommand:      "lym loom validate-discussion",
		FinalSelfCheckCommand: finalSelfCheckCommand,
		Gated:                 fixerOn,
		DispositionPlace:      "the last turn of the support log's `## Interview` section",
		LensReportPath:        lensreport.PathPattern(DiscussionSupportLog(layout)),
		LensReportSpecPath:    stencilstore.Path(specsDir, lensreport.SpecName),
	})
}

// selfCheckBlock returns the body of a writer prompt's self-check step, read at call time from stencilsDir: the gated fragment when a cold fixer clears the writer's gate, otherwise the ungated one.
func selfCheckBlock(stencilsDir string, fixerOn bool, ungatedStencil, gatedStencil string) (string, error) {
	name := ungatedStencil
	if fixerOn {
		name = gatedStencil
	}
	content, err := stencilstore.Read(stencilsDir, name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stencil.StripLeadingComment(string(content))), nil
}
