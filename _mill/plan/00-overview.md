# Plan: gitexec: add the checked entry point and migrate the call sites

```yaml
task: 'gitexec: add the checked entry point and migrate the call sites'
slug: gitexec-checked-entry-point
approved: false
started: '20260813-142251'
parent: main
root: ""
verify: go build ./... && go vet ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: gitexec-checked-entry-point
    file: 01-gitexec-checked-entry-point.md
    depends-on: []
    verify: go build ./... && go test ./internal/gitexec/... && go test -tags integration ./internal/gitexec/...
  - number: 2
    name: gitrepo-checked-pair
    file: 02-gitrepo-checked-pair.md
    depends-on: [1]
    verify: go build ./... && go test ./internal/gitrepo/... ./cmd/lyx/... && go test -tags integration ./internal/gitrepo/...
  - number: 3
    name: fabric-destroy-executors
    file: 03-fabric-destroy-executors.md
    depends-on: [1]
    verify: go build ./... && go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
  - number: 4
    name: fabric-destroy-caller-files
    file: 04-fabric-destroy-caller-files.md
    depends-on: [3]
    verify: go build ./... && go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
  - number: 5
    name: fabric-probe-clone-reconcile
    file: 05-fabric-probe-clone-reconcile.md
    depends-on: [4]
    verify: go build ./... && go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
  - number: 6
    name: fabric-remaining-sites
    file: 06-fabric-remaining-sites.md
    depends-on: [5]
    verify: go build ./... && go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
  - number: 7
    name: outer-call-sites
    file: 07-outer-call-sites.md
    depends-on: [1]
    verify: go build ./... && go test ./internal/lyxcwd/... ./internal/fabriccli/... ./internal/websterengine/... ./internal/configcli/... ./internal/reedcli/... ./internal/idecli/... ./internal/loomengine/... && go test -tags integration ./internal/lyxcwd/... ./internal/fabriccli/... ./internal/websterengine/...
  - number: 8
    name: checked-call-invariant-and-docs
    file: 08-checked-call-invariant-and-docs.md
    depends-on: [2, 6, 7]
    verify: go test ./... && go test -tags integration ./...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: the-two-shape-split

- **Decision:** `gitexec.Run(args []string, cwd string) (string, error)` is added beside `gitexec.RunGit`, which stays byte-for-byte unchanged in name, signature, and semantics.
  `internal/gitrepo` gains `runChecked` beside `run`.
  Neither raw form is deprecated, renamed, or marked legacy.
- **Rationale:** the raw form stays permanently correct at sites where a non-zero exit is an answer rather than a failure;
  giving the short name to the must-succeed form makes the safe path the default one.
- **Applies to:** all batches

### Decision: giterror-shape

- **Decision:** `GitError` carries exactly `Args []string`, `Dir string`, `ExitCode int`, `Stderr string`, and is always returned as `*GitError`.
  `Error()` renders `git <args>: exit <code>: <trimmed stderr>`, omitting the trailing `: <stderr>` segment entirely when the trimmed stderr is empty.
  Args are rendered space-separated, `%q`-quoted only when an arg is empty or contains whitespace.
  No `Stdout` field, no redaction of `Args`.
- **Rationale:** four fields are what every merged message needs and no more;
  stdout always arrives in `Run`'s first return value, which is what makes omitting the field coherent.
- **Applies to:** all batches

### Decision: exec-failures-unwrapped

- **Decision:** when git cannot be executed at all, `Run` returns the raw underlying error, never wrapped in a `GitError`.
  `*GitError` is produced only when git ran and exited non-zero, so `errors.As(err, &gitErr)` means precisely "git ran and rejected this".
- **Rationale:** every predicate-recovery site and the destructive-fallback split in batch 4 depend on that distinction being exact.
- **Applies to:** all batches

### Decision: default-merge-rule

- **Decision:** at a site where `err != nil` and `exitCode != 0` are separate guards with separate messages, the exit-path message wins: `%s`-of-stderr becomes `%w`-of-error, the exec-path message is dropped, and the `(git exit %d)` / `exited %d` fragment is deleted together with its `exitCode` argument.
  Before applying it at any site, check the site against the four carve-outs below;
  the rule presumes the exit branch is a *message*.
- **Rationale:** the exit-path message is the one written for the failure operators hit, and the returned error's own text now supplies what the exec-path message used to say.
- **Applies to:** all batches

### Decision: merge-rule-carve-outs

- **Decision:** the default merge rule does NOT apply at four kinds of site, each of which is called out per card in the batch that owns it:
  1. the exit branch is control flow, not a message (batch 4's `remove.go` and `prune.go` executor call sites) — split on `errors.As`, never merge;
  2. the failure sink is a plain `string` field, so `%w` is unavailable (batch 4's `cleanup.go` `entry.Error` assignments) — use `%v` of the error, still dropping the exit-code fragment;
  3. the message cites a **prior** call's exit code or stderr (batch 2's `pushWithRebaseRetry`, batch 4's `prune.go`, batch 5's `readBranch`) — keep the prior call's `*GitError` in scope and fill the fragment from it;
  4. the branch returns a sentinel whose identity `errors.Is` consumers depend on (batch 2's `PushRebaseFree` and batch 7's `lyxcwd.gitWorktreeRoot`) — the sentinel keeps the `%w` verb.
- **Rationale:** a mechanical merge at (1) routes an exec failure or a gate refusal into a destructive primitive;
  at (2) `%w` inside `fmt.Sprintf` renders `%!w(…)` rather than failing the build;
  at (3) it discards a second call's diagnostic;
  at (4) it breaks `errors.Is` at a production consumer.
- **Applies to:** all batches

### Decision: the-raw-vs-checked-discriminator

- **Decision:** the single question asked before any other classification is **does the site report the exec path separately from the exit path?**
  If yes, the site takes the checked form, with `errors.As` recovery wherever the exit path is an answer.
  If no, it may be raw, and only within three truthfully-markable classes: no error channel in the signature;
  test-pinned deliberate suppression;
  the raw half of the `gitrepo` pair itself.
- **Rationale:** a `//gitexec:raw` marker at a site that already returns an error on the exec path would have to assert something the code contradicts, which turns the justification requirement into a formality.
- **Applies to:** all batches

