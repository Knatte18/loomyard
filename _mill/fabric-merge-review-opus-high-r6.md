# fabric merge surface — independent review (round 6, tag `opus-high-r6`)

Reviewer: Opus, high effort. Clean-room: findings below were formed before reading any prior-round `_mill/fabric-merge-review-*` material.
Worktree: `/home/knatte/Code/loomyard/wts/fabric-merge-crucible-round4`, branch `fabric-merge-crucible-round4`.

## What was tested

Appended as each command/scenario returned.

### Baseline gates (before any edit)

- `go build ./...` — clean.
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — clean.
- `go test -count=5 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...` — all ok (fabricengine 0.355s, fabriccli 0.004s, gitrepo 0.005s, cmd/lyx 0.963s).
- `go test -tags integration -count=1 -timeout 30m ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — all ok (fabricengine 31.4s, fabriccli 2.6s, gitrepo 1.6s).

### Sabotage sweep — round 5's and round 4's own mechanisms (each applied alone, suite re-run, then reverted)

Harness: apply one production-source edit, run `go test -tags integration -count=1 ./internal/fabricengine/...`, `git checkout --` the file.
"GREEN" = the whole integration suite still passed with the mechanism removed, i.e. no test can detect that clause's loss.

| # | Sabotage | Result |
|---|---|---|
| S1 | `MergeIn`: delete the under-lock `weftStart` re-read (keep warp's) | **GREEN — uncovered** |
| S2 | `MergeIn`: delete the post-lock record re-check | red (`TestMergeIn_RecordAppearingWhileWaitingForLock_...`) |
| S3 | `Merge`: delete the post-lock record re-check | red (`TestMerge_RecordAppearingWhileWaitingForLock_...`) |
| S4 | `MergeAbort`: move record load + conclude-landed guard back ahead of the lock | red (`TestMergeAbort_ConcludeLandingWhileWaitingForLock_...`) |
| S5 | `MergeContinue`: move record load back ahead of the lock | red (`TestMergeContinue_RecordRetiredWhileWaitingForLock_...`) |
| S6 | `sideConcludeAlreadyLanded`: drop the live-`MERGE_HEAD` refusal | **GREEN — uncovered** |
| S7 | `sideConcludeAlreadyLanded`: drop the `squash` refusal | **GREEN — uncovered** (the squash test passes on `len(parents) < 2` instead) |
| S8 | `sideConcludeAlreadyLanded`: drop the `sourceSHA == ""` refusal | GREEN, but behaviourally a no-op (no real parent SHA equals `""`), so nothing to cover |
| S9 | `sideConcludeAlreadyLanded`: drop `parents[0] != start` | red (`TestMergeContinue_MergeOfSourceOntoWrongBase_...`) |
| S10 | `sideConcludeAlreadyLanded`: drop the source-membership loop | red (`TestMergeContinue_MergeOfWrongSourceOntoStart_...`) |
| S11 | `sideConcludeAlreadyLanded`: drop the `head == start` early return | GREEN, but behaviourally a no-op (`parents[0] != start` refuses the same states) |
| S12 | `sideConcludeMayHaveLanded`: drop the `committed != ""` clause | **GREEN — uncovered** |
| S13 | `sideConcludeMayHaveLanded`: drop the HEAD-moved clause | red (2 tests) |
| S14 | `sideConcludeMayHaveLanded`: drop the outcome filter | red (`TestMergeIn_OneSideFastForwardsOtherConflicts_...`) |
| S15 | `detachedHeadReason`: drop the refusal | red (`TestMergeCrucible_DetachedHeadRefused`) |
| S16 | `resolveMergeSources`: drop the fabric-managed refusal | red (`TestMergeIn_NotFabricManaged_NothingMutated`) |

### Non-`-z` git-output-parsing audit (round 5 F1's family)

Every `--name-only` / `--porcelain` / `ls-files` site reachable from the merge surface:
`gitrepo/merge.go:155` (`ConflictedFiles`, has `-z`), `fabricengine/dirtiness.go:57` (`status --porcelain`, emptiness-only — quoting cannot affect the boolean),
`gitrepo/gitrepo.go:148` (`ls-files --cached`, emptiness-only).
`status.go:178`, `pull.go:435`, `warpprobe.go:128`, `weftgit.go:163`, `worktreelist.go:28` are outside the merge surface.
**No second instance of the F1 class exists in the merge surface.**

### Live driving (real hub, `.dev-bin/lyx`, no launcher)

Lab hub built by hand at `<scratch>/lab/h1`: `GIT_CONFIG_GLOBAL` with `init.defaultBranch = main`, `git init --bare` warp + weft (weft empty — the documented bootstrap shape), seeded warp `main`, `lyx fabric clone <weft-bare> <warp-bare>` from an empty work dir. Prime pair `warp`/`warp-weft` on `main`/`main-weft`.

- `lyx fabric add task1` → pair created, `task1`/`task1-weft`.
- Divergence on BOTH sides (warp `README.md` via plain git, weft `_lyx/README.md` via `lyx fabric commit`), on the pair and on the prime, so the merge is a real merge.
- `lyx fabric merge-in task1` from the prime → exit 1, `ok:false`, `conflicts:["README.md","_lyx/README.md"]` — one flat, worktree-relative, side-free list; `mutations` carries two `merge_staged` entries; `partial:false`. Correct.
- Conflict markers on both sides carry the **SHA**, never a branch: warp `>>>>>>> eef5fd82…`, weft `>>>>>>> b91a3da4…`. No `-weft` leak.
- Sibling refusals while the record is live: `commit`, `pull`, `push`, `sync`, `checkout task1`, `remove task1` all returned the single fixed `a merge is in progress; run MergeContinue or MergeAbort first`, `mutations: []`.
- `lyx fabric merge --continue -m …` with conflicts still unresolved → `merge preconditions failed: unresolved conflicts remain`, nothing mutated.
- Resolved both sides, `git add` each, `lyx fabric merge --continue -m "merged task1 into main"` → `ok:true`, `committed:true`, `already_up_to_date:false`, one `merge_committed` per side; both checkouts clean, no merge in progress, both logs carry the named conclude commit.

### Live driving, continued

- **Precondition matrix** (hub `h3`/`h4`, each refusal `mutations: []`, both HEADs unmoved):
  dirty warp → `worktree dirty`; dirty weft (tracked file — my first attempt used an untracked one and silently *passed the merge*, a fixture degradation I caught and redid) → `worktree dirty`;
  detached warp HEAD and detached weft HEAD → `checkout is not on a branch` (byte-identical either side);
  nonexistent source → `source branch is not fabric-managed; source branch not found`;
  `HEAD`, a raw SHA, and a tag → `source branch is not fabric-managed`;
  a `--upload-pack=evil`-shaped source → refused at the guard, no git spawn takes it as a ref.
- **Hostile git config** (hub `h4`): `merge.ff = only` did NOT break a non-fast-forward merge (fabric pins `--ff`);
  `core.editor = "sleep 600"` did NOT hang `merge --continue` — it returned in well under the 60s `timeout` (fabric pins `--no-edit`). No hang.
- **Flag pre-flight** (hub `h4`): `merge --abort --squash` → `usage: --squash cannot be combined with --continue or --abort`; `merge --abort -m msg` → `usage: -m cannot be combined with --abort`;
  `merge --continue --abort` → cobra's mutually-exclusive error; `merge --continue x` / `merge --abort x` → `takes no positional arguments`; bare `merge` → the one-branch usage error. All correct, none silently ignored.
- **Foreign merge state** (hub `h4`), both shapes — a conflicted plain `git merge` (MERGE_HEAD live) and a conflicted `git merge --squash` (no MERGE_HEAD):
  `merge-in`, `merge --continue`, `merge --abort` and `commit` all returned `git merge state exists that fabric did not start…`, and MERGE_HEAD + `git status` were byte-identical before and after. `pull` refused earlier, on its own dirty-warp guard.
- **Empty-result merge** (hub `h5`, F20): the same change made independently on both branches → `committed: true`, `already_up_to_date: false`, and **no** `MERGE_HEAD` left in either checkout; the follow-up `merge --abort` answered `no merge in progress`, not foreign state. Correct.
- **Squash companion** (hub `h5`): `merge dup2 --squash` over the same fixture → `already_up_to_date: true`, `committed: false`, no MERGE_HEAD. Correct.
- **Half-concluded abort** (hub `h5`, failing `pre-commit` hook in the real weft common gitdir — my first two attempts wrote the hook into the wrong gitdir and into a `core.hooksPath`-disabled config, both caught and redone):
  `merge-in` → `merge conclude did not finish; run MergeContinue again`, `partial: true`, record retained with `warp_committed` set and `weft_committed` empty;
  `merge --abort` → **refused** with `merge preconditions failed: merge conclude already landed`, warp's conclude commit untouched;
  hook removed, `merge --continue` → `committed: true` with a single `merge_committed` for the weft side only, warp HEAD unmoved. Exactly as documented.
- **Non-ASCII conflict paths** (hub `h6`): a conflict on `ä-warp.md` (warp) and `_lyx/ä-nöte.md` (weft) simultaneously → `conflicts:["_lyx/ä-nöte.md","ä-warp.md"]`, raw bytes, never git's C-quoted form, never "outside the fabric-managed tree". Round 5's F1 has not regressed.
- **`mergeSourceInFlight`** (hub `h6`): with the prime pair mid-merge on `na`, `lyx fabric remove na` refused from the `na` worktree AND from the prime worktree.
- **Multi-segment subpath anchor** (hub `h7`, `--subpath apps/backend`): clone records `apps/backend` verbatim in `.lyx-anchor`, the junction lands at `warp/apps/backend/_lyx`, and a weft-side conflict maps to `apps/backend/_lyx/base.txt`. Correct on Linux — see F6 for what this same fixture does on Windows.

## Findings

Nine findings: 3 MEDIUM, 3 LOW, 3 NIT. Two carry a behavioural defect (F1, F6); four are proof-quality gaps proven by sabotage (F2–F5); three are doc/contract accuracy (F7–F9).

---

### F1 — `sideConcludeAlreadyLanded` adopts an octopus merge fabric could never have produced — MEDIUM — CONFIRMED (reproduced live)

`internal/fabricengine/mergelifecycle.go:158`

```go
if len(parents) < 2 || parents[0] != start {
	return "", false, nil
}
for _, parent := range parents[1:] { ... }
```

`len(parents) < 2` is a **lower** bound, and the source-membership loop scans *all* remaining parents. `doc.go:906-910` and this function's own godoc both claim the opposite: "the resulting conclude-commit has **exactly two** parents", "a genuine conclude-commit is a merge commit whose first parent is the recorded pre-merge SHA and one of whose remaining parents is that recorded source SHA, exactly. **Nothing short of all of that is adopted.**"

**Failure scenario, driven end to end on the real substrate (lab hub `h2`).**
A `merge-in` where warp merges cleanly (`warp_outcome: staged`) and weft conflicts, so the record survives with `warp_committed: ""`.
The operator then discards the staged warp merge with `git merge --abort` and merges the recorded source *together with an unrelated branch* in one go — `git merge <warp_source> <other>` — a perfectly ordinary thing to do, and git builds a genuine octopus:

```
HEAD    4a8db3d
parents 4b096c23 (== warp_start)  ec8e56df (== warp_source)  367927eb (unrelated)
```

`lyx fabric merge --continue` then answered:

```json
{"already_up_to_date":false,"committed":true,
 "mutations":[{"kind":"merge_committed","target":"warp","detail":"4a8db3d6…"}, …],"ok":true,"partial":false}
