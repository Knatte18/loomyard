MILL_REVIEW_BEGIN
# Review: fabric: merge-conflict primitive — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewer_self_id: Claude Opus-class model (env reports claude-opus-5); exact build not self-verifiable
reviewed_file: plan/
date: 2026-08-19
```

## Findings

### [BLOCKING:design] Weft ownership kind refuses the prime pair
**Location:** batch 2 / card 6 (and every consumer: cards 8, 11)
**Issue:** `ownedRegisteredLinkedWorktree`'s own godoc (`destroy.go:255-264`) says it matches a worktree "OTHER than the main one", and `clone.go` creates the weft primary with `cloneRepo(opts.WeftURL, weftPath)` — a full clone, i.e. the weft repo's MAIN worktree. So `resetMergeSides` refuses on the prime pair, which is exactly the pair `Merge` targets when merging a task into main; the card's proving test uses `newMergePairFixture` (`hubforge.NewHub`+`AddPair`), which only ever produces a linked weft worktree and therefore passes while the real case stays broken.
**Fix:** state the weft ownership kind decision up front (an `ownedWeftCheckout` mirroring `ownedWarpCheckout`'s any-worktree membership test) and require card 6's test to cover the prime pair explicitly, not "prove by test" against an AddPair-only fixture.

### [BLOCKING:design] Cleanup protection mechanism does not exist as described
**Location:** batch 5 / cards 13, 14
**Issue:** `Topology.Cleanup` deletes orphaned weft BRANCHES, never worktrees, and skips a pair outright (`cleanup.go:150-153` `continue`) whenever a live warp worktree is on the paired branch — so a mid-merge pair never reaches `Entries` at all. `CleanupBranchEntry` has only `Branch`/`Deleted`/`Protected`/`Error`; there is no field to carry "`ErrMergeInProgress`'s message text", and `Error` is documented as deletion-failure-only. Card 14's assertion ("present in the result's protection entries with the fixed message") cannot hold.
**Fix:** re-scope the Cleanup clause — either state that Cleanup needs no new guard because live pairs are already skipped, or specify the concrete new field/entry the protection reason travels in.

### [BLOCKING:design] Foreign-state disposition contradicts the discussion, undeclared
**Location:** batch 3 / cards 8, 10 (`MergeContinue`/`MergeAbort`)
**Issue:** the plan has the record-driven pair return `*ErrNoMergeInProgress` on foreign git merge state, matching `discussion.md:280`, but `discussion.md:627` states "all four verbs refuse with `*ErrForeignMergeState`". The discussion contradicts itself, the plan silently picks one side, and Shared Decision 1 ("where a card and the discussion ever disagree, the discussion wins") points an implementer back at the other.
**Fix:** add a Shared Decision naming both discussion clauses and pinning which one governs, so the card is not overridden by Decision 1's tie-breaker.

### [BLOCKING:consistency] Shared Decision 1 overrides Shared Decision 6
**Location:** `00-overview.md` `## Shared Decisions`
**Issue:** "the discussion wins" (Decision 1) collides head-on with "fabric-managed guard accepts a post-fetch remote-only weft counterpart" (Decision 6), which deliberately widens `discussion.md:287`'s `weftBranchExists` pin. As written, an implementer applying Decision 1 reverts Decision 6 and fails card 10's "source existing only remotely → merged" scenario.
**Fix:** scope Decision 1's precedence rule to "except where a later Shared Decision explicitly supersedes a discussion clause", and cross-reference Decision 6 there.

### [BLOCKING:design] No disposition for a genuine MergeStart error mid-attempt
**Location:** batch 3 / card 8 step 8; batch 4 / card 11 step 8
**Issue:** both cards say "attempt both sides unconditionally … via `MergeStart`" and specify only the conflict and clean outcomes. `MergeStart` can return a genuine non-conflict error (per card 1's own classification) after the state record was already written and possibly after the warp side mutated — the plan never says whether that self-aborts, retains the record, or which error surfaces.
**Fix:** pin the merge-attempt-phase error path explicitly (e.g. self-abort via `resetMergeSides` + `deleteMergeState` + wrapped error), in both cards, symmetric across sides.

### [BLOCKING:scope] Context lists omit files whose identifiers the Requirements name
**Location:** batch 2 / cards 4, 6; batch 3 / cards 7, 8
**Issue:** card 4 requires driving a fixture with `gitkit.MustRun` with no `internal/gitkit/gitkit.go` in Context; card 6 requires driving `MergeStart`/`MergeHeadPresent` with no `internal/gitrepo/merge.go`; card 7 names `Fetch` and `IsAncestor`, declared in `internal/gitrepo/pull.go` and `ancestry.go`, neither listed; card 8 names `lyxcwd.ResolveWorktree`, the "`Pull` allocation pattern" and `IsAncestor` with `internal/lyxcwd/lyxcwd.go`, `internal/fabricengine/pull.go` and `internal/gitrepo/ancestry.go` all absent.
**Fix:** add the four files to the respective `Context:` lists.

### [BLOCKING:design] addMergeVerbs signature elided; fab captures nil
**Location:** batch 6 / card 16
**Issue:** the card writes `addMergeVerbs(cmd *cobra.Command, ...)` with the parameter list literally elided. `fab` is a local of `addWeftVerbs` assigned inside `PersistentPreRunE` (`weft_verbs.go:41,52`), so passing it by value at registration time captures nil and every merge verb nil-panics; only a `**fabricengine.Fabric` or a getter closure works.
**Fix:** pin the full signature and state that the handle must be passed indirectly, not by value.

### [NIT:consistency] User-facing error text names Go identifiers, not CLI verbs
**Location:** batch 2 / card 3; batch 6 / card 17
**Issue:** `ErrMergeInRequired`/`ErrMergeIncomplete`/`ErrMergeInProgress` messages instruct the operator to "run MergeIn"/"run MergeContinue", and card 16 routes those fixed strings straight into the CLI envelope, where the verbs are `merge-in` and `merge --continue`/`--abort`.
**Fix:** note the mismatch in card 3 (the strings are discussion-pinned) or add a CLI-side remap so the envelope names the shipped verb spelling.

## Verdict

REQUEST_CHANGES
Two gate/verb mechanisms rest on false premises; several dispositions and Context lists are incomplete.
MILL_REVIEW_END
