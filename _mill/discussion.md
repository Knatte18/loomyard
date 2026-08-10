# Discussion: gitexec: decide whether RunGit should return a typed error carrying stderr

```yaml
task: 'gitexec: decide whether RunGit should return a typed error carrying stderr'
slug: gitexec-error-shape-decision
status: discussing
parent: main
```

## Problem

`internal/gitexec.RunGit` returns git's stderr as an ordinary string alongside stdout, an exit code and an error:
`stdout, stderr, exitCode, err := gitexec.RunGit(args, dir)`.
`err` is non-nil only when git could not be executed at all — a command that *ran* and *failed* comes back with `err == nil`, a non-zero `exitCode`, and its explanation sitting in a string that nothing in Go's tooling pushes the caller to use.
Because discarding a return value is easier than threading it into an error message, callers discard it: the fabric v2 crucible's R5 round found **55 of 74** `RunGit` call sites in fabric dropped stderr entirely, **33** of those turned a failure into a bare exit code with no explanation at all, and **6** paths in `add.go` replaced the real cause with a fixed, wrong string (`"cwd is not a valid git worktree"`, reported for failures that had nothing to do with the cwd).
The operator sees `git exit 128` and nothing else.

The same sweep found the *silent-swallow* class genuinely closed — zero of 388 `if err != nil` blocks failed to handle the error.
The codebase is disciplined about not dropping errors and undisciplined about keeping git's explanation of them, and that asymmetry tracks exactly the difference between an `error` value and a plain string return.
R5 fixed the individual sites; it did not fix the signature that produced them, so the next module to use `gitexec` starts the count over from zero.

**Why now:** this task's stated precondition — the affected-caller count outside fabric — was measured and closed on 2026-08-10, so the decision is unblocked.
The task's output is a **verdict, not a migration**: nothing in `internal/gitexec` or its callers is edited here.
The verdict is written down, the implementation is filed as its own task, and the design doc is deleted when that task lands.

## Scope

**In:**

- Rewrite `manifest/designs/gitexec-error-shape.md` from an open question into a **recorded verdict**, carrying the decision, its reasoning, the counter-argument weighed, the new-shape specification, the site inventories as *shapes plus regeneration queries*, and the migration recipe including the two-message merge rule.
- Replace the decision item at `manifest/roadmap.md:70` in place with the implementation item, stating the verdict in one line.
- File the implementation task in the wiki: slug `gitexec-checked-entry-point`, `depends_on: ['fabric-corrindex-record-race']`.

**Out:**

- **All production code.** No edit to `internal/gitexec/gitexec.go`, `internal/gitrepo`, `internal/fabricengine`, or any call site. The `GitError` type is *specified* here, not written.
- **The guard test and its CONSTRAINTS invariant.** The "gitexec Checked-Call Invariant" and `TestGitexecCheckedCalls_PinnedRawCallSites` land with the implementation task — a set-equality test cannot pin call sites of a function that does not exist yet. `CONSTRAINTS.md` is **not** edited by this task.
- **Re-litigating crucible R5's per-site error messages.** The 55 discard sites were already fixed site-by-site; the implementation task migrates their *shape*, not their wording.
- **The `go-git` feasibility spike** (`manifest/roadmap.md:188`). It is cited in the verdict as a supporting argument and otherwise untouched.
- **`docs/overview.md`.** No module is added and the execution stack does not change; the module table already lists `internal/gitexec` under shared infra.

## Decisions

### verdict-second-entry-point

- **Decision:** Add a second, must-succeed entry point to `internal/gitexec` alongside the existing `RunGit`, and pin the remaining raw-`RunGit` call sites with a guard test.
  `RunGit` keeps its name, its 4-value shape, and its semantics — it stays the correct tool for sites where a non-zero exit is an *answer*.
  The new `gitexec.Run(args []string, cwd string) (string, error)` returns stdout and, on a non-zero exit, a `*GitError` carrying the diagnostic.
