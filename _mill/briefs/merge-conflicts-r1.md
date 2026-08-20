# Conflict Resolution Brief

Your sole job is to resolve git conflict markers in the listed files, stage each resolved file, and report success.
Do NOT commit.
Do NOT run `git merge --continue` — the SKILL does that after receiving `{"status":"success"}`.

## Task intent

These excerpts describe what THIS branch is trying to accomplish.
When the merge introduces a parent-side change that conflicts with this branch's intent, the resolution preserves THIS branch's intent.
In particular: if a file appears under a batch's `Deletes:` list and the merge introduces a modified version of that file from the parent, the resolution is to delete the file (your branch's intent overrides).
Stage the deletion with `git -C /home/knatte/Code/loomyard/wts/landing-publish-finalize-producers rm <file>`.

### From discussion.md

# Discussion: landing: Publish + Finalize producers

```yaml
task: 'landing: Publish + Finalize producers'
slug: landing-publish-finalize-producers
status: discussing
parent: standalone-producers
```

## Problem

`loom`'s producer list (`internal/loomshed`) ends with two stub rows: `Publish` (row 12) and `Finalize` (row 13).
A `Shed` run therefore walks the whole phase machine and then stops, having produced an implemented, reviewed branch that nobody publishes and nobody merges back.
These two rows are the last mechanical gap between "the phase machine runs" and "a loom run closes itself out".

Why now: the `fabric: merge-conflict primitive` task landed on the parent branch (commit `a2bf44e2`), which was the sole blocker.
`Fabric` now carries the whole merge lifecycle — `MergeIn`, `Merge`, `MergeContinue`, `MergeAbort`, `MergeInProgress` — so the shared merge-in/conflict-resolution engine both producers need can finally be built on top of a real primitive rather than sketched against a missing one.

Neither producer is `loom`'s own.
Each is an ordinary `ShedProducer` any `Shed` producer list may name — `loom`'s today, the Someday `Hardener`'s later — one definition named twice, never copied, never `Shed`-special-cased.

## Scope

**In:**

- `internal/mergeresolve` — the shared merge-in + conflict-resolution engine.
  Wraps `Fabric.MergeIn(parent)`, and on conflict spawns a fresh higher-capability LLM session in a clean session to resolve it, verifies the resolution mechanically, and concludes or aborts.
  Not a producer; a plain package both producers call.
- `internal/landingshed` — the two `ShedProducer` implementations (`Publish`, `Finalize`), their `Deps` struct, and the `landing.yaml` config surface.
- `contracts/stencils/landing/landing-template-conflict.md` — the conflict-resolution prompt stencil.
- `internal/landingshed/template.yaml` — the embedded `landing.yaml` template, reconciled by `lyx config reconcile` like every other module config.
- `fabricengine.MergeStageResolved(paths []string) (StageResult, error)` — a new narrow verb on `Fabric` that stages resolved conflict paths, without which `MergeContinue` can never succeed after an agent resolution.
  Brings with it a `StageResult` type embedding `MutationRecord`, a new `Kind` member in `mutation.go`, and its `internal/gitrepo` staging counterpart, which must be added to the gitrepo Client Boundary Invariant's pinned method list in the same commit.
- `gitrepo.RemoteURL(name string) (string, error)` — a go-git-backed local config read.
- A path-told, rebase-free push wrapper in `internal/fabricengine` over `gitrepo.PushRebaseFree`.
  Named in `fabricengine` (an owner package), never in `landingshed` — see `landing-never-names-fabric-vocabulary`.
- `githubclient.ParseOwnerRepo(remoteURL string) (owner, repo string, err error)` — a pure stdlib parser.
- Wiring: `internal/loomshed`'s rows 12 and 13 swap their `newStub(...)` backing for the real producers, and `loomshed.Deps` grows a single `Landing landingshed.Deps` passthrough field.
- Registration sites, without which the listed artifacts are inert:
  - `contracts/stencils/stencils.go` — a `//go:embed landing/landing-template-conflict.md` var plus its `entries` registry row.
    `contracts/stencils/registry_test.go` fails in both directions on an unregistered or unembedded `.md`.
  - `internal/configreg`'s `Modules()` — a `{Name: "landing", Template: landingshed.ConfigTemplate}` entry, plus the matching addition to `configreg_test.go`'s `want` list.
    Without it `lyx config reconcile` never sees `landing.yaml`.
  - `internal/loomshed/seam_enforcement_test.go`'s `loomshedAllowedImports` — gains `internal/landingshed`.
  - `cmd/lyx/configstrictness_test.go`'s strict pinned set — gains `landingshed`.
  - `CONSTRAINTS.md`'s Told-Geometry Invariant machine-enforced list — gains both new packages.
- Docs, same commit: delete `manifest/designs/landing.md`; fold durable content into the two new packages' `doc.go` files; move roadmap item 1 from Planned to Done; correct the Someday item's wording; add both packages to `docs/overview.md`'s module table.

**Out:**

- The discrepancy-document conflict shape.
  Only the ordinary-git-conflict shape shipped with the merge-conflict primitive; the document shape stays a Someday roadmap item.
- Raddle regeneration.
  Raddle does not exist as code, so there is nothing for `Finalize` to call and no seam is built for it.
- Worktree, branch, junction, and portal teardown.
  `lyx fabric cleanup` already owns all of it.
- Any "`_lyx` teardown" step inside `Finalize`.
  The phrase was loose wording in `landing.md` and names nothing in the codebase.
- The out-of-`Shed` PR-comment-resolution flow (a human-triggered interactive CLI session).
  Not designed, not a producer, not this task.
- Any new `lyx` CLI command.
  Both producers are reached only through a `Shed` run.
- Real network calls in tests.
  No test contacts GitHub or a real model.

## Decisions

### package-split-mergeresolve-and-landingshed

- Decision: two packages — `internal/mergeresolve` (the engine) and `internal/landingshed` (the two producers plus `landing.yaml`).
- Rationale: mirrors the already-established `loomengine`/`loomshed` split.
  `landingshed` gets its own `seam_enforcement_test.go` policing `internal/lyxcwd` out of its production import set, exactly as `internal/loomshed` does, so the Told-Geometry Invariant is machine-enforced rather than a review obligation.
  A future `hardenershed` fills the same `landingshed.Deps` from its own geometry, with no duplication.
- Rejected: putting the producers in `internal/shedadapters` — that package holds thin seams over already-generic engines with no domain logic of their own, and `loomshed`'s own `doc.go` explicitly forbids touching it;
  `Publish`/`Finalize` carry real domain logic (the require-PR check, PR creation, merge orchestration).
  Also rejected: one combined `internal/landingengine` — the engine half is genuinely shared and shouldn't import the producer half.

### git-conflict-shape-only

- Decision: `mergeresolve` handles only the ordinary git-conflict shape, i.e. `MergeResult.Conflicts` — a flat, lexically sorted, deduplicated list of unified, worktree-relative paths.
  No shape-detection abstraction, no result variant anticipating a second shape.
- Rationale: only that shape shipped with the merge-conflict primitive.
  The discrepancy-document shape is a parked Someday roadmap item, and building a two-variant type with one variant permanently empty is designing for a hypothetical requirement.
- Rejected: a two-variant result type with only one variant populated.
- Consequence: `manifest/designs/landing.md`'s two-shape section and the roadmap's Someday item both get corrected in this task's commit, so no document promises what the code does not have.

### no-raddle-hook

- Decision: `Finalize` builds nothing for Raddle — no hook, no injectable interface, no no-op call, no scaffolded lock span with an empty body.
  The fact that Raddle regeneration folds into `Finalize`'s merge critical section when Raddle lands is recorded in `Finalize`'s package documentation only.
- Rationale: Raddle is a Someday roadmap item with no code.
  An interface with a permanently-nil implementation is exactly the hypothetical-requirement design this codebase avoids.
  The Raddle task owns that wiring when it actually lands.
- Rejected: a `RaddleRegenerator` interface injected nil today; a pre-built lock span with an empty body.

### mergeresolve-drives-shuttle-directly

- Decision: `mergeresolve` drives `shuttleengine.Runner` through its own narrow one-method seam (same shape as `shedadapters.Shuttle`: `Run(shuttleengine.Spec) (shuttleengine.Result, error)`), not through `shedadapters.SingleLLMProducer`.
- Rationale: `shedadapters.SingleLLMProducer` **is** a `shedengine.ShedProducer`.
  `mergeresolve` is not a producer and has no `ShedProducer` seam to satisfy, so the adapter is structurally the wrong shape for it regardless of any output-file question.
  A one-method seam also keeps the package unit-testable against a fake with no `shedengine` import at all.
- **The `OutputFiles` requirement is a `Runner` rule, not an adapter rule.**
  `shuttleengine`'s `Spec.validate` (`spec.go:115`, called from `run.go:143` inside `Runner.Start`) hard-errors on an empty `OutputFiles` — "a run's output file IS its return value" — and separately rejects any entry that already exists on disk, because outcome classification tests bare file existence and a stale file would classify the run `done` on its first turn end.
  Bypassing `SingleLLMProducer` does not escape either rule.
- Decision, therefore: the conflict spec's `OutputFiles` names exactly one **fresh, absolute** artifact — a resolution report at `<ScratchDir>/conflict-resolution-r<attempt>.md`, which the session writes as its terminal act.
  `ScratchDir` is a **told absolute path** carried in `landingshed.Deps` and passed down to `mergeresolve`;
  the caller resolves it as `<AnchorPath>/.lyx/landing`, exactly as `loomengine.LoomRunLock` resolves its own (`filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, "loom", …)`).
  Absolute matters: `Spec.validate` resolves a *relative* `OutputFiles` entry against `worktreeRoot`, not `AnchorPath()`, so on any anchored hub (`AnchorRel != "."`) a relative `.lyx/…` entry would land at `<worktreeRoot>/.lyx/…` — the wrong directory, and a Durable-vs-Ephemeral violation, since `.lyx` is `_lyx`'s sibling under the anchor.
  Neither package may derive that path itself, per the Told-Geometry Invariant.
  There is deliberately **no** `_lyx/landing` twin: the report is ephemeral debugging output with no durable counterpart, so nothing belongs on the durable side.
  **Whoever writes into `ScratchDir` `os.MkdirAll`s it first**, on every write path — the conflict report in `mergeresolve`, the stuck-reason files in `landingshed`.
  Creating a *told* directory is legal under the Told-Geometry Invariant and is exactly what `shedengine` already does for its two lock parents ("the only paths the package constructs are the two lock parents it creates so a told path is usable").
  Deriving the path would not be legal;
  making a told one usable is.
  The path is **per-attempt** (`r1`, `r2`), because `validate` rejects a pre-existing entry and the one-retry path would otherwise fail its second `Runner.Start` on the first attempt's own artifact.
  The report is not parsed for control flow — `mergeresolve`'s marker scan over `MergeResult.Conflicts` remains the verification, per `verify-before-conclude`.
  It exists to satisfy the runner contract and to leave a human an audit trail of what the session claims it resolved.
