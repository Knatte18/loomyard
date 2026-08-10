# gitexec: should `RunGit` return a typed error carrying git's stderr?

> **Status: a decision, not a plan.**
> This item exists to produce a verdict — "change the shape", "add a second entry point", "enforce by lint", or "decided not worth it, recorded" — before any implementation is scoped.
> Deleted once the verdict is recorded, wherever it lands.

Filed as GitHub issue #145 (`question`) by the fabric v2 crucible campaign's orchestrator, and folded into the manifest here;
the issue is closed, pointing at this file.

## The observation

`gitexec.RunGit` returns git's stderr as an ordinary return value alongside stdout, an exit code and an error.
Because discarding a return value is easier than using it, callers discard it — and they did, at **55 of 74 call sites in fabric alone**.

The individual sites are fixed (crucible R5).
The signature that produced them is not.

## Evidence

Crucible R5 swept error handling across `internal/fabricengine`, `internal/fabriccli` and `internal/lyxcwd` — 58 non-test files — and enumerated every `gitexec.RunGit` call site and its stderr disposition.
The enumeration method is reproducible shell, stated in the round's report, and the totals were checked against an orchestrator ground-truth count made before the round was spawned.

- **74** `RunGit` call sites.
- **55** discard git's stderr entirely.
- **33** of those turn a failure into a bare exit code with no explanation at all.
- **6** paths in `add.go` replaced the real cause with a fixed, wrong string — `"cwd is not a valid git worktree"`, reported for failures that had nothing to do with the cwd.

For contrast, the same sweep found the *silent-swallow* class genuinely closed: of 388 `if <err> != nil` blocks walked, **zero** failed to return, wrap, or record the error, and 48 of 52 deliberate discards were judged correct as written with the reasoning documented in the code.

So the codebase is disciplined about **not dropping errors** and undisciplined about **keeping git's explanation of them**.
That asymmetry tracks exactly the difference between an error value — which Go's tooling and conventions push you to handle — and a plain string return, which nothing pushes you to handle.

## Why the current shape invites the mistake

```go
stdout, stderr, exitCode, err := gitexec.RunGit(args, dir)
```

`err` is non-nil only when git could not be executed at all.
A git command that *ran* and *failed* returns `err == nil` with a non-zero `exitCode`, and its explanation is in `stderr` — a string the caller must remember to thread into its own error message.
Sites that only care about success write `_, _, exitCode, err := gitexec.RunGit(args, dir)`, and the diagnostic is gone at the point of the failure, not later.
The operator then sees `git exit 128` and nothing else.

There was also a live contradiction in the tree about whether stderr should surface at all.
Two older tests (`TestList_NotAGitRepo`, `TestCloneRepo_InvalidURLFails`) asserted git's stderr must **never** appear in a fabric error, while `TestCheckout_WarpSwitchFailureCarriesGitStderr` (added by crucible R1) asserted the opposite — and 19 sites already printed it.
R5 resolved this in fabric's favour: the operator needs both, because *fabric's* message says WHERE and *git's* says WHY.
Worth knowing the question was live in the codebase, not settled by default.

## Sketch of the change

Have `RunGit` return an error for a non-zero exit, with the diagnostic attached:

```go
type GitError struct {
    Args     []string
    Dir      string
    ExitCode int
    Stderr   string
}
func (e *GitError) Error() string  // includes exit code and trimmed stderr
```

A caller that wants the old behaviour must reach for the typed error deliberately (`errors.As`), which is the right amount of friction: throwing the diagnostic away becomes a visible decision rather than the default.

## The questions this item exists to answer

- **Is a breaking signature change acceptable**, or should a second entry point (`RunGitE`, or similar) be added and callers migrated incrementally?
  Incremental migration avoids a big-bang change but leaves the footgun loaded for anyone who does not migrate.
