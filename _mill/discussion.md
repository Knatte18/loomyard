# Discussion: Expand the sandbox suite: subfolder init, weft, warp, config reconcile + coverage invariant

```yaml
task: Expand the sandbox suite: subfolder init, weft, warp, config reconcile + coverage invariant
slug: sandbox-suite-expand
status: discussing
parent: main
```

## Problem

`tools/sandbox/SANDBOX-SUITE.md` is the black-box scenario script an agent follows
when driving a real `lyx.exe` against the GitHub sandbox repos. It currently only
covers S0–S4 plus S6 (S5 was deleted at some point, leaving a numbering gap) —
`init`, `board`, and `config` get scenarios, but `weft` (a core module) is only
grazed in passing by S2, and `warp` (host↔weft pairing) has **no scenario at
all** — the existing S6 (renamed S5) is a generic, module-agnostic
wrong-directory/bad-flag/unknown-subcommand ergonomics check with no
warp-specific content. There is also no
mechanism that stops a brand-new lyx module from silently never getting a sandbox
scenario — nothing parallels the "exists ⇒ registered" guard the CLI/Cobra
Invariant already enforces for `newRoot()` registration.

This task grows the suite to match lyx's current command surface (subfolder
init, weft lifecycle, warp introspection, config reconcile), closes the S5
numbering gap, and adds a machine-enforced "Sandbox Suite Coverage" invariant
so that guarantee holds going forward.

## Scope

**In:**

- New scenario **S6 — Subfolder init**: from a non-root subdirectory of the
  host repo, run `lyx init` there, then exercise `config` from that subdir to
  confirm lyx resolves against the subdir's own `_lyx`; also run `board` from
  the subdir as a "still works from any subfolder" smoke check (board's data
  lives at the hub level, so it does not itself demonstrate subfolder-scoped
  resolution — see Decisions for why). Finally, run `lyx init --undo` from
  the subdir to reverse the scaffolding — this is both S6's cleanup step and
  a real exercise of the newly-merged `init --undo` reversal path (see
  Technical context).
- Rewrite the "Operating model" note in `SANDBOX-SUITE.md` (currently forbids
  nested `_lyx` scaffolding during a session) to carve out this one scenario
  as the explicit, controlled exception.
- New scenario **S7 — Weft lifecycle**: `weft status/commit/push/pull/sync`
  end to end — make a change, commit via weft, verify status/mirroring.
- New scenario **S8 — Warp introspection**: valid `warp list/pairs/checkout/
  reconcile`. `warp` has no existing scenario today — S8 is the module's
  first (the renamed S5 error-ergonomics check is generic and mentions no
  warp-specific case).
- Extend existing **S4 — Config round-trip**: the round-trip now uses the
  non-interactive `lyx config <module> --set key=value` write path (merged
  into `main` after this discussion began — see Technical context) followed
  by `lyx config <module> --print` to read the value back, then finally
  `lyx config reconcile`. This makes the whole S4 round-trip sandbox-native
  (no editor), which the original discussion could not assume.
- Renumber: rename the current S6 (wrong-directory/error-ergonomics) to
  **S5**; new scenarios from this task take S6, S7, S8. Fix the session-log
  format list (`S0`...`S5`... `S8`) to match. This renumber is forward-only —
  past run reports and the frozen `sandbox-cli-ergonomics` task keep their
  historical "S6" references untouched.
- New machine-enforced **Sandbox Suite Coverage** invariant: a Go test,
  parallel to `cmd/lyx/registration_test.go`, that fails the build if a
  registered lyx module has neither a scenario nor an explicit exclusion.
  Record this invariant in `CONSTRAINTS.md` in the same commit (Documentation
  Lifecycle rule).

**Out:**

- `muxpoc`, `ide`, and `selfreport` get **no new scenarios** — see Decisions
  for why all three are whole-module exclusions in the coverage allowlist,
  not partial/subcommand exclusions.
- The `config` module's interactive editor flow (`lyx config <module>` with
  neither `--print` nor `--set`, which launches a real editor process) and
  the interactive numbered menu (`lyx config` with no module) stay untested —
  `--print`, `--set`, and `reconcile` are the sandbox-exercised paths. This
  does **not** need its own allowlist entry: coverage is module-level, and
  `config` is already covered via S4.
