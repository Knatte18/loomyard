# Conflict Resolution Brief

Your sole job is to resolve git conflict markers in the listed files, stage each resolved file, and report success.
Do NOT commit.
Do NOT run `git merge --continue` — the SKILL does that after receiving `{"status":"success"}`.

## Task intent

These excerpts describe what THIS branch is trying to accomplish.
When the merge introduces a parent-side change that conflicts with this branch's intent, the resolution preserves THIS branch's intent.
In particular: if a file appears under a batch's `Deletes:` list and the merge introduces a modified version of that file from the parent, the resolution is to delete the file (your branch's intent overrides).
Stage the deletion with `git -C /home/knatte/Code/loomyard/wts/shedadapters-burler-producer rm <file>`.

### From discussion.md

# Discussion: shedadapters: Burler-round producer

```yaml
task: 'shedadapters: Burler-round producer'
slug: shedadapters-burler-producer
status: discussing
parent: main
```

## Problem

`Shed` drives a flat, ordered producer list and routes purely on two verdicts, `Done` and `Stuck`, with per-producer `OnDone`/`OnStuck` targets and a per-producer bounce budget (shipped in the immediately preceding roadmap item, commit `fa71d2a9`).
A review loop therefore no longer needs an engine that owns its own round loop:
the loop is `Shed`'s own bounce mechanism between two rows in one segment — a round producer that runs exactly one review+fix round, and a judge (the `Bouncer`) that decides whether another round is needed.

Today the only way a `Shed`-based product can review anything is `shedadapters.NewPerchProducer`, which wraps a whole `perchengine` block — and `perchengine` is a thin configuration layer over `internal/treadleengine`, whose `Engine.Run` owns its *own* round loop, milestone ladder, and progress judge.
Nesting that inside `Shed` means two round loops stacked on top of each other, with the outer one blind to the inner one's rounds.
This task builds the first half of the replacement: a reusable `ShedProducer` in `internal/shedadapters` that wraps `internal/burlerengine`'s existing one-round (A-review → B-fix) API directly as a single `Shed` row, bypassing `perch`/`treadleengine` entirely.

**Why now:** the `Shed` per-producer bounce budget and explicit `OnDone` routing landed in the previous roadmap item, so the outer loop this producer depends on exists.
The `Bouncer` (the judge half) is the immediately following roadmap item, and the three `loom` review-producer tasks depend on both.
`NewPerchProducer` is not a third sibling of this producer — this item and `Bouncer` are what supersede it.

## Scope

**In:**

- A new `BurlerProducer` in `internal/shedadapters` (new file `burler.go`, plus its test file), implementing `shedengine.ShedProducer` over `internal/burlerengine`.
- A narrow `BurlerRunner` seam in the same package, with a compile-time assertion that `*burlerengine.Engine` satisfies it.
- From-disk round resolution and this producer's own round-artifact path convention inside its told run dir, including stale-artifact archiving before every attempt via the package's existing `archiveStaleOutputs`.
- A minimal, self-contained bounded-retry slice (one deterministic retry on `died`/`timeout`) owned by the adapter.
- The structured next-round focus file's Go-side contract: an exported struct and a fail-safe parser in `internal/shedadapters`, which the `Bouncer` task will marshal the same struct into.
- A new exported `Profile.ClusterExclude []string` field in `internal/burlerengine`, filtering the resolved cluster fan per call.
- Doc updates in the same commit: `internal/shedadapters/doc.go`, `internal/burlerengine/doc.go`, `manifest/designs/shed.md`'s Engine-adapters section, **its status blockquote at `manifest/designs/shed.md:3`, and its `## Process` section's adapter-count sentence at `manifest/designs/shed.md:324`** (both present-tense about what the adapters are, so both carry the stale count) (a fourth site carrying the stale count — it is present-tense about what the adapters are, not purely historical, so it is updated rather than left standing), `docs/overview.md`'s three "three adapters" sites (`docs/overview.md:235,316,318` — the module-tree line, the prose sentence, and the status-table sentence), and `manifest/roadmap.md` (a Planned item completing).

**Out:**

- The `Bouncer` producer itself — the next roadmap item. This task defines only the *read* side of the focus-file contract plus the shared struct; it writes no focus file and ships no `Bouncer`.
  **One exception, and it is deliberate:** `manifest/roadmap.md:23` currently specifies the `Bouncer`'s seed-vs-judge test as "if the round producer's report artifact for the current round does not exist yet" — a single-artifact predicate, which is exactly the review-only-orphan wedge the pair predicate closes.
  That sentence is amended in this task's commit to the pair predicate, because the `Bouncer` task will implement from it and would otherwise reintroduce the wedge this task just designed out.
  The amendment is a correction to a still-Planned item's specification, distinct from (and in addition to) the roadmap's ordinary lifecycle move for this task's own completed item.
  The pair predicate is *also* recorded durably in `internal/shedadapters/doc.go` and `manifest/designs/shed.md`'s Engine-adapters section as the binding two-sided contract, so it survives independently of the roadmap entry, which is deleted when the `Bouncer` item completes.
- Any wiring into `internal/loomshed` or `loom`'s producer list. `loom.md` does not mention `Burler`/`Bouncer` today; the three `loom` review-producer roadmap items own that, and they depend on the `Bouncer` too.
- Retiring or changing `shedadapters.NewPerchProducer`, `internal/perchengine`, `internal/treadleengine`, or the `lyx perch run|pause` CLI. Their fate is the Someday `Bouncer → Perch` item and stays deferred.
- Porting `treadleengine`'s asking-triage, its progress judge, its milestone ladder, its handoff/ledger machinery, or its pre-round targeting.
- Any change to `internal/shedengine`. The producer adapts onto the existing seam unchanged.
- Any LLM-prompt or rubric content. Rubrics are the three `loom` review-producer tasks' business; this producer is told its `burlerengine.Profile`.
- Any new CLI verb or Cobra command.

## Decisions

### Always `Stuck`, never `Done` — and only for a *completed* round

- Decision: `BurlerProducer.Call` returns `shedengine.Stuck` on every successful round, never `Done`.
  Its own doc comment states explicitly that this `Stuck` is a routine hand-off signal to the segment's `Bouncer` via `OnStuck`, never a real stuck condition, so an operator reading a status file or a reviewer reading the code is never misled.
  Correspondingly, a round that did **not** reach `shuttleengine.OutcomeDone` (after the bounded retry below) is a hard error, never `Stuck`.
- Rationale: a round producer has no independent notion of "finished" — only the judge does — so `Stuck` is a pure reuse of the existing routing primitive.
  The producer is reached only via an explicit `OnStuck`/`OnDone` jump, never via `Done`-fallthrough, so its physical position in the producer list carries no routing meaning and may sit wherever reads best.
  The "only a completed round" half is load-bearing rather than stylistic: the `Bouncer` tells its seed call from its judge call by whether the round's review artifact exists on disk, so a failed round returning `Stuck` with no review file written would be silently misread as a seed call and would re-seed instead of surfacing the failure.