- ~~**Which callers outside fabric exist, and how many are affected?**~~ **Answered — see [First deliverable, closed](#first-deliverable-closed--measured-2026-08-10) below.**
- **Does the wrapping belong in `gitexec` at all**, or should it stay a caller responsibility with a lint rule enforcing it?
  A lint rule is cheaper and less invasive, but only catches what it is written to catch.
- **How does this interact with the sites where discarding is correct?**
  Best-effort teardown (`gitexec.RunGit([]string{"worktree","prune"}, ...)` with `//nolint:errcheck`) is deliberate and documented in several places, and any change must keep that expressible without ceremony.

## First deliverable, closed — measured 2026-08-10

The caller count outside fabric was this item's stated precondition.
Production call sites, excluding tests and the declaration itself, 75 total:

| package | sites |
|---|---|
| `internal/fabricengine` | 70 |
| `internal/gitrepo` | 2 |
| `internal/websterengine` | 1 |
| `internal/lyxcwd` | 1 |
| `internal/fabriccli` | 1 |

**Five production call sites exist outside fabric.**
The blast radius is almost entirely the module the crucible already swept, which is the opposite of what "shared infrastructure" suggests and weakens the counter-argument below accordingly.
Tests add 50 more — `fabricengine` 15, `cmd/lyx` 10, `gitrepo` 10, `configcli` 5, `gitexec` 4, and a few elsewhere — which migrate with the signature but carry no design weight.
`internal/boardengine` and `internal/githubclient`, named above as places to look, have **no** production call sites.

R5 counted 74 in fabric where this count finds 71 (70 in `fabricengine` plus 1 in `fabriccli`);
code changed between the two counts, and nothing in the argument turns on the difference.

### The exit code is provably redundant

Every exit-code comparison in `internal/fabricengine` is against zero — all 63 of them:

```
44  exitCode != 0        8  exitCode == 0
 7  code != 0            2  code == 0
 1  unbornExit != 0      1  statusExit != 0
```

No call site reads a specific exit code.
`exitCode` therefore carries nothing `err != nil` does not already carry, so the verdict does not have to weigh whether to keep it in the signature.
The code has answered that one.

### The migration splits unevenly, and the split should inform the verdict

- **Mechanical, whole-tree sweep** — dropping `exitCode` and rewriting `if exitCode != 0` to `if err != nil`.
  Safe across all 75 sites given the finding above;
  `gofmt -r` or a small AST tool handles both the binding and the condition, and the five full-discard sites (`_, _, _, _`) come along for free.
- **Per-site judgement, not automatable** — the 55 sites that discard stderr today.
  The question at each is what the operator should see when it fails, which is the entire point of the item.
  The 6 paths in `add.go` that substitute a fixed wrong string must each be read.

Sites that do bind stderr today follow one uniform shape: each named `*Stderr` variable appears exactly twice, once bound and once used in an error message, and does nothing else with it.

### Sequencing follows from the split

The **decision** is independent of the fabric chain and can be taken at any time — it reads call sites and writes a verdict, touching no production code.

The **implementation**, if the verdict is a signature change, cannot.
It rewrites 70 call sites in `internal/fabricengine`, the package [fabric-crucible-followups.md](fabric-crucible-followups.md)'s chain is serialised to protect from concurrent edits.
File it as its own task behind that chain rather than folding it into this one.

## The counter-argument, weighed rather than dismissed

This touches shared infrastructure to fix a **diagnostic-quality** problem, not a correctness one.
No data was lost because of a missing stderr.
That is a legitimate reason to decide "not worth it".

The argument for doing it anyway is that the 55/74 number is not an accident of who wrote those lines — it is what the API shape produces.
R5 fixed the sites;
without a shape change, the next module to use `gitexec` starts the count over from zero.

Either way it should be **decided and written down**, which is what this item is for.

## Related

- [fabric-crucible-followups.md](fabric-crucible-followups.md) — the four fabric-local classes from the same campaign, scoped as slices 12-15.
  This one was split out because `internal/gitexec` is shared by every module that touches git, so its blast radius is much larger than theirs.
- [CONSTRAINTS.md](../../CONSTRAINTS.md#gitrepo-client-boundary-invariant) — the gitrepo Client Boundary Invariant pins which `gitrepo` methods reach `gitexec`;
  that pinned list is the starting inventory for the outside-fabric caller count.
