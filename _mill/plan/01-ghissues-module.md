# Batch: ghissues-module

```yaml
task: "ghissues module — file LoomYard bugs as GitHub issues"
batch: "ghissues-module"
number: 1
cards: 3
verify: go test ./internal/ghissues/...
depends-on: []
```

## Batch Scope

This batch delivers the self-contained `internal/ghissues` package: the gh-wrapping
core (`ghissues.go`), the Cobra command tree (`cli.go`), and the white-box unit tests
(`cli_test.go`). After this batch, `ghissues.Command()` and `ghissues.RunCLI(...)`
exist and are fully tested in isolation, but are NOT yet wired into the lyx root — that
is batch 2. The external interface batch 2 consumes is `ghissues.Command() *cobra.Command`.

Batch-local decisions (beyond `## Shared Decisions`):
- The unit test is **white-box** (`package ghissues`, file `cli_test.go`) so it can
  override the unexported `runGH` and `stdin` seams. This follows the internal-test
  precedent `internal/board/skipenv_internal_test.go`. (The discussion's earlier mention
  of a black-box test is superseded: overriding unexported seams requires white-box.)
- `createIssue` lives in `ghissues.go` and returns `(url string, number int, err error)`;
  body-from-stdin resolution and the label default live in the `cli.go` RunE, keeping
  `ghissues.go` free of cobra/flag concerns.

## Cards

### Card 1: gh-wrapping core — `internal/ghissues/ghissues.go`

- **Context:**
  - `internal/warp/warp.go`
  - `internal/gitexec/gitexec.go`
  - `internal/proc/proc_windows.go`
  - `internal/proc/proc_other.go`
  - `internal/muxpoc/review.go`
- **Edits:** none
- **Creates:**
  - `internal/ghissues/ghissues.go`
- **Deletes:** none
- **Requirements:**
  - Package `ghissues` with a warp-style package-doc header comment documenting: the
    module's purpose (file LoomYard bugs/enhancements as GitHub issues from `lyx.exe`),
    that it is a Cobra module reachable as `lyx ghissues create`, that it wraps the `gh`
    CLI, and that the target repo is hardcoded (not derived from cwd).
  - Const `targetRepo = "Knatte18/loomyard"`.
  - Package-level seam `var stdin io.Reader = os.Stdin` (consumed by `cli.go`).
  - Package-level seam `var runGH = realRunGH` where the type is
    `func(args []string) (stdout, stderr string, exitCode int, err error)`.
  - `realRunGH(args []string) (stdout, stderr string, exitCode int, err error)`: first
    `exec.LookPath("gh")` — on failure return `("", "", -1, err)` (the LookPath error).
    Otherwise run `exec.Command("gh", args...)` mirroring `gitexec.RunGit`: capture
    stdout/stderr into `bytes.Buffer`s, call `proc.HideWindow(cmd)`, run, extract exit
    code from `*exec.ExitError` (non-zero exit is NOT a Go error: return
    `err == nil`, `exitCode > 0`); a non-ExitError failure returns `("","",-1,err)`.
  - `buildCreateArgs(title string, body *string, labels []string) []string` returning
    `["issue","create","--repo",targetRepo,"--title",title]` then `"--body", *body` when
    `body != nil`, then one `"--label", l` pair per label in order.
  - `createIssue(title string, body *string, labels []string) (url string, number int, err error)`:
    call `runGH(buildCreateArgs(...))`. If the returned `err != nil`, distinguish the
    cause: when `errors.Is(err, exec.ErrNotFound)` return `gh not found on PATH: <err>`
    (the `LookPath` miss); otherwise return `failed to run gh: <err>` (a generic exec
    failure — both share `exitCode == -1`, so use `errors.Is`, not the exit code, to tell
    them apart). If `err == nil` and `exitCode != 0` return `gh issue create failed: <stderr-trimmed>`.
    On success, take the last non-empty trimmed line of stdout as `url`; parse the
    trailing `/<n>` path segment as `number` (strconv.Atoi). If the segment is not a
    parseable int, return `(url, 0, nil)` — success with `number == 0` (cli.go omits a
    zero number).
  - Godoc comments per `golang-comments` conventions on exported identifiers (note:
    most identifiers here are unexported; document them with normal comments).
- **Commit:** `feat(ghissues): add gh-wrapping core (hardcoded repo, runGH seam, createIssue)`

### Card 2: Cobra command tree — `internal/ghissues/cli.go`

