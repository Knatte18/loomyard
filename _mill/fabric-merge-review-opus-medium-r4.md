# fabric merge surface — independent review, round 4 (`opus-medium-r4`)

Reviewer: crucible round agent, medium effort, Opus.
Worktree: `/home/knatte/Code/loomyard/wts/fabric-merge-crucible-round4`, branch `fabric-merge-crucible-round4`.
Clean-room: findings below were formed without reading `_mill/fabric-merge-review-HANDOFF.md` or any prior round report.
The round prompt's own summaries of residual V1/V2 were read (they are part of the brief), and both were then independently re-confirmed — V1 by driving a real hub, V2 by reading the source.

## Scope reviewed

`internal/fabricengine/{merge,mergelifecycle,mergeerrors,mergeguards,mergestate,mergestage,mergepaths}.go`,
`internal/gitrepo/merge.go`,
`internal/fabriccli/merge_verbs.go`,
`internal/fabricengine/doc.go`'s "# The merge surface" section,
`internal/fabricengine/mergevocab_test.go`,
`CONSTRAINTS.md`.

## What was tested

Hermetic baseline, before any edit:

- `go build ./...` — clean.
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — clean.

Live substrate — a real hub built by hand, no launcher:

- `./deploy-dev` → `.dev-bin/lyx` @ `9e6d6e5c`.
- Hub recipe: `GIT_CONFIG_GLOBAL` with `[init] defaultBranch = main` exported before the first `git init`; bare warp seeded with `main`; bare weft left EMPTY (fabric's clone refuses a weft bare whose history carries neither `.lyx-anchor` nor an empty tree — a seeded weft bare is rejected, so the recipe must leave it empty); `lyx fabric clone <weft-bare> <warp-bare>` from an empty work dir.
- `lyx fabric add feat` → fabric-managed pair `feat` / `feat-weft`.
- Warp-side commit on `feat` (`7cad9bb`), weft-side commit on `feat-weft` via `lyx fabric commit` (`7857f60`).
  Note: `lyx fabric commit` takes NO `-m` — the message is the fixed string `weft sync`.
- Conflicting warp-side commit on `main` (`e4809f1`).
- `lyx fabric merge-in feat` from `warp` →
  `{"conflicts":["file.txt"],"ok":false,...}` with `merge_staged` on both sides.
  **Fixture precondition checked, per the round prompt's silent-degradation warning:** the record shows `warp_outcome: conflicted`, `weft_outcome: fast_forwarded`.
  So this fixture exercises the conclude on the WARP side only; the weft side is skipped by `concludeMergeSides`' fast-forward arm. That is sufficient for the V1 scenario (the adoption arm is per-side) but must be asserted explicitly in any test built on it, and a weft-side variant needs a weft change that genuinely conflicts or stages.
- **V1 adversarial sequence, driven live:**
  `git -C warp merge --abort`; then one unrelated warp commit (`0f68d66`, "TOTALLY UNRELATED COMMIT"); then `lyx fabric merge --continue`.
  Result:
  `{"already_up_to_date":false,"committed":true,"mutations":[{"kind":"merge_committed","target":"warp","detail":"0f68d66..."}],"ok":true,"partial":false}`
  — `ok:true`, `committed:true` naming the unrelated commit, the merge record deleted (`warp-weft/.git/fabric-merge.json` gone), and `git merge-base --is-ancestor 7cad9bb HEAD` reporting `feat` is NOT merged.
  A silent false success. **CONFIRMED.**

Teardown: all scratch hubs live under the session scratchpad, outside the repo.

## Findings

### R4-F1 — BLOCKING — CONFIRMED (reproduced live)

`internal/fabricengine/mergelifecycle.go:105-121` (`sideConcludeAlreadyLanded`), consumed at `mergelifecycle.go:44` and `:67`.

The adoption arm's predicate is "HEAD moved off the recorded pre-merge start AND no live `MERGE_HEAD`" and nothing more.
That predicate cannot distinguish this merge's own conclude-commit from ANY other commit landed on the checkout while the record was live.
`MergeContinue` therefore adopts an arbitrary commit as the conclude, writes it into `st.WarpCommitted`, records `KindMergeCommitted`, deletes the record, and returns `committed:true` — while the merge source is still un-merged and no record survives to inspect.

This is the round prompt's core lesson made concrete: `concludeLandedReason`/`sideConcludeMayHaveLanded` (`mergeguards.go:270`) read the same ambiguous "HEAD moved" signal to **refuse** a destructive abort, which is safe when ambiguous; `sideConcludeAlreadyLanded` reuses it to **claim** a successful adopt, which is not.

Failure scenario (driven, above): record live with `warp_outcome: conflicted`, operator runs plain `git merge --abort` then any unrelated commit, then `lyx fabric merge --continue` → `ok:true`, `committed:true` on the unrelated SHA, record deleted, source un-merged.

Suggested fix — require positive, discriminating evidence before adopting:

1. Persist the resolved per-side merge-source SHA in `mergeState` (new `warp_source`/`weft_source` fields), written at the same `saveMergeState` that already records `Verb`/`Source`/`*Start`. `mergeState.Source` today holds only the caller's branch NAME, which can be re-pointed between the crash and the resume, so it is not usable evidence.
2. Adopt only when ALL of: HEAD moved off `start`; no live `MERGE_HEAD`; the record is not a squash; the recorded source SHA is non-empty; `HEAD` has ≥2 parents; `parents[0] == start`; and some `parents[1:]` equals the recorded source SHA.
   `git merge --no-commit <sha>` sets `MERGE_HEAD` to exactly the SHA fabric passed, so a genuine conclude's second parent is that SHA verbatim — an exact equality check, not a heuristic.
3. Squash (`st.Squash`): a squash conclude has one parent and carries no discriminating evidence at all, so REFUSE to adopt and stay honest-but-stuck rather than silently inheriting the non-squash predicate. The re-run `git commit --no-edit` then fails on the clean tree and `*ErrMergeIncomplete` is returned with the record retained — the pre-fix, honest behaviour.
4. A pre-upgrade record (empty source SHA) also refuses to adopt, for the same reason: no evidence, so no claim.

Both directions must be proven by sabotage and re-driven live, including the squash shape.

**Audit of every other reader of the same "HEAD moved, no live MERGE_HEAD" signal**, per the round prompt:

- `sideConcludeMayHaveLanded` (`mergeguards.go:270`) — resolves the ambiguity toward REFUSING `MergeAbort`. Correct direction; it deliberately does not even consult `MERGE_HEAD`, so it over-refuses rather than under-refuses. No change.
- `foreignMergeStatePresent` (`mergestate.go:230`) — resolves toward REFUSING every mutating verb. Correct direction.
- `mergeAttemptIncompleteReason` (`mergelifecycle.go:146`) — resolves toward REFUSING `MergeContinue`. Correct direction.
- `gitrepo.MergeStart`'s `headAfter != headBefore` → `MergeFastForwarded` (`internal/gitrepo/merge.go:118`) — a positive claim, but read inside a single call that captured `headBefore` itself moments earlier, with no crash window in between. Not the same shape. No change.
- `mergeState.bothSidesAlreadyUpToDate` / `landedConcludeCommit` — positive claims, but read off recorded outcome/SHA fields written by the code that observed them, not off an ambiguous re-read. No change.

`sideConcludeAlreadyLanded` is the only claim-shaped reader of the ambiguous signal.

### R4-F2 — MEDIUM — CONFIRMED (by inspection)

`internal/fabricengine/mergevocab_test.go:50` — `parser.ParseFile(fset, "mergeerrors.go", nil, ...)`.

The closed guard-reason set's AST-backed closure test parses exactly one hardcoded filename.
A `mergeReason*` constant declared anywhere else in `package fabricengine` — `mergeguards.go` being the natural home, right beside the guard that would consume it — is invisible to `TestMergeVocabulary_GuardReasonSetMatchesConstBlock`, and therefore invisible to every vocabulary/side-free/path-free assertion those tests drive off `pinnedMergeReasons`.
The invariant "the closed guard-reason set is closed and every member is side-free" is asserted by a mechanism that cannot detect its violation outside one file.

Suggested fix: parse every non-test `.go` file in the package directory rather than one filename, AND assert that no `mergeReason*` constant is declared outside `mergeerrors.go` (the const block's own godoc claims that placement, so it should be machine-real).
Prove by sabotage: declare a `mergeReason*` in `mergeguards.go`, confirm the hermetic tier now fails where it previously stayed green.

### R4-F3 — MEDIUM — CONFIRMED (by inspection)

`internal/fabricengine/merge.go:314-319` vs `merge.go:344-352`.

`Merge`'s pre-merge sync step (`syncSideBeforeMerge` → `repo.MergeFFOnly`) mutates BOTH checkouts — including the weft checkout — **before** the weft write lock is acquired at line 348, and before any merge-state record exists (the record is written at line 365).

So during the sync window there is neither of the two mechanisms that serialize fabric's weft writes:

- the weft write lock is not yet held, and
- `mergeBlocksMutation` reports false (no record), so `Commit`/`Pull`/`Checkout`/`Remove` do not refuse.

A concurrent `lyx fabric commit`/`push`/`pull` on the same pair therefore runs its own git index/worktree mutation against the weft checkout while `git merge --ff-only` is running there. That is two uncoordinated writers on one worktree — `index.lock` contention at best, a half-applied checkout at worst.

The code comment at merge.go:312 even names the hazard's shape ("the first thing that touches either checkout") without drawing the conclusion that it must therefore be inside the lock.

Suggested fix: acquire the weft write lock **before** the sync step (immediately after the guard aggregation), and keep it for the rest of the call. The guard stage above it is read-only apart from `Fetch` (see R4-F6), so widening the hold costs nothing but the fetch/guard duration.

### R4-F4 — LOW — CONFIRMED (by inspection)

`internal/fabricengine/mergestage.go:28` (`MergeStageResolved`).

`MergeStageResolved` is a mutating verb (`git add -A` on both sides, `KindMergeResolvedStaged` recorded) that takes no weft write lock and has no merge-record precondition, and its godoc explains neither omission.

It is in fact safe today, but only for reasons stated nowhere: with no merge in progress both sides' `ConflictedFiles()` are empty so every path errors out before anything is staged, and while a merge IS in progress every sibling weft-mutating verb already refuses on the record. That is a real argument, and it is exactly the kind of "unguarded and safe for stated reasons rather than by omission" reasoning `doc.go` already writes out for the push family — it just was never written for this verb.

Suggested fix: state the reasoning in `MergeStageResolved`'s godoc and in `doc.go`'s merge-surface section, so the next reader can tell "deliberately unlocked" from "forgot the lock".

### R4-F5 — LOW — CONFIRMED (by inspection)

`internal/fabricengine/mergelifecycle.go:223`.

`MergeContinue` returns `MergeResult{Conflicts: mergeNoConflicts, Committed: st.landedConcludeCommit()}` and never derives `AlreadyUpToDate` from `st.bothSidesAlreadyUpToDate()`, unlike `MergeIn` (merge.go:236) and `Merge` (merge.go:430).

`MergeResult`'s own godoc (merge.go:38-42) says "**Both** flags are derived from the merge-state record's own fields … never hardcoded per return site". For `MergeContinue` the second flag IS hardcoded, to the zero value.

Scenario: `MergeIn` loses the unlocked pre-lock race, both sides classify `up_to_date`, and the process dies between the second `saveMergeState` and `concludeMergeSides`. A resumed `lyx fabric merge --continue` reports `already_up_to_date:false` where the sequential equivalent reports `true`.
Low severity because the shape needs both a lost race and a crash, and because `committed:false` is already correct — but the invariant the round prompt names ("read off the record's own fields, never hardcoded per return site") is violated at this one site.

Suggested fix: `AlreadyUpToDate: st.bothSidesAlreadyUpToDate()` at that return, and a hermetic test pinning it.

### R4-F6 — NIT — CONFIRMED (by inspection)

`internal/fabricengine/merge.go:278-279` — "The guard stage is strictly read-only: nothing mutates here, including the sync step, which runs only after every guard passed."

False as written. `resolveMergeSources` (`mergeguards.go:45` and `:55`), which is called from inside that same guard aggregation, runs `f.warp.Fetch()` and `f.weft.Fetch()`. A fetch mutates remote-tracking refs and `FETCH_HEAD` in both repos. The claim is true of the *worktrees* and of fabric's own state, not of the repositories.

The same over-broad claim appears in `mergeguards.go:5` ("Every helper here … mutates nothing").

Suggested fix: narrow both claims to what is actually true — no worktree, index, branch tip, or fabric-record mutation — and name the best-effort fetch as the one exception.

### R4-F7 — NIT — CONFIRMED (by inspection)

`internal/fabricengine/doc.go`, "# The merge surface", the adoption paragraph ("A resumed `MergeContinue` detects that shape — HEAD moved off the recorded pre-merge SHA with no live `MERGE_HEAD` — and adopts the landed commit").

This is exactly the unsound predicate R4-F1 removes, stated in the docs as if it were sound, and the paragraph immediately below it tells an operator that hand-committing each unfinished side with plain git will be picked up by the adoption arm. After R4-F1 that advice is only correct if the hand-landed commit is a real merge commit of the recorded source SHA.

Suggested fix: rewrite the paragraph in the same commit as R4-F1 — state the parentage evidence the adoption arm now requires, state that a squash conclude is deliberately not adoptable, and correct the plain-git escape-hatch instructions to say `git merge <recorded source SHA>` rather than "commit each unfinished side by hand".

## Deferred items re-evaluated

- **Windows path behaviour in `weftPathVisible`/`unifyConflictPaths`.**
  NOT TOUCHED. This round ran entirely on Linux; I neither executed nor simulated Windows path behaviour, and I am not claiming coverage of it. It remains a carried-forward gap.
- **The squash shape of V1.** Driven this round as part of R4-F1's fix verification (see the fixer report). Not left as reasoning.
- **Four states where `MergeContinue` gets stuck** (conclude lands but `CurrentSHA`/`saveMergeState` fails).
  Re-evaluated: R4-F1 does NOT change that calculus, and confirms the prior round's reasoning.
  Those four states are the ones where the conclude genuinely landed, so the strengthened adoption arm still adopts them — the landed commit is a real two-parent merge of the recorded source. What R4-F1 removes is adoption of commits that are NOT that. So the recovery story for rows 27/28/30/31 is unchanged (and, if anything, now rests on evidence rather than on an ambiguous read).
- **Round 2's 45-row per-site adjudication of the post-record error-return class.** Not re-walked row by row this round; not required. No row was touched by this round's fixes except where R4-F3 moves the lock acquisition earlier, which adds no new post-record error return.

## Counts

BLOCKING 1 · MEDIUM 2 · LOW 2 · NIT 2 — 7 findings, all CONFIRMED.
