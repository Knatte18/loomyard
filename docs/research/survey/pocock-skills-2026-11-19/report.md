# What LoomYard could take from Matt Pocock's skills repository

Survey of 2026-11-19 over the local clone of `mattpocock/skills`:
`main` at `5863806` and the unmerged branch `origin/constrain-skill` at `31964fa`.
Each candidate is judged for lym's runs, its agents' prompts and its own code,
against the skills under `plugins/`, the stencils under `contracts/stencils/`, the recipes and the rules in `pattern/`.

## Method

One pass by one writer, with no subagents and no tests run, over:

- every file under `skills/engineering`, `skills/productivity`, `skills/in-progress` and `skills/misc` on `main`;
- the `docs/` pages, read for their headings and for what each says a skill replaced, renamed or dropped;
- the `constrain` skill and its `CHECKERS.md` on `origin/constrain-skill`.

`skills/deprecated` was out of scope; it is empty on `main`.

The operator's lens, applied to every candidate:

1. **Structure over steering.**
   His method leans on skills and live conversations with agents, which is token heavy and less robust.
   LoomYard turns what works into structure: verbs, recipe rows, gates, guards and stencil rules.
   A candidate scores by whether it can become such structure;
   an idea that only works as a skill steering a live chat scores low.
2. **Has it held?**
   He often reverses what he called best a few weeks later, and deletes retired skills.
   The `git log` of each candidate's files gives its age and any earlier take it replaced.
   A candidate younger than about two weeks, or one that replaced an earlier take, is flagged **unproven**.

Reversals the log and the docs record, for scale:
`tdd` dropped its refactor phase in June 2026 because agents almost never ran it;
`prototype` went from "bin the code" to keeping it on a `prototype/<name>` branch;
`resolving-merge-conflicts` was archived in v1.3.0 with no replacement;
four skills were folded into others (`ubiquitous-language`, `design-an-interface`, `qa`, `request-refactor-plan`),
and three were renamed (`to-prd`, `pathfinder`, `writing-great-skills`).
The `constrain` skill was rewritten twice in the two days after it was added ("drop the facade").

Ages below are the first and last commit dates on the skill's directory.

## Candidates

| Candidate | What it does in his repository | Where it would land in LoomYard | Expected gain | Rough cost | Verdict |
| --- | --- | --- | --- | --- | --- |
| Merge danger (`pr`) | The PR body ends with a one-way or two-way door call and a one-word blast radius. Held since 2026-09-17. | A `## Merge danger` section the darn and Describe stencils write into `_lym/landing/summary.md`; `validate-description` checks it is there; a Code-Review rubric line checks the call against the diff. | The operator sees, before merge, which runs change a record format, a config meaning or other data a revert does not undo. | Small: two stencils, one validator check, one rubric line. | **Adopt** |
| Red-first regression test (`diagnosing-bugs`, `tdd`) | No fix before one command goes red on this bug; the regression test is watched failing before the fix; the confirmed hypothesis goes in the commit message. Red-capable loop since 2026-06-17. | A gate step for a board entry labelled `bug`: a test the diff adds or changes fails on the merge base and passes at HEAD. A `Root-Cause:` trailer, checked like `Loosened-Check:`. | A bug run cannot land a fix whose test would have passed before it; the root cause reaches the landing commit. | Medium: the gate runs the diff's tests once on the base tree in a gate slot; `testreach` names them. | **Adopt** |
| Tautological tests (`tdd`) | A test whose expected value recomputes what the code computes passes by construction; expected values come from an independent source. Held since 2026-06-29. | A Code-Review rubric line under "Test economy"; one rule in `scribe:testing`. A mutation survivor on a line whose test recomputes the value is its evidence. | Catches a test kind neither the redundancy report nor the reach report sees; the mutation report sees only part of it. | Small: one rubric line, one skill rule. | **Adopt** |
| Expand–contract for wide changes (`to-tickets`) | A change that breaks every call site at once is sequenced as expand, migrate in batches, contract, so each batch stays green. Held since 2026-07-07. | A Plan-Review rubric line: a card set that changes a shared record format or signature by hand, which a `Rename` group cannot do, is ordered expand, migrate, contract. | Fewer red Webster batches on wide migrations. | Small: one rubric line. Rare case. | Adapt |
| ADR three-part test (`domain-modeling`) | Record a decision only when it is hard to reverse, surprising without context, and the result of a real trade-off. Held since 2026-05-31. | A Discussion-Review rubric line for each bullet of `## PATTERN changes`: an entry that fails any of the three tests is a finding. | Keeps `PATTERN.md` to rules a writer would otherwise break. | Small: one rubric line. | Adapt |
| Out-of-scope memory (`triage`) | Rejected requests live in one file per concept with the reason; a new request that matches one is surfaced before triage. Held since 2026-04-28. | `lym board` intake compares a new entry's title and brief with `abandoned` entries and names the closest in its envelope. | The operator is not asked again about an idea already dropped. | Medium: a similarity rule in `boardcli` intake, and an abandon reason that is required. | Adapt, low priority |
| Prompt the positive (`writing-for-agents`) | A prohibition makes the banned act more available; state the target act, and keep a ban only as a guardrail paired with its positive. Held since 2026-07-06. | A stencil authoring rule, checkable by a stencil lint that flags a `never` sentence with no positive sentence beside it. | Fewer guard refusals from agents that try the banned command. Unmeasured. | Medium: a lint with false positives, since `PATTERN-refusal-way-forward` wants hard `never` guardrails. | Adapt, measure first |
| Fowler smell baseline (`code-review`) | Twelve named smells as judgement calls on the Standards axis, overridden by repo rules. Held since 2026-06-30. | Code-Review rubric. | Little: `scribe:code-quality` already covers speculative generality, duplication and naming, and more lines raise over-flagging. | Small. | Skip |
| `constrain`: fence a tool's API with an allowlist and a ratchet | Per-object member allowlists and import-edge allowlists in lint rules, messages that name the alternative, a committed baseline that only shrinks. Two days old, unmerged, rewritten twice: **unproven**. | Already in LoomYard: `scankit` import allowlists with no-stale checks in the `*_enforcement_test.go` tests, and refusal messages that name the way forward. | The per-member allowlist is new, but no PATTERN rule needs it now. | n/a | Skip; revisit if it graduates |
| `software-factory`: brakes, throttles, ratchet and fix tasks | Automated checks, automated review and human review as brakes; one trigger at a time; every loop capped before a human. Added 2026-10-31, in-progress: **unproven**. | Already in LoomYard: gates, Burler review, capped Bouncer decisions and operator landings. | None now. | n/a | Skip |
| `chief-of-staff` wiki that writes history | A long session keeps dated, commit-pinned claims in a wiki and runs a retro on one subagent in five. Added 2026-10-05, eight commits in a month: **unproven**. | Already in LoomYard: dated research reports that are never rewritten, `lym retro`. | None now. | n/a | Skip |

