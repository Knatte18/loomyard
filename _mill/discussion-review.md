# Review: discussion.md — gitexec-error-shape-decision

Reviewed 2026-08-10 from the `loomyard` (main) worktree, against `main` at `c52faee4`.
Every count below was re-measured against the tree, not taken from the document.

**Verdict: four blocking, four non-blocking.**

The verdict itself — second entry point split by intent, raw form kept permanently for predicate sites, guard test with justification comments — is well argued and I would not reopen it.
The predicate-site discovery is the document's real contribution: it turns "incremental migration leaves a footgun" from an unanswered objection into a wrong framing, because the raw form is not legacy.

The blocking findings are all in the **cost model and the hand-off**, not in the decision.
Three of them share one root: the document inherited "the sweep is mechanical" from the task body — which I wrote — and built a migration plan on it without testing it against the call sites.
That claim does not survive contact with the tree.

---

## BLOCKING 1 — the "mechanical, whole-tree" sweep is not mechanical at 51 of ~70 sites

`discussion.md:248` says the failure-site rewrite collapses to `out, err := gitexec.Run(a, d)` and that "`gofmt -r` or a small AST tool handles the binding and the condition."

Measured: **51 call sites in `internal/fabricengine` have both an `if err != nil` block and an `if exitCode != 0` block after the same call.**
Each carries its own message — the first for "git would not run", the second for "git ran and refused".
Under the new shape both conditions become `if err != nil`, so the rewrite is not a substitution but a **merge**, and every merge is a decision about which of two existing messages survives, or what the combined one says.

`gofmt -r` rewrites expressions.
It cannot merge two statements with divergent bodies, and an AST tool that did would be making the editorial choice silently.

This inverts the document's own cost model.
`discussion.md:128` argues the per-site "what should the operator see" judgement is out of scope because R5 already did it, leaving 55 discard sites as the only judgement and the rest as a sweep.
The real split is closer to the opposite: ~51 sites need a message decision *because* they have two messages today, and the count of sites needing no thought is small.

This does not overturn the verdict — it is an argument about migration cost, and the verdict is defensible at a higher cost.
It does overturn `implementation-task-migrates-shape-not-wording` as written, and it should change the implementation task's size estimate.

**What to record instead:** the two-message merge is the dominant per-site pattern;
the default rule is that the exit-path message wins and the exec-path becomes `%w`-wrapped, with the sites that do not fit that rule read individually.
State the rule in the verdict so the implementer is not deciding it 51 times from scratch.

## BLOCKING 2 — the file:line inventory will be stale before anyone can use it

`implementation-task-identity` (`discussion.md:154`) sets `depends_on: ['fabric-corrindex-record-race']`, which puts the implementation behind the **whole** serialised fabric chain.
`verdict-carries-the-migration-recipe` (`discussion.md:142`) and the Testing section's acceptance bar (`discussion.md:291`) require the verdict to be executable "from the doc alone", by way of an enumerated file:line inventory.

Those two decisions fight each other.
Slice 12 rewires roughly 29 destructive call sites.
Slice 14's own scope note says it rewrites **every verb's result path** — which is precisely the `if exitCode != 0 { return fmt.Errorf(...) }` blocks this migration edits.
Every line number in the predicate inventory, the discard inventory and the 51-site figure is captured from a tree that four slices will rewrite before the implementation starts.

A concrete instance already exists.
`checkout.go:188`, `:190` and `:193` are three of the five full-discard sites this document sweeps "for free".
`checkout.go:193` is a `git branch -D` call, and it is one of the five primitives the in-flight chokepoint slice routes through an executing gate — after which it returns a refusal that the current `_, _, _, _ =` discards.
That line will not look like this when the migration runs.

**What resolves it:** record the inventory as **shapes plus a regeneration recipe** — the grep/AST queries that produced each count — and state explicitly that file:line must be re-derived at implementation time.
Then change the acceptance bar from "executable from the doc alone" to "re-derivable from the doc alone", which is the honest and achievable version.

## BLOCKING 3 — the discard count is 7, not 2, and one of them is contested ground

`discussion.md:236` — "Deliberate best-effort discards. **Only two**" — counts only the two bare `gitexec.RunGit(...)` calls at `prune.go:284-285`.

There are five more, assigned to blanks:

```
checkout.go:188   _, _, _, _ = RunGit({"switch", originalBranch}, ...)
checkout.go:190   _, _, _, _ = RunGit({"switch", originalWeftBranch}, ...)
checkout.go:193   _, _, _, _ = RunGit({"branch", "-D", forkedWeftBranch}, ...)
remove.go:202     _, _, _, _ = RunGit({"worktree", "prune"}, ...)
reconcile.go:426  _, _, _, _ = RunGit({"worktree", "prune"}, ...)
```

The document knows they exist — `discussion.md:248` says "the five full-discard `_, _, _, _` sites come along for free" — but never connects them to the section that counts deliberate discards, so the open question inherited from the original item ("how does this interact with the sites where discarding is correct?") is answered against 2 sites when it has 7.

