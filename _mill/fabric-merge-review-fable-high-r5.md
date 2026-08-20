# fabric merge surface — independent review (fable-high-r5)

Round 5 of the fabric-merge crucible campaign (second instalment).
Clean-room review by a fresh agent; no prior-round review/fixer material read before this findings list was complete.

## Scope reviewed

- `internal/fabricengine`: merge.go, mergelifecycle.go, mergeerrors.go, mergeguards.go, mergestate.go, mergestage.go, mergepaths.go + tests
- `internal/gitrepo/merge.go` + tests
- `internal/fabriccli/merge_verbs.go`, `cmd/lyx` wiring
- Docs: `internal/fabricengine/doc.go` "# The merge surface", `docs/overview.md`, `CONSTRAINTS.md`, `README.md`
- SPEC: `git show 3b800bc8:_mill/discussion.md` (+ plan batch headings)

## What was tested

### Hermetic gates (all green)

- `go build ./...` — OK
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — OK
- `go test -count=5 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...` — all ok
- `go test -tags integration -count=1 -timeout 30m` over the three packages — all ok (fabricengine 46.7s, fabriccli 4.3s, gitrepo 2.4s)

### Code reading pass

- Read the full SPEC discussion (decisions: recorded merge, two verbs, no-new-commit-until-clean, unified conflict paths, SHA-not-branch merges, aggregated side-free guards, conclude-never-rolls-back, weft-gated reset, correspondence, CLI mirror).
- Read all seven fabricengine merge production files, gitrepo/merge.go, fabriccli/merge_verbs.go, destroy.go's resetMergeSides, commit.go's sibling guard, doc.go lines 846–1046, mergevocab_test.go.

### Live driving (dev binary `.dev-bin/lyx`, deployed via `go run ./tools/deploy -dev`, hub recipe per prompt)

- Hub 2 (`$SCRATCH/hub2`): bare warp/weft (`-b main`), seeded warp `main` (root.txt, src/app.go), `lyx fabric clone` — ok.
- `lyx fabric add task-a` — pair created, both branches pushed.
- Scenario: both-sides divergent conflict. task-a: warp edit root.txt + weft `_lyx/note.md` via `lyx fabric commit`; prime: same paths, different content. `lyx fabric merge-in task-a` → `conflicts:["_lyx/note.md","root.txt"]`, exit envelope `ok:false, partial:false`, markers labelled `>>>>>>> <SHA>` on BOTH sides (no `-weft` token anywhere). Resolved both (weft path staged via `cd warp/_lyx && git add note.md` through the junction), `lyx fabric merge --continue` → `committed:true`, two-parent merge commits on both sides, subjects name SHAs, `fabric-merge.json` gone (find over the hub returns nothing). PASS.
- Scenario: non-ASCII conflicted path. Probe at the git layer first: `git diff --name-only --diff-filter=U` on a conflicted `ä-file.txt` emits the C-quoted form `"\303\244-file.txt"`, while `-z` emits raw bytes. Then live end-to-end: pair task-b, add/add conflict on `_lyx/ä-note.md` (both sides `lyx fabric commit`), `lyx fabric merge-in task-b` → **`fabricengine: merge produced conflicts outside the fabric-managed tree; operator intervention required`** (ErrUnmergeableState), both sides reset. The conflict is squarely INSIDE the fabric-managed tree; the quoted path `"_lyx/ä-note.md"` fails `weftPathVisible`'s prefix test because it begins with a `"` character. CONFIRMED defect — see F1.

## Provisional findings (to be confirmed)

- F1 (MEDIUM, CONFIRMED live): `gitrepo.ConflictedFiles` returns C-quoted paths for non-ASCII names (`core.quotepath` default); a wired weft conflict is then spuriously classified unmappable → `ErrUnmergeableState` self-abort, making the merge impossible through fabric; a warp-side non-ASCII conflict is reported as a garbled literal the operator cannot resolve against; `MergeStageResolved` cannot match the real path. Fix: `-z` + NUL-split in `ConflictedFiles`.
- F2 (MEDIUM, CONFIRMED by trace): lifecycle guard reads run OUTSIDE the write lock (TOCTOU). Sharpest shape: `MergeAbort` evaluates `concludeLandedReason` and loads the record BEFORE acquiring the weft write lock; a concurrent `MergeContinue` that concludes and deletes the record between guard-eval and lock-acquire leaves `MergeAbort` to `resetMergeSides` (force:true) the freshly landed conclude commits — exactly the destruction `concludeLandedReason` exists to prevent — and `deleteMergeState` tolerates the record's absence, so nothing notices. `MergeContinue` has the mirror shape (stale record adopted/resurrected after another process concluded), and `MergeIn`/`Merge`'s record-existence guard has the same window (a second `MergeIn` during another's resolution window overwrites the live record with a new source). Same family as R4-F3 (mutation-before-mechanism). Fix: re-load + re-validate the record (and the landed guard) after acquiring the lock, refusing on any change.
- F3 (LOW, CONFIRMED by reading): `doc.go`'s merge-surface paragraph "The rest of the mutating surface is deliberately unguarded ... for stated reasons rather than by omission" enumerates every unguarded mutating verb EXCEPT `MergeStageResolved`, which is a mutating, unlocked, unguarded merge-surface verb (its own godoc explains why, but the doc.go enumeration's completeness claim is now false by omission).
- F4 (NIT, CONFIRMED by reading): genuine-failure wrap messages inside the public merge verbs name sides ("fabricengine: resolve warp HEAD", "sync weft before merge", "check warp head attachment", etc.). These are unexpected-infra-error paths, consistent with package-wide practice and outside the named-error/side-free-result contract, but they do cross to non-owner-set callers on infrastructure failures, where two scenarios differing only in side produce different error text. (Assess: normalize the merge-surface wrap texts to side-free forms, keeping the side in internal logs.)

## Findings

(final graded list)