```

Record deleted, correspondence recorded, and `from-other.txt` — content this merge never brought in, that the paired weft side never saw, and that no `merge_staged` entry mentions — is now on `main`, accounted for as this merge's own conclude.

This is the same defect class round 4 closed at a different arity: a **positive claim** resting on evidence that does not discriminate. The adoption arm exists to be exact, and "at least two parents, source somewhere among them" is not exact.

**Suggested fix.** Require exactly what fabric produces: `len(parents) != 2 || parents[0] != start || parents[1] != sourceSHA` ⇒ refuse. This rejects nothing fabric can create and nothing doc.go's plain-git recovery route creates (both `git commit --no-edit` over a live MERGE_HEAD and `reset --hard <start>` + `git merge <source>` yield exactly two parents in that order). Update the godoc and `doc.go` so the stated rule and the code are the same rule.

---

### F2 — `MergeIn`'s under-lock **weft** start re-read has zero coverage — MEDIUM — CONFIRMED (sabotage S1)

`internal/fabricengine/merge.go:165-172`, test `internal/fabricengine/mergelock_integration_test.go:233`

Round 5's F2 re-reads BOTH pre-merge starts under the write lock. `TestMergeIn_StartsAreReReadUnderLock` lands a concurrent commit on the **warp** side only and asserts **`st.WarpStart`** only. Deleting the weft re-read entirely:

```go
	weftStart, err = f.weft.CurrentSHA()   // deleted
	if err != nil { … }
