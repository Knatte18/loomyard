# Discussion: Extract shared primitives (paths, output)

```yaml
task: Extract shared primitives (paths, output)
slug: mhgo-extract-primitives
status: discussing
parent: main
```

## Problem

Three small, behaviour-preserving extractions are needed in the `mhgo` Go CLI
before the `worktree` module (roadmap milestone 4/5) can be built cleanly. Today
the only user-facing module is `board`, and a handful of mechanical primitives
that future modules will all need are either buried inside `board`/`config` or
absent entirely:

1. `_mhgo/` directory resolution is inlined inside `config.Load`, so a module
   can only discover "where is the initialised base dir / is this dir
   initialised" by loading a full module config (reading YAML, `.env`, expanding
   env vars). The upcoming `worktree` module needs the base-dir resolution
   without paying for a config load.
2. There is no named helper for "find the git repository root"
   (`git rev-parse --show-toplevel`). `worktree` needs the repo root to compute
   the container directory, and the windowless-git rule (docs/shared-libs/git.md)
   means every git invocation must go through `internal/git`.
3. The JSON envelope writers (`{"ok":true,...}` / `{"ok":false,"error":...}`) are
   duplicated across `internal/board/cli.go` and `internal/board/init.go`. Every
   future module will emit the same envelope shape; the writer belongs in one
   shared, tested place.

**Why now:** these are the prerequisites for the `worktree` module. The roadmap
makes refactor milestones explicitly behaviour-preserving — board's existing
test suite is the guardrail, so nothing observable changes until the new module
that needs the extracted lib arrives.

## Scope

**In:**

- `internal/config`: add `FindBaseDir(cwd string) (string, error)` that performs
  the `_mhgo/` existence check and returns the base dir; refactor `Load` to call
  it for that check.
- `internal/git`: add `FindRoot(cwd string) (string, error)` wrapping
  `RunGit(["rev-parse","--show-toplevel"], cwd)`.
- New package `internal/output`: `Ok(w io.Writer, fields map[string]any) int` and
  `Err(w io.Writer, msg string) int`.
- Switch `internal/board` (`cli.go`, `init.go`) over to the new `output` helpers
  and to `config.FindBaseDir` where applicable — board is the guardrail that
  proves the extraction is behaviour-preserving.
- Unit tests for all three new helpers.
- Update `docs/shared-libs/config.md` and `docs/shared-libs/git.md` to document
  the new helpers.

**Out:**

- No upward/parent-directory walk for `FindBaseDir` — cwd-authoritative resolution
  is preserved exactly (see Decisions).
- No new `git.FindRoot` call site in `board` — board has no need for the repo
  root; the helper is additive, exercised by its own tests, and consumed later by
  `worktree`.
- No change to the config grammar, env expansion, `.env` loading, or the
  three-layer/`.mhgo/` redesign (that is a separate, already-completed/planned
  milestone).
- No change to JSON key *values* or the set of keys any command emits. (Key
  *ordering* is already alphabetical via `map[string]any` marshaling and is not
  asserted by the guardrail tests anyway.)
- No changes to `mux`, `state`, or any not-yet-existing module.

## Decisions

### FindBaseDir — strict cwd check, no upward walk

- Decision: `FindBaseDir(cwd string) (string, error)` checks whether
  `filepath.Join(cwd, "_mhgo")` exists and is reachable. If it exists, return
  `cwd` (the base dir) and `nil`. If it does not exist, return `""` and an error
  whose message contains `"not initialized"`. `os.Stat` errors other than
  `IsNotExist` are wrapped and returned. No walking up to parent directories.
- Rationale: behaviour-preserving. This is exactly what `config.Load` does today
  at `internal/config/config.go:36-43`. The config model is **cwd-authoritative**
  (docs/shared-libs/config.md): the cwd does not need to equal the git root, and
  `_mhgo/` presence in cwd is what makes a directory "initialised". An upward walk
  would change board's "not initialized" semantics and break
  `TestLoad_UninitializedDir`.
- Rejected: walk cwd→parents for the first `_mhgo/`. More convenient for
  subdirectory invocation, but contradicts the documented cwd-authoritative
  invariant and is not behaviour-preserving.

### FindBaseDir error text and Load wiring

- Decision: `FindBaseDir` returns the same generic message `Load` returns today —
  `"not initialized: _mhgo/ directory not found in %s"` (with the base dir) — and
  wraps non-`IsNotExist` stat errors as `"stat _mhgo: %w"`. `Load` is refactored
  so its existence-check block delegates to `FindBaseDir(baseDir)` instead of
  inlining the `Stat`. `Load` keeps its current signature
  `Load(baseDir, module string, defaults map[string]string)`.