- Consequence worth naming: because this producer never returns `Done`, its `Shed` bounce episode never resets for the life of the run — `episodeStuckCount` walks history back to the producer's own most recent `Done` and finds none (`internal/shedengine/run.go:275`) — so its `effectiveMaxBounces` (its own `ProducerDef.MaxBounces`, else `Shed.MaxBounces`, else the internal default of ten, `internal/shedengine/shed.go:21-30,61-63`) stops being a bounce-loop guard and becomes a cap on review rounds.
  **But this row's budget is not, by itself, the segment's cap, and a reader must not be told it is.** The `Bouncer` row has exactly the same unresetting property — it returns `Done` only when it approves, which exits the segment — and per the roadmap it is the segment's *entry point*, so its `Stuck` sequence is the seed call plus one per rejection: always one ahead of this producer's round count.
  With equal budgets the `Bouncer` therefore exhausts first and blocks the segment. **The segment's round cap is the smaller of the two rows' budgets, and the `Bouncer`'s normally binds; raising the cap means raising both rows together.**
  The boundary is exact, and `Shed` pins it in its own comment (`internal/shedengine/run.go:197-199`): the check compares the pre-append history, so a budget of `N` permits `N` bounce-backs and blocks on the `N+1`th `Stuck`.
  With the default ten on both rows, the segment runs ten complete rounds and blocks when the `Bouncer` would reject an eleventh time.
  This coupling goes in the producer's own doc comment as well as here — stated as the two-row relationship, never as "raise this row's `MaxBounces`", which would be advice that does not work — because a reader who knows `MaxBounces` only as a bounce-loop guard elsewhere in `Shed` would not expect it to be the round cap at all.
- Rejected: returning `Done` when `burlerengine` reports `VerdictApproved` — that would make the round producer its own judge, which is exactly the split this segment shape exists to avoid, and would also mean the producer self-grades a fix its own round made (see the Review Round Invariant's no-self-grading rule).
  Also rejected: mapping a non-done outcome onto `Stuck` for the `Bouncer` to sort out, for the seed-call-confusion reason above.

### Non-done outcomes: one deterministic retry, no triage port

- Decision: the adapter owns a minimal, self-contained bounded-retry slice — attempt 1, then, on `shuttleengine.OutcomeDied` or `OutcomeTimeout`, exactly one retry (attempt 2) with a `logger.Warn` naming the outcome and session id.
  A second consecutive `died`/`timeout` is a hard error naming both attempts' session ids and kept run dirs.
  `shuttleengine.OutcomeAsking` is a hard error on the first occurrence, with `Result.LastAssistantMessage` surfaced in the error text, and is never retried.
  No asking-triage LLM call is ported, and nothing is extracted out of `internal/treadleengine`.
- Rationale: this resolves the roadmap's own stated open risk. Verified during exploration: the two-attempt retry policy, asking-triage, stale-artifact move-aside, and round/attempt token naming all live in `treadleengine.Engine` (`run.go`'s `runRound`, `internal/treadleengine/run.go:443`), not in `burlerengine` — `burlerengine.Engine.Run` is a bare one-round call that returns a nil error for `asking`/`died`/`timeout` and leaves the branch to its caller (`internal/burlerengine/engine.go:163`).
  So the adapter must supply whatever slice it wants. A burler round is expensive (a full A/B pass, potentially with cluster fan-out), and `died`/`timeout` are nearly always environmental, so a single cheap deterministic retry has real value.
  Asking-triage does not: it is an ephemeral LLM utility call needing stencils, a judge model/effort pair, and shuttle access — a whole subsystem to classify one message — while `Shed`'s own `blocked`/`failed` state already surfaces a human, which is what an asking round needs anyway.
- Cancellation interacts with the retry explicitly: the producer re-checks `ctx.Err()` *before* starting attempt 2 and returns the `cancelErr` result instead of spawning it.
  A retry is a fresh, expensive LLM round, so starting one on an already-cancelled run would be exactly the waste the entry check exists to prevent — and, unlike a round already in flight, there is no paid-for artifact to protect here.