```

left `go test -tags integration ./internal/fabricengine/...` **fully green**. Half of the fix that finding shipped is unguarded, and the failure it protects against is the destructive one: a stale `WeftStart` means a later `MergeAbort` resets the weft checkout *through* a commit the lock's previous holder landed.

**Suggested fix.** Extend the test to land a concurrent commit on the weft side too (via `commitOnCurrentBranch` in `h.PrimeWeft()`) and assert `st.WeftStart` equals it, so each side's re-read is independently proven.

---

### F3 — the squash-refusal test passes for the wrong reason — MEDIUM — CONFIRMED (sabotage S7)

`internal/fabricengine/mergelifecycle.go:150`, test `internal/fabricengine/mergein_recovery_integration_test.go:921`

Deleting `squash ||` from `if squash || sourceSHA == ""` left the suite **green**, including `TestMergeContinue_SquashConcludeLandedByHand_IsNeverAdopted`, the test written to pin that very clause. The reason is that the test's hand-landed squash conclude is an **ordinary one-parent commit**, so `len(parents) < 2` refuses it on its own — the squash clause is never the thing under test.

The clause is genuinely load-bearing for a shape the test does not build: a **squash** record whose side carries a real two-parent merge of the recorded source on the recorded start. That happens when a squash conclude fails, the operator `reset --hard`s back to the start (a squash leaves no MERGE_HEAD, so `git merge --abort` is unavailable) and finishes with a plain `git merge <source>` instead. Without the clause, fabric would adopt a *merge* commit as a *squash* record's conclude and record correspondence for a pair whose two sides then carry structurally different history.

This is round 5's own F7 shape found in round 5's own work: a fresh test whose fixture never reaches the clause it names.

**Suggested fix.** Add a test building exactly that shape — squash record, side reset to the recorded start, plain two-parent `git merge <recorded source>` landed by hand — and assert `*ErrMergeIncomplete` with the record retained and no `KindMergeCommitted`.

---

### F4 — the live-`MERGE_HEAD` refusal in the adoption arm has zero coverage — LOW — CONFIRMED (sabotage S6)

`internal/fabricengine/mergelifecycle.go:143-149`

Replacing the `if mergeHeadPresent { return "", false, nil }` arm with a no-op left the suite **green**. No test reaches a state where HEAD has moved off the recorded start *and* a MERGE_HEAD is live.

That state is constructible and matters: the operator hand-lands this merge's conclude (HEAD moves, MERGE_HEAD clears) and then starts a second, clean `git merge --no-commit <other>` in the same checkout before running `MergeContinue`. The index carries no conflicts, so `MergeContinue`'s guard passes and control reaches the adoption probe. With the clause, fabric refuses (`*ErrMergeIncomplete`, record retained) — correct, because adopting there would delete the record and record correspondence against a HEAD that the operator's pending second merge is about to move. Without it, fabric adopts and walks away.

**Suggested fix.** Add an integration test for that shape.

---

### F5 — `sideConcludeMayHaveLanded`'s recorded-SHA clause has zero coverage — LOW — CONFIRMED (sabotage S12)

`internal/fabricengine/mergeguards.go:282-284`

Deleting `if committed != "" { return true, nil }` left the suite **green**: in every fixture the second clause (outcome staged/conflicted ∧ HEAD moved) fires too. The first clause is the record's own authority and is the only thing that still refuses when HEAD has been put *back* to the recorded start after a conclude landed — e.g. a half-concluded attempt where the operator `reset --hard`s the landed side. `MergeAbort` must still refuse there, because the recorded conclude SHA is the evidence a commit was made and its content may be unreachable from the current tip.

**Suggested fix.** Plant a record with `WarpCommitted` set while warp HEAD sits at `WarpStart`, and assert `MergeAbort` refuses with `merge conclude already landed`.

---

### F6 — weft conflict-path visibility breaks on Windows under a multi-segment anchor — MEDIUM — PLAUSIBLE (traced end to end; Windows half not executed — Linux host)

`internal/fabricengine/mergepaths.go:39-50` (`weftPathVisible`), against `internal/lyxcwd/anchor.go:84` (`ValidateAnchorRel`)

```go
prefix := name + "/"
if anchorRel != "." {
	prefix = path.Join(anchorRel, name) + "/"
}
if strings.HasPrefix(weftPath, prefix) { … }
```

`weftPath` is git's own output and is **always** forward-slash. `anchorRel` is `l.AnchorRel`, which `ValidateAnchorRel` produces as `filepath.Clean(filepath.FromSlash(trimmed))` — an **OS-separator** path. `path` (the slash package) does not normalize separators; it only concatenates.

So on Windows, with the multi-segment anchor `apps/backend`:

- `.lyx-anchor` holds `apps/backend`; `ValidateAnchorRel` returns `apps\backend`.
- `path.Join("apps\\backend", "_lyx") + "/"` = `apps\backend/_lyx/`.
- git reports the conflict as `apps/backend/_lyx/base.txt`.
- `HasPrefix` fails ⇒ `weftPathVisible` false ⇒ `unmappable` true ⇒ `MergeIn` self-aborts **the entire merge** on both sides with `*ErrUnmergeableState` ("operator intervention required"), for a conflict that is perfectly mappable.

Every weft-side conflict under a multi-segment anchor is affected; single-segment anchors and `"."` are not, because they carry no separator to differ. I proved the configuration is reachable and correct on Linux (lab hub `h7`: `--subpath apps/backend` clones, junctions at `warp/apps/backend/_lyx`, and a weft conflict maps to `apps/backend/_lyx/base.txt`).

`unifyConflictPaths`'s godoc states the reverse of what the code does: "anchorRel is normalized with `path.Join` semantics, never filepath's OS-dependent one." `path.Join` is not a normalizer of an already-OS-separated input.

**Suggested fix.** Convert once at the geometry seam (`resolveMergeGeometry`) with `filepath.ToSlash`, which is **identity when `filepath.Separator == '/'`** and therefore provably cannot change Linux behaviour — including the case of a Linux directory whose name legitimately contains a backslash, which a naive `strings.ReplaceAll` would corrupt. Correct the godoc to describe the conversion that actually happens.

Honest scope statement: the Windows half remains **unexecuted**, on this round as on all five before it. What is new here is that the mechanism is now traced to two exact lines rather than carried as "Windows untested".

---

### F7 — `MergeStageResolved` is the one mutating merge verb that will write into foreign merge state — LOW — CONFIRMED

`internal/fabricengine/mergestage.go:29-35`, against `internal/fabricengine/doc.go:861-863`

`doc.go` states of foreign git merge state: "**every mutating merge verb refuses rather than touch it**, while `MergeInProgress` — a read-only probe — reports `false` for it".
`MergeStageResolved` is a mutating merge verb (it returns `StageResult`, embeds `MutationRecord`, and runs `git add -A`) and has no foreign-state arm.

Its own godoc justifies having no guard and no lock with: "With no merge in progress both sides' `ConflictedFiles()` are empty, so every path fails the partition above and the call errors before staging anything — there is no state it can reach where it writes outside a merge."

That premise is false exactly in the foreign case, and I proved the state live (hub `h4`): with a plain-git conflicted merge in the warp checkout, `MergeInProgress()` is false while `git status --porcelain` reports `UU README.md`, so `ConflictedFiles()` is non-empty. A caller handing that path to `MergeStageResolved` stages into the operator's own merge index, with no record, no lock, and no refusal — the one thing the foreign-state rule exists to prevent.

The shipped caller (`internal/mergeresolve`) gates on `MergeInProgress()` first, so this is not a live data-loss path today; it is a public engine verb whose stated invariant and whose module-level doc claim are both untrue as written.

**Suggested fix.** Add the same record-then-foreign arm `MergeIn`/`Merge` already use (refuse with `*ErrForeignMergeState` only when no fabric record exists), and correct the godoc's premise. Cost during a real merge is one record read.

---

### F8 — `MergeResult.Conflicts` is nil on every error return — NIT — CONFIRMED

`internal/fabricengine/merge.go:33-37` and every `return MergeResult{}, err` in `merge.go` / `mergelifecycle.go`

The type's own godoc and the SPEC both say `Conflicts` is "empty, never nil". A prior round fixed the two success paths that were nil (`TestMergeCrucible_ConflictsIsEmptyNeverNil` pins exactly those two), but every *error* return still hands back a zero `MergeResult`, so `json.Marshal` of it yields `"conflicts":null`. `MergeResult` carries json tags, so it is a marshallable public type; the fabric CLI happens not to emit the field on error paths, which is why nothing has noticed.

**Suggested fix.** Normalize in each verb's existing named-return defer (the same place `res.Mutations` is already set), so the property holds for every return of all four verbs rather than for the two paths a test happened to name. Extend the existing test to cover an error return.

---

### F9 — the fabric sandbox suite's session-log template stops at F13 while the suite defines F0–F20 — NIT — CONFIRMED

`tools/sandbox/SANDBOX-FABRIC-SUITE.md` ("Session log format")

The template enumerates `F0:` … `F13:` only. The suite defines scenarios through **F20**, and F18/F19/F20 are the three merge-lifecycle scenarios. An agent that fills in the template as written silently omits every merge scenario from its verdict list — the suite's merge coverage exists but never reaches the report.

**Suggested fix.** Extend the template to F20.

---

## Re-examination targets from the round brief

### 1. The `parents[0] == start` adoption-evidence tradeoff — verdict: **KEEP** (independently re-derived)

Derived without reading round 5's reasoning. The clause requires the adopted commit's first parent to be exactly the recorded pre-merge SHA. Walking the flows that could be blocked:

- Hand-landing the conclude with `git commit --no-edit` while MERGE_HEAD is live — first parent *is* the recorded start. Not blocked.
- `git commit --amend` on that conclude — parents preserved. Not blocked.
- `git merge --abort` then `git merge <recorded source>` from the same start — first parent is the start. Not blocked. (Confirmed by construction while building F1's fixture.)
- Conclude landed, then further commits on top before `MergeContinue` — blocked. But adoption there would be **wrong**: fabric would record correspondence against a HEAD carrying post-conclude work the paired side never saw. doc.go already documents the reset-then-remerge route out.
- A rebased or `--no-ff`-recreated conclude — blocked, correctly, for the same reason.

No real-world recovery flow is wrongly blocked that doc.go does not already give a route out of. The clause stays.
What this round *does* change is the complementary half: F1 shows the evidence is too **loose** on parent count while being correctly tight on first parent.

### 2. Round 5's own new mechanisms — sabotaged independently, results above

Four post-lock/lock-ordering sites: `MergeAbort` (S4) and `MergeContinue` (S5) lock-ordering are proven; `MergeIn` (S2) and `Merge` (S3) record re-checks are proven; **`MergeIn`'s weft start re-read is not** (S1 ⇒ F2).
F7's two parentage tests: the first-parent clause (S9) and the source-membership clause (S10) are proven; the squash clause's test proves nothing (S7 ⇒ F3) and the MERGE_HEAD clause is unproven (S6 ⇒ F4).

## Deferred items — re-evaluated

- **Windows path behaviour in `weftPathVisible`/`unifyConflictPaths`.** Still **not executed** — Linux host, no Windows machine available, and nothing headless can substitute. Stated plainly rather than left to silence. This round does upgrade it from "untested" to a traced, line-level defect with a fix that is provably a no-op on Linux: see **F6**.
- **The four states where `MergeContinue` gets stuck** (first instalment round 2, rows 27/28/30/31). Re-confirmed unchanged. Round 5's lock-ordering change does not interact with them: it moves *when* the record is read, not what happens when `saveMergeState` fails after a conclude landed. The adoption arm continues to cover the non-squash half of that family, and F1/F3/F4 tighten it without narrowing what it recovers.
- **The post-record error-return class's 45-row per-site adjudication.** Not re-walked row by row; not required this round. F8 touches the same return sites (normalizing `Conflicts`), but changes no error value and no site's disposition.

## Out of scope, confirmed clean

- **A second instance of round 5's F1 (non-`-z` git path parsing).** Audited every candidate site in the merge surface — none exists. Table above.
- **A fifth mutation-or-guard-read-before-lock site.** `MergeIn`, `Merge`, `MergeContinue`, `MergeAbort` are the four; `MergeStageResolved` is unlocked by design and `MergeInProgress` is read-only. The only pre-lock mutation left is `resolveMergeSources`' best-effort `Fetch()`, which the code already documents as remote-tracking-only and which git serializes with its own per-ref locks. No fifth site.
