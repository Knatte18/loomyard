# Discussion: Sandbox suite: emit findings JSON on the shared analysis contract

```yaml
task: 'Sandbox suite: emit findings JSON on the shared analysis contract'
slug: sandbox-report-json
status: discussing
parent: main
```

## Problem

The sandbox suite is an internal black-box QA harness: a Claude agent drives the
deployed `lyx.exe` against a throwaway Hub host repo and records WARN/FAIL findings.
Today the agent files each finding as a **GitHub issue** via `lyx selfreport create`,
which a human then triages back out and closes — using the public LoomYard tracker as
transport for an internal harness (that was the whole lifecycle of issues #35–#41).

Instead, the suite should emit a structured **`sandbox-report.json`** of the WARN/FAIL
findings on the **shared analysis contract** `{source, meta, items:[{ref,title,body}]}`
— the same format a source-agnostic triage consumes (the consumer half is being
extracted in **millhouse#586**). LoomYard is the **producer** of that contract here.
The loomyard-side launcher then validates and fetches that report into loomyard's
`.scratch/` so the triage skill can consume it directly. No GitHub issues are created
by the sandbox anymore.

**Why now:** millhouse#586 establishes the shared `{source,meta,items}` seam; this task
lands the LoomYard producer against it. `selfreport` itself is **not** removed — it stays
as the real channel for lyx self-reporting a bug while running in a real/other repo. It is
simply no longer the sandbox's transport.

## Scope

**In:**

- `tools/sandbox/SANDBOX-SUITE.md` (the embedded scheme): replace the "file WARN/FAIL
  findings via `lyx selfreport create`" instructions with "write WARN/FAIL findings to
  `./sandbox-report.json` on this exact schema", giving the agent the exact contract and
  confining free text to the `title`/`body` string fields. Instruct the agent to **always**
  write the report file, even when there are zero WARN/FAIL findings (`items: []`).
- `tools/sandbox/suite.go` (+ a new `tools/sandbox/report.go`): after the agent session
  returns cleanly, read `<host>/sandbox-report.json`, **validate** it (typed decode +
  `source` check), **stamp** `meta.fingerprint` from the authoritative `binaryInfo`, and
  **write** the normalized report to loomyard `.scratch/sandbox-report-<sha12>.json`.
  Handle malformed-report and missing-report cases with clear errors.
- Register `sandbox-report.json` in the **host repo's** `.git/info/exclude` (alongside the
  existing `SANDBOX-SUITE.md` entry) so the host repo stays clean.
- `sandbox.cmd`: pass the loomyard repo root (`%~dp0`) into the suite tool via a new
  **top-level** flag so the tool knows where to fetch the report to (see Decision:
  scratch-destination).
- `suite.go`: before launching the agent, **remove any pre-existing
  `<host>/sandbox-report.json`** so a clean exit with no rewrite surfaces as missing-report
  rather than a stale finding list under a fresh fingerprint (see Decision: clean-slate-report).
- `tools/sandbox/suite_test.go`: tests for the fetch/validate/stamp logic — happy path lands
  at the expected `.scratch` path with stamped meta; malformed report → parse error; missing
  report → missing-file message.
- `docs/sandbox-howto.md` and `docs/sandbox-hub.md`: drop the `gh` auth prerequisite and the
  "agent files findings via `lyx selfreport create`" prose; describe the
  emit→fetch→triage flow instead. **Also fix the pre-existing `tools/sandbox/test-scheme.md`
  bug** (see Decision: scheme-filename).

**Out:**

- The triage analysis itself — extracted in millhouse#586. This task only **produces** the
  contract-shaped JSON and fetches it into `.scratch/`.
- Removing or changing the `selfreport` module (`internal/selfreportengine`,
  `internal/selfreportcli`) — it stays for real-repo self-reporting. Untouched.
- Any change to the Hub-build (`build` subcommand), warp clone, or weft topology.
- The future psmux launcher swap — the file contract is unchanged by that and is out of scope.

## Decisions

### meta-fingerprint-authorship

- Decision: `suite.go` stamps `meta.fingerprint` itself, from the authoritative
  `binaryInfo` it already computes (`Path`, `Size`, `ModTime`, `SHA256[:12]`). The agent
  writes **only** `{source, items}`; any `meta` the agent happens to write is ignored and
  overwritten.
- Rationale: the fingerprint is machine-known data; making an LLM transcribe four fields
  into JSON is needless error surface. Stamping guarantees `meta` is always present and
  exact, and keeps the agent's job to the part only it can do (the findings).
- Rejected: agent writes the full contract including `meta` — relies on the LLM copying the
  fingerprint header correctly; brittle for zero benefit.

### scratch-destination

- Decision: the loomyard-side tool (`tools/sandbox`, run via `sandbox.cmd` from the loomyard
  root) performs the fetch. It learns the loomyard root from the **launcher**: `sandbox.cmd`
  passes its own directory (`%~dp0`, which *is* the loomyard repo root) into the tool via a
  new **top-level** flag `-loomyard`, exactly as it already hardcodes `-parent C:\Code`.
  - **Flag placement (top-level, not suite-local).** `sandbox.cmd` is `go run ./tools/sandbox
    -parent C:\Code %*`, and `%*` already carries the `suite` subcommand token. A flag added
    to the launcher therefore lands *before* `suite` and must be parsed by the top-level
    flagset `fs` in `main.go` (alongside `-parent`/`-reset`), then threaded into `runSuite`
    as a parameter — **not** as a suite-local flag on `sf`.
  - **`%~dp0` trailing-backslash quoting.** `%~dp0` ends in a backslash, so a naive
    `-loomyard "%~dp0"` becomes `-loomyard "C:\...\"` where the trailing `\"` escapes the
    closing quote and corrupts parsing. Pass it as `-loomyard "%~dp0."` (append a `.`, giving
    `...\.`) and `filepath.Clean` it inside the tool, or otherwise strip the trailing
    separator. The plan must use the dot-suffix form (or an equivalent that avoids a
    backslash immediately before the closing quote).
  - The report is written to `<loomyard-root>/.scratch/sandbox-report-<sha12>.json`. The
    fetch helper takes the **loomyard root** as its destination argument (see the
    fetch-helper signature note in Technical context) and itself joins `.scratch` and
    `MkdirAll`s it.
- Rationale: the sandbox **agent** runs only in the sandbox and writes only there; retrieval
  is done by the LoomYard side, which knows where itself is and where the sandbox is. The
  Path Invariant forbids `os.Getwd`/`git rev-parse` in `tools/sandbox` (it is scanned and not
  allowlisted), so the root must arrive as data — the launcher is the natural source and
  already owns the machine-specific paths. Passing it as a flag keeps the fetch function pure
  and unit-testable (it takes a destination dir).
- Rejected: resolve via `os.Getwd()` inside the tool — banned by the Path Invariant. Write to
  a literal relative `./.scratch/` against the inherited cwd — works (no `os.Getwd` call) but
  leaves the destination implicit and cwd-fragile; the explicit flag is clearer and testable.

### validation-strictness

- Decision: after reading the agent's report, **typed-decode** it into the contract struct
  and require `source == "sandbox-report"` and a **present** `items` array before trusting it.
  A parse failure or a wrong/missing `source` is a clear, immediate error from the suite run.
- **Decode shape (absent vs empty `items`).** With `encoding/json`, a missing `items` key and
  `items: []` both decode to a nil/empty `[]Item` if the field is a plain slice, so a plain
  slice cannot tell "absent" from "empty". To enforce *present*, decode `Items` as a
  **pointer to slice** (`Items *[]Item` with tag `json:"items"`): a `nil` pointer means the
  key was absent → reject as malformed; a non-nil pointer to a possibly-empty slice (`[]` is
  valid) means present → accept. (`json.RawMessage` is the equivalent alternative.) An empty
  `items: []` is a **valid** report — see clean-slate-report: empty findings are written, not
  treated as missing.
- Rationale: the JSON is LLM-produced, so it can be malformed or valid-but-wrong-shape.
  Catching it at suite time gives a legible error instead of a confusing crash later in the
  triage. The decode→stamp→re-encode also naturally produces the normalized output file.
- Rejected: bare `json.Unmarshal` that only checks "is it JSON" — a valid-JSON-wrong-shape
  report would slip through to the triage. Plain `Items []Item` — cannot distinguish an
  absent `items` key from a legitimately empty one.

### scheme-filename

- Decision: the embedded scheme file is and stays **`SANDBOX-SUITE.md`** — that is its name
  on disk and the name it is copied under into the sandbox host repo. The proposal's and both
  docs' references to `tools/sandbox/test-scheme.md` describe a file that **does not exist**;
  this is a bug. Fix it: edit the real `SANDBOX-SUITE.md`, and correct every dangling
  `test-scheme.md` reference in `docs/sandbox-howto.md` and `docs/sandbox-hub.md` to point at
  `SANDBOX-SUITE.md`. No rename, no new file.
- Rationale: there is no separate scheme file; `suite.go` does `//go:embed SANDBOX-SUITE.md`
  and writes it under `suiteFileName = "SANDBOX-SUITE.md"`. The `test-scheme.md` name is stale
  doc rot.
- Rejected: renaming `SANDBOX-SUITE.md` → `test-scheme.md` to match the docs — larger blast
  radius (the `//go:embed` directive, the `suiteFileName` const, the exclude entry, the
  fingerprint-header prose) for no benefit; the on-disk/host name should stay `SANDBOX-SUITE.md`.

### normalized-copy-not-bytewise

- Decision: the report landed in `.scratch/` is `suite.go`'s **re-serialized** form
  (decode the agent file → stamp `meta` → marshal), not a byte-for-byte copy of the
  agent's file. The agent's original file in the host repo is left as written (and is
  git-excluded there).
- Rationale: re-serialization is what lets `suite.go` stamp `meta` and normalize formatting;
  a byte copy could not carry the authoritative fingerprint.
- Rejected: `io.Copy` of the raw bytes — cannot stamp `meta`.

### clean-slate-report

- Decision: in `runSuite`, **before launching the agent**, remove any pre-existing
  `<host>/sandbox-report.json` (e.g. `os.Remove`, ignoring `os.IsNotExist`). This mirrors how
  `SANDBOX-SUITE.md` is overwritten fresh each run, so every session starts with no report on
  disk.
- Rationale: `sandbox-report.json` is git-excluded in the host repo, so a copy from a prior
  run persists there. Without a pre-launch clean, an agent that exits 0 but does **not**
  rewrite the report (e.g. it forgot, or the session ended early) would leave the previous
  run's findings in place; `suite.go` would then fetch that **stale** list and stamp it with
  the **current** binary's fingerprint — a silent correctness bug (wrong findings attributed
  to a new binary). With the pre-launch removal, that same scenario correctly surfaces as the
  missing-report case.
