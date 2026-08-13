# Batch: gitexec-checked-entry-point

```yaml
task: 'gitexec: add the checked entry point and migrate the call sites'
batch: gitexec-checked-entry-point
number: 1
cards: 4
verify: go build ./... && go test ./internal/gitexec/... && go test -tags integration ./internal/gitexec/...
depends-on: []
```

## Batch Scope

This batch delivers the whole new surface every later batch consumes: the `GitError` type with its rendering rules, the `Run` entry point, and the shared unexported exec core both exported functions become thin wrappers over.
Nothing outside `internal/gitexec` changes, and `RunGit` keeps its published four-value shape byte-for-byte — including blanking stdout and returning `-1` on an exec-level failure.
The external interface batches 2 through 7 consume is `gitexec.Run(args []string, cwd string) (string, error)` plus `*gitexec.GitError` with its four exported fields.
`internal/gitexec` is a zero-dependency leaf (`bytes`, `os/exec`, `internal/proc`);
this batch may add only `fmt` and `strings`, nothing else.

Batch-local decision beyond `## Shared Decisions`: `Run` and `RunGit` share one exec body via an unexported helper and neither calls the other.
Implementing `Run` as a wrapper over `RunGit` would make the stdout-on-error contract unsatisfiable, because `RunGit`'s exec path blanks stdout and returns the `-1` sentinel this design rejects.

## Cards

### Card 1: GitError type and its rendering rules

- **Context:**
  - `internal/gitrepo/doc.go`
- **Edits:**
  - `internal/gitexec/gitexec.go`
- **Creates:**
  - `internal/gitexec/errorrender_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an exported struct `GitError` to package `gitexec` with exactly four exported fields, in this order: `Args []string`, `Dir string`, `ExitCode int`, `Stderr string`.
  Add the method `func (e *GitError) Error() string`.
  It renders the literal `git `, then the args joined by single spaces, then `: exit <ExitCode>`, then — only when `strings.TrimSpace(e.Stderr)` is non-empty — `: ` followed by that trimmed stderr.
  An individual arg is rendered through `fmt.Sprintf("%q", arg)` when it is the empty string or contains any of space, tab, or newline;
  every other arg is rendered bare.
  Use only `fmt` and `strings` for this — do not add any other import to the package.
  Write `internal/gitexec/errorrender_test.go` first and commit it with the type: it is an untagged Tier 1 external-package (`package gitexec_test`) file that spawns no git and must carry no `//go:build` line.
  Its scenarios are: stderr present renders the trailing segment;
  stderr empty and stderr whitespace-only both omit the trailing segment entirely;
  a stderr with surrounding whitespace is trimmed;
  an arg containing a space is `%q`-quoted;
  an empty arg is `%q`-quoted;
  an ordinary arg is unquoted;
  and one mixed vector renders both forms in a single string.
  Do not add a `Stdout` field, and do not redact or rewrite `Args` — a godoc line on the type states that callers must not pass credentials in args.
- **Commit:** `feat(gitexec): add GitError with its rendering rules`

### Card 2: the shared exec core and the Run entry point

- **Context:**
  - `internal/proc/proc_linux.go`
- **Edits:**
  - `internal/gitexec/gitexec.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extract `RunGit`'s current body into an unexported helper — name it `runCore(args []string, cwd string) (stdout, stderr string, exitCode int, err error)` — that builds the `exec.Command`, sets `cmd.Dir`, wires the two `bytes.Buffer`s, calls `proc.HideWindow`, runs the command, and maps an `*exec.ExitError` to its exit code with a nil error while returning any other non-nil error as an exec-level failure.
  Rewrite `RunGit` as a thin wrapper over `runCore` that preserves its published shape byte-for-byte: on an exec-level failure it returns `("", "", -1, err)`;
  otherwise it returns the core's stdout, stderr, exit code, and a nil error.
  Add `func Run(args []string, cwd string) (string, error)` as the second thin wrapper: on an exec-level failure it returns `("", err)` with the raw underlying error unwrapped — never a `GitError`;
  on a non-zero exit it returns the core's stdout together with `&GitError{Args: args, Dir: cwd, ExitCode: <code>, Stderr: <stderr>}`;
  on success it returns the core's stdout and a nil error.
  `Run` must not call `RunGit` and `RunGit` must not call `Run`.
  `Run`'s godoc states the stdout-on-error contract explicitly: stdout is returned in every case where git actually ran, including alongside a `*GitError`, and is empty on an exec-level failure only because git never ran.
- **Commit:** `feat(gitexec): add the checked Run entry point over a shared exec core`

### Card 3: Tier 2 coverage for Run

- **Context:**
  - `internal/gitexec/testmain_test.go`
- **Edits:**
  - `internal/gitexec/gitexec_test.go`
  - `internal/gitexec/errorrender_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend the existing `//go:build integration` file `internal/gitexec/gitexec_test.go` with coverage for `Run` against a real git repository, mirroring the fixture style already in that file.
  Assert: a successful command returns its stdout with a nil error;
  a non-zero exit returns an error that `errors.As` recovers as `*gitexec.GitError` carrying the right `ExitCode`, the `Args` slice it was given, the `Dir` it was given, and a non-empty `Stderr`;
  stdout is still returned alongside that error, using a command that writes to stdout and exits non-zero;
  and an exec-level failure — a `cwd` that does not exist — returns a non-nil error that `errors.As` does **not** match as `*gitexec.GitError`.
  That last assertion is the load-bearing one for every `errors.As` recovery site in batches 2 through 7 and must be present.
  Also assert in the same integration file that `RunGit`'s exec-level failure still returns exit code `-1` with blanked stdout and stderr, pinning the unchanged shape.
  The only edit to `internal/gitexec/errorrender_test.go` in this card is adding scenarios if card 1's list turned out incomplete;
  leave it untouched otherwise, and do not add a build tag to it.
- **Commit:** `test(gitexec): cover Run's success, GitError, stdout-on-error, and exec-failure paths`

### Card 4: package header carries the durable rationale

- **Context:**
  - `manifest/designs/gitexec-error-shape.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/gitexec/gitexec.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace the file's current three-line header comment with a package header carrying the durable rationale, tightly: the two-shape contract and when to reach for each form (`Run` by default;
  `RunGit` where a non-zero exit is an answer rather than a failure, which is why it is not legacy);
  why exec-level failures stay unwrapped, so `errors.As` means precisely "git ran and rejected this";
  the stdout-on-error contract;
  that `Args` are rendered verbatim with no redaction and callers must not pass credentials in them;
  and a pointer to the gitexec Checked-Call Invariant in `CONSTRAINTS.md` as the mechanism keeping raw sites deliberate.
  Do not port the rejected-alternatives list from the design doc into the header — it dies with the doc in batch 8.
  Do not edit `manifest/designs/gitexec-error-shape.md` in this card;
  it is read only as the source of the rationale being carried over, and batch 8 deletes it.
- **Commit:** `docs(gitexec): carry the two-shape rationale into the package header`

## Batch Tests

`verify:` runs `go build ./...`, then the package's Tier 1 suite (`go test ./internal/gitexec/...`, covering the new `internal/gitexec/errorrender_test.go` rendering scenarios with no git spawn), then its Tier 2 suite (`go test -tags integration ./internal/gitexec/...`, covering `internal/gitexec/gitexec_test.go`'s `Run` and `RunGit` behaviour against real git).
The scope is a single package because that is the whole blast radius of this batch — no call site changes here.
`go build ./...` is included so an accidental signature change to `RunGit` fails at this batch rather than at batch 3.