They are not one class.
`remove.go:202` and `reconcile.go:426` are bookkeeping `worktree prune`, genuinely best-effort, and `remove.go`'s own comment says it must not turn a completed removal into an error.
`checkout.go:193` is not: it is a gated primitive under the chokepoint slice, and once that gate executes, a discarded return can swallow a *refusal* — a different thing from swallowing a best-effort failure.

Also worth stating plainly: `discussion.md:263` records that this repo has no `golangci-lint`.
`//nolint:errcheck` therefore enforces nothing here, so "it reads the same as today" (`discussion.md:244`) is true but hollow — nothing checks either shape.
If deliberate discard is meant to be visible, the guard test is the mechanism, not the comment.

## BLOCKING 4 — `gitexec.Run`'s stdout on failure is unspecified

`giterror-shape` and `drop-exitcode-from-the-checked-signature` fix the signature as `(string, error)` and specify `GitError` in full, but nothing says what the **string** is when the error is non-nil.

Today's `RunGit` returns whatever git wrote to stdout even on a non-zero exit.
The new form could return that, or `""`.
The document does not choose, and `GitError` has no `Stdout` field — rejected at `discussion.md:83` as speculative, which is reasonable *only if* stdout still comes back in the first return value.

For a document whose stated acceptance bar is that a future session implements from it alone, this is a one-line gap that changes behaviour.
Choose it and say so.

---

## NON-BLOCKING

### 5 — the exit-comparison arithmetic does not add up

`discussion.md:110` says the mechanical classification covers "the 59 exit-code comparisons in `fabricengine`", split 48 error-constructing / 11 not.
The actual total is **63**: 44 `exitCode != 0`, 8 `exitCode == 0`, 7 `code != 0`, 2 `code == 0`, 1 `unbornExit != 0`, 1 `statusExit != 0`.

The 63 figure is quoted correctly elsewhere in the same document (`discussion.md:104`), so this is an internal inconsistency, not a mismeasurement.
Four comparisons are unaccounted for in the classification that produces the predicate-site inventory — the document's load-bearing evidence.
Reconcile the two, or say which four were excluded and why.

### 6 — two set-equality guards will overlap on `gitrepo`, and the composition is unstated

`gitrepo-run-is-covered` puts `gitrepo.run`'s raw call sites under the new gitexec Checked-Call Invariant.
`CONSTRAINTS.md`'s existing gitrepo Client Boundary Invariant already pins `run` call sites via `TestGitrepoBoundary_PinnedRunCallSites`.

After the change, two set-equality tests assert over overlapping sets with different purposes — one about *which methods* may reach the CLI, one about *which call sites* may use the raw form.
That is probably fine, but a future edit to `gitrepo` will trip whichever it trips first with no guidance on which list to update.
Say how they compose, in the invariant text.

### 7 — `GitError.Args` renders caller-supplied URLs

`Error()` prints the full arg vector.
`clone.go:521` passes `gitURL` straight through from caller input, and `add.go:195` pushes to a named remote.
No path in this repo constructs a URL with an embedded credential today — I checked `githubclient` and `gitrepo` and found none — so this is not a live leak.

But `GitError` is being specified now, as shared infrastructure, and error strings reach logs and the board.
One sentence in the spec — either "args are rendered verbatim; callers must not pass credentials in args" or a redaction rule for `userinfo` in URL-shaped args — costs nothing and closes it before someone adds token auth.

### 8 — the `//nolint:errcheck` answer deserves the same treatment as the rest

`discussion.md:244` closes the inherited open question with "it barely does".
Given finding 3 (7 sites, not 2) and the absence of a linter, the honest closing is that the question was scoped to the wrong set and that the guard test is what will actually express intent.
Minor, but this is the one inherited open question the document claims to close, so it should close cleanly.

---

## Verified correct — do not re-measure

These were re-counted against the tree and match the document exactly:

- **75 production call sites**, 70 `fabricengine` / 2 `gitrepo` / 1 each `websterengine`, `lyxcwd`, `fabriccli`. Five outside fabric.
- **`gitrepo.run` has 21 production call sites** (`grep -c 'r\.run('`, non-test).
- **`internal/gitexec/gitexec.go` is 36 lines and declares exactly one function**, `RunGit` at line 15.
- **All 63 exit-code comparisons in `fabricengine` are against zero** — no site reads a specific code, so dropping it from the checked signature is sound.
- **`internal/boardengine` and `internal/githubclient` have no production call sites**, contrary to the original item's guess.

## Decisions I would not reopen

Argued well enough that re-litigating them costs a round:

- The intent split rather than a legacy-vs-new split. The predicate sites make the raw form permanently correct, which is a genuinely different argument from "migrate incrementally".
- `Run` gets the short name. The path of least resistance being the safe one is the whole mechanism.
- Exec-level failures stay unwrapped, so `errors.As` means "git ran and rejected this". This is the sharpest decision in the document.
- Dropping the exit code from the checked signature — settled by measurement, not taste.
- Not writing the guard test or the CONSTRAINTS entry in this task. An invariant with no enforcing test is the rot `CONSTRAINTS.md` exists to prevent.
- `go-git` as a supporting argument rather than a risk.
