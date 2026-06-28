# Batch: sandbox-suite-launcher

```yaml
task: "Sandbox test-suite launcher and task harvester"
batch: "sandbox-suite-launcher"
number: 1
cards: 5
verify: go test ./tools/sandbox/...
depends-on: []
```

## Batch Scope

Delivers the `sandbox suite` launcher end-to-end inside the existing `tools/sandbox`
Go tool: a tracked, embedded test-scheme template; subcommand dispatch (`build`
default + `suite`); the suite run logic (Hub check, `lyx.exe` fingerprint, fresh
`SANDBOX-SUITE.md` copy into the Hub host repo, `.git/info/exclude` hygiene, and the
interactive `claude` launch); unit tests; and the doc update. It is one batch because
every piece lives in `package main` under `tools/sandbox/` (two small Go files plus a
markdown template and one doc) and shares the same handful of `Context:` files — well
under the context budget. No external interface is exposed to a later batch; the only
out-of-package edit is `docs/sandbox-hub.md`.

Batch-local conventions (in addition to `## Shared Decisions` in the overview):

- The existing `main.go` `const` block holds `hostURL`, `weftURL`, `hubName`. The new
  `hostDirName = "lyx-test"` const (the host clone's subdir under `lyx-test-HUB`, per
  `docs/sandbox-hub.md`) is added in `suite.go` (Card 2), not `main.go`, so each commit
  compiles standalone; `suite`-specific consts (`suiteFileName`, `defaultInstruction`)
  also live in `suite.go`.
- Testability seams are package-level `var` function values mirroring `cloneRun` /
  `removeAll`, so tests stub them without launching `claude`/`lyx` or touching PATH.

## Cards

### Card 1: Tracked, embeddable test-scheme template

- **Context:**
  - `.scratch/test-scheme-sandbox.md`
  - `docs/sandbox-hub.md`
  - `internal/ghissues/cli.go`
- **Edits:** none
- **Creates:**
  - `tools/sandbox/test-scheme.md`
- **Deletes:** none
- **Requirements:** Create `tools/sandbox/test-scheme.md` as the tracked master
  test-scheme, based on `.scratch/test-scheme-sandbox.md` but refreshed: (1) drop the
  "operator must be handed the command surface until cobra lands" / "assumes built-in
  help has landed" caveats — cobra help has landed, so scenario **S0 — Discovery** is
  itself the help-surface test; (2) replace the "Roll ❌/⚠️ into LoomYard tasks" capture
  step with: file **each** non-✅ (❌/⚠️) finding from inside the Hub via
  `lyx ghissues create`, which feeds the downstream `GitHub issue →
  mill-ghissues-to-tasks` pipeline. The scheme must instruct the agent to **discover the
  command's flags itself via `lyx ghissues create --help`** (black-box / S0 ethos)
  rather than listing hardcoded flags, and must state explicitly that there is **no
  harvester and no `lyx board upsert`** — `lyx ghissues create` is the only capture
  path. (The implementer reads `internal/ghissues/cli.go` only to confirm the verb
  exists; the scheme text points the agent at `--help`.) (3) add a **Pre-conditions** note
  that `gh` must be installed and authenticated (it backs `lyx ghissues create`),
  alongside `lyx` on PATH; (4) state the **black-box rule** explicitly — the agent
  tests `lyx.exe` only, must not look for or reason about lyx's source code, and works
  in the Hub host repo as a real user with just the binary; (5) note that the launcher
  prepends a "binary under test" fingerprint header to the copied `SANDBOX-SUITE.md` at
  runtime, and instruct the agent to include that fingerprint in every issue it files.
  Keep the existing scenario spine (S0–S6) and the ✅/⚠️/❌ verdict bucketing. Use
  ASCII-friendly markdown per the `markdown` rules.
- **Commit:** `docs(sandbox): add tracked test-scheme template for the suite agent`

### Card 2: `suite` run logic

- **Context:**
  - `tools/sandbox/main.go`
  - `internal/muxpoc/review.go`
- **Edits:** none
- **Creates:**
  - `tools/sandbox/suite.go`
