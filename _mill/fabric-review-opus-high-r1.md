# fabric `weft-is-never-merged` diff — independent review (round `opus-high-r1`)

Scope: the diff landed by merge commit `ab99f531` ("Add a local-only file category to weft").
Worktree: `/home/knatte/Code/loomyard/wts/fabric-crucible-hardening`, branch `fabric-crucible-hardening`.
Clean-room: no prior `_mill/fabric-review-*` material existed in this worktree when this pass started (only `_mill/fabric-review-prompt.md` and `_mill/status.md`), so every finding below is independently formed.

## Executive summary

The as-built diff is a **redesign** of the SPEC recovered at `4b30b14e:_mill/discussion.md`, not an implementation of it, and the redesign is the better solution.
The SPEC proposed `MergeOptions.LocalOnlyPaths` plus a child-side index-only delete and a parent-side restore-from-HEAD; the shipped change instead removes the weft from the merge surface entirely.
Since `structuralCommittedDirs` routes exactly one directory (`_lyx`) to the weft, "the weft is never a merge participant" and "`_lyx` is a local-only path set with exactly one member" are behaviourally the same contract, reached with far less machinery — no forced `--no-ff`, no restore window between `MergeStart` and `concludeMergeSides`, no `DropLocalOnly` closure, no `landingshed` step-ordering dependency.
Every merge-shape the SPEC's own Testing section demanded (parent absent, parent diverged, #497 bug-2 delete, FF-able parent with `Squash: false`, symmetric `MergeIn`) is satisfied trivially rather than mechanically, because the parent weft is never written at all.

The mechanism is **correct where it matters** — I could not falsify the core claim by driving it.
A warp-only merge proceeds with the weft simultaneously dirty and detached; an abort restores warp alone and leaves in-flight weft commits intact; the parent weft is byte-identical after a landing; `--apply` alone deletes an orphan weft branch while the primary and unmanaged branches stay protected; `Pull`'s weft arm is genuinely non-fatal.

What the round found is **one real interaction defect and a cluster of doc claims that a single live command falsifies**.
The defect is the CommitStatus seam's probe-then-act window: a `MERGE_HEAD` landing in the weft between `MergeStateActive` returning false and the status commit running does not merely halt the run, it *stages the status file into the foreign merge's index*, so the operator's own merge commit carries content that merge never brought in.
The doc claims are load-bearing rather than cosmetic: `pushanchored.go` describes an `errors.Is(err, gitrepo.ErrPushRejected)` discrimination in its consumer that does not exist anywhere in the tree, and `doc.go`'s "merge surface" section — the diff's own summary of its central claim — makes two absolute statements ("permanently empty, never populated"; "the weft has lost its power to block a merge") that I disproved live in one command each.

Counts: **0 BLOCKING, 4 MEDIUM, 5 LOW, 4 NIT.**
Merge bar (correctness in the normal single-instance flow) is met; nothing here blocks the merge.

## Scope assessment — SPEC (`4b30b14e:_mill/discussion.md`) vs shipped

| SPEC item | Shipped | Verdict |
| --- | --- | --- |
| `shedengine.Shed.CommitStatus` seam called from `persist` | Yes, `run.go:358-380`, nil-safe, outside the state lock | Delivered as specified |
| `loomrecipe.ShedPaths.CommitStatus` threaded through | Yes, both fill sites (`wiring.go:129`, `wiring.go:346`) | Delivered |
| `loomcli` closure commits then pushes, respecting `SkipPush` | Yes, `newCommitStatusSeam` + `EnvSyncOptions()` | Delivered |
| `fabricengine.PushAnchored(l, opts)` synchronous, vocabulary-neutral | Yes, `pushanchored.go`, exact mirror of `PushWarpRebaseFreeAt` | Delivered |
| `MergeOptions.LocalOnlyPaths` + parent-side restore + child-side restore + forced `--no-ff` | **Not built** — superseded by removing the weft from the merge surface | Deliberate, better redesign; net contract identical for the only member (`_lyx`) |
| `landingshed.Deps.LocalOnlyPaths` / `Deps.DropLocalOnly` / pre-merge delete step | **Not built** — unnecessary under the redesign | Deliberate, consistent |
| `landingshed.Deps.CommitStatus` kept as a no-op safety net | Yes, `landingdeps.go` doc rewritten to say exactly that | Delivered |
| `publish.go` untouched | Yes | Delivered |
| `CONSTRAINTS.md` gains a third state category | Replaced by one bullet: "Weft content is per-branch and is never a merge participant in either direction" | Adapted to the redesign; acceptable, and stronger than the SPEC's wording |
| `manifest/designs/loom.md` + `shed.md` same commit | Yes | Delivered |
| millhouse untouched | Yes | Delivered |