- `warp add`/`clone`/`remove`/`prune`/`cleanup` are not exercised by S8 —
  they mutate/destroy worktree pairs and are out of scope for an
  introspection scenario. Only `list`, `pairs`, `checkout`, `reconcile` are
  in scope for S8.
- Subcommand-level coverage granularity is out of scope for this task (see
  Decisions — module-level only, for now).
- **Implementing the `init --undo` reversal command is not part of this
  task** — it was surfaced by this discussion, filed as the `lyx-deinit`
  backlog task, and has since been **implemented and merged into `main`**
  (as `lyx init --undo`, a flag on `init`, not a standalone `deinit` module).
  This task now simply *uses* that command as S6's cleanup step (see
  Decisions → Subfolder init scenario); the earlier plan for a temporary
  ad-hoc filesystem/git cleanup workaround is obsolete and has been removed.

## Decisions

### Coverage mechanism: explicit `**Covers:**` tags, not prose matching

- Decision: Each scenario in `SANDBOX-SUITE.md` that drives a specific module
  gets a `**Covers:** <module>[, <module>...]` line, in the same bold-label
  style as the existing `**Goal:**`/`**Watch:**`/`**Verdict:**` lines. The new
  Go test parses only these lines — it never infers coverage from scenario
  prose.
  - `S6` (subfolder init) → `**Covers:** init`
  - `S3` (board) → `**Covers:** board`
  - `S4` (config, extended with reconcile) → `**Covers:** config`
  - `S7` (weft lifecycle) → `**Covers:** weft`
  - `S8` (warp introspection) → `**Covers:** warp`
  - `S0` (discovery) and `S1` (hub orientation) drive no single module —
    they carry **no `Covers:` line at all**. (An earlier draft floated an
    explicit `**Covers:** (discovery)` sentinel as an alternative; that is
    rejected because a literal `(discovery)` token would then appear in
    `covered` and trip Assert 2's drift guard, which requires every `covered`
    token to be a registered module. Mandating "no line" removes that
    tension without needing a parenthesized-token-stripping parser branch.)
  - `S5` (error ergonomics, renamed from S6) also drives no single module —
    it's a cross-cutting ergonomics check, not a module scenario.
- Rationale: module names ("config", "ide", "warp"...) already appear
  incidentally in scenario prose without that scenario actually exercising
  the module — substring-grepping prose would produce both false positives
  and false negatives. An explicit tag is the same "structured fact, not
  prose" discipline the Cobra guards already use (`Short` presence is
  machine-checked; `Short`/`Long` *accuracy* is a review obligation — same
  split applies here: tag *presence* is machine-checked, scenario *content*
  accuracy stays a review obligation).
- Rejected: doc-parsing via fuzzy prose search (see above); a Go-only
  hardcoded module→scenario map with no doc-side trace (loses the audit
  trail of *why* a module is covered when reading the doc alone).

### Coverage granularity: module-level only

- Decision: The invariant checks whole modules, not individual subcommands.
  A module counts as covered if **any** scenario tags it with `Covers:`,
  regardless of which subcommand(s) that scenario actually exercises.
- Rationale: matches CONSTRAINTS.md's literal phrasing for the parallel
  guard ("every registered lyx module has a scenario..."), and reuses
  `longlist_test.go`'s existing module-enumeration pattern unchanged. Concretely:
  `config` has both an excluded editor flow and a covered `reconcile`
  subcommand — under module-level coverage this is fine and expected: one
  covered subcommand is enough for the whole module to pass.
