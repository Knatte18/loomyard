# Review: discussion.md — fabric-destructive-chokepoint (slice 12)

Reviewed 2026-08-10 from the `loomyard` (main) worktree, against `main` at `faa0fe2b`.
Every code claim below was verified by reading the cited file, not inferred from the discussion.

**Verdict: three blocking findings, four non-blocking.**
The document is accurate where it makes factual claims — every line number, scope split and helper location I spot-checked resolved correctly.
The blocking findings are gaps in the *design*, not errors in the write-up: one primitive has no ownership model, the proposed guard cannot see two of its own listed sites, and two decisions contradict each other on the same code path.

---

## BLOCKING 1 — `git branch -D` has no ownership kind, and one of the eight defects was a destroyed branch

The ownership enum (`discussion.md:145`) is four **path-shaped** kinds:

```
ownedRegisteredLinkedWorktree(repoDir)   // a directory
ownedFabricHub                            // a directory
ownedHubGeometryChild(container)          // a directory
ownedInTransaction(reason)                // the escape hatch
```

But `git branch -D` is one of the five gated primitives (`discussion.md:52`), with four sites: `cleanup.go:273`, `checkout.go:193`, `add.go:277`, `weftwiring.go:192`.
Its target is a **ref**, not a path.

Consequences, in order of severity:

1. **Containment is meaningless for a ref.** The gate's first check has nothing to say.
2. **No enum kind can express branch ownership.** The only one that would compile is `ownedInTransaction(reason)` — the trust-me variant. That means the gate would cover `branch -D` *nominally* while delegating the real check back to the call site, which is the class this slice exists to close, reproduced for one of its five primitives.
3. **This is not hypothetical.** R3's defect (`discussion.md:39`) was `cleanup` destroying **the primary weft branch**. What protects it today is branch-space logic the enum cannot reach: `primaryWeftBranch(l)` at `cleanup.go:107`, the `branch == primaryWeft` carve-out at `cleanup.go:154`, and a deliberate fail-closed direction on an unreadable primary at `cleanup.go:205-211` ("irreversible, so an unreadable primary is the one direction that must fail closed").
4. **Dirtiness means something different for a ref too.** `TestCleanup_PrimaryBranchSurvivesForceWhenNotCheckedOut` implies the relevant question is "is this branch checked out somewhere", not "are there uncommitted files at a path". The two-value scope enum (`dirtyScopeTracked` / `dirtyScopeAll`) does not express it.

**What the discussion needs to add.** Either:

- a fifth, branch-shaped ownership kind — `ownedNonPrimaryWeftBranch(repoDir)` or similar — that resolves `primaryWeftBranch` **inside** the gate and inherits its fail-closed direction, with containment declared structurally N/A for ref targets; or
- an explicit statement that `branch -D` declares containment N/A, names which check carries its ownership, and argues why that is not `ownedInTransaction` in disguise.

Silence here means the implementer picks one under time pressure, and the cheap pick is the escape hatch.

## BLOCKING 2 — the proposed guard cannot see the `RemoveAll` seam it documents

Proposed banned tokens (`discussion.md:284`) include `os.RemoveAll(`.
The `RemoveAll` seam is `var RemoveAll = os.RemoveAll` at `clone.go:32`, and both call sites are **bare**:

```
clone.go:569:   if err := RemoveAll(hubPath); err != nil {
clone.go:605:   if err := RemoveAll(hubPath); err != nil {
```

`os.RemoveAll(` is not a substring of `RemoveAll(hubPath)`.
The guard as specified misses both — and `clone.go:605` is `teardownHub`, which is gap #2 in the discussion's own list (`discussion.md:268`).
So the two sites the slice most wants policed are the two the guard is blind to.

The discussion notices the seam twice (`discussion.md:236`, `:258`) without connecting it to the token set.

**Fix:** add `RemoveAll(` to the banned token set. It is a superset that also catches `os.RemoveAll(`, so the latter becomes redundant.
Alternatively delete the seam and inject the removal function some other way, but that is more work for no added property.

Also verify the same class for the other tokens: `.ResetHard(` catches `f.warp.ResetHard(` (`pull.go:233,267,285`) — confirmed, since those are method calls, not a package alias.

## BLOCKING 3 — `teardownHub`'s ownership kind is unresolved, and both candidates fail

Two decisions collide on one code path and the document never reconciles them:

- `rollback-paths-go-through-the-gate` (`discussion.md:111`) puts `teardownHub` (`clone.go:604`) through the gate with "containment and ownership **enforced**" and only *dirtiness* declared N/A.
- Gap #2 (`discussion.md:268`) says `teardownHub` needs the ownership check its `--reset` sibling has.

Neither says **which kind** it declares. Both available answers are bad:

- **`ownedFabricHub`** — `looksLikeHub` (`clone.go:579`) requires a `_board` entry or at least one weft sibling. `teardownHub` fires on *any* clone-or-worktree-add failure, including one early enough that neither exists yet. The rollback would then be **refused**, and `clone.go:606` leaves "residual hub left at %s; remove it manually". That is a regression: today teardown always cleans up. A gate that blocks the cleanup of a half-built hub is worse than the gap it closes.
- **`ownedInTransaction(reason)`** — closes gap #2 in name only. The whole point of naming it a gap was that R4's `clone --reset` defect *was* a teardown path, so "teardown is special" is the reasoning that produced a data-loss bug.

