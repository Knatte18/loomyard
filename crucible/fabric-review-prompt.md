# `fabric` — independent review + fix (round agent prompt)

> Instantiated from [`review-prompt-template.md`](review-prompt-template.md) for the `fabric`
> module. See [README.md](README.md) for the loop this prompt runs inside.

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `fabric`
module in the loomyard repo, followed by FIXING what you find. Work in the worktree at
`/home/knatte/Code/loomyard/wts/fabric` (branch `fabric`). Adjust that path/branch if the task
lives elsewhere now.

**Hub geometry note:** `fabric` does NOT share `warp`/`weft`'s hub. Any live driving you do
against a real hub MUST use the two dedicated GitHub repos `Knatte18/lyx-fabric-test` (host) and
`Knatte18/lyx-fabric-test-weft` (weft) — never the shared `Knatte18/lyx-test` /
`Knatte18/lyx-test-weft` repos warp/weft use. `lyx fabric clone` materializes this dedicated hub;
see `SANDBOX-FABRIC-SUITE.md`'s Pre-conditions for the exact clone/reuse semantics
(`lyx-fabric-test-HUB` is reused idempotently if it already exists on this machine, never reset).

## Your two jobs, in order
1. REVIEW: form your own independent judgment of `fabric`'s scope and correctness. Hunt for bugs
   by reading the code AND by driving the real substrate (real git repos/worktrees over real
   filesystem junctions/symlinks — no mocked git) — this is where the defects hide.
2. FIX: after you have a findings list, implement the fixes one at a time, verify each against the
   real substrate, keep the whole test suite green, and update the docs in the same change as the
   fix they document. COMMIT after each individual fix lands green (see "Commit per fix" below). Do
   NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green (`go build`/`vet`/hermetic test, plus the live
smoke/suite check if the finding needed one), and its doc update (if any) is included, COMMIT it —
on the current branch, no push — before starting the next finding. Commit message format:
`fabric: fix <finding-id> — <one-line what/why>` (e.g. `fabric: fix F3 — reconcile drift repair
double-counts orphaned weft branches`). Do not commit `.scratch/` (gitignored; your review and
fixer reports never belong in a commit regardless).

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to
`.scratch/fabric-review-<yourtag>.md` on disk — before you touch (edit, create, or delete) a
single production or test file. Do not fix findings as you go, even ones that look small and
obviously right. Write it down as a finding, keep reading, finish the review, save the file, THEN
start Job 2.

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first. Do NOT read any prior review or review-dialogue files before you have
your own list. Specifically do not open anything under `.scratch/` (gitignored; holds prior reviews
`fabric-review-*.md` and `*-fixer-report.md`). Reading the design SPEC and the module docs is
expected and required (those are not reviews). AFTER you have written your own independent findings,
you MAY consult the prior rounds' `.scratch/fabric-review-*` material — regardless of which model
produced it (rounds rotate across Opus / Fable / Sonnet; the most recent prior round is whichever
`fabric-review-*` file is newest), EXCEPT your own `-<yourtag>` deliverables — to (a) confirm
previously-fixed behaviors have not regressed and (b) re-evaluate the deferred items at the bottom.

## What to read
- Code: `internal/fabricengine/**`, `internal/fabriccli/**`, `internal/gitrepo/**` (fabric's git
  operator — `Repo`, `StageAndCommit`, `Push`, `CurrentSHA`, `ChangedFilesSince`, `SHAExists`,
  `SnapshotSHA`/`SetSnapshotSHA`), `internal/fslink/**` (cross-OS dir-link primitive fabric's
  junction/symlink wiring rides on), and the `cmd/lyx` integration wiring the `fabric` command tree
  into the root.
- Docs: `manifest/designs/fabric.md`, `docs/overview.md`, `manifest/roadmap.md`, `CONSTRAINTS.md`,
  `README.md`.
