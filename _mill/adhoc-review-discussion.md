# Ad hoc review of discussion.md

> Written by the main-loop session (running in the `loomyard` hub worktree), at the operator's
> explicit request, as an independent second look at `fabric`'s `discussion.md` — outside this
> worktree's own discussion/review-round machinery. Not a burler/perch round; a manual sanity
> check.

## Finding 1 (high): `fabric` and `board-use-gitrepo` are both growing `internal/gitrepo` right
now, and neither discussion mentions the other

`discussion.md`'s "Most git mechanics grow into gitrepo" decision commits this task to adding, to
`internal/gitrepo`, in the same package `board-use-gitrepo` already touches: fast-forward pull,
lock-serialized commit, and a SHA-validated hard reset. Nothing in this discussion — or in
`board-use-gitrepo`'s own `discussion.md` — acknowledges that the other task is concurrently
extending the same package's surface.

This isn't a theoretical risk: `board-use-gitrepo` is **already past discussion and mid-implementation**
(status.md phase: `implementing`) and has already committed
`a84b35a8 feat(gitrepo): add StageAllAndCommit wildcard-stage method`, which edits
`internal/gitrepo/doc.go` (adding `StageAllAndCommit` to the primitives list, rewriting the Push
surface commentary, adding a paragraph about "board's opt-in exception"). `fabric` is still at
phase `discussed`, about to move into planning — meaning fabric's plan will be written against
`gitrepo`'s *current* surface, but by the time fabric actually lands its own gitrepo additions
(lock-serialized commit, hard reset, pull), `board-use-gitrepo`'s changes to the same file will
likely already be merged into `main` (or not — order isn't coordinated either way).

Concretely, at minimum this means:
- A near-certain merge/rebase conflict in `internal/gitrepo/doc.go` (both tasks rewrite the same
  doc-comment sections) whenever the second of the two lands.
- An **unaddressed design question**: does fabric's planned "lock-serialized commit" wrap only
  `StageAndCommit`, or does it also need to serialize `StageAllAndCommit`'s wildcard path? Neither
  discussion raises this.
- `git-native-library`'s feasibility spike (Someday→Planned, depends on `gitrepo` +
  `board-use-gitrepo`) targets "gitrepo's surface after board-use-gitrepo lands" — but if fabric's
  own gitrepo additions land in between or concurrently, that spike's scope is stale the moment
  either of these two tasks finishes.

**Recommendation:** before fabric moves into planning, either (a) have this task's plan explicitly
check `board-use-gitrepo`'s actual landed diff to `internal/gitrepo` first and design its
additions against that real state, or (b) flag this as an operator-level sequencing decision (land
one before starting the other's implementation) rather than letting both run to completion
independently and resolve the conflict at merge time.

## Finding 2 (low, clarity): SyncWeft's async-push rationale reads as if board routes through
`fabric.SyncWeft`, which contradicts `board-weft-storage.md`

The "SyncWeft: behavior parity plus the Warp-SHA trailer" decision says async push must "remain
first-class ... explicitly required for board once board-weft-storage puts board data on
`weft:main` (operator constraint)." Read in isolation this suggests board's `weft:main` writes go
through `fabric.SyncWeft`/`RevertWithWeft`. But `board-weft-storage.md` is explicit that they
don't: *"Board-related reads/writes to `weft:main` are a separate, standalone concern, not routed
through `fabric.SyncWeft`/`fabric.RevertWithWeft`."*

On a charitable reading this sentence in `discussion.md` means the underlying primitive
(`gitrepo.PushCoalesced`, non-blocking push) must stay generally available — since board will use
*that* directly, not `fabric.SyncWeft` — not that board calls into fabric's coordination API. That
reading is consistent with the rest of the decision (it cites `gitrepo`'s `PushCoalesced`, not a
fabric method, as the thing that must remain first-class). Worth a one-line rewrite so a future
reader doesn't have to reconcile the two documents to figure out which reading is correct.

## Finding 3 (low): fabric's `clone` parity doesn't mention a teardown/test-seam equivalent to
warp's `RemoveAll`

Technical context lists warpengine's exported surface as including a `RemoveAll` test seam
alongside `CloneHub`/`DeriveHostName`. The "Clone: full parity, board repo included" decision
specifies behavioral parity for `CloneHub` itself but is silent on whether fabric's differential
clone test tears down via warp's existing `RemoveAll` (reusing it against fabric-cloned repos) or
needs its own equivalent. Minor, but worth a one-line decision so the differential clone test's
teardown isn't improvised ad hoc when it's actually written.

## Not flagged, checked and found consistent

- `RevertWithWeft`'s partial-failure posture (rollback warp on weft-reset failure, typed
  both-states error if rollback itself fails) is coherent and mirrors `Checkout`'s existing
  discipline as claimed.
- The correspondence-index layering split (index component never touches git; fabric layer owns
  gitdir resolution) correctly resolves the Tier-1/integration-tagged test-purity conflict raised
  in review round 2.
- The engine-synchronous vs. CLI-detached `SyncWeft` push split, and the self-correction path for
  stale index entries via `SHAExists` + `RebuildIndex`, is internally consistent and matches
  `gitrepo`'s documented rebase-recovery contract (re-read `CurrentSHA` post-push).
