# Discussion: Local lyx sandbox for manual experimentation

```yaml
task: Local lyx sandbox for manual experimentation
slug: lyx-sandbox
status: discussing
parent: main
```

## Problem

There is no one-command way to stand up a real, on-disk lyx **Hub** to drive `lyx`
against. The `internal/lyxtest` fixtures build host/weft repos in temp dirs and tear
them down — invisible and ephemeral — and warp is already covered by integration tests,
so poking warp by hand proves nothing.

The real purpose is a **dogfood bench**: a persistent Hub built from a small, real
project that lyx's machinery (today by hand or with Claude driving `lyx`; later by
`loom`) can operate in, where a failure is *signal* — when orchestration breaks against
this Hub, the break is a **LoomYard bug to fix**, not a problem with the bench. This task
delivers the tool that builds that Hub, plus durable docs recording the convention.

**Why now:** the real dogfood repos already exist on GitHub (`lyx-test`, `lyx-test-weft`),
`warp clone` and the deploy tool have landed, and the weft repo's wiki (the board) was
just initialized — so the Hub can be produced end-to-end today.

## Scope

**In:**

- A small **Go dev tool** at `tools/sandbox` (`package main`, run via `go run ./tools/sandbox`)
  that builds the dogfood Hub on disk by driving the **deployed** `lyx warp clone` against
  the real GitHub repos.
- A **very thin** `sandbox.cmd` launcher at the repo root (analogous in *shape* to
  `deploy.cmd`: pushd to repo root, `go run ./tools/sandbox`, pass the machine-specific
  parent dir). The launcher holds the machine value; the Go tool stays general.
- `--reset` behavior: rebuild the Hub from scratch when asked.
- A `--parent <dir>` flag selecting the directory the Hub is created **in**.
- A durable docs entry: a **new `docs/dogfood-hub.md`** plus a short **section in
  `docs/overview.md`** pointing to it, recording the two repo URLs, the board=weft-wiki
  derivation, the Hub-naming convention, and the dogfood purpose.

**Out:**

- **Deploying `lyx`.** The sandbox tool has nothing to do with building/installing
  `lyx.exe`. It drives whatever `lyx` is on PATH. If `lyx` is not deployed, the tool can
  do nothing — and that is by design, not a case the tool handles or papers over.
  `deploy.cmd` / `tools/deploy` stay completely independent; the sandbox tool never
  invokes them and is not modeled on them.
- **Seeding `lyx-test` with a real project.** Making the host repo a "small but real
  project" (real code + a test + a green/red build) is assumed already done (or a
  separate task). This task does not push content into `lyx-test`. *(Contents of
  `lyx-test` were not verified during discussion — a clone-peek was permission-blocked.)*
- **Fabricated local "sandbox" repos.** The old proposal's offline `git init` of
  `sandbox.git` / `sandbox-weft.git` / `sandbox.wiki.git` is **dropped**. This is now
  explicitly GitHub-based against the two real repos.
- **Building the Hub inside or under `C:\Code\loomyard\`.** The Hub must live *outside*
  loomyard so it is never mistaken for part of loomyard.
- **`loom` itself / using the Hub.** This task builds the bench; driving `loom` (or an
  agent) against it is the downstream use, not delivered here.
- **`lyx-test` / `lyx-test-weft` override flags.** These two repos are dedicated to this
  dogfood use only; their URLs are fixed constants in the tool, not configurable.

## Decisions

### deliverable-form

- Decision: A Go tool `tools/sandbox` (`package main`) plus a very thin `sandbox.cmd`
  launcher. The structural pair (Go tool + thin `.cmd`) matches `tools/deploy` +
  `deploy.cmd`, but the content/purpose is its own — deploy is not used as a template.
- Rationale: The repo is Go; a Go tool gives cross-OS behavior, real flag parsing
  (`--parent`, `--reset`), robust directory-wipe and error handling, and a testable core.
  A `.cmd`-only script would be Windows-only and brittle around path and dir-removal logic.
- Rejected: A plain `sandbox.cmd`/`.sh` shell script (Windows-only, brittle). Mirroring
  `tools/deploy`'s internals as a template (explicitly not wanted — only the launcher
  *shape* is shared).

### drive-the-deployed-lyx

- Decision: The tool builds the Hub by invoking the on-PATH `lyx` binary
  (`lyx warp clone …`) as a subprocess — it does **not** reimplement clone logic or call
  `internal/warp` in-process.
- Rationale: The whole point is dogfooding the **real CLI a user runs**. Driving the
  deployed binary exercises the actual command surface, JSON output, and topology wiring.
- Rejected: Importing `internal/warp` and calling `cloneHub` directly (would test the
  library, not the shipped binary, defeating the dogfood goal).

### real-github-repos

- Decision: Host = `https://github.com/Knatte18/lyx-test`, weft =
  `https://github.com/Knatte18/lyx-test-weft`, board = **default-derived** (omit the third
  `warp clone` argument → `https://github.com/Knatte18/lyx-test-weft.wiki.git`). These two
  URLs are hardcoded constants in the tool.
