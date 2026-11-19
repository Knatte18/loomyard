<!-- This is the parent-review section of the Bouncer's judge prompt, for a round that asked the run's parent orch to review it.
     judgeCall reads it through stencilstore and appends it after the judge template's own text.
     It carries no markers. -->

## The parent orch's review

This round asked the run's parent orch to review the round's review and fixer report beside you.
Before you write any of the three output files, wait for the parent's answer:
run `lym loom parent-review await` in the foreground with a shell command timeout above its cap.
While it answers `ready: false`, run it again in a turn that does nothing else, until it answers `ready: true`.

When it answers `ready: true` with `state: done`, read the parent's report at the path its `report` names.
The parent is the operator's delegate, so never overrule it:
every finding of a report that holds findings stands as an open finding of this round.
Such a report rules out `CONVERGED`, and lym refuses that verdict.
Name each of its findings by id in the focus file you write, so the next round's reviewer keeps it.

When the await answers any other state, or the report is satisfied, judge the round without the parent.