**What resolves it:** a containment-only formulation. `teardownHub` removes exactly the path this invocation created, so the honest check is "is `hubPath` the path this transaction just created, and does it resolve strictly below the parent the operator named?" — which is containment plus a transaction identity, not a hub predicate.
If that is the intent, say so and say why it is stronger than `looksLikeHub` here rather than weaker.
Note this is also the one place `ownedInTransaction` might be legitimately correct — but the doc must argue it, not default to it.

---

## NON-BLOCKING

### 4 — the Out-section contradicts the In-section on behaviour changes

`discussion.md:69` lists "**Behaviour changes.**" as out of scope: "This is a consolidating refactor."
`discussion.md:53` lists closing three gaps as **in** scope — and all three are behaviour changes by construction: paths that were destroyed will now be refused.

As written, a reviewer of the resulting commit can flag the gap closures as scope violations, citing the document's own Out list.

**Fix:** reword to "behaviour changes **other than the three named gaps**", and keep the sharp part that is doing real work — every current site keeps its current dirtiness scope, and any existing named test needing an edit is surfaced rather than edited.

### 5 — force × containment is unspecified and untested

The testing section (`discussion.md:334`) pins "`force: true` satisfies a dirtiness check and never satisfies an ownership check".
It says nothing about **containment**.

`remove ..` — the defect that destroyed an entire hub — is a containment failure. If `--force` were ever read as satisfying containment, that defect returns behind a flag.
The gate's step 4 in the manifest says force answers "discard work I might want" and never "delete something that was never fabric's"; containment deserves the same explicit sentence, and a test.

Add: `force: true` satisfies dirtiness only, and never containment or ownership.

### 6 — the guard's substring brittleness should be stated, per repo convention

Proposed tokens include `"worktree", "remove"` — a raw substring with a specific spacing. `[]string{"worktree","remove"}` (no space) evades it, as does a dynamically built arg slice.

Every other guard in this repo states its blind spot explicitly — the gitrepo Client Boundary Invariant has a "Known guard blind spot" paragraph, and the Fabric Vocabulary Invariant has a whole "what the machine check does and does not reach" section written "stated honestly, not implying full coverage".

The `CONSTRAINTS.md` entry should carry one sentence in that register. This is a convention, not a defect.

### 7 — `checkout.go:193` deliberately discards its error; the executing gate changes that

`rollbackSwitch` calls `_, _, _, _ = gitexec.RunGit([]string{"branch", "-D", forkedWeftBranch}, ...)` — a deliberate four-way discard on a best-effort rollback path.

Once that becomes an executor that runs the gate and returns an error, the call site has to decide: keep ignoring (and then a *refusal* is silently swallowed, which is a new way to lose a signal), or start propagating (which changes rollback behaviour).

Same question applies to `remove.go`'s post-fallback `worktree prune`, which the code comments explicitly as "Bookkeeping only … must not turn a completed removal into an error".

Name the policy once: best-effort executors return a value the caller may discard, but a **refusal** is never best-effort.

---

## Settled — do not reopen in review rounds

These were argued and resolved with evidence; re-litigating them costs a round:

- Dirtiness scope is caller-declared. The 4/4 split is real and verified: tracked-only at `add.go:43`, `checkout.go:41`, `prune.go:215`, `pull.go:143`; untracked-inclusive at `remove.go:61`, `remove.go:132`, `warpclean.go:54`, `reconcile.go:299`.
- The probe stays fabric-local. `Topology` holds only `cfg Config` (`topology.go:18-20`) — six of eight probe sites have no `Repo` to hang a `gitrepo` method off.
- Ownership is a closed enum, not a caller predicate. (An interface would **not** be equivalently closed unless sealed with an unexported method — Go interfaces are open sets. The discussion's rejection line is right for the wrong reason, which does not matter to the outcome.)
- Exclude rewrite stays out: `gitexclude.go`'s four `os.Remove` calls (`:108,112,116,120`) are all temp-file cleanup inside `writeFileAtomically`, and `mutateGitExclude` is already its own chokepoint under CONSTRAINTS.md's Fabric Git Invariant.
- `removeJunctionRecords` containment is a live gap: `weftwiring.go:145-161` takes `WarpJunctions(l, slug, names)` — slug-derived paths, the same family that let `<hub>/_launchers/./..` resolve to `<hub>`.
- One commit for the slice, rebase before push.

## Note on a stale manifest sentence

`manifest/designs/fabric-crucible-followups.md`'s slice-12 open questions still say the dirtiness probe "is deliberately tracked-only today" and that the chokepoint "should inherit it rather than silently widen it".
That sentence over-generalised `prune.go`'s comment to the whole package and is contradicted by the 4/4 split above.

It is in the section a reviewer of this slice reads. Left standing, it invites a reviewer to file the caller-declared enum as a deviation from spec.
The `dirtiness-scope-is-caller-declared` decision should correct it in the same commit, not merely diverge from it.