- Rationale: board's `LoadConfig` (internal/board/config.go:74-80) detects the
  substring `"not initialized"` and rewraps it to
  `not initialized here; run "mhgo init"`. Preserving the generic text keeps that
  detection and the board-level message unchanged. Keeping `Load`'s signature
  avoids touching every caller.
- Rejected: changing `Load` to take `cwd` and return the resolved base dir — a
  larger, unnecessary signature churn for no behavioural gain in the
  cwd-authoritative model (base dir == cwd).

### git.FindRoot — additive wrapper over RunGit

- Decision: `FindRoot(cwd string) (string, error)` calls
  `RunGit([]string{"rev-parse", "--show-toplevel"}, cwd)`. On a successful run
  (exit code 0) return `strings.TrimSpace(stdout)`. On a non-zero exit (e.g. 128,
  not a git repo) return an error that includes the captured stderr. Propagate the
  underlying `err` from `RunGit` when it is non-nil (process failed to start).
- Rationale: matches the brief and the `internal/git` design (docs/shared-libs/
  git.md) — the package centralises only the windowless `RunGit` primitive plus
  thin, opinion-free helpers; `FindRoot` adds no command *sequence*, just a single
  named git invocation. `git` outputs a forward-slash absolute path with a
  trailing newline; trimming whitespace is the only normalisation. Returning the
  raw (forward-slash) path is fine — downstream callers normalise as needed.
- Rejected: adding a board call site now (speculative — board operates on the
  board repo path, never needs the toplevel); `filepath.Clean`-ing or
  backslash-converting the result (premature; leave normalisation to the consumer).

### internal/output — Ok/Err returning exit codes

- Decision: new package `internal/output` (`internal/output/output.go`) with:
  - `func Ok(w io.Writer, fields map[string]any) int` — sets `fields["ok"] = true`
    (mutating the passed map is acceptable; callers pass freshly-built literals),
    marshals the single map to JSON, writes it as one line (`fmt.Fprintln`) to `w`,
    returns `0`.
  - `func Err(w io.Writer, msg string) int` — marshals
    `map[string]any{"ok": false, "error": msg}`, writes one line to `w`, returns
    `1`.
  - JSON marshal errors are ignored (`data, _ := json.Marshal(...)`), matching the
    existing `writeJSON` behaviour.
- Rationale: this is the exact shape board already emits via `writeJSON` +
  `map[string]any` literals; using a single map keeps Go's alphabetical key
  ordering identical to today. Returning the exit code lets call sites stay
  `return output.Ok(...)` / `return output.Err(...)`, matching cli.go's current
  `return outputSuccess(out)` / `return outputError(out, msg)` pattern.
