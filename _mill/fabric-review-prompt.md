# `fabric weft-is-never-merged diff` — independent review + fix (prompt instance)

> Filled from `crucible/review-prompt-template.md` for a **diff-scoped** crucible round — not a full-module sweep. `internal/fabricengine`/`internal/fabriccli` have already been through multiple full crucible campaigns (see `crucible/README.md`'s worked examples and `git log --oneline | grep crucible` for `fabric-merge-crucible-round4`, `fabric-cutover`, `fabric-v2-crucible`, `fabric-crucible-hardening` (2026, earlier instance)). This round's job is the NEW surface one specific merge introduced, not re-litigating what those campaigns already covered.

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of **the diff landed by merge commit `ab99f531`** ("Add a local-only file category to weft" — the `weft-local-only-files` task's `weft-is-never-merged` redesign), followed by FIXING what you find.
Work in the worktree at `/home/knatte/Code/loomyard/wts/fabric-crucible-hardening` (branch `fabric-crucible-hardening`).
Adjust that path/branch if the task lives elsewhere now.

## Scope — read this before anything else

This round reviews **only** the files `ab99f531` touched, not the whole fabric module:

```
internal/fabriccli/fabric.go
internal/fabriccli/weft_verbs.go
internal/fabricengine/cleanup.go
internal/fabricengine/destroy.go
internal/fabricengine/doc.go
internal/fabricengine/merge.go
internal/fabricengine/mergeguards.go
internal/fabricengine/mergelifecycle.go
internal/fabricengine/mergestate.go
internal/fabricengine/mergestateactive.go   (NEW)
internal/fabricengine/pull.go
internal/fabricengine/pushanchored.go        (NEW)
internal/loomcli/wiring.go
internal/loomcli/landingdeps.go
internal/loomcli/cli.go
internal/loomrecipe/loomrecipe.go
internal/shedengine/run.go
internal/shedengine/shed.go
internal/landingshed/finalize.go             (via finalize_integration_test.go)
manifest/designs/loom.md
manifest/designs/shed.md
```

Get the exact diff yourself first: `git show ab99f531 --stat` and `git show ab99f531 -- <path>` per file above — do not trust this list's completeness over the real diff.

**Explicitly OUT of scope** (do not review, do not flag as missing coverage):
- Anything in `internal/fabricengine`/`internal/fabriccli` NOT touched by `ab99f531` — that surface has its own crucible history (multiple prior campaigns, see header note above).
- `internal/loomcli`'s pre-existing `smoke_test.go`/`smoke_attachprobe_test.go` — these are reed/loom-attach LLM-driving smoke coverage, unrelated to this diff. **Do not run them.** The CommitStatus wiring this diff added is covered by `wiring_commitstatus_test.go`/`run_commitstatus_test.go` instead — both Tier 1 (hermetic, no build tag, stub-closure-driven, no git spawn) per their own doc comments.
- raddle (removed from the codebase; do not review or resurrect anything related to it).

## Your two jobs, in order
1. REVIEW: form your own independent judgment of this diff's scope and correctness. Hunt for bugs by reading the code AND by driving the real substrate (real git repos via `gitkit`/`hubforge` fixtures, and the real `lyx fabric`/`lyx loom` CLI against a scratch hub) — this is where the defects hide.
2. FIX: after you have a findings list, implement the fixes one at a time, verify each against the real substrate, keep the whole test suite green, and update the docs in the same change as the fix they document.
   COMMIT after each individual fix lands green (see "Commit per fix" below).
   Do NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green (`go build`/`vet`/hermetic test, plus the live integration check if the finding needed one), and its doc update (if any) is included, COMMIT it — on the current branch, no push — before starting the next finding.
Commit message format: `fabric: fix <finding-id> — <one-line what/why>`.
Also commit `_mill/fabric-review-<yourtag>.md` and `_mill/fabric-review-<yourtag>-fixer-report.md` as you write or update them — they are the campaign's durable record, not gitignored scratch.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to `_mill/fabric-review-<yourtag>.md` and committed — before you touch (edit, create, or delete) a single production or test file. Do not fix findings as you go. Write it down, keep reading, finish the review, save the file, THEN start Job 2.

## Log as you go during Job 1 (BLOCKING)
Append your observations to `_mill/fabric-review-<yourtag>.md`'s "What was tested" section immediately after each command/scenario returns. Jot findings into the file's findings section provisionally as you spot them. COMMIT each meaningful append.

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first. Do NOT read any prior review or review-dialogue file before you have your own list — specifically nothing under `_mill/` matching `fabric-review-*`. Reading the design SPEC and module docs is expected and required.
AFTER you have your own independent findings, you MAY consult prior rounds' `_mill/fabric-review-*` material (except your own `-<yourtag>` deliverables) to (a) confirm previously-fixed behaviors have not regressed and (b) re-evaluate the deferred items at the bottom.

## What to read
- Code: the file list above — read every one of them in full, not excerpts.
- Docs: `manifest/designs/fabric-unified-view.md`, `manifest/designs/loom.md`, `manifest/designs/shed.md`, `docs/overview.md`, `CONSTRAINTS.md` (esp. the Fabric Destruction Chokepoint Invariant, Mutation Record Invariant, Fabric Vocabulary Invariant), `README.md`.
- Sandbox suite (scenario ideas only, never invoke its launcher): `tools/sandbox/SANDBOX-FABRIC-SUITE.md`.
- **Design intent (SPEC)**: the original task's `_mill/discussion.md` no longer exists in this worktree (its own worktree was torn down after landing) — recover it from git history instead: `git show ec433317:_mill/discussion.md`. That commit ("mill-start: discussion-gap-fix round 5 for weft-local-only-files") is the LAST write to that file and the authoritative source of intended scope/behavior for this diff. Do NOT use `4b30b14e` (the initial write) — it describes the wiki brief's original `MergeOptions.LocalOnlyPaths` per-path design, which `4ccd610d` (gap-fix round 1) explicitly rejected in favor of the "weft is never a merge participant" design actually shipped; rounds 2-5 (through `ec433317`) only add precision/detail on top of that same rejection, not a further scope change. Treat it as the SPEC, not a review — reading it is required, not clean-room-violating. (round `sonnet-high-r2` caught the `4b30b14e` staleness live; this citation was corrected by the orchestrator afterward.)
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md`.

## Mission (assess on two axes, be adversarial)
1. Scope — does the as-built diff deliver what `_mill/discussion.md` (recovered above) intended? Gaps, over-reach, silently-dropped requirements.
2. Correctness — bugs, races, error handling, edge cases, concentrated in the high-yield focus list below. Also docs accuracy (do `doc.go`'s new/edited paragraphs and `manifest/designs/loom.md`/`shed.md` match the code?) and operability.

## High-yield focus — where this diff's real bugs live (drive these, do not just read them)
This diff's central claim is **"the weft is never a merge participant, in either verb or either direction"** (`doc.go`'s new "merge surface" section). Every item below is a way that claim could be false, or could be true-but-unsafe:

- **Weft state must never block or alter a warp-only merge, even in combination.** `mergeguards.go` dropped weft from `pairDirtyReason`, `detachedHeadReason`, `syncedToUpstreamReason`, and `resolveMergeSources`'s refusal arm — but only per-guard. Drive a merge where the weft sibling is SIMULTANEOUSLY dirty AND detached AND mid-foreign-merge at once (not one at a time, as the unit tests likely do) and confirm the warp-only merge still proceeds/refuses purely on warp state. Repro: build a `newMergePairFixture`, dirty the weft worktree, detach its HEAD, and drop a real `MERGE_HEAD` into its `.git` before calling `MergeIn`/`Merge`.
- **`MergeAbort` restoring warp-only — what happens to weft-side partial state a failed attempt left behind?** `doc.go` now says abort "restor[es] the warp side to its pre-merge SHA" (no more "both sides"). If some earlier step of a merge attempt DID touch weft (check `mergelifecycle.go`, `mergestate.go` for any weft write between `MergeIn`/`Merge` start and a warp-side abort), confirm that write is either impossible by construction or is itself cleaned up — an abort that silently leaves weft-side debris behind would be a real regression from the old "restore both sides" contract. Trace this by reading, then prove it live: interrupt a merge attempt (conflict on warp) and inspect the weft worktree's `git status`/`git log` before and after `MergeAbort`.
- **`MergeStateActive`'s probe-then-act window (TOCTOU) in the new CommitStatus seam.** `newCommitStatusSeam` (`internal/loomcli/wiring.go`) calls `MergeStateActive` (weft-only git-level probe: `MERGE_HEAD` present or conflicted index), and if false, proceeds to `Commit` then `Push` — with NO lock held across the probe and the commit. Drive this live: start a real transition loop (or a narrow harness calling the seam directly) and, in the gap between the probe returning false and the commit landing, land a real `MERGE_HEAD` on the weft worktree (a concurrent `lyx fabric merge`/manual conflict). Confirm the commit-status write either fails cleanly or does not corrupt/interleave with the concurrent merge's own conflict resolution — this is exactly the class of bug rule 5/6 in `crucible/README.md`'s fabric-campaign refinements (sabotage-prove, don't just trust a green run) exists to catch.
- **`PushAnchored`'s unwrapped `gitrepo.ErrPushRejected` must survive to the seam's `errors.Is` check.** `pushanchored.go`'s own doc comment calls this "load-bearing, not incidental." Sabotage it: force a real push rejection (push, then advance the remote weft branch out from under the local worktree, then have the seam push again) and confirm `newCommitStatusSeam`'s push-warns disposition actually fires (a WARN log, `Run` continues) rather than halting the run — then separately confirm a DIFFERENT push error (e.g. no network / bad remote URL) is NOT silently swallowed the same way, since only `ErrPushRejected` should warn-and-continue.
- **The three CommitStatus dispositions (commit-hard-errors / push-warns / skip-while-mid-merge) — prove each live, not just via the Tier 1 stub tests.** Wire `loomCommitStatusDeps` against a REAL fabric pair (not stub closures) and drive an actual multi-transition `lyx loom` run (or the narrowest real harness that exercises `Shed.Run`'s `CommitStatus` hook) through all three paths: (a) a real commit failure (e.g. a locked/corrupted weft `.git`), (b) a real push failure that IS `ErrPushRejected`, (c) a real weft-side `MERGE_HEAD` present throughout a transition. Confirm the run's actual halt/continue/skip behavior matches the doc comment, and that the WARN-level log lines actually appear (grep captured output).
- **`cleanup.go`/`destroy.go`'s narrowed carve-outs (the raddle-gate-removal fallout).** This diff itself trimmed a stale-comment/dead-test tail from an EARLIER change (the `raddleFoldedBack` gate removal) — re-verify live, not just by reading: an orphaned weft branch is deletable under `Cleanup(apply=true, force=false)`; the PRIMARY weft branch and an unmanaged non-`-weft`-suffixed branch both stay protected under the same call. Drive all three via a real `lyx fabric cleanup --apply` against a scratch hub, not just the (now-removed) pinning unit tests — confirm the removal of that dedicated test file didn't leave a real behavior gap, only a redundant-coverage gap.
- **`doc.go`'s claim that `unifyConflictPaths`' weft conflict list is "permanently empty, never populated."** Try to falsify it: construct a scenario where a weft-side change WOULD have conflicted under the old two-sided merge (e.g. the same `_lyx/loom/status.json` path edited differently on both source and target weft branches) and confirm `MergeResult.Conflicts` genuinely never surfaces a weft entry, under a real `lyx fabric merge`, not just the packaged `mergeweftlocal_integration_test.go` scenarios (read them first, then go beyond — try combinations that file doesn't cover, e.g. a weft-side rename/delete instead of just content rewrites).

## Explicitly OUT of scope for this diff
- Anything in `fabricengine`/`fabriccli` untouched by `ab99f531` (see file list above) — do not go hunting there; that surface has its own crucible history.
- `internal/loomcli`'s reed/loom-attach LLM-driving smoke tests — unrelated pre-existing coverage.
- raddle — removed, do not resurrect.
- The correctness of `Fabric.Commit`'s general async-push/lock machinery predating this diff (only the NEW `PushAnchored` entry point and its consumption by the CommitStatus seam are in scope).

## Round context seeded from prior-round verification
**Safety pass.** Round `opus-high-r1` found 0 BLOCKING, 4 MEDIUM, 5 LOW, 4 NIT and fixed all 13. The orchestrator independently verified — cold `go build`/`go vet`/`go test -count=5` over the in-scope packages, cold `-tags integration` over fabricengine/fabriccli/landingshed, 3× concurrent copies of the fabricengine integration binary (clean, no real FAIL/panic/race marker), and personally sabotage-proved the one real production fix (see below) by reverting it, watching all three of its regression tests fail at the intended assertion, then restoring and confirming an empty diff. No residual was found. Full detail: `_mill/fabric-review-opus-high-r1.md` and `_mill/fabric-review-opus-high-r1-fixer-report.md` — you MAY read these now that you are seeded (the clean-room constraint only applies to forming your OWN findings first; consult them afterward per that section).

Do a genuinely independent clean-room pass — form your own findings first, per the high-yield focus list above, BEFORE reading the round-1 material — to find anything round 1 missed, or honestly confirm merge-readiness ("no new defects, ship it" is the expected, valuable outcome of a safety pass — do not invent work).

**Do NOT re-open the following CLOSED-AND-VERIFIED items** (all independently confirmed by the orchestrator, not just self-reported by round 1):
- F1 (`pushanchored.go` doc no longer claims an `errors.Is(ErrPushRejected)` discrimination that doesn't exist) — `79947900`
- F2/F11/F13 (the `MergeStateActive` probe-then-commit TOCTOU in `internal/loomcli/wiring.go`'s `newCommitStatusSeam` now takes the skip disposition instead of hard-erroring when a commit fails because a merge went live after the probe; doc enumeration reordered to match execution; `loom.md` names both skip causes) — `c974f8ae`, sabotage-proved by the orchestrator directly
- F3 (`doc.go`: the weft conflict list is not permanently empty — `MergeContinue` populates it from a real foreign weft conflict) — `694029c6`
- F4 (`doc.go`: the weft has not lost ALL power to block a merge — `foreignMergeStatePresent` and a weft conflicted index both still refuse) — `d16b8adc`
- F5 (`merge.go` file header no longer claims `MergeIn` touches the weft) — `df185c8b`
- F6 (`ErrWarpDirty` no longer promises the weft was fast-forwarded) — `84687354`
- F7 (`cleanup`'s reserved `--force` pinned as inert; orphan/primary/unmanaged carve-outs all reconfirmed live) — `79366900`
- F8 (real-substrate integration coverage added for the CommitStatus seam against a live fabric pair, closing the stub-only gap) — `0135044f`
- F9 (narrowed weft guards pinned in combination — dirty AND detached together — not just one at a time) — `4c00a8e3`
- F10 (single merge-state write at `MergeIn`/`Merge` start, `WeftOutcome` pre-filled in the struct literal, closing the empty-`WeftOutcome` crash window) — `769ec82d`
- F12 (`PushResult`'s doc states the invariant instead of a roll-call that had already gone stale twice) — `6045f34c`

Two items round 1 explicitly disclosed as residuals rather than defects — do not re-flag these as new findings unless you find a reason the disclosure itself is wrong:
1. F2's remaining `git add` staging into a foreign merge's index on a lost race (the commit-hard-error is now avoided, but the file is still staged in the operator's index before the failure is detected) — bounded, closing it needs a lock the operator doesn't take.
2. `sideRecordedMergeGone`'s squash exemption — pre-existing, untouched by `ab99f531`, out of this campaign's scope.

State the **merge bar**: correctness in the NORMAL single-instance flow is the gate. An N×-concurrent suite (if you run one against the integration-tagged git tests — see cost declaration below, this is CHEAP for this module, unlike an LLM-driving one) is a diagnostic amplifier, not a merge blocker.

## Live-substrate cost declaration
**LLM-DRIVING: no**, for everything in scope. `fabricengine`/`fabriccli`'s live tests are tagged `//go:build integration` (not `smoke`) and drive real git repos only (via `gitkit`/`hubforge` fixtures) — no LLM subprocess, no tmux. This is the CHEAP substrate class (like reed's tmux case): a bare `-run Integration`-style pattern or N concurrent copies is safe to run broadly for the packages in scope.
The one thing to actively AVOID: `internal/loomcli`'s `smoke_test.go`/`smoke_attachprobe_test.go` are LLM-driving (real `claude` subprocesses, per the loom-crucible-hardening round 1 seed's empirical finding) and are OUT OF SCOPE — do not run them, bare or otherwise. The CommitStatus wiring you ARE reviewing in `loomcli` is covered by separate, untagged, hermetic Tier 1 tests (`wiring_commitstatus_test.go`, `run_commitstatus_test.go`) — safe to run freely.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
```sh
go build ./...
go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/loomcli/... ./internal/shedengine/... ./internal/landingshed/... ./internal/loomrecipe/... ./cmd/lyx/...
go test -count=5 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/loomcli/... ./internal/shedengine/... ./internal/landingshed/... ./internal/loomrecipe/... ./cmd/lyx/...
```

Live integration (real git substrate, behind the `integration` build tag — cheap, see cost declaration):
```sh
go test -tags integration ./internal/fabricengine/... -v -count=1
go test -tags integration ./internal/fabriccli/... -v -count=1
go test -tags integration ./internal/landingshed/... -v -count=1
```
Scan output for FAIL and for any panic/data-race marker. A couple of concurrent runs of the fabricengine integration suite (compile once, run N copies, same pattern as `orchestrator-prompt.md`'s verification protocol) is a fine diagnostic amplifier here — this is the cheap-substrate case, not the LLM-driving one.

Live driving — YOU drive it directly, no launcher (PRIMARY — where the bugs surface):
- Deploy the current source as the dev binary: `deploy-dev.cmd` (`deploy-dev` on POSIX). Re-deploy after EVERY source change before re-testing live.
- Do NOT invoke `sandbox-fabric-suite.cmd`. Run real `lyx fabric ...`/`lyx loom ...` commands yourself, foreground, waiting for each to return.
- Walk the High-yield focus list above — each bullet names a concrete repro. Devise more adversarial combinations beyond it (chase anything the code makes you suspicious of, e.g. interacting guard states, timing windows around the CommitStatus seam).
- Report exact commands + observations for each.

TEARDOWN DISCIPLINE: no tmux/LLM substrate to tear down here, but confirm no stray `.weft/*.lock` files, no leftover `fabric-merge.json` records, and no dangling scratch hubs/worktrees survive in any location outside `t.TempDir()`-managed test dirs after your live driving. Be honest about what you could NOT verify and why.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED (reproduced/traced) vs PLAUSIBLE (looks wrong, unverified).
For scope: `_mill/discussion.md` (recovered at `ec433317`)-promised vs shipped; flag deferred-that-should-be-v1 and shipped-beyond-scope.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings you record get fixed in Job 2 — including every NIT.
A LARGE finding (a genuine subsystem addition, a cross-cutting refactor reaching outside this diff's scope) does NOT belong in this round's commit-per-fix loop — record it fully, mark it explicitly NOT-FIXED-THIS-ROUND with the reason, and the orchestrator will spin it into its own mill-wiki task.

## Deferred items from the prior round — RE-EVALUATE these
None — round 1.

## Fixing — after the review
- Fix EVERY finding from your review, all severities including NIT.
- Load `/code-quality` AND `mill:golang-build`/`mill:golang-testing`/`mill:golang-comments` before editing.
- For every bug you fix, add or extend a test that would have caught it — a hermetic unit test for a pure helper, or an `//go:build integration` test for composed real-git behavior.
- Keep `go build`/`vet`/`test` green after every change. Re-deploy (`deploy-dev.cmd`) and re-run every live scenario yourself, directly.
- Update `manifest/designs/fabric-unified-view.md`, `manifest/designs/loom.md`, `manifest/designs/shed.md` (and `docs/overview.md`/`CONSTRAINTS.md` if invariants move) IN THE SAME change as the fix. Do NOT add bugfix/hardening notes to `manifest/roadmap.md`.
- COMMIT each fix as you finish it — do NOT push unless explicitly asked. Report the changed files and how you verified each fix.

## Deliverables
1. A structured review report at `_mill/fabric-review-<yourtag>.md` (executive summary, scope assessment, code findings severity-ranked with file:line + scenario + fix + CONFIRMED/PLAUSIBLE, docs & operability findings, what-was-tested with exact commands + results). Commit it as you build it incrementally.
2. A fixer report at `_mill/fabric-review-<yourtag>-fixer-report.md` (what you implemented, what you deferred and why, exact test commands + results, changed files). Commit it.
3. In your final chat message: a concise summary (executive summary + counts by severity + the two report paths + an explicit merge-readiness verdict). Do not paste the whole reports.

Begin with the clean-room review (read the SPEC at `ec433317:_mill/discussion.md` + the diff files + the module docs, then drive the real substrate), produce your independent findings, then implement and verify the fixes.
