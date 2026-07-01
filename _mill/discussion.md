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
  resolution — see Decisions for why).
- Rewrite the "Operating model" note in `SANDBOX-SUITE.md` (currently forbids
  nested `_lyx` scaffolding during a session) to carve out this one scenario
  as the explicit, controlled exception.
- New scenario **S7 — Weft lifecycle**: `weft status/commit/push/pull/sync`
  end to end — make a change, commit via weft, verify status/mirroring.
- New scenario **S8 — Warp introspection**: valid `warp list/pairs/checkout/
  reconcile`. `warp` has no existing scenario today — S8 is the module's
  first (the renamed S5 error-ergonomics check is generic and mentions no
  warp-specific case).
- Extend existing **S4 — Config round-trip** to also run
  `lyx config reconcile` after the existing edit/round-trip check.
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
- The `config` module's interactive editor flow (`lyx config <module>` without
  `--print`, which launches a real editor process) stays untested — only
  `--print` and `reconcile` are sandbox-exercised. This does **not** need its
  own allowlist entry: coverage is module-level, and `config` is already
  covered via S4.
- `warp add`/`clone`/`remove`/`prune`/`cleanup` are not exercised by S8 —
  they mutate/destroy worktree pairs and are out of scope for an
  introspection scenario. Only `list`, `pairs`, `checkout`, `reconcile` are
  in scope for S8.
- Subcommand-level coverage granularity is out of scope for this task (see
  Decisions — module-level only, for now).
- **Adding a `lyx deinit`/`init --undo` command is explicitly out of scope
  for this task.** Writing S6 surfaced that no such command exists (see
  Decisions → Subfolder init scenario), which is a real product gap — it's
  filed as a separate backlog task (`lyx-deinit`, in the wiki) rather than
  bundled into this task's scope. This task's S6 cleanup uses a temporary,
  ad-hoc plain-filesystem/git workaround (see Decisions) until that
  follow-up task lands.

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
  - `S0` (discovery) and `S1` (hub orientation) drive no single module — no
    `Covers:` line (or an explicit `**Covers:** (discovery)` that the test
    ignores as a non-module token; implementer's call).
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
    two directories deep from repo root just like that one.
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
  each session). S6 must therefore remove the nested `_lyx/` (and revert any
  `.gitignore` change) from the subfolder at session end, so a later run
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
  junction into the weft worktree. Removing only the host-side junction
  leaves the real weft-side `<subdir>/_lyx/config/*.yaml` (and any weft
  commit made against it) behind — the durability note above must say
  cleanup removes **both** the host junction *and* the weft-side target
  directory it points to (or an equivalent weft-side revert), not just the
  host `_lyx/`/`.gitignore`, or the "reliably observes not-yet-initialized"
  guarantee it exists to provide does not actually hold.
- **No `lyx` command exists to reverse `init`** (confirmed: no `deinit`
  subcommand, no `--undo` flag on `init`). Cleanup must therefore fall back
  to plain filesystem/git housekeeping, outside `lyx`, for the parts of the
  reversal `lyx` doesn't own — the same precedent S2 already documents
  ("committing host changes with plain git is acceptable and not a
  finding... absence of a lyx-owned command is an intentional design
  choice, not a gap"). This is an explicit **temporary/ad-hoc** measure: a
  new backlog task, `lyx-deinit` (filed in the wiki during this discussion),
  proposes an actual `lyx init --undo` / `lyx deinit` command. Once that
  lands, S6's cleanup note should be rewritten to call the new command
  instead of the manual steps below, and the durability guarantee
  re-reviewed to see if the manual fallback is still needed at all.
  Concretely, S6's cleanup note is: remove the host-side `_lyx` junction,
  remove the weft-side `<subdir>/_lyx` directory it pointed to, and revert
  the `.gitignore` change — all via plain filesystem/git commands, not
  through `lyx`.
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
- **`internal/configcli/configcli.go`**: `config [module]` (edit/menu,
  excluded from sandbox testing when it opens the real editor),
  `config [module] --print` (read-only, already sandbox-safe), and
  `config reconcile [--apply]` (dry-run by default — the new S4 addition).
- **`internal/initcli/initcli.go`**: `lyx init` requires an existing weft
  pairing (`l.WeftWorktree()` must already exist) before it will scaffold
  `_lyx/`; this is already true at the hub level before S6 runs (the hub was
  materialized by `sandbox-build.cmd` with host+weft cloned), so it isn't a
  new precondition S6 needs to set up.
- **`internal/hubgeometry/hubgeometry.go`**: `Resolve(cwd)` — `WorktreeRoot`
  is the git toplevel, `RelPath` is `filepath.Rel(WorktreeRoot, cwd)`. This
  is the mechanism S6 is validating end-to-end.

## Constraints

- **Hub Geometry Invariant** — S6 exercises `hubgeometry.Resolve`'s
  subfolder contract but does not change it; no code in `internal/hubgeometry`
  is touched by this task.
- **CLI / Cobra Invariant** — the new coverage test is a direct sibling of
  the existing registration/longlist/helptree guards under this invariant;
  no new CLI surface is added by this task (only a new test + doc changes),
  so the existing `Short`/registration/help-tree guards are unaffected.
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
  new CLI features? **A:** Temporary ad-hoc plain-filesystem/git cleanup for
  now (matching S2's existing "plain git for housekeeping lyx doesn't own"
  precedent), plus a new backlog task (`lyx-deinit`) filed in the wiki
  proposing a real `lyx init --undo`/`lyx deinit` command — once that lands,
  it should replace the ad-hoc cleanup steps in S6.