- Rejected: variadic `Ok(w, key, val, …)` or dedicated typed structs — more
  ceremony and a worse fit for board's existing map-based call sites; a `void`
  `Err` (init.go's current `outputInitError` shape) — returning `1` is strictly
  more ergonomic and lets init.go collapse `outputInitError(...); return 1` into
  `return output.Err(...)`.

### board switches to the new helpers (the guardrail)

- Decision: rewire `internal/board` so the extraction is proven by board's
  existing suite:
  - `cli.go`: keep the typed wrapper helpers (`outputSuccess`,
    `outputSuccessWithCount`, `outputSuccessWithTask`, `outputGetTask`,
    `outputListBrief`, `outputListFull`, `outputError`) as **thin shims** whose
    bodies now delegate to `output.Ok` / `output.Err`. Remove the private
    `writeJSON` (its callers go through `output`). Call-site code in `RunCLI` is
    unchanged.
  - `init.go`: replace `outputInitError` body with a call to `output.Err`, and
    emit the success envelope via `output.Ok(out, map[string]any{...})` instead of
    the local `json.Marshal`/`Fprintln`. `RunInit` keeps returning its own exit
    code where the current control flow expects it.
  - `config.go` (`LoadConfig`): no required change — `Load` already routes through
    `FindBaseDir` internally, so board's `not initialized` rewrap keeps working.
    (Optional, only if clean: call `config.FindBaseDir` directly for the existence
    pre-check. Default is to leave `LoadConfig` as-is to minimise churn.)
- Rationale: the brief says "board switches to the new helpers as the guardrail."
  Keeping the typed wrappers as shims gives minimal diff and keeps `RunCLI`
  readable; delegating their bodies is what actually exercises `internal/output`
  through board's test suite.
- Rejected: inlining `output.Ok(out, map[string]any{...})` at every `RunCLI` call
  site and deleting the wrappers — larger diff in cli.go with no benefit.

## Technical context

Go 1.26 module `github.com/Knatte18/mhgo`. Layout:

- `cmd/mhgo/main.go` — thin module dispatcher (`init`, `board`).
- `internal/config/config.go` — generic two-layer config loader. `Load` at
  `:35`; the `_mhgo/` stat check to extract is at `:36-43`.
- `internal/git/git.go` — only `RunGit(args, cwd) (stdout, stderr, exitCode, err)`.
  Note: non-zero git exit is **not** an error (`err == nil`, `exitCode != 0`);
  only a failure to start the process yields `err != nil`/`exitCode == -1`.
  `FindRoot` must branch on `exitCode`, not just `err`.
- `internal/board/cli.go` — `RunCLI`; envelope helpers at `:271-317`
  (`writeJSON`, `outputError`, `outputSuccess`, `outputSuccessWithCount`,
  `outputSuccessWithTask`, `outputGetTask`, `outputListBrief`, `outputListFull`).
- `internal/board/init.go` — `RunInit`; local `outputInitError` at `:216` and an
  inline success-envelope marshal at `:93-101`.
- `internal/board/config.go` — `LoadConfig` at `:63`; rewraps the generic
  `"not initialized"` error into `not initialized here; run "mhgo init"` at
  `:77-79`.

Guardrail tests:

- `internal/config/config_test.go` — `TestLoad_UninitializedDir` pins the
  "not initialized" behaviour `FindBaseDir` must preserve; many `TestLoad_*` cover
  the unchanged config path.
- `internal/board/cli_test.go` — asserts on **parsed** JSON (`result["ok"]`,
  `result["task"]`, etc.), not byte-exact strings, so output key ordering is not
  part of the contract.
- `internal/git/git_test.go` — style reference for `FindRoot` tests (uses
  `git init` in a `t.TempDir()` then runs a `rev-parse`).

Design docs to keep aligned: `docs/shared-libs/config.md` (cwd-authoritative
rule), `docs/shared-libs/git.md` (RunGit-only-plus-thin-helpers rule),
`docs/modules/worktree.md` (the downstream consumer), `docs/roadmap.md`
(behaviour-preserving refactor framing).

## Testing

Behaviour-preserving refactor: board's existing suite must stay green throughout —
it is the primary guardrail. Plus targeted unit tests for each new helper.

- **`internal/output` (TDD candidate — new package, no existing guardrail):**
  - `Ok` writes a single JSON line containing `"ok":true` plus the supplied
    fields; parse the output and assert keys/values (do not assert byte-exact
    string ordering). Verify the returned exit code is `0`.
  - `Err` writes `{"ok":false,"error":<msg>}`; parse and assert `ok==false` and
    the error string; verify returned exit code is `1`.
  - Write to a `bytes.Buffer` as `io.Writer`.

- **`git.FindRoot` (TDD candidate — new helper):**
  - Inside a fresh `t.TempDir()` run `git init`, then `FindRoot(tempDir)` returns a
    non-empty path and `nil` error. (Account for symlink/`/private` path
    normalisation by comparing with a tolerance, e.g. suffix/`EvalSymlinks`, rather
    than exact-string equality — temp dirs on some platforms resolve differently.)
  - In a non-repo `t.TempDir()`, `FindRoot` returns a non-nil error (git exits
    128) and an empty path.

- **`config.FindBaseDir`:**
  - `<cwd>/_mhgo` present → returns the cwd and `nil`.
  - absent → returns `""` and an error containing `"not initialized"`.
  - The existing `TestLoad_*` suite continues to pass unchanged, confirming `Load`
    still behaves identically after delegating to `FindBaseDir`.

- **board regression:** run the full `internal/board` suite (including
  `boardtest`) after rewiring `cli.go`/`init.go`; all envelopes must parse to the
  same shape as before.

Build/test command: `go test ./...` from the repo root.

## Q&A log

- **Q:** Should `FindBaseDir` walk up the tree to find `_mhgo/`, or strictly check
  cwd? **A:** Strict cwd check — preserves cwd-authoritative behaviour and the
  existing "not initialized" semantics.
- **Q:** What is the `internal/output` API shape? **A:**
  `Ok(w, fields map[string]any) int` + `Err(w, msg string) int`, both returning
  the process exit code (0/1); `Ok` injects `"ok":true` into the map.
- **Q:** Keep board's typed wrappers or inline `output.Ok`? **A:** Keep them as
  thin shims delegating to `output`, to minimise churn and keep `RunCLI` readable.
- **Q:** Does `git.FindRoot` get a board call site now? **A:** No — additive only,
  tested on its own; the consumer is the future `worktree` module.
- **Q:** Update the shared-libs docs as part of this task? **A:** Yes — update
  `docs/shared-libs/config.md` and `git.md` to document the new helpers.
- **Q (tie-breaker):** Preserve the exact `Load` error text? **A:** Yes —
  `FindBaseDir` returns the same `"not initialized: _mhgo/ directory not found in
  %s"` so board's `LoadConfig` rewrap keeps matching.
- **Q (tie-breaker):** Does `Load` change signature? **A:** No — it keeps
  `Load(baseDir, module, defaults)` and calls `FindBaseDir(baseDir)` internally.
