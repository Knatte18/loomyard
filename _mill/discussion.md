# Discussion: dev/test `lyx` separated from production deploy

```yaml
task: dev/test lyx.exe separated from production deploy
slug: dev-test-binary
status: discussing
parent: main
```

## Problem

Today there is exactly **one** deploy target for the `lyx` binary. `deploy.cmd`
builds `lyx` from the current checkout and installs it to a single location on PATH —
that same binary is *both* "the binary an operator uses day to day" *and* "the binary
every review/sandbox/test flow exercises." There is no separation.

The consequence is a structural footgun the codebase already documents but only mitigates
by discipline: any review/sandbox run that deploys before it finishes validating can
**clobber the stable production binary** with an in-progress "test variant." A test run
that never finishes, or fails partway, leaves the operator's production `lyx` pointing at
unvetted code. Every `crucible/*.md` prompt carries a "re-deploy after EVERY source change"
warning precisely because the current design is aware of this risk but does not prevent it
structurally.

**Why now:** this is a direct operational requirement (not derived from the vacation
design-discussion issues). The full design lived at `manifest/designs/dev-test-binary.md`;
per the documentation lifecycle, building this converts that doc into a `CONSTRAINTS.md`
invariant and deletes the design doc in the same commit.

## Scope

**In:**

- A **second, dev-only deploy target** for `lyx`, structurally distinct from the production
  install location, whose path is **derived** (never hardcoded) — `<repoRoot>/.dev-bin/lyx`
  (`.exe` on Windows), with `.dev-bin/` gitignored.
- A `-dev` flag on `tools/deploy` that builds into that derived `.dev-bin` directory, plus
  sibling launchers `deploy-dev.cmd` (Windows) and `deploy-dev` (POSIX shell) that call it.
- `tools/sandbox` binary resolution stops doing a bare `lookPath("lyx")` and instead
  resolves the derived dev binary first (falling back to PATH when no dev binary exists), at
  all three current call sites: `suite.go` (`runSuite`), `report.go` (`runFetch`),
  `main.go` (`cloneRun`).
- The launched black-box **agent** exercises the dev binary by prepending the derived
  `.dev-bin` directory to the **agent child process** PATH only. The post-session `mux down`
  already runs the resolved dev binary by **absolute path** (and `lyx mux` re-invokes itself
  via `os.Executable()`, i.e. the same dev binary), so it needs no PATH change.
- A visible **dev/prod source marker** in the sandbox fingerprint block (and thus in
  `sandbox-report.json`).
- Full documentation/prompt sweep so every "deploy before testing" instruction targets the
  dev flow: `crucible/{board,builder,webster,review-prompt-template,README,orchestrator}.md`,
  all seven `tools/sandbox/SANDBOX-*-SUITE.md` deploy-instruction lines,
  `docs/sandbox-howto.md`, `docs/overview.md` (§sandbox), `docs/sandbox-hub.md`.
- New `CONSTRAINTS.md` invariant recording the dev/prod separation; deletion of
  `manifest/designs/dev-test-binary.md`.

**Out:**

- **No change to the production deploy path or its behaviour.** `deploy.cmd` / the existing
  `-dest` prod install location is untouched. Prod-only flows (no dev binary present) behave
  exactly as today via the PATH fallback.