**Silently dropped requirement:** none that matters. The SPEC's `commit-hard-errors-push-warns` decision says a push failure warns and continues — the shipped seam does exactly that for *every* push error, which is what the SPEC asked for. It is `pushanchored.go`'s own doc comment that invented a narrower contract (F1).

**Shipped beyond scope:** the diff also narrows `Fabric.Pull`'s weft arm to non-fatal and rewrites `cleanup.go`/`destroy.go` around the `raddleFoldedBack` removal. Both are direct consequences of the redesign (a warned-past status push makes a locally diverged weft routine; a merge that never resets the weft makes `ownedWeftCheckout` dead) and are documented in place. In scope by consequence, not over-reach.

## Code findings

### F1 — MEDIUM — CONFIRMED — `internal/fabricengine/pushanchored.go:31-35`

`PushAnchored`'s doc comment states, and emphasises as "load-bearing, not incidental":

> the loom-side per-transition closure this function was added for matches exactly that sentinel with `errors.Is` to warn and continue on a routine rejection **while treating every other push error as fatal**

No such discrimination exists. `newCommitStatusSeam` (`internal/loomcli/wiring.go:105-110`) warns and returns `nil` for **every** push error:

```go
if err := deps.Push(); err != nil {
    logger.Warn("loomcli: status push failed, next transition will catch up", ...)
    return nil
}
```

`grep -rn "errors.Is" internal/ | grep -i pushrejected` finds consumers only in `coalesce.go`, `landingshed/publish.go`, and the two test files — never in `loomcli`.

**Failure scenario:** a reader (or a future refactor) trusts the stated contract, wraps the sentinel somewhere in the chain, and believes the loom-side behaviour changes. It does not — because the discrimination the comment describes was never written. The comment describes a safety property nothing enforces.

**Live proof** (`s3_commitstatus.sh` case (c), weft remote URL pointed at a non-existent repo):

```
WARN push-failed b/running: rejected=false err=gitrepo: git push: ... does not appear to be a git repository
RUN outcome="done" halted="c" err=<nil>
```

A non-rejection push error warns and the run completes — identical to the rejection case (case (b): `rejected=true`, run completes).