- Interaction with empty findings: a run with no WARN/FAIL findings still has the agent write
  `{"source":"sandbox-report","items":[]}`; the pre-launch clean does not conflict with that —
  an empty-but-present report is valid and is fetched. Only a truly absent file is the
  missing-report error.
- Rejected: leave the stale file and rely on the agent always rewriting it — depends on
  perfect LLM compliance for a correctness property; the cheap pre-launch `os.Remove` makes it
  structural.

### fetch-only-on-clean-exit

- Decision: the fetch/validate runs only when the agent session exits 0. A non-zero agent
  exit returns the existing `claude exited with code N` error without attempting a fetch.
- Rationale: a crashed agent session is already a failure the operator must see; a partial or
  absent report from a crashed session is not worth special-casing. Keeps the control flow
  simple and the existing non-zero-exit test intact.
- Rejected: always attempt the fetch regardless of exit code — muddies which failure the
  operator is looking at.

## Technical context

- **`tools/sandbox/` is a standalone `package main`** in the loomyard module
  (`github.com/Knatte18/loomyard`), built/run via `sandbox.cmd` (`go run ./tools/sandbox
  -parent C:\Code %*`). `sandbox.cmd` does `pushd "%~dp0"` first, so the tool's cwd is the
  loomyard repo root and `%~dp0` *is* that root.