- **No de-hardcoding of the existing prod launchers** (`deploy.cmd`'s `C:\Code\tools\bin`,
  the sandbox launchers' `-parent C:\Code`). That pre-existing machine-path hardcoding lives
  only in the `.cmd` launcher layer (Go tooling is already general); cleaning it up is a
  separate task with a larger blast radius (it moves where prod lands). Explicitly deferred.
- **No renaming of the binary.** The dev build keeps the name `lyx`(`.exe`); disambiguation
  is by directory + fingerprint, not by name (see Decisions).
- **No `LYX_DEV_BIN` (or any) env-var / persistent config.** The dev path is fully derived.
- **No dedicated separate mechanism for the Go smoke/integration tests** — they already build
  their own binary from the working tree (`buildLyxBinary` → `go build -o <temp> ./cmd/lyx`)
  and never consume the deployed binary, so they are already isolated from deploy.

## Decisions

### same-binary-name-separate-directory

- **Decision:** The dev build is the file **`lyx`(`.exe`)** in a *separate directory*
  (`<repoRoot>/.dev-bin/`), not a distinctly-named binary like `lyx-dev`.
- **Rationale:** The seven `SANDBOX-*-SUITE.md` black-box suites contain **hundreds** of
  literal `lyx <subcommand>` command lines (68/47/40/29/25/24/20 across the suites) that the
  agent types verbatim, and the CORE suite's stated contract is *"tests `lyx.exe` as a black
  box — exactly as a real user with only the binary on PATH."* A real user types `lyx`.
  Renaming to `lyx-dev` would force rewriting 250+ command lines across 7 files, break that
  fidelity contract, and merely *relocate* the silent-fallback risk (miss one `lyx ` → test
  prod). Disambiguation is instead solved by (a) explicit directory resolution in the tooling
  and (b) the SHA256 fingerprint already stamped into every suite file and report — stronger
  than a name.
- **Rejected:** Distinct name `lyx-dev` (churn + fidelity loss + relocated footgun);
  both-name-and-dir differ (maximum churn, YAGNI).

### derived-dev-path-never-hardcoded

- **Decision:** The dev directory is **`<repoRoot>/.dev-bin/`**, with `repoRoot` derived via
  `runtime.Caller` (the exact mechanism `tools/deploy`'s existing `repoRoot()` already uses).
  `.dev-bin/` is added to the root `.gitignore`. No machine-specific path is introduced
  anywhere — not in Go, not in the launchers.
- **Rationale:** Operator constraint: *never hardcode machine paths.* A repo-relative derived
  path is per-worktree, cross-platform (Windows/Linux) for free, and needs zero config. The
  sandbox tool runs as `go run ./tools/sandbox` compiled from the repo, so `runtime.Caller`
  yields the source path → repo root, identically to the deploy tool.
- **Rejected:** Hardcoded sibling dir (e.g. `C:\Code\tools\dev-bin`) — violates the
  constraint; an `LYX_DEV_BIN` env var — needs operator config and can drift; a fixed default
  baked into the general `tools/deploy` tool — that would be a machine-ish default in a tool
  whose contract is to stay general (a *derived* repo-relative path is acceptable because it
  is computed, not fixed).

### shared-devbin-helper

- **Decision:** Put the path convention in **one** place — a small shared package
  `tools/internal/devbin` exposing the repo-root derivation and the `.dev-bin` binary/dir
  paths — imported by both `tools/deploy` and `tools/sandbox`. (Both live under `tools/`, so
  `tools/internal/...` is importable by each.) Exact API is mill-plan's call; the invariant is
  that deploy and sandbox never disagree on where `.dev-bin` is.
- **Rationale:** Two tools must resolve the identical location; a single source of truth
  prevents divergence. Duplicating the `.dev-bin` string + `runtime.Caller` depth in two
  `main` packages is a correctness hazard, not just DRY.
- **Rejected:** Independent `repoRoot()`/path logic in each tool (drift risk); a helper under
  product `internal/` (couples the shipped product to dev-only tooling — keep it under
  `tools/`).

### resolution-with-path-fallback

- **Decision:** A `resolveLyx()` helper in `tools/sandbox` returns `(path, source)` where
  `source ∈ {dev, prod}`: if the derived `<repoRoot>/.dev-bin/lyx` **exists**, return it with
  `source=dev`; otherwise fall back to the existing `lookPath("lyx")` seam with `source=prod`.
  Applied at `runSuite`, `runFetch`, and `cloneRun`.
- **Rationale:** Backward-compatible by construction — with no dev binary deployed, every
  path behaves exactly as today (prod on PATH). Stops the "bare `lookPath` against ambient
  PATH" non-determinism the design called out, without breaking existing prod flows. The
  `source` value drives the fingerprint marker (Q8).
- **Existence-check is sufficient (not a freshness footgun):** `resolveLyx` selects
  `.dev-bin/lyx` on `os.Stat` existence alone. This does **not** reintroduce the clobber risk
  for dev: `go build -o <path>` writes to a temp file and atomically renames it into place, so
  an aborted/partial `deploy-dev` leaves the **prior** dev binary intact, never a half-written
  one at the final path. Any *staleness* of the dev binary (an old but complete build) is
  deliberately **accepted and surfaced by the fingerprint** — the SHA256 + the new `Source:`
  marker (below) are exactly the mechanism a reviewer uses to notice they tested an old build.
  A freshness guard beyond existence (e.g. rebuild-on-resolve, mtime-vs-source checks) is
  **out of scope**.
- **Rejected:** Hard-require a dev binary (breaks first-run / prod-only flows); env-var
  override (no config, per above); an existence-plus-freshness check (out of scope; atomicity
  + fingerprint already cover the failure the reviewer raised).

### agent-path-prepend-child-only

- **Decision:** When `source=dev`, prepend `filepath.Dir(devPath)` (i.e. `.dev-bin`) to the
  **agent child process** PATH for `launchAgent` **only** (it currently inherits the parent
  env unchanged). `muxDown` is **not** env-threaded — it already execs the resolved dev binary
  by absolute path, and internal re-invocation uses `os.Executable()` (below). The dev
  directory is **never** placed on the operator's own PATH.
- **Rationale:** Q7 — bare `lyx` in an operator shell must stay prod (safe default; dev is
  never run in production by accident, prod is never clobbered). The agent inside the Hub
  keeps typing bare `lyx`; prepending `.dev-bin` to its PATH makes that resolve to dev
  deterministically, while the fingerprint proves which binary ran. `mux down` is different:
  `muxDown` runs `exec.Command(lyxPath, "mux", "down")` with the **absolute** resolved dev path
  (`suite.go` ~208), and `lyx mux` re-invokes lyx via `os.Executable()` — the running dev
  binary — not a bare `lyx` PATH lookup (`internal/muxengine/lifecycle.go:524`,
  `headerpane.go`). So prepending `.dev-bin` to muxDown's PATH would resolve nothing; the
  absolute-path pass-through already guarantees the dev binary. When `source=prod`, no PATH
  change (backward compat).
- **Rejected:** Rewriting every SUITE.md `lyx` call to an absolute path (touches all 7 docs,
  brittle); putting `.dev-bin` on the operator PATH (Q7 rejected — risks running dev in prod);
  env-threading `muxDown` (redundant — it already runs the dev binary by absolute path).

### fingerprint-source-marker

- **Decision:** Add a `Source` field to `binaryInfo` and a `Source:` line to the fingerprint
  header (e.g. `Source: .dev-bin (dev)` vs `Source: PATH (prod)`), so it appears both in the
  stamped suite file and in `sandbox-report.json`'s `meta.fingerprint`.
- **Rationale:** Q8 — the `Path` line already shows the directory, but an explicit dev/prod
  label makes it unmistakable for anyone skimming a report which binary produced a finding.
  Cheap, directly serves the legibility concern behind the whole task.
- **Rejected:** Rely on the `Path` line alone (less legible to a reader unfamiliar with the
  convention).

### deploy-dev-launchers

- **Decision:** Add a `-dev` flag to `tools/deploy/main.go` that installs into the derived
  `.dev-bin` (via `tools/internal/devbin`), and add sibling launchers `deploy-dev.cmd`
  (Windows) and `deploy-dev` (POSIX shell) that are simply `go run ./tools/deploy -dev` — no
  hardcoded path in either launcher. Both platforms (Q4) because the derived path is
  cross-platform.
- **Rationale:** Keeps the general tool general (the `-dev` target is *derived*, not a fixed
  machine path), gives operators a one-word command mirroring `deploy.cmd`, and works
  identically on Windows and the current Linux host without per-machine editing.
- **Rejected:** Sibling launcher with a hardcoded dev `-dest` (violates the no-hardcode
  constraint — this superseded the earlier Q5 recommendation); requiring operators to pass
  `-dest` by hand each run (ergonomics).

## Technical context

Relevant files and current behaviour (all under the repo root
`/home/knatte/Code/loomyard/wts/dev-test-binary`):

- **`tools/deploy/main.go`** — general deploy tool. Builds `./cmd/lyx` → `lyx`(`.exe`),
  `-dest` else `go env GOBIN`/GOPATH-bin. Already derives the module root via
  `repoRoot()` using `runtime.Caller(0)` — the pattern to reuse for `.dev-bin`. The `-dev`
  flag installs to `devbin.Dir()`; `-dev` and `-dest` are mutually exclusive (error if both).
- **`tools/sandbox/suite.go`** — `runSuite` (line ~366) does `lookPath("lyx")`; the resolved
  path feeds `binaryFingerprint` (the stamped header) **and** `muxDown(hostRepoDir, lyxPath)`.
  `launchAgent(hostRepoDir, claudePath, instruction)` and `muxDown(hostRepoDir, lyxPath)` are
  package-var seams that currently do **not** set `cmd.Env` (they inherit). **Only
  `launchAgent`** gains the dev-bin dir (so it can build a PATH-prepended env for the agent);
  `muxDown` keeps its current signature and behaviour (it already execs the resolved absolute
  `lyxPath`). `binaryInfo`/`header()` gain the `Source` field/line.
- **`tools/sandbox/report.go`** — `runFetch` (line ~81) does the same `lookPath("lyx")` for
  the fetch-time fingerprint; switch to `resolveLyx()` (no agent launch here, so no PATH
  prepend — fingerprint + `Source` only).
- **`tools/sandbox/main.go`** — `cloneRun` (line ~34) is a package-var seam currently shaped
  `func(parentDir string) error` that hardcodes `exec.Command("lyx", "warp", "clone", …)` and
  a `"lyx not found on PATH"` startup message (~33–45). Change: the caller resolves the binary
  via `resolveLyx()` and passes it in — **new seam shape `cloneRun(parentDir, lyxPath string)`**
  — and `cloneRun` execs `lyxPath` so Hub provisioning uses the dev binary when present (else
  prod via fallback). The `"lyx not found on PATH"` startup-error string goes **stale** (the
  path is now resolved upstream, not looked up here) and must be updated (e.g. to reference the
  resolved path / point at `deploy-dev`).
- **`lookPath`** in `tools/sandbox/suite.go` is a package-var seam (`var lookPath =
  exec.LookPath`) — reuse it for the prod fallback and in tests.
- **Consumers of the *deployed* binary** are only the three `tools/sandbox` sites above plus
  the black-box agent-in-Hub. The Go smoke/integration tests build their own binary
  (`internal/muxcli/smoke_test.go:buildLyxBinary`) and are not consumers.
- **Launchers** `deploy.cmd` and `sandbox-*.cmd` hardcode `C:\Code\tools\bin` / `-parent
  C:\Code` in the `.cmd` layer only (out of scope to change). New `deploy-dev.cmd` /
  `deploy-dev` carry no path.
- **Docs/prompts with deploy instructions** to retarget: `crucible/board-review-prompt.md`
  (~217–221, 283), `crucible/builder-review-prompt.md` (~353, 450), `crucible/webster-
  review-prompt.md` (~332–335, 420), `crucible/review-prompt-template.md` (~123, 206),
  `crucible/README.md` (~191), `crucible/orchestrator-prompt.md` (~34); each
  `SANDBOX-*-SUITE.md`'s "Deploy a fresh binary" line (CORE ~17); `docs/sandbox-howto.md`
  (deploy step ~54, prereqs, troubleshooting); `docs/overview.md` (~439); `docs/sandbox-hub.md`
  (~8, 44, 81, 86, 97, 179). `crucible/gitrepo-review-prompt.md` explicitly has "no deploy
  step" — leave it.

Gotchas:

- `go run ./tools/...` compiles to a temp dir but `runtime.Caller(0)` still returns the
  compile-time **source** path, which is why repo-root derivation works — do not switch to
  `os.Executable()`.
- The prod PATH entry may still contain a `lyx`; the whole point is that resolution must
  prefer the derived `.dev-bin` explicitly rather than trusting PATH order.
- `.dev-bin/` must be gitignored *and* excluded from the Hub host repo's untracked-file
  noise is **not** a concern here (the dev binary lives in the source repo, not the Hub).

## Constraints

From `CONSTRAINTS.md` (hub root) and this discussion:

- **Sandbox Suite Coverage / Test Tier Purity / Hermetic Git Test Environment invariants** —
  the new tests must stay **Tier 1 pure**: untagged test files perform no `exec.Command`,
  `gitexec.RunGit`, or fixture-tree copies. Use the existing package-var seams (`lookPath`,
  `launchAgent`, `muxDown`, `cloneRun`) and pure helpers; do not spawn real processes or a
  real `lyx`.
- **CLI / Cobra Invariant** — not touched (no new cobra command; `tools/*` are standalone
  `main` packages, not lyx modules).
- **Documentation Lifecycle** — this task ships the behaviour *and* the docs in the same
  commit(s): add the new invariant to `CONSTRAINTS.md` and **delete**
  `manifest/designs/dev-test-binary.md` (the design doc self-declares this).
- **New invariant to add — "Dev/Prod Binary Separation":** the sandbox tooling resolves the
  dev binary from the derived `.dev-bin` (falling back to PATH), never a bare `lookPath("lyx")`
  that could silently resolve prod; the dev binary is never installed to the prod location;
  `.dev-bin/` is gitignored; the dev directory is prepended only to child-process PATH, never
  the operator's. Enforcement is partly a guard test (below) and partly review discipline —
  record it in the same commit per CLAUDE.md.

## Testing

All tests **Tier 1 / untagged / no real spawns**, via the existing seams:

- **`tools/internal/devbin`** (TDD candidate) — unit-test the derived paths: `Dir()` /
  binary path resolve to `<repoRoot>/.dev-bin` and `.../lyx` with the correct extension per
  `runtime.GOOS` (`.exe` on Windows). Pure.
- **`resolveLyx()`** (TDD candidate) — with a temp `.dev-bin/lyx` present → returns that path
  with `source=dev`; absent → falls back through the `lookPath` seam and returns `source=prod`;
  seam returning an error → propagated. Inject via the `lookPath` package var and a temp dir.
- **PATH prepend helper** (TDD candidate) — a pure `prependPath(dir, environ)`: asserts `dir`
  is first on the resulting `PATH`, the prior PATH entries are preserved after it, other env
  vars are untouched, and an empty `dir` yields the env unchanged. Pure.
- **Fingerprint `header()` / `binaryInfo`** — includes the `Source:` line for both `dev` and
  `prod` inputs.
- **`launchAgent` env threading** — replace the seam in-test to capture the env it is handed;
  assert `.dev-bin` is prepended when `source=dev` and the env is unchanged when `source=prod`.
  (`muxDown` is **not** env-threaded — assert only that it still receives/execs the resolved
  absolute `lyxPath`, unchanged from today.)
- **`cloneRun`** — with the new `cloneRun(parentDir, lyxPath)` seam, replace it to assert the
  caller passes the resolved dev path when a dev binary is present (else the PATH fallback).
- **Guard test for the new invariant** — a cheap string/AST scan asserting `lookPath("lyx")`
  appears only inside `resolveLyx` across `tools/sandbox/*.go`, so no future site regresses to
  bare PATH resolution.
- **Existing suite/report/main tests** — keep green; update any that asserted the old bare
  `lookPath("lyx")` resolution.
- **No new integration/smoke tests required** — the change is resolution/env plumbing,
  coverable at Tier 1.

## Q&A log

- **Q:** With same name in a different directory, how do consumers know which `lyx` they use?
  **A:** Disambiguation is by explicit directory resolution in the tooling + the SHA256
  fingerprint (already stamped everywhere), not by name. Go tests build their own binary and
  aren't consumers; the only deployed-binary consumers are the 3 sandbox sites + the agent,
  all of which resolve/fingerprint the dev binary deterministically.
- **Q:** Separate binary name (`lyx-dev`) or separate directory? **A:** Separate directory,
  name stays `lyx` — renaming would rewrite 250+ black-box command lines and break the "exactly
  what a real user types" suite contract.
- **Q:** Is anything really hardcoded to `C:\Code\tools` today? **A:** Yes — in the `.cmd`
  launchers only (`deploy.cmd` `-dest C:\Code\tools\bin`; `sandbox-*.cmd` `-parent C:\Code`),
  not in Go. Deliberate "machine paths in the caller" pattern. It is Windows-only and doesn't
  apply to the current Linux host.
- **Q:** Hardcode the dev directory? **A:** No — never hardcode. Derive it as
  `<repoRoot>/.dev-bin/` via `runtime.Caller`; gitignored; cross-platform; zero config.
- **Q:** Put dev-bin on the operator's PATH? **A:** No (Q7) — bare `lyx` in a shell must stay
  prod so dev is never run in production by accident. Prepend `.dev-bin` only to the agent
  child-process PATH.
- **Q:** Visible dev/prod marker in the fingerprint? **A:** Yes (Q8) — a `Source:` line
  (`dev`/`prod`) in the fingerprint header and report.
- **Q:** Docs/prompt update scope? **A:** Full sweep (Q9) of all crucible prompts with deploy
  instructions, all 7 SUITE.md deploy lines, `sandbox-howto.md`, `overview.md`, `sandbox-hub.md`.
- **Q:** De-hardcode the existing prod launchers now? **A:** No (Q10) — out of scope; separate
  task with larger blast radius. This task introduces zero new hardcoding but leaves the
  existing prod `.cmd` hardcoding alone.
- **Q:** Does `mux down` also need the `.dev-bin` PATH prepend? (discussion-review r1 gap)
  **A:** No — `muxDown` execs the resolved dev binary by absolute path and `lyx mux`
  re-invokes via `os.Executable()` (`muxengine/lifecycle.go:524`), so the prepend is redundant.
  Only `launchAgent` (agent types bare `lyx`) gets it. Resolved without user input: pure
  technical correctness, one right answer.
