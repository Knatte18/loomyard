MILL_REVIEW_BEGIN
# Review: fabric: one ownership-and-dirtiness gate for all destruction (slice 12)

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-10
```

## Findings

### [BLOCKING:design] Closed ownership enum has no link-shaped kind
**Section:** `ownership-is-a-closed-enum` vs the `fslink.Remove(` table / `link-repoint-is-gated-too`
**Issue:** The enum is five path kinds plus two ref kinds and says "Nothing else", yet the six gated link sites need "this path is a fabric link pointing where fabric expects" — the table instead says each site's "existing `fslink.IsLink` / `linkResolved != targetResolved` refusal becomes the ownership kind", which is no enum member and re-admits per-site checks.
**Fix:** Name the link-shaped kind(s) explicitly in the enum (and say whether `repointLink`'s target-comparison is a separate kind), or state that link sites reuse an existing member.

### [BLOCKING:decision] Rollback/other sites carry no stated ownership kind
**Section:** `rollback-paths-go-through-the-gate`, `os.Remove(` table
**Issue:** The document insists teardown's kind "must be stated rather than left to the implementer", then leaves `rollbackAdd` (`add.go:224`), `rollbackSwitch` (`checkout.go:187`), `add.go:265`'s `worktree remove --force`, and `launchers.go:165,170` with no kind assigned; `ownedHubGeometryChild` is defined but never mapped to a site.
**Fix:** Give every gated site in the disposition tables an explicit ownership-kind column, as `teardownHub` and the `branch -D` sites already have.

### [BLOCKING:design] `ownedFreshlyCreatedPath` is unverified at gate time
**Section:** `rollback-paths-go-through-the-gate`
**Issue:** The kind takes no repo context, so the gate can check nothing for it — the transaction guarantee lives in `CloneHub`'s `os.Stat`/`MkdirAll` sequence (verified at `clone.go:163`, `:212`, `:220`), not in the gate; nothing stops a future site declaring it, which is the `ownedInTransaction` trust-me the same section rejects.
**Fix:** State what the gate executes for this kind (if anything) and the rule limiting which sites may declare it, plus how a guard or review catches a new declaration.

### [BLOCKING:consistency] `ownedManagedWeftBranch` given two different signatures
**Section:** `gate-call-shape` / `ownership-is-a-closed-enum` vs `branch-deletion-is-ref-shaped`
**Issue:** The first two say `ownedManagedWeftBranch(l *lyxcwd.Location)` (justified because `primaryWeftBranch(l)` needs it — confirmed `cleanup.go:206`), the third says `ownedManagedWeftBranch(weftRepoRoot)`; a `weftRepoRoot` cannot resolve `primaryWeftBranch`.
**Fix:** Pick one parameter and correct the other two mentions.

### [BLOCKING:consistency] Constraints section restates the superseded `l`-on-the-gate model
**Section:** `## Constraints`, Cwd Resolution bullet
**Issue:** It says "The gate takes a `*lyxcwd.Location`", which round 2 explicitly superseded — `l` comes off the request entirely and travels only with `ownedManagedWeftBranch`; a plan writer reading the Constraints section gets the retracted shape.
**Fix:** Reword to "each check's inputs travel with the check; only `ownedManagedWeftBranch` takes a `*lyxcwd.Location`".

### [NIT:consistency] Q&A log's first two entries are stale and unmarked
**Section:** `## Q&A log` (first two bullets)
**Issue:** They name a single `destructiveRequest` struct and a dirtiness enum without `dirtyCheckedOutBranch`, both superseded later in the same log — while another entry is explicitly annotated "superseded in round 2".
**Fix:** Annotate or update those two entries to match the two-shape model.

### [NIT:consistency] `branchRequest` described as having no path fields
**Section:** `branch-deletion-is-ref-shaped`, check 1
**Issue:** "carries no path fields at all" contradicts its own declared `repoDir` field in `gate-call-shape`; the accurate claim is the narrower "no `container` or `target`".
**Fix:** Use the narrower wording in check 1.

## Verdict

REQUEST_CHANGES
Ownership-kind coverage and two superseded statements need resolving before plan writing.
MILL_REVIEW_END
