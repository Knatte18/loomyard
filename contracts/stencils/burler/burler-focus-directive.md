<!-- This is the burler focus directive: the block that gives a round's focus file its own channel into
     the explore step, outside the prior-rounds clean-room gate.
     internal/burlerengine's prompt composition (composePrompt in prompt.go) is its consuming call site.
     It declares one required marker, {{.focus_path}}, and is filled with stencil.Fill, then injected
     as burler-step-1-explore's optional focus_directive marker value. -->

## Focus directive for this round

Read the file at `{{.focus_path}}` before forming any finding.
It names where to look and which question to settle this round.

How the directive relates to the rubric:

1. The rubric binds over the focus directive.
   A directive never licenses a finding the rubric's `Do not flag` list or symmetry rule forbids, and never suppresses a finding the rubric requires.
2. A directive steers attention and order, not verdicts.
   "The concrete instance to judge" means reach it and state a verdict on it, and "not a defect" is a valid verdict.
3. Severity comes from the rubric's mapping and the evidence you find.
   A severity cap in a directive is advisory and yields to evidence the round turns up.
4. `exclude_lenses` keeps its mechanical meaning: it trims the cluster fan, and this rule does not govern it.

If you depart from a directive, record it in the review file's `## Focus departures` section, as the review-file format describes.