- Scenario ideas (not a review): `tools/sandbox/SANDBOX-FABRIC-SUITE.md`. You run every scenario
  yourself, directly, with your own tool calls; you do NOT invoke its `sandbox-fabric-suite.cmd`
  launcher (that spawns a SEPARATE, context-free interactive `claude` session for a human
  operator's own dogfooding — meaningless for you to spawn on top of yourself).
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md`
  (Hub Geometry, CLI/Cobra, lyxtest Leaf, Sandbox Suite Coverage, Documentation Lifecycle). A change
  that ships behaviour without updating the module doc / invariants in the SAME change is incomplete.
- Design intent (SPEC, not a review): the task's `_mill/discussion.md` and `_mill/plan/*` were
  deleted from the working tree by mill-finalize's pre-merge cleanup once the task reached
  `pr-pending`. Recover them from git history at sha `448f0a54` (the last commit before cleanup):
  `git show 448f0a54:_mill/discussion.md` and `git show 448f0a54:_mill/plan/00-overview.md` (plus
  `git show 448f0a54:_mill/plan/<NN-batchname>.md` for the six per-batch cards — list them via
  `git ls-tree -r --name-only 448f0a54 -- _mill/plan/`). Use this as the authoritative source of
  intended v1 scope/behavior — fabric unifies `warp` + `weft` into one git-coordination module
  (host+weft worktree pairs on a fixed `<branch>-weft` suffix scheme, unlike warp's mirrored
  branch names).

## Mission (assess on two axes, be adversarial)
1. Scope / omfang — is the module's scope right? Does the as-built code deliver what the design
   (recovered plan/discussion) intended? Gaps, over-reach, silently-dropped requirements,
   deferred-that-should-ship-in-v1.
2. Correctness — bugs, races, error handling, edge cases; concentrate on the historically-fragile
   areas below. Also assess docs accuracy (do the docs match the code?) and operability.

## High-yield focus — where `fabric`'s real bugs live (drive these, do not just read them)
The pure/unit-tested parts are usually solid; defects concentrate in the COMPOSED, LIVE behavior the
hermetic tests never exercise. Treat each as an INVARIANT you must actively verify by driving the
real substrate — a green `go test` proves nothing here.
- **Pair topology invariants** — `lyx fabric add` must always produce a weft branch named exactly
  `<host-branch>-weft` (the fixed suffix scheme), never a mirrored name like `warp clone`'s. Verify
  this holds for an empty branch prefix, a slash-containing slug, and a slug that collides with an
  existing branch. Confirm `lyx fabric pairs` reports sync state honestly after manual drift (edit
  one side out from under fabric with plain git, then check `pairs`/`reconcile` notice it — do not
  trust a green in-process report without independently checking the actual git refs yourself).
- **Reconcile / prune / cleanup drift repair** — remove the host side of a pair by hand (not via
  `lyx fabric remove`), then run `reconcile`. Does it detect and repair the orphaned `<slug>-weft`
  branch as fabric-managed (by suffix), or does it mis-attribute it to warp/weft? Run `prune`/
  `cleanup` dry-run first, then `--apply` — confirm dry-run and apply agree, and that apply is
  idempotent (running it twice does nothing destructive the second time). Chase double-counting:
  does reconcile ever repair the same drift twice, or leave the repo in a worse state than before?
- **Junction/symlink wiring under fslink** — `lyx fabric checkout` re-points a directory
  junction/symlink to switch pairs together. Kill/interrupt mid-checkout (Ctrl-C or process kill
  between the two sides re-pointing) and confirm the module either recovers cleanly on the next
  invocation or reports an honest, actionable error — never a silently half-switched pair. Confirm
  the cross-OS contract: directory-only links (`fslink.CreateDirLink`), no reliance on Windows file
  symlinks anywhere in fabric's own wiring.
- **Weft content sync honesty** — `fabric commit` always uses the fixed message `"weft sync"` (no
  `-m` override — confirm via `--help`) and every weft commit carries a trailing `Warp-SHA: <sha>`
  trailer naming the paired host's current HEAD. Verify the trailer names a real, resolvable commit
  after several sync cycles, including one where the host side advanced between two `fabric sync`
  calls. `fabric sync` pushes via a **detached child process** — verify `fabric status` immediately
  after `sync` can legitimately lag the actual push (expected async gap) but never *permanently*
  diverges — poll until it catches up and confirm it does, don't just assert the lag once and move
  on.

## Explicitly OUT of scope for `fabric` v1
`fabric` is a **parallel-build** module: it exists alongside `warp`/`weft`, is not yet the default,
and is not expected to touch or migrate any existing `warp`/`weft` sandbox state, hub, or config.
Do not flag the absence of a cutover/migration path from warp+weft to fabric, or the absence of
fabric being wired as the default — those are explicitly future work, not v1 gaps. Do not flag
fabric's dedicated sandbox hub (`lyx-fabric-test-HUB`) being separate from the shared
`lyx-test`/`lyx-test-weft` hub — that separation is intentional per `SANDBOX-FABRIC-SUITE.md`.

## Round context seeded from prior-round verification
**Safety pass — round 5 (FINAL planned round).** Rounds 1-4 (tags `opus-r1`/Opus, `fable-r2`/Fable,
`opus-r3`/Opus, `fable-r4`/Fable — we are alternating Opus/Fable for 5 rounds total, so this round is
Opus) between them found and fixed one BLOCKING bug plus twenty-five lower-severity ones; the
orchestrator has independently verified every fix (see CLOSED-AND-VERIFIED below) — including
reproducing the not-false-green proof for the behavior-changing fixes by reverting each production
file to its pre-fix state, confirming the round's own new/updated test FAILS at the right assertion,
then restoring the fix and confirming an empty diff. There is **no known residual**. Do a genuinely
independent clean-room pass to find anything rounds 1-4 missed — or, if you genuinely find nothing,
honestly confirm merge-readiness. Do NOT re-open or re-litigate the CLOSED-AND-VERIFIED work below.
This is the fifth and (per the planned campaign) final round — after this round's fixes are
independently verified, if it finds nothing new the campaign is judged converged and ready to merge;
if it finds real defects, the operator will decide whether to extend beyond 5 rounds.

Round 2's fixer phase was interrupted mid-way by a session crash (R1-R3 fixed by the crashed
session, recovered and committed by the orchestrator; R4-R11 finished by a continuation round agent).
Both halves were independently verified by the orchestrator from a cold state on the committed tree,
same bar as any other round — the crash is operational history, not a review caveat.

Round 3's own review agent (originally spawned as `sonnet-r3`) was killed by the orchestrator before
it wrote anything or touched code — model rotation was corrected to Opus per the Opus/Fable
alternation and relaunched clean as `opus-r3`; no partial state from the killed attempt exists.

**CLOSED-AND-VERIFIED (do not re-litigate):**

Round 1 (`opus-r1`):
- **F1 (BLOCKING, commit `dea44b3e`):** `fabric cleanup --apply --force` deleted the primary weft
  branch `main-weft` because liveness was judged against host worktree *directory* names instead of
  branch names. Fixed by comparing live host *branches* instead. Orchestrator reproduced the bug by
  reverting `internal/fabricengine/cleanup.go` to its pre-fix state and confirmed
  `TestCleanup_DifferentialEquivalence/PrimaryBranchSurvivesForceWhenNotCheckedOut` fails at exactly
  the right assertions (branch reported as orphan + branch deleted); restoring the fix passes it
  again with an empty diff against the committed tree.
- **F2+F3 (MEDIUM+LOW, commit `94677546`):** `prune` double-reported one orphaned pair when the host
  was removed by a bare `rm` (stale registration) — dry-run disagreed with `--apply`. Also
  `removeStalePair` falsely reported `Removed=true` when no weft worktree existed. Fixed by tracking
  Pass-1-emitted slugs to skip in Pass 2, and stat-guarding the weft dir before claiming removal.
  Orchestrator reproduced the bug by reverting `internal/fabricengine/prune.go` and confirmed
  `TestPrune_StaleRegistrationReportedOnce` (both subtests) fails at exactly the right assertions;
  restoring the fix passes it again with an empty diff.
- **F4 (NIT, commit `c860152c`):** comment-only fix in `internal/fabriccli/fabric.go`, no behavior
  change — verified by inspection.
- **F5 (LOW/operability, commit `9fffe6d7`):** doc-only addition to `SANDBOX-FABRIC-SUITE.md`'s
  Pre-conditions, no behavior change — verified by inspection; sandbox coverage guard confirmed
  still green.

Round 2 (`fable-r2`), findings R1-R11, full review at `.scratch/fabric-review-fable-r2.md`, fixer
report at `.scratch/fabric-review-fable-r2-fixer-report.md`:
- **R1 (MEDIUM, commit `dc8dd1d7`):** Add's rollback deleted a pre-existing (merely adopted) weft
  branch — including unpushed commits — on a post-adopt failure (e.g. host push). Fixed by threading
  a `weftBranchAlreadyExists` flag through rollback so an adopted branch's worktree is torn down but
  the branch itself survives. Verified by inspection (integration test
  `add_rollback_adopt_test.go::TestAddRollback_AdoptedWeftBranchSurvives`, added same commit).
- **R2 (MEDIUM, commit `412f93f1`):** Reconcile could never repair a hand-deleted weft worktree — the
  stale git registration made `git worktree add` fail identically on every run. Fixed by running
  `git worktree prune` in the weft repo before adopting. Verified by inspection (integration test
  `reconcile_stale_registration_test.go::TestReconcile_RecreatesHandDeletedWeftWorktree`).
- **R3 (MEDIUM, commit `51a9759e`):** `seedLyxJunction` refused to repair a wrong-target or dangling
  `_lyx` junction with a factually wrong "predates weft" error, breaking Reconcile's documented
  junction-repair contract. Fixed by re-pointing a corrupted link (remove + recreate) and reserving
  the refusal for a real non-link directory. Orchestrator reproduced by reverting `junction.go` to
  pre-fix and confirming the new `junction_repoint_test.go` (both wrong-target and dangling cases)
  fails; restoring passes with an empty diff.
- **R4 (MEDIUM, commit `a499f079`):** `Add` accepted a slash-containing slug the rest of the module
  (pairs/reconcile/prune) cannot represent, causing immediate self-contradiction. Fixed by rejecting
  `/`/`\` in the slug before any git operation. Orchestrator reproduced by reverting `add.go` and
  confirming `TestAdd_RejectsSeparatorSlug` fails at exactly the right assertion (wrong error
  surfaces instead of the validation error); restoring passes.
- **R5 (LOW, commit `81b6973f`):** Cleanup misclassified a live pair as orphaned when the host
  worktree was on a detached HEAD (branch-name liveness check missed it), attempting a doomed
  `git branch -D` on a checked-out branch. Fixed by treating any weft branch checked out at a
  worktree as Protected. Orchestrator reproduced by reverting `cleanup.go` and confirming
  `TestCleanup_DifferentialEquivalence/DetachedHostHeadProtectsCheckedOutWeftBranch` fails at all
  three assertions (Protected=false, Deleted attempted, doomed-delete Error present); restoring
  passes.
- **R6 (LOW, commit `dff8c947`):** `prune --apply` removed a dead pair's weft worktree but left its
  portal junction and launcher dir behind permanently, with no verb to clean them. Fixed by
  `removeStalePair` calling `removePortal`/`removeLaunchers`, mirroring Remove. Orchestrator
  reproduced by reverting `prune.go` and confirming
  `TestPrune_DifferentialEquivalence/ApplyRemovesPortalAndLaunchers` fails (portal + launcher dir
  both still present); restoring passes.
- **R7 (LOW/docs, commit `4524a0b8`):** doc-only board-URL fallback added to
  `SANDBOX-FABRIC-SUITE.md`'s Pre-conditions, no behavior change — verified by inspection; sandbox
  coverage guard confirmed still green.
- **R8 (NIT, commit `efbd98b1`):** vestigial `l *hubgeometry.Layout` parameter and a garbled header
  comment in `reconcile.go`, no behavior change — verified by inspection.
- **R9 (NIT, commit `0918eba6`):** dead `_user_exit=$?` capture dropped from the hook chain wrapper,
  no behavior change — verified by inspection.
- **R10 (LOW, commit `ad4444b9`):** `PairInSync` reported a real (non-link) `_lyx` directory as
  "junction missing" instead of distinguishing it from the genuinely-missing case. Fixed with an
  explicit `os.Lstat` before the `IsLink` check. Orchestrator reproduced by reverting `drift.go` and
  confirming `TestPairInSyncAndHostClean_DifferentialEquivalence/PairInSync_RealDirNotAJunction`
  fails (wrong reason string); restoring passes.
- **R11 (LOW, commit `4e72124c`):** re-evaluation of `opus-r1`'s deferred F6 — `SyncWeft`'s index
  record could disagree with the trailer it just wrote (re-read of warp HEAD instead of parsing the
  trailer back out of the pushed commit). Fixed without a signature change by parsing the `Warp-SHA`
  trailer out of the commit SyncWeft itself just produced. **Note for this round:** the review itself
  judged the pre-fix discrepancy benign in the single-instance synchronous flow (nothing can advance
  warp HEAD between the two reads inside one orchestrated call) — consistent with that, the
  orchestrator's revert-and-confirm-fail check on the new `TestSyncWeft_WarpSHAMatchesTrailer` did
  **not** fail pre-fix (it passed against both old and new code, since the test's synchronous
  scenario cannot manufacture the divergence). The fix was instead verified sound by code inspection
  (trailer round-trips through `parseWarpSHATrailer`, missing-trailer case surfaced as an error). If
  you can find a real, live-reproducible scenario where the pre-fix behavior actually diverged from
  the trailer, that would be new information — otherwise treat this as closed.

Round 3 (`opus-r3`), full review at `.scratch/fabric-review-opus-r3.md`, fixer report at
`.scratch/fabric-review-opus-r3-fixer-report.md`:
- **F1 (MEDIUM, commit `2a41a63f`):** `Checkout` left a half-switched pair when junction wiring
  (step 5) failed after the weft branch had already switched (step 4) — rollback reverted only the
  host, stranding the weft on the new branch, violating Checkout's documented all-or-nothing
  contract. Fixed by capturing the weft's original branch up front and rolling back BOTH sides on
  any post-switch failure (`rollbackSwitch` replacing the host-only `rollbackHostSwitch`).
  Orchestrator reproduced by reverting `checkout.go` and confirming
  `TestCheckout_JunctionFailureRollsBackBothSides` fails at exactly the predicted assertion (weft
  branch = the new target, not the original — half-switched); restoring passes.
- **F2 (MEDIUM, data loss, commit `af50f2af`):** `add <slug>-weft` (a slug ending in the reserved
  weft suffix) created a host worktree directory indistinguishable from a weft worktree, which a
  later `prune --apply` misclassified as orphaned and `os.RemoveAll`'d — silent loss of the host
  worktree and any uncommitted work. Fixed by rejecting the suffix collision in Add's step-0
  validation. Orchestrator reproduced by reverting `add.go` and confirming
  `TestAdd_RejectsWeftSuffixSlug` fails (wrong error, git-step failure instead of validation
  rejection); restoring passes.
- **F3 (NIT, commit `c6fa6dc4`):** an empty/whitespace slug produced a misleading "hub already
  exists" error surfaced deep in a later step instead of an honest validation rejection. Fixed in
  Add's step-0 validation alongside F2. Orchestrator reproduced by reverting `add.go` to the
  post-F2/pre-F3 state and confirming `TestAdd_RejectsEmptySlug` fails (same wrong-error shape);
  restoring passes.
- Re-evaluated `opus-r1`'s deferred F6 again (now via R11's fix): remains closed, no new
  live-reproducible divergence found.
- One thing explicitly NOT fixed, by design: empty result lists still serialize as JSON `null`
  rather than `[]` — a nil-slice convention shared across the whole lyx codebase and warp;
  diverging fabric alone would break differential parity for zero functional gain. Do not re-flag
  this as a fabric-specific defect.
- Round 3's own review agent was originally spawned as Sonnet (`sonnet-r3`) but killed before it
  wrote or touched anything, then relaunched clean as `opus-r3` to correct the model rotation — see
  above; no findings or partial work from the killed attempt exist to re-litigate.

Round 4 (`fable-r4`), full review at `.scratch/fabric-review-fable-r4.md`, fixer report at
`.scratch/fabric-review-fable-r4-fixer-report.md`:
- **F3-r4+F6-r4 (MEDIUM, commit `b90e75e9`):** `clone` of a hub whose weft remote already had
  `<primary>-weft` forked a new *untracked* local branch at the primary's HEAD instead of adopting
  the existing remote branch — silently disowning all previously-synced weft history (confirmed
  live: 185 lines of `_lyx/config` lost) with no recovery path (the first push of the untracked
  branch can never rebase-reconcile with the disowned history). This hit every `clone --reset`
  re-clone. Fixed: `suffixWeftPrimaryBranch` checks for `refs/remotes/origin/<suffixed>` first and
  checks out a tracking branch of it when present, creating fresh only otherwise. (F6-r4, same
  commit: the clone docs' false "renamed" wording corrected alongside.) Orchestrator reproduced by
  reverting `clone.go` and confirming `TestCloneHub_AdoptsExistingRemoteWeftPrimaryBranch` fails;
  restoring passes.
- **F2-r4 (MEDIUM, commit `8012862c`):** fabric's own lock artifacts (`.weft/`,
  `.gitrepo-push.lock`) were never git-excluded, so they permanently dirtied the weft worktree —
  `remove` without `--force` refused every pair that had ever synced, with a "run lyx fabric sync"
  hint that could never actually clear the dirt. Fixed: idempotent `info/exclude` seeding
  (`seedWeftArtifactExcludes`) at the lock choke point, which also heals pre-existing dirt since
  excludes are evaluated at status-time. `gitrepo`'s `pushLockFile` exported as `PushLockFileName`
  so the literal has one owner. Orchestrator reproduced by reverting `weftgit.go` and confirming
  `TestCommitWeft_LockArtifactsExcludedFromStatus` fails (both `.weft/` and the lock file show as
  porcelain dirt); restoring passes.
- **F1-r4 (MEDIUM, commit `fc2a0c05`):** the per-worktree correspondence index cache survived a
  coordinated `checkout` branch switch; because `SHAExists` only checks that a recorded commit
  exists *somewhere* in the weft repo (not on the current branch), stale cross-branch entries kept
  passing validation, so lookups (and `RevertWithWeft`) could serve a weft SHA the current branch's
  own trailer history would never produce. Fixed: Checkout discards and rebuilds the index
  (`refreshCorrIndexAfterSwitch`) from the newly-current branch's trailers after a successful
  switch; `manifest/designs/fabric.md`'s correspondence-index section documents the
  per-worktree-cache-vs-per-branch-source mismatch. Orchestrator reproduced by reverting
  `checkout.go`+`index.go` and confirming `TestCheckout_RefreshesCorrespondenceIndex` fails
  (`WeftSHAForWarpSHA` returns a stale cross-branch answer with nil error instead of
  `ErrNoCorrespondence`); restoring passes.
- **F4-r4 (LOW, commit `7e3bd922`):** `Add` accepted a slug matching a reserved hub-geometry name
  (`_lyx`, `_raddle`, `_board`, `_portals`, `_launchers`), live-confirmed to create a host worktree
  whose directory collides with paths lyx composes at the hub level. Fixed with a new
  `hubgeometry.IsReservedHubName` (single-owner literals per the Hub Geometry Invariant) and Add
  step-0 rejection. Orchestrator reproduced by reverting `add.go` and confirming
  `TestAdd_RejectsReservedHubNameSlug` fails (wrong error: git-step failure instead of the
  validation rejection); restoring passes.
- **F5-r4 (LOW, commit `1913a409`):** `Add`'s branch-already-exists rejection was a bare error with
  no next step. Fixed: the message now names both remedies (checkout onto the branch, or
  `git branch -D` a leftover) — message-only change, Remove's branch-preserving behavior unchanged.
  Verified by inspection + the new `TestAdd_ExistingBranchErrorNamesRemedy`.
- **F7-r4 (NIT, commit `1ef60af1`):** a rolled-back fork-checkout left the weft branch Checkout had
  just forked stranded as an orphan (adopted pre-existing branches were correctly preserved; only
  the fork case leaked). Fixed: `rollbackSwitch` deletes a forked branch on rollback, never an
  adopted one. Orchestrator reproduced by reverting `checkout.go` and confirming
  `TestCheckout_JunctionFailureDeletesForkedWeftBranch` fails ("forked weft branch survived the
  rollback"); restoring passes.
- Also `6adda8dc`: goimports formatting-only normalization of a pre-existing comment list in
  `add.go`, no behavior change — verified by inspection.
- Observations recorded but deliberately NOT fixed (reasons in the fixer report, do not re-flag):
  reconcile-from-a-broken-worktree config error (warp-parity, workable advice as-is);
  `links_removed` always reporting 0 (warp-parity, cosmetic); JSON `null` for empty result lists
  (closed whole-repo convention, same as R11's rationale above).

Full suite confirmed green by the orchestrator from a cold state on the committed tree after every
round (not just trusting either round's own report): `go build ./...`, `go vet` on the
fabricengine+fabriccli+gitrepo+cmd/lyx packages, `go test -count=5` (hermetic, all packages),
`go test -tags integration -count=1` (all packages). Round 4 also touched the shared
`internal/gitrepo` and `internal/hubgeometry` packages (a mechanical exported-constant rename and a
purely-additive helper, respectively) — the orchestrator additionally ran whole-repo
`go build ./...` / `go vet ./...` / `go test -count=5 ./...` / `go test -tags integration -count=1
./...` and confirmed no regression anywhere, including `warpengine`/`weftengine`.

State the **merge bar** so you calibrate: correctness in the NORMAL single-instance flow is the
gate; an N×-concurrent suite (if you choose to run one — fabric is not inherently a
concurrency-heavy module like mux, so this is optional here, not a required gate) is a diagnostic
amplifier at most, never a merge blocker on its own.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...`
- `go test -count=5 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...`

Live smoke (real substrate, behind the `smoke` build tag):
- `go test -tags smoke ./internal/fabriccli/... -run Smoke -v -count=1`
- Substrate is plain `git` (must be on PATH) plus real filesystem junctions/symlinks via
  `internal/fslink` — no external binary/tool dependency beyond `git` itself.

Live driving — YOU drive it directly, no launcher (PRIMARY — where the bugs surface):
- Deploy the current source as the binary under test (`deploy.cmd` if present in this repo;
  otherwise `go build -o <path-on-PATH>/lyx.exe ./cmd/lyx` — confirm which applies here).
  **FOOTGUN:** live driving runs the DEPLOYED snapshot, not your working tree — re-deploy after
  EVERY source change or you validate a stale binary.
- **Do NOT invoke `sandbox-fabric-suite.cmd`.** That launcher spawns a SEPARATE, context-free
  interactive `claude` session for a human operator's own dogfooding, not for you to spawn on top
  of yourself. Instead, run the real CLI commands yourself, directly, foreground, waiting for each
  to return: walk the "High-yield focus" list above (and `SANDBOX-FABRIC-SUITE.md`'s F0-F3
  scenarios, for extra ideas) against the dedicated fabric hub geometry it describes, and record
  OK/WARN/FAIL for each. This spawns real git subprocesses and real filesystem links underneath —
  that is expected and required.
- The list above is a FLOOR — devise and run MANY more adversarial scenarios of your own beyond it
  (combine verbs in orders nothing has tried; chase anything the code makes you suspicious of:
  crash/rebirth mid-op, cross-worktree scope confusion, dead-but-present state, mid-op-failure
  orphans, rapid add/remove/checkout churn).
- **"Headless" means "no human required" — NOT "no time/token cost to me."** A real git operation
  takes real wall-clock time, not zero. That cost is EXPECTED and BUDGETED FOR, never a reason to
  skip a scenario. You are explicitly forbidden from writing "operator-assisted", "cost-bearing",
  "long-running", "impractical", or "automated context" as a reason to skip live driving.
- **Before writing "could not verify", ask yourself literally: "would a human's physical eyes be
  required here, or am I just trying to avoid spending my own time/turns?"** Only the first is a
  real reason. If a scenario just takes several minutes of you waiting on a real command to
  return, that is not a reason — wait for it, and report the actual output (with the commands you
  ran) as evidence.
- The only legitimate "cannot verify" cases are: (a) a scenario that structurally requires a human
  to visually confirm something, or (b) a genuine environment gap (missing `git`, no GitHub auth
  for the dedicated test repos — check for this FIRST, before anything else). Flag those specific
  cases as not-headlessly-verifiable rather than skipping silently, and say exactly what blocked
  you.

TEARDOWN DISCIPLINE (critical): if you clone/materialize any hub, worktree, or link during testing,
tear it down. At the end, confirm ZERO stray fabric-managed worktrees/junctions/symlinks and no
uncommitted drift left in the dedicated test repos you touched. Leave no stray state. Be honest
about what you could NOT verify and why.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong behavior),
severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED (reproduced/traced) vs
PLAUSIBLE (looks wrong, unverified). For scope: plan-promised vs shipped; flag
deferred-that-should-be-v1 and shipped-beyond-scope.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings you record get
fixed in Job 2 — including every NIT — not just BLOCKING/MEDIUM ones. The only legitimate reason to
leave a finding unfixed is that fixing it genuinely requires something you cannot do alone this
round — an operator decision on a real design tradeoff, or a live capability you don't have. Even
then you must say so explicitly, with the specific reason, in the fixer report's deferred section —
never bucket something as "deferred, low priority" just because it felt small.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
- **F6 (NIT, from `opus-r1`):** `internal/fabricengine/syncweft.go`'s `SyncWeft` re-reads
  `f.Warp.CurrentSHA()` independently of the SHA `CommitWeft` already used for the `Warp-SHA`
  trailer — a benign discrepancy only if warp HEAD could move mid-sync, which cannot happen in the
  single-instance merge-gate flow (`RebuildIndex` reconciles from the trailer regardless). Deferred
  because fixing it means threading the trailer's SHA out of `CommitWeft` through its CLI callers —
  a signature change, not a bug, at the merge bar. Re-evaluate: do you agree it's genuinely benign at
  the merge bar, or does your own review surface a path where it matters?

## Fixing — after the review
- Fix EVERY finding from your review, all severities including NIT.
- Load the code-quality guidance (`/code-quality` skill) AND `mill:golang-build`/
  `mill:golang-testing`/`mill:golang-comments` before editing.
- For every bug you fix, add or extend a test that would have caught it. For a live-only defect, add
  a `//go:build smoke` test that walks the failing scenario against the real substrate.
- MAKE SMOKE TESTS DETERMINISTIC. Git/filesystem operations are not instantaneous; a test that
  assumes a verb is synchronous passes on a quiet machine and FLAKES on a loaded one. Wait on the
  actual state transition (poll with a deadline), never sleep a fixed amount.
- Extend `tools/sandbox/SANDBOX-FABRIC-SUITE.md` when your review surfaces a live/visual behavior it
  doesn't cover (match the existing scenario shape; keep the coverage guard green in the SAME
  change — every scenario file needs at least one `**Covers:** fabric` tag). If the change doesn't
  warrant a new scenario, note it in your fixer report instead.
- Keep `go build`/`vet`/`test` green after every change. Then RE-DEPLOY and re-run every live
  scenario yourself, directly.
- Update `manifest/designs/fabric.md` (and `docs/overview.md` / `CONSTRAINTS.md` if invariants or
  the module table move) IN THE SAME change. Do NOT add bugfix/hardening notes to
  `manifest/roadmap.md`.
- Tear down all substrate state; confirm zero stray processes/worktrees/links. COMMIT each fix as
  you finish it (see "Commit per fix" above) — do NOT push unless the user explicitly asks. Report
  the changed files and how you verified each fix.

## Deliverables
1. A structured review report (Executive summary with top risks + merge-readiness opinion; Scope
   assessment plan-vs-shipped; Code findings severity-ranked with file:line + scenario + fix +
   CONFIRMED/PLAUSIBLE; Docs & operability findings; What-was-tested with exact commands + observed
   results, including what you could NOT verify and why). Write it to
   `.scratch/fabric-review-<yourtag>.md`.
2. A fixer report: what you implemented, what you deliberately deferred (with reasons), the exact
   test commands run + results, and the changed files. Write it to
   `.scratch/fabric-review-<yourtag>-fixer-report.md`.
3. In your final chat message: a concise summary (executive summary + counts by severity + the two
   report paths + an explicit merge-readiness verdict). Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code + docs, then drive the real substrate),
produce your independent findings, then implement and verify the fixes.