- **`main.go`** parses flags and dispatches `build` (default) vs `suite`. The `suite` branch
  parses `-claude`/`-prompt` and calls `runSuite(absParent, claude, prompt)`. A new
  **top-level** `-loomyard` flag (on `fs`, alongside `-parent`/`-reset` — *not* on the
  suite-local `sf`, because `sandbox.cmd`'s `%*` puts `suite` after the launcher's own flags)
  threads the loomyard root into `runSuite` as an added parameter.
- **`suite.go`** today: fingerprints `lyx` (`binaryFingerprint` → `binaryInfo{Path,Size,
  ModTime,SHA256[:12]}`), renders `SANDBOX-SUITE.md` (header + embedded body) into
  `<parent>/lyx-test-HUB/lyx-test/`, registers it in that repo's `.git/info/exclude`
  (`ensureGitExclude`, idempotent), then `launchAgent`s claude with cwd = host repo. **It
  does no retrieval today** — `runSuite` returns right after the launch. The new fetch logic
  goes after the successful `launchAgent` return.
- **`binaryInfo`** already holds exactly the `meta.fingerprint` fields. `header()` renders the
  markdown fingerprint block; its prose ("Every issue filed … must include that fingerprint")
  must be rewritten since issues are gone — its new purpose is operator/provenance context,
  and `meta.fingerprint` is now sourced from `binaryInfo` directly.
