# Batch: emitter-and-fetch

```yaml
task: 'Sandbox suite: emit findings JSON on the shared analysis contract'
batch: emitter-and-fetch
number: 1
cards: 6
verify: go build ./... && go test ./tools/sandbox/... ./internal/hubgeometry/...
depends-on: []
```

## Batch Scope

This batch delivers the whole producer+fetch feature inside the standalone `package main`
under `tools/sandbox`, plus its launcher and embedded scheme. It is one batch because
`tools/sandbox` is a single Go compilation unit: the new contract types and fetch helper
(`report.go`), the `runSuite` signature change and fetch wiring (`suite.go`), and the new
`-loomyard` flag (`main.go`) must all land together to compile and pass `go test
./tools/sandbox/...`. The embedded `SANDBOX-SUITE.md` scheme and the `sandbox.cmd` launcher
flag are part of the same wiring and ship here too. The external interface the `docs` batch
documents is: the suite emits `sandbox-report.json` in the host repo, and `suite.go` fetches a
normalized, fingerprint-stamped copy to `<loomyard-root>/.scratch/sandbox-report-<sha12>.json`.

Batch-local note: `report.go` and `suite.go` share `package main` and the `binaryInfo` type
(defined in `suite.go`); the helper consumes it directly. Tests are split per the repo's
one-test-file-per-source convention — pure-helper tests in the new `report_test.go`,
`runSuite`-wiring tests in the existing `suite_test.go`.

## Cards

### Card 1: Contract types + fetchReport helper (report.go)

- **Context:**
  - `tools/sandbox/suite.go`