- **Context:**
  - `internal/warp/warp.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
  - `internal/board/cli.go`
  - `internal/ghissues/ghissues.go`
- **Edits:** none
- **Creates:**
  - `internal/ghissues/cli.go`
- **Deletes:** none
- **Requirements:**
  - `Command() *cobra.Command`: parent `&cobra.Command{Use: "ghissues", Short: "<non-empty>"}`
    with no `PersistentPreRunE` and no persistent flags. Add one subcommand `create`.
  - `create` command: `Use: "create <title>"`, a non-empty `Short`, and a `Long` that
    includes concrete usage examples — at minimum
    `lyx ghissues create "Crash on empty board" -b -` with prose stating that `-` means
    "read the issue body from stdin", and
    `lyx ghissues create "Add dark mode" --label enhancement`.
  - `create` sets `Args: cobra.ExactArgs(1)` and `RunE: clihelp.WrapRun(runCreate)` where
    `runCreate` is declared inline or as a closure so it can read the command's flags.
  - Flags on `create`: `--body`/`-b` as a `String` flag (default `""`); `--label` as a
    pflag `StringArray` (NOT `StringSlice`) with an empty default `[]string{}`.
  - `runCreate(out io.Writer, args []string) int`: `title := args[0]`. Read the `--body`
    flag value; resolve body to `*string`: if the flag was not set leave body `nil`; if
    set and equal to `"-"`, read all of the `stdin` seam (`io.ReadAll(stdin)`) into a
    string and point body at it; otherwise point body at the flag string. Read the
    `--label` values; `if len(labels) == 0 { labels = []string{"bug"} }`. Call
    `createIssue(title, body, labels)`. On error → `output.Err(out, err.Error())`. On
    success → `output.Ok(out, map[string]any{"url": url})` plus `"number": number` only
    when `number != 0`.
  - To distinguish "flag not set" from "set to empty", read via the cobra command's
    `Flags().Changed("body")` (the closure has the `*cobra.Command`).
  - `RunCLI(out io.Writer, args []string) int` = `return clihelp.Execute(Command(), out, args)`.
  - Optional small output helpers mirroring `internal/board/cli.go` if they reduce noise.
- **Commit:** `feat(ghissues): add cobra create command (ExactArgs, --body/-b stdin, --label)`

### Card 3: white-box unit tests — `internal/ghissues/cli_test.go`

- **Context:**
  - `internal/board/cli_test.go`
  - `internal/board/skipenv_internal_test.go`
  - `internal/gitexec/gitexec.go`
  - `internal/output/output.go`
  - `internal/ghissues/ghissues.go`
  - `internal/ghissues/cli.go`
- **Edits:** none
- **Creates:**
  - `internal/ghissues/cli_test.go`
- **Deletes:** none
- **Requirements:**
  - White-box `package ghissues` so the test can swap the unexported `runGH` and `stdin`
    seams. A helper installs a fake `runGH` that records the argv it received and returns
    caller-specified `(stdout, stderr, exitCode, err)`, and restores the originals via
    `t.Cleanup`. Drive the command through `RunCLI(&buf, args)`; parse the JSON envelope.
  - Cover (one test each, table-driven where natural):
    - Happy path: `["create","My bug title"]`, fake returns
      `https://github.com/Knatte18/loomyard/issues/123` + exit 0 → envelope `ok:true`,
      `url` matches, `number == 123`; recorded argv equals
      `["issue","create","--repo","Knatte18/loomyard","--title","My bug title","--label","bug"]`.
    - Custom labels: `["create","T","--label","enhancement","--label","p1"]` → argv has
      `--label enhancement --label p1` and NOT `bug` (default replaced, not appended).
    - Body via flag: `["create","T","-b","details"]` → argv contains `--body details`.
    - Body via stdin: `["create","T","-b","-"]` with the `stdin` seam set to a reader
      containing multi-line markdown → argv `--body` carries the full content intact.
    - Body omitted: `["create","T"]` → argv has no `--body` element; `ok:true`.
    - Wrong arg count: `["create"]` and `["create","a","b"]` → non-zero exit, cobra
      "accepts 1 arg(s)" message; the fake `runGH` is never called.
    - gh not found: fake returns `exitCode -1` + non-nil err → `ok:false`, exit 1,
      message mentions gh/PATH.
    - gh non-zero exit: fake returns exit 1 + stderr → `ok:false`, exit 1, error surfaces
      the stderr text.
    - Unparseable URL: fake returns non-URL stdout + exit 0 → `ok:true`, `url` present,
      `number` absent from the envelope.
    - Number parsing: URL ending `/issues/123` → `number == 123`. Note: decoding the
      envelope into `map[string]any` yields `float64`, so the test must compare against
      `float64(123)` (or decode into a typed struct with an `int` field) — do not assert
      an `int` literal against the map value.
  - Follow `golang-testing` conventions (table tests, `t.Run` subtests, no real network).
- **Commit:** `test(ghissues): white-box unit tests for create (argv, stdin, labels, gh failures)`

## Batch Tests

`verify: go test ./internal/ghissues/...` runs the white-box test file `cli_test.go`,
which exercises `Command`/`RunCLI`/`createIssue`/`runGH`/`buildCreateArgs` end-to-end
through the seams. Scope is exactly this new package — no cross-package surface — so the
focused `./internal/ghissues/...` selector is correct (no full-suite run needed).
