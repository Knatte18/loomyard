<!-- This is the Raddle-Review rubric, the split perch's rubric.
     The Bouncer row and the review row of the Raddle-Review perch read it.
     The Raddle-Bouncer row interpolates it as the rubric marker value of bouncer-template-seed.md and bouncer-template-judge.md,
     and the Raddle-Burler-Review row interpolates it the same way into internal/burlerengine's review prompt.
     It is a marker VALUE, never a template: its only markers are the ones ReadRubric fills,
     and internal/stencil's StripLeadingComment removes this comment before either consumer sees it. -->

# Raddle-Review rubric

The subject under review is the raddle proposal, `_lym/raddle/proposal.yaml`: the split of the run's decision record into board entries.
The authority the split is judged against is the decision record, `_lym/discussion/decision-record.md`.
Read the whole record, every `### Added after Discussion` entry included, before judging the split.
The record is read-only context: no finding asks to change it.
The proposal's format is `{{.specs_dir}}/raddle/raddle-proposal-spec.md`.
The raddle agent's own stencil is `{{.stencils_dir}}/raddle/raddle-template-agent.md`: read it to learn what the agent was told.

## Already enforced

Every mechanical check of the proposal spec, the perch-mode outcome checks included, runs in the `raddle` gate.
The gate passes every proposal before this round sees it and every fix before the next round sees it.
Do not re-derive those checks.
A malformed field, an unknown recipe or profile, a decision id no part names, a dependency cycle or a shape perch mode refuses is never a finding here.

## Judge the split

Judge the split's judgement, not its format:

- **Every decision lands where it can be built.**
  Every decision of the record, and every `Added after Discussion` entry, lands in a part whose scope and contract can build it.
  An `Added after Discussion` entry carries no decision id, so the gate cannot tell whether a part builds it; this round can.
- **The seams hold.**
  Each part's contract, what it takes, delivers and exports, matches what the parts on its `depends_on` path deliver.
  A part that takes what no part before it delivers is a finding.
- **The cut runs the most parts at once.**
  The split adds no part to gain concurrency: its part count is what the work gives.
  Of the cuts with that part count, it takes the one that lets the most parts run at once:
  the one with a short `depends_on` chain and the most parts with no `depends_on` path between them.
  A part added only so parts run at once is a finding,
  and so is a cut that runs fewer parts at once than another cut with the same part count.
- **No part guesses at another's interface.**
  When one part builds what another uses, `depends_on` chains them, or both parts' bodies fix the same interface.
  Two unchained parts where one uses what the other builds, with no interface fixed in both bodies, is a finding.
- **The order follows the record.**
  The parts follow the record's stated order, or each departure is listed in `order_departures` with its cost.
- **Each part's labels name its modules.**
  A part's `labels` name the module of each package its body names, and no module the part does not change.
  A label for a module the part never touches is a finding, and so is a named package whose module no label names.
- **Each part is bounded.**
  A `darn` part past about 1,500 changed lines, deleted lines left out, or past about eight packages,
  with a seam that holds, is a finding.
  So is a `tapestry` part with no shared interface or data shape that needs one plan.
- **Each part's recipe and profile fit it.**
  `darn` is the default and `light` its profile; `solid` needs a data risk, a wide change or a bundle.
  Each cost line rests on the cells it cites and states the sum of its part's package sizes.
- **Running work is ordered.**
  A part with no `depends_on` on a run in flight or a released task that changes the same package
  or behaviour the part reads, where the overlap is more than a few lines, is a finding.
- **Every part lands whole.**
  Code whose only production caller a later part builds, or a criterion a later part's work makes true, is a finding.
- **The later decision wins.**
  A part that builds what the Sketch's board entry or a later decision has replaced is a finding.
- **The outcome fits the record's scope.**
  One task or a family of parts fits the scope the record states.

## Do not flag

- **Wording and style of a part's title, brief or body.**
  A finding about how a part reads, when what it says is right, is not a finding.
- **A split you would cut differently.**
  A different split that is no better against the judgements above is not a finding.

## Amendment

{{.amendment}}

When the note above is not `none`, this round reviews an amendment: review only the change it names, and report nothing about the settled remainder.

## Finding class

Every finding carries one class: `design`, `scope`, `decision` or `consistency`.
Their generic meanings are in burler's review step; this section does not restate them.
In this segment, `design` means the split is wrong:
a decision no part can build, a seam that does not hold,
a part added only so parts run at once,
a cut that runs fewer parts at once than another cut with the same part count,
labels that do not match the packages a part changes,
a part past its bound, an overlap with running work left unordered, a part that does not land whole,
a part that builds a replaced decision,
or a recipe or profile that does not fit its part.
In this segment, `decision` means a question only the run's parent can settle:
two record entries that contradict each other on what the split must build,
or a landed change or another run that overtakes what the record asks the parts to build.
A choice the raddle agent can make is never `decision`.
Every class is fixed by the fixer in the proposal alone, except `decision`, which goes to the run's parent and never to the fixer.