- **Deletes:** none
- **Requirements:** Create `tools/sandbox/suite.go` (`package main`) implementing the
  suite launcher. This card is sequenced **before** the dispatch card so each commit
  builds: `suite.go` defines `runSuite` and its own `hostDirName` const, and only reads
  the existing `hubName` const from `main.go` (a package-level function that nothing yet
  calls compiles fine in Go). Add `hostDirName = "lyx-test"` as a `const` in `suite.go`
  (the host clone's subdir under `lyx-test-HUB`, per `docs/sandbox-hub.md`). Embed the
  template: add a blank `import _ "embed"` and `//go:embed test-scheme.md` bound to a
  package var (e.g. `var testSchemeMD string`) — the blank `embed` import is required or
  the file will not compile. Add constants `suiteFileName = "SANDBOX-SUITE.md"`
  and `defaultInstruction = "Read ./SANDBOX-SUITE.md and follow the instructions in it
  exactly."`. Provide testability seams as package-level `var`s: `lookPath =
  exec.LookPath` (resolve `lyx`/`claude` on PATH) and `launchAgent = func(hostRepoDir,
  claudePath, instruction string) int { ... }` which runs `exec.Command(claudePath,
  "--dangerously-skip-permissions", instruction)` with `Dir = hostRepoDir`, inheriting
  `os.Stdin/Stdout/Stderr` and the current env, waits, and returns the child exit code
  (best-effort: derive from `*exec.ExitError`, else non-zero on other errors). Implement:
  - `binaryInfo` struct (`Path`, `Size int64`, `ModTime time.Time`, `SHA256 string`)
    and `func binaryFingerprint(path string) (binaryInfo, error)` — `os.Stat` for size +
    modtime (UTC), stream-hash the file with `crypto/sha256`, store the first 12 hex
    chars as `SHA256`; wrap errors with `%w`.
  - `func (binaryInfo) header() string` — a small markdown block stamping abs path, size,
    modtime (RFC3339 UTC), and short sha256.
  - `func renderScheme(info binaryInfo) string` — `header()` + a blank line + the
    embedded `testSchemeMD` body.
  - `func ensureGitExclude(repoDir, entry string) error` — idempotent: ensure
    `<repoDir>/.git/info/` exists, read `exclude` if present, append `entry` on its own
    line only if not already present, preserving existing content; create the file when
    missing.
  - `func runSuite(parentDir, claudeOverride, promptOverride string) error` — compute
    `hostRepoDir := filepath.Join(parentDir, hubName, hostDirName)`; if it does not
    exist (`os.Stat` / `os.IsNotExist`) return an error naming the path and telling the
    operator to run `sandbox build` first; resolve `lyx` via `lookPath("lyx")` (error
    mentions deploying the binary) and fingerprint it; write `renderScheme(info)` to
    `filepath.Join(hostRepoDir, suiteFileName)` (overwrite, 0o644); call
    `ensureGitExclude(hostRepoDir, suiteFileName)`; resolve the claude binary
    (`claudeOverride` if non-empty else `lookPath("claude")`, error "claude not found on
    PATH"); pick the instruction (`promptOverride` if non-empty else
    `defaultInstruction`); call `launchAgent(hostRepoDir, claudePath, instruction)` and,
    if the returned code is non-zero, return an error so `run` propagates a non-zero exit
    (best-effort under `go run`). Do NOT use `os.Getwd` or `git rev-parse` anywhere.
- **Commit:** `feat(sandbox): implement suite launcher (fingerprint, copy, exclude, launch)`

### Card 3: Subcommand dispatch in `main.go`

- **Context:**
  - `sandbox.cmd`
  - `tools/sandbox/suite.go`
- **Edits:**
  - `tools/sandbox/main.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Refactor `tools/sandbox/main.go` to dispatch on a subcommand while
  preserving back-compat. Extract the parse-and-dispatch body of `main()` into a new
  testable `func run(argv []string) int` (returns an exit code; `main()` becomes
  `os.Exit(run(os.Args[1:]))`). In `run`: build a top-level `flag.FlagSet` that defines
  `-parent` (string, required) and `-reset` (bool, build-only, kept top-level for
  back-compat); parse `argv`; resolve `*parent` to an absolute path exactly as today
  (error to stderr + return 1 when empty or on `filepath.Abs` failure). Determine the
  subcommand from the first remaining positional (`fs.Args()`): empty or `"build"` →
  compute `hubPath := filepath.Join(absParent, hubName)` and call the existing
  `decideClone(hubPath, *reset)` (so bare `sandbox.cmd` and `sandbox.cmd -reset` keep
  working unchanged); `"suite"` → parse the remaining args (`fs.Args()[1:]`) with a
  dedicated `flag.FlagSet` defining `-claude` (string, claude binary override) and
  `-prompt` (string, instruction override), then call `runSuite(absParent, claudeFlag,
  promptFlag)` (added in Card 2); any other token → print `sandbox: unknown subcommand
  %q` to stderr and return 1. (`hostDirName` is already defined in `suite.go` from Card
  2 — do not redeclare it here.) All errors keep the `sandbox:` stderr prefix and
  non-zero return.
- **Commit:** `feat(sandbox): add build/suite subcommand dispatch`

### Card 4: Tests for dispatch and suite

- **Context:**
  - `tools/sandbox/main.go`
  - `tools/sandbox/suite.go`
- **Edits:**
  - `tools/sandbox/main_test.go`
- **Creates:**
  - `tools/sandbox/suite_test.go`
- **Deletes:** none
- **Requirements:** Follow the existing `main_test.go` style (`t.TempDir()`, seam
  stubs with `defer` restore, no network, no real launch). In `main_test.go` add tests
  for `run(argv)`: bare/`build` default routes to the clone path (stub `cloneRun`,
  assert it fired); `-reset` (no subcommand) routes to build with reset (stub `cloneRun`
  + `removeAll`); a `suite` positional routes to the suite path (stub `launchAgent` /
  `lookPath` so nothing real launches); an unknown subcommand returns a non-zero code;
  missing `-parent` returns non-zero. In a new `suite_test.go` cover: `runSuite` returns
  a clear error and does not call `launchAgent` when the Hub host subdir is absent;
  `binaryFingerprint` over a temp file yields the correct size and a stable 12-char
  sha256, and errors on a missing path; `renderScheme` output contains the fingerprint
  header fields and the embedded scheme body; `ensureGitExclude` adds the entry once,
  is a no-op on a second call, preserves pre-existing exclude content, and creates a
  missing `info/exclude`; `runSuite` invokes `launchAgent` with the host-repo dir,
  resolved claude path, and the expected instruction, honouring `-claude`/`-prompt`
  overrides and propagating a non-zero launch code as an error; and a `lookPath` stub
  that fails for `claude` surfaces a clear error. Stub `lookPath` to return fake paths
  for `lyx`/`claude` and point fingerprinting at a real temp file.
- **Commit:** `test(sandbox): cover subcommand dispatch and suite launcher`

### Card 5: Document the suite command

- **Context:**
  - `tools/sandbox/main.go`
  - `tools/sandbox/suite.go`
  - `tools/sandbox/test-scheme.md`
- **Edits:**
  - `docs/sandbox-hub.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add a section to `docs/sandbox-hub.md` documenting the new
  `sandbox suite` subcommand: that it is run from the lyx repo (`sandbox.cmd suite`),
  requires the Hub already built (`sandbox build` first) and `lyx`/`gh` on PATH; that it
  copies a fresh `SANDBOX-SUITE.md` (embedded from the tracked `tools/sandbox/test-scheme.md`)
  into the Hub host repo, stamps a `lyx.exe` fingerprint header, ignores the copy via
  `.git/info/exclude`, and starts an interactive black-box `claude` session whose only
  instruction is to read and follow that file; that findings flow out via
  `lyx ghissues create`; and the `go run` exit-code caveat (success/failure only). Note
  the `build` default still covers the existing clone/reset behaviour, and that psmux
  will later replace the `claude` launch. Keep the existing "See Also" links valid.
- **Commit:** `docs(sandbox): document the suite launcher command`

## Batch Tests

`verify: go test ./tools/sandbox/...` runs the package's tests — the extended
`main_test.go` (subcommand dispatch + back-compat) and the new `suite_test.go`
(`binaryFingerprint`, `renderScheme`, `ensureGitExclude`, `runSuite` Hub-missing /
launch-invocation / override / claude-not-found paths). Scope is the single
`tools/sandbox` package the batch touches; no cross-cutting helper is involved, so the
package-scoped command is the correct (non-isolated, native Go) verify. The embedded
`test-scheme.md` is compiled in via `//go:embed`, so a successful `go test` build also
proves the embed resolves. Card 5 is doc-only and has no runnable surface.
