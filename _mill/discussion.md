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

- Rewrite `manifest/designs/gitexec-error-shape.md` from an open question into a **recorded verdict**, carrying the decision, its reasoning, the counter-argument weighed, the new-shape specification, the enumerated predicate-site inventory, and the migration recipe.
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

### predicate-sites-are-real-and-must-stay-expressible

- **Decision:** Non-zero exit is recorded in the verdict as a **load-bearing answer**, not a failure, at the enumerated sites below. These keep the raw form permanently and are the reason the raw form is not deprecated.
- **Rationale:** The task body's claim that "the exit code is provably redundant" is true of the *value* and false of the *zero/non-zero predicate*. Mechanically classifying the 59 exit-code comparisons in `fabricengine` by whether their branch constructs an error puts 48 in an error-constructing branch and 11 in one that does not.
  The inventory (see [Technical context](#technical-context) for the full list) is `rev-parse --verify [--quiet] <ref>` used as a ref-existence check, the `warpprobe.go` "is this weft a weft" probes, `gitrepo.IsAncestor`'s explicit tri-state `switch`, and `diff --cached --quiet` mapped to `ErrIndexNotEmpty`.
- **Rejected:** Treating these as unmigrated debt to be swept later — they are not debt; sweeping them would be a regression.

### guard-test-with-justification-comments

- **Decision:** A new CONSTRAINTS invariant, the **gitexec Checked-Call Invariant**, enforced by a set-equality test pinning every remaining raw `gitexec.RunGit` and `gitrepo.run` call site. Each pinned site must carry a comment stating why a non-zero exit is not a failure there.
  Written and landed by the **implementation** task, not by this one.
- **Rationale:** Mirrors the established pattern in this repo — `cmd/lyx/gitrepoboundary_test.go` (`TestGitrepoBoundary_PinnedRunCallSites`) and `internal/gitrepo/noforceadd_test.go` — so it needs no new machinery and no `golangci-lint` (this repo has none; "lint rule" here means a guard test).
  The justification comment is what turns the pin from a bookkeeping list into a review artifact: a reviewer sees the claim being made, not just that the count changed.
- **Rejected:**
  - Pinning without requiring a justification comment — cheaper to maintain, weaker at review time.
  - No guard test at all — then the verdict is indistinguishable from plain incremental migration, and the raw form does silently become legacy debt.
- **Known blind spot, inherited from the sibling invariant:** set-equality on call sites does not catch a raw call slipped into an already-pinned function; per-call review still applies there.

### implementation-task-migrates-shape-not-wording

- **Decision:** The implementation task performs a mechanical shape migration. Error *messages* improve as a free consequence of `%w`-wrapping the `GitError`. The one hand-read exception is the 6 paths in `internal/fabricengine/add.go` that substitute a fixed wrong cause — each must be read individually.
- **Rationale:** The per-site "what should the operator see when this fails" judgement is precisely what crucible R5 already did, site by site, with the totals independently re-counted. Redoing it wholesale is a second campaign, not a migration, and it would turn a bounded 75-site sweep into open-ended work.
  The `add.go` paths are the exception because the shape change alone does not remove a *wrong* cause — it just puts a right one next to it.
- **Rejected:** Full per-site message review of all 55 (open-ended, re-litigates R5); shape-only with no `add.go` exception (leaves six actively misleading messages standing).

### verdict-doc-lifecycle

- **Decision:** `manifest/designs/gitexec-error-shape.md` is rewritten *in place* as the verdict by this task. The implementation task deletes it when it lands and moves the durable rationale into `internal/gitexec`'s package header comment.
- **Rationale:** This is the Documentation Lifecycle rule for module-design docs verbatim — deleted when the work lands, with purpose and key rationale moving into the Go package header next to the code. The doc's own status block already says "Deleted once the verdict is recorded, wherever it lands."
- **Rejected:**
  - Promoting it to `docs/reference/` as a durable contract doc — it documents one package's error shape, not a cross-module *file* contract honoured by a real consumer, so it does not meet that bar.
  - Recording the verdict directly in the package comment now and deleting the doc — the package comment would describe a shape the code does not have.

### verdict-carries-the-migration-recipe

- **Decision:** The rewritten verdict includes a "How the migration goes" section with the concrete rewrite patterns, the enumerated predicate-site inventory, and an `errors.As` recovery snippet.
- **Rationale:** The doc is deleted when the implementation lands, so anything not written into it has to be rediscovered from scratch by a session with no memory of this exploration. Recording the mechanics is what makes the follow-up task cheap.
- **Rejected:** Keeping the verdict to decision-and-reasoning only — cleaner as a document, more expensive as a hand-off.

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

These are the sites that keep the raw form. This list must be carried into the verdict doc verbatim, because it is the load-bearing evidence for the two-entry-point decision.

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

Other non-error-constructing branches surfaced by the mechanical classification, to be re-read when the inventory is finalised: `cleanup.go:280`, `reconcile.go:271`, `reconcile.go:300`, `reconcile.go:534`, `prune.go:218`, `prune.go:266`.

### Deliberate best-effort discards

Only two, both trivially expressible in either shape:

```go
// internal/fabricengine/prune.go:284-285
gitexec.RunGit([]string{"worktree", "prune"}, weftRepoRoot)     //nolint:errcheck
gitexec.RunGit([]string{"worktree", "prune"}, l.WorktreePath()) //nolint:errcheck
```

The original item flagged "how does this interact with the sites where discarding is correct?" as an open question; the answer is that it barely does — `//nolint:errcheck` on `gitexec.Run` reads the same as it does today.

### The migration recipe to record in the verdict

- **Mechanical, whole-tree.** At failure sites: `_, stderr, exitCode, err := gitexec.RunGit(a, d)` + `if err != nil {...}` + `if exitCode != 0 { return fmt.Errorf("...: %s", stderr) }` collapses to `out, err := gitexec.Run(a, d)` + `if err != nil { return fmt.Errorf("...: %w", err) }`. `gofmt -r` or a small AST tool handles the binding and the condition; the five full-discard `_, _, _, _` sites come along for free.
- **Uniform prior shape.** Sites that *do* bind stderr today follow one shape — each named `*Stderr` variable appears exactly twice, once bound and once used in an error message, and does nothing else with it. That uniformity is what makes the sweep safe.
- **Predicate recovery**, for any site that needs the code back:

  ```go
  var gitErr *gitexec.GitError
  if errors.As(err, &gitErr) && gitErr.ExitCode == 1 { /* the answer, not a failure */ }
  ```

- **Hand-read exception.** The 6 paths in `internal/fabricengine/add.go` that report `"cwd is not a valid git worktree"` for unrelated failures.

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
- **Self-containment check on the verdict doc** — the acceptance bar is that a session with no memory of this discussion could execute the migration from the doc alone. Concretely: the predicate-site inventory is enumerated with file:line, the `GitError` definition is given in full, the exec-level-failure rule is stated, and the `errors.As` recovery snippet is present.

Scenarios that must be covered by the verdict's prose, since they are what a future reader will challenge it on:

- Why the raw `RunGit` is not deprecated (the predicate sites).
- Why a non-zero exit from a `--verify` probe is not an error.
- Why exec-level failures are not `GitError`s.
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