- **Edits:** none
- **Creates:**
  - `tools/sandbox/report.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `tools/sandbox/report.go` in `package main`. Define the contract
  types with JSON tags matching millhouse#586: `type sandboxReport struct { Source string
  \`json:"source"\`; Meta reportMeta \`json:"meta"\`; Items *[]reportItem \`json:"items"\` }`;
  `type reportMeta struct { Fingerprint reportFingerprint \`json:"fingerprint"\` }`; `type
  reportFingerprint struct { Path string \`json:"path"\`; SHA256 string \`json:"sha256"\`; Size
  int64 \`json:"size"\`; ModTime string \`json:"modtime"\` }`; `type reportItem struct { Ref
  string \`json:"ref"\`; Title string \`json:"title"\`; Body string \`json:"body"\` }`. Define
  `const reportFileName = "sandbox-report.json"` and `const reportSourceID = "sandbox-report"`.
  Implement `func fetchReport(hostRepoDir, loomyardRoot string, info binaryInfo) error`:
  (1) read `filepath.Join(hostRepoDir, reportFileName)` — if `os.IsNotExist`, return an error
  like `sandbox report not found at <path>: the agent produced no report`; other read errors
  wrapped with `%w`. (2) `json.Unmarshal` into `sandboxReport`; on error return `parse sandbox
  report <path>: %w`. (3) Validate: `report.Source == reportSourceID` else error `sandbox report
  has wrong source %q (want %q)`; `report.Items != nil` else error `sandbox report is missing
  its items array`. (4) Stamp `report.Meta.Fingerprint = reportFingerprint{Path: info.Path,
  SHA256: info.SHA256, Size: info.Size, ModTime: info.ModTime.Format(time.RFC3339)}`.
  (5) `json.MarshalIndent` the report (2-space indent). (6) `scratchDir :=
  filepath.Join(loomyardRoot, ".scratch")`; `os.MkdirAll(scratchDir, 0o755)`. (7) Write to
  `filepath.Join(scratchDir, "sandbox-report-"+info.SHA256+".json")` with `0o644`. Return nil.
  Wrap each I/O error with path context. An empty-but-present `items: []` is valid and must be
  written (the `Items != nil` check passes because the pointer is non-nil).
- **Commit:** `feat(sandbox): add sandbox-report.json contract + fetchReport helper`

### Card 2: -loomyard flag + runSuite signature/fetch wiring (main.go, suite.go)

- **Context:**
  - `tools/sandbox/report.go`
  - `sandbox.cmd`
- **Edits:**
  - `tools/sandbox/main.go`
  - `tools/sandbox/suite.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** This card lands the `-loomyard` flag, the `runSuite` signature change, and
  its call site together so the package compiles at the card boundary (the signature + call site
  are inherently atomic — splitting them red-lines `go build`).
  **`main.go`:** on the top-level flagset `fs` (next to `parentDir`/`reset`), add `loomyard :=
  fs.String("loomyard", "", "loomyard repo root for fetching the sandbox report (required for
  the suite subcommand)")`. In the `case "suite":` branch, after `absParent` is resolved:
  require `*loomyard != ""` else `fmt.Fprintln(os.Stderr, "sandbox: -loomyard is required for the
  suite subcommand"); return 1`. Resolve it to an absolute, cleaned path: `absLoomyard, err :=
  filepath.Abs(filepath.Clean(*loomyard))` (the `Clean` strips the trailing `.`/separator that
  `sandbox.cmd` passes via `%~dp0.`); on error print `sandbox: resolve loomyard path: %v` and
  `return 1`. Pass `absLoomyard` into `runSuite` as a new second positional argument:
  `runSuite(absParent, absLoomyard, *claudeFlag, *promptFlag)`. The `build` subcommand is
  unchanged and does not read `-loomyard`.
  **`suite.go`:** change the signature to `func runSuite(parentDir, loomyardRoot, claudeOverride,
  promptOverride string) error`. After `ensureGitExclude(hostRepoDir, suiteFileName)` and before
  resolving claude: remove any stale report — `os.Remove(filepath.Join(hostRepoDir,
  reportFileName))` and ignore the error when `os.IsNotExist` (return wrapped on any other
  error); then register the report in the host repo's exclude: `ensureGitExclude(hostRepoDir,
  reportFileName)` (wrap error). After the existing `launchAgent` non-zero-exit guard returns its
  error, and before `return nil`, on clean exit call `fetchReport(hostRepoDir, loomyardRoot,
  info)` and return its wrapped error (`fetch sandbox report: %w`) if non-nil. Update the
  `binaryInfo` doc comment and the `header()` doc comment that reference "filed issues"/"issues
  filed" to describe the emitted `sandbox-report.json` / `meta.fingerprint` provenance instead
  (e.g. "so the emitted report can be traced to the exact binary"). Do not change `header()`'s
  rendered markdown block.
- **Commit:** `feat(sandbox): add -loomyard flag and emit + fetch sandbox-report.json`

### Card 3: Rewrite SANDBOX-SUITE.md scheme to emit the JSON report

- **Context:** none
- **Edits:**
  - `tools/sandbox/SANDBOX-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the "## What this is" intro, reword the line "treating every break,
  surprise, or rough edge as a LoomYard bug to file." so it reflects **recording** each finding
  in `sandbox-report.json` rather than filing an issue (e.g. "…as a LoomYard finding to record
  in the report."). Remove Pre-conditions item 4 (`gh` installed/authenticated) entirely.
  Rewrite the "Capturing findings" section: instead of filing each non-OK finding via `lyx
  selfreport create`, instruct the agent to write **all** WARN/FAIL findings to
  `./sandbox-report.json` (in the host-repo cwd) on this exact schema, and to **always** write
  the file even when there are zero WARN/FAIL findings (`"items": []`). Embed the contract
  block verbatim:
  `{"source":"sandbox-report","items":[{"ref":"S6","title":"…","body":"verdict: WARN\n\n…repro…"}]}`
  (pretty-printed in the doc). Specify: `source` is the literal `"sandbox-report"`; `items[]`
  holds only WARN/FAIL findings; `ref` = scenario id (`S0`–`S6`); `title` = short summary;
  `body` = detail + repro + verdict folded into one markdown string; the agent writes only
  `source` and `items` (the launcher stamps `meta`). Tell the agent to confine all free text to
  the `title`/`body` string fields so the JSON stays well-formed. Update the "Fingerprint
  header" section: the fingerprint now identifies the binary for the report's provenance/`meta`,
  replacing "Every issue filed during this session must include that fingerprint". Rewrite the
  "Session log format" section to drop "File one GitHub issue per WARN or FAIL finding via `lyx
  selfreport create`" and the "Issues filed: <count> (links)" line — replace with the
  sandbox-report.json emission summary. Keep the Verdict key, Black-box rule, Operating model,
  and all S0–S6 scenarios unchanged.
- **Commit:** `feat(sandbox): rewrite suite scheme to emit sandbox-report.json`

### Card 4: Launcher passes -loomyard (sandbox.cmd)

- **Context:** none
- **Edits:**
  - `sandbox.cmd`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the `go run ./tools/sandbox -parent C:\Code %*` line, insert the
  `-loomyard` flag pointing at the launcher's own directory: `go run ./tools/sandbox -parent
  C:\Code -loomyard "%~dp0." %*`. The trailing `.` after `%~dp0` prevents the directory's
  trailing backslash from escaping the closing quote; `main.go` `filepath.Clean`s it. Add a
  brief `REM` line noting that `%~dp0` is the loomyard repo root (where the fetched report
  lands under `.scratch/`).
- **Commit:** `feat(sandbox): pass -loomyard repo root from the launcher`

### Card 5: Unit tests for fetchReport (report_test.go)

- **Context:**
  - `tools/sandbox/report.go`
  - `tools/sandbox/suite.go`
- **Edits:** none
- **Creates:**
  - `tools/sandbox/report_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `tools/sandbox/report_test.go` in `package main`, following the
  `t.TempDir()` style of `suite_test.go`. Use a helper that writes a given JSON body to
  `<hostRepoDir>/sandbox-report.json` and a fixed `binaryInfo` (with a known `SHA256` like
  `"abc123def456"`). Cover: (a) **happy path** — a valid `{source:"sandbox-report",items:[…]}`
  → file lands at `<loomyardRoot>/.scratch/sandbox-report-<sha>.json`; decode the written file
  and assert `meta.fingerprint` equals the passed `binaryInfo` (path/size/sha256, and
  `modtime` == `info.ModTime.Format(time.RFC3339)`), `items` are preserved, and any `meta`
  present in the input was overwritten. (b) **empty-but-present items** — `"items":[]` → still
  written, no error. (c) **absent items key** — `{"source":"sandbox-report"}` (no `items`) →
  error, nothing written. (d) **malformed JSON** — truncated/non-JSON → parse error mentioning
  the path; nothing written to `.scratch`. (e) **wrong source** — valid JSON, `source` missing
  or wrong → validation error. (f) **missing report** — no file in host repo → missing-file
  error distinct from the parse error. (g) **.scratch created** — `loomyardRoot/.scratch` does
  not pre-exist → it is created. Assert "nothing written" by checking the `.scratch` dir is
  absent or empty.
- **Commit:** `test(sandbox): cover fetchReport validate/stamp/fetch paths`

### Card 6: runSuite + run wiring tests — loomyard param, fetch, stale-removal, exclude (suite_test.go, main_test.go)

- **Context:**
  - `tools/sandbox/suite.go`
  - `tools/sandbox/report.go`
  - `tools/sandbox/main.go`
- **Edits:**
  - `tools/sandbox/suite_test.go`
  - `tools/sandbox/main_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** **`suite_test.go`:** Update every existing `runSuite(...)` call site for the
  new signature, passing a `t.TempDir()` loomyard root: `TestRunSuite_HubAbsent`,
  `TestRunSuite_LaunchInvocation`, `TestRunSuite_Overrides`, `TestRunSuite_NonZeroLaunchCode`,
  `TestRunSuite_ClaudeNotFound`. **Critical:** any call site whose `launchAgent` stub returns 0
  and asserts `runSuite` returns nil (`TestRunSuite_LaunchInvocation`, `TestRunSuite_Overrides`)
  must have its stub **also write a valid `{source:"sandbox-report",items:[…]}` into
  `<hostRepoDir>/sandbox-report.json`** — otherwise the new post-launch `fetchReport` hits the
  missing-report error and the test fails. `TestRunSuite_NonZeroLaunchCode` (stub returns
  non-zero) and the two pre-launch-return tests (`HubAbsent`, `ClaudeNotFound`) need no report
  (no fetch occurs) — just the new loomyard-root argument. Add `TestRunSuite_FetchesReport`: a
  stub that writes a valid report and returns 0 → assert the report lands under
  `<loomyardRoot>/.scratch/sandbox-report-<sha>.json`. Add `TestRunSuite_StaleReportRemoved`:
  pre-create `<host>/sandbox-report.json` with stale content, use a stub that writes nothing and
  returns 0 → assert the prior file is gone after `runSuite` and `runSuite` returns the
  missing-report error (no stale fetch into `.scratch`). Add `TestRunSuite_ExcludesReport`: use a stub that
  writes a valid report and returns 0 (so the full clean path runs), then assert
  `<host>/.git/info/exclude` contains `sandbox-report.json` (alongside the existing
  `SANDBOX-SUITE.md` entry). Ensure `TestRunSuite_NonZeroLaunchCode` still asserts the original
  non-zero error and that no report is fetched (`.scratch` stays absent).
  **`main_test.go`:** Update `TestRun_SuiteRoutesSuiteToLaunch` (currently
  `run([]string{"-parent", tmpDir, "suite"})`): pass `-loomyard <t.TempDir()>` in the argv, and
  make its `launchAgent` stub write a valid `sandbox-report.json` into `hostRepoDir` so the fetch
  succeeds and `run` still returns 0. Add `TestRun_SuiteRequiresLoomyard`: call
  `run([]string{"-parent", tmpDir, "suite"})` with **no** `-loomyard` → assert a non-zero return
  and that `launchAgent` was **not** called (covers the new required-flag guard from Card 2).
- **Commit:** `test(sandbox): wire loomyard root + fetch + stale-removal + exclude`

## Batch Tests

`verify: go build ./... && go test ./tools/sandbox/... ./internal/paths/...`

- `go build ./...` is a cheap whole-repo compile guard (a few seconds for Go) honoring the
  discussion's "`go build ./...` green" requirement; `tools/sandbox` is a standalone `main`
  package nothing imports, so cross-package breakage is not expected, but the build stays green.
- `go test ./tools/sandbox/...` runs `main_test.go` + `suite_test.go` + the new `report_test.go`
  — the full set of behaviour for this batch (fingerprinting, `renderScheme`, `ensureGitExclude`,
  `runSuite` wiring, and `fetchReport`).
- `go test ./internal/paths/...` runs the Path Invariant enforcement guard
  (`enforcement_test.go`), which scans `tools/sandbox`. It is included so any accidental
  `os.Getwd`/`git rev-parse`/banned-geometry-literal in the new `tools/sandbox` code fails this
  batch's verify directly, rather than only at a repo-wide run. This is the cross-cutting guard
  for the new code; the scope is justified because the new code lives in a tree that guard scans.
- TDD intent: `fetchReport` (card 1) is the TDD candidate — its `report_test.go` (card 5)
  pins the validate/stamp/fetch contract. Cards are ordered impl-before-test only so the package
  compiles at each card boundary; the verify gate runs over the whole batch.