- **The contract** (must match millhouse#586):
  ```json
  {
    "source": "sandbox-report",
    "meta": { "fingerprint": { "path": "...", "sha256": "...", "size": 0, "modtime": "..." } },
    "items": [
      { "ref": "S6", "title": "terse errors, no --help hint",
        "body": "verdict: WARN\n\n…repro…" }
    ]
  }
  ```
  - `source`: literal `"sandbox-report"` (discriminator telling the analysis nothing external
    needs closing).
  - `meta.fingerprint`: `path` (string), `sha256` (the `SHA256[:12]` prefix — same value used
    in the `.scratch` filename), `size` (int bytes), `modtime` (RFC3339 string, as `header()`
    formats it).
  - `items[]`: WARN/FAIL findings only. `ref` = suite step id (`S0`,`S1`,…,`S6`); `title` =
    short summary; `body` = detail + repro + verdict folded into one markdown string.
- **Suggested file split:** put the contract struct types + `validate`/`stamp`/`fetch`
  helpers in a new `tools/sandbox/report.go`; keep `suite.go` as orchestration. Matches the
  existing one-concern-per-file layout (`main.go` dispatch, `suite.go` launcher).
- **Filename fingerprint:** `<sha12>` = `binaryInfo.SHA256` (already the first 12 hex chars).
  Used for both the `.scratch` filename and `meta.fingerprint.sha256`.
- **`.scratch/` is gitignored** at the loomyard root (`.gitignore`: `**/.scratch/`); the tool
  must `os.MkdirAll` it before writing. It does not exist by default.
- **Existing tests** (`suite_test.go`) cover fingerprinting, `renderScheme`, `ensureGitExclude`
  (4 cases), and `runSuite` orchestration via `lookPath`/`launchAgent` seams + temp host repo.
  The new tests follow the same seam/temp-dir pattern; the fetch helper should be a pure
  function whose signature is `(hostRepoDir, loomyardRoot string, info binaryInfo) error` —
  the destination argument is the **loomyard root**, and the helper itself joins `.scratch`
  and `MkdirAll`s it (it does not receive a pre-joined `.scratch` dir). That keeps it testable
  directly without launching anything.
- **No `_codeguide/`** in this repo; navigation is via `docs/overview.md`, `git log`, and grep.
- Both docs reference `tools/sandbox/test-scheme.md` (howto "See also"; hub step 3) — these are
  the stale references to repoint at `SANDBOX-SUITE.md` per Decision: scheme-filename.

## Constraints

From `CONSTRAINTS.md` (hub root):

- **Path Invariant.** `tools/sandbox/*.go` is scanned by `internal/paths/enforcement_test.go`
  and is **not** allowlisted (only `internal/paths` and `cmd/lyx/main.go` are). Therefore no
  `os.Getwd` and no `git rev-parse --show-toplevel` may appear in any non-test `.go` file
  under `tools/sandbox`. The loomyard root must arrive as data (the `-loomyard` flag), not be
  discovered. The geometry-literal ban (`_board`,`-weft`,`-HUB`,`_portals`,`_launchers`,
  `_codeguide`,`_lyx`) is whole-token; `tools/sandbox` already uses compound names
  (`"lyx-test-HUB"`, `"lyx-test"`) that are not flagged, and the new code introduces no bare
  geometry token (`.scratch`/`sandbox-report.json`/`sandbox-report` are not in the ban list).
- **Documentation Lifecycle / Task completion (`CLAUDE.md`).** This change alters observable
  behaviour (suite no longer files issues; emits + fetches JSON), so the module/runbook docs
  (`docs/sandbox-howto.md`, `docs/sandbox-hub.md`) **must** be updated in the same commit.
  No new cross-cutting invariant is introduced, so `CONSTRAINTS.md` needs no new entry. This
  is delivered work, not a planned milestone, so `docs/roadmap.md` is **not** touched.
- **CLI / Cobra Invariant.** Not engaged — `tools/sandbox` is a standalone `flag`-based tool,
  not a cobra module under `cmd/lyx`. No `Command()`/`RunCLI` seam, no help-tree pins.
- **Help-prose accuracy.** No `lyx` command surface changes; the `selfreport` CLI is untouched.
  The only "help" text affected is the agent scheme + the two docs, all updated here.

## Testing

Go tests (`tools/sandbox/suite_test.go`), following the existing seam + `t.TempDir()` pattern;
`go build ./... && go test ./...` must stay green.

- **TDD candidate — the fetch/validate/stamp helper** (new, in `report.go`): pure function
  `(hostRepoDir, loomyardRoot string, info binaryInfo) error` — the destination is the
  **loomyard root**; the helper joins `.scratch` and `MkdirAll`s it. Write its tests first:
  - **Happy path:** a valid `{source:"sandbox-report", items:[…]}` file in the host repo →
    lands at `<loomyardRoot>/.scratch/sandbox-report-<sha12>.json`; the written file's
    `meta.fingerprint` equals the passed `binaryInfo` (path/size/modtime/sha256); `items` are
    preserved; any `meta` in the input is overwritten.
  - **Empty findings:** `items: []` (present but empty) is valid → still written (not treated
    as missing).
  - **Absent `items` key:** JSON with correct `source` but **no** `items` key → clear
    validation error (the `*[]Item` pointer is nil); distinct from the empty-but-present case
    above.
  - **Malformed report:** non-JSON / truncated file → clear parse error mentioning the path;
    nothing written to `.scratch`.
  - **Wrong shape:** valid JSON but `source` missing/incorrect → clear error (validation), not
    a silent pass.
  - **Missing report:** no `sandbox-report.json` in the host repo → clear missing-file message
    distinct from the parse error.
  - **`.scratch` created:** the loomyard root's `.scratch` dir does not pre-exist → the helper
    creates it (`MkdirAll`).
- **Clean-slate removal:** a `sandbox-report.json` left in the host repo from a prior run is
  removed before launch — assert that after `runSuite` with a stub `launchAgent` that writes
  **nothing**, the prior file is gone and the run reports the missing-report case (not a stale
  fetch). (See Decision: clean-slate-report.)
- **`runSuite` wiring:** extend the existing `launchAgent`-stub tests so a stub that "writes" a
  valid report into the temp host repo results in the report landing under the temp loomyard
  root's `.scratch`; the existing `TestRunSuite_NonZeroLaunchCode` must still pass (non-zero
  exit → no fetch attempt, original error returned). `runSuite` now takes the loomyard root as
  a parameter — update the existing call sites/tests accordingly.
