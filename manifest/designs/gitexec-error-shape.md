# gitexec: the RunGit error-shape verdict

> **Status: the verdict is recorded here.**
> This doc survives until the implementation lands, then is deleted under the [Documentation Lifecycle](../../docs/overview.md#documentation-lifecycle) rule, with the durable rationale moving into `internal/gitexec`'s package header comment.

Filed as GitHub issue #145 (`question`) by the fabric v2 crucible campaign's orchestrator, and folded into the manifest here;
the issue is closed, pointing at this file.

## The verdict

Add a second, must-succeed entry point, `gitexec.Run(args []string, cwd string) (string, error)`, alongside the existing `RunGit`. `RunGit` keeps its name, its four-value shape, and its semantics — it stays the correct tool for sites where a non-zero exit is an *answer*, not a failure. `internal/gitrepo` gains the same pair: `run` stays as the raw form,
and a checked sibling, `runChecked`, is added.
The remaining raw call sites — the ones that keep `RunGit`/`run` after the migration — are pinned by a guard test requiring an adjacent, written justification comment.

This is **not** a legacy-vs-new split that leaves a deprecated wart behind.
The raw form is permanently correct for a real class of call sites and is not deprecated by this change.

## Why

The two-shape split maps onto a distinction the code genuinely has: at roughly a dozen sites in fabric, plus several in `gitrepo`, a non-zero exit is a legitimate, non-error answer, not a failure.
The short, obvious name goes to the form that should be reached for by default, so the path of least resistance is the safe one — that is the entire mechanism of this change.
Because the raw form remains permanently correct for a real class of sites, the usual objection to incremental migration — "it leaves the footgun loaded for anyone who does not migrate" — is answered by the guard test rather than by a big-bang rewrite: adding a raw site becomes a deliberate, reviewed act with a written justification, instead of a silent default.

The `go-git` feasibility spike (`manifest/roadmap.md`'s Done section) is cited as an argument *for* this change, not a risk against it.
If `gitexec` may later be backed by `go-git` instead of shelling out, callers consuming an `error` rather than a synthesised `(stderr string, exitCode int)` pair is what makes that backend swap possible at all — the shell-out shape currently leaks into 74 call sites, and a backend swap under the present signature would have to synthesise a plausible exit code and stderr string for every one of them.

## The counter-argument, weighed

This touches shared infrastructure to fix a **diagnostic-quality** problem, not a correctness one.
No data was lost because of a missing stderr, and "not worth it, recorded" was a legitimate outcome under consideration.

It was weighed and not accepted, for two reasons.
The 55-of-74 discard figure is what the API shape produces, not who wrote the lines — R5 fixed the sites, and without a shape change the next module to use `gitexec` starts the count over from zero.
And the blast radius outside fabric, which "shared infrastructure" suggests is large, turned out to be small: only four production call sites live outside fabric (see [Site inventories](#site-inventories--shapes-and-regeneration-queries) below).

## Rejected alternatives

- **Breaking signature change** (`RunGit` itself returns `*GitError`) — one shape and one migration,
  but it degrades every predicate site and forces `errors.As` into code that is currently a clean `if exitCode == 0`.
- **Guard test only, no shape change** — cheapest,
  but a grep-shaped test cannot tell a predicate site from a failure site, so it either false-positives on the correct sites or is written loosely enough to miss the real ones.
- **"Not worth it, recorded"** — a legitimate outcome on its own terms, rejected because the 55/74 number is what the API shape produces rather than who wrote the lines,
  and the blast radius outside fabric turned out to be four production sites, not a scale that matches "shared infrastructure".
- **The `RunGitE` name** — the `E` suffix is a Go-stdlib-ism this repo does not use anywhere else,
  and it reads as "variant of the real one" rather than "the default one".
- **Renaming the raw form to `RunGitRaw`** and giving `RunGit` the checked signature — the best *final* naming,
  but it is a breaking change at all 74 sites, which collapses it into the rejected breaking-signature-change option above.
- **Adding a `Stdout` field to `GitError`** — speculative;
  no site in the tree reads stdout on a failure path today.
- **A minimal `{ExitCode, Stderr}` struct** — smallest surface,
  but every caller then re-adds the command and directory to its own wrapper.
- **A literal `(no stderr)` marker** — explicit, but noisy in what is the common case for commands that fail on exit status alone.
- **Wrapping exec-level failures as `GitError{ExitCode: -1}`** — one error type to handle,
  but it destroys the `errors.As` distinction and makes `-1` a sentinel every caller must know about.
- **Returning `(stdout string, exitCode int, err error)` from the checked form** — keeps the code at hand without `errors.As`, at the cost of reintroducing the habit the change exists to break.
- **Returning `""` on error** — reads as tidier and prevents a caller accidentally consuming partial output,
  but it throws away data the process already captured and would force a `Stdout` field onto `GitError` to get it back.
- **Scoping the verdict to `gitexec` only**, filing `gitrepo` as a follow-up — two decisions about one shape, taken at different times, by different sessions.
- **Deleting `gitrepo.run` in favour of calling `gitexec.Run` directly at all 21 sites** — removes the duplicate shape entirely, which is attractive, but `run` binds `r.path`,
  and its removal churns the gitrepo Client Boundary Invariant's pinned method list for no gain the paired form does not already deliver.
- **Implementing `runChecked` on top of `run`** — forces `gitrepo` to construct `*gitexec.GitError` itself, duplicating logic that belongs in one place and requiring `gitexec` to export a constructor for it;
  it would also leave `gitexecTotal == 1` and both boundary assertions passing, which is only an apparent saving, since the invariant's real point is that git-CLI access is funnelled through named helpers, and two named helpers satisfy that as well as one.
- **Pinning raw sites without requiring a justification comment** — cheaper to maintain, weaker at review time.
- **No guard test at all** — then the verdict is indistinguishable from plain incremental migration,
  and the raw form does silently become legacy debt.

## The new shape

```go
type GitError struct {
    Args     []string
    Dir      string
    ExitCode int
    Stderr   string
}
func (e *GitError) Error() string
```

Returned as `*GitError`. `Error()` renders `git <args>: exit <code>: <trimmed stderr>`, and omits the trailing `: <stderr>` segment entirely when stderr is empty, yielding `git <args>: exit <code>`.

**Arg joining: space-separated, `%q`-quoted only when an arg needs it** — an arg containing whitespace,
or an empty arg. `git status --porcelain` stays readable;
`git commit -m "fix the thing"` stays unambiguous and copy-pasteable.
Quoting every arg unconditionally was rejected as noise on the common case (`git "status" "--porcelain"`),
and a bare join was rejected because commit messages, `--filter=` values and paths with spaces all occur in this tree and would render ambiguously.

**Argument rendering and credentials.** `Error()` renders `Args` **verbatim**, with no redaction,
and the godoc must say so: *callers must not pass credentials in args.*
Arg vectors reach error strings, which reach logs and the board.
No path in this repo constructs a URL with embedded `userinfo` today, so this is not a live leak — but `GitError` is being specified now as shared infrastructure,
and the rule needs to exist before someone adds token auth.
Stating the contract is chosen over implementing `userinfo` stripping, because a redaction rule invites callers to rely on it and it only covers the URL-shaped case.

**Exec-level failures stay unwrapped.**
When git cannot be executed at all (binary missing, `cwd` does not exist), `gitexec.Run` returns the raw underlying error **unwrapped in a `GitError`**. `*GitError` is produced only when git ran and exited non-zero.
This makes `errors.As(err, &gitErr)` mean precisely "git ran and rejected this", which is the distinction predicate-recovery depends on — a caller asking "does this branch exist?" must be able to tell "git said no" from "git never ran", and conflating them behind a sentinel `ExitCode: -1` would silently turn an infrastructure failure into a negative answer.

**Stdout is returned even when the error is non-nil.** `gitexec.Run` returns whatever git wrote to stdout **in every case**, including when it returns a `*GitError`.
The first return value is never blanked on failure,
and this must be stated in the function's godoc, not left to the reader.
It is what today's `RunGit` does, so the two forms stay consistent,
and it is what makes rejecting a `Stdout` field on `GitError` coherent — that rejection is only reasonable if stdout still arrives in the first return value.

**The checked signature drops the exit code.** `gitexec.Run` returns `(string, error)` — stdout and error only.
The exit code is reachable on `*GitError.ExitCode` for anyone who needs it.
Every one of the 63 exit-code comparisons in `internal/fabricengine` is against zero, so on the checked form the value is dead weight in the signature,
and it is exactly the redundant return that produced the `if exitCode != 0` habit in the first place.

**`gitrepo` gains the same pair.** `runChecked` calls `gitexec.Run` directly, as a second chokepoint beside `run`:

```go
func (r *Repo) run(args ...string) (stdout, stderr string, code int, err error) { return gitexec.RunGit(args, r.path) }
func (r *Repo) runChecked(args ...string) (string, error)                       { return gitexec.Run(args, r.path) }
```

## How the migration goes

### The dominant pattern is a two-message merge, not a substitution

At a failure site today, `err != nil` (exec-path) and `exitCode != 0` (exit-path) are separate conditions, each with its own message:

```go
_, stderr, exitCode, err := gitexec.RunGit(a, d)
if err != nil          { return fmt.Errorf("<exec-path message>: %w", err) }
if exitCode != 0       { return fmt.Errorf("<exit-path message>: %s", stderr) }
```

Under the new shape both conditions collapse into `if err != nil`, so every one of those sites is a decision about which message survives.
Roughly **51 of ~70** `fabricengine` call sites are in this shape. `gofmt -r` rewrites expressions;
it cannot merge two statements with divergent bodies,
and an AST tool that did would be making the editorial choice silently.

**The default merge rule, so the implementer is not re-deciding it 51 times:** the exit-path message wins — it is the one written for the failure operators actually hit — `%s`-of-stderr becomes `%w`-of-error,
and the exec-path message is dropped, because the returned error's own text (`GitError.Error()`) now supplies what the exec-path message used to say:

```go
out, err := gitexec.Run(a, d)
if err != nil { return fmt.Errorf("<exit-path message>: %w", err) }
```

Sites that do not fit the rule — where the exec-path message carries information the exit-path one lacks — are read individually and the deviation noted.

### The `(git exit %d)` fragment is dropped together with its argument

This is not optional,
and it is not covered by "`%s`-of-stderr becomes `%w`-of-error": roughly 30 production exit-path messages embed the exit code as well as the stderr, for example:

```go
// internal/fabricengine/checkout.go:95 — same shape at remove.go:67, add.go:203, clone.go:528,
// weftwiring.go:116, reconcile.go:470, status.go:190, cleanup.go:243, cleanup.go:282
return CheckoutResult{}, fmt.Errorf("warp switch to branch %q failed (git exit %d): %s",
    branch, exitCode, strings.TrimSpace(switchStderr))
```

`gitexec.Run` deletes the `exitCode` binding the `%d` consumes, so leaving the fragment in place is both unfillable and a duplicate of what `GitError.Error()` already renders.
The merged form is `fmt.Errorf("warp switch to branch %q failed: %w", branch, err)`.

**Exception — a `%d` that cites a *prior* call's exit code is not a duplicate and must not be dropped.** `internal/fabricengine/reconcile.go:546` and `:550` render `exitCode` from an earlier `rev-parse --abbrev-ref HEAD` call, while the error being merged belongs to a later `branch --show-current` call:

```go
return "", fmt.Errorf("rev-parse exited %d and branch --show-current exited %d", exitCode, unbornExit)
```

Deleting that fragment discards a second call's diagnostic rather than a duplicate of `GitError.Error()`.
At such a site, keep the earlier call in the raw form, or capture its `*GitError` and cite it explicitly, so the combined message survives.

### Sentinel-returning exit paths keep their sentinel identity

The merge rule above assumes both branches carry a *message*;
some carry a sentinel error instead, and `%w`-wrapping the `GitError` over the top of one would break `errors.Is` at its consumers.
The clause: the sentinel stays the `%w` verb, and the `GitError` goes in as `%v` — a shape the tree already uses:

```go
// internal/lyxcwd/lyxcwd.go:149 — already exactly this, for the exec-path error
return "", fmt.Errorf("%w: %v", ErrNotAGitRepo, err)
```

A bare `return "", Sentinel` (`lyxcwd.go:152`) may also stay bare.
Both preserve `errors.Is(err, Sentinel)` for `internal/loomengine/preflight.go:46`,
and the exact-string test assertions in `internal/lyxcwd/lyxcwd_test.go`, `internal/configcli/reconcile_test.go`, `internal/reedcli/cli_test.go` and `internal/idecli/cli_test.go`, all of which pin the bare-sentinel surface.

### Exit paths that suppress stderr deliberately keep the raw form

A class that most complicates this document's thesis: some sites withhold git's stderr *on purpose*, as a documented contract, and `%w`-wrapping a `GitError` would embed it and break them. `internal/gitrepo/pull.go` is the worked example. `Pull` and `Fetch` substitute a reproduction pointer for the raw diagnostic:

```go
// pull.go:14-17, paraphrased godoc: "raw stderr is deliberately NOT folded into the error
// (pull_test.go pins that contract), so the reproduction pointer is what keeps a nonzero exit
// diagnosable instead of a bare number."
return fmt.Errorf("gitrepo: pull --ff-only in %s: git exited %d (run `git -C %s pull --ff-only` for git's own diagnosis)", r.path, code, r.path)
```

`internal/gitrepo/pull_test.go` fails if `err.Error()` contains `"fatal:"`, and separately requires the `git -C` reproduction string.
These two sites stay raw with a `//gitexec:raw` marker citing the pinned contract.
This class is a live counter-example to "every discard is an accident of the shape" — here the discard is a considered decision with a test behind it,
and the verdict must not present every discard as an accident of the API.

### Error-constructing helpers that take stderr and cause separately must be re-signatured

The merge is not always between two `fmt.Errorf` calls at the call site — sometimes both branches call a shared helper,
and the helper's own signature encodes the two-value split the change removes:

```go
// internal/fabricengine/warpprobe.go:146 — takes stderr and cause as separate parameters,
// and picks between them: stderr wins, falling back to cause.Error(), then to a fixed string.
func wrapProbeError(weftURL, op, stderr string, cause error) error
```

Seven call paths across two functions route through it (`warpProbe`, `probeTreeHasPath`) — four pass an empty stderr with an error for the exec-path, three pass stderr with a nil error for the exit-path, so each pair collapses into one call and the helper's internal stderr-vs-cause choice becomes dead.
**Decision: re-signature the helper to `wrapProbeError(weftURL, op string, cause error) error`** and drop its detail-selection branch, since `GitError.Error()` already renders the stderr it was choosing between.
Feeding the old helper `err.Error()` as the `stderr` argument was rejected — it keeps a parameter whose reason for existing is gone, and stringifies an error that callers may want to `errors.As`.
This shape is not unique to `warpprobe.go`;
the implementation task must check every error-constructing helper the merge touches for the same split.

### Predicate recovery

For any site that needs the code back:

```go
var gitErr *gitexec.GitError
if errors.As(err, &gitErr) && gitErr.ExitCode == 1 { /* the answer, not a failure */ }
```

### The content-sniffing class — stderr content, not the exit code, decides answer-versus-failure

Two sites exist tree-wide where the non-zero branch *reads* stderr to decide whether this is an answer or a failure:

```go
// internal/fabricengine/index.go:217 — unborn HEAD is not a scan failure, it is an empty history
if strings.Contains(stderr, "does not have any commits yet") { return nil, nil }

// internal/gitrepo/push.go:64 — "no rebase in progress" means the abort had nothing to abort
if abortCode != 0 && !strings.Contains(strings.ToLower(abortStderr), "no rebase in progress") {
```

**Disposition: the checked form**, with the sniff moved onto the recovered error — `errors.As(err, &gitErr) && strings.Contains(gitErr.Stderr, "…")`.
These sites get *better* under the change: the string they inspect and the diagnostic they fall through to are now the same value, instead of a string that has to stay in scope alongside an exit code.
This is a distinct class from the pure predicate (exit code alone decides) and the mixed tri-state below (exit code decides, some codes are failures).

### The mixed tri-state class

Non-zero exit is a **load-bearing answer**, not a failure, at the pure predicate sites — but a **mixed tri-state** is different: some codes are answers and the rest are failures. `internal/gitrepo/ancestry.go` is exactly the bare-exit-code class this change exists to close:

```go
switch code {
case 0:  return true, nil                                        // answer
case 1:  return false, nil                                       // answer
default: return false, fmt.Errorf("...: git exited %d", code)    // FAILURE, stderr discarded
}
```

**Disposition: the checked form, with `errors.As` recovery** — not the raw form:

```go
_, err := r.runChecked("merge-base", "--is-ancestor", sha, ref)
if err == nil { return true, nil }
var gitErr *gitexec.GitError
if errors.As(err, &gitErr) && gitErr.ExitCode == 1 { return false, nil }
return false, fmt.Errorf("gitrepo: merge-base --is-ancestor %s %s in %s: %w", sha, ref, r.path, err)
```

This is strictly better than today: the answer codes still answer,
and the `default:` branch gains the stderr it currently throws away.

`internal/gitrepo/gitrepo.go` has two `diff --cached --quiet` sites, both mixed, but **their answer codes are inverted** — transcribe the `errors.As` recovery per site, not once:

| site | exit 0 | exit 1 | default |
|---|---|---|---|
| `:140` (`CommitEmpty`) | falls through, proceeds to commit | `return "", ErrIndexNotEmpty` | error (already binds stderr) |
| `:193` (`StageAllAndCommit`) | `return "", false, nil` — nothing to commit | falls through, proceeds to commit | error (already binds stderr) |

Both take the same treatment as `IsAncestor`, differing only in which code the `errors.As` branch tests for and what it returns.

This class takes the checked form rather than staying raw for two reasons, both rejections carried from the discussion.
Treating these sites as unmigrated debt to be swept later is rejected because they are not debt, and sweeping them would be a regression.
Folding mixed tri-states into "raw, permanently correct" is rejected because that is what an earlier draft did,
and it silently preserved a bare-exit-code failure path inside a site labelled as needing no diagnostic.

### The four outside-fabric sites — raw vs checked

| site | disposition | why |
|---|---|---|
| `internal/lyxcwd/lyxcwd.go:147` | **raw** | A predicate ("is cwd inside a git repo?") whose exit path returns the bare `ErrNotAGitRepo` sentinel; `preflight.go:46` does `errors.Is` on it and four CLI tests pin the bare-sentinel string. Gets a `//gitexec:raw` marker. |
| `internal/fabriccli/fabric.go:491` | **raw** | `branch --show-current`; a non-zero exit means "no current branch", and the path prints a usage string, not a diagnostic. Predicate. |
| `internal/websterengine/gitwrap.go:31` | **checked** | `status --porcelain`; both branches are genuine failures with real messages — a clean two-message merge. `cmd/lyx/rawgitmutation_test.go` grandfathers this file by name, naming `gitexec.RunGit`; that exemption must be updated in the same commit. |
| `internal/gitrepo/gitrepo.go:60` | **both** | The `run` helper gains a checked sibling. |

### The seven full-discard sites split into two classes

They are not one class,
and the distinction matters to the verdict.

- **Genuinely best-effort bookkeeping** — four `worktree prune` discards. `remove.go`'s own comment states it must not turn a completed removal into an error.
  These migrate as discards and stay discards.
- **Rollback and teardown on an error path** — three `checkout.go` discards (switching back to the original branch, switching back to the original weft branch, deleting the forked weft branch with `branch -D`).
  One of the three is a primitive the in-flight fabric chokepoint slice routes through an executing gate;
  once that gate is live, a discarded return can swallow a **refusal**, which is a different failure from a best-effort operation not working.
  These three must be read individually at implementation time, not swept.

**On `//nolint:errcheck`.**
The comment enforces nothing here — this repo has no `golangci-lint`, so `//nolint:errcheck` is documentation, and nothing checks either the old shape or the new one.
If deliberate discard is meant to be *visible*, the guard test is the mechanism, not the comment — the Checked-Call Invariant's justification-comment requirement is what makes each of these seven state its own case.

### Hand-read exceptions

The six paths in `internal/fabricengine/add.go` that report a fixed, wrong string (`"cwd is not a valid git worktree"`) for failures unrelated to the cwd,
and the three rollback discards in `checkout.go` named above (one of which becomes a gate-refusal path under the chokepoint slice).
A shape change alone does not remove a wrong cause, so these are read individually rather than swept.

## Site inventories — shapes and regeneration queries

> **Snapshot, not a coordinate list.** Every file:line below was measured on 2026-08-10 (2026-08-11 for the `warpprobe`/content-sniff queries) against `main` at `c52faee4`.
> The implementation runs behind the serialised fabric chain, which rewrites this exact code, so this section records shapes plus the query that finds them, and the implementer **re-derives** the lines.
> A concrete instance of why the coordinates rot: `checkout.go:193` is a `git branch -D` call and one of the seven discard sites, and it is one of the five primitives the in-flight chokepoint slice routes through an executing gate — after which it returns a *refusal* that the current `_, _, _, _ =` swallows.
> That line will not look like this when the migration runs.
> **The acceptance bar for this section is that it is re-derivable from the doc alone, not executable from it.**

### Production call-site count

74 total, excluding tests and the declaration:

| package | sites |
|---|---|
| `internal/fabricengine` | 70 |
| `internal/gitrepo` | 1 |
| `internal/websterengine` | 1 |
| `internal/lyxcwd` | 1 |
| `internal/fabriccli` | 1 |

R5 counted 74 in fabric where this count finds 71 (70 `fabricengine` + 1 `fabriccli`);
code changed between the counts and nothing turns on the difference.

### The four outside-fabric sites

- `internal/gitrepo/gitrepo.go:60` — the `run` helper (see the `gitrepo` fan-out below).
- `internal/websterengine/gitwrap.go:31` — `status --porcelain`, wrapping `gitexec` directly because `gitrepo.Repo` exposes no porcelain method.
- `internal/lyxcwd/lyxcwd.go:147` — `rev-parse --show-toplevel`.
- `internal/fabriccli/fabric.go:491` — branch read.

### The `gitrepo` fan-out

**21 production `r.run(...)` call sites** across `gitrepo.go`, `push.go`, `pull.go`, `reset.go`, `ancestry.go`.
**Regeneration query:** `grep -rn 'r\.run(' --include='*.go' internal/gitrepo | grep -v _test` for the total, and for the discard subset match any `_` in the second (stderr) binding position — `^\s*\S+,\s*_,\s*\S+,\s*\S+\s*:?=\s*r\.run\(`.
Do not use `_, _, .*= r\.run(`: it returns four, missing `push.go:133` (`stdout, _, code, err`), which discards stderr while binding stdout.

**Five discard stderr:**

| site | why it discards | disposition |
|---|---|---|
| `pull.go:19` (`Pull`) | deliberate, godoc-documented, `pull_test.go:87` fails if the error contains `"fatal:"` | **raw** |
| `pull.go:33` (`Fetch`) | same contract, same pinning | **raw** |
| `push.go:133` (`HasUnpushed`) | pure predicate — non-zero folds into `(true, nil)` | **raw** |
| `ancestry.go:26` (`IsAncestor`) | mixed tri-state; its `default:` branch is a real failure that should carry stderr | **checked** + `errors.As` |
| `reset.go:18` (`ResetHard`) | no stated reason — an ordinary discard | **checked** |

The remaining sixteen bind stderr and thread it into an error message.
This is why "four sites outside fabric" understates the shape's reach — behind one of those four sits a second fan-out of 21.

### The predicate-site inventory — non-zero exit as an answer

**Every entry below is keyed to the `RunGit` call site**, with its comparison line given as `→ :<n>`.

**`rev-parse` existence and state probes — 8 call sites:**

| call site | command | comparison |
|---|---|---|
| `fabricengine/add.go:58` | `rev-parse --verify refs/heads/<warp>` | `→ :62` — `if exitCode == 0`, branch already exists |
| `fabricengine/boardweft.go:25` | `rev-parse --verify --quiet refs/heads/<warp>` | `→ :32` — `if exitCode == 0`, adopt local weft branch |
| `fabricengine/weftwiring.go:90` | `rev-parse --verify refs/heads/<branch>` | `→ :96` — `return exitCode == 0`; the function *is* `weftBranchExists` |
| `fabricengine/weftwiring.go:73` | `rev-parse --is-inside-work-tree` | `→ :78` — `return exitCode == 0` |
| `fabricengine/clone.go:433` | `rev-parse --verify --quiet <remoteRef>` | remote primary-branch probe |
| `fabricengine/clone.go:472` | `rev-parse --verify --quiet refs/heads/<branch>` | `→ :476` — `if exitCode == 0` |
| `fabricengine/warpprobe.go:77` | `rev-parse --verify --quiet HEAD` | `→ :81` — unborn HEAD; returns `warpProbeResult{Found: false, WeftLooksLikeWeft: true}, nil` |
| `fabricengine/pull.go:131` | `rev-parse --abbrev-ref --symbolic-full-name @{u}` | `→ :135` — `return code == 0, nil`; the godoc states non-zero "is the nothing-to-pull-from case, never an error" |

`warpprobe.go:77` is the only predicate in `warpprobe.go`.
The comparisons at `:71`, `:95` and `:136` look identical but return `wrapProbeError(...)` — they are error paths, misfiled as predicates by an earlier classifier that recognised only `fmt.Errorf` / `errors.New`.

**`internal/gitrepo/push.go:133`** — `rev-list --count @{u}..HEAD` in `HasUnpushed`;
`→ :136` returns `true, nil` on a non-zero exit, with the godoc stating "rev-list errors fold into `(true, nil)`" (no upstream configured is treated as unpushed so the first push still happens).
A pure predicate,
and it stays raw.
It was previously listed only among the discard sites and never in the shape list, which is a reminder that `gitrepo`'s 21 sites need per-site raw/checked dispositions, not a count — the implementation task must classify each one.

**`gitrepo` tri-states and quiet probes — 3 call sites** (all mixed, so all take the checked form with `errors.As` recovery — see [the mixed tri-state class](#the-mixed-tri-state-class) above):

`internal/gitrepo/ancestry.go:26` — `merge-base --is-ancestor`, with the tri-state stated in the method's own godoc.

`internal/gitrepo/gitrepo.go:140` and `:193` — `diff --cached --quiet`, where exit 1 means "index is dirty" and maps to `ErrIndexNotEmpty`.

**`internal/lyxcwd/lyxcwd.go:147`** — `rev-parse --show-toplevel`;
`→ :151` returns the bare `ErrNotAGitRepo` sentinel.

**Unclassified, to be re-read when the inventory is regenerated:** `cleanup.go:280`, `reconcile.go:271`, `reconcile.go:300`, `reconcile.go:534`, `prune.go:218`, `prune.go:266`, `checkout.go:77`.
The classifier put them in the non-error-returning column;
none has been read individually.

### The aggregate-classification numbers

**Regeneration query (approximate).**
Walk each non-test `.go` file under `internal/fabricengine`, match `\b(exitCode|code|unbornExit|statusExit)\s*(!=|==)\s*0\b`, and for each hit inspect the following ~8 lines for any error-returning branch.
Two traps, both hit in practice:

- Do **not** restrict the match to lines containing `if` or `switch` — that filter drops the bare `return <code> == 0` predicates and multi-line `if` continuations, and reported 59 instead of 63.
- Do **not** look only for `fmt.Errorf` / `errors.New` — `warpprobe.go` constructs errors through the `wrapProbeError` helper, so some comparisons were misfiled as predicates.
  Match helper constructors too (`wrap\w*Error`, `Error(`, a bare `err` return).

Current result: 63 total, ~51 error-returning, ~12 predicate.
**Treat this as approximate.**
The classifier cannot see every error-constructing helper,
and the decision does not rest on it — the shape list above does.
Any future pass must assume there is another error-constructing helper it has not accounted for.

### The two-message-merge regeneration query

For each `gitexec.RunGit(` call in a non-test file under `internal/fabricengine`, inspect the following window (~20 lines, to the next call or function end) and count it as a merge site when the window contains **both** an `err != nil` guard and an exit-code comparison.
A coarse 22-line window returns 63 of 70;
the careful count that respects block boundaries returns ~51.
Re-derive before quoting.

> **Two different 51s — do not conflate them.** The ~51 two-message merge sites and the ~51 error-returning exit-code comparisons from the predicate classification are separate measurements over different units (call sites vs comparisons) that happen to land on the same number.
> Neither confirms the other, and a future pass that treats one as corroborating the other is reasoning from a coincidence.

### The full-discard query

`grep -rn "_, _, _, _ *= *gitexec.RunGit\|^\s*gitexec.RunGit(" --include="*.go" internal/ | grep -v _test`.

### The error-constructing-helper query

`grep -n 'wrapProbeError(' internal/fabricengine/warpprobe.go`, excluding the declaration.
Seven call paths across two functions (2026-08-11 snapshot) — `warpProbe` and `probeTreeHasPath`.
Four pass an empty stderr with an error for the exec-path, three pass stderr with a nil error for the exit-path.

### The content-sniff query

`grep -rn 'Contains(stderr\|Contains(.*Stderr' --include='*.go' internal/ | grep -v _test` (2026-08-11).
Two exist tree-wide — see [the content-sniffing class](#the-content-sniffing-class--stderr-content-not-the-exit-code-decides-answer-versus-failure) above.

## The guard test and its invariant

Neither the gitexec Checked-Call Invariant nor its guard test is written by this task — both land in the implementation commit, and `CONSTRAINTS.md` is not edited here, because an invariant with no enforcing test is exactly the rot that file exists to prevent.

**Marker-comment form.**
Every remaining raw `gitexec.RunGit` / `gitrepo.run` call site carries an adjacent marker comment, `//gitexec:raw — <why the raw form is correct here>`.
The justification is "why the raw form is correct here", not the narrower "why non-zero is not a failure" — the narrower wording is unfillable at the deliberate-suppression sites, where non-zero *is* a failure whose stderr is deliberately withheld.
Since this wording lands in `CONSTRAINTS.md`, it has to cover both raw classes:

- **pure predicate** — every exit code is an answer (`rev-parse --verify` existence checks, `return exitCode == 0`, `HasUnpushed`);
- **pinned deliberate-suppression contract** — non-zero is a failure, but folding stderr into the message would break a documented, test-enforced surface (`gitrepo/pull.go`'s `Pull` and `Fetch`).

**Keyed on the marker comment, not on a location.**
The guard asserts that every raw call site has the marker, and separately pins a **per-package count** of raw sites as a drift tripwire.
There is no file:line list and no enclosing-function list.
Location keys rot on every unrelated edit above the line — the exact staleness the [site inventories](#site-inventories--shapes-and-regeneration-queries) section designs around — and an enclosing-function key churns on renames while still not enforcing the justification.
Keying on the marker makes the justification requirement *be* the enforcement rather than a convention standing beside it,
and the count keeps "a new raw site appeared" a visible diff.

**Test files are exempt from this invariant.**
Roughly 50 `*_test.go` sites use `RunGit` for fixture setup where exit status is legitimately irrelevant;
demanding a written justification at each is ceremony with no design weight.
Test-side coverage comes instead from the three token guards below, which must learn the new entry point.

**Pattern to mirror.** `cmd/lyx/gitrepoboundary_test.go` (`TestGitrepoBoundary_PinnedRunCallSites`) and `internal/gitrepo/noforceadd_test.go` are the two worked examples already in the repo — this needs no new machinery.
This repo has no external linter, so "lint rule" here means a guard test.

**Composition with the gitrepo Client Boundary Invariant.**
After this change two set-equality guards assert over overlapping sets in `internal/gitrepo`,
and each invariant's `CONSTRAINTS.md` entry must carry a one-line cross-reference to the other, so a future edit does not have to guess from whichever test fails first.
The **Client Boundary Invariant** answers *which `gitrepo` methods may reach the git CLI at all* and is keyed by **method name** — update it when a method gains or loses a `run`/`gitexec` call.
The **Checked-Call Invariant** answers *which call sites may use the raw, unchecked form* and is keyed by **call site** — update it only when a site moves between the raw and checked forms.
A new `gitexec` call inside an already-pinned method trips the second and not the first;
a new method reaching the CLI trips both.

**The three Client Boundary assertions that break in the implementation commit, stated as fact rather than risk.** `cmd/lyx/gitrepoboundary_test.go:174` asserts exactly one non-comment `gitexec.` occurrence in all of `internal/gitrepo` non-test source, and `:177` requires that one occurrence to sit inside `run`'s body — `runChecked` calling `gitexec.Run` makes it two and fails both, because `runChecked` is decided to call `gitexec.Run` directly as a second chokepoint beside `run`, not to wrap `run`.
The assertions become "exactly two, one inside `run` and one inside `runChecked`". `:167` runs set-equality on `gitrepoPinnedRunBoundMethods`, keyed on methods containing `r.run(`;
any method migrated from `r.run` to the checked sibling silently drops out of that set and trips the diff.

**Three further guards key on the literal token `gitexec.RunGit` and go blind to `gitexec.Run`,** since `gitexec.Run(` does not contain that substring — each must gain the new token in the same commit or its invariant is silently holed:

- `cmd/lyx/tierpurity_test.go:54` (`bannedTokens`) — Test Tier Purity Invariant;
  without it an untagged test can spawn git through the new entry point.
- `cmd/lyx/hermeticenv_test.go:49` (`gitSpawnTokens`) — Hermetic Git Test Environment Invariant;
  without it a non-hermetic package escapes the check.
- `cmd/lyx/rawgitmutation_test.go:37` — Fabric Git Invariant raw-git-mutation guard;
  without it a fabric-bypassing raw call goes undetected.
  This file grandfathers `internal/websterengine/gitwrap.go`, one of the four outside-fabric sites,
  and that grandfather exemption must be updated in the same commit.

**Known blind spot, inherited from the sibling invariant:** set-equality on call sites does not catch a raw call slipped into an already-pinned function;
per-call review still applies there.

## The implementation task

Slug `gitexec-checked-entry-point`, `depends_on: ['fabric-corrindex-record-race']`.
The slug names what is built rather than what was decided.
The dependency hangs it off the tail of the serialised fabric chain (`fabric-destructive-chokepoint` → `fabric-live-state-harness` → `fabric-mutation-record-envelope` → `fabric-corrindex-record-race`), because the implementation rewrites 70 call sites in `internal/fabricengine`, the exact package that chain is serialised to protect from concurrent edits.

**Corrected size estimate.**
The original "55 discard sites need judgement, the rest is a sweep" cost model is close to inverted — the sites needing no thought are the small group, and roughly 51 need a message decision precisely *because* they already carry two messages.

**Two scope exclusions.**
The implementation does not re-review the wording quality of the sites the crucible already fixed — that judgement was made site by site and re-litigating it is a second campaign.
The six fixed-wrong-cause paths in `add.go` remain a separate hand-read exception, because a shape change alone does not remove a wrong cause.

**Hand-off note.**
The implementation task deletes this doc and removes the roadmap's link to it **in the same commit**, or Markdown Link Integrity fails on a dangling relative link.

## Related

- [internal/fabricengine](../../internal/fabricengine/doc.go) package documentation — the four fabric-local classes from the same campaign, scoped as slices 12-15 and now landed.
  This one was split out because `internal/gitexec` is shared by every module that touches git, so its blast radius is much larger than theirs.
- [CONSTRAINTS.md](../../CONSTRAINTS.md#gitrepo-client-boundary-invariant) — the gitrepo Client Boundary Invariant pins which `gitrepo` methods reach `gitexec`;
  that pinned list is the starting inventory for the outside-fabric caller count.
