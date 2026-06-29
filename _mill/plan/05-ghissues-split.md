# Batch: ghissues split

```yaml
task: "Rename Cobra modules to `<module>cli`, extract kernels as `<module>engine`"
batch: "ghissues split"
number: 5
cards: 3
verify: "go build ./... && go test ./... && go test -tags integration ./..."
depends-on: [4]
```

## Batch Scope

Split `internal/ghissues` into `internal/ghissuesengine` (the gh-issue domain) and
`internal/ghissuescli` (cobra command), delete `internal/ghissues`, retarget
`cmd/lyx/main.go`. The current `ghissues.go` mixes a CLI-input seam (`var stdin io.Reader
= os.Stdin`, line 30, read by `runCreate` in `cli.go`) with engine seams (`var runGH`,
`createIssue`). Resolve the seam placement: `stdin` goes to `ghissuescli`; the engine
exports exactly two symbols — `CreateIssue` (the renamed `createIssue`, returning
`(url, number, error)`) and a settable `var RunGH = realRunGH`. `targetRepo`, `realRunGH`,
`buildCreateArgs`, and `lastNonEmptyLine` stay engine-internal.

## Cards

### Card 15: Create `internal/ghissuesengine` domain package

- **Context:**
  - `internal/ghissues/ghissues.go`
  - `internal/ghissues/cli.go`
- **Edits:** none
- **Creates:**
  - `internal/ghissuesengine/ghissues.go`
- **Deletes:** none
- **Requirements:** Create `internal/ghissuesengine/ghissues.go` (package
  `ghissuesengine`) holding everything from the current `ghissues.go` **except** the
  `var stdin io.Reader = os.Stdin` seam (that moves to `ghissuescli` in card 16). Rename
  `createIssue` → exported `CreateIssue` (signature
  `(title string, body *string, labels []string) (url string, number int, err error)`
  unchanged) and `runGH` → exported settable `var RunGH = realRunGH`; update
  `CreateIssue`'s internal call from `runGH(...)` to `RunGH(...)`. Keep `targetRepo`,
  `realRunGH`, `buildCreateArgs`, and `lastNonEmptyLine` unexported. Drop the now-unused
  `os`/`io` imports if `stdin`'s removal makes them unused (keep whatever `realRunGH` etc.
  still need). `cli.go` is read-only Context (do not move — card 16). Do not delete
  `internal/ghissues` (card 17).
- **Commit:** `refactor(ghissues): extract ghissuesengine domain package`

### Card 16: Create `internal/ghissuescli` command package

- **Context:**
  - `internal/ghissues/cli.go`
  - `internal/ghissues/cli_test.go`
  - `internal/ghissues/ghissues.go`
  - `internal/clihelp/exec.go`
- **Edits:** none
- **Creates:**
  - `internal/ghissuescli/cli.go`
  - `internal/ghissuescli/cli_test.go`
- **Deletes:** none
- **Requirements:** Move `cli.go` (cobra tree, `runCreate`, `Command()`, the `RunCLI`
  seam) into `internal/ghissuescli/cli.go` with package `ghissues` → `ghissuescli`, and
  **add the `var stdin io.Reader = os.Stdin` seam here** (with its `io`/`os` imports) since
  only `runCreate` reads it. Add the `internal/ghissuesengine` import and change
  `runCreate`'s `createIssue(...)` call to `ghissuesengine.CreateIssue(...)`. Move
  `cli_test.go` to `internal/ghissuescli/cli_test.go` with package `ghissuescli`: it now
  swaps the exported `ghissuesengine.RunGH` seam (replacing the old in-package `runGH`
  swap, formerly at L24) and swaps the local `stdin` seam (formerly at L161). Preserve
  every existing scenario (happy path, custom labels, body via flag/stdin/omitted, wrong
  arg count, gh-not-found, gh non-zero exit, unparseable URL, number parsing). The
  `RunCLI` seam body stays exactly `clihelp.Execute(Command(), out, args)`. Preserve any
  build tag the original `cli_test.go` carried (it is currently untagged — keep it
  untagged).
- **Commit:** `refactor(ghissues): extract ghissuescli command package`

### Card 17: Retarget importer and delete `internal/ghissues`

- **Context:**
  - `internal/ghissues/cli.go`
  - `internal/ghissues/ghissues.go`
- **Edits:**
  - `cmd/lyx/main.go`
- **Creates:** none
- **Deletes:**
  - `internal/ghissues/cli.go`
  - `internal/ghissues/ghissues.go`
  - `internal/ghissues/cli_test.go`
- **Requirements:** In `cmd/lyx/main.go` replace the `internal/ghissues` import with
  `internal/ghissuescli` and change `ghissues.Command()` to `ghissuescli.Command()` in
  `newRoot()`. Then delete the entire `internal/ghissues` directory.
- **Commit:** `refactor(ghissues): retarget importer and remove internal/ghissues`

## Batch Tests

`verify` is repo-wide (Tier 1 + Tier 2). The ghissues suite is Tier 1 (untagged), but the
repo-wide form is kept for the `cmd/lyx/main.go` compile guarantee and uniformity with the
other split batches. Moved coverage: the ghissuescli `cli_test` drives the full
`cobra → flag → CreateIssue → RunGH` pipeline through `RunCLI`, swapping
`ghissuesengine.RunGH` and the local `stdin` seam across every existing scenario. cmd/lyx
guard tests self-derive and re-validate the renamed `ghissuescli` registration.
