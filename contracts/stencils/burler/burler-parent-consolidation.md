<!-- This is burler's parent consolidation part: the review step's rules for a round that asked the run's parent orch to review.
     The review step carries it after the cluster rules and any assistant part.
     `parentRulesBlock` (internal/burlerengine/parentseat.go) fills it.
     The result becomes the review step's optional parent_rules marker value.
     A round with no parent-review ask renders the review step without it.
     It declares one required marker, {{.parent_origin}}, the origin value a kept parent finding carries. -->
### The parent orch's review

This round asked the run's parent orch to review the same target beside you.
Before you consolidate, and before you write anything to the review file, wait for the parent's answer:
run `lym loom parent-review await` in the foreground with the Bash tool's `timeout` at 3600000.
While it answers `ready: false`, run it again in a turn that does nothing else, until it answers `ready: true`.

When it answers `ready: true` with `state: done`, read the parent's report at the path its `report` names.
Keep every finding of that report as a finding of your review, tagged in its frontmatter with `origin: {{.parent_origin}}`,
with a `parent_ids` list naming the ids of the report's findings it covers, such as `parent_ids: [P1]`:

- Its `severity` is the report item's `severity`, or `BLOCKING` when the item gives none.
- Its `class` is the item's `class` when that is `design`, `scope` or `consistency`, and `design` when the item gives `decision` or no class.
- Its `fix` carries the item's `change`; give it the `location` and `summary` the change points at.

You may merge a parent finding with a duplicate of your own; the merged finding keeps `origin: {{.parent_origin}}` and lists the ids it covers.
Never move a parent finding to `## Rejected`: the parent is the operator's delegate, so a reviewer never overrules it.
Your verdict reflects every parent finding as it does any finding you keep.
lym checks the review against the report and sends it back while a report id is in no `parent_ids` list.

When the await answers any other state, consolidate without the parent.
Below the frontmatter, write one line saying the parent review did not arrive and why:
name the state, and the `reason` the answer gives, when it gives one.
