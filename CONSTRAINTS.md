# Constraints

Short, authoritative list of the repo's structural invariants. Each is partly
machine-enforced (named test, fails `go test` / CI) and partly a review obligation.
Fuller design/how-to lives in godoc and `docs/`, not here — this file is the index.

## Hub Geometry Invariant

`internal/hubgeometry` owns all cwd, worktree-root, and geometry resolution.

- All cwd / worktree-root queries go through `hubgeometry.Getwd()` / `Resolve()`. Raw
  `os.Getwd` and `git rev-parse --show-toplevel` are banned outside `internal/hubgeometry`
  and `cmd/lyx/main.go`.
- Geometry tokens — `_board`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_raddle`,
  `_lyx` — are owned solely by `internal/hubgeometry`. No other package may use them in a
  path-construction context (a `filepath.Join` arg, a `+` operand, or a string `const`).
  Whole-token match; production files only; comparisons and git-pathspec slice literals
  are not path construction and stay allowed.
- `_lyx`, its `config/` subdir, and any `<module>.yaml` resolve through
  `hubgeometry.LyxDirName` / `ConfigDir(base)` / `ConfigFile(base, module)` — **in test
  code too** (a config-layout migration once broke a hardcoded test fixture).
- Geometry is structural, never config/env-overridable (the board dir is `--board-path`
  flag > `hubgeometry.BoardDir(l.Hub)`, not a config key).
- **Enforced by** `internal/hubgeometry/enforcement_test.go` (`TestEnforcement_GeometryLiterals`)
  on every `go test`. API and helpers: godoc for `internal/hubgeometry`.

## lyxtest Leaf Invariant

`internal/lyxtest` stays a leaf: it imports only the standard library and
`internal/hubgeometry` — never `internal/configreg` or any feature package
(`boardengine`/`boardcli`, `warpengine`/`warpcli`, `weftengine`/`weftcli`, …).

- A `lyxtest → configreg → feature` edge closes a test-build cycle under
  `-tags integration`. Tests needing real config call `lyxtest.SeedConfig(tb, dir,
  map[string]string{...})`; the `configreg`→map conversion happens at the test site, in a
  package that may legally import `configreg`.
- **Enforced by** `internal/lyxtest/leaf_enforcement_test.go` on every `go test`.

## Modelspec Leaf Invariant

`internal/modelspec` production code imports only stdlib, `internal/hubgeometry`, and
`gopkg.in/yaml.v3` — so every future consumer (builder, perch/burler/loom configs) can
import it without cycles.

- `configreg` → `modelspec` is the allowed direction (for `modelspec.ConfigTemplate`);
  the reverse import (`modelspec` → `configreg` or any feature package) is never allowed.
- **Enforced by** `internal/modelspec/leaf_enforcement_test.go`
  (`TestLeafInvariant_AllowlistOnly`) on every `go test`.

## CLI / Cobra Invariant

Every lyx CLI module is a cobra subtree assembled under one root in `cmd/lyx/main.go`.

- **Seam.** Each module exposes `Command() *cobra.Command` and a thin
  `RunCLI(out io.Writer, args []string) int` = `clihelp.Execute(Command(), out, args)`.
  Tests and root both drive the module through this seam.
- **Registration.** A new module is wired into `newRoot()`: import, `root.AddCommand(...)`,
  and append the module name to the root `Long` module-list. Unregistered ⇒ invisible to
  `--help`.
- **`Short` on every command** (parent + sub), non-empty. Self-discoverable commands also
  carry a `Long` with concrete examples.
- **Help accuracy is a review obligation.** Presence of `Short` is machine-checked;
  prose-vs-behaviour is not. When a change alters observable behaviour, the reviewer must
  re-read every affected `Short`/`Long` and confirm it matches the code as changed — stale
  help is a review-blocking defect. Prefer generating mechanical help facts from source
  (e.g. configcli's `Known modules:` from `configreg.Names()`).
- **Errors are JSON.** Results and errors go through the `internal/output` envelope
  (`output.Ok` / `output.Err`), one JSON object per line, via the `clihelp.Execute` /
  root seam (`SilenceErrors = true`). No bare plain-text error paths. Parent groups set
  `RunE = clihelp.GroupRunE` to reject unknown subcommands.
- **Interactive-handoff exception (narrow, per-command).** A subcommand whose whole job is
  to hand the operator's stdio to another interactive program and block (`ide menu`'s stdin
  picker; `mux attach`'s `tmux attach`) cannot emit the JSON envelope on that terminal-handover
  tail. The exception is scoped tightly: everything that can fail runs **pre-flight and stays
  on the envelope** (`output.Err`, non-zero exit); only the post-handoff tail is exempt, and on
  success it emits no JSON. `mux attach` follows the pre-existing `ide menu` precedent; see the
  `internal/muxcli` attach command's godoc/`Long` and
  [docs/overview.md#modules](docs/overview.md#modules) for the full rationale.
- **Package naming.** A Cobra-registered package is `<module>cli`; its extracted domain
  kernel is `<module>engine`. cli imports engine; engine never imports cli or cobra.
  Litmus: returns `(T, error)` with no cobra/`io.Writer`/exit codes ⇒ engine. Skip the
  engine only for trivial wrappers (`configcli`) or a throwaway proof-of-concept meant
  to be deleted once it proves its point (e.g. `muxpoccli`, which followed exactly that
  path — deleted once `mux` shipped); "no consumer today" is not a skip reason.
  `initcli`/`initengine` follows the standard
  split (no longer exempt — `lyx init --undo` grew enough core logic that mixing it into
  the cli package was rot, not simplicity).
- **Enforced by** `cmd/lyx/drift_test.go` (every command has `Short`),
  `helptree_test.go` (root names every module, module names every subcommand),
  `registration_test.go` (exists ⇒ registered), `longlist_test.go` (registered ⇒ in
  `root.Long`). Update the pinned sets in the same commit when adding a module/subcommand.

## Shuttle Provider-Seam Invariant

Provider specifics live ONLY under `internal/shuttleengine/claudeengine`.

- CLI flags, the `settings.json` hook schema, TUI startup/trust markers, and pane key
  choreography are all Claude-specific and stay inside `internal/shuttleengine/claudeengine`.
  `internal/shuttleengine` and `internal/muxengine` stay provider-invariant: they define the
  `Engine` interface (and, for mux, the opaque `cmd`/`resumeCmd`/strand contract) and never import
  or reference Claude specifics.
- `internal/shuttleengine` never imports `internal/shuttleengine/claudeengine` — the reverse
  import only. Wiring a concrete engine into the run loop happens in `internal/shuttlecli`, which
  imports both.
- **Enforced by** `internal/shuttleengine/seam_enforcement_test.go` (`TestProviderSeamImportRule`)
  on every `go test`, for the import-graph half of the rule. The semantic half — no Claude marker
  strings, hook payload shapes, or TUI grammar leaking outside `claudeengine` — is a review
  obligation, not machine-checked.

## Shell Mechanics Seam

Pane-shell command strings — argument quoting, the call operator, and the
prompt-file read idiom — are built ONLY via `internal/shell`.

- `internal/shell` defines the provider-invariant `Shell` interface (`Quote`/`Invoke`/
  `ReadFile`) with a `pwsh` and a `posix` implementation, selected at runtime by
  `shell.ForGOOS()` (or exercised directly via `shell.Pwsh()`/`shell.Posix()`).
  `internal/shell` stays stdlib-only and provider-invariant: no Claude flags, marker
  strings, or hook shapes may appear in it, complementing the Shuttle Provider-Seam
  Invariant above (which keeps those Claude specifics confined to `claudeengine`).
- `internal/shuttleengine/claudeengine` (and any future provider engine) never emits raw
  pwsh/posix shell syntax directly — it composes its launch/resume command lines only by
  calling into `internal/shell`.
- **Enforced by** review obligation today: a grep-guard test — e.g. asserting the
  `Get-Content -Raw` idiom appears only under `internal/shell` — is a cheap future
  machine-check, deferred per YAGNI.

## Weft Git Invariant

Every git operation on the weft repo goes through the weft/warp engines in Go, driven by the
orchestration layer in-process — never raw git, and never an LLM agent.

- **Module ownership.** Weft-internal git (`commit`/`push`/`pull`/`sync`) goes through
  `internal/weftengine`; coordinated host↔weft topology (a checkout that moves both and re-points
  junctions, dual-worktree add/remove/clone) goes through `internal/warpengine`. No other package
  runs raw git against a weft worktree. The **host** repo is unrestricted — it is an ordinary project
  repo.
- **Orchestration, not agent.** The weft commit is Go calling the engine in-process
  (`weftengine.Sync`/`Commit`) at a round/phase boundary the loop owner (loom, or perch's CLI
  standalone) controls. An LLM agent never drives weft git — not raw git, not by shelling `lyx weft`.
  Agents ride the file contract: they **write** overlay files (reviews, fixer-reports, status, raddle
  docs) into `_lyx`/`_raddle` via the junction; Go **reads and commits** them. Asymmetry: an agent
  **does** commit its own code to the **host** repo (commit-per-fix, normal dev git) — the weft, never.
- **Why.** A weft commit is an orchestration act (persist round/phase state at the right boundary,
  coordinate host↔weft) — the deterministic Go responsibility that is the whole lyx thesis. An
  agent-run weft commit reintroduces the non-deterministic, untestable, mis-ordered LLM orchestration
  lyx exists to remove.
- **Enforced by** review obligation: agent prompt templates never instruct a weft git op, and weft
  git stays inside `weftengine`/`warpengine`. The module-ownership half is a candidate for a future
  import/grep guard; not machine-checked today.

## Review Round Invariant

One review+fix round (burler now, hardener later) follows the round discipline: A-before-B
(the review is fully written to disk before any target file is touched); every recorded
finding is fixed in B, all severities including LOW/NIT; no self-grading (round N's fix is
judged by round N+1's fresh review, never its own); commit-per-fix on host source, never push.

- **Enforced by** `internal/burlerengine/template_test.go` (`TestTemplate_StatesRoundDiscipline`)
  for the template's sequencing and fix-everything statements. The rest — no self-grading,
  commit-per-fix discipline — is a review obligation on prompt templates, not machine-checked.

## Sandbox Suite Coverage

Every registered lyx module must be exercised by the black-box sandbox suite or be
explicitly excluded with a reason.

- **Tagging.** A scenario in **any** suite file matching
  `tools/sandbox/*SUITE.md` (today: `SANDBOX-CORE-SUITE.md`,
  `SANDBOX-MUX-SUITE.md`) that drives a specific module declares it with a
  `**Covers:** <module>[, <module>...]` line, in the same bold-label style as the
  scenario's `**Goal:**`/`**Watch:**`/`**Verdict:**` lines. The guard unions tags
  across all matched files. Coverage is checked at module granularity against the
  live cobra root (`newRoot().Commands()`, skipping `help`/`completion`) — the same
  enumeration `longlist_test.go` already uses, never a separately hand-maintained
  list. The guard fails fast if the glob matches fewer than two files (vacuous-glob
  protection).
- **Allowlist.** Modules that are intentionally never sandbox-exercised across any
  suite file are named on the test's `excludedModules` allowlist with a one-line
  reason: `ide` (side-effect heavy: `spawn` opens a real VS Code window, `menu` is
  an interactive stdin picker), `selfreport` (`create` files a real GitHub issue).
- **Exists ⇒ covered or excluded.** Adding a new registered module requires either
  a scenario in some suite file tagged with that module's `**Covers:**` or a new
  allowlist entry with a reason — the same "exists ⇒ registered" discipline as the
  CLI/Cobra Invariant's registration guard.
- **Enforced by** `cmd/lyx/sandbox_coverage_test.go`
  (`TestSandboxCoverage_AllModulesCoveredOrExcluded`) on every `go test`.

## Test Tier Purity Invariant

Untagged test files perform no expensive spawns — no `git init` / `git worktree add` /
fixture-tree copies; Tier 1 stays offline and fast.

- **Statement.** A test file whose first non-empty line is not a `//go:build`
  constraint mentioning `integration` or `smoke` is "untagged" and runs on every plain
  `go test`. Untagged files must not spawn: no `gitexec.RunGit`, no
  `exec.Command`/`exec.CommandContext`, no `lyxtest.Copy*` fixture-tree copy. A
  platform-only constraint (e.g. `//go:build windows`) still counts as untagged — it
  still runs in Tier 1 on that platform, so its spawns still count. This is deliberately
  narrower than "spawn no processes": an untagged test that reaches
  `hubgeometry.Resolve` on an error path still spawns one cheap failing `git rev-parse`,
  which the guard does not ban.
- **Mechanics.** The guard walks every `*_test.go` file under the module root (resolved
  via `go env GOMOD`, cwd-independent) and checks each untagged file's source for a
  banned token as a **raw substring** — `gitexec.RunGit`, `exec.Command` (which also
  matches `exec.CommandContext`), or `lyxtest.Copy` (prefix-matches `CopyPaired`,
  `CopyPairedLocal`, `CopyHostHub`, `CopyWeft`, and any future `Copy*` fixture). Raw
  substring matching is deliberate: a comment or string-literal mention in an untagged
  file trips the guard too (rename the mention or tag the file).
- **Allowlist.** Exists ⇒ tagged or allowlisted with a reason. A file or directory-path
  prefix that must legitimately spawn in an untagged file is named on the guard's
  `allowedSpawners` map with a one-line reason: `internal/proc` (process control is the
  package's subject — its tests must spawn) and `cmd/lyx/tierpurity_test.go` itself
  (contains the banned token strings as its own test data).
- **Enforced by** `cmd/lyx/tierpurity_test.go`
  (`TestTierPurity_UntaggedTestsSpawnNothing`) on every `go test`.

## Hermetic Git Test Environment Invariant

Every test package whose tests spawn git — directly or via the lyxtest fixture
helpers — runs under the hermetic git test environment, so no test behaviour
depends on the operator's `~/.gitconfig` or the system gitconfig.

- **Statement.** A package is "git-spawning" when any of its `*_test.go` files
  spawns git directly (`gitexec.RunGit`, `exec.Command`/`exec.CommandContext`) or
  indirectly through a lyxtest fixture helper (`lyxtest.Copy*`, `lyxtest.MustRun`,
  `lyxtest.SeedConfig`). Every such package must contain a `TestMain` that calls
  `lyxtest.HermeticGitEnv()` before `m.Run()`, or be named on the allowlist below
  with a reason. The concrete failure this kills: a global `core.fsmonitor=true`
  in the operator's gitconfig spawning hundreds of `fsmonitor--daemon` processes
  per integration run — see `docs/benchmarks/fixture-copy.md` for measured
  numbers.
- **Mechanics.** The guard walks every `*_test.go` file under the module root
  (resolved via `go env GOMOD`, cwd-independent) and checks each file's source
  for the bare, unqualified `HermeticGitEnv` substring — matching both the
  qualified `lyxtest.HermeticGitEnv()` call form (other packages) and the
  unqualified `HermeticGitEnv()` form `internal/lyxtest`'s own tests use. Unlike
  the Test Tier Purity Invariant's guard, this one scans **every** test file
  regardless of build constraint: the git-spawning set is almost exactly the
  integration-tagged set, so skipping tagged files would make the guard
  vacuous. This proves presence only — the mechanical half of the check. The
  semantic half (a real `TestMain` that calls the helper before `m.Run()`) is a
  review obligation, exactly like the repo's other grep-guards (the Shell
  Mechanics Seam and Provider-Seam entries above).
- **Allowlist.** Exists ⇒ hermetic or allowlisted with a reason. A package
  directory-path prefix that spawns non-git processes for which a git-hermetic
  `TestMain` would be meaningless is named on the guard's `allowedNonHermetic`
  map with a one-line reason: `internal/proc` (process control is the
  package's subject — its tests must spawn, just not git). The guard's own
  file, `cmd/lyx/hermeticenv_test.go`, carries the tokens (including the bare
  `HermeticGitEnv` presence token) as its own test data; it is a **per-file
  scan exclusion**, not a package-level exemption — `cmd/lyx` itself
  genuinely spawns git in its e2e tests and satisfies the requirement through
  its own real `TestMain`.
- **Enforced by** `cmd/lyx/hermeticenv_test.go`
  (`TestHermeticGitEnv_GitSpawningPackagesHaveTestMain`) on every `go test`.

## Documentation Lifecycle

Which docs are kept vs deleted (mechanical per-module docs vs durable design docs):
see [docs/overview.md#documentation-lifecycle](docs/overview.md#documentation-lifecycle).
