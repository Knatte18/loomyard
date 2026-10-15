// twill.go renders the twill lens review directive of the Discussion-Write and Plan-Write prompts, filled with the host's own values and the lenses of its fan.

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

const (
	// planTwillSpawnPoint is the Plan-Write host's spawn point, rendered verbatim into the twill directive.
	// It overrides the plan template's opening line, which says the producer has no review logic of its own.
	planTwillSpawnPoint = "This lens review is the one review step you run on your own plan: it overrides this prompt's opening line, which says you have no review logic of your own.\n" +
		"Once you have written the plan per Step 3 and Step 4 and `lyx loom validate-plan` passes per Step 5, run this procedure once, before you end your turn.\n" +
		"Add this line to every fork's prompt, after the fork boilerplate and before its lens text, as the one addition to it: \"Report defects only, never a passed check. Give each finding's evidence as the `path:line` or glyph read.\""

	// planTwillSeverityRule is the Plan-Write host's severity rule, rendered verbatim into the twill directive.
	planTwillSeverityRule = "a severity (`BLOCKING` when the plan as written fails its verify or ships a wrong behavior, `MEDIUM` when a decision, criterion or branch goes unreached or untested, `LOW` otherwise)"
)

// twillLenses returns the lenses of the named fan, resolved against burler.yaml's layer stack under dirs and the embedded template.
// key is the loom.yaml key that names the fan, which the errors carry:
// an unreadable burler.yaml, a fan that is undefined or holds no lenses, and a fan naming an undefined lens are each refused naming the key and the way forward.
func twillLenses(dirs configengine.Dirs, fan, key string) ([]twilldirective.Lens, error) {
	burlerConfig, err := burlerengine.LoadConfig(dirs)
	if err != nil {
		return nil, fmt.Errorf("loom config key %q: burler.yaml unreadable for the %q fan: %w; fix burler.yaml, or set the key to \"\" to turn the lens forks off", key, fan, err)
	}
	resolved, err := burlerengine.ResolveFan(burlerConfig, fan)
	if err != nil {
		return nil, fmt.Errorf("loom config key %q: fan %q unresolvable through burler.yaml: %w; define the fan and its lenses in burler.yaml, or set the key to \"\" to turn the lens forks off", key, fan, err)
	}
	lenses := make([]twilldirective.Lens, len(resolved))
	for i, lens := range resolved {
		lenses[i] = twilldirective.Lens{Name: lens.Name, Text: lens.Text}
	}
	return lenses, nil
}

// planLensDirective returns the twill directive for the Plan-Write lead's prompt, filled with the plan host's values and the lenses of the named fan, resolved through burler.yaml's layer stack under dirs.
// The forks review the overview and every card file its Card Index names, and the writer records the lens review in the plan directory's lens review file.
// A fan that does not resolve is refused naming the plan_lenses key.
func planLensDirective(layout *lyxcwd.Location, dirs configengine.Dirs, stencilsDir, fan string) (string, error) {
	lenses, err := twillLenses(dirs, fan, planLensesKey)
	if err != nil {
		return "", err
	}
	anchorPath := layout.AnchorPath()
	return twilldirective.Directive(stencilsDir, twilldirective.Values{
		ReviewedFiles:         "`" + planparser.PlanOverview(anchorPath) + "` and every card file its Card Index names",
		SpawnPoint:            planTwillSpawnPoint,
		GoverningInput:        "the decision record",
		DisputeEvidence:       "a `path:line` or glyph, a `PATTERN` entry, a test or caller the change would break, or the `lyx loom validate-plan` output",
		SeverityRule:          planTwillSeverityRule,
		RubricPath:            stencilstore.Path(stencilsDir, "loom-rubric-plan-review"),
		Lenses:                lenses,
		FormatStep:            "Step 3 and Step 4",
		SelfCheckStep:         "Step 5",
		SelfCheckCommand:      "lyx loom validate-plan",
		FinalSelfCheckCommand: "lyx loom validate-plan --lens-review",
		DispositionPlace:      "`" + filepath.Join(planparser.PlanDir(anchorPath), planparser.LensReviewFileName) + "`",
	})
}

// discussionLensDirective returns the twill directive for the Discussion-Write prompt.
// The lenses come from the named fan, resolved against burler.yaml's layer stack under dirs and the embedded template.
// An unreadable burler.yaml, or a fan that does not resolve, is refused naming the discussion_lenses key and the way forward.
func discussionLensDirective(layout *lyxcwd.Location, dirs configengine.Dirs, stencilsDir, fan string) (string, error) {
	lenses, err := twillLenses(dirs, fan, discussionLensesKey)
	if err != nil {
		return "", err
	}
	return twilldirective.Directive(stencilsDir, twilldirective.Values{
		ReviewedFiles:         "`" + DiscussionDecisionRecord(layout) + "` and `" + DiscussionSupportLog(layout) + "`",
		SpawnPoint:            "Once you have written both output files per Step 5 and `lyx loom validate-discussion` passes per Step 6, run this procedure once, before you end your turn:",
		GoverningInput:        "the board entry",
		DisputeEvidence:       "the board entry, the discussion spec, the rubric, a `PATTERN` entry, the `lyx loom validate-discussion` output, or a coarse code read within your Step 2 bound, with no `path:line` required",
		SeverityRule:          "a severity (`BLOCKING`, `MEDIUM` or `LOW`)",
		RubricPath:            stencilstore.Path(stencilsDir, "loom-rubric-discussion-review"),
		Lenses:                lenses,
		FormatStep:            "Step 5",
		SelfCheckStep:         "Step 6",
		SelfCheckCommand:      "lyx loom validate-discussion",
		FinalSelfCheckCommand: "lyx loom validate-discussion",
		DispositionPlace:      "the last turn of the support log's `## Interview` section",
	})
}
