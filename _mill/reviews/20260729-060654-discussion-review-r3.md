MILL_REVIEW_BEGIN
# Review: board: move storage to weft:main

```yaml
verdict: GAPS_FOUND
reviewer_model: sonnetmax
reviewed_file: /home/knatte/Code/loomyard/wts/board/_mill/discussion.md
date: 2026-07-29
```

## Findings

### [GAP] promote-note dual-presence vs. slug uniqueness
**Section:** `promote-note` — pure mechanical cross-store move / Global slug uniqueness
**Issue:** The resolved crash-safety order (upsert `tasks.json` first, remove `notes.json` second) deliberately leaves the same slug in both stores momentarily. But the Global slug uniqueness decision has every upsert check both stores' slug indices and reject a match found in the other store, and `promote-note`'s own bullet says it first "verifies the slug is absent from `tasks.json`" — a literal retry after a crash finds that false, contradicting the "recovery is a plain idempotent retry" claim two bullets later.
**Fix:** State explicitly that `promote-note`'s internal upsert is exempt from (or sequenced around) the generic cross-store uniqueness gate, and that a retry treats an already-present, field-identical `tasks.json` entry as success rather than re-checking absence.

### [GAP] promote-note leaves cross-store depends_on unaddressed
**Section:** `promote-note` / Global slug uniqueness (depends_on scoping)
**Issue:** `depends_on` validation stays strictly per-store ("a notes.json entry may only depend_on other notes.json entries"). If a `notes.json` entry depends on another `notes.json` entry and only the dependent is promoted, the upsert into `tasks.json` fails `Store.validateWrite`'s dangling-dependency check against `tasks.json`'s own snapshot — undiscussed.
**Fix:** Decide and record the behavior: reject promotion with a clear error, require promoting dependencies first, or state this combination is not expected to occur.

### [GAP] `_board`'s hardcoded "main" vs. the dynamically-discovered weft-primary branch
**Section:** `_board` becomes a second weft worktree, not a separate clone
**Issue:** The decision hardcodes `git worktree add <hub>/_board main` and an `origin/main` adopt-check, but `suffixWeftPrimaryBranch` (clone.go) — the adopt pattern this explicitly mirrors — never hardcodes a branch name; it reads whatever the weft primary is actually checked out on via `git branch --show-current` immediately after clone. A hub whose weft primary's un-suffixed branch isn't literally "main" would always take the fresh-orphan path for `_board` and leave that repo's real (mis-suffixed) primary branch stranded/unreferenced.
**Fix:** State explicitly whether "main" is a fixed, host-branch-independent literal for every hub, or `_board`'s worktree-add should derive the branch name the same way `hostBranch` is derived in `suffixWeftPrimaryBranch`.

### [NOTE] boardengine's own doc comments missing from the update list
**Section:** Scope / Technical context (doc updates in the same commit)
**Issue:** `internal/boardengine/board.go`'s package doc ("the detached sync path talks to git through a single gitrepo.Repo... never hand-rolled gitexec calls") and `sync.go`'s header comment (both describe `gitrepo.StageAllAndCommit`/`PushCoalesced` directly) go stale once `sync.go` calls `fabricengine.CommitWeftAt`/`PushWeftAt` instead, but neither file is in the Scope's stale-doc list (which names `fabricengine/doc.go`, `gitrepo/doc.go`, `hubgeometry.go`, `fabriccli`'s clone help).
**Fix:** Add `board.go`'s and `sync.go`'s header comments to the same-commit doc-update list.

### [NOTE] docs/overview.md's board-storage section is unconditionally stale
**Section:** Technical context
**Issue:** The item is hedged ("update if the module/execution-stack table describes board's storage model"), but `docs/overview.md`'s topology diagram literally labels `_board/` "(board repo; the task store)" and its artifact table describes "_board/ | Hub | Board | Task board at a configured board-repo URL... defaults to the weft repo's GitHub wiki" — both need real rewrites, not a conditional touch-up.
**Fix:** Drop the hedge; scope the edit explicitly to the topology-diagram line and the `_board` artifact-table row (including its "Repo" column value, which should become "Weft").

## Verdict

GAPS_FOUND
promote-note's crash-recovery story conflicts with global slug uniqueness; `_board`'s branch name is hardcoded against dynamically-derived precedent.
MILL_REVIEW_END