- Rejected: passing the conflicted paths as `OutputFiles` — they exist on disk by definition, so `validate` rejects them outright;
  changing `shuttleengine` to permit an empty `OutputFiles` (that would remove the fail-loud guard protecting every other shuttle consumer, for one caller's convenience).

### verify-before-conclude

- Decision: after the LLM session returns, `mergeresolve` verifies mechanically — it re-reads each path from `MergeResult.Conflicts` and checks for remaining conflict markers.
  Clean → stage the resolved paths via the new `MergeStageResolved` verb (see below), then `MergeContinue`.
  Still marked → one retry of the session, then on a second failure `MergeAbort` and report stuck.
- **An absent path is resolved-by-deletion, not a failure.**
  `ConflictedFiles()` includes delete/modify conflicts, whose correct resolution is that the file is gone.
  A path that no longer exists on disk therefore skips the marker scan and counts as resolved;
  it is still passed to `MergeStageResolved`, which stages the removal.
  Only a read error that is *not* "does not exist" is a genuine failure.
- **The marker scan is content-only, line-anchored, and deliberately biased toward refusing.**
  It matches `<<<<<<< `, `=======`, and `>>>>>>> ` only at the start of a line.
  Consequence, stated rather than hidden: a resolved file whose own legitimate content carries line-start conflict markers can never pass, and that merge escalates to `Stuck`.
  That is the safe direction — refusing to conclude an irreversible `MergeContinue` — and such a file is out of scope for automated resolution.
  The conflict stencil itself must therefore never contain literal line-start markers;
  it describes them indented or fenced, since the stencil is a file the agent may well copy from.
- Note the layering: this scan is `mergeresolve`'s own pre-check, and `MergeContinue`'s index guard remains the authoritative gate.
  The scan can only make `mergeresolve` refuse something Fabric would have accepted, never the reverse.
- Rationale: `MergeContinue` is irreversible at the Fabric layer — `internal/fabricengine`'s package documentation states there is no post-conclude undo, and `MergeAbort` covers only the uncommitted attempt window.
  Verify-before-conclude with `MergeAbort` as the checkpoint is the discipline the merge primitive itself was designed around.
- Rejected: trusting the session's `Done` verdict and concluding unconditionally;
  leaving the pair mid-merge without aborting (that strands the worktree in a state the next run must clean up, and contradicts the abort-is-the-checkpoint decision).

### merge-stage-resolved-verb

- Decision: `internal/fabricengine` gains a narrow verb, `MergeStageResolved` (full signature in the Signature bullet below).
  It takes unified, worktree-relative paths and stages each on the side that actually has it unmerged.
- **The discriminator is index membership, not a path prefix.**
  `mergepaths.go`'s `weftPathVisible` is a *total* function — under a wired name ⇒ weft, otherwise ⇒ warp — so "the inverse of `unifyConflictPaths`" has no third outcome and could never produce a "maps to neither side" error.
  The verb instead reads each side's `ConflictedFiles()` and stages a path on whichever side lists it.
  A path listed by **neither** side is the error condition: the caller passed something that is not conflicted, which is a caller bug worth failing loudly on rather than silently staging.
  A path listed by both sides cannot occur — `unifyConflictPaths` already treats that collision as `unmappable` and self-aborts the merge.
- **Deletions stage too.**
  A delete/modify conflict is legitimately resolved by the file being gone, so the underlying `gitrepo` call must use a form that stages a removal (`git add -A -- <paths>`), never one that errors on a missing path.
  `mergeresolve` calls it after its own marker scan passes, immediately before `MergeContinue`.
  `MergeContinue`'s existing index guard is left exactly as shipped.
- Rationale: `MergeContinue` (`mergelifecycle.go:104-113`) refuses with `mergeReasonUnresolvedConflicts` while either side's `ConflictedFiles()` is non-empty, and that is `git diff --name-only --diff-filter=U` — an *index* probe.
  Editing a file's content never clears its unmerged index entry, so without a staging step `MergeContinue` can never succeed after an agent resolution.
  No actor is otherwise able to stage: the agent is forbidden git outright, `mergeresolve` is bound by the Fabric Git Invariant, and `fabricengine`'s merge surface exposes no staging verb.
  Index manipulation is a Fabric primitive's job, not `mergeresolve`'s.
  Keeping `MergeContinue`'s guard untouched gives defence in depth — `mergeresolve`'s marker scan and Fabric's index check are two independent gates, not one relocated one.
- Rejected: making `MergeContinue` stage every conflicted path itself and scan for markers instead of the index — that changes a shipped, already-reviewed surface and removes precisely the guard a human running `lyx fabric merge --continue` after their own `git add` depends on.
  Also rejected: an opt-in self-staging flag on `MergeContinue` — added complexity on that same shipped surface for a need that is entirely `mergeresolve`'s.
- Signature: `MergeStageResolved(paths []string) (StageResult, error)`, where `StageResult` embeds `MutationRecord`.
  Staging mutates the index, so it is a mutating verb, and the Mutation Record Invariant requires every mutating verb's result type to embed the record.
  A new `Kind` member for the staging primitive lands in `internal/fabricengine/mutation.go` in the **same commit** as its recording site — a `Kind` with no recording site is caught by review, not by any guard.
  The guard row that *is* required is `{"StageResult", "internal/fabricengine/<file>.go"}` in `cmd/lyx/destructiveguard_test.go`'s `destructiveGuardMutatingResultTypes` table (line 154), which pins every mutating result type that must embed `MutationRecord`.
  The record is appended only after the primitive observably changed state, never on a no-op.
- Obligation: the underlying `internal/gitrepo` staging method reaches the git CLI, so it must be added to the gitrepo Client Boundary Invariant's pinned method list in the same commit, or `cmd/lyx/gitrepoboundary_test.go` fails.
  It uses the checked form, so it adds no `//gitexec:raw` site and leaves the gitexec Checked-Call Invariant's pinned counts unchanged.

### crash-recovery-aborts

- Decision: `mergeresolve` probes `Fabric.MergeInProgress()` at entry.
  A merge in progress → `MergeAbort` unconditionally, then start a clean attempt.
- Rationale: `Shed` re-calls a producer verbatim after crash recovery, so this path is required, not optional.
  A half-resolved worktree left by a killed LLM session is not a state to resume into blindly;
  `MergeAbort` restores both sides to their pre-merge SHAs, which is a known-good starting point.
- Rejected: resuming the session against still-conflicted paths (risks building on a partially-corrupt state);
  reporting stuck immediately (turns every crash into human work when a clean retry is available).

### foreign-merge-state-refused

- Decision: `*ErrForeignMergeState` — real git merge state a human left in the worktree that fabric did not start — is never aborted, never touched.
  `mergeresolve` returns stuck and the producer escalates.
- Rationale: not a judgment call.
  Every mutating fabric merge verb already refuses `*ErrForeignMergeState` rather than touch it;
  `mergeresolve` inherits that refusal.
  Assuming a human's in-flight merge is disposable is exactly the failure mode the refusal exists to prevent.
  Note `MergeInProgress` (read-only) reports `false` for foreign state, so the crash-recovery probe above does not catch this case — the abort call itself surfaces the typed error.
- Rejected: aborting it and proceeding.

### landing-never-names-fabric-vocabulary

- Decision: neither `internal/landingshed` nor `internal/mergeresolve` may contain a `warp` or `weft` token in **any** identifier, string literal, or comment — including a selector on a `fabricengine` call.
  Three consequences follow, and they change `Deps`:
  1. `Deps` carries **no** warp-path field.
  2. The rebase-free push is injected as a third closure, `PushBranch func() error`, filled by the CLI layer alongside the two Fabric openers.
     That layer names the `fabricengine` push verb;
     `landingshed` only calls `deps.PushBranch()`.
  3. The `origin` URL is a told string, `OriginURL`, read by the same CLI layer via `gitrepo.RemoteURL`.
     `landingshed` calls only `githubclient.ParseOwnerRepo(deps.OriginURL)`, which names nothing forbidden.
- Rationale: this is machine-enforced, not a style rule.
  `internal/lyxcwd/enforcement_test.go`'s `fabricVocabularyHits` walks the AST and flags a bare vocabulary token inside **any** `*ast.Ident` (line 712-715), plus any string literal or comment, for every non-owner production file.
  Neither new package is in the owner set, so `fabricengine.PushWarpRebaseFreeAt(...)` written inside `landingshed` fails the build on the identifier alone — the call would not even need to run.
  The merge verbs `mergeresolve` calls are unaffected: `Open`, `MergeIn`, `Merge`, `MergeContinue`, `MergeAbort`, `MergeInProgress`, and `MergeStageResolved` are all vocabulary-free by construction.
  The closure injection is not a workaround invented here — it is the same shape already chosen for the two Fabric openers, extended to the one other call whose name is not vocabulary-free.
- Consequence for docs: both packages' `doc.go` files describe one repo, never two.
  The design content folded in from `landing.md` must be reworded accordingly, since `landing.md` itself discusses the split freely.
- Rejected: renaming the `fabricengine` verb to something vocabulary-free — the token is accurate *inside* the owner package, and bending an owner's own naming to suit a consumer inverts the invariant's direction.

### stuck-reasons-are-logged-and-filed-never-returned

- Decision: every "`Stuck` with a distinct message" in this discussion means two concrete things, because `Shed` has no reason channel of its own:
  1. a `logger.Warn` line with structured key/value fields (`producer`, `reason`, plus the case's own fields — branch, PR number, path, error);
  2. a one-line reason file written to `<ScratchDir>/<producer>-stuck.md`, overwritten each attempt.

  The producer still returns bare `shedengine.Stuck`.
- Rationale: `ShedProducer.Call` returns only `(Outcome, OutputPointer, error)` (`producer.go:31`), and on a `Stuck` with `OnStuck: ""` `Run` persists the **fixed** string `"stuck with no OnStuck target"` (`run.go:190-200`) — a producer-supplied reason has nowhere to go.
  Returning an error instead is not a substitute: that persists `StateFailed` and aborts the run, where these cases want `blocked`, which a human resumes.
  `OutputPointer` cannot carry it either — `shedengine` never persists that field anywhere.
  The `logger.Warn` half is established precedent: `shedadapters/singlellm.go:104` already logs exactly this way for its own non-`Done` outcome.
  The file half exists because a log line scrolls away in an unattended run, and because it gives the tests something to assert on.
- Consequence for testing: every test in this discussion asserting that two `Stuck` cases are "distinguishable" asserts on the **reason file's contents**, never on a returned reason string and never on `Run`'s persisted reason, which is the same fixed text in all of them.
- Rejected: adding a reason field to the `ShedProducer` contract — that changes a shipped seam with a machine-enforced import allowlist, for one consumer's benefit;
  returning an error to smuggle the message out, which flips `blocked` into `failed` and ends the run.

### unlisted-typed-merge-errors-escalate

- Decision: any typed merge error `mergeresolve` does not name explicitly — `*ErrUnmergeableState`, `*ErrMergeIncomplete`, `*MergeGuardError`, `*ErrNoMergeInProgress`, `*ErrMergeInProgress` — is a catch-all `Stuck`, surfaced with the error text, and `mergeresolve` calls no `MergeAbort` of its own.
- Rationale: `MergeIn` returns `*ErrUnmergeableState` when `unifyConflictPaths` finds a path that maps outside the single visible worktree tree, and it **already self-aborts the whole attempt** before returning — a second abort would hit `*ErrNoMergeInProgress` and turn a clear diagnosis into a confusing one.
  The guard errors likewise refuse before mutating anything, so there is nothing to unwind.
  A default of escalate-and-say-why is right for a class of errors whose members are each a genuine human-diagnosable condition, and it means a future `fabricengine` error type does not silently fall into a wrong branch.
- Rejected: aborting unconditionally on any error (double-abort noise, and it discards state a human may need);
  treating unlisted errors as retryable.

### publish-require-pr-is-a-base-list

- Decision: `require_pr_to_base` is a list of base-branch names, not a bool.
  `Publish` requires a PR only when the told parent branch matches an entry.
  Default template value: `[main]`.
  No match → no-op `Done`, no merge-in, nothing else.
- Rationale: tasks branch off other tasks in this workflow (this very task's parent is `standalone-producers`, not `main`), so a bool would force a PR on every intermediate task-to-task merge.
  `landing.md`'s own gloss — "no direct merge to `main` without a PR" — is a statement about *which* base, not about all of them.
- Rejected: a bool flipped per run in `loom.yaml`/`hardener.yaml` — the parent branch is a per-task runtime fact a static profile file cannot know in advance, so someone would hand-edit config per task, and forgetting it silently removes the PR gate on a `main`-bound merge.
  Also rejected: hardcoding "only if parent is the repo's default branch" — that hardcodes precisely what `landing.yaml` exists to configure, against the "profiles live in the caller, not the callee" precedent.

### publish-blocks-on-open-pr

- Decision: when `Publish` opens (or finds) a PR that is still open, it returns `shedengine.Stuck`, not `Done`.
  With `OnStuck: ""` on row 12, `Shed` persists `blocked` and the run ends there.
  The human-triggered out-of-`Shed` flow later resumes the run, and `Publish` re-runs from row 12.
- Rationale: as `landing.md` was written, `Publish` returning `Done` would let `Shed` advance to `Finalize` (row 13) and merge to the parent seconds after opening the PR, defeating the PR entirely.
  `Stuck` under `OnStuck: ""` means exactly "a human must act before this proceeds", and reuses the `blocked` machinery `shedengine` already has.
- Rejected: making `Finalize` refuse to merge while an open PR exists — that duplicates the check in the wrong place;
  it is `Publish`'s own job that is not finished, not something `Finalize` should discover.
  Also rejected: accepting the immediate merge and treating the PR as a record rather than a gate.

### publish-resume-reads-pr-state

- Decision: a re-called `Publish` queries GitHub for a PR with `head:<taskBranch>` and `base:<parentBranch>` and branches on its state:
  - none found → do the merge-in, open the PR, return `Stuck`;
  - found, `state: open` → return `Stuck` again, no second PR and no merge-in;
  - found, `state: closed` and `merged: true` → return `Done`, run proceeds to `Finalize`;
  - found, `state: closed` and `merged: false` → return `Stuck` with a distinct message.
- Rationale: GitHub is the authoritative source, with no local state to go stale — the same reasoning that settled the duplicate-PR check.
  A merged PR does not make `Finalize` redundant: GitHub only ever sees the **warp** repo, so the weft branch was never in the PR at all.
  After a GitHub-side merge, `Finalize`'s `Merge` syncs the target to its upstream first and finds the warp side `AlreadyUpToDate`, while the weft side genuinely merges — the correct division of work, not a no-op.
  A PR closed without merging must never read as "proceed"; it is a human decision to stop.
- Rejected: branching on review-approval state instead — it leaves the PR open, needs an explicit close call (a squash merge will not auto-close it), and decouples "ready" from "actually landed", opening a race if someone pushes after approval.
  Also rejected: an explicit readiness marker written by the out-of-`Shed` flow — that reintroduces the second source of truth already rejected for the duplicate check.

### publish-repo-resolution

- Decision: owner/repo come from the warp repo's `origin` remote URL;
  the PR base is the told parent branch;
  title and body come verbatim from `websterengine.ParseSummary` (`Summary.Title`, `Summary.Body`), with no LLM call in `Publish`.
- Mechanism, since none exists today: `internal/gitrepo` gains `RemoteURL(name string) (string, error)`, implemented with **go-git** (`Remote(name).Config().URLs[0]`).
  `internal/githubclient` gains a pure stdlib parser, `ParseOwnerRepo(remoteURL string) (owner, repo string, err error)`, accepting the SSH form (`git@github.com:owner/repo.git`), the HTTPS form (`https://github.com/owner/repo.git`), an optional `.git` suffix, and an optional trailing slash.
- **`Publish` pushes the warp branch before it creates the PR.**
  Nothing else does: agents commit per fix on warp and never push (Review Round Invariant), so without this step `<taskBranch>` exists only locally, `PullRequests.Create` fails 422, and the resume query for `head:<taskBranch>` could never match either.
  The push happens **after** the merge-in and **before** the existing-PR query, so the branch GitHub is asked about is the branch that exists.
  On a re-run that finds an open PR, the push still runs first, so a resumed `Publish` also refreshes the PR with any commits added since.
- **The push must be rebase-free, and that rules out `PushWarpAt`.**
  `fabricengine.PushWarpAt` (`spawn.go:89`) routes to `gitrepo.PushCoalesced` → `pushWithRebaseRetry`, which runs `git pull --rebase` on a rejected push (`push.go:43-83`).
  That rewrites the **warp** task branch's SHAs while the weft side is not rebased, which desynchronizes the pair and invalidates the correspondence index `RecordCorrespondence` maintains.
  `PushCoalesced` additionally writes `gitrepo.PushLockFileName` (`.gitrepo-push.lock`) at the repo root, and unlike the weft side (`seedWeftArtifactExcludes`) the warp repo has no exclude entry for it — `spawn.go`'s own doc comment names this as an undischarged precondition for any future caller.
- Decision: a new thin, path-told wrapper in `internal/fabricengine` routes to `gitrepo.PushRebaseFree` (`push.go:90`) instead.
  Its name necessarily carries a fabric-vocabulary token, so `landingshed` never names it — see `landing-never-names-fabric-vocabulary` below.
  It never rebases and never takes the push lock, so **both** hazards above are discharged rather than mitigated: no SHA rewrite, and no untracked residue in the operator's own repo.
  `PushWarpAt` keeps its "no production caller" status and its doc comment stays accurate — no edit needed there, and no warp-side exclude seeding enters this task's scope.
  A rejected push surfaces as `gitrepo.ErrPushRejected`, meaning the remote task branch has commits this checkout lacks;
  `Publish` returns `Stuck` on it, since a human has to decide what happened.
  Any other push error → `Stuck` too, with the error surfaced.
  No PR is attempted in either case.
- **The skip flags are refused, not silently honoured.**
  `SyncOptions` is bound by the CLI layer inside the `PushBranch` closure, not carried in `Deps` — but the skip decision is still `Publish`'s, so `Deps` carries a plain `PushSkipped bool` the same layer sets.
  The new `fabricengine` wrapper mirrors the existing one's gating and returns an empty result with a nil error when `opts.SkipGit || opts.SkipPush`.
  Relying on that would produce a PR for an unpushed branch — a silent 422, exactly the failure this whole decision exists to prevent.
  So `Publish` checks `deps.PushSkipped` **itself**, before calling the closure, and returns `Stuck` when it is set and the base branch requires a PR.
  When no PR is required the flag is irrelevant, because that branch no-ops before reaching the push at all.
- **Only the GitHub-visible side is pushed.**
  The PR is an artifact of the repo GitHub can see, and `Fabric`'s other internal side is invisible to it, so `Publish` has no reason to push anything else.
  That side's own remote state is `Finalize`'s merge and fabric's sync path to deal with, not the PR's.
  Stated this way deliberately: `landingshed`'s own source may not name the two sides at all (see below).
- Failure mode: an absent `origin`, an unparseable URL, or a non-GitHub host makes `Publish` return `Stuck` with a distinct message — never a silent no-PR `Done`.
  Silently skipping the PR when the base branch demanded one is the one outcome the gate exists to prevent.
- Rationale: a hardcoded `owner/repo` constant like `selfreportengine.targetRepo` is right for self-reporting (always the loomyard repo) and wrong here — `Publish` runs against whatever repo the hub was cloned from.
  Reading a remote URL is a read-only git query, explicitly exempt from the Fabric Git Invariant's mutation rule.
  Implementing it with go-git rather than `gitexec` keeps it on go-git's side of the gitrepo Client Boundary Invariant — it resolves state already on disk, spawns no process, and therefore adds no entry to that invariant's pinned `gitexec` method list.
  The parser belongs in `githubclient` because that package owns GitHub knowledge, and a stdlib-only string function sits inside its existing import allowlist.
  `summary.md` is documented as "the future loom-finalize PR-text source", which this is.
- Rejected: implementing the read with `gitexec` (`git remote get-url origin`) — same result, an unnecessary process spawn, and it would force a Client Boundary Invariant list update nobody asked for.
  Also rejected: owner/repo as a required `landing.yaml` key (duplicates what git already knows and goes stale on a fork or rename — the same fragility as a hardcoded constant, moved one layer out);
  a hub-level `lyx config` setting (same problem, further away).

### finalize-merge-geometry

- Decision: `Finalize` always calls `mergeresolve`'s merge-in against the parent first, from the task worktree.
  It then obtains the parent pair's `*Fabric` through its injected `OpenParentFabric` closure (see `fabric-handles-are-injected-closures` below) and calls `parentFabric.Merge(taskBranch, opts)`.
  On `*ErrMergeInRequired` it re-runs `mergeresolve` in the task worktree and retries the parent-side `Merge` exactly once;
  still failing → `Stuck`.
- Rationale: this is the `merge-in` here, then `merge` there flow `lyx fabric merge`'s own help text documents, and `Merge` structurally requires the target pair's handle.
  One retry rather than zero, because the window between `Finalize`'s own catch-up merge-in and the later parent-side `Merge` is genuinely unprotected: no lock spans both, so a competing task can land in the parent in between.
  `*ErrMergeInRequired` there is a real, if rare, drift case rather than an impossible state.
- Rejected: zero retries (treats real drift as a bug);
  merging from the task worktree's own handle (`Merge` needs the target's handle — structurally impossible).

### fabric-handles-are-injected-closures

- Decision: `landingshed.Deps` carries two injected opener closures — `OpenFabric func() (*fabricengine.Fabric, error)` for the task pair and `OpenParentFabric func() (*fabricengine.Fabric, error)` for the parent pair — filled by the CLI/orchestrator layer, which legitimately imports `internal/lyxcwd`.
  `mergeresolve` never opens a handle;
  it is handed one, behind its own narrow merge interface.
- Rationale: `fabricengine.Open(l *lyxcwd.Location)` is the only exported constructor (`newPaired` is unexported), so opening a handle inside either new package would require a direct `internal/lyxcwd` import — exactly what both packages' `seam_enforcement_test.go` forbids, and what the Told-Geometry Invariant exists to prevent.
  The closure-injection shape is established precedent, not invention: `internal/perchcli` already carries `openFabric func() (*fabricengine.Fabric, error)` (`cli.go:64`), assigns it as a closure over a resolved `Location` in hub mode (`wiring.go:140`), and sets it nil in standalone mode (`wiring.go:225`).
  Laziness matters for the same reason `perchcli` documents: `fabricengine.Open` stat-checks the paired layout, so opening eagerly would fail before `Preflight` has confirmed fabric is wired at all.
- **Where the parent pair's path comes from.**
  The parent is known only as a branch name (`loomengine.Status.Parent`), so the CLI/orchestrator layer resolves it before building the closure: `fabricengine.List(sourceDir)` returns `[]WorktreeEntry{Path, Head, Branch, Main}`, and the entry whose `Branch` equals the parent branch gives the parent worktree's path.
  That path then goes through `lyxcwd.ResolveWorktree(path)` → `fabricengine.Open(loc)`, all inside the layer that already imports `lyxcwd`.
  `landingshed` sees only the closure.
- **Nobody fills these closures in this task, and that is deliberate.**
  `loomshed.New` has no production caller anywhere in the tree today — only `internal/loomshed/*_test.go` reference it, there is no `loomcli`, and this task adds no `lyx` command.
  The resolution chain above is therefore **specified here but not built here**: `landingshed.Deps` declares both closure fields, `loomshed.Deps` passes them through, and the roadmap's next item, `loom: session bootstrap` (which builds `lyx loom run`), is what fills them with the `List` → branch-match → `ResolveWorktree` → `Open` chain.
  Until then both closures are exercised only by this task's own tests, which fill them directly against `hubforge` fixtures — legitimate, since test files are not bound by the Told-Geometry Invariant's import rule.
  A nil closure is a construction error, not a silent no-op: `NewFinalize` rejects a nil `OpenParentFabric` up front, the same way `loomshed.New` already rejects a nil `deps.Preflight`.
- **No live pair for the parent branch** → `OpenParentFabric` returns an error and `Finalize` returns `Stuck` with a distinct message naming the branch.
  `Finalize` never creates a worktree to merge into;
  materializing a pair is `lyx fabric add`'s job and a human's decision.
- **Parent worktree dirty** → `Merge`'s own guard already refuses with `*MergeGuardError{Reasons: ["worktree dirty"]}` (`mergeguards.go`'s `pairDirtyReason`).
  `Finalize` surfaces that reason verbatim and returns `Stuck`.
  It never stashes, never resets, and never force-merges — someone has uncommitted work in the parent, and only they can decide what happens to it.
- Rejected: a path-based `fabricengine.OpenAt(warpPath string)` constructor — `Open`'s `*lyxcwd.Location` argument is what lets `fabricengine` derive weft pairing and junction geometry correctly, so a bare-path constructor would either duplicate that derivation outside the package that owns it or assume simplifications that do not always hold.
  Also rejected: `loomshed` resolving both handles eagerly and passing `*Fabric` values in `Deps` — that reintroduces into `loomshed` the very `lyxcwd` import its own seam test forbids, and opens a handle before `Preflight` has run.

### finalize-squash-default-true

- Decision: `Finalize` squashes by default.
  `landing.yaml` carries a `squash` key defaulting to `true`, overridable per orchestrating profile, threaded into `fabricengine.MergeOptions{Squash: ...}`.
- Rationale: a loom run produces one commit per Card, so an ordinary merge floods the parent with implementation noise.
  The ancestry cost is known and accepted: `internal/fabricengine`'s documentation notes a squash-merged branch carries no merge commit linking it to its target, so "was this branch merged?" is unanswerable from git alone after a squash.
  That trade-off was weighed against commit noise and squash won.
- Rejected: ordinary merge by default;
  squash hardcoded with no config key.

### rows-12-13-escalate-on-stuck

- Decision: both rows keep `OnStuck: ""` — escalate to a human, never bounce.
- Rationale: the same reasoning `loomshed.New`'s own documentation gives for `Preflight` and `Batchifier` — a gate whose guarded artifact is produced by no row in the list has nothing to bounce to.
  An unresolvable merge conflict against the parent's current state, an unreachable GitHub, a drifting parent, and an open PR awaiting human review are all things only a human fixes.
- Rejected: `Finalize` bouncing to `Publish` (re-opening a PR does not fix a merge conflict);
  bouncing to `Webster` (a conflict against the parent's current state is not a defect in Webster's diff, and re-running Webster resolves nothing).

### landing-yaml-strict-loader

- Decision: `internal/landingshed` owns `landing.yaml`, loaded with `configengine.Load` (strict) against an embedded `template.yaml`, following `loomengine.LoadConfig`'s exact shape including load-time `modelspec.Parse` validation.
- Rationale: the Config Strictness Invariant's membership rule is whether the module has, or is slated to have, a standalone entry point.
  Neither producer has one — both are reachable only through a `Shed` run inside a hub, where an absent config means a broken hub, not a supported config-less mode.
  That is why `shuttleengine`/`reedengine`/`perchengine`/`websterengine`/`batcher` are on the degrading side and these are not.
- Rejected: `LoadOrTemplate` (degrading);
  putting the config in `internal/mergeresolve` (the engine is told its values, and `require_pr_to_base`/`squash` are producer concerns, not engine ones).

### told-values-via-landingshed-deps

- Decision: `landingshed.Deps` carries every told value — worktree root, task branch, parent branch, webster dir, stencils dir, modelspec registry, `OriginURL`, the two Fabric opener closures, the `PushBranch` closure, and `ScratchDir` (the told absolute `<AnchorPath>/.lyx/landing` path) — with `NewPublish(deps)` / `NewFinalize(deps)` constructors that reject a nil required closure up front rather than nil-panicking at call time.
  No field names a fabric-internal side, per `landing-never-names-fabric-vocabulary`.
- `Deps` deliberately carries **no** `AnchorPath` field.
  `ScratchDir` is the only thing landing would use an anchor for, and carrying both would be exactly the derived-path near-duplicate `loomshed.Deps`'s own doc comment warns invites silent divergence.
  Told rather than derived is also required, not merely preferred: deriving it would mean joining the `.lyx` literal, which the Lyxdirs Single-Declarer Invariant reserves to `lyxdirs.DotLyxDirName`, and computing geometry, which the Told-Geometry Invariant forbids these packages outright.
  `loomshed.Deps` grows a single `Landing landingshed.Deps` passthrough field, and `loomshed.New` constructs both producers from it.
- Rationale: mirrors `loomshed`'s own `Deps` pattern and keeps `loomshed` the place loom's list is assembled.
  A future `hardenershed` fills the same struct from its own geometry with no duplication.
- Rejected: flattening every landing value into `loomshed.Deps` as individual fields (pollutes `loomshed` with landing-specific fields and gives `Hardener` nothing to reuse);
  injecting both as pre-constructed `ShedProducer` values like `Preflight` (`Preflight` is special-cased because it is the only row that spawns git directly — `Publish`/`Finalize` should follow the `Deps`-driven construction that `Discussion-Validate`, `Plan-Validate`, and `Batchifier` use).

### conflict-stencil-is-a-file

- Decision: the conflict-resolution prompt is a new stencil, `contracts/stencils/landing/landing-template-conflict.md`, resolved from the told stencils directory.
  `mergeresolve` builds a `shuttleengine.Spec` with `Interactive: false`, `Role: "conflict"`, `Model`/`Effort`/`Version` resolved from `landing.yaml`'s `conflict` model-spec key through `modelspec.Registry`, and `Timeout` from `conflict_timeout_min`.
- Rationale: the Stencil Ownership Invariant requires prompts to come from the told stencils directory, never a Go string literal.
  Conflict resolution is its own task type; no existing stencil is written for it.
  The prompt names only unified, worktree-relative paths and never mentions warp, weft, or a second repo — `mergeresolve` is not in the Fabric Vocabulary Invariant's owner set, and the `templates-describe-one-repo` rule binds every agent prompt.
- Rejected: a Go string literal (violates the Stencil Ownership Invariant);
  reusing an existing stencil.

### docs-lifecycle-landing-md-deletes

- Decision: `manifest/designs/landing.md` is deleted in this task's final commit.
  Durable content folds into `internal/mergeresolve`'s and `internal/landingshed`'s `doc.go` files.
  `manifest/roadmap.md` moves item 1 from Planned to Done **and rewrites its body** to match what actually ships — the current body says "conflict-shape detection", "`_lyx` teardown", and "Returns `Done` once the PR exists", all three of which this discussion overturns.
  The Someday `finalize: the discrepancy-document conflict shape` item needs **no** change: it already reads "Only the ordinary-git-conflict shape shipped; the document shape is not built".
  `docs/overview.md`'s module table gains both packages.
- Rationale: `landing.md`'s own status banner says it deletes when both producers land, per the Documentation Lifecycle, and `loom.md`/`shed.md` already follow that pattern.
- Inbound-link inventory, enumerated by `grep -rn "landing.md" manifest docs internal contracts` rather than from memory, and cited by file because line numbers drift:
  - `manifest/designs/loom.md` — four links: producer-table rows 12 and 13, the Raddle-fold paragraph, the build-ordering paragraph.
  - `manifest/designs/shed.md` — four links: the status banner, the `Finalize` worked-example line, the task-bundling paragraph, the Related list.
  - `manifest/designs/raddle.md` — one **anchored** link, `landing.md#raddle-regeneration--part-of-the-merge-not-a-step-before-it`.
    An anchored link needs a replacement target that genuinely carries that content, so `Finalize`'s package doc must cover the merge-critical-section contract raddle.md points at.
  - `manifest/designs/fabric-unified-view.md` — one link, describing "the document-driven weft-conflict mechanism", which this task explicitly does not build;
    that reference needs rewording, not merely repointing.
  - `manifest/roadmap.md` — the `See [designs/landing.md]` line under item 1.
  - `internal/loomshed/loomshed.go:19` — a prose reference inside a Go comment, not a Markdown link, so the link checker will not catch it;
    repoint it by hand.
  - `CONSTRAINTS.md:427` — the Markdown Link Integrity bullet cites `landing.md`'s own outgoing `../../CONSTRAINTS.md#fabric-git-invariant-warp--weft` link as a live worked example of the anchor-resolution rule.
    Deleting the file makes that example stale.
    It is prose, so no test catches it;
    rewrite the bullet against a surviving example in the same commit.
  All of these move in the same commit, or Markdown Link Integrity breaks.
- Rejected: keeping `landing.md` updated to match what shipped.

## Technical context

### What already exists and must be reused

- **`internal/fabricengine`'s merge surface** (landed in `a2bf44e2`).
  `Fabric.MergeIn(source)` merges `source` into the current pair's own warp and weft checkouts, in the task worktree, and reports conflicts as a result state rather than an error: `(MergeResult{Conflicts: […]}, nil)`, leaving the pair mid-merge.
  `Fabric.Merge(source, MergeOptions{Squash, Message})` merges into the target pair a separate handle was opened on, synchronizes that target to its own upstream first, and self-aborts to `*ErrMergeInRequired` on any conflict.
  `MergeContinue(msg)` concludes, `MergeAbort()` discards and restores both sides to pre-merge SHAs, `MergeInProgress()` is a read-only probe that reports `false` for foreign merge state.
  `MergeResult` carries `AlreadyUpToDate`, `Conflicts` (empty, never nil), `Committed`, plus an embedded `MutationRecord`.
  Typed errors live in `internal/fabricengine/mergeerrors.go`: `*ErrMergeInRequired`, `*ErrForeignMergeState`, `*ErrNoMergeInProgress`, `*ErrMergeIncomplete`, `*ErrUnmergeableState`, `*ErrMergeInProgress`, `*MergeGuardError`.
  Read `internal/fabricengine/doc.go`'s merge section before writing any of this — it states the lifecycle contract, the recorded-merge rationale, and the no-post-conclude-undo rule.
- **`internal/shedengine`'s producer contract** (`internal/shedengine/producer.go`).
  `Call(ctx) (Outcome, OutputPointer, error)` must return exactly `Done` or `Stuck`, and must surface context cancellation as a non-nil error, never as `Stuck`.
  `OutputPointer.Path == ""` means no artifact.
  `ProducerDef.OnStuck == ""` escalates; a non-empty value bounces to that producer name.
- **`internal/loomshed`** — the wiring site.
  `loomshed.go` holds the 13 name constants and `New(deps)`;
  `stub.go`'s `stubProducer` doc comment lists eight stubbed rows and must drop `Publish` and `Finalize` from that list.
  `ctx.go` holds `entryErr`/`cancelErr`, the shared cancellation helpers every producer in that package calls at entry — `landingshed` needs its own equivalents (`shedadapters/ctx.go` has the three-argument variant carrying an engine label).
  `internal/loomshed/seam_enforcement_test.go` (`TestToldGeometryInvariant_AllowlistOnly`) is the model for `landingshed`'s own import-policing test.
- **`internal/shedadapters/singlellm.go`** — the pattern to imitate but not reuse: the `Shuttle` one-method seam, `SpecSource`, and the outcome switch mapping `shuttleengine.OutcomeDone`/`Asking`/`Died`/`Timeout` onto the `ShedProducer` contract.
  `mergeresolve` needs the same seam shape and a similar outcome switch, without the `OutputFiles` machinery.
- **`internal/loomengine/discussion.go`'s `DiscussionSpec`** — the exact shape for building a `shuttleengine.Spec` from a config model-spec string: `modelspec.Parse(cfg.X)` → `reg.Resolve(spec)` → `Spec{Model: resolved.Model, Effort: resolved.Params["effort"], Version: resolved.Params["version"], …}`.
- **`internal/loomengine/config.go`'s `LoadConfig` + `configtemplate.go` + `template.yaml`** — the exact shape for `landingshed`'s config: `configengine.Load(baseDir, module, []byte(ConfigTemplate()))`, the `"not initialized"` rewrap, `yaml.Unmarshal`, then per-key `modelspec.Parse` validation.
- **`internal/websterengine/summary.go`** — `SummaryPath(websterDir)` and `ParseSummary(path) (*Summary, error)`, returning `Summary{Title, Body}`.
  `ParseSummary` fails loud on a missing, empty, or non-`# <title>`-headed file.
- **`internal/githubclient`** — `New() (*github.Client, error)` is the only legal way to obtain an authenticated client.
  `internal/selfreportengine/selfreport.go` is the reference consumer: a package-level `var NewGitHubClient = githubclient.New` test seam, a bounded `context.WithTimeout` around the API call, and three-way error discrimination (`errors.Is(err, githubclient.ErrTokenUnresolvable)` / `errors.As(err, &ghErr)` for `*github.ErrorResponse` / everything else).
  `Publish` needs `client.PullRequests.List` (or `Search`) plus `client.PullRequests.Create`.

### Gotchas found during exploration

- `manifest/designs/landing.md`'s status banner and its lines 3 and 58 claim the merge-conflict primitive "does not exist as code yet".
  That is stale — it landed in `a2bf44e2`.
  The roadmap's own item 1 already says "Depends on the Done `fabric: merge-conflict primitive` item below — unblocked".
- `landing.md` line 63 says `Finalize`'s merge-back forwards only Raddle's output "via a Fabric commit scoped to `["_lyx"]`".
  With Raddle unbuilt there is nothing to forward and no such commit to make;
  the ordinary `Merge` call carries whatever the branch holds.
  This sentence goes away with the file.
- GitHub sees only the warp repo.
  The weft branch is in no PR, ever.
  Any reasoning about "the PR merged, so we're done" is wrong for the pair as a whole.
- `Fabric.Merge` synchronizes the target to its own upstream before merging, which is what makes the post-GitHub-merge `AlreadyUpToDate` warp result correct rather than a sign of a missed pull.
- `MergeInProgress()` reports `false` for foreign merge state by design.
  The crash-recovery probe therefore cannot detect a human's own in-flight git merge;
  the typed `*ErrForeignMergeState` from the mutating call is what surfaces it.
- `MergeContinue`'s conflict gate is an **index** probe, not a content probe.
  `gitrepo.ConflictedFiles()` runs `git diff --name-only --diff-filter=U`;
  a file whose conflict markers were edited away still has an unmerged index entry until something stages it.
  This is why `MergeStageResolved` has to exist.
- `fabricengine.Open(l *lyxcwd.Location)` is the only exported constructor and it stat-checks the paired layout, which is exactly why `internal/perchcli` keeps its opener as a lazy closure (`wiring.go:138-140`) rather than opening eagerly at wiring time.
  Read that comment before designing the `Deps` closures.
- `internal/gitrepo` has no remote-URL reader today, and `internal/selfreportengine` sidesteps the question with a hardcoded `targetRepo` constant.
  Both new helpers (`RemoteURL`, `ParseOwnerRepo`) are genuinely new code.
- `internal/shedadapters/archive.go`'s `archiveStaleOutputs` archives every path in `spec.OutputFiles` before a run.
  This is a concrete reason conflicted paths must never be passed as `OutputFiles`.
- `loomshed.Deps` has no base-directory field on purpose (`AnchorPath` feeds both `planparser.PlanDir` and `batcher.Active`), and its doc comment explains why two fields would invite divergence.
  Follow the same discipline in `landingshed.Deps`: no derived paths, no redundant near-duplicates.

## Constraints

From `CONSTRAINTS.md`, in the order they bind this work:

- **Told-Geometry Invariant.**
  Both new packages take absolute paths from their caller and must have no direct production import of `internal/lyxcwd`.
  `landingshed` must be machine-enforced via its own `seam_enforcement_test.go`, matching `internal/loomshed`'s `TestToldGeometryInvariant_AllowlistOnly`.
  `mergeresolve` likewise.
  Add both to the invariant's machine-enforced list in `CONSTRAINTS.md` in the same commit.
- **Shed Producer-Seam Invariant.**
  `internal/shedengine`'s import allowlist is stdlib + `internal/state` + `internal/lock`.
  Nothing in this task may add an import to that package.
- **Fabric Git Invariant.**
  Every *mutating* git operation on either repo goes through `internal/fabricengine`, in-process, never raw git and never an LLM agent.
  Read-only verbs (current SHA, `git status --porcelain`, reading a remote URL) are exempt — that exemption is what makes `Publish`'s `origin`-URL read legal.
  The conflict-resolution agent edits files in the worktree; it must never be instructed to run git.
- **Fabric Vocabulary Invariant.**
  Neither `mergeresolve` nor either producer is in the owner set, so none of them may name the two fabric-internal sides — in an identifier, a string literal, or a comment.
  This is machine-enforced by `internal/lyxcwd/enforcement_test.go`'s AST walk over every `*ast.Ident`, so it constrains which `fabricengine` functions these packages may call **by name**, not merely what they say.
  See `landing-never-names-fabric-vocabulary` for the resulting `Deps` shape.
  The conflict stencil describes one repo, per `templates-describe-one-repo`.
- **Stencil Ownership Invariant.**
  The conflict prompt is a file under `contracts/stencils/`, resolved from the told stencils directory — never a Go string literal.
- **Config Strictness Invariant.**
  `landingshed` adopts `configengine.Load` (strict) and must be added to the strict pinned set;
  `cmd/lyx/configstrictness_test.go`'s `TestConfigStrictness_PinnedCallSiteSets` will fail otherwise.
- **GitHub Auth Invariant.**
  All GitHub auth goes through `internal/githubclient`.
  No package outside it may shell out to `gh` — `cmd/lyx/ghguard_test.go` enforces this.
- **Mutation Record Invariant.**
  `MergeResult` embeds `MutationRecord`;
  anything surfacing a merge result must carry the record through rather than dropping it.
  The new `MergeStageResolved` verb mutates the index, so its `StageResult` must embed the record too, and its new `Kind` member lands with its recording site and `cmd/lyx/destructiveguard_test.go` entry in one commit.
- **Durable-vs-Ephemeral State Invariant.**
  The conflict session's resolution report lives under `.lyx/`, never `_lyx/` — machine-local, never tracked, never in a weft-commit pathspec.
- **gitrepo Client Boundary Invariant.**
  The new staging method reaches the git CLI and must be added to that invariant's pinned method list in the same commit — `cmd/lyx/gitrepoboundary_test.go` is set-equality, so an omission fails the build.
  `RemoteURL` is go-git-backed and must **not** be added there.
- **gitexec Checked-Call Invariant.**
  The staging method uses the checked form, so the per-package pinned raw-site counts stay unchanged and it carries no `//gitexec:raw` marker.
- **Markdown Link Integrity.**
  Deleting `manifest/designs/landing.md` breaks thirteen inbound references across seven files — eleven Markdown links plus two prose references (one in a Go comment, one in `CONSTRAINTS.md` itself), neither of which any test catches.
  The full inventory is in the `docs-lifecycle-landing-md-deletes` decision above, including the one anchored link (`raddle.md`) and the one non-Markdown prose reference (`internal/loomshed/loomshed.go:19`) the link checker cannot see.
  All must be repointed in the same commit.
- **Documentation Lifecycle.**
  Durable design content folds into package docs;
  the design doc deletes;
  `docs/overview.md` and `manifest/roadmap.md` update in the same commit.
- **CLI / Cobra Invariant.**
  Not engaged — this task adds no command.
  If a `lyx` verb is added despite the scope above, it needs a `Short` and a help-tree test entry.
- **Test Tier Purity / Hermetic Git Test Environment Invariants.**
  Integration tests using `hubforge` fixtures must follow the existing hermetic-git discipline;
  see `internal/fabricengine/mergein_integration_test.go` and `merge_target_integration_test.go` for the established patterns.
- **Never Force-Add Invariant.**
  No `git add -f` anywhere in this work.

## Testing

Three tiers, no test ever contacting a real model or GitHub.

**`internal/mergeresolve` — unit, against two fakes.**
Fake the Fabric merge surface behind a narrow interface (`MergeIn`, `MergeStageResolved`, `MergeContinue`, `MergeAbort`, `MergeInProgress`) and fake the shuttle seam.
TDD candidates — write the tests first, the whole package's behaviour is decision-table shaped:

- clean merge → no LLM call at all, resolved;
- conflict → session spawned, markers gone on re-read → `MergeStageResolved` called with exactly the conflicted paths, **then** `MergeContinue`, resolved;
- ordering assertion: `MergeContinue` is never called before `MergeStageResolved` — the whole point of the new verb;
- conflict → session returns, markers still present → exactly one retry, then `MergeAbort` and stuck;
- `MergeInProgress` true at entry → `MergeAbort` called before any new attempt;
- `*ErrForeignMergeState` → stuck, and `MergeAbort` never called;
- `*ErrUnmergeableState` from `MergeIn` (an unmappable conflict path) → stuck with the error surfaced, and `MergeAbort` never called, since `MergeIn` already self-aborted;
- an unrecognised typed merge error → the same catch-all stuck path, proving the default is escalate rather than fall-through;
- a conflicted path that no longer exists after the session → treated as resolved-by-deletion, marker scan skipped, still passed to `MergeStageResolved`;
- a resolved file whose legitimate content carries line-start conflict markers → refused, escalating to stuck rather than concluding;
- a read error other than not-exist on a conflicted path → a genuine failure, distinct from the deletion case;
- shuttle outcome `Asking` / `Died` / `Timeout` → each mapped correctly, `MergeContinue` never called;
- context cancellation surfaced as a non-nil error, never as a stuck verdict;
- `AlreadyUpToDate` → resolved with no session and no `MergeContinue`.

**`internal/landingshed` — unit, against a faked `mergeresolve` and a faked GitHub client** (`var NewGitHubClient` seam, exactly as `internal/selfreportengine/selfreport_test.go` does it).
TDD candidates:

- `Publish` with parent not in `require_pr_to_base` → `Done`, no merge-in, no GitHub call whatsoever;
- `Publish` with parent in the list, no existing PR → merge-in runs, PR created with title and body byte-identical to `ParseSummary`'s output, verdict `Stuck`;
- `Publish` re-called, PR open → `Stuck`, and `PullRequests.Create` never called;
- `Publish` re-called, PR closed + merged → `Done`;
- `Publish` re-called, PR closed + not merged → `Stuck`, with a message distinguishable from the open case;
- `Publish` with an unresolvable token → the `githubclient.ErrTokenUnresolvable` path surfaces distinctly from a network failure;
- `Publish` with `origin` absent, unparseable, or non-GitHub → `Stuck` with a distinct message, and no PR attempted;
- `Publish` pushes the warp branch before querying for an existing PR, and before creating one — assert the ordering, not just that the push happened;
- `Publish` when the push fails → `Stuck`, and `PullRequests.Create` never called;
- `Publish` on `gitrepo.ErrPushRejected` → `Stuck`, with its own reason-file text, distinct from a generic push failure;
- `Publish` with `PushSkipped` set and a PR required → `Stuck` before the push closure or any GitHub call, never a PR for an unpushed branch;
- `Publish` with `PushSkipped` set and **no** PR required → plain `Done`, since that branch never reaches the push;
- an AST-level assertion, or the existing repo-wide vocabulary test, covering that neither new package names a fabric-internal side — this is a build-breaking constraint, not a stylistic one;
- `ScratchDir` absent on disk → the first write creates it rather than failing;
- every `Stuck` case writes its reason file, and two different `Stuck` causes produce different file contents — this is what "distinguishable" is asserted against, since `Run` persists the same fixed reason for all of them;
- `Publish` on a re-run with an open PR → the push still runs, so later commits reach the PR;
- `Publish` never pushes the weft side;
- `ParseOwnerRepo` as a table test: SSH form, HTTPS form, with and without `.git`, with a trailing slash, a non-GitHub host, and garbage input;
- `Publish` with a missing or malformed `summary.md` → fails loud, no PR created;
- `Finalize` → merge-in always runs, then the parent-side `Merge`, with `Squash` threaded from config;
- `Finalize` with `*ErrMergeInRequired` on the parent-side `Merge` → exactly one `mergeresolve` re-run and one retry, then `Stuck`;
- `Finalize` when `OpenParentFabric` errors (no live pair for the parent branch) → `Stuck` naming the branch, and no worktree created;
- `Finalize` when the parent-side `Merge` returns `*MergeGuardError` with `worktree dirty` → `Stuck` surfacing that reason, and no stash, reset, or retry;
- `mergeresolve`'s second attempt uses a distinct report path from the first, so `Runner.Start`'s already-exists rejection cannot fire on the retry;
- both producers: cancellation at entry surfaces as an error, never `Stuck`;
- `landing.yaml` loading: absent config errors (strict);
  a malformed `conflict` model-spec is rejected at load time, not at first use.

**Integration — one `hubforge`-backed test per producer**, driving real two-worktree fabric pairs with only the LLM seam faked.

- `Finalize`: build a parent pair and a task pair, create a genuine conflicting change on both, run `Finalize` with a fake session that writes real resolutions, and assert the parent pair actually carries the task's content on both sides afterward, that the squash flag took effect, and that no merge record is left behind.
- `Publish`: the merge-in half against real pairs with a faked GitHub client, asserting the pair is left clean and current before the PR call is made.

This tier is worth its cost.
The merge-conflict primitive's own review round surfaced 19 findings, several about exactly this kind of two-worktree interaction — the unit tiers above would not have caught them.

**Not tested:** driving both producers through a real `shedengine.Run` over a short producer list.
That re-tests `shedengine` itself, which already has its own resume, crash-recovery, and pause suites.

**`internal/fabricengine` — the new `MergeStageResolved` verb.**
Integration-tested alongside the existing merge suites (`mergein_integration_test.go` is the model): a real conflicted pair, resolve the files on disk, call `MergeStageResolved`, assert both sides' `ConflictedFiles()` are now empty and a subsequent `MergeContinue` succeeds.
Also: a path listed in neither side's `ConflictedFiles()` is an error, not a silent skip;
a delete/modify conflict resolved by deletion stages successfully rather than erroring on the missing file;
an empty `paths` slice is a no-op, not an error;
and `StageResult`'s `MutationRecord` is populated on a real staging call and left empty on the no-op, per the Mutation Record Invariant's "never on a no-op" rule.

**Enforcement tests to add or update** (not behaviour tests, but required by the constraints above):

- `internal/landingshed/seam_enforcement_test.go` and the `mergeresolve` equivalent — production import sets exclude `internal/lyxcwd`;
- `internal/loomshed/seam_enforcement_test.go`'s `loomshedAllowedImports` gains `internal/landingshed`;
- `cmd/lyx/configstrictness_test.go`'s pinned set gains `landingshed` on the strict side;
- `cmd/lyx/gitrepoboundary_test.go`'s pinned method list gains the new staging method (and must *not* gain `RemoteURL`);
- `internal/configreg/configreg_test.go`'s `want` list gains `"landing"`;
- `contracts/stencils/registry_test.go` passes only once the new stencil is both embedded and registered.

## Q&A log

- **Q:** Where do the two producers and `mergeresolve` live? **A:** `internal/mergeresolve` + `internal/landingshed`, mirroring the `loomengine`/`loomshed` split. `shedadapters` rejected — it holds thin seams over generic engines with no domain logic, and `loomshed`'s `doc.go` forbids touching it.
- **Q:** Build the discrepancy-document conflict shape? **A:** No. Only the git-conflict shape shipped; the document shape stays a parked Someday item, and the docs get corrected so they stop promising it.
- **Q:** What does `Finalize` do about Raddle, which doesn't exist? **A:** Nothing — no hook, no nil-injected interface, no scaffolded lock span. The Raddle task owns that wiring when it lands.
- **Q:** Drive the conflict session through `SingleLLMProducer`? **A:** No — its `OutputFiles` contract doesn't fit in-place edits of existing conflicted files, and `archiveStaleOutputs` would archive the very files needing resolution. `mergeresolve` gets its own narrow shuttle seam.
- **Q:** Who decides a conflict is actually resolved? **A:** `mergeresolve`, mechanically — re-scan for markers, one retry, then `MergeAbort` + stuck. `MergeContinue` is irreversible at the Fabric layer, so `MergeAbort` is the only checkpoint that covers the attempt window.
- **Q:** What is `Finalize`'s "`_lyx` teardown"? **A:** Nothing — it was loose wording, names nothing in the codebase, and `lyx fabric cleanup` already owns worktree/branch/junction/portal teardown. Worktree and branch deliberately survive the merge so a human can inspect the finished task.
- **Q:** How does the final merge to parent run? **A:** Told the parent worktree path, open a second `Fabric` handle, call `parentFabric.Merge(taskBranch, opts)`. One retry on `*ErrMergeInRequired`, then `Stuck`.
- **Q:** Why one retry rather than zero? **A:** No lock spans the window between `Finalize`'s own catch-up merge-in and the later parent-side `Merge`, so a competing task can genuinely land in the parent in between. Real drift, not a buggy impossibility.
- **Q:** Squash or ordinary merge? **A:** Squash, `landing.yaml` key defaulting to `true`. One commit per Card makes an ordinary merge noisy; the ancestry-link loss is documented in `fabricengine`'s own docs and accepted.
- **Q:** How does `Publish` resolve owner/repo? **A:** From the warp repo's `origin` remote URL — a read-only git query, exempt from the Fabric Git Invariant's mutation rule. A hardcoded constant is wrong; `Publish` runs against whatever repo the hub was cloned from.
- **Q:** `Publish` idempotency after crash-recovery? **A:** Query GitHub for `head:<taskBranch> base:<parent>` first. GitHub is authoritative — a locally-tracked PR number would be a second source of truth that can go stale.
- **Q:** `landing.yaml` strict or degrading? **A:** Strict (`configengine.Load`). Neither producer has a standalone entry point, unlike shuttle/reed/perch/webster; inside a hub an absent config means a broken hub.
- **Q:** How do the producers get their told values? **A:** `landingshed.Deps` + `NewPublish`/`NewFinalize`, with `loomshed.Deps` growing a single `Landing landingshed.Deps` passthrough. Pre-constructed injection like `Preflight` is the wrong precedent — `Preflight` is special-cased because it spawns git directly.
- **Q:** `OnStuck` for rows 12 and 13? **A:** Both `""`. Nothing in the list produces what these two gate. Bouncing to `Webster` is actively wrong: a conflict against the parent's current state is not a defect in Webster's diff.
- **Q:** New stencil or reuse? **A:** New — `contracts/stencils/landing/landing-template-conflict.md`. A Go string literal violates the Stencil Ownership Invariant; no existing stencil is written for conflict resolution.
- **Q:** Crash mid-merge — resume or abort? **A:** `MergeAbort` unconditionally, then start clean. A half-resolved worktree from a killed session is not a state to build on.
- **Q:** `*ErrForeignMergeState`? **A:** Never touched. Fabric's own merge verbs already refuse it; `mergeresolve` inherits that refusal rather than deciding a human's in-flight merge is disposable.
- **Q:** With `require_pr_to_base` set, doesn't `Shed` merge seconds after opening the PR? **A:** It would have, as written — a real gap. `Publish` returns `Stuck` (not `Done`) while a PR is open, so `Shed` persists `blocked` and the run ends there until a human resumes it.
- **Q:** How does a resumed `Publish` tell "still open" from "done"? **A:** By the PR's `state`/`merged` fields. Open → `Stuck` again; closed+merged → `Done`; closed+unmerged → `Stuck` with a distinct message. Review-approval state rejected: it leaves the PR open, needs an explicit close call, and races against post-approval pushes.
- **Q:** Doesn't a merged PR make `Finalize` redundant? **A:** No. GitHub only sees the warp repo, so the weft branch was never in the PR. After a GitHub-side merge, `Finalize`'s `Merge` finds the warp side `AlreadyUpToDate` and genuinely merges the weft side.
- **Q:** Is `require_pr_to_base` a bool? **A:** No — a list of base branches, defaulting to `[main]`. A bool would force a PR on every task-to-task merge in a stacked workflow, which this task's own parent (`standalone-producers`, not `main`) proves is wrong. A bool flipped per run in a profile file can't work either: the parent branch is a per-task runtime fact a static profile cannot know.
- **Q:** Keep `manifest/designs/landing.md`? **A:** No — it deletes in the final commit per its own status banner and the Documentation Lifecycle, with durable content folded into the two packages' `doc.go` files.
- **Q:** (review round 1) Nothing stages the resolved conflicts, so `MergeContinue`'s index guard can never pass. Who stages? **A:** A new narrow `fabricengine.MergeStageResolved(paths)` verb. Index manipulation is a Fabric primitive's job; `mergeresolve` has no git access and is bound by the Fabric Git Invariant regardless. `MergeContinue`'s guard stays untouched as a second, authoritative check — defence in depth rather than a relocated guard. Modifying the shipped `MergeContinue` was rejected as too risky for an already-reviewed primitive, and it would remove the guard a human relies on after their own `git add`.
- **Q:** (review round 1) How does either package get a `*Fabric` handle without importing `lyxcwd`? **A:** Injected opener closures in `landingshed.Deps`, filled by the CLI layer — `internal/perchcli`'s existing pattern, including its laziness (`Open` stat-checks the paired layout, so eager opening would fail before `Preflight` runs) and its nil-in-standalone form. A bare-path `OpenAt` constructor was rejected: `Open`'s `*lyxcwd.Location` is what lets `fabricengine` derive weft pairing and junction geometry, and a path-only constructor would duplicate or over-simplify that derivation.
- **Q:** (review round 1) `origin` → owner/repo has no implementation anywhere. Where does it live? **A:** `gitrepo.RemoteURL` via go-git (a local config read — no new `gitexec` call site, no Client Boundary Invariant list change) plus a stdlib `githubclient.ParseOwnerRepo`. Absent, unparseable, or non-GitHub `origin` → `Stuck`, never a silent no-PR `Done`.
- **Q:** (review round 2) The `OutputFiles` requirement lives in `Runner.Start`'s `validate`, not in `SingleLLMProducer` — so bypassing the adapter doesn't escape it. What does the conflict spec name? **A:** One fresh per-attempt resolution report at `.lyx/landing/conflict-resolution-r<attempt>.md`. Per-attempt because `validate` also rejects a pre-existing entry, which would fail the retry. It is never parsed for control flow — the marker scan stays the verification — it satisfies the runner contract and leaves an audit trail. Loosening `shuttleengine`'s guard for one caller was rejected. The decision to bypass `SingleLLMProducer` stands on a different footing: it is itself a `ShedProducer`, and `mergeresolve` is not a producer.
- **Q:** (review round 2) Where does the parent pair's `*Fabric` actually come from, given the parent is only a branch name? **A:** `fabricengine.List(sourceDir)` already returns `Branch`+`Path` per worktree; the CLI layer matches the parent branch, then `lyxcwd.ResolveWorktree` → `fabricengine.Open`. No live pair → `Stuck`, never auto-create a worktree. Parent dirty → `Merge`'s own `worktree dirty` guard fires and `Finalize` surfaces it as `Stuck`, never stashing or resetting someone else's uncommitted work.
- **Q:** (review round 2) Does `MergeStageResolved` need a mutation record? **A:** Yes — it mutates the index, so it returns `StageResult` embedding `MutationRecord`, with its new `Kind` member, recording site, and guard-table entry all landing in the same commit.
- **Q:** (review round 3) `.lyx/landing/…` as a relative `OutputFiles` entry resolves against `worktreeRoot`, not `AnchorPath` — wrong directory on an anchored hub. **A:** `ScratchDir` becomes a told absolute path in `landingshed.Deps`, resolved by the caller as `<AnchorPath>/.lyx/landing` exactly as `loomengine.LoomRunLock` does. No `_lyx/landing` twin: the report is ephemeral with no durable counterpart.
- **Q:** (review round 3) Who actually fills the two Fabric opener closures? **A:** Nobody in this task — `loomshed.New` has no production caller at all today. The chain is specified here and built by the next roadmap item, `loom: session bootstrap`. This task's own tests fill the closures directly against `hubforge` fixtures, and a nil required closure is a construction error, not a silent no-op.
- **Q:** (review round 3) What happens on a typed merge error the design doesn't name, like `*ErrUnmergeableState`? **A:** Catch-all `Stuck` with the error surfaced, and no `MergeAbort` — `MergeIn` already self-aborts on that one, and the guard errors refuse before mutating anything.
- **Q:** (review round 4) Who pushes the task branch? Without a push there is no `head:<taskBranch>` for GitHub to see. **A:** `Publish` does, after the merge-in and before the existing-PR query. Push failure → `Stuck`, no PR attempted. Only the GitHub-visible side is pushed. **Superseded in part by round 5:** this answer originally named `fabricengine.PushWarpAt`, which round 5 rejected for its rebase retry and push-lock residue — the push goes through the new rebase-free wrapper instead, and round 6 moved the call itself behind a `PushBranch` closure. Do not implement `PushWarpAt`.
- **Q:** (review round 4) `MergeStageResolved`'s "maps to neither side" error is unreachable, since `weftPathVisible` is total. **A:** The discriminator changes to index membership: stage a path on whichever side's `ConflictedFiles()` lists it, and error when neither does. Deletions stage via `git add -A -- <paths>` so a delete/modify resolution doesn't error on the missing file.
- **Q:** (review round 4) What does the marker scan do with a file that was correctly deleted, or one whose real content contains markers? **A:** Absent file = resolved-by-deletion, scan skipped, still staged; only a non-not-exist read error is a failure. The scan is line-anchored and content-only, and a file with legitimate line-start markers is refused into `Stuck` — the safe direction, since `MergeContinue` is irreversible. The stencil must not contain literal line-start markers.
- **Q:** (review round 3) `CONSTRAINTS.md:427` cites `landing.md` as a worked example. **A:** Added to the same-commit doc edits; the bullet gets rewritten against a surviving example. It is prose, so no test would catch it.
- **Q:** (review round 5) Where does a "`Stuck` with a distinct message" actually go? `Shed` persists one fixed reason string and never persists `OutputPointer`. **A:** Two carriers — a structured `logger.Warn` (the precedent `shedadapters/singlellm.go:104` already sets) and a reason file at `<ScratchDir>/<producer>-stuck.md`. The producer returns bare `Stuck`. Every "distinguishable" test asserts on the file, not on a returned reason. Extending the `ShedProducer` contract was rejected; returning an error was rejected because it flips `blocked` into `failed`.
- **Q:** (review round 5) `PushWarpAt` no-ops on `SkipGit`/`SkipPush` and its rebase retry rewrites warp SHAs while weft isn't rebased. **A:** Don't use it. A new `PushWarpRebaseFreeAt` routes to `gitrepo.PushRebaseFree`, which neither rebases nor writes the push-lock file — discharging both the correspondence-index hazard and the untracked-residue precondition, and leaving `PushWarpAt`'s "no production caller" doc comment true. `SyncOptions` is told in `Deps`, and `Publish` checks the skip flags itself rather than letting a skip silently produce a PR for an unpushed branch.
- **Q:** (review round 6) Can `landingshed` even name the `fabricengine` push verb? **A:** No — the Fabric Vocabulary Invariant's AST walk flags a forbidden token in any identifier, so the call expression alone fails the build. The push moves behind a `PushBranch` closure filled by the CLI layer, `Deps` drops its path field in favour of a told `OriginURL`, and both packages' docs describe one repo. The merge verbs are unaffected; their names are already vocabulary-free.
- **Q:** (review round 6) Who creates `ScratchDir`? **A:** Whoever writes into it, on every write path. Creating a told directory is legal under Told-Geometry — `shedengine` does exactly this for its lock parents — whereas deriving the path would not be.
- **Q:** (review round 5) `Deps` carrying both `AnchorPath` and `ScratchDir` is a derived near-duplicate. **A:** `AnchorPath` is dropped. `ScratchDir` is told, which is also mandatory rather than stylistic: deriving it would name the `.lyx` literal (Lyxdirs Single-Declarer) and compute geometry (Told-Geometry), both forbidden to these packages.


### From _mill/plan/00-overview.md


```yaml
task: 'landing: Publish + Finalize producers'
slug: landing-publish-finalize-producers
approved: true
started: '20260819-125210'
parent: 'standalone-producers'
root: ""
verify: go vet ./...
```

### From _mill/plan/01-merge-stage-resolved-verb.md


```yaml
task: 'landing: Publish + Finalize producers'
batch: 'merge-stage-resolved verb'
number: 1
cards: 6
verify: go test ./internal/gitrepo/... ./internal/fabricengine/... ./cmd/lyx/... && go test -tags integration ./internal/gitrepo/... ./internal/fabricengine/...
depends-on: []
```



- **Edits:**
  - `internal/gitrepo/merge.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `cmd/lyx/gitrepoboundary_test.go`
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/fabricengine/mutation.go`
- **Creates:** none
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/mergestage.go`
- **Deletes:** none
- **Edits:**
  - `cmd/lyx/destructiveguard_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/mergestage_integration_test.go`
- **Deletes:** none

### From _mill/plan/02-remote-and-push-helpers.md


```yaml
task: 'landing: Publish + Finalize producers'
batch: 'remote and push helpers'
number: 2
cards: 5
verify: go test ./internal/gitrepo/... ./internal/githubclient/... ./internal/fabricengine/... ./cmd/lyx/... && go test -tags integration ./internal/gitrepo/... ./internal/fabricengine/...
depends-on: []
```



- **Edits:** none
- **Creates:**
  - `internal/gitrepo/remote.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/gitrepo/remote_integration_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/githubclient/parseownerrepo.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/githubclient/parseownerrepo_test.go`
- **Deletes:** none
- **Edits:**
  - `internal/fabricengine/spawn.go`
- **Creates:**
  - `internal/fabricengine/pushrebasefree_integration_test.go`
- **Deletes:** none

### From _mill/plan/03-mergeresolve-engine.md


```yaml
task: 'landing: Publish + Finalize producers'
batch: 'mergeresolve engine'
number: 3
cards: 8
verify: go test ./internal/mergeresolve/... ./contracts/stencils/... ./internal/lyxcwd/...
depends-on: [1]
```



- **Edits:** none
- **Creates:**
  - `internal/mergeresolve/doc.go`
  - `internal/mergeresolve/deps.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/mergeresolve/markers.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/mergeresolve/spec.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/mergeresolve/mergeresolve.go`
  - `internal/mergeresolve/ctx.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/mergeresolve/seam_enforcement_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/mergeresolve/mergeresolve_test.go`
  - `internal/mergeresolve/markers_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `contracts/stencils/landing/landing-template-conflict.md`
- **Deletes:** none
- **Edits:**
  - `contracts/stencils/stencils.go`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/04-landingshed-producers.md


```yaml
task: 'landing: Publish + Finalize producers'
batch: 'landingshed producers'
number: 4
cards: 11
verify: go test ./internal/landingshed/... ./internal/configreg/... ./internal/fabricengine/... ./cmd/lyx/...
depends-on: [2, 3]
```



- **Edits:** none
- **Creates:**
  - `internal/landingshed/doc.go`
  - `internal/landingshed/deps.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/landingshed/config.go`
  - `internal/landingshed/configtemplate.go`
  - `internal/landingshed/template.yaml`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/landingshed/ctx.go`
  - `internal/landingshed/stuck.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/landingshed/publish.go`
- **Deletes:** none
- **Edits:**
  - `internal/fabricengine/mergeerrors.go`
- **Creates:**
  - `internal/landingshed/finalize.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/landingshed/seam_enforcement_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/landingshed/publish_test.go`
- **Deletes:** none
- **Edits:**
  - `internal/fabricengine/mergeerrors_test.go`
- **Creates:**
  - `internal/landingshed/finalize_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/landingshed/config_test.go`
- **Deletes:** none
- **Edits:**
  - `internal/configreg/configreg.go`
  - `internal/configreg/configreg_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `cmd/lyx/configstrictness_test.go`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/05-loomshed-wiring-and-integration.md


```yaml
task: 'landing: Publish + Finalize producers'
batch: 'loomshed wiring and integration'
number: 5
cards: 5
verify: go test ./internal/loomshed/... ./internal/landingshed/... ./cmd/lyx/... && go test -tags integration ./internal/loomshed/... ./internal/landingshed/...
depends-on: [4]
```



- **Edits:**
  - `internal/loomshed/loomshed.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/loomshed/seam_enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/loomshed/stub.go`
  - `internal/loomshed/stub_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/loomshed/loomshed_test.go`
  - `internal/loomshed/fixture_test.go`
  - `internal/loomshed/sequence_test.go`
  - `internal/loomshed/resume_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/landingshed/testmain_integration_test.go`
  - `internal/landingshed/finalize_integration_test.go`
  - `internal/landingshed/publish_integration_test.go`
- **Deletes:** none

### From _mill/plan/06-documentation-lifecycle.md


```yaml
task: 'landing: Publish + Finalize producers'
batch: 'documentation lifecycle'
number: 6
cards: 6
verify: go test ./internal/lyxcwd/... ./cmd/lyx/... ./internal/landingshed/... ./internal/mergeresolve/...
depends-on: [5]
```



- **Edits:**
  - `internal/mergeresolve/doc.go`
  - `internal/landingshed/doc.go`
- **Creates:** none
- **Deletes:** none
- **Edits:** none
- **Creates:** none
- **Deletes:**
  - `manifest/designs/landing.md`
- **Edits:**
  - `manifest/designs/loom.md`
  - `manifest/designs/shed.md`
  - `manifest/designs/raddle.md`
  - `manifest/designs/fabric-unified-view.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none

## Conflicting files

- `internal/loomshed/loomshed.go`
- `internal/loomshed/sequence_test.go`
- `internal/loomshed/stub.go`
- `manifest/designs/loom.md`
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
5. Run `git -C /home/knatte/Code/loomyard/wts/landing-publish-finalize-producers add <file>` to stage the resolved file.
6. For modify/delete (DU) conflicts: if Task intent above lists this file under a batch's `Deletes:`, run `git -C /home/knatte/Code/loomyard/wts/landing-publish-finalize-producers rm <file>` instead of editing;
   that stages the intentional deletion.
7. For UD conflicts — files this branch **modified** that the parent branch **deleted**: do not silently keep the modification.
   Instead: a. Run `git log --diff-filter=D --oneline MERGE_HEAD -- <file>` to find the deletion commit on the parent. b. Run `git show <deletion-commit>` to inspect context. c. If the deletion commit message mentions a replacement file (e.g. "replaced by", "moved to", "consolidated into"),
   or the commit also adds a file in the same directory with overlapping content: stage the deletion — `git -C /home/knatte/Code/loomyard/wts/landing-publish-finalize-producers rm <file>`. d. If detection is inconclusive: report `{"status":"stuck","stuck_type":"logic","reason":"modify/delete conflict on <file>: cannot determine if parent deletion is a replacement -- operator must decide"}` and halt.
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
Use `git -C /home/knatte/Code/loomyard/wts/landing-publish-finalize-producers` for any git commands;
do not `cd`.
Worktree cwd is `/home/knatte/Code/loomyard/wts/landing-publish-finalize-producers`.
