# Orchestrator review — discussion.md

Reviewed against `main` (unchanged in this worktree — only `_mill/discussion.md`/`status.md` exist so far).

## Citation check

The densest of the discussions reviewed so far in this initiative. Verified every concrete file/line/API claim.

| Claim | Status |
|---|---|
| `treadleengine.ParseJudgeVerdict`'s second param `framing judgeFraming` is unexported, so no external caller can construct one | Correct — `judgeverdict.go:47` `type judgeFraming string`, unexported; `ParseJudgeVerdict(content []byte, framing judgeFraming)` at `judgeverdict.go:68` |
| `perchengine`'s doc already records `ParseJudgeVerdict` as never callable externally | Correct, exact — `perchengine/doc.go:319`: "ParseJudgeVerdict was never callable" |
| `ParseHandoff` is callable, but its schema carries `covers_rounds`, treadle-loop-specific | Correct — `handoff.go:61` `func ParseHandoff`, `CoversRounds []int` field at `handoff.go:43`, extensively round-loop-scoped per its own error text ("covers_rounds must be a non-empty list...") |
| `treadleengine/judge.go` `runJudgeCall`, `previousHandoffMarker` | Correct, exact — `judge.go:109` and `:98` |
| `treadleengine/roundfiles.go` — `roundToken`, `artifactPaths`, attempt-suffix scheme | Correct — `roundfiles.go:18,42` |
| `burlerengine/verdict.go` `ParseReview`, `splitFrontmatter` | Correct — `verdict.go:78,139` |
| `treadleengine/handoff.go` `frontmatterProse`, shared `splitFrontmatter` contract note | Correct — `handoff.go:129`, and the comment there explicitly states the shared-contract reasoning the discussion paraphrases |
| `shuttleengine.Spec.validate` rejects a pre-existing `OutputFiles` entry outright | Correct, exact — `spec.go:140-141`: `os.Stat` succeeds → hard error, "a pre-existing file would satisfy the file contract immediately" |
| `internal/shedadapters` already imports `logger`, `shuttleengine`, `perchengine`, `websterengine` | Correct — confirmed all four import sites (`perch.go`, `singlellm.go`, `webster.go`) |
| `contracts/stencils/stencils.go` is the single file where every `//go:embed` directive must live | Correct — file's own header comment states this verbatim |
| `contracts/stencils/registry_test.go` enforces registry completeness | Correct — `TestRegistry_MatchesOnDiskTree`, `TestRegistry_DefaultsAndRelPathAreConsistent` exist |
| `internal/lyxcwd/docslink_test.go` checks markdown links in `manifest/`/`docs/` | File exists, consistent with the claim |
| `Live-Substrate Spawn Observability` invariant name | Correct — `CONSTRAINTS.md:459` |
| `Test Tier Purity Invariant` name | Correct — `CONSTRAINTS.md:487` |
| `shedengine.ProducerDef` fields (`OnStuck`, `OnDone`, `Segment`, `MaxBounces`), `OnDone` empty-value silent-end semantics, `validate()` requiring shared `Segment` | Correct, matches `producer.go` exactly (same file verified in the sibling Burler-round-producer review) |

No inaccurate citation found anywhere in this discussion — every line number, every "X is/is not callable" claim, and every cross-file naming convention checked out exactly, including several that would be easy to get subtly wrong (the unexported-type reason `ParseJudgeVerdict` is uncallable, not just "it's private"; the exact wording `shuttleengine.Spec.validate` uses for its pre-existing-file rejection).

## Design read

**The parser-reuse decision is the standout piece.** Rather than asserting "treadleengine's parsers don't fit," the discussion establishes two independent, verifiable reasons layered on top of each other: `ParseJudgeVerdict` is mechanically uncallable (unexported type in its own signature — not a design preference, a Go-level fact), and `ParseHandoff`, while callable, carries a schema field (`covers_rounds`) whose entire purpose is bounding a judge's read-set across a nested round loop that no longer exists in this design. Layering "impossible" under "possible but wrong" is more rigorous than the discussion needed to be to make its point, and it closes the roadmap's own explicitly-deferred "reuse option to resolve during this task" cleanly.

**The two-mode-by-file-existence design (§"Two modes, told apart by file existence only") correctly reuses an established pattern** rather than inventing a new one — the discussion names the `Discussion-Validate`/`Plan-Validate` findings-discarded-on-`Stuck` gap as the precedent for the same discriminator, which is a genuine consistency argument, not just convenience. The rejected alternative (an in-memory mode flag) is correctly tied back to the same crash-restart-safety reasoning the per-producer bounce budget itself was built on — a coherent line of reasoning running through both this task and its immediate predecessor, not an isolated local choice.

**The fail-safe-toward-`Stuck`-never-`Done` posture is the single most safety-critical decision in the document, and it's treated that way**: stated as a decision, restated in the Discovered-during-discussion constraints list ("`Done` must never be reachable from a degraded path"), and covered by an explicit test requirement ("Assert explicitly in each that the outcome is not `Done` — this is the one property that must never regress"). Three independent places state the same invariant, which is appropriate given what a false-`Done` would mean here — an unreviewed artifact silently passing.

**The `OutputPointer` delta from `PerchProducer` (ledger path on both `Done` and `Stuck` from a judge call, versus perch's always-empty pointer) is correctly flagged as a deliberate divergence requiring explicit doc treatment**, not left as an implicit behavior change a future reader would have to reverse-engineer from a diff against the adapter it supersedes. Good instinct — this is exactly the kind of small, easy-to-miss inconsistency that turns into a "wait, why does this one behave differently" question during review otherwise.

**The exported round-resolution helper (§"An exported round-resolution helper") correctly identifies a cross-task coupling risk before it can manifest**: two independently-implemented halves of one segment (this task's Bouncer, the separate Burler-round-producer task) must agree exactly on how a round number maps to a file name, and the discussion resolves this by having the *first* task to land define and export the convention rather than leaving the second task to reverse-engineer or duplicate it. This is a real cross-task-ordering hazard correctly defused at the design stage.

One soft observation, not a blocker: the ledger's "every entry from the previous ledger is carried forward, open or resolved, never dropped" rule is explicitly stated as prompt-enforced only ("the Go side does not enforce it"). That's a reasonable scope call for this task, but it does mean a misbehaving judge LLM could silently drop a ledger entry with nothing catching it at the Go layer — worth the planner noting as a known soft spot rather than a full gap, since the alternative (Go-side enforcement) would need the parser to diff against the *previous* ledger's key set, which is a real feature, not a one-line addition.

## Verdict

Sound. Nothing here should block moving to Plan. No citation errors to fix — this discussion's citation accuracy is the cleanest of the four reviewed in this initiative so far.