### Decision: the-five-raw-sites-and-their-pinned-counts

- **Decision:** exactly five raw sites survive the migration, and the per-package pinned map is `internal/gitrepo` 3 (`run`'s own body, `Pull`, `Fetch`), `internal/fabricengine` 2 (`weftRepoExists`, `weftBranchExists`), `internal/lyxcwd` 0, `internal/fabriccli` 0, `internal/websterengine` 0.
  Each raw site carries an adjacent `//gitexec:raw — <why the raw form is correct here>` marker, added in the batch that owns its file.
  A package with a pinned zero is listed explicitly rather than omitted.
- **Rationale:** the pinned count is a drift tripwire and the marker is the justification;
  a zero listed explicitly states "this package has no raw sites, deliberately" rather than leaving it to be inferred from an absence.
- **Applies to:** 02-gitrepo-checked-pair, 04-fabric-destroy-caller-files, 08-checked-call-invariant-and-docs

### Decision: deliberate-discards-migrate-as-discards

- **Decision:** the four best-effort `worktree prune` discard sites (`prune.go` ×2, `reconcile.go`, `remove.go`) migrate to `_, _ = gitexec.Run(…)`, and each gains its own comment stating why discarding is correct there.
  Only `prune.go`'s two sites carry a `//nolint:errcheck` comment today, and those two are deleted;
  the `reconcile.go` and `remove.go` sites already discard via the bare `_, _, _, _ =` form with no such comment, so they need the why-comment added and nothing removed.
  They are NOT `//gitexec:raw` sites and must not appear in the pinned map's counts.
- **Rationale:** they use the checked form and discard its error;
  a raw marker would be a lie and would inflate the raw-site counts.
- **Applies to:** 04-fabric-destroy-caller-files, 05-fabric-probe-clone-reconcile

### Decision: no-source-file-reads-outside-a-card-s-lists

- **Decision:** every card's `Context:` is an allowlist.
  The implementer re-derives its own file:line coordinates inside the files a card names, using the regeneration queries recorded in `_mill/discussion.md`'s "Technical context" section, and never widens the read set beyond the card's `Context:` + `Edits:` + `Creates:`.
- **Rationale:** the discussion deliberately records shapes and dispositions rather than a coordinate list, because coordinates go stale;
  re-derivation inside a bounded file set is what keeps that safe.
- **Applies to:** all batches

### Decision: verification-runs-both-tiers-per-batch

- **Decision:** every batch's `verify:` runs `go build ./...` plus the Tier 1 (untagged) and Tier 2 (`-tags integration`) suites of the packages that batch touches, not only at the end.
  The module-wide `verify:` in this overview's frontmatter is `go build ./... && go vet ./...`.
- **Rationale:** `fabricengine` and `gitrepo`'s real coverage lives behind the `integration` tag, and a mis-merged message found at the end of an 80-site migration is expensive to bisect.
- **Applies to:** all batches

### Decision: error-wording-is-review-enforced-not-test-enforced

- **Decision:** no test asserts on most of the merged error strings.
  The merge rule above is the specification;
  a site that deviates from it must say so in its commit message.
- **Rationale:** stating the rule once stops the implementer re-deciding it at roughly fifty sites, and makes a deviation a reviewable event rather than an invisible one.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/checkedcall_test.go`
- `cmd/lyx/gitrepoboundary_test.go`
- `cmd/lyx/hermeticenv_test.go`
- `cmd/lyx/rawgitmutation_test.go`
- `cmd/lyx/tierpurity_test.go`
- `docs/shared-libs/README.md`
- `internal/fabriccli/fabric.go`
- `internal/fabricengine/add.go`
- `internal/fabricengine/boardweft.go`
- `internal/fabricengine/checkout.go`
- `internal/fabricengine/cleanup.go`
- `internal/fabricengine/clone.go`
- `internal/fabricengine/destroy.go`
- `internal/fabricengine/destructivegaps_integration_test.go`
- `internal/fabricengine/dirtiness.go`
- `internal/fabricengine/export_test.go`
- `internal/fabricengine/gitexclude.go`
- `internal/fabricengine/hook.go`
- `internal/fabricengine/index.go`
- `internal/fabricengine/prune.go`
- `internal/fabricengine/pull.go`
- `internal/fabricengine/reconcile.go`
- `internal/fabricengine/remove.go`
- `internal/fabricengine/status.go`
- `internal/fabricengine/warpprobe.go`
- `internal/fabricengine/weftgit.go`
- `internal/fabricengine/weftwiring.go`
- `internal/fabricengine/worktreelist.go`
- `internal/gitexec/errorrender_test.go`
- `internal/gitexec/gitexec.go`
- `internal/gitexec/gitexec_test.go`
- `internal/gitrepo/ancestry.go`
- `internal/gitrepo/doc.go`
- `internal/gitrepo/gitrepo.go`
- `internal/gitrepo/pull.go`
- `internal/gitrepo/push.go`
- `internal/gitrepo/reset.go`
- `internal/lyxcwd/lyxcwd.go`
- `internal/websterengine/gitwrap.go`
- `manifest/roadmap.md`