**Fix:** correct the doc comment to describe what the consumer actually does — the sentinel stays unwrapped so a consumer *can* discriminate, and `pushanchored_integration_test.go` pins that property, but the loom-side closure deliberately does not discriminate (per the SPEC's `commit-hard-errors-push-warns` decision). Do not change the seam's behaviour: blanket warn-and-continue is what the SPEC decided and what an offline laptop needs.

### F2 — MEDIUM — CONFIRMED — `internal/loomcli/wiring.go:88-113` (the `MergeStateActive` probe-then-act window)

`newCommitStatusSeam` probes `MergeStateActive` with no lock held, then — on false — calls `Commit`, which is `CommitAnchoredPaths` → `CommitWeftPaths` → `gitrepo.StageAndCommit`. `StageAndCommit` runs `git add -- <path>` *before* it runs `git commit -m … -- <path>`.

If a `MERGE_HEAD` lands in the weft inside that window, two things happen, neither of them clean:

1. `git commit -- <path>` is a **partial** commit, which git refuses mid-merge (`fatal: cannot do a partial commit during a merge`). The seam's disposition 1 (commit-hard-errors) fires and **halts the whole loom run** — in exactly the situation disposition 3 (skip-while-mid-merge) exists to handle gracefully.
2. The preceding `git add` **succeeds**, staging `_lyx/loom/status.json` into the foreign merge's index. When the operator concludes their own merge, the status file rides along in their merge commit — content no side of that merge brought in.

**Live proof** (`s4d_toctou.sh`; harness sleeps 3000 ms between the probe and the commit, a background saboteur runs `git merge --no-commit --no-ff` in the weft at t=1.0 s):

```
[saboteur] Automatic merge went well; stopped before committing as requested
[saboteur] MERGE_HEAD: 679b67a3dfb9261d375ae59a462b3e9b1aa0feaf
RUN outcome="" halted="" err=gitrepo: git commit: git commit -m "loom: b -> running" -- _lyx/loom/status.json: exit 128: fatal: cannot do a partial commit during a merge.
  HARD-ERROR commit b/running: ...
=== index contamination check ===
A  _lyx/loom/status.json
A  other.txt
--- concluding the operator merge as they would ---
6247b63 Merge branch 'sidebranch-weft' into task-weft
 _lyx/loom/status.json | 19 +++++++++++++++++++
 other.txt             |  1 +
 2 files changed, 20 insertions(+)
```

The good news the same run establishes: the concurrent merge is **not** corrupted in the sense the high-yield list feared — `MERGE_HEAD` stays live, the weft HEAD does not move, no conflict resolution is interleaved or destroyed, and the status-file write itself is already durable on disk (`shed.md`'s ordering claim holds). The damage is bounded to a run halt plus one extra staged path.

**Fix:** on a `Commit` failure, **re-probe** `MergeActive`; if it now reports true (or errors), take the skip disposition — warn and return `nil` — instead of hard-erroring. That converts a lost race into the skip it was always meant to be and stops a raw-git merge session from killing an autonomous run. The residual `git add` staging cannot be closed from this side (the raw-git operator takes no lock fabric could contend on) and must be stated in the seam's doc comment rather than papered over.

### F3 — MEDIUM — CONFIRMED — `internal/fabricengine/doc.go:876-880`

> Two consequences follow directly: `unifyConflictPaths`' weft conflict list is **permanently empty, never populated** …

False. `MergeIn` does pass `nil` (`merge.go:260`), but `MergeContinue` reaches the same function through `unifiedRemainingConflicts` with a **real** `f.weft.ConflictedFiles()` (`mergelifecycle.go:304-312`, `:209`). A weft-side conflicted index therefore populates that arm and surfaces a weft path in the caller's `unresolved` list.

**Live proof** (`s6_weftconflict.sh`): warp-side conflict from `lyx fabric merge-in task`, then a raw-git conflicting merge in the weft on `_lyx/loom/status.json`, then resolve the warp side and `lyx fabric merge --continue`:

```
{"error":"fabricengine: merge preconditions failed: unresolved conflicts remain","mutations":[],"ok":false,"partial":false,"unresolved":["_lyx/loom/status.json"]}
```

**Fix:** docs only. The behaviour is right — reporting the hidden weft path is precisely what `unifiedRemainingConflicts` exists for, and removing the weft read would let a legacy-record resume (`WeftOutcome: "staged"` from a pre-change binary, which `concludeMergeSides`' retained weft arm still handles) conclude a weft merge over unresolved conflicts. Restate the claim as scoped to the two merge verbs' own conflict results.

### F4 — MEDIUM — CONFIRMED — `internal/fabricengine/doc.go:1042-1045`

> … now evaluates the warp side alone throughout; **the weft has lost its power to block a merge**, on top of having already lost its participation in one.

False as written, and the same file says so 150 lines earlier (`mergestate.go:268-271` records that *both weft probes are deliberately kept*). Two weft states still block a merge verb outright:

- A weft `MERGE_HEAD` refuses `MergeIn`/`Merge` through the retained `foreignMergeStatePresent`.
- A weft conflicted index refuses `MergeContinue` (F3).

**Live proof** (`s1c_foreign.sh`, weft simultaneously dirty **and** detached **and** carrying a live `MERGE_HEAD`):

```
{"error":"fabricengine: git merge state exists that fabric did not start; conclude or abort it with plain git, then retry","mutations":[],"ok":false,"partial":false}
weft HEAD unchanged: yes
weft MERGE_HEAD still there: yes
```

The refusal is the *foreign-state* one, not a guard reason naming dirty or detached — which is the correct, designed outcome. Only the sentence is wrong.

**Fix:** docs only. Qualify the sentence to the four guards it actually names (`pairDirtyReason`, `detachedHeadReason`, `syncedToUpstreamReason`, `resolveMergeSources`' refusal arm) and state the two retained weft-reading refusals explicitly, so the exception is discoverable where the claim is made.

### F5 — LOW — CONFIRMED — `internal/fabricengine/merge.go:1-7` (file header)

The file header still reads:

> MergeIn merges a source branch into the current pair's **own warp and weft checkouts** and surfaces any conflicts for resolution in that same worktree.

directly contradicting `MergeIn`'s own godoc twelve lines of code later ("merges source into f's current pair's warp checkout; the weft side is not a merge participant") and `Merge`'s. A reader opening the file meets the stale claim first.

**Fix:** rewrite the header to match the shipped contract.

### F6 — LOW — CONFIRMED — `internal/fabricengine/pull.go:132-138` (`ErrWarpDirty`)

> The weft side **has already been fast-forwarded** when this is returned; warp is untouched.

No longer guaranteed. The same diff made the weft arm non-fatal, so `ErrWarpDirty` is reachable with `WeftPulled` false — a failed upstream probe or a failed `git pull --ff-only` warns and falls through to exactly the dirty-warp check that returns this sentinel. Every neighbouring doc (`PullResult.WeftPulled`, `PartialPullError`, the `Pull` godoc, `weft_verbs.go`'s `pullResultMap`) was updated for the non-fatal arm; this one was missed.

**Fix:** restate as "the weft arm has already run and may or may not have advanced; warp is untouched".

### F7 — LOW — CONFIRMED — `internal/fabricengine/cleanup.go:96` and `internal/fabriccli/fabric.go:370`

`Topology.Cleanup(l *lyxcwd.Location, apply, force bool)` no longer reads `force` anywhere in its body — the `raddleFoldedBack` gate was its only consumer. `lyx fabric cleanup --force` is therefore a user-facing flag with no effect whatsoever, and nothing in the test suite pins that it *stays* inert.

**Live proof** (`s5_cleanup.sh` — one orphan weft branch, the checked-out primary, and an unmanaged `legacy-notes`):

```
### --apply alone (force=false)
{"entries":[{"branch":"legacy-notes","protected":true},{"branch":"main","protected":true},{"branch":"orphan-weft","deleted":true}], ...}
### --apply --force (should be identical: force answers no gate)
{"entries":[{"branch":"legacy-notes","protected":true},{"branch":"main","protected":true}], ...}
```

The three carve-outs the removal was supposed to preserve all hold — orphan deletable under `--apply` alone, primary weft branch untouched, unmanaged branch protected — so the removal of the dedicated pinning test left a redundant-coverage gap, not a behaviour gap. What it *did* leave is an unpinned reserved flag.

**Fix:** keep the flag and the parameter (removing either churns the CLI help tree and the verb table for no behavioural gain), but pin the reserved-ness with a test asserting `--apply` and `--apply --force` produce identical verdicts, so a future accidental re-wiring of `force` is caught rather than shipped.

### F8 — LOW — coverage gap — `internal/loomcli`

`wiring_commitstatus_test.go` and `run_commitstatus_test.go` are both Tier 1 stub-closure tests. Nothing anywhere wires `loomCommitStatusDeps` against a real fabric pair, so the composed seam — real `MergeStateActive`, real `CommitAnchoredPaths`, real `PushAnchored`, real `Shed.Run` — is unexercised. F2 is exactly the class of defect only that composition surfaces: every stub test passes today, and the partial-commit interaction is invisible to all of them.

I closed the gap for this review with a scratch harness outside the repo (see "What was tested"), which is not a durable substitute.

**Fix:** add an `//go:build integration` test in `internal/loomcli` driving the real seam over a `hubforge` pair through all three dispositions.

### F9 — LOW — coverage gap — `internal/fabricengine/weftguards_integration_test.go`

The narrowed guards are pinned one at a time (`…DirtyWeftDoesNotRefuse…`, `…DetachedWeftDoesNotRefuse…`). No row drives them **in combination**, which is where a per-guard narrowing could still leave an aggregate refusal: the guard stage appends every reason before deciding, so a single retained weft conjunct in any one helper would only be visible when the others are also unhappy.

I drove the combination live and it is correct (`s1_combined_weft_sabotage.sh`, `s1c_foreign.sh`), but nothing in the repo holds it.

**Fix:** add a combined-state row — weft dirty **and** detached at once, warp clean — asserting the merge completes and the weft is byte-identical afterwards.

### F10 — NIT — CONFIRMED — `internal/fabricengine/merge.go:233-239` and `:453-459`

Both verbs write the merge-state record twice back to back:

```go
if err := f.saveMergeState(st); err != nil { … }
st.WeftOutcome = mergeOutcomeAlreadyUpToDate
if err := f.saveMergeState(st); err != nil { … }
```

Nothing happens between them. Setting `WeftOutcome` on the struct literal (or before the first save) makes one write do the same work. The intermediate state is not merely redundant, it is strictly worse: a record with an empty `WeftOutcome` is what `mergeAttemptIncompleteReason` refuses a `MergeContinue` resume on, so the extra write opens a small crash window whose only outcome is an unresumable record — for a state the code deliberately never wants to be in.

**Fix:** collapse to a single `saveMergeState` with `WeftOutcome` pre-filled.

### F11 — NIT — CONFIRMED — `internal/loomcli/wiring.go:70-84`

`newCommitStatusSeam`'s doc says it implements "three dispositions **in this exact order**: 1. commit-hard-errors … 2. push-warns … 3. skip-while-mid-merge". The code's actual order is skip-while-mid-merge, then commit, then push. Either the enumeration or the "in this exact order" claim has to go.

**Fix:** reorder the enumeration to match execution.

### F12 — NIT — CONFIRMED — `internal/fabricengine/weftgit.go:265-268`

`PushResult`'s doc comment claims "All three push entry points — `PushWeft`, `PushWarpAt`, and `CoalescePushBothAt` — return this same type". `PushWarpRebaseFreeAt` already broke the count before this diff; `PushAnchored` breaks it again. An enumeration that has to be maintained per entry point is the wrong shape for a doc comment.

**Fix:** replace the enumeration with the invariant it exists to state — every push entry point returns this type — so it stops going stale.

### F13 — NIT — `manifest/designs/loom.md:277`

> A status commit is skipped outright while the weft is mid-merge, logged at warn.

Accurate but incomplete: the seam also skips when the *probe itself* fails, which is a deliberate decision (`wiring.go:82-84`: "an unreadable probe is the same 'git state cannot be trusted right now' category"), not an implementation detail. A reader auditing why a transition produced no commit has two causes to consider, not one.

**Fix:** name the probe-failure skip alongside the mid-merge skip.

## Docs & operability findings

Consolidated above: F1, F3, F4, F5, F6, F11, F12, F13 are all doc-accuracy findings, and three of them (F1, F3, F4) are on *load-bearing* claims — sentences a reader would reasonably rely on to decide whether a change is safe.

Operability observations that are **not** findings:

- The commit-hard-errors disposition halting `Run` leaves the status file already written and durable, so a resumed run picks up from the correct on-disk state. Verified live (`s4_toctou.sh` case (a), stale `index.lock` planted in the weft gitdir): the run halted, and `status.json` on disk already read `current_producer: "b", state: "running"` with the `a → done` history entry present. `shed.md`'s ordering claim holds exactly as written.
- A broken weft remote produces one multi-line `fatal:` blob per transition in the run log, for the whole run, by design. Noisy but correct, and self-healing per the SPEC.
- No stray state survives a driven merge/abort cycle: `fabric-merge.json` is deleted, and the only `.lock` files left are `internal/lock`'s own persistent lock files (`weft.write.lock`, `origin.json.lock`, `board.lock`, `.gitrepo-push.lock`), which are expected residue, not leaks.

## What was tested

All commands run from `/home/knatte/Code/loomyard/wts/fabric-crucible-hardening` unless noted. Scratch hubs live under the session scratchpad and are torn down with it; nothing was written outside it and the repo worktree.

### Hermetic

```sh
go build ./...                                                    # clean
go vet ./internal/fabricengine/... ./internal/fabriccli/... \
       ./internal/loomcli/... ./internal/shedengine/... \
       ./internal/landingshed/... ./internal/loomrecipe/... ./cmd/lyx/...   # clean
go test -count=5 ./internal/fabricengine/... ./internal/fabriccli/... \
       ./internal/loomcli/... ./internal/shedengine/... \
       ./internal/landingshed/... ./internal/loomrecipe/... ./cmd/lyx/...   # exit 0, 0 FAIL
```

### Live integration (real git substrate, `integration` build tag)

```sh
go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/... \
        ./internal/landingshed/... -count=1                        # exit 0, no FAIL, no race marker
```

### Live driving — real `lyx` binary against scratch hubs

The dev deploy (`./deploy-dev`) was refused by the environment's command classifier, so the current source was built to a scratchpad binary instead — `go build -o <scratch>/bin/lyx ./cmd/lyx` — and every scenario drove that binary directly, foreground, one command at a time. Hubs were bootstrapped with real `lyx fabric clone` against bare local origins (`mkhub.sh`), i.e. through the product's own wiring path rather than by hand.

| Scenario | Command / script | Observed |
| --- | --- | --- |
| Weft dirty **and** detached during a warp-only `merge-in` | `s1_combined_weft_sabotage.sh` | `ok:true`, `merge_staged` on warp only; weft HEAD `613eb87` unchanged, still dirty, still detached |
| Same, but a genuine non-FF merge | `s1b_foreign.sh` | `committed:true`, `merge_committed` on warp only; weft untouched |
| Weft dirty **and** detached **and** live `MERGE_HEAD`, `merge-in` | `s1c_foreign.sh` | Refused with the **foreign-state** error (not a dirty/detached guard reason); nothing mutated; `MERGE_HEAD` left intact |
| Same fixture, `Merge` verb from the task pair | `s1c_foreign.sh` | `committed:true` on the task pair's warp; unaffected by the other pair's weft state |
| Warp conflict → weft commits land mid-attempt → `merge --abort` | `s2_abort.sh` | Abort reset warp only; the weft commit landed *during* the attempt survived (`6` commits, HEAD unchanged); no `fabric-merge.json` residue |
| CommitStatus seam, ordinary path, real pair | `s3_commitstatus.sh` (0) | Three transitions → three real weft commits (`loom: b -> running`, `loom: c -> running`, `loom: c -> done`), `unpushed: 0` |
| Push failure that **is** `ErrPushRejected` | `s3_commitstatus.sh` (b) | `rejected=true`, WARN per transition, run completed `done` |
| Push failure that is **not** `ErrPushRejected` (bad remote URL) | `s3_commitstatus.sh` (c) | `rejected=false`, WARN per transition, run completed `done` — **F1** |
| Live weft `MERGE_HEAD` throughout a run | `s3_commitstatus.sh` (d) | All three transitions skipped with a WARN; weft HEAD unchanged |
| Real commit failure (stale `index.lock` in the weft gitdir) | `s4_toctou.sh` (a) | Run halted; `status.json` already durable on disk with the correct transition and history |
| `MERGE_HEAD` planted **inside** the probe→commit window | `s4c/s4d_toctou.sh` | Run halted on `fatal: cannot do a partial commit during a merge`; `status.json` **staged into the foreign merge's index** and carried into the operator's own merge commit — **F2** |
| `cleanup` dry-run / `--apply` / `--apply --force` | `s5_cleanup.sh` | Orphan deleted under `--apply` alone; primary weft branch and unmanaged `legacy-notes` protected in every mode; `--force` changed nothing — **F7** |
| Weft conflicted index + `merge --continue` | `s6_weftconflict.sh` | `"unresolved":["_lyx/loom/status.json"]` — a weft path through `unifyConflictPaths` — **F3**, **F4** |
| `Pull` with the weft locally diverged from its own origin | `s7_pull.sh` | WARN + `weft_pulled:false` inside `ok:true`; warp advanced to the fetched tip; local weft commit preserved |

The CommitStatus scenarios were driven through a **scratch harness outside the repo** (`<scratch>/harness`, its own Go module with a `replace` onto this worktree) that reproduces `newCommitStatusSeam`'s disposition order verbatim against the real `fabricengine.MergeStateActive` / `CommitAnchoredPaths` / `PushAnchored` and a real `shedengine.Shed.Run`. No repo file was created or edited during Job 1.

### What I could NOT verify

- **`lyx loom run` end to end.** LLM-driving, explicitly out of scope per the cost declaration. The harness above is the narrowest real substitute and covers `Shed.Run`'s hook, but not loom's own producer set.
- **Windows behaviour.** Linux only; `weftPathVisible`'s separator conversion and the junction/symlink split are untested here.
- **N×-concurrent integration runs.** Single `-count=1` pass over the integration suites plus `-count=5` hermetic; no concurrency amplifier was run, and the merge bar does not require one.
- **The `git add` contamination in F2 under a *conflicted* (rather than clean) foreign weft merge.** I proved it with a cleanly-merging foreign merge; a conflicted one would refuse the `git add` differently and was not driven.

## Teardown

No tmux or LLM substrate involved. After the driving: no `fabric-merge.json` survives in any scratch hub, the only `.lock` files present are `internal/lock`'s expected persistent locks, and every hub, origin, harness and binary lives under the session scratchpad — nothing was created in the repo worktree, `/tmp` proper, or the user's home.