- **Rationale:** The two-shape split is **not** a legacy-vs-new split that leaves a deprecated wart behind. It maps onto a distinction the code genuinely has, discovered during exploration: at roughly 15 sites tree-wide a non-zero exit is a legitimate, non-error answer (see [predicate-sites](#predicate-sites-are-real-and-must-stay-expressible)).
  Those sites discard stderr *correctly* — there is no diagnostic because there is no failure — and a breaking "non-zero → error" signature makes every one of them worse, turning a branch-existence check into an error that must be caught with `errors.As` and re-interrogated.
  Because the raw form remains permanently correct for a real class of call sites, the usual objection to incremental migration ("it leaves the footgun loaded for anyone who does not migrate") is answered by the guard test rather than by a big-bang rewrite: adding a raw site becomes a deliberate, reviewed act with a written justification.
  A caller that wants the old behaviour on a failure path must now reach for it explicitly, which is the right amount of friction — throwing the diagnostic away becomes a visible decision rather than the default.
- **Rejected:**
  - *Breaking signature change* (`RunGit` itself returns `*GitError`) — one shape and one migration, but it degrades the ~15 predicate sites and forces `errors.As` into code that is currently a clean `if exitCode == 0`.
  - *Guard test only, no shape change* — cheapest, but a grep-shaped test cannot tell a predicate site from a failure site, so it either false-positives on the correct sites or is written loosely enough to miss the real ones.
  - *"Not worth it, recorded"* — a legitimate outcome (this is diagnostic quality, not correctness; no data was lost). Rejected because the 55/74 number is what the API shape produces, not who wrote the lines, and the blast radius turned out to be far smaller than "shared infrastructure" suggests: only five production call sites live outside fabric.

### naming-run-vs-rungit

- **Decision:** The new checked form is `gitexec.Run`. `RunGit` keeps its name and shape.
- **Rationale:** The short, obvious name goes to the form that should be reached for by default, so the path of least resistance is the safe one — which is the entire mechanism of this change. No rename churn across 75 call sites.
- **Rejected:** `RunGitE` — the `E` suffix is a Go-stdlib-ism this repo does not use anywhere else, and it reads as "variant of the real one" rather than "the default one".
  Renaming the raw form to `RunGitRaw` and giving `RunGit` the checked signature is the best *final* naming, but it is a breaking change at all 75 sites, which collapses it into the rejected breaking-change option above.

### giterror-shape

- **Decision:**

  ```go
  type GitError struct {
      Args     []string
      Dir      string
      ExitCode int
      Stderr   string
  }
  func (e *GitError) Error() string
  ```

  Returned as `*GitError`. `Error()` renders `git <args joined by space>: exit <code>: <trimmed stderr>`, and omits the trailing `: <stderr>` segment entirely when stderr is empty, yielding `git <args>: exit <code>`.
- **Rationale:** `Args` and `Dir` make the message self-locating, so callers can stop repeating "which command, in which directory" in their own wrappers — that ceremony is part of what the change removes.
  Trimming stderr keeps git's trailing newline out of wrapped messages. Omitting the empty segment avoids a dangling colon while still surfacing the exit code.
- **Argument rendering and credentials.** `Error()` renders `Args` **verbatim**, with no redaction, and the godoc must say so: *callers must not pass credentials in args.*
  Arg vectors reach error strings, which reach logs and the board. `clone.go:521` passes a caller-supplied `gitURL` straight through and `add.go:195` pushes to a named remote, so URL-shaped args already flow into this vector.
  No path in this repo constructs a URL with embedded `userinfo` today, so this is not a live leak — but `GitError` is being specified now as shared infrastructure, and the rule needs to exist before someone adds token auth. Stating the contract is chosen over implementing `userinfo` stripping, because a redaction rule invites callers to rely on it and it only covers the URL-shaped case.
- **Rejected:**
  - Adding a `Stdout` field — speculative; no site in the tree reads stdout on a failure path today.
  - A minimal `{ExitCode, Stderr}` — smallest surface, but every caller then re-adds the command and directory to its own wrapper.
  - Substituting a literal `(no stderr)` marker — explicit, but noisy in what is the common case for commands that fail on exit status alone.

### exec-level-failures-stay-unwrapped

- **Decision:** When git cannot be executed at all (binary missing, `cwd` does not exist — today's `err != nil, exitCode -1` path), `gitexec.Run` returns the raw underlying error **unwrapped in a `GitError`**. `*GitError` is produced only when git ran and exited non-zero.
- **Rationale:** This makes `errors.As(err, &gitErr)` mean precisely "git ran and rejected this", which is the distinction predicate-recovery depends on. A caller asking "does this branch exist?" must be able to tell "git said no" from "git never ran"; conflating them behind a sentinel `ExitCode: -1` silently turns an infrastructure failure into a negative answer.
- **Rejected:** Wrapping exec-level failures as `GitError{ExitCode: -1}` — one error type to handle, but it destroys the `errors.As` distinction and makes `-1` a sentinel every caller must know about.

### gitrepo-run-is-covered

- **Decision:** The verdict binds `internal/gitrepo.run` as well as `internal/gitexec.RunGit`. `gitrepo` gains the same pair — the existing `run` stays as the raw predicate form, and a checked sibling is added that returns stdout plus a wrapped `*GitError`.
- **Rationale:** `gitrepo.run` (`internal/gitrepo/gitrepo.go:60`) is a straight re-export of the identical 4-value shape and is the same footgun with **21 production call sites of its own**, six of which discard stderr. Deciding it separately guarantees the two drift apart. Both raw forms are pinned by the same guard test.
- **Rejected:**
  - Scoping the verdict to `gitexec` only and filing `gitrepo` as a follow-up — two decisions about one shape, taken at different times, by different sessions.
  - Deleting `gitrepo.run` in favour of calling `gitexec.Run` directly at all 21 sites — it removes the duplicate shape entirely, which is attractive, but `run` binds `r.path` and its removal churns the gitrepo Client Boundary Invariant's pinned method list for no gain the paired form does not already deliver.

### drop-exitcode-from-the-checked-signature

- **Decision:** `gitexec.Run` returns `(string, error)` — stdout and error only. The exit code is reachable on `*GitError.ExitCode` for anyone who needs it.
- **Rationale:** Every one of the 63 exit-code comparisons in `internal/fabricengine` is against zero (44 `exitCode != 0`, 8 `exitCode == 0`, 7 `code != 0`, 2 `code == 0`, plus one each for `unbornExit` and `statusExit`). No call site reads a *specific* code. On the checked form the value is dead weight in the signature, and it is exactly the redundant return that produced the `if exitCode != 0` habit in the first place.
- **Rejected:** `(stdout string, exitCode int, err error)` — keeps the code at hand without `errors.As`, at the cost of reintroducing the habit the change exists to break.

### stdout-is-returned-even-when-the-error-is-non-nil

- **Decision:** `gitexec.Run` returns whatever git wrote to stdout **in every case**, including when it returns a `*GitError`. The first return value is never blanked on failure.
  This must be stated in the function's godoc, not left to the reader.
- **Rationale:** It is what today's `RunGit` does, so the two forms stay consistent and the migration introduces no behavioural change beyond the error. It is also what makes rejecting a `Stdout` field on `GitError` coherent — that rejection is only reasonable if stdout still arrives in the first return value; blank it and the field becomes necessary after all.
  Discarding captured output on a failure path is the exact sin this whole change exists to correct, and several git commands (`push`, `worktree add`) write useful context to stdout while exiting non-zero.
- **Rejected:** Returning `""` on error — it reads as tidier and prevents a caller accidentally consuming partial output, but it throws away data the process already captured and would force a `Stdout` field onto `GitError` to get it back.

### predicate-sites-are-real-and-must-stay-expressible

- **Decision:** Non-zero exit is recorded in the verdict as a **load-bearing answer**, not a failure, at the enumerated sites below. These keep the raw form permanently and are the reason the raw form is not deprecated.
- **Rationale:** The task body's claim that "the exit code is provably redundant" is true of the *value* and false of the *zero/non-zero predicate*. Classifying **all 63** exit-code comparisons in `fabricengine` by whether their branch constructs an error puts **48 in an error-constructing branch and 15 in one that does not**.
  The inventory (see [Technical context](#technical-context) for the full list) is `rev-parse --verify [--quiet] <ref>` used as a ref-existence check, the `warpprobe.go` "is this weft a weft" probes, the three `return <code> == 0` predicate functions, `gitrepo.IsAncestor`'s explicit tri-state `switch`, and `diff --cached --quiet` mapped to `ErrIndexNotEmpty`.
- **Note on the count.** A first pass reported 48/11 = 59 because its classifier only examined comparisons appearing on a line containing `if` or `switch`. The four it skipped are `weftwiring.go:78`, `weftwiring.go:96`, `pull.go:135` (all `return <code> == 0`-shaped) and `checkout.go:77` (a multi-line `if` continuation).
  All four are **predicate** sites, so the correction moves them into the non-error-constructing column and strengthens rather than weakens the evidence: 48 + 15 = 63, matching the total quoted in [drop-exitcode-from-the-checked-signature](#drop-exitcode-from-the-checked-signature).
- **Rejected:** Treating these as unmigrated debt to be swept later — they are not debt; sweeping them would be a regression.

### guard-test-with-justification-comments

- **Decision:** A new CONSTRAINTS invariant, the **gitexec Checked-Call Invariant**, enforced by a set-equality test pinning every remaining raw `gitexec.RunGit` and `gitrepo.run` call site. Each pinned site must carry a comment stating why a non-zero exit is not a failure there.
  Written and landed by the **implementation** task, not by this one.
- **Rationale:** Mirrors the established pattern in this repo — `cmd/lyx/gitrepoboundary_test.go` (`TestGitrepoBoundary_PinnedRunCallSites`) and `internal/gitrepo/noforceadd_test.go` — so it needs no new machinery and no `golangci-lint` (this repo has none; "lint rule" here means a guard test).
  The justification comment is what turns the pin from a bookkeeping list into a review artifact: a reviewer sees the claim being made, not just that the count changed.
- **How it composes with the gitrepo Client Boundary Invariant.** After this change two set-equality guards assert over overlapping sets in `internal/gitrepo`, and the invariant text must say which is which so a future edit does not have to guess from whichever test fails first:
  the **Client Boundary Invariant** answers *which `gitrepo` methods may reach the git CLI at all* and is keyed by **method name** — update it when a method gains or loses a `run`/`gitexec` call;
  the **Checked-Call Invariant** answers *which call sites may use the raw, unchecked form* and is keyed by **call site** — update it only when a site moves between the raw and checked forms.
  A new `gitexec` call inside an already-pinned method trips the second and not the first; a new method reaching the CLI trips both. Each invariant's `CONSTRAINTS.md` entry must carry a one-line cross-reference to the other.
- **Rejected:**
  - Pinning without requiring a justification comment — cheaper to maintain, weaker at review time.
  - No guard test at all — then the verdict is indistinguishable from plain incremental migration, and the raw form does silently become legacy debt.
- **Known blind spot, inherited from the sibling invariant:** set-equality on call sites does not catch a raw call slipped into an already-pinned function; per-call review still applies there.

### the-migration-is-a-two-message-merge-not-a-substitution

- **Decision:** The dominant per-site pattern is a **merge of two existing error messages**, not a mechanical substitution, and the verdict must say so and supply the merge rule.
  Measured: roughly **51 of the ~70** `fabricengine` call sites carry *both* an `if err != nil` block and an `if exitCode != 0` block after the same call, each with its own message — the first for "git would not run", the second for "git ran and refused". Under the new shape both conditions collapse into `if err != nil`, so every one of those sites is a decision about which message survives.
  **The default merge rule, to be stated in the verdict so the implementer is not re-deciding it 51 times:** the **exit-path message wins** (it is the one written for the failure operators actually hit) and the exec-path message is dropped, with the returned error `%w`-wrapped so `GitError`'s own text supplies what the exec-path message used to say. Sites that do not fit the rule — where the exec-path message carries information the exit-path one lacks — are read individually and the deviation noted.
- **Rationale:** This corrects a claim this document inherited from the task body and did not test against the tree. `gofmt -r` rewrites expressions; it cannot merge two statements with divergent bodies, and an AST tool that did would be making the editorial choice silently.
  The original cost model — "55 discard sites need judgement, the rest is a sweep" — is close to inverted: the sites needing *no* thought are the small group, and ~51 need a message decision precisely *because* they already have two messages. This does not overturn the verdict, which is defensible at the higher cost, but it must change the implementation task's size estimate rather than surprising it.
- **Consequence for scope:** the implementation task still does **not** re-review the *wording quality* of R5's messages — that judgement was made site by site and re-litigating it is a second campaign. What it must do is decide, per site, which of two existing messages survives the merge, under the default rule above. The 6 paths in `internal/fabricengine/add.go` that substitute a fixed wrong cause remain a separate hand-read exception, because a shape change alone does not remove a wrong cause.
- **Rejected:** Keeping "mechanical, whole-tree sweep" as written (measurably false at 51 sites, and it would hand the implementer an estimate off by an order of magnitude); full per-site wording review of all 55 (open-ended, re-litigates R5); leaving the merge rule unstated and letting the implementer choose per site (51 uncoordinated editorial choices is exactly the inconsistency the guard test cannot catch).

### verdict-doc-lifecycle

- **Decision:** `manifest/designs/gitexec-error-shape.md` is rewritten *in place* as the verdict by this task. The implementation task deletes it when it lands and moves the durable rationale into `internal/gitexec`'s package header comment.
- **Rationale:** This is the Documentation Lifecycle rule for module-design docs verbatim — deleted when the work lands, with purpose and key rationale moving into the Go package header next to the code. The doc's own status block already says "Deleted once the verdict is recorded, wherever it lands."
- **Rejected:**
  - Promoting it to `docs/reference/` as a durable contract doc — it documents one package's error shape, not a cross-module *file* contract honoured by a real consumer, so it does not meet that bar.
  - Recording the verdict directly in the package comment now and deleting the doc — the package comment would describe a shape the code does not have.

### verdict-carries-shapes-and-a-regeneration-recipe-not-durable-line-numbers

- **Decision:** The rewritten verdict includes a "How the migration goes" section carrying the rewrite patterns, the merge rule, the `errors.As` recovery snippet, and the site inventories — but the inventories are recorded as **shapes plus the grep/AST query that produced each count**, with every file:line marked as a snapshot taken 2026-08-10 that **must be re-derived at implementation time**.
  The acceptance bar in [Testing](#testing) changes from "executable from the doc alone" to **"re-derivable from the doc alone"**.
- **Rationale:** The implementation sits behind the whole serialised fabric chain (`depends_on: fabric-corrindex-record-race`), and that chain rewrites the exact code the inventory points at. Slice 12 rewires roughly 29 destructive call sites; slice 14's scope note says it rewrites *every verb's result path* — which is precisely the `if exitCode != 0 { return fmt.Errorf(...) }` blocks this migration edits.
  A concrete instance already exists: `checkout.go:193` is a `git branch -D` call and one of the seven discard sites, and it is one of the five primitives the in-flight chokepoint slice routes through an executing gate — after which it returns a *refusal* that the current `_, _, _, _ =` swallows. That line will not look like this when the migration runs.
  So "executable from the doc alone" was never achievable, and promising it would hand the implementer stale coordinates presented as fact. The queries survive the chain; the line numbers do not.
- **Rejected:**
  - Keeping the enumerated file:line inventory as the deliverable and the "executable from the doc alone" bar — it fails on contact with the chain the same document sequences behind.
  - Dropping the dependency so the inventory stays fresh — the chain exists to keep concurrent edits out of `internal/fabricengine`, and a 70-site rewrite is the worst possible thing to run beside it.
  - Recording no inventory at all and telling the implementer to measure from scratch — the *shape* classification (which sites are predicates, which are two-message merges) is the expensive part and it is what this exploration actually produced.

### go-git-spike-is-a-supporting-argument

- **Decision:** The verdict cites `manifest/roadmap.md:188` (the `go-git` native-library feasibility spike) as an argument **for** the change, not as a risk against it.
- **Rationale:** If `gitexec` may later be backed by `go-git` instead of shelling out, then callers consuming an `error` rather than a `(stderr string, exitCode int)` pair is what makes that swap possible at all. The shell-out shape currently leaks into 75 call sites; a backend swap under the present signature would have to synthesise a plausible exit code and stderr string for every one of them.
- **Rejected:** Framing it as a risk that the spike could obsolete the work (it would not — the caller-facing contract is what survives a backend change), or omitting it (it is the strongest forward-looking argument available and the verdict should not leave it unsaid).

### implementation-task-identity

- **Decision:** Slug `gitexec-checked-entry-point`, `depends_on: ['fabric-corrindex-record-race']`.
- **Rationale:** The slug names what is built rather than what was decided. The dependency hangs it off the tail of the serialised fabric chain (`fabric-destructive-chokepoint` → `fabric-live-state-harness` → `fabric-mutation-record-envelope` → `fabric-corrindex-record-race`), because the implementation rewrites 70 call sites in `internal/fabricengine`, the exact package that chain is serialised to protect from concurrent edits.
- **Rejected:** `gitexec-error-shape-impl` (reads as a sub-part of a closed task); no `depends_on` with the sequencing stated in prose only (the chain exists precisely because prose sequencing did not hold).

## Technical context

### The function being decided about

`internal/gitexec/gitexec.go` is 36 lines and declares exactly one function:

```go
func RunGit(args []string, cwd string) (stdout, stderr string, exitCode int, err error)
```

It runs `exec.Command("git", args...)` with `cmd.Dir = cwd`, captures both streams into buffers, calls `proc.HideWindow(cmd)`, and then — critically — **converts an `*exec.ExitError` into a non-zero `exitCode` with `err = nil`**. Only a failure to execute at all propagates as an error, returning `("", "", -1, err)`.

### Call-site inventory (measured 2026-08-10)

Production call sites, excluding tests and the declaration, **75 total**:

| package | sites |
|---|---|
| `internal/fabricengine` | 70 |
| `internal/gitrepo` | 2 |
| `internal/websterengine` | 1 |
| `internal/lyxcwd` | 1 |
| `internal/fabriccli` | 1 |

Only five production sites live outside fabric. Tests add 50 more (`fabricengine` 15, `cmd/lyx` 10, `gitrepo` 10, `configcli` 5, `gitexec` 4, a handful elsewhere) — they migrate with the signature and carry no design weight.
`internal/boardengine` and `internal/githubclient`, named in the original item as places to look, have **no** production call sites.
R5 counted 74 in fabric where this count finds 71 (70 `fabricengine` + 1 `fabriccli`); code changed between the counts and nothing turns on the difference.

The concrete outside-fabric sites:

- `internal/gitrepo/gitrepo.go:60` — the `run` helper (see below).
- `internal/websterengine/gitwrap.go:31` — `status --porcelain`, wrapping `gitexec` directly because `gitrepo.Repo` exposes no porcelain method.
- `internal/lyxcwd/lyxcwd.go:147` — `rev-parse --show-toplevel`.
- `internal/fabriccli/fabric.go:491` — branch read.

### `gitrepo.run` — the second copy of the shape

```go
// internal/gitrepo/gitrepo.go:59-61
func (r *Repo) run(args ...string) (stdout, stderr string, code int, err error) {
    return gitexec.RunGit(args, r.path)
}
```

**21 production `r.run(...)` call sites** across `gitrepo.go`, `push.go`, `pull.go`, `reset.go`, `ancestry.go`. Six discard stderr (`reset.go:18`, `pull.go:19`, `pull.go:33`, `push.go:133`, `ancestry.go:26`, and the `_, _, code, err` form generally). The remaining fifteen bind it and thread it into an error message.
This is why the "5 sites outside fabric" figure understates the shape's reach — behind one of those five sits a second fan-out of 21.

### The predicate-site inventory — non-zero exit as an answer

These are the sites that keep the raw form. The **shape classification** below is the load-bearing evidence for the two-entry-point decision and must be carried into the verdict.

> **Snapshot, not a coordinate list.** Every file:line here was measured on 2026-08-10 against `main` at `c52faee4`. The implementation runs behind the serialised fabric chain, which rewrites this exact code, so the verdict records these as *shapes plus the query that finds them* and the implementer **re-derives** the lines. See [verdict-carries-shapes-and-a-regeneration-recipe-not-durable-line-numbers](#verdict-carries-shapes-and-a-regeneration-recipe-not-durable-line-numbers).

**Regeneration query** — classify every exit-code comparison by whether its branch constructs an error. Walk each non-test `.go` file under `internal/fabricengine`, match `\b(exitCode|code|unbornExit|statusExit)\s*(!=|==)\s*0\b`, and for each hit inspect the following ~8 lines for `fmt.Errorf` / `errors.New`.
Do **not** restrict the match to lines containing `if` or `switch` — that filter is what caused this document's first pass to report 59 instead of 63.
Current result: 63 total, 48 error-constructing, 15 not.

`rev-parse --verify [--quiet] <ref>` used as a ref-existence check — 6 sites:

- `internal/fabricengine/add.go:58` → `if exitCode == 0 { /* branch already exists */ }`
- `internal/fabricengine/boardweft.go:25` → `if exitCode == 0 { /* adopt local weft branch */ }`
- `internal/fabricengine/weftwiring.go:90` → `return exitCode == 0` (the whole function *is* the predicate)
- `internal/fabricengine/clone.go:433`
- `internal/fabricengine/clone.go:472` → `if exitCode == 0`
- `internal/fabricengine/warpprobe.go:77` → `if exitCode != 0 { return warpProbeResult{Found: false, WeftLooksLikeWeft: true}, nil }`

`internal/fabricengine/warpprobe.go` more broadly — non-zero means "this is not a weft", returned as a value with a nil error, at lines 71, 81, 95, 136.

`internal/gitrepo/ancestry.go:26` — `merge-base --is-ancestor` with an explicit tri-state, documented in the method's own godoc as "true if an ancestor, false if not (both with nil error), or an error on failure":

```go
switch code {
case 0:  return true, nil
case 1:  return false, nil
default: return false, fmt.Errorf(...)
}
```

`diff --cached --quiet` where exit 1 means "index is dirty" — `internal/gitrepo/gitrepo.go:140` (mapped to `ErrIndexNotEmpty`) and `internal/gitrepo/gitrepo.go:193`.

**Bare `return <code> == 0` predicate functions** — the whole function *is* the predicate, so there is no error to construct and nothing for the checked form to give them: `internal/fabricengine/weftwiring.go:78`, `weftwiring.go:96`, `internal/fabricengine/pull.go:135` (`return code == 0, nil`), plus `internal/fabricengine/checkout.go:77` (`); werr == nil && code == 0 {`, a multi-line `if` continuation).
These four are the comparisons the first classification pass skipped (see the note under [predicate-sites-are-real-and-must-stay-expressible](#predicate-sites-are-real-and-must-stay-expressible)).

Other non-error-constructing branches surfaced by the classification, to be re-read when the inventory is regenerated: `cleanup.go:280`, `reconcile.go:271`, `reconcile.go:300`, `reconcile.go:534`, `prune.go:218`, `prune.go:266`.

### Full-discard sites — seven, in two classes

Query: `grep -rn "_, _, _, _ *= *gitexec.RunGit\|^\s*gitexec.RunGit(" --include="*.go" internal/ | grep -v _test`.

```go
internal/fabricengine/remove.go:202     _, _, _, _ = RunGit({"worktree", "prune"}, ...)
internal/fabricengine/reconcile.go:426  _, _, _, _ = RunGit({"worktree", "prune"}, ...)
internal/fabricengine/checkout.go:188   _, _, _, _ = RunGit({"switch", originalBranch}, ...)
internal/fabricengine/checkout.go:190   _, _, _, _ = RunGit({"switch", originalWeftBranch}, ...)
internal/fabricengine/checkout.go:193   _, _, _, _ = RunGit({"branch", "-D", forkedWeftBranch}, ...)
internal/fabricengine/prune.go:284      RunGit({"worktree", "prune"}, weftRepoRoot)     //nolint:errcheck
internal/fabricengine/prune.go:285      RunGit({"worktree", "prune"}, l.WorktreePath()) //nolint:errcheck
```

**They are not one class, and the distinction matters to the verdict:**

- **Genuinely best-effort bookkeeping** — `remove.go:202`, `reconcile.go:426`, `prune.go:284`, `prune.go:285`. All `worktree prune`; `remove.go`'s own comment states it must not turn a completed removal into an error. These migrate as discards and stay discards.
- **Rollback and teardown on an error path** — `checkout.go:188`, `:190`, `:193`. `checkout.go:193` (`branch -D`) is one of the five primitives the in-flight chokepoint slice routes through an executing gate. Once that gate is live, a discarded return can swallow a **refusal**, which is a different failure from a best-effort operation not working. These three must be read individually at implementation time, not swept.

**On `//nolint:errcheck`.** The original item flagged "how does this interact with the sites where discarding is correct?" as an open question, and the honest closing is that the question was scoped to the wrong set — it was answered against 2 sites when there are 7, spanning two classes with different correctness arguments.
The comment itself enforces nothing here: this repo has no `golangci-lint` (see [Repo conventions](#repo-conventions-this-task-must-follow)), so `//nolint:errcheck` is documentation, and nothing checks either the old shape or the new one.
If deliberate discard is meant to be *visible*, the **guard test is the mechanism, not the comment** — the Checked-Call Invariant's justification-comment requirement is what makes each of these seven state its own case.

### The migration recipe to record in the verdict

- **The dominant pattern is a two-message merge, and it is not mechanical.** At a failure site today:

  ```go
  _, stderr, exitCode, err := gitexec.RunGit(a, d)
  if err != nil          { return fmt.Errorf("<exec-path message>: %w", err) }
  if exitCode != 0       { return fmt.Errorf("<exit-path message>: %s", stderr) }
  ```

  Under the new shape both conditions become `if err != nil`, so the two blocks **merge** and one of the two messages has to go. Roughly **51 of ~70** `fabricengine` sites are in this shape. `gofmt -r` rewrites expressions; it cannot merge statements with divergent bodies, and an AST tool that silently picked a message would be making the editorial call for the implementer.
  **Default merge rule:** the exit-path message wins, `%s`-of-stderr becomes `%w`-of-error, and the exec-path message is dropped because `GitError`'s own text now carries what it said:

  ```go
  out, err := gitexec.Run(a, d)
  if err != nil { return fmt.Errorf("<exit-path message>: %w", err) }
  ```

  Deviate only where the exec-path message carries information the exit-path one lacks, and note each deviation.
- **Genuinely mechanical subset.** Sites with only one of the two blocks, and the four best-effort `worktree prune` discards. Small — this is the group the original "whole-tree sweep" framing mistook for the majority.
- **Uniform prior shape for stderr binding.** Sites that *do* bind stderr follow one shape — each named `*Stderr` variable appears exactly twice, once bound and once used in an error message, and does nothing else with it. That uniformity is what makes the *binding* half of the rewrite safe; it says nothing about the merge.
- **Predicate recovery**, for any site that needs the code back:

  ```go
  var gitErr *gitexec.GitError
  if errors.As(err, &gitErr) && gitErr.ExitCode == 1 { /* the answer, not a failure */ }
  ```

- **Hand-read exceptions.** The 6 paths in `internal/fabricengine/add.go` that report `"cwd is not a valid git worktree"` for unrelated failures, and the three rollback discards at `checkout.go:188/190/193` (one of which becomes a gate-refusal path under the chokepoint slice).

### Repo conventions this task must follow

- **Markdown:** semantic line breaks, one sentence per line, no fixed-column wrapping. Table cells and blockquotes stay on one line. See the `mill:markdown` skill.
- **Guard-test pattern** for the future invariant: `cmd/lyx/gitrepoboundary_test.go` and `internal/gitrepo/noforceadd_test.go` are the two worked examples.
- **No `golangci-lint`** in this repo — invariants are enforced by Go tests, not by an external linter.
- **Roadmap discipline** (CLAUDE.md): `manifest/roadmap.md` moves only on completing or adding a planned item. Both apply here — the decision item completes and the implementation item is added — so the roadmap edit is in scope.

## Constraints

From `CONSTRAINTS.md`:

- **gitrepo Client Boundary Invariant** (`CONSTRAINTS.md:348`) — go-git owns local object and ref reads; `gitexec` is the only path to the git CLI, pinned to `StageAndCommit`, `CommitEmpty`, `StageAllAndCommit`, `Push`, `PushCoalesced`, `PushRebaseFree`, `Pull`, `Fetch`, `ResetHard`, `CheckoutDetached`, `RestoreBranch`, `IsAncestor`, `HasUnpushed`.
  Any new `gitexec` call inside `internal/gitrepo` must update that list in the same commit, and the pinned list is enforced by `TestGitrepoBoundary_PinnedRunCallSites`.
  This task adds no call and so does not touch the list; the implementation task changes the *shape* of calls inside already-pinned methods, which the set-equality check tolerates — but it must confirm that, not assume it.
- **Documentation Lifecycle** (`CONSTRAINTS.md:368` → `docs/overview.md:86`) — module-design docs under `manifest/designs/` are deleted when their module lands, with rationale moving into the Go package header comment. This is the rule that governs [verdict-doc-lifecycle](#verdict-doc-lifecycle).
- **Markdown Link Integrity** (`CONSTRAINTS.md:229`) — the rewritten design doc and the edited roadmap entry must keep every relative link resolving. The existing doc links to `fabric-crucible-followups.md` and `../../CONSTRAINTS.md#gitrepo-client-boundary-invariant`; the roadmap links to `designs/gitexec-error-shape.md`, and that link must survive this task (the doc is rewritten, not deleted, here).

Discovered during discussion:

- **Do not edit `CONSTRAINTS.md` in this task.** The gitexec Checked-Call Invariant is real and decided, but an invariant with no enforcing test is exactly the rot the file is meant to prevent. It lands with the guard test, in the implementation task's commit.
- **Worktree isolation.** This worktree is `gitexec-error-shape-decision` off `main`; direct pushes to `main` are forbidden from here.
- **Serialised fabric chain.** Nothing in this task touches `internal/fabricengine`, so it does not join the chain — but the task it files does, which is why the `depends_on` is not optional.

## Testing

This task changes no Go code, so there is no unit-test surface and no TDD candidate.
Verification is the repo's existing documentation guards plus a read-through.

- **Markdown link integrity** — run the repo's link guard over `manifest/designs/gitexec-error-shape.md` and `manifest/roadmap.md`. Every relative link in the rewritten doc must resolve, including the intra-document anchors if the verdict adds any, and `manifest/roadmap.md`'s link to `designs/gitexec-error-shape.md` must still point at a file that exists.
- **Markdown reflow** — `tools/mdreflow` is the repo's semantic-line-break tooling; the rewritten doc and the roadmap edit must satisfy it (one sentence per line, no fixed-column wrap, table cells on one line).
- **`go build ./...` / `go test ./...`** — expected to be a no-op pass. Run it as a negative check that nothing outside `manifest/` was touched, not as evidence of anything positive.
- **Wiki task creation is verified by reading it back** — after `upsert_task`, re-fetch `gitexec-checked-entry-point` and confirm the slug, the title, and that `depends_on` is exactly `['fabric-corrindex-record-race']`.
- **Self-containment check on the verdict doc** — the acceptance bar is that a session with no memory of this discussion could **re-derive** the migration from the doc alone, against a tree the fabric chain has since rewritten. Not "execute from the doc alone": the line numbers will be stale by then, and promising otherwise hands the implementer stale coordinates presented as fact.
  Concretely, the doc must carry: the `GitError` definition in full; the stdout-on-failure rule; the exec-level-failure rule; the `errors.As` recovery snippet; the two-message merge rule; and, for each inventory (predicate sites, two-message merges, seven discards), the **shape** and the **query that regenerates it**, with its file:line list labelled as a 2026-08-10 snapshot.

Scenarios that must be covered by the verdict's prose, since they are what a future reader will challenge it on:

- Why the raw `RunGit` is not deprecated (the predicate sites).
- Why a non-zero exit from a `--verify` probe is not an error.
- Why exec-level failures are not `GitError`s, and why stdout still comes back when one is returned.
- Why the migration is a message merge at ~51 sites rather than a mechanical sweep, and what the default merge rule is.
- Why the seven discard sites split into two classes, and why three of them are hand-read.
- Why the counter-argument ("diagnostic quality, not correctness") was weighed and not accepted.

## Q&A log

- **Q:** What is the verdict — signature change, second entry point, lint rule, or not worth it? **A:** Second entry point split by *intent* (not by legacy-vs-new), plus a pinned-call-site guard test.
- **Q:** Which form keeps the name `RunGit`? **A:** The raw form keeps it; the new checked form is `gitexec.Run`, so the short name belongs to the one that should be reached for by default.
- **Q:** Does the verdict cover `gitrepo.run`? **A:** Yes — same footgun, 21 call sites behind it; deciding separately guarantees drift. Not deleted in favour of direct `gitexec.Run` calls, because `run` binds `r.path` and removing it churns the Client Boundary Invariant's pinned list for no gain.
- **Q:** Does the checked form return the exit code? **A:** No — stdout and error only. All 63 exit-code comparisons in `fabricengine` are against zero, so the value is dead weight; it lives on `*GitError.ExitCode`.
- **Q:** Is `GitError` returned for exec-level failures too? **A:** No. `errors.As` must mean "git ran and rejected this", so a missing binary or bad cwd propagates unwrapped rather than as `ExitCode: -1`.
- **Q:** What does `Error()` print when stderr is empty? **A:** `git <args>: exit <code>` — the segment is omitted rather than filled with a `(no stderr)` marker.
- **Q:** Does this task write the guard test? **A:** No. It cannot pin call sites of a function that does not exist yet; the test and its CONSTRAINTS invariant land with the implementation.
- **Q:** Does the implementation task re-review the 55 discard sites' error wording? **A:** No — shape migration only, messages improve via `%w`. The 6 wrong-string paths in `add.go` are the single hand-read exception, because a shape change alone does not remove a wrong cause.
- **Q:** Where does the verdict live long-term? **A:** In `manifest/designs/gitexec-error-shape.md` until the implementation lands, then deleted with the rationale moved into `internal/gitexec`'s package header — the Documentation Lifecycle rule for module-design docs.
- **Q:** Does the verdict carry the migration mechanics? **A:** Yes — rewrite patterns, predicate-site inventory, `errors.As` snippet. The doc is deleted on landing, so anything unwritten must be rediscovered.
- **Q:** How does the `go-git` feasibility spike interact? **A:** Cited as an argument *for* the change: a backend swap is only possible if callers consume an `error` rather than a synthesised `(stderr, exitCode)` pair.
- **Q:** Slug and sequencing of the implementation task? **A:** `gitexec-checked-entry-point`, `depends_on: ['fabric-corrindex-record-race']` — the tail of the serialised chain protecting `internal/fabricengine` from concurrent edits.
- **Q:** Roadmap treatment? **A:** `manifest/roadmap.md:70` is replaced in place by the implementation item with the verdict stated in one line — the decision item completes and a planned item is added, which is exactly when the roadmap moves.

Resolved in discussion review (orchestrator review, 2026-08-10):

- **Q:** Is the failure-site rewrite really mechanical? **A:** No — measured, ~51 of ~70 `fabricengine` sites carry both an `err != nil` and an `exitCode != 0` block with separate messages, so the rewrite is a **merge** and each merge is an editorial choice. The claim was inherited from the task body and did not survive contact with the tree. The verdict now states a default merge rule (exit-path message wins, exec-path dropped, `%w`-wrap) so the implementer is not deciding it 51 times, and the implementation task's size estimate goes up accordingly.
- **Q:** Can the verdict promise a file:line inventory that is "executable from the doc alone"? **A:** No — the implementation sits behind the whole fabric chain, and slice 14 rewrites every verb's result path, which is exactly the code the inventory points at. `checkout.go:193` is already a live instance: the chokepoint slice routes its `branch -D` through an executing gate, after which the current `_, _, _, _ =` swallows a refusal. Inventories are now recorded as shapes plus regeneration queries, and the acceptance bar is **re-derivable** from the doc alone.
- **Q:** How many deliberate-discard sites are there? **A:** Seven, not two — the document counted only the two bare `prune.go` calls and never connected the five `_, _, _, _` sites to the section answering the inherited open question. They split into two classes: four best-effort `worktree prune` (stay discards) and three `checkout.go` rollback sites that must be hand-read.
- **Q:** What is the first return value of `gitexec.Run` when it returns a `*GitError`? **A:** Captured stdout, always — never blanked. This is what makes rejecting a `Stdout` field on `GitError` coherent, and blanking it would discard data the process already has.
- **Q:** 59 or 63 exit-code comparisons? **A:** 63. The 59 came from a classifier that only looked at lines containing `if` or `switch`; the four it missed (`weftwiring.go:78`, `weftwiring.go:96`, `pull.go:135`, `checkout.go:77`) are all `return <code> == 0`-shaped **predicate** sites, so the correction is 48 error-constructing / 15 predicate and it strengthens the inventory.
- **Q:** Does `//nolint:errcheck` express deliberate discard? **A:** No — this repo has no `golangci-lint`, so the comment enforces nothing and "it reads the same as today" is true but hollow. The guard test's justification-comment requirement is the mechanism.
- **Q:** How do the two set-equality guards on `gitrepo` compose? **A:** Client Boundary is keyed by **method name** (which methods may reach the CLI); Checked-Call is keyed by **call site** (which sites may use the raw form). A new call inside a pinned method trips only the second; a new method reaching the CLI trips both. Each `CONSTRAINTS.md` entry cross-references the other.
- **Q:** Can `GitError.Args` leak credentials? **A:** Not today — no path in the repo builds a URL with embedded `userinfo`. The spec now states args are rendered **verbatim** and callers must not pass credentials in them, chosen over a `userinfo` redaction rule because redaction invites reliance and covers only the URL-shaped case.