- **`ensureGitExclude` for the report:** assert the host repo's `.git/info/exclude` contains
  `sandbox-report.json` after a run (idempotent alongside the existing `SANDBOX-SUITE.md`
  entry).
- **Manual (operator) check, not automated:** a real `sandbox.cmd suite` run produces
  `.scratch/sandbox-report-<fingerprint>.json` on the contract and creates **no** GitHub
  issues. (Documented in the howto; not a Go test.)

## Q&A log

- **Q:** Who populates `meta.fingerprint`? **A:** `suite.go` stamps it from `binaryInfo`; the
  agent writes only `{source, items}`. Avoids LLM-transcription error.
- **Q:** How does `suite.go` find the loomyard root for `.scratch/` given the Path Invariant
  bans `os.Getwd` there? **A:** It doesn't discover it — the LoomYard-side launcher
  (`sandbox.cmd`), which sits *on* the loomyard root (`%~dp0`), passes it in as a flag, just
  like `-parent`. The sandbox agent never needs to know; it only writes inside the sandbox.
- **Q:** How strict should the post-fetch validation be? **A:** Typed decode + require
  `source == "sandbox-report"` and a present `items` array — catch valid-JSON-wrong-shape
  before triage.
- **Q:** The proposal/docs reference `tools/sandbox/test-scheme.md` — handle it how? **A:**
  That file does not exist; the real file is `SANDBOX-SUITE.md`. This is a bug — keep the name
  `SANDBOX-SUITE.md` and fix the stale `test-scheme.md` references in both docs.
- **Q:** Byte-copy or re-serialize into `.scratch`? **A:** Re-serialize (decode → stamp `meta`
  → marshal), since a byte copy can't carry the stamped fingerprint.
- **Q:** Fetch on a non-zero agent exit? **A:** No — only on a clean (exit 0) session; a
  non-zero exit returns the existing error without fetching.