- Rationale: The repos already exist and are dedicated solely to this dogfood use. The
  weft-wiki board derivation is lyx's built-in default (`deriveBoardURL`, `clone.go:259`),
  verified correct in code — no code change needed. The weft wiki was initialized during
  discussion and is now reachable (`master` branch).
- Rejected: A dedicated third board repo or the host's wiki (default derivation is exactly
  what's wanted). Local offline repos (reverses the old proposal — now explicitly
  GitHub-based). Configurable host/weft URLs (YAGNI; repos are dedicated).

### board-precondition

- Decision: Treat the initialized weft wiki (`lyx-test-weft.wiki.git`) as an operational
  precondition, documented — not enforced or created by the tool.
- Rationale: `warp clone` clones the board last and tears down the whole Hub if the board
  is unreachable. A missing/uninitialized GitHub wiki is an operator concern (enable Wikis
  + create one page on `Knatte18/lyx-test-weft`), not a bug and not the tool's job. It is
  currently initialized.
- Rejected: Having the tool create/seed the wiki (out of scope; GitHub wiki init is a
  web-UI/operator action).

### hub-location

- Decision: The Hub is built at `<parent>/lyx-test-HUB/`. `<parent>` comes from a required
  `--parent <dir>` flag (no default baked into the committed Go tool); `sandbox.cmd`
  supplies the machine value, defaulting to `C:\Code` → `C:\Code\lyx-test-HUB\`. The flag
  accepts an absolute or relative path.
- Rationale: The Hub must live **outside** `C:\Code\loomyard\` so it is never read as part
  of loomyard. Keeping the machine-specific path in the thin launcher (not the Go source)
  follows the repo's own lesson (`deploy.cmd` holds `C:\Code\tools\bin`; `tools/deploy`
  stays general). `warp clone` already derives the Hub name from the host basename
  (`lyx-test` → `lyx-test-HUB`, members `lyx-test/`, `lyx-test-weft/`, `_board/`).
- Rejected: Hardcoding `C:\Code` in the Go tool (machine path leaks into committed source).
  Building under `C:\Code\loomyard\` or a system temp dir (explicitly rejected — must be
  visibly outside loomyard, at `C:\Code\lyx-test-HUB`).
- Open for the plan: the base against which a **relative** `--parent` resolves (process
  cwd). Because `sandbox.cmd` `pushd`es to the repo root for `go run`, a relative value
  resolves from the repo root. Recommend the launcher pass an **absolute** `C:\Code` to
  avoid ambiguity, while the flag still *accepts* relative paths.

### reset-semantics

- Decision: If `lyx-test-HUB` already exists and `--reset` is **not** given, the tool is a
  no-op: report that the Hub already exists (and that `--reset` rebuilds it) and exit
  successfully without cloning. With `--reset`, delete the existing Hub directory, then
  re-clone from scratch.
- Rationale: No point re-cloning an existing Hub. This also aligns with `warp clone`'s own
  "hub already exists" guard (`clone.go:110`): wiping first with `--reset` is exactly what
  lets the subsequent `warp clone` succeed.
- Rejected: Always wipe + re-clone on every run (safe given the repos are disposable
  dogfood, but needless network churn and risk of nuking a Hub mid-experiment). Refusing
  with an error when the Hub exists (a clean no-op is friendlier).

### docs-placement

- Decision: A new `docs/dogfood-hub.md` carrying the full record, **and** a short section
  in `docs/overview.md` that points to it.
- Rationale: The convention (repo URLs, board derivation, Hub naming, the
  outside-loomyard location, dogfood purpose, and the "current `lyx` on PATH" precondition)
  must be durable and discoverable. A dedicated page holds the detail; an overview section
  makes it findable from the main entry point.
- Rejected: Only a section in `overview.md` (insufficiently durable/detailed); only the
  proposal (the proposal is per-task and not in `docs/`).

## Technical context

- **`lyx warp clone`** (`internal/warp/clone.go`) is the command the tool drives. Signature:
  `lyx warp clone <host-url> <weft-url> [board-url]`. It derives the Hub name from the host
  basename (`deriveHostName`), creates `<cwd>/<name>-HUB/`, and clones host → `<name>/`,
  weft → `<name>-weft/`, board → `_board/`. With no `board-url`, it derives
  `<weft>.wiki.git` (`deriveBoardURL`, `clone.go:259`). It refuses if the Hub dir already
  exists (`clone.go:110`) and tears the whole Hub down if any clone fails (`teardownHub`).
  **Run it with the subprocess cwd set to `<parent>`** (e.g. `exec.Command(...).Dir = parent`)
  so it creates `<parent>/lyx-test-HUB`.
- **There is no `lyx git-clone` subcommand.** `cmd/lyx/main.go` routes
  `init|board|config|update|ide|muxpoc|weft|warp`. `ly-git-clone` is a *skill* name, not a
  binary command. The tool must call `lyx warp clone`.
- **`tools/deploy/main.go`** is the structural reference *only* for the launcher pair
  pattern (a general Go tool + a thin `.cmd` holding the machine path). Note it locates the
  module root via `runtime.Caller(0)` and uses `git rev-parse --short HEAD` (not
  `--show-toplevel`) — patterns that stay clear of the path-invariant ban (below).
- **Path-invariant constraint (CONSTRAINTS.md / overview.md):** `os.Getwd` and
  `git rev-parse --show-toplevel` are banned outside `internal/paths` and
  `cmd/lyx/main.go`, enforced by `internal/paths/enforcement_test.go` scanning the **entire
  source tree**. `tools/sandbox` must avoid both literals. It does not need them: the parent
  dir comes from `--parent`, and the subprocess cwd is set via `exec.Command.Dir` — no
  `os.Getwd`, no `git rev-parse`.
- **Dogfood repos are reachable (verified during discussion):** host `lyx-test` ✓, weft
  `lyx-test-weft` ✓, board `lyx-test-weft.wiki.git` ✓ (initialized to `master`).
- **Use-model (carry into docs, not code):** the Hub is exercised two ways — (1) manual
  `lyx` commands by the operator, or (2, the real one) Claude/loom driving `lyx` inside the
  Hub. Either way a **deployed `lyx` on PATH is a hard precondition**; without it nothing
  works, and the sandbox tool deliberately does not deploy.

## Constraints

- **Path invariant** (CONSTRAINTS.md §Path Invariant): no `os.Getwd` / `git rev-parse
  --show-toplevel` in `tools/sandbox`. Get the parent from the flag; set subprocess cwd via
  `exec.Command.Dir`.
- **Machine paths stay out of committed Go** (overview.md principle, mirrored from
  `tools/deploy`): the `C:\Code` parent lives in `sandbox.cmd`, not in the Go source.
- **Drive the deployed binary, not the library** — preserves the dogfood property.
- The `lyxtest` leaf invariant and `_lyx`/config-path helper rules in CONSTRAINTS.md do not
  apply: `tools/sandbox` is a `package main` dev tool that touches neither `internal/lyxtest`
  nor config-file paths.

## Testing

`tools/sandbox` is a thin orchestration tool shelling out to `lyx` and `git`; a live clone
depends on GitHub and is unsuitable for CI. Split testable pure logic from the network:

- **TDD candidates (pure, no network):**
  - Hub-path computation: `<parent>/lyx-test-HUB` from a given `--parent` (absolute and
    relative inputs).
  - The exists/reset decision: Hub absent → clone; Hub present + no `--reset` → no-op
    success (no clone invoked); Hub present + `--reset` → remove-then-clone.
- **Command-dispatch seam:** factor the subprocess call behind an injectable runner
  variable (mirror `tools/deploy`'s `var removeAll = os.RemoveAll` seam; add a runner seam).
  A fake runner asserts the tool invokes `lyx warp clone <host> <weft>` (board arg omitted)
  with `Dir == <parent>`, and that `--reset` triggers `RemoveAll(<hub>)` before the clone.
- **No live-network test in CI.** If a real end-to-end check is wanted, gate it behind a
  build tag / manual invocation; it is not part of `go test ./...`.
- **Enforcement guard:** `go test ./internal/paths/...` (the source-tree scan) must stay
  green after adding `tools/sandbox` — confirms no banned `os.Getwd` / `git rev-parse
  --show-toplevel` slipped in.
- Docs changes (`docs/dogfood-hub.md`, `docs/overview.md` section) need no tests beyond
  link/markdown sanity.

## Q&A log

- **Q:** Is the board a third repo, or derived? **A:** Use the default — the weft repo's
  wiki (`lyx-test-weft.wiki.git`). Verified the code already derives exactly this
  (`deriveBoardURL`), so no bug; the wiki was uninitialized and the user initialized it
  during discussion.
- **Q:** Where does the Hub live? **A:** `C:\Code\lyx-test-HUB\` — **absolutely not** under
  `C:\Code\loomyard\`, so it is never seen as part of loomyard.
- **Q:** Does the sandbox tool run deploy (step 0)? **A:** No — deploying `lyx.exe` is fully
  independent. The tool must not touch deploying; it drives whatever `lyx` is on PATH, and
  if `lyx` isn't deployed it simply can't do anything (by design).
- **Q:** Deliverable form? **A:** A small Go tool `tools/sandbox` **with** a very thin
  `sandbox.cmd` launcher — same structural shape as `tools/deploy` + `deploy.cmd`, its own
  content.
- **Q:** How does the tool get the `C:\Code` parent without hardcoding it in Go? **A:** A
  `--parent` flag; `sandbox.cmd` holds the value. It may be a relative path.
- **Q:** Reset behavior? **A:** Don't re-clone if the Hub already exists; `--reset` wipes
  and re-clones.
- **Q:** Is seeding `lyx-test` as a real project part of this task? **A:** No — out of
  scope; assumed already done or a separate task.
- **Q:** Docs location? **A:** A new `docs/dogfood-hub.md` **and** a section in
  `docs/overview.md`.
- **Q:** Are `lyx-test` / `lyx-test-weft` general or dedicated? **A:** Dedicated to this
  dogfood use only — so their URLs are fixed constants in the tool.
```