Each of these is something LoomYard already does, so it gets one line:

- `retro`: the retro stencil already routes a mechanical fault to a check, a judgement call to a rubric line,
  and an instruction with no effect to a deletion.
- `code-review`'s Spec axis: the Code-Review rubric's scope bullet already flags what the answer key asks for and the diff lacks,
  and the reverse.
- `grilling` and `grill-with-docs`: Discussion already asks questions in batches, each with a recommended answer.
- `implement-spec` and `to-tickets`' frontier: Webster batches and the board's `depends_on` already order work
  and run it in parallel.
- `wayfinder`: board projects, notes and the Sketch already hold decisions apart from implementation work.
- `handoff` and `claude-handoff`: `scribe:handoff` covers the handoff, and lym refuses a headless launch.
- `git-guardrails-claude-code` and `setup-pre-commit`: `internal/shellguard` and `lym gate` cover both.
- `research`: this kind of report, under `docs/research/`.
- `codebase-design` and `improve-codebase-architecture`: `scribe:code-quality` already sets the two-call-site rule,
  and the `cleanup` research and the code-cleanup project already find what to cut.
- `setup-ts-deep-modules`: package boundaries are `scankit` allowlist tests.
- `writing-for-agents`' leading words and no-ops: lym's nicknames are leading words,
  and the retro asks which instruction had no effect.
- `to-spec`'s pre-agreed test seams: `scribe:testing` already puts tests at the public surface,
  and the redundancy and reach reports check them.

These have no use for lym:
`prototype`, `wizard`, `teach`, `to-questionnaire`, `wait-what`, `loop-me`, the `writing-*` skills,
`scaffold-exercises`, `migrate-to-shoehorn`, `setup-matt-pocock-skills` and `ask-matt`.

## The three best candidates

Each is written so it can become a board note.

### 1. Merge danger in every change description

The change description at `_lym/landing/summary.md` gets a `## Merge danger` section with two lines.
**Door:** `one-way` or `two-way`, where one-way means a revert of the landing commit does not undo the change's effect:
a record or board format another binary reads, a config reinterpretation `hubreconcile` applies,
data a run deletes, or anything pushed outward.
**Blast radius:** one word naming what breaks if the change is wrong.
The darn and Describe stencils ask for the section, `lym loom validate-description` refuses a description without it,
and a Code-Review rubric line checks the door against the diff, most suspicious of a two-way call.
The source is `pr` (2026-09-17, two commits, unchanged since 2026-09-24).
His own docs warn that the agent grading its change is the one that wrote it,
which is why the rubric line belongs to Code-Review and not to the writer alone.

### 2. A red-first gate and a root-cause trailer for bug runs

For a board entry labelled `bug`, a gate step proves the fix's test was red before the fix:
it runs the tests the diff adds or changes, which `testreach` names, once on the merge-base tree,
and fails when every one of them passes there.
The writer also adds a `Root-Cause:` trailer to the fix's commit naming the cause it confirmed,
and Code-Review checks it like a `Loosened-Check:` trailer.
The source is `diagnosing-bugs` (red-capable loop since 2026-06-17, twelve commits, last on 2026-10-07)
and `tdd`'s red-before-green rule, which has held since 2026-04-28.
In his repository both are instructions to a live session; here they become a check that no session can skip.
A bug with no seam for a test is a stop the record names, never a silent pass.

### 3. Tautological tests as a Code-Review finding

The Code-Review rubric's "Test economy" list gains one bullet:
a test whose expected value is computed the way the code computes it, by calling the code under test,
a helper it shares or the same formula, is a finding naming the test and the independent value it should assert.
`scribe:testing` gains the matching rule: an expected value comes from a literal, a worked example or the spec.
A mutation survivor on a line whose covering test recomputes its value is evidence for the finding.
The source is `tdd`'s tautological anti-pattern, added on 2026-06-29 and kept through every later change to `tdd`.
Neither the redundancy report nor the reach report can see it:
such a test covers the lines, and kills no mutant of a formula it shares with the code.