- Rejected: extracting `treadle`'s retry + triage into a new shared package — a large refactor of a shipped, working engine for one consumer, and `treadle` is itself expected to end up with zero consumers (Someday `Bouncer → Perch`), so the extraction would be paid for twice.
  Also rejected: no retry at all (`SingleLLMProducer`'s posture, `internal/shedadapters/singlellm.go:107`) — cheap there because a single LLM turn is cheap to redo from a `blocked` state, expensive here.

### Round resolution from disk, and this producer's artifact path convention

- Decision: the constructor is told an absolute `runDir`. Artifact paths are flat inside it, **one canonical pair per round number, with no attempt suffix**: `round-<N>-review.md` and `round-<N>-fixer-report.md`, where `N` is a plain positive decimal integer with no leading zeros.
  A retry (attempt 2) writes to the **same** two paths as attempt 1 — the retry is not a second artifact, it is a second try at producing the one artifact round `N` owes.
  Round resolution on every `Call`: scan `runDir` for the highest `N` for which **both** `round-<N>-review.md` **and** `round-<N>-fixer-report.md` exist; that round is complete, so this `Call` runs round `N+1`.
  The predicate is the **pair**, never the review alone. An absent or empty `runDir` starts at round 1. A directory entry not matching the exact `round-<N>-review.md` shape is ignored — never adopted, never deleted.
  `Call` creates `runDir` with `os.MkdirAll(runDir, 0o755)` before resolving the round, so a fresh clone's first `Call` works; the constructor validates that `runDir` is non-empty and absolute but does **not** require it to exist.
  Before **every** attempt (attempt 1 included), the producer runs the package's existing `archiveStaleOutputs` (`internal/shedadapters/archive.go`) over the round's two paths, renaming anything already there to a timestamped sibling.
  **The producer also archives both round paths before every return in which the round did not produce a usable review** — every such error exit and every such cancellation exit, without exception, not merely the ones where `Result.Outcome` is `shuttleengine.OutcomeDone`.
  Two returns leave the round's files in place: the success return (`Stuck` plus the review pointer), and a cancellation detected after the round already completed and parsed (see the Cancellation decision — that return is an error, but its artifacts are intact and must survive).
  The rule is therefore keyed on *whether the round produced a usable review*, not on whether the return is an error.
  `RunOpts.Round` still carries the attempt-distinguishing token (`3` then `3b`), because that value flows into `shuttleengine.Spec.Round` and so names the shuttle *run* — which genuinely differs per attempt — not the artifact.
- Rationale: this is the two-sided contract the `Bouncer` reproduces, and it must be stated as such because both sides depend on the same discriminator.
  **The presence of *both* `round-<N>-review.md` and `round-<N>-fixer-report.md` means, and only means, that round `N` completed and produced a usable review.** The `Bouncer` uses exactly that pair predicate to tell its seed call from its judge call, and this producer uses exactly that pair predicate to decide whether to advance — the two sides must run the same test, and it must be the pair.
  A review-only orphan is what makes the pair load-bearing rather than pedantic: the producer's own archive rules run on *returns*, so a producer **process killed mid-round** — in the same phase-A-written/phase-B-pending window named below — leaves `round-<N>-review.md` with no fixer report beside it, and no exit path ever ran to clean it up.
  Under a review-only predicate the next `Call` would advance to `N+1` and hydrate `round-<N>-fixer-report.md`, which `Profile.validate`'s `requireExistingPaths` rejects fail-loud (`internal/burlerengine/profile.go:83-88`) — and would do so on every subsequent `Call`, permanently wedging the segment while the `Bouncer` judged the half-finished round `N` as complete.
  Under the pair predicate the same orphan simply means round `N` is incomplete: the next `Call` re-runs `N`, and its pre-attempt archive sweeps the orphan aside as a stamped sibling.
  This also makes the producer's predicate agree with `shuttleengine`'s own definition of a finished run — every `OutputFiles` entry present, not just the first.
  The archive-on-every-non-success-exit rule above is what makes the second half of that sentence true, and the naive assumption it replaces — "a failed round writes no review file" — is simply false, on two independent paths:
  - **A partial round.** `shuttleengine` classifies a run as `OutcomeDone` only when *every* `OutputFiles` entry exists (`allOutputFilesExist`, `internal/shuttleengine/wait.go:272-273,299,305`). A burler round writes its review in phase A and its fixer report in phase B, so a round that completes its review and then dies or times out mid-fix is reported as `OutcomeDied`/`OutcomeTimeout` **with `round-<N>-review.md` already on disk**. For an A-review→B-fix round that is the *most likely* death window, not an exotic one.
  - **An error after a complete write.** `burlerengine.Run` returns a hard error on two paths reached only *after* the shuttle already reported `OutcomeDone`, and therefore after every output file exists: a cluster-audit policy violation (`internal/burlerengine/engine.go:176-179`) and a verdict-parse failure on the review file (`engine.go:188-191`).

  A pre-attempt archive alone does not close this, which is why the rule is stated on the exits rather than only on the entries: it covers attempt 2, but a second consecutive `died`/`timeout`, an `asking` hard error, and a cancellation between attempts all return while a phase-A-only review sits at the round's canonical path.
  The next `Call` would then advance to `N+1` and the `Bouncer` would judge a half-finished round — a review with no fixes applied — as a completed one.
  Archiving instead of deleting keeps the partial or malformed review as a stamped sibling, which is exactly what an operator needs to diagnose why the round died.
  A per-attempt suffix would break both at once: a round whose attempt 1 died and whose attempt 2 succeeded would leave its review at `round-3b-review.md`, so a scan for `round-<N>-review.md` would still resolve `2` and re-run round 3 forever, while the `Bouncer`, finding no `round-3-review.md`, would read a completed round as a seed call.
  That is precisely why `treadleengine`'s suffixed naming (`roundfiles.go:18,42`) must not be copied here despite the surface similarity: `treadle` can afford per-attempt artifacts because it records attempt identity in its own `state.json` and never infers round identity from file existence, whereas this segment has no such state and file existence *is* the protocol.
  Archiving before every attempt is mandatory rather than tidy: `shuttleengine`'s own spec validation rejects a pre-existing output file, and `burlerengine.Run` reads `p.ReviewPath` back after a done run (`internal/burlerengine/engine.go:183`), so a leftover file at the round's path — from a crash, a partial write, or an operator copy — would otherwise either fail the run or be parsed as this round's verdict.
  It also preserves the dead attempt's partial output as `round-<N>-review-<stamp>.md` for diagnosis rather than deleting it, and those stamped siblings are ignored by the round scan because they do not match the exact shape.
  Rediscovering identity from disk rather than holding a counter in adapter memory means a process restart resolves the same round, which is exactly why `PerchProducer.resolveRunID` (`internal/shedadapters/perch.go`) works the way it does; `highestRunAttempt` in the same file is the parsing discipline to mirror (exact-shape match, no leading zeros, unrelated entries skipped).
  `treadleengine`'s naming helpers are unexported, so this package re-declares its own — the same precedent `archive.go` set by re-implementing `firstFreeArchivePath` rather than exporting `websterengine`'s copy.
- Rejected: keeping `treadle`'s per-attempt artifact suffix, for the round-identity and seed-call reasons above.
  Also rejected: reading `Shed`'s status-file history to derive the round — no adapter reads `Shed`'s status file (`internal/shedadapters/perch.go:188` states this for `PerchProducer`), and history is Shed-owned.
  Also rejected: a caller-supplied path-factory func — pushes the convention into every caller and leaves the `Bouncer` with nothing stable to read.
  Also rejected: deleting a stale artifact instead of archiving it — the package already owns an archive helper, and a dead attempt's partial review is the most useful thing an operator has when diagnosing why it died.

### Prior-round hydration

- Decision: on every round after the first, the producer fills `burlerengine.Profile.PriorReviews` and `PriorFixerReports` from `runDir` using the **same exact-shape, same pair predicate the round scan uses** — `round-<N>-review.md` / `round-<N>-fixer-report.md` with `N` a positive decimal integer, no leading zeros, no suffix, and *both* present — for every complete `N` below the current round, in ascending round order, plus any absolute paths the focus file's `hydrate` list names (appended to `PriorReviews`).
  Told `Profile.PriorReviews`/`PriorFixerReports` values from the caller are treated as a fixed prefix and preserved.
  The exact-shape match is load-bearing rather than tidy: `archiveStaleOutputs` writes a dead or malformed attempt as `round-<N>-review-<stamp>.md` (`internal/shedadapters/archive.go`), so a loose prefix scan would hydrate archived partial and unparseable reviews back into the next round alongside the real ones.
  One scan predicate serves both round resolution and hydration, so the two can never disagree about what counts as a completed round — and, because that predicate requires the pair, hydration can never name a fixer report that does not exist, which is precisely the fail-loud `requireExistingPaths` wedge described above.
- Decision: hydration is **full history, deliberately unwindowed** — every prior round in the segment, not a sliding window.
  Rationale: `burlerengine` has no cross-round finding-identity tracking of its own (its package doc is explicit that judging progress across rounds is the caller's job), so the only thing giving round `N` any memory of round `N-3` is the artifact list handed to it.
  The growth is bounded by the segment's own round cap — the smaller of the two rows' `MaxBounces`, default ten (see the "Always `Stuck`, never `Done`" decision) — so with the default the tenth and final round hydrates nine prior rounds, eighteen artifacts, which is a prompt-size cost rather than an unbounded one.
  A segment that raises `MaxBounces` far above the default and finds this too heavy has a seam ready without a code change: the `Bouncer` maintains the distilled finding-identity ledger and names it through the focus file's `hydrate` list, so windowing is a `Bouncer`-side policy decision rather than something this producer should guess at now.
- Rationale: `burlerengine.Profile.validate` requires every hydration path to exist on disk (`requireExistingPaths`), so the producer must supply real paths, and a round with no memory of prior rounds would re-find the same findings forever.
  Routing the `Bouncer`'s own prose ledger through the focus file's `hydrate` list rather than a hardcoded path is what keeps this adapter ignorant of the `Bouncer`'s path convention while still letting the ledger reach the round.
- Rejected: mirroring `treadleengine.collectPriorHydration`'s gate-file rule — there is no command gate at this seam.

### The next-round focus file: structured, fail-safe, defined here

- Decision: the focus file is JSON, at `round-<token>-focus.json` beside the round's other artifacts in `runDir`, resolved at the round's attempt-1 token and reused unchanged by a retry.
  This task defines the exported struct and its parser in `internal/shedadapters`; the `Bouncer` task marshals the same exported struct, so there is exactly one format and no unused writer here.
  Two optional fields: `exclude_lenses []string` (lens names the round must drop) and `hydrate []string` (absolute paths to feed into `PriorReviews`).
  **The token names the round the directives are *for*, not the round that produced them.** `round-<N>-focus.json` carries directives for round `N`, so a `Bouncer` that has just judged round `N` and is rejecting it writes `round-<N+1>-focus.json`, and the seed call — which runs before any round exists — writes `round-1-focus.json`.
  This has to be stated where the read contract is stated, because getting it wrong fails *silently*: a file named after the round it judged is simply never looked for, and the file's own fail-safe rule degrades that to "no directive", losing the entire trimming feature with nothing but a `logger.Warn` to show for it.
  Reading is fail-safe end to end: an absent file, an unreadable file, malformed JSON, an unknown field, or a `hydrate` entry that is relative or does not exist all degrade to "no directive from that field" with a name-prefixed `logger.Warn`, never an error.
  Fail-safety extends past parsing to *application*, which is the half that is easy to lose: a well-formed directive that cannot be honoured — naming a lens the fan no longer has, naming every lens in the fan, or arriving when the template `Profile` names no fan at all — is degraded and warned about by the producer, never allowed to become a `validate` hard error downstream (see the `ClusterExclude` decision's authorship split below).
- Rationale: the roadmap requires the directive to be mechanically parseable so the round producer's *Go code*, not an LLM's discretion, decides whether to drop a lens — that is the whole reason it is structured rather than prose (unlike `treadle`'s `PreRoundTargeting`, which is unconstrained prose a runner may ignore).
  JSON with a Go struct makes the two sides mechanically identical.
  Fail-safety is the same argument `treadleengine`'s targeting doc makes: a missed directive costs the round the trimming it would have had, never correctness — and the file's author is an LLM, so an unattended run must not be taken down by one malformed write.
- Rejected: defining the struct in `internal/burlerengine` — the file is a `Shed`-segment concern authored by a domain-agnostic `Bouncer`, and `burlerengine` knows nothing about `Shed`.
  Also rejected: deferring the format entirely to the `Bouncer` task — this task is the consumer, so it would ship a read path against an undefined format.
  Also rejected: fail-loud parsing, for the unattended-run reason above.

### Cluster fan-out trimming: `Profile.ClusterExclude`

- Decision: add an exported `ClusterExclude []string` field to `burlerengine.Profile`.
  `validate` resolves `ClusterFan` via `ResolveFan` exactly as today, then drops every resolved lens whose name appears in `ClusterExclude`, and stores the survivors in the existing unexported `clusterLenses`.
  The three edge cases split on **who authored the input**, and that split is the decision:
  - `ClusterExclude` set with an empty `ClusterFan` is a **`validate` error**. This is a caller mistake — a Go caller asking to trim a fan it never named — and it is unreachable from a focus file, because the producer sets `ClusterExclude` only when its own template `Profile` carries a non-empty `ClusterFan`, which it checks itself and otherwise drops the directive with a `logger.Warn`.
  - An exclusion naming a lens **not in the resolved fan** is a **no-op for that name**, with a `logger.Warn`, not an error.
  - An exclusion that would empty the fan **drops the whole exclusion**, keeping the fan intact, with a `logger.Warn` — never a solo round, and never an error.

  The producer sets this field from the focus file's `exclude_lenses` per call.
- Rationale for the split: an operator-authored `burler.yaml` fan or lens name is config, and config that does not resolve is a defect the operator must see, which is why `ResolveFan` is fail-loud on it (`config.go:91`) and stays that way.
  An `exclude_lenses` entry is something else — an LLM-authored, per-round *advisory* directive over a config-owned fan the operator may edit between rounds, so a name that no longer resolves is **stale, not wrong**, and taking an unattended segment down over it would be the worst available outcome.
  The fan is authoritative; the directive is advisory. That is also why a full exclusion clamps rather than erroring: dropping to zero lenses is never what "these found nothing last round" meant, and the fallback (run the full fan again) costs tokens, never correctness.
  This is what keeps the focus file's stated fail-safety honest end to end — a well-formed but unusable LLM directive is degraded and warned about, never turned into a hard error downstream.
- Rationale: `clusterLenses` is the single value both prompt composition (`internal/burlerengine/prompt.go:213`, reached via `composePrompt` at `engine.go:126`) and `auditClusterRound`'s exact-N fork check (`engine.go:176`) read, so filtering there propagates to prompt composition and the fork audit with no second code path and no risk of the audit demanding forks for lenses the prompt never named.
  An exclusion list rather than an allow-list is the natural shape of the `Bouncer`'s own output ("these lenses found nothing last round") and means a lens an operator later adds to the fan participates by default instead of being invisible until every call site is updated.
  Note this does not weaken `burlerengine`'s existing `ErrClusterForksMissing` posture, that a fork *shortfall* is an infrastructure defect rather than a degrade-to-solo (its package doc): that rule is about the fan the round actually asked for versus the forks that ran, and `clusterLenses` is still the exact-N number after filtering, so a trimmed round demands exactly the forks it named.
  Clamping happens before the round asks for anything; the audit's contract is untouched.
- Rejected: an allow-list `ClusterLenses []Lens` on `Profile` — exposes the `Lens` value type as caller input and makes an operator's config edit invisible to a running segment.
  Also rejected: an exported `ResolveFanFiltered(cfg, fan, exclude)` helper — leaves the filtering outside `validate`, so a caller could hand `Run` a fan and an exclusion list that never met.

### Seam shape and constructor validation

- Decision: a narrow `BurlerRunner` interface in `shedadapters` — `Run(burlerengine.Profile, burlerengine.RunOpts) (burlerengine.Result, error)` — with `var _ BurlerRunner = (*burlerengine.Engine)(nil)` as a compile-time proof, mirroring `PerchRunner` (`internal/shedadapters/perch.go`).
  A plain seam over an already-constructed engine, not a factory: `burlerengine` has no pause-callback constructor option, so the factory shape `PerchProducer` needs does not apply.
  `NewBurlerProducer` takes `name`, the runner, a template `burlerengine.Profile` (content fields only — `ReviewPath`/`FixerReportPath`/`PriorReviews`/`PriorFixerReports`/`ClusterExclude` are overwritten per round), a `burlerengine.RunOpts` (whose `Round` field is overwritten per round), and the absolute `runDir`.
  It returns an error for a nil runner, an empty `runDir`, a non-absolute `runDir`, or an empty `name`.
- Rationale: every existing adapter validates its told paths in its constructor before touching a directory, and every path here is told, never derived.
- Rejected: taking a `Profile` factory func — the content contract does not vary per round; only paths and the exclusion list do, and those are the producer's own business.

### Cancellation

- Decision: **cancellation always returns a non-nil error, never `Stuck`** — including when the round completed and parsed.
  `entryErr` at `Call` entry, `cancelErr` on every exit, with no success exception.
  What the completed-and-cancelled case gets instead is a **carve-out from the archive rule**: its two artifacts are left in place rather than archived, so the round stays complete on disk.
- Rationale: `shedengine`'s seam states as a binding implementation obligation that a producer must "surface context cancellation as a non-nil error, never as `Stuck`" (`internal/shedengine/producer.go:28-29`).
  Returning a success `Stuck` under cancellation would deviate from that obligation, and the deviation is not cosmetic: `Shed`'s loop would route the `Stuck` to `OnStuck`, append a history entry, and consume one bounce of the segment's round cap before pausing at the `Bouncer` on the next iteration (`internal/shedengine/run.go:180-215`) — a cancelled run would silently cost a review round.
  The package's own shared exception ("a genuine success verdict survives cancellation") exists for one reason: so a finished artifact and a paid-for LLM session are never discarded.
  That purpose is fully served here **without** deviating, because this producer's success is durable on disk and self-recovering: the review and fixer report stay where they are, and from-disk round resolution means the next `Call` after the operator resumes sees round `N` complete and advances to `N+1` on its own.
  Only the in-memory verdict is dropped, and it was re-derivable from the filesystem anyway.
  So the correct statement in the package doc is not "this producer reads the exception differently" but "this producer does not need the exception at all" — the artifact is protected by the archive carve-out rather than by a deviating verdict.
- Rejected: returning the success `Stuck` under cancellation and recording the deviation's consequences — the seam's wording is an obligation on implementations, not a default, and amending `shedengine` is out of scope for this task (and would be the wrong fix regardless, since nothing here needs the deviation).
- Rejected: archiving the completed round's artifacts on the cancellation path — that is exactly the "discard a paid-for session" outcome the shared exception exists to prevent, and it would also make the next `Call` re-run a round that already succeeded.
- Rejected: no mid-run cancellation bridge is installed, matching `SingleLLMProducer` and `WebsterProducer` — `burlerengine` exposes no pause seam, so a cancel is observed only once the round reaches a terminal outcome or its own `RunOpts.Timeout` elapses. This limitation goes in the package doc's Limitations section.

### Output pointer

- Decision: report `OutputPointer{Path: <this round's review path>}`.
- Rationale: unlike `PerchProducer`, whose empty pointer is a deliberate statement that a gate's verdict is always re-derived, this producer genuinely produces a human-readable artifact per call, and it is the exact artifact the `Bouncer` will read next.
- Rejected: pointing at the fixer report — the review is the round's primary artifact and the one the judge consumes.

## Technical context

- **`internal/shedengine/producer.go`** — the seam: `Call(ctx) (Outcome, OutputPointer, error)`, `Outcome` is exactly `Done` or `Stuck`, context cancellation must surface as a non-nil error and never as `Stuck`.
  `ProducerDef` carries `Name`, `OnStuck`, `OnDone`, `Segment`, `MaxBounces`; `validate` requires a non-empty `OnStuck` to name a target sharing the producer's `Segment`.
  `Shed` never introspects the pointer path and never stats it for a control-flow decision.
- **`internal/shedadapters`** — existing files to follow: `perch.go` (from-disk identity resolution, narrow seam + compile-time assertion, constructor path validation, per-outcome mapping with `cancelErr` on every non-success path), `singlellm.go` (outcome switch shape, absolute-path check, `archiveStaleOutputs` use), `ctx.go` (`entryErr`/`cancelErr` — reuse verbatim, do not add a third), `archive.go` (`archiveStaleOutputs`, `firstFreeArchivePath`).
  `doc.go` is a long structured package doc with `# Outcome mapping`, `# Told, never derived`, `# Shared cancellation rule`, and `# Limitations` sections that each need a `BurlerProducer` entry.
  Its opening sentence and `docs/overview.md:235,316,318` all say "three adapters" and must become four.
- **`internal/burlerengine`** — `Engine.Run(p Profile, opts RunOpts) (Result, error)` (`engine.go:96`) is the one entry point.
  `Result` carries `Outcome`, `Verdict`, `Findings`, `ReviewPath`, `FixerReportPath`, `SessionID`, `StrandGUID`, `LastAssistantMessage`, `RunDir`, `ForkAudit`, `ClusterWarnings`.
  `Run` returns a nil error for `asking`/`died`/`timeout` and reserves errors for an invalid profile, a shuttle failure, a cluster-audit violation, and a verdict parse failure on a done run.
  The last two are the errors-after-a-written-artifact case (`engine.go:176-179,188-191`): both are reached only once the shuttle reported `OutcomeDone`, which by `shuttleengine`'s own contract means every `OutputFiles` entry already exists on disk. `Result.Outcome` is populated on those error returns, which is how the producer detects the case.
  `Profile.validate` (`profile.go:59`) resolves relative paths against the engine's `Geometry.WorktreeRoot`, requires every hydration path to exist, requires a non-empty `Rubric`, requires `FixScope` to be exactly `overlay` or `source`, resolves `ClusterFan` via `ResolveFan`, and rejects `ReviewPath == FixerReportPath`.
  `ResolveFan` (`config.go:91`) is fail-loud, and *rejects* a fan carrying more than `maxClusterN` (16) entries rather than truncating it (`config.go:102-104`).
  `Lens` is `{Name, Text}`.
- **`internal/treadleengine`** — read for reference only; this task adds no import of it and changes nothing in it.
  `run.go:443` `runRound` is the retry/triage source; `roundfiles.go` is the artifact-naming source; both are unexported.
  Note `treadle`'s `Engine.runRound` treats a second consecutive non-done attempt as an infrastructure error deliberately *not* modeled as a stuck verdict — the same distinction this producer keeps.
- **`internal/perchengine/adapter.go`** — the closest existing analogue of the mapping this producer performs (`burlerengine.Profile` assembly + `Result` mapping), worth reading before writing `burler.go`, but it maps onto `treadle`'s vocabulary rather than `Shed`'s.
- **Gotcha:** `burlerengine.Profile.validate` mutates the profile in place (resolving paths and filling `clusterLenses`), and `Engine.Run` takes `Profile` by value — so the producer's stored template profile is not mutated by a call, but the producer must still build a fresh copy per round rather than reusing a resolved one.
- **Gotcha:** `shuttleengine`'s spec validation rejects a pre-existing output file, which is why `treadleengine` moves stale artifacts aside before an attempt and why this producer archives before every attempt rather than only on a retry.
- **Gotcha:** `auditClusterRound` demands exactly `len(clusterLenses)` fork transcripts. Any trimming that does not go through `clusterLenses` would make the audit demand forks for lenses the prompt never spawned.
- **Gotcha:** `burlerengine.Run` writes per-round instruction files under `<AnchorPath>/.lyx/burler/round-*` and never prunes them; that is existing accepted machine-local litter, not something this task changes.

## Constraints

From `CONSTRAINTS.md`:

- **Shed Producer-Seam Invariant** — `internal/shedengine` imports only stdlib, `internal/state`, `internal/lock`. This task must not add anything to `shedengine`; producers adapt onto the seam from their own package. Enforced by `internal/shedengine/seam_enforcement_test.go`.
- **Told-Geometry Invariant** — `internal/burlerengine` is a told-geometry producer (review obligation, not machine-enforced): it takes its absolute paths from its caller and must not gain a direct `internal/lyxcwd` import. `shedadapters` constructors take already-resolved absolute paths and already-constructed engines, call no `lyxcwd`/`os.Getwd`/git, and write neither `_lyx` nor `.lyx` literals — that holds for this producer too.
- **Lyxdirs Single-Declarer Invariant** — no `_lyx`/`.lyx` literal in path-construction context anywhere in the new code.
- **Review Round Invariant** — A-before-B, every finding fixed in B at all severities, no self-grading, commit-per-fix on source scope. This producer must not weaken it: in particular the no-self-grading half is why the producer never returns `Done` on its own `VerdictApproved`.
- **Treadle Runner-Seam Invariant** — restricts `treadleengine`'s own imports only. Nothing here imports `treadleengine`, so the invariant is untouched either way.
- **Producer Pointer-Rule Invariant** — binds instruction files, not Go source. No prompt content changes here, so it does not engage.
- **CLI / Cobra Invariant** — no new command, so it does not engage.
- **Documentation Lifecycle** — a task adding a module or cross-cutting infrastructure updates its docs in the same commit (see `CLAUDE.md`); `manifest/roadmap.md` moves because a Planned item completes.
- **Config Strictness Invariant** — `ClusterExclude` is a Go struct field on `Profile`, not a `burler.yaml` key, so `burler.yaml`'s seed-only posture and `LoadConfig`'s strict decode are unaffected.
- No new cross-cutting invariant is expected from this task, so no `CONSTRAINTS.md` edit is anticipated. If the implementation discovers one, it is recorded there in the same commit.

## Testing

TDD candidates, in the order they should be written:

- **`internal/burlerengine` — `ClusterExclude` in `validate`** (pure, no LLM, table-driven; the cheapest and most self-contained piece, so write it first).
  Scenarios: exclusion drops exactly the named lenses and preserves fan order for the rest; an exclusion naming a lens not in the fan is a no-op for that name rather than an error; `ClusterExclude` set with an empty `ClusterFan` is an error; an exclusion covering every lens in the fan leaves the fan intact rather than erroring or emptying `clusterLenses`; a duplicate name in the exclusion list is harmless; no exclusion behaves exactly as today.
  Existing `profile_test.go` and `cluster_test.go` show the table shape and fixture-config helpers to reuse.
- **`internal/shedadapters` — focus-file parsing** (pure, no LLM).
  Scenarios: a well-formed file yields both fields; each field independently absent; absent file; unreadable file; malformed JSON; unknown JSON field; a relative `hydrate` entry; a `hydrate` entry that does not exist on disk. Every degraded case must return "no directive" and no error.
- **`internal/shedadapters` — round resolution and artifact paths** (pure filesystem, no LLM).
  Scenarios: empty/absent run dir starts at round 1; a complete round `N` (both files) advances to `N+1`; a round whose review file is absent is re-run at the same `N` rather than skipped; a **review-only orphan** at `N` (the killed-process window) counts as incomplete, so `N` is re-run and the orphan is archived, and hydration never names the missing fixer report; a fixer-report-only orphan behaves the same way; a run dir holding unrelated files is ignored; archived stale siblings (`round-2-review-<stamp>.md`) do not satisfy the round scan and never shift the resolved round; non-numeric, zero-padded, or attempt-suffixed tokens (`round-3b-review.md`) are ignored rather than adopted (the `highestRunAttempt` precedent in `perch.go` shows the parsing discipline expected); a pre-existing file at the round's own paths is archived aside, not deleted and not passed through, before the runner is invoked; an absent `runDir` is created rather than erroring; hydration uses the same exact-shape match and therefore never picks up a stamped archive sibling.
- **`internal/shedadapters` — `BurlerProducer.Call`**, driven by a fake `BurlerRunner` returning scripted `burlerengine.Result` values, in the same style `perch_test.go`/`singlellm_test.go` use for their fakes. No LLM in this suite.
  Scenarios: a done round returns `Stuck` with the review path as pointer and never `Done`, for both `VerdictApproved` and `VerdictBlocking`; the profile handed to the runner carries the derived per-round review/fixer-report paths, the derived hydration lists, the round token in `RunOpts.Round`, and the focus file's `exclude_lenses` in `ClusterExclude`; `died` then done retries once and succeeds, writing its review at the same `round-<N>-review.md` the dead attempt owned; `timeout` twice is a hard error naming both attempts; a context cancelled between attempt 1 and attempt 2 returns the cancellation error and the runner is invoked exactly once; `asking` is a hard error on the first occurrence with the assistant message in the text; a focus file naming every lens in the fan, or naming a lens the fan lacks, reaches the runner as a profile that still runs (clamped/no-op) rather than as an error; a runner error is wrapped, not swallowed; every non-success exit archives both round paths before returning, so a following `Call` re-runs the same round `N` rather than advancing — asserted by driving two `Call`s against a fake runner and checking the second one's round token, across all four exits that can leave a file behind: two consecutive `died`/`timeout` where attempt 1 wrote only the review (the phase-A-only partial), `asking`, a cancellation between attempts, and a runner error returned with `Result.Outcome == OutcomeDone` (the cluster-audit and verdict-parse cases); a cancelled context at entry returns an error without invoking the runner; a context cancelled during a *failed* round yields the cancellation error and archives; a context cancelled during a *completed* round also yields a non-nil error and **never** `Stuck` (the seam obligation), but leaves both artifacts in place, so a second `Call` on a fresh context advances to `N+1` instead of re-running `N`; constructor rejection of nil runner, empty name, empty `runDir`, and relative `runDir`.
- **Docs/consistency tests** — `docs/overview.md` and `manifest/designs/shed.md` are covered by the repo's Markdown Link Integrity checking; adding a fourth adapter must not break an existing anchor, and the roadmap edit removes a Planned item rather than an anchor target.

No new opt-in real-engine smoke test is added: `internal/burlerengine`'s existing `smoke_round_test.go`/`smoke_cluster_test.go` already cover the real round and the real cluster fan, and this producer adds no new LLM interaction of its own — only path assembly, retry, and outcome mapping, all of which the fake-runner suite covers deterministically.

## Q&A log

- **Q:** Does `treadleengine`'s two-attempt retry-on-death/timeout policy and asking-triage already live in (or move cleanly to) `burlerengine`'s one-round API, and if not, what does this adapter do about it? **A:** [auto-pick] Adapter owns a minimal self-contained slice: one deterministic retry on `died`/`timeout`, `asking` a hard error, no triage port, no extraction from `treadle`. **Why:** exploration confirmed the machinery is in `treadleengine.Engine.runRound` (`run.go:443`), not `burlerengine`; triage is a whole LLM subsystem for one classification while `Shed`'s `blocked` state already surfaces a human, and a non-done round writes no review file, which the `Bouncer` would misread as a seed call.
- **Q:** What API shape should cluster fan-out trimming take in `burlerengine`? **A:** [auto-pick] A new exported `Profile.ClusterExclude []string`, applied inside `validate` after `ResolveFan`. **Why:** `clusterLenses` is the single value both prompt composition and the exact-N fork audit read, so filtering there needs no second code path; an exclusion list matches the `Bouncer`'s own "these found nothing" output and lets a newly-seeded lens participate by default.
- **Q:** Where is the structured next-round focus file's contract defined, given the `Bouncer` that writes it is the next task? **A:** [auto-pick] Define the exported struct and a fail-safe parser here in `internal/shedadapters`; JSON at `round-<token>-focus.json`, fields `exclude_lenses` and `hydrate`, both optional. **Why:** one format and one parser with no unused writer; `hydrate` keeps this adapter ignorant of the `Bouncer`'s own path convention; fail-safe because the author is an LLM and a missed directive costs tokens, never correctness.
- **Q:** How does the producer know which round it is on? **A:** [auto-pick] Told an absolute `runDir` and resolve the round from disk each `Call`, mirroring `PerchProducer.resolveRunID`. **Why:** a process restart resolves the same round, and no adapter may read `Shed`'s status file.
- **Q:** What seam does the producer hold? **A:** [auto-pick] A narrow `BurlerRunner` interface plus a compile-time assertion over `*burlerengine.Engine`, mirroring `PerchRunner`. **Why:** `burlerengine` has no pause-callback constructor option, so `PerchProducer`'s factory shape buys nothing here.
- **Q:** What does the producer report as its `OutputPointer`? **A:** [auto-pick] This round's review path. **Why:** unlike a gate producer, this producer genuinely writes an artifact per call, and it is exactly what the `Bouncer` reads next.
- **Q:** How does the shared cancellation rule's success exception apply to a producer that never returns `Done`? **A:** [auto-pick] Initially: a completed round survives cancellation, returned as `Stuck` with its pointer. **Superseded in round 4** — see the seam-obligation entry below; the answer is now that the exception is not needed at all, because the artifact is protected by an archive carve-out rather than by a deviating verdict.
- **Q:** Round 1's BLOCKING finding: a retry-suffixed artifact token (`round-3b-review.md`) breaks both the round scan and the `Bouncer`'s seed-vs-judge discriminator. **A:** [auto-pick] Drop the attempt suffix from artifact names entirely — one canonical `round-<N>-review.md`/`-fixer-report.md` pair per round, a retry writing to the same paths after archiving whatever is there; `RunOpts.Round` keeps the `3b` token because it names the shuttle *run*, not the artifact. **Why:** file existence *is* the two-sided protocol between this producer and the `Bouncer`, so exactly one path per round may carry the meaning "round N completed"; `treadle` can afford per-attempt artifacts only because it records attempt identity in `state.json` and never infers round identity from the filesystem.
- **Q:** Reading the focus file is claimed fail-safe, yet a well-formed directive can still hard-error in `validate`. Which gives? **A:** [auto-pick] Fail-safety extends to application, split by authorship: operator-authored config stays fail-loud (`ResolveFan` unchanged), LLM-authored directives clamp — unknown lens is a no-op, a full exclusion drops the whole exclusion, and a directive arriving against a fan-less template profile is dropped by the producer before it reaches `validate`. **Why:** the fan is authoritative and the directive advisory; the fallback costs tokens, never correctness, and taking an unattended segment down over a stale lens name is the worst available outcome.
- **Q:** Round 2's first BLOCKING finding: `burlerengine.Run` can return a hard error *after* the shuttle reached `OutcomeDone`, so `round-<N>-review.md` exists for a round that failed. **A:** [auto-pick] The producer archives both round paths before returning any error whose `Result.Outcome` is `OutcomeDone`, and the discriminator is stated as "completed *and produced a usable review*". **Why:** the cluster-audit and verdict-parse error paths are exactly the case where the agent has already written every output file, so without this the `Bouncer` would judge a broken review as a completed round; archiving rather than deleting keeps the malformed review for diagnosis.
- **Q:** Round 2's second BLOCKING finding: who creates `runDir`? **A:** [auto-pick] `Call` creates it with `os.MkdirAll(runDir, 0o755)` before resolving the round; the constructor requires only non-empty and absolute. **Why:** `archiveStaleOutputs` silently skips absent paths, so without the `MkdirAll` the failure would surface as the agent being unable to write rather than as a clear error. `PerchProducer.resolveRunID` carries a structurally similar Call-time `MkdirAll`, though for its *scratch* dir specifically — its run dirs are tracked and its scratch tree never is (`internal/shedadapters/perch.go:120-126`), so it is the pattern that is analogous here, not the directory.
- **Q:** Round 3's first BLOCKING finding: a `died`/`timeout` round can still leave `round-<N>-review.md`, because a burler round writes its review in phase A and `shuttleengine` reports `OutcomeDone` only when *every* output file exists. **A:** [auto-pick] Restate the archive rule on the *exits*: archive both round paths before every return that is not a completed, parsed round — not only on errors whose `Result.Outcome` is `OutcomeDone`. **Why:** phase-A-written/phase-B-died is the most likely death window for an A→B round, and a pre-attempt archive alone leaves that partial review in place on a double-`died`, an `asking`, or a mid-retry cancellation.
- **Q:** Round 3's second BLOCKING finding: is this row's `MaxBounces` really the segment's round cap? **A:** [auto-pick] No — restated. Both rows carry an unresetting episode, and the `Bouncer` is the segment's entry point, so its `Stuck` sequence runs one ahead; with equal budgets it exhausts first. The cap is the smaller of the two budgets, the `Bouncer`'s normally binds, and raising it means raising both rows. **Why:** the original wording would have shipped advice into the producer's doc comment ("raise this row's `MaxBounces`") that does not actually raise the round cap.
- **Q:** Round 4's BLOCKING finding: a producer process killed mid-round leaves a review with no fixer report, and no exit path ever runs to clean it up — the next `Call` then hydrates a fixer report that does not exist and `requireExistingPaths` wedges the segment permanently. **A:** [auto-pick] Make the completion predicate the **pair** — both `round-<N>-review.md` and `round-<N>-fixer-report.md` — for round resolution, for hydration, and for the `Bouncer`'s seed-vs-judge test. **Why:** an orphan then reads as "round `N` incomplete", so `N` is simply re-run and the pre-attempt archive sweeps the orphan aside; it also aligns the predicate with `shuttleengine`'s own definition of a finished run.
- **Q:** Round 4's demoted finding: `shedengine/producer.go:28-29` binds implementations to "surface context cancellation as a non-nil error, never as `Stuck`", which the success-survives-cancellation decision deviated from. **A:** [auto-pick] Drop the deviation — cancellation always errors, never `Stuck` — and protect the artifact with an archive carve-out instead: a completed-then-cancelled round keeps its files, so from-disk round resolution advances past it on the next `Call`. **Why:** the shared exception exists only to avoid discarding a paid-for artifact, and here the artifact is durable and self-recovering, so the purpose is served without deviating; keeping the deviation would also have silently consumed one bounce of the segment's round cap on every cancelled run.
- **Q:** An out-of-band orchestrator review flagged the `singlellm.go:107` citation as pointing at the `OutcomeAsking` case rather than the `OutcomeDied`/`OutcomeTimeout` case the "no retry at all" claim is about. Correct? **A:** No — verified in the file: line 100 is `case shuttleengine.OutcomeAsking:`, line 107 is `case shuttleengine.OutcomeDied, shuttleengine.OutcomeTimeout:`, and line 113 is `default:`. The citation stands as written; the suggested "fix" to ~113 would point at `default:`. Recorded here so the same nit is not re-raised.
- **Q:** What test tier does this need, and does it need a real-engine smoke test? **A:** [auto-pick] A fake-`BurlerRunner` unit suite in `shedadapters` plus `ClusterExclude` unit tests in `burlerengine`; no new smoke test. **Why:** the producer adds only path assembly, retry, and outcome mapping over an engine whose real-round and real-cluster behavior is already smoke-covered.


### From _mill/plan/00-overview.md


```yaml
task: 'shedadapters: Burler-round producer'
slug: 'shedadapters-burler-producer'
approved: true
started: '20260820-152710'
parent: 'main'
root: ""
verify: go build ./...
```

### From _mill/plan/01-cluster-exclude.md


```yaml
task: 'shedadapters: Burler-round producer'
batch: 'burlerengine ClusterExclude'
number: 1
cards: 3
verify: go test ./internal/burlerengine/
depends-on: []
```



- **Edits:**
  - `internal/burlerengine/profile.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/burlerengine/profile_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/burlerengine/doc.go`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/02-focus-file.md


```yaml
task: 'shedadapters: Burler-round producer'
batch: 'focus-file contract'
number: 2
cards: 2
verify: go test ./internal/shedadapters/
depends-on: []
```



- **Edits:** none
- **Creates:**
  - `internal/shedadapters/focus.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/shedadapters/focus_test.go`
- **Deletes:** none

### From _mill/plan/03-burler-producer.md


```yaml
task: 'shedadapters: Burler-round producer'
batch: 'BurlerProducer'
number: 3
cards: 5
verify: go test ./internal/shedadapters/
depends-on: [1, 2]
```



- **Edits:** none
- **Creates:**
  - `internal/shedadapters/burler.go`
- **Deletes:** none
- **Edits:**
  - `internal/shedadapters/burler.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/shedadapters/burler.go`
- **Creates:** none
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/shedadapters/burler_test.go`
- **Deletes:** none
- **Edits:**
  - `internal/shedadapters/burler_test.go`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/04-docs.md


```yaml
task: 'shedadapters: Burler-round producer'
batch: 'docs'
number: 4
cards: 4
verify: go test ./internal/lyxcwd/ ./internal/shedadapters/
depends-on: [3]
```



- **Edits:**
  - `internal/shedadapters/doc.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `manifest/designs/shed.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none

## Conflicting files

- `docs/overview.md`
- `internal/shedadapters/doc.go`
- `manifest/roadmap.md`

## Instructions

For each file listed above:

1. Read the file and locate every conflict block (`<<<<<<<`, `=======`, `>>>>>>>`).
2. Understand both sides of the conflict — what each branch intended.
3. Write a resolution that preserves the intent of both sides.
   When both sides modify **different, non-overlapping parts** of the same conflict region — for example, different columns of one table row, different keys of one object, or disjoint lines of a prose block — **combine both edits** into a single resolved structure.
   Do NOT pick one side wholesale just because the region overlaps syntactically;
   picking one side wholesale is correct only when the two changes are genuinely mutually exclusive (e.g. the same key is renamed to two different values).
   Worked example: if `ours` changes column A and `theirs` changes column B of the same table row, the resolution keeps both column changes in a single row — it does not discard either.
4. Before keeping content from either side inside a conflict hunk, search the rest of the file (outside the hunk) for that same content.
   This judgment call is scoped narrowly — it applies only when a hunk's content might be a moved duplicate of content living elsewhere in the file;
   it does NOT apply to every ordinary step-3 disjoint-region combine (e.g. the column-A/column-B worked example above), which remains today's silent, high-confidence success path.
   Two branches:
   - **Confident case:** if the content clearly already exists elsewhere and the surrounding context makes it unambiguous that this is the same item having been moved (not two independent, separately-intended copies) — do not re-add it in the hunk;
     keep only the other side's unrelated edit.
     Worked example: one side moves a roadmap item from `## Planned` to `## Done`, while the other side makes an unrelated edit elsewhere in the file.
     The resolution keeps the item only under `## Done`;
     it is not re-added under `## Planned`.
   - **Ambiguous case:** if you cannot confidently tell whether this is the same moved content or a legitimate independent duplication — fall back to step 3's default (keep both) rather than guessing, and report the ambiguity via the `discarded` field (see Report section) with the description `"kept both sides of a conflict, ambiguous move-vs-duplicate"`.
     Worked example: a similarly-worded item appears in two different sections and you cannot tell whether it is the same item moved or a legitimate second, independently-added item.
     The resolution keeps both occurrences and reports the ambiguity via `discarded`.
5. Run `git -C /home/knatte/Code/loomyard/wts/shedadapters-burler-producer add <file>` to stage the resolved file.
6. For modify/delete (DU) conflicts: if Task intent above lists this file under a batch's `Deletes:`, run `git -C /home/knatte/Code/loomyard/wts/shedadapters-burler-producer rm <file>` instead of editing;
   that stages the intentional deletion.
7. For UD conflicts — files this branch **modified** that the parent branch **deleted**: do not silently keep the modification.
   Instead: a. Run `git log --diff-filter=D --oneline MERGE_HEAD -- <file>` to find the deletion commit on the parent. b. Run `git show <deletion-commit>` to inspect context. c. If the deletion commit message mentions a replacement file (e.g. "replaced by", "moved to", "consolidated into"),
   or the commit also adds a file in the same directory with overlapping content: stage the deletion — `git -C /home/knatte/Code/loomyard/wts/shedadapters-burler-producer rm <file>`. d. If detection is inconclusive: report `{"status":"stuck","stuck_type":"logic","reason":"modify/delete conflict on <file>: cannot determine if parent deletion is a replacement -- operator must decide"}` and halt.
   Do NOT silently keep the modification.
8. Before reporting `{"status":"success"}` (with or without `discarded`), re-read each file listed in Conflicting files in full and explicitly verify no contradictory losing-side claims survive the resolution — e.g. a stale value from one side of the conflict left alongside the correct value from the other side, or a claim that only made sense before the other side's edit was applied.
   If you find a contradiction you missed, fix it before reporting.
   If you find a contradiction you cannot confidently resolve, report `{"status":"stuck","stuck_type":"logic","reason":"self-verification found an unresolved contradiction in <file>: <description>"}` instead of `{"status":"success"}`.

Never use `git checkout --ours` or `git checkout --theirs` — they silently discard one side of the conflict.

## Report

Your last output line MUST be a bare JSON object (no code fence, no backticks):

On success (nothing discarded):

{"status":"success"}

On success with discarded content — if you had to drop content from one side (e.g. two sides made mutually exclusive changes and only one could survive), list each dropped item:

{"status":"success","discarded":["<short description of what was dropped from which side>"]}

An empty or absent `discarded` field means nothing was lost.
If anything was discarded, you MUST list it;
an empty list when content was actually dropped is a protocol violation. `discarded` also carries the step 4 ambiguous-case entry `"kept both sides of a conflict, ambiguous move-vs-duplicate"` — even though nothing was technically dropped in that case, the field's purpose is to surface anything the operator should double-check before `git merge --continue`, which covers both a genuine drop and a kept-both ambiguity.
The `mill-merge-in` frontend reads this field and surfaces any losses (or ambiguities) to the operator before continuing, rather than silently running `git merge --continue`.

If you cannot resolve one or more conflicts:

{"status":"stuck","stuck_type":"logic","reason":"<one-line description of what you could not resolve>"}

Anything other than this JSON object on the last line is a protocol violation;
the merge-in dispatcher treats that as stuck_type: logic with reason "no structured report" — your work is lost.
Do not wrap the JSON in a code fence;
do not add commentary after it.

## Tools

Available: Read, Edit, Write, Bash, Grep, Glob.
Use `git -C /home/knatte/Code/loomyard/wts/shedadapters-burler-producer` for any git commands;
do not `cd`.
Worktree cwd is `/home/knatte/Code/loomyard/wts/shedadapters-burler-producer`.
