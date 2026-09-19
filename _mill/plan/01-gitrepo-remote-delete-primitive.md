# Batch: gitrepo-remote-delete-primitive

```yaml
task: 'fabric: no remote/GitHub branch deletion'
batch: 'gitrepo-remote-delete-primitive'
number: 1
cards: 2
verify: go test ./internal/gitrepo/... && go test -tags integration ./internal/gitrepo/...
depends-on: []
```

## Batch Scope

This batch delivers the one new git primitive the whole task rests on: a `gitrepo` method that deletes a branch on a named remote, distinguishing "removed it" from "there was nothing there" from "it failed".
It is one batch because the method and the three outcomes its tests pin are a single unit with no dependency on anything in `fabricengine`, and because everything downstream needs its exact signature and its exact idempotence contract before it can be written.

External interface batch 2 consumes: `func (r *Repo) DeleteRemoteBranch(remote, branch string) (deleted bool, err error)` on `*gitrepo.Repo`.
`deleted == false, err == nil` means the remote ref was already absent.

Batch-local decisions beyond the overview's Shared Decisions:

- The method lands in `internal/gitrepo/push.go` rather than a new file, because `push.go` already owns every push-shaped remote call and `PushRebaseFree` is the direct structural model for it.
- The tests are written first within card 2's own work, per the discussion's TDD call for this method;
  card 1 lands the method and card 2 lands its tests, which is commit ordering, not a weakening of that call — the assertions come from the discussion's three named outcomes, not from reading the finished body.

## Cards

### Card 1: DeleteRemoteBranch on gitrepo.Repo

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/remote.go`
  - `internal/gitexec/gitexec.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/gitrepo/push.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add an unexported package-level constant next to the existing `rebaseRetryTriggers` declaration, named `remoteRefAbsentTrigger`, whose value is the exact string `remote ref does not exist`.
  Give it a doc comment recording that git's fuller wording (`error: unable to delete '<branch>': remote ref does not exist`) contains it verbatim, so one substring is enough and no list is needed, and that a future git rewording must fail the test that pins it rather than silently reclassify the common case as an error.

  Add the exported method `DeleteRemoteBranch` on `*Repo` with the signature `func (r *Repo) DeleteRemoteBranch(remote, branch string) (deleted bool, err error)`.
  Implement it as: call `r.runChecked("push", remote, "--delete", branch)`;
  on a nil error return `(true, nil)`;
  otherwise recover a `*gitexec.GitError` with `errors.As` and, when that succeeds and `strings.Contains` finds `remoteRefAbsentTrigger` in the recovered value's `Stderr`, return `(false, nil)`;
  in every other case return `(false, fmt.Errorf("gitrepo: git push --delete: %w", err))`.

  Use `runChecked`, never `run` — this call has no reason for a `//gitexec:raw` pin, and the gitexec Checked-Call Invariant makes `runChecked` the default.

  The doc comment must state the idempotence contract in the terms the caller relies on: a remote ref that does not exist is reported as `(false, nil)` — an idempotent success recording no mutation and surfacing no error — because every executor in fabric's destruction gate is idempotent for an already-absent target and the common case is a branch that was never pushed.
  It must state that `deleted` is returned separately from `err` precisely so the caller can tell "removed it" from "there was nothing there", which is what the caller's mutation record needs.

  Per the overview's fabric-vocabulary-stops-at-gitrepo decision, neither the constant, the method, its parameters, its doc comment, nor its error strings may contain the words `weft` or `warp`.
  The existing package error-string prefix `gitrepo: ` applies.
- **Commit:** `feat(gitrepo): add DeleteRemoteBranch with absent-ref idempotence`

### Card 2: Integration tests for the three outcomes

- **Context:**
  - `internal/gitrepo/push.go`
  - `internal/gitrepo/push_test.go`
  - `internal/gitrepo/fetch_integration_test.go`
  - `internal/gitrepo/testmain_test.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/gitrepo/deleteremotebranch_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create the file with a leading `//go:build integration` line, a file-header comment naming what it covers, and `package gitrepo_test`, matching `internal/gitrepo/fetch_integration_test.go`'s own shape.
  It spawns git, so an untagged file would violate the Test Tier Purity Invariant;
  the package's `TestMain` already calls `gitkit.HermeticGitEnv()` in `internal/gitrepo/testmain_test.go`, so no new `TestMain` is added.

  Reuse the existing bare-remote and clone fixture helpers `newBareRemote`, `newRepoWithRemote`, `cloneFromBare`, `runGit`, `writeFile`, and `commitAll` already declared in `internal/gitrepo/push_test.go`, exactly as `internal/gitrepo/fetch_integration_test.go` reuses them.
  Declare no duplicate fixture helpers.

  Three tests, one per outcome the discussion names:

  1. Deleting a branch that exists on the remote returns `(true, nil)`, and a subsequent `git ls-remote --heads` against the bare remote no longer lists the ref.
     Build the state by creating a second branch in the clone, pushing it, and asserting the bare remote holds it before the call.
  2. Deleting a branch that is absent from the remote returns `(false, nil)` — the idempotence contract.
     This must run a real `git push --delete` against a ref that genuinely is not there, so the test observes git's own stderr rather than a fixture.
     The test's own comment must record that this is the tripwire on the single pinned substring `remote ref does not exist`: a future git rewording makes this test fail loudly instead of silently reclassifying the common case as an error.
  3. A genuine failure returns a non-nil error and `deleted == false`.
     Induce it without a network by pointing the clone's remote at a filesystem path that does not exist, then calling the method.
     Assert the returned error is non-nil and that `deleted` is false;
     do not assert on git's exact failure wording, which is not pinned by any decision.

  Every test name states the outcome it pins, in this package's existing `TestX_Condition_Result` style.
  Per the overview's fabric-vocabulary-stops-at-gitrepo decision, use branch names with no `weft` or `warp` token — `feature-x`, `gone-branch`, and similar.
- **Commit:** `test(gitrepo): pin DeleteRemoteBranch's delete, absent-ref, and failure outcomes`

## Batch Tests

`verify:` runs `go test ./internal/gitrepo/...` (the untagged tier, which for this package is `keyvalidation_test.go` alone and does no git spawning) and then `go test -tags integration ./internal/gitrepo/...`, which is where every test this batch adds actually runs.
Both halves are needed: card 1 changes a production file the untagged tier compiles against, and card 2's new file is `integration`-tagged, so an untagged-only run would never execute a single new assertion.

The three assertions are exactly the three outcomes `_mill/discussion.md`'s Testing section names for this method, and the absent-ref case is the one most likely to regress, since it is the only one whose correctness depends on a string match against git's stderr.

No test here reaches `fabricengine`: this batch's whole surface is one `gitrepo` method, and the gate that will call it does not exist until batch 2.
