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
- Decision, therefore: the conflict spec's `OutputFiles` names exactly one **fresh** artifact — a resolution report at `.lyx/landing/conflict-resolution-r<attempt>.md`, which the session writes as its terminal act.
  `.lyx` is machine-local and never committed, per the Durable-vs-Ephemeral State Invariant, so the report never reaches a weft-commit pathspec and never lands in the parent.
  The path is **per-attempt** (`r1`, `r2`), because `validate` rejects a pre-existing entry and the one-retry path would otherwise fail its second `Runner.Start` on the first attempt's own artifact.
  The report is not parsed for control flow — `mergeresolve`'s marker scan over `MergeResult.Conflicts` remains the verification, per `verify-before-conclude`.
  It exists to satisfy the runner contract and to leave a human an audit trail of what the session claims it resolved.
- Rejected: passing the conflicted paths as `OutputFiles` — they exist on disk by definition, so `validate` rejects them outright;
  changing `shuttleengine` to permit an empty `OutputFiles` (that would remove the fail-loud guard protecting every other shuttle consumer, for one caller's convenience).

### verify-before-conclude

- Decision: after the LLM session returns, `mergeresolve` verifies mechanically — it re-reads each path from `MergeResult.Conflicts` and checks for remaining conflict markers.
  Clean → stage the resolved paths via the new `MergeStageResolved` verb (see below), then `MergeContinue`.
  Still marked → one retry of the session, then on a second failure `MergeAbort` and report stuck.
- Rationale: `MergeContinue` is irreversible at the Fabric layer — `internal/fabricengine`'s package documentation states there is no post-conclude undo, and `MergeAbort` covers only the uncommitted attempt window.
  Verify-before-conclude with `MergeAbort` as the checkpoint is the discipline the merge primitive itself was designed around.
- Rejected: trusting the session's `Done` verdict and concluding unconditionally;
  leaving the pair mid-merge without aborting (that strands the worktree in a state the next run must clean up, and contradicts the abort-is-the-checkpoint decision).

### merge-stage-resolved-verb

- Decision: `internal/fabricengine` gains a narrow verb, `MergeStageResolved(paths []string) error`.
  It takes unified, worktree-relative paths, routes each to the side that owns it — the inverse of `mergepaths.go`'s `unifyConflictPaths` — and stages it.
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
  A new `Kind` member for the staging primitive lands in `internal/fabricengine/mutation.go` in the **same commit** as its recording site and its `cmd/lyx/destructiveguard_test.go` guard-table entry — never ahead of either.
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

- Decision: `landingshed.Deps` carries every told value, with `NewPublish(deps)` / `NewFinalize(deps)` constructors.
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
  Neither `mergeresolve` nor either producer is in the owner set, so none of them may name warp or weft.
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
  Deleting `manifest/designs/landing.md` breaks twelve inbound references across six files — eleven Markdown links plus one prose reference in a Go comment.
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
Also: a path that maps to neither side is an error, not a silent skip;
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