- Rejected: subcommand-level granularity (would catch e.g. "`ide spawn`
  tested but `ide menu` isn't") — meaningfully more implementation work
  (walking each module's full cobra subtree, per-subcommand allowlist) for a
  suite that's deliberately small. Explicitly deferred as a future
  extension, not designed here.

### Coverage allowlist: whole-module exclusions for `muxpoc`, `ide`, `selfreport`

- Decision: The Go test's allowlist excludes exactly three modules, each with
  a one-line reason:
  - `muxpoc` — PoC, slated for replacement by the mux module.
  - `ide` — side-effect heavy: `spawn` opens a real VS Code window, `menu` is
    an interactive stdin picker over worktrees. Neither subcommand is
    practical to drive from a scripted/LLM sandbox session beyond the S0
    discovery smoke test.
  - `selfreport` — `create` files a real GitHub issue; not something to
    exercise repeatedly in a sandbox run.
- Rationale: read the actual command implementations
  (`internal/idecli/cli.go`, `internal/selfreportcli/cli.go`,
  `internal/muxpoccli/cli.go`) — every subcommand under all three modules is
  side-effect-heavy or throwaway end-to-end, not just the one subcommand each
  named in the task brief ("ide spawn", "selfreport create"). Since coverage
  is module-level (previous decision), a module needs either full coverage
  or full exclusion — there's no partial-credit path — so excluding the
  whole module is the only option that matches how these modules actually
  behave.
- Rejected: excluding only the exact subcommands named in the task brief
  (`ide spawn`, `selfreport create`) and leaving `ide menu` / all of `muxpoc`
  uncovered — under module-level granularity this would just fail the new
  test immediately, since `ide` and `muxpoc` would be neither covered nor
  excluded.

### Coverage test: location and mechanics

- Decision: New test file `cmd/lyx/sandbox_coverage_test.go`, in `package
  main` alongside `registration_test.go`, `longlist_test.go`, and
  `helptree_test.go` — the same package these already live in, because it
  needs the exact same `newRoot()` access they use.
  - `registered`: build `root := newRoot()`, iterate `root.Commands()`, skip
    `"help"` and `"completion"` — the identical pattern `longlist_test.go`
    already uses (`cmd/lyx/longlist_test.go`), not a separately hand-maintained
    list.
  - `covered`: every module token parsed from `**Covers:**` lines in
    `tools/sandbox/SANDBOX-SUITE.md`. Resolve the doc's absolute path the same
    way `registration_test.go` resolves the repo root — via
    `runtime.Caller(0)` + `filepath.Dir` walk-up, since this test file lives
    two directories deep from repo root just like that one. Because S0/S1
    carry no `Covers:` line (see Coverage mechanism), every token that *does*
    appear is expected to be a bare registered-module name — so Assert 2 can
    treat all of `covered` as module tokens with no special-casing.
  - `excluded`: the hardcoded allowlist map from the previous decision
    (`muxpoc`, `ide`, `selfreport`), each with its reason string.
  - Assert 1 (coverage): for every `m` in `registered`, `m` must be in
    `covered` or `excluded`. On failure, list every uncovered, non-excluded
    module by name (not just fail generically) — matches
    `registration_test.go`'s style of naming the exact offending package.
  - Assert 2 (drift guard): every token appearing in `covered` or as a key in
    `excluded` must actually be a name in `registered`. Catches typos in
    `Covers:` lines and stale allowlist/tag entries left behind when a module
    is renamed or removed.
  - Consider a sanity sub-test mirroring `registration_test.go`'s
    `discovered_non_empty` (e.g. `registered` and `covered` are both
    non-empty) so a silently-broken parse doesn't produce a vacuous pass —
    implementer's call on exact shape.
- Rationale: `cmd/lyx/` is where every other guard needing live command-tree
  introspection already lives; reusing `newRoot()` directly is simpler and
  more accurate than re-deriving the module list via AST parsing (which is
  what `registration_test.go` does, but only because *that* test's job is to
  compare two independent sources — main.go's `AddCommand` calls vs. discovered
  `Command()` funcs — and can't call `newRoot()` for one side without making
  the test trivially circular).
- Rejected: `tools/sandbox/coverage_test.go` — different package, can't call
  `newRoot()` without either re-implementing AST-based module enumeration
  (duplicate logic already living in `registration_test.go`) or importing
  `cmd/lyx` as a library, which isn't how Go `main` packages are consumed.

### Scenario numbering

- Decision: after renaming the current S6 (wrong-directory/error-ergonomics)
  to S5, the three brand-new scenarios take S6/S7/S8 in this order:
  S6 = subfolder init, S7 = weft lifecycle, S8 = warp introspection. Config
  reconcile is **not** a new scenario — it's appended to existing S4 in
  place, under S4's own `Covers: config` tag.
- Rationale: matches the order the task body's Scope section already lists
  them in (1. subfolder init, 2. weft lifecycle, 3. warp introspection,
  4. config reconcile as an S4 extension, 5. renumber).
- Rejected: reordering by conceptual proximity (e.g. slotting subfolder-init
  right after S1 orientation) — the task explicitly specifies the renumber
  is forward-only (S6→S5, new scenarios take S6, S7, ...), so mid-sequence
  insertion was never on the table.
- Correction from discussion review round 1: an earlier draft of this
  document claimed the renamed S5 already tested a "bad-checkout" case for
  warp and framed S8 as going "beyond" it. The actual S5/S6 Watch text is
  generic (wrong-directory / bad-flag / unknown-subcommand) with no
  warp-specific content anywhere — `warp` has zero scenario coverage before
  this task, and S8 is its first scenario, full stop.

### Weft lifecycle scenario (S7): durability note required

- Decision: S7 gets the same style of durability/cleanup note S3 (board)
  already carries: make a small, clearly-marked test change, run it through
  `commit`/`push`/`pull`/`sync`, and don't leave the weft/host remote in a
  diverged or broken state for the next session.
- Rationale: S7 runs against the real shared GitHub sandbox repos
  (`Knatte18/lyx-test` / `Knatte18/lyx-test-weft`), same durability concern
  S3 already documents for the board repo.
- Technical detail for the implementer: `lyx weft commit`'s message is
  **always** the fixed string `"weft sync"` (see
  `internal/weftcli/cli.go`'s `commitCmd` Long text) — not customizable, so
  the scenario shouldn't hunt for a `-m` flag. Staging is scoped to the
  weft config's configured dirs (default `_lyx`), so the "small test change"
  should land inside that scope (e.g. a config file change) to actually be
  picked up by `commit`/`push`/`sync`. `lyx weft sync` pushes via a detached
  child process — status may not immediately reflect an in-flight sync; if
  that's confusing, that's exactly the kind of rough edge S7 exists to
  surface as a WARN, not something to pre-judge here.

### Warp introspection scenario (S8): checkout-and-restore

- Decision: S8 records the branch active before the scenario starts, runs
  `warp checkout <other-branch>` to prove the coordinated switch works, then
  `warp checkout <original-branch>` to restore it — same cleanup discipline
  as S3/S7.
- Rationale: leaves hub state clean for the rest of the session/future runs,
  consistent with the durability notes already established for S3 and S7.
- Scope for S8: `list`, `pairs`, `reconcile`, `checkout` only — `add`,
  `clone`, `remove`, `prune`, `cleanup` are excluded (see Scope/Out) because
  they mutate or destroy worktree pairs rather than introspect them.
- Technical note for the implementer: `warp reconcile` (in
  `internal/warpcli/warp.go`, `runReconcile`) has **no** `--apply`/dry-run
  flag, unlike `config reconcile` — it always performs its repair check
  directly. On an already-healthy pair this should be a no-op/idempotent
  read+report; confirm this in practice while writing the scenario, since a
  destructive no-dry-run `reconcile` on a healthy pair would itself be a
  sandbox finding worth recording.

### Subfolder init scenario (S6): "any subfolder" contract

- Decision: S6 verifies that `lyx init`, run from a subdirectory of the
  already-initialized host repo, creates `_lyx/` scoped to that subdirectory
  (not at the repo root), and that a subsequent `config` command run from
  that same subdir resolves against the subdir's own `_lyx/config` rather
  than the root's. `board` is also run from the subdir, but only as a
  "still works, doesn't error" smoke check — see the correction below on
  why board does not demonstrate subfolder-scoped resolution the way config
  does.
- Rationale: this is `hubgeometry.Resolve(cwd)`'s existing contract —
  `WorktreeRoot` comes from `git rev-parse --show-toplevel` (repo root,
  unaffected by cwd depth) while `RelPath` is `filepath.Rel(WorktreeRoot,
  cwd)`; `lyx init`'s weft-pairing check (`l.WeftWorktree()`) depends on
  `Hub`/slug derived from `WorktreeRoot`, so it succeeds from a subfolder of
  an already-paired host exactly as it would from the root. This scenario is
  meant to prove that contract holds in practice, not just in code.
- The "Operating model" paragraph in `SANDBOX-SUITE.md` currently states the
  agent "must not scaffold nested `_lyx/` during a session" — that rule
  stands everywhere except this one controlled scenario. Rewrite the note to
  name S6 as the explicit, sole exception rather than deleting or weakening
  the rule generally.
- Open item for the implementer to verify while writing/running S6: init's
  `.gitignore` handling calls `gitignore.Ensure(cwd, ".lyx/")` (note the
  literal string is `.lyx/`, not `_lyx/` — verify whether this is a
  pre-existing naming quirk unrelated to this task or something the
  scenario should flag) — if it looks like a real bug, record it as a
  scenario finding rather than silently working around it; it is not a
  reason to change the invariant test design above.
- Durability note required (added in discussion review round 1, same style
  as S3/S7/S8): S6 scaffolds a real nested `_lyx/` in the subfolder and
  touches `.gitignore` there — state persisting across sandbox sessions
  unless the hub is rebuilt with `sandbox-build.cmd -reset` (which the
  suite's own Pre-conditions describe as optional, not mandatory, before
  each session). S6 must therefore reverse that scaffolding at session end
  via `lyx init --undo` (see the cleanup bullet below), so a later run
  reliably observes "not yet initialized" rather than silently reusing a
  prior run's leftovers.
- Correction from discussion review round 2: the nested `_lyx/` the
  subfolder init creates is **not a plain directory — it's a real directory
  junction**, wired by `warpengine.WireJunctions` before init's own
  `os.Stat`/`MkdirAll` step even runs. `HostLyxLink(slug)` and
  `WeftLyxDirFor(slug)` (`internal/hubgeometry/hubgeometry.go`) are both
  keyed on `RelPath`, so the subfolder junction is
  `<host>/<subdir>/_lyx` → `<weft-worktree>/<subdir>/_lyx`, and
  `ReconcileAll(cwd, true)` writes the module config YAMLs *through* that
  junction into the weft worktree. This is exactly why a naive "delete the
  nested `_lyx/`" cleanup is insufficient — it would leave the real
  weft-side `<subdir>/_lyx/config/*.yaml` (and any weft commit made against
  it) behind. The merged `lyx init --undo` command handles this correctly:
  it unwires the host junction, clears the weft-side `_lyx` content and
  commits+pushes that deletion, and reverts both the `.gitignore` block and
  the `.git/info/exclude` entry (`internal/initengine/undo.go`). That is
  precisely the "remove **both** the host junction and the weft-side target"
  requirement this correction identified — which is why S6's cleanup is now
  a single `lyx init --undo` call rather than manual multi-step housekeeping.
- **S6 cleanup = `lyx init --undo`** (revised after merging `main`). An
  earlier draft of this discussion recorded that no `lyx` command could
  reverse `init` and therefore planned a temporary ad-hoc filesystem/git
  workaround. That command now exists — `lyx init --undo` was implemented and
  merged (the `lyx-deinit` backlog task), so the ad-hoc plan is dropped
  entirely. S6's cleanup is a single `lyx init --undo` run from the same
  subdirectory. Key properties the implementer should rely on
  (`internal/initcli/initcli.go`, `internal/initengine/undo.go`):
  - It is a **clean no-op on a never-initialized directory** (no
    weft-pairing pre-gate, unlike plain `init`), so it is always safe to run
    at session end even if S6 bailed early.
  - It is **not purely local**: clearing the weft-side `_lyx` content
    commits and pushes that deletion to the real shared `lyx-test-weft`
    remote. This is correct (it restores pristine state) but means each S6
    run leaves an init-then-undo commit pair in the weft repo's history —
    call this out in the S6 note the same way S7's weft-lifecycle note flags
    that it writes to the shared remote.
  - Running `lyx init --undo` also **exercises a real reversal path** end to
    end, so S6's cleanup doubles as coverage of the `init --undo` surface —
    no separate scenario is needed for it, and it does not change the
    `Covers: init` mapping (still module-level).
  - The JSON envelope reports per-step outcomes (`lyx_junction`,
    `weft_content`, `git_exclude`, `gitignore` each `removed`/`cleared`/
    `reverted`/`not_present`/`unchanged`); a legible OK envelope here is the
    expected outcome, not a finding.
- Correction from discussion review round 2 (accuracy note): `board`'s data
  directory is `hubgeometry.BoardDir(layout.Hub)` (`internal/boardcli/cli.go`)
  — hub-level and cwd-depth-invariant, unlike `config`'s subdir-scoped
  `_lyx/config`. Running `lyx board list` from the S6 subdir returns the
  *same* hub board as from the root; it does not prove subfolder-scoped
  resolution. `config` is the scenario's actual subfolder-scoping
  demonstrator; `board`-from-subdir only shows the command still runs
  without error from a non-root cwd.

## Technical context

- **Module registry lives in `cmd/lyx/main.go`**: `newRoot()` registers
  `initcli`, `boardcli`, `configcli`, `idecli`, `muxpoccli`, `weftcli`,
  `warpcli`, `selfreportcli`. Cobra command names (via `.Name()`) are the
  lowercase `Use` first tokens: `init`, `board`, `config`, `ide`, `muxpoc`,
  `weft`, `warp`, `selfreport` — these are the tokens `Covers:` tags and the
  allowlist must use, not the Go package names.
- **Existing guard-test siblings** (`cmd/lyx/registration_test.go`,
  `longlist_test.go`, `helptree_test.go`, `drift_test.go`) are the direct
  precedent for the new coverage test's style: table-driven where possible,
  explicit per-offender error messages, and (per `registration_test.go`) a
  sanity sub-test guarding against a silently-vacuous pass.
- **`internal/weftcli/cli.go`**: `weft` subcommands are `status`, `commit`,
  `push`, `pull`, `sync`. No JSON payload args (unlike `board`/`config`) —
  scenario prompts don't need the PowerShell JSON-quoting note S3 carries.
- **`internal/warpcli/warp.go`**: `warp` subcommands are `clone`, `add`,
  `list`, `remove`, `checkout`, `pairs`, `reconcile`, `prune`, `cleanup`.
  Only `list`/`pairs`/`checkout`/`reconcile` are in scope for S8.
- **`internal/configcli/configcli.go`** (updated on `main` after this
  discussion began): `config [module]` (edit/menu — launches a real editor
  or interactive stdin menu, excluded from sandbox testing),
  `config [module] --print` (read-only, sandbox-safe),
  `config [module] --set key=value` (**new**: repeatable `StringArray` flag,
  fully non-interactive write bypassing the editor; mutually exclusive with
  `--print`; requires a module argument; e.g.
  `lyx config board --set proposal_prefix=foo- --set home=Home.md`), and
  `config reconcile [--apply]` (dry-run by default). `--set` + `--print` are
  what makes S4's round-trip fully sandbox-native. The write path is backed
  by the new `internal/configengine/set.go` and `internal/yamlengine/set.go`
  (engines, not CLI modules — they add no new registered command, so the
  coverage-invariant module set is unchanged).
- **`internal/initcli/initcli.go`** (updated on `main`): `lyx init` requires
  an existing weft pairing (`l.WeftWorktree()` must already exist) before it
  scaffolds `_lyx/`; already true at the hub level before S6 runs (hub was
  materialized by `sandbox-build.cmd` with host+weft cloned). The scaffolding
  logic now lives in the extracted `internal/initengine` package. **New:**
  `lyx init --undo` (flag on `init`, handled by `initengine.Undo`) reverses a
  prior init — no weft-pairing pre-gate, clean no-op on an uninitialized dir,
  and it commits+pushes the weft-side deletion. This is S6's cleanup step
  (see Decisions → Subfolder init scenario). Both `--undo` and `--set` are
  new *flags/subpaths on existing modules* (`init`, `config`) — they add no
  new top-level module, so `registered` (init, board, config, ide, muxpoc,
  weft, warp, selfreport) is unchanged and the coverage design holds as-is.
- **`internal/warpcli` / `internal/warpengine/checkout.go`** (updated on
  `main`, commit `edde385`): `warp checkout`'s error path was hardened —
  it previously interpolated raw git stderr (`host switch failed: <stderr>`)
  and now emits a clean, wrapped message (`host switch to branch %q failed
  (git exit %d)`). Relevant to **S8** (a bad `warp checkout` now yields a
  legible wrapped error, not raw git output) and to the renamed **S5**
  error-ergonomics scenario (this is precisely the "raw subprocess string
  leaking unwrapped" case S5's Watch note calls a finding — that specific
  warp leak is now fixed, so S5/S8 should observe the clean form).
- **`internal/hubgeometry/hubgeometry.go`**: `Resolve(cwd)` — `WorktreeRoot`
  is the git toplevel, `RelPath` is `filepath.Rel(WorktreeRoot, cwd)`. This
  is the mechanism S6 is validating end-to-end.

## Constraints

- **Hub Geometry Invariant** — S6 exercises `hubgeometry.Resolve`'s
  subfolder contract but does not change it; no code in `internal/hubgeometry`
  is touched by this task.
- **CLI / Cobra Invariant** — the new coverage test is a direct sibling of
  the existing registration/longlist/helptree guards under this invariant.
  *This* task adds no CLI surface of its own (only a new test + doc changes),
  so the existing `Short`/registration/help-tree guards are unaffected. The
  `init --undo` and `config --set` surface merged from `main` was added by
  other tasks with their own guard updates; because both are flags on
  existing modules (not new registered commands), they leave the coverage
  test's `registered` module set unchanged.
- **Documentation Lifecycle** — this task **must** add the new "Sandbox
  Suite Coverage" invariant to `CONSTRAINTS.md` in the same commit as the new
  test, following the existing invariants' format (short prose + "Enforced
  by" pointer to the test name).

## Testing

- The only new automated Go test is `cmd/lyx/sandbox_coverage_test.go` (see
  Decisions → "Coverage test: location and mechanics" for exact assertions).
  This is the TDD candidate for this task: write it against the *current*
  `SANDBOX-SUITE.md` content plan (with all `Covers:`/exclusion changes
  already made) so it's red before the doc changes land and green after.
- `tools/sandbox/SANDBOX-SUITE.md` itself is **not** exercised by automated
  Go tests — it's a prose script an LLM-driven sandbox session follows by
  hand against real GitHub repos. Verify the existing `tools/sandbox/*_test.go`
  files (`main_test.go`, `report_test.go`, `suite_test.go`) don't hardcode
  scenario IDs in ways that would break from the renumber — a `grep -rn "S6"
  tools/sandbox/*.go` during discussion turned up only arbitrary fixture
  strings (`"ref": "S6"` in `report_test.go`/`suite_test.go`) with no coupling
  to actual scenario content, so no code changes are expected there — just
  confirm this holds once the doc changes are made.
- No new engine/CLI test coverage is needed for `weft`, `warp`, `config`, or
  `init` themselves — this task only adds sandbox scenarios and a doc-driven
  coverage guard, not new product behavior.

## Q&A log

- **Q:** How should the coverage test know which scenario covers which
  module — doc-parsed tags, prose matching, or a Go-only hardcoded map? **A:**
  Explicit `**Covers:**` tag lines in `SANDBOX-SUITE.md`, parsed by the test;
  never infer from prose.
- **Q:** Module-level or subcommand-level coverage granularity? **A:**
  Module-level only; subcommand granularity is an explicitly out-of-scope
  future extension.
- **Q:** Which modules are whole-module allowlist exclusions? **A:**
  `muxpoc`, `ide`, `selfreport` — all three, based on reading their actual
  command implementations, not just the subcommands the task brief named.
- **Q:** Where does `registered` (the set of live modules) come from? **A:**
  `newRoot().Commands()` filtered to drop `help`/`completion` — the same
  source `longlist_test.go` already uses, not a separately maintained list.
- **Q:** New scenario numbering after the S6→S5 rename? **A:** S6 subfolder
  init, S7 weft lifecycle, S8 warp introspection, in that order; config
  reconcile extends S4 in place rather than becoming a new scenario.
- **Q:** Does S7 (weft) need a durability/cleanup note like S3's board note?
  **A:** Yes, same style — small marked test change, don't leave the shared
  sandbox remotes in a diverged/broken state.
- **Q:** Should S8's `warp checkout` restore the original branch afterward?
  **A:** Yes — checkout to another branch to prove it works, then back to
  the original, same cleanup discipline as S3/S7.
- **Q:** Where should the new coverage test file live? **A:**
  `cmd/lyx/sandbox_coverage_test.go` — same package as `registration_test.go`/
  `longlist_test.go` because it needs `newRoot()`, confirmed by checking that
  every other guard needing live command-tree introspection already lives in
  `cmd/lyx/`.
- **Q:** (Discussion review round 1, gap) Does the renamed S5
  error-ergonomics scenario already cover a "bad-checkout" case for warp,
  making S8 an extension rather than warp's first scenario? **A:** No —
  verified against the actual `SANDBOX-SUITE.md` text, S5/S6's Watch section
  is generic (wrong-directory/bad-flag/unknown-subcommand) with no
  warp-specific content; `warp` has zero coverage before this task and S8 is
  its first scenario.
- **Q:** (Discussion review round 1, gap) Does S6 (subfolder init) need a
  durability/cleanup note like S3/S7/S8? **A:** Yes — it scaffolds a real
  nested `_lyx/` and touches `.gitignore` in the subfolder, which persists
  across sessions since hub `-reset` is optional, not automatic; S6 must
  clean up the nested `_lyx/`/`.gitignore` change at session end.
- **Q:** (Discussion review round 2, gap) Does the S6 cleanup note need to
  say more than "remove the nested `_lyx/`"? **A:** Yes — the nested `_lyx/`
  is a real directory junction into the weft worktree
  (`HostLyxLink`/`WeftLyxDirFor` are both `RelPath`-keyed), and
  `ReconcileAll` writes config YAMLs through it into the weft side; cleanup
  must remove the weft-side target too, not just the host junction.
- **Q:** (Discussion review round 2, note) Does running `board` from the S6
  subdir actually prove subfolder-scoped resolution? **A:** No —
  `hubgeometry.BoardDir(layout.Hub)` is hub-level and cwd-depth-invariant;
  `config` is the real subfolder-scoping demonstrator, `board`-from-subdir
  is only a "still runs from here" smoke check.
- **Q:** There's no `lyx` command to reverse `init` — how should S6's
  cleanup be handled, given this task's scope is the sandbox suite/doc, not
  new CLI features? **A:** (superseded — see next entry) At the time:
  temporary ad-hoc plain-filesystem/git cleanup, plus a new `lyx-deinit`
  backlog task proposing a real reversal command.
- **Q:** (Revision after merging `main`) The `lyx-deinit` task and a
  `config set` task both landed and merged into `main` — how does that
  change S6 and S4? **A:** S6's cleanup is now a single `lyx init --undo`
  call (the reversal command shipped as a flag on `init`), replacing the
  obsolete ad-hoc workaround; running it also doubles as coverage of the
  reversal path. S4's config round-trip is now fully sandbox-native via the
  new non-interactive `lyx config <module> --set key=value` write plus
  `--print` read-back, then `reconcile`. Neither change adds a new registered
  module, so the coverage-invariant design is unchanged.
- **Q:** (Revision after merging `main`) Does the merged `warp checkout`
  error-wrapping change affect any scenario? **A:** Yes, as context: a bad
  `warp checkout` now emits a clean wrapped error instead of leaking raw git
  stderr, so S8 (warp) and the renamed S5 (error ergonomics) should observe
  the legible form — the specific raw-stderr leak S5's Watch note flags as a
  finding is now fixed for warp checkout.
