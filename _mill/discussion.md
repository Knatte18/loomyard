# Discussion: batcher: split out of webster into a standalone configreg module with its own batcher.yaml

```yaml
task: 'batcher: split out of webster into a standalone configreg module with its own batcher.yaml'
slug: batcher-standalone-split
status: discussing
parent: main
```

## Problem

`internal/batcher` already ships as its own package — a `Batcher` interface, a name-keyed registry members self-register into, and `Select`, which resolves one active batchifier by name.
What it does not have is its own configuration: the name fed to `Select` lives in `webster.yaml`'s `batcher:` key, loaded into `websterengine.Config.Batcher`, so batching reads as webster-internal execution policy rather than a step in its own right.

Why now: the `Shed` producer model puts `Batchifier` at position 8 in `loom`'s producer list, between `Plan-Review` and `Webster` (`manifest/designs/loom.md:56`).
A producer with its own position in the list cannot have its selection key owned by a different module's config file.
The full task specification is `manifest/designs/shed-followups.md`'s section `## F — batcher-standalone-split` (lines 547–648) — that section, not this file, is the authority on *why*; this file is the authority on *how*, including the two places where the user overrode it (see Decisions).

The key cannot go straight to `loom.yaml` instead: both live `Select` call sites are webster's, and `Shed` — the thing that would own a `loom.yaml` batchifier key — does not exist, so making webster read `loom.yaml` would either break standalone `lyx webster run` or couple two modules' configs for no live benefit.

`depends_on: plan-format-drop-v3-suffix` — satisfied on `main`: `docs/reference/plan-format.md` already carries the un-suffixed filename.

## Scope

**In:**

- `internal/batcher` gains its own config module shape: `template.yaml`, `template.go` (embed + `ConfigTemplate()`), `config.go` (unexported config struct + exported `Active(baseDir string) (Batcher, error)`).
- `internal/configreg/configreg.go` — register `{Name: "batcher", Template: batcher.ConfigTemplate}`, first in the alphabetical list. `SeedOnly` is deliberately omitted, i.e. `false`: batcher's key set is closed and template-owned (one `active:` key), so reconcile manages the file normally rather than seeding it once and never touching it again — unlike `models` (operator-owned aliases) and `burler` (lenses/fans). `configreg_test.go:32`'s `TestModules_SeedOnly` therefore needs no edit: its `want` is computed as `m.Name == "models" || m.Name == "burler"`, which is already `false` for `batcher`.
- `internal/configreg/configreg_test.go:17` — add `"batcher"` to the pinned `want` list.
- `internal/websterengine/config.go` — remove the `Batcher` field from `Config`.
- `internal/websterengine/template.yaml:3` — remove the `batcher: ""` key and its trailing comment.
- `internal/websterengine/runlevel.go` — `RunDeps` gains a `Batcher batcher.Batcher` field; the `batcher.Select(deps.Config.Batcher)` call at `:327` is deleted and `deps.Batcher` used in its place; a `deps.Batcher == nil` guard returning a typed error is added beside the zero-batch refusal (see Decisions → runlevel-call-site).
- `internal/websterengine/runlevel_test.go` — `newRunFixture` (`:250–290`) gains a `Batcher:` field in its `RunDeps` literal, and the file header comment at `:7–8` ("Config.Batcher is left empty in every fixture, resolving to internal/batcher's own DefaultName") is rewritten to describe the injected batchifier instead. This file is `//go:build integration` (Tier 2).
- `internal/webstercli/cli.go:158` — `batcher.Select(websterCfg.Batcher)` becomes `batcher.Active(layout.AnchorPath())`.
- `internal/webstercli/run.go:57` — populate the new `RunDeps.Batcher` field from `c.batcher`.
- `internal/websterengine/config_test.go` — in scope whole, not just one line: the `cfg.Batcher == "identity"` assertion at `:125–127` moves into `internal/batcher`'s own tests in the shape of an `Active` test, the `Batcher: ""` field in the `Config` literal at `:61` is removed with the field, and the `batcher: identity` lines in the YAML fixtures at `:106` and `:139` go stale and must be dropped.
- `internal/webstercli/verbs_test.go` — `seedPersistentPreRunFixture` seeds `batcher.yaml`; the gate-test pair at `:696–732` string-replaces against batcher's template instead of webster's; the comment at `:218–220` is rewritten (the `batcher.Select("")` call itself stays — see Decisions → verbs-test-select-helper).
- Four doc amendments (see Decisions → doc-amendments): `internal/batcher/doc.go`, `CONSTRAINTS.md`'s Batcher Registry+Config Invariant, `docs/overview.md`, `docs/reference/plan-format.md`.
- Five further doc sites discovered during exploration and review, none named in the manifest: `internal/websterengine/doc.go:23–29`, `docs/reference/webster-contract.md:14`, `internal/websterengine/master-template.md:37`, `tools/sandbox/SANDBOX-WEBSTER-SUITE.md:28`, and `internal/planparser/doc.go:4`.

**Out:**

- The `Batcher` interface, the registry, `Select`, and `identityBatcher` — untouched. Only where the name fed to `Select` comes from changes, plus registration and docs.
- No `lyx batcher` cobra command. "Standalone" means configreg-registered, not user-facing.
- No `**Covers:** batcher` sandbox tag (see Constraints).
- No migration handling for a pre-split worktree — user override, see Decisions → no-migration-path.
- `manifest/designs/loom.md` row 8 — explicitly task E's, written after this task lands.
- `manifest/roadmap.md` — not this task's; roadmap moves only on completing or adding a planned item, and this task is one of six already-recorded follow-ups.
- Strict unknown-key validation in `configengine` — considered and rejected, see Decisions → no-migration-path.

## Decisions

### module-shape

- Decision: `batcher` becomes a `configreg`-registered config module with its own `batcher.yaml`, not a cobra command.
- Rationale: a batchifier has no user-facing verb — nothing to invoke. Config registration is the whole of what "standalone" needs to mean here.
- Rejected: a `lyx batcher` command tree (nothing for it to do); leaving the key in `webster.yaml` (contradicts the split); moving the key to `loom.yaml` now (no `loom.yaml` reader exists).

### entry-point-name

- Decision: the exported entry point is `Active(baseDir string) (Batcher, error)`, and the key inside `batcher.yaml` is `active:`.
- Rationale: "active" is the package's own existing vocabulary, not a new coinage — `registry.go:27`'s `Select` doc says it "resolves the active batcher by name", `doc.go:11` says "The active batcher is chosen via …", and `webstercli/cli.go:158`/`:54` already name the local `activeBatcher` and document the field as "the load-time-resolved, config-selected batchifier". The registry is a library: N implementations self-register at init and all exist in the process at once, but exactly one is selected per worktree. "Active" names that selection against the registered-but-idle rest. `batcher.yaml`'s `active:` key and `batcher.Active()` accessor then pair one-to-one.
- Rejected: `Load` (returns a `Batcher`, not a config — the name misleads); `Configured` (says nothing about the library-of-many framing, unused elsewhere in the repo); `Current` (implies runtime mutability; selection is fixed at load time); `Selected` (sits next to `Select` doing something different — worse than a distinct word). For the key name: `batcher:` verbatim (stutters as `batcher.yaml → batcher:`); `active: identity` spelled literally (makes the empty-string default path unreachable from the shipped template and duplicates `DefaultName`).

### default-value

- Decision: `batcher.yaml`'s shipped template is `active: ""`.
- Rationale: preserves the existing empty-resolves-to-`DefaultName` semantics that `Select` and its tests already pin, so the default path stays exercised by the shipped template rather than bypassed by a literal.
- Rejected: `active: identity` — see entry-point-name.

### file-layout

- Decision: mirror the config-module shape every other module already uses — `template.yaml` holding the literal YAML, `template.go` with `//go:embed template.yaml` + `ConfigTemplate() string`, and `config.go` with an unexported config struct plus exported `Active`.
- Rationale: `perchengine`, `reedengine`, `shuttleengine`, and `websterengine` all share the embed-and-accessor shape; `configreg.Module.Template` is typed `func() string` and expects exactly it. Deviating buys nothing.
- Rejected: folding the loading into `registry.go` — fewer files, but breaks the shared shape and mixes registry mechanics with config I/O.

### runlevel-call-site

- Decision: `websterengine.RunDeps` gains a `Batcher batcher.Batcher` field, populated by `webstercli/run.go` from the already-resolved `c.batcher`. `runlevel.go:327`'s `batcher.Select` call is deleted outright; the engine never loads `batcher.yaml`.
- Rationale: this matches the existing layering exactly — `RunDeps` already carries `Config` and `Roles` pre-resolved by the CLI, and `websterengine.Run` does no config I/O today. The alternative was tried on paper first and produced concrete counter-evidence: `newRunFixture` (`internal/websterengine/runlevel_test.go:250`) builds `RunDeps.Config` as an in-memory literal and points `WorktreeRoot` at a bare scratch git repo with no `_lyx/` tree, so a `batcher.Active(deps.WorktreeRoot)` call inside `Run` fails `configengine.Load`'s `FindBaseDir` check and breaks all 15 `TestRun_*` tests.
- **Both options cost the same one-line fixture edit** — that is not the deciding factor, and an earlier draft of this document wrongly claimed the 15 tests would pass untouched under the chosen design. They will not: `newRunFixture` sets no `Batcher` field either, so it needs an injected batchifier just as the alternative needed a seeded config file. What separates the options is the *standing* consequence, not the one-off cost: under the alternative, `Run` becomes the first engine entry point requiring a materialized `_lyx/` tree on `WorktreeRoot`, and every future `Run` test inherits that requirement forever. Under the chosen design the engine stays a pure function of its injected deps.
- **Nil contract:** `Run` guards `deps.Batcher == nil` and returns the named sentinel `ErrNilBatcher = errors.New("webster: RunDeps.Batcher not populated")` before any batching happens. The name and text are decided here, not left to the plan writer, because a dedicated test asserts them: webster's existing sentinels are all package-level `var Err… = errors.New("webster: …")` with a lowercase message (`ErrRunBusy`, `ErrFingerprintMismatch`, `ErrPaused`, `ErrMasterAsking`/`Died`/`Timeout`, `ErrNoBeginRecord`), and this one follows that convention exactly. It lives in `runlevel.go` beside `ErrRunBusy` (`:45`). This is required, not optional: today `RunDeps.Config.Batcher`'s zero value is `""`, which `Select` safely resolves to `identity`, so a caller that forgot the field still worked. A zero-value `batcher.Batcher` is a nil interface, and `deps.Batcher.Batch(plan.Cards)` panics. `Run` has no general deps-validation pass — only `deps.Bisector` is nil-checked (`runlevel.go:820`), and that one has a production fallback rather than a refusal. The guard belongs immediately beside the existing zero-batch refusal, which is the same shape of loud pre-flight over the batchifier, and it is pinned by its own test.
- Rejected for the nil contract: leaving population as an undocumented caller obligation (a missed field is a nil-pointer panic inside the engine rather than a diagnosable error); falling back to `batcher.Select(batcher.DefaultName)` when nil (preserves today's zero-value semantics and needs no fixture change at all, but silently substitutes `identity` when the wiring is wrong — precisely the invisible-divergence hazard this task exists to remove, and the same reason honouring the old `webster.yaml` key was rejected: two worktrees with identical `batcher.yaml` files would batch differently with nothing saying so).
- Rejected: `runlevel` calling `batcher.Active` itself (the literal reading of "both call sites move onto batcher's own entry point" — rejected on the evidence above, and it would also load `batcher.yaml` twice per `run`); a `RunDeps.BatcherName string` field with `Select` still in `runlevel` (preserves the old shape, but the name is meaningless to webster once the key is not webster's).
- Note on manifest fidelity: `shed-followups.md:583–585` says "Both call sites move, not one". Both are still handled — `runlevel.go:327`'s `Select` call disappears entirely and `webstercli/cli.go:158` becomes the single `batcher.Active` call — but only one of the two ends up *calling* batcher's new entry point. This is a deliberate, evidence-driven divergence from the manifest's wording, recorded here so the plan does not read it as an oversight.

### no-migration-path

- Decision: this task ships no migration handling for a pre-split worktree. No orphan-key reporting code, no migration test, no strict unknown-key validation.
- Rationale: **user override, recorded verbatim in intent** — no pre-split worktrees exist, so the state being migrated from does not exist and does not need supporting. What remains is free and requires no code: once `batcher:` leaves `internal/websterengine/template.yaml`, `yamlengine.Reconcile` already reports it as a `Removed` key for module `webster` and prunes it on `lyx config reconcile --apply`; `configengine.Load` validates only *missing* keys, and `yaml.Unmarshal` into a `Config` with no `Batcher` field silently ignores an extra one. So a leftover key is already reported-and-never-honoured by existing generic machinery.
- Rejected: a dedicated orphan-key notice on `configsync.Result` mirroring `MigratedFrom` (invents module-specific migration machinery for a case the generic path covers); a hardcoded batcher-specific hint in `configcli`'s reconcile output (task-specific string in a registry-driven surface); strict unknown-key rejection in `configengine` (cross-cutting, affects all ten modules, far larger than this task); a `configsync` test asserting `Removed` contains `batcher` (re-tests `yamlengine`'s own generic behaviour, pins nothing this task can break).
- Manifest divergence: this drops step 4 "Migration" of `shed-followups.md:593–600`, which requires reconcile to report the leftover key as an orphaned key. The *observable behaviour* that section asks for still holds via the generic path; what is dropped is building or testing anything for it.

### absent-config-is-a-hard-error

- Decision: `batcher.Active` returns `configengine.Load`'s standard error when `_lyx/config/batcher.yaml` is absent — "config file …/batcher.yaml not found; run \"lyx config reconcile\"". No fallback to `DefaultName`.
- Rationale: identical to every other module's behaviour; no batcher-specific absence path. A module that silently tolerates a missing config is the one module whose config never gets created.
- Rejected: falling back to `DefaultName` when the file is absent — never blocks a run, but hides that the split happened and makes `batcher` the sole exception.

### reconcile-required-for-pre-registry-wefts

- Decision: a weft whose tracked `_lyx/config/` was written before `batcher` joined `configreg` gets no `batcher.yaml` until an operator runs `lyx config reconcile --apply` and commits the result. Until then, every `lyx webster <verb>` aborts in `PersistentPreRunE` with `configengine.Load`'s "config file …/batcher.yaml not found; run \"lyx config reconcile\"" error. This is accepted as the standard reconcile step, not worked around.
- Rationale: `_lyx/config/*.yaml` is committed weft content, materialized once by `configsync.ReconcileAll(res.WeftBase, true)` at `internal/fabriccli/clone.go:83` — writing one file per module registered in `configreg` *as of the binary that ran that clone*. Nothing re-runs that afterwards; `lyx config reconcile` is the only thing that adds a newly-registered module's file. So the affected set is defined by the weft's committed config content, not by worktree age: a worktree created after this task lands still lacks `batcher.yaml` if it branches off such a weft, and one `reconcile --apply` plus commit on the weft fixes every worktree off it at once. A hub cloned after this lands is unaffected, since `clone` reconciles against the then-current registry — which also means the sandbox suites (`tools/sandbox/SANDBOX-WEBSTER-SUITE.md`) that clone a fresh hub need no change.
- The error is loud, names the exact missing file, and already names the fixing command, so it is self-resolving for an operator who hits it.
- Sandbox precision: the suites keep *running* green, since each drives a fresh `clone` that reconciles against the then-current registry. Their *prose* is a separate matter — `tools/sandbox/SANDBOX-WEBSTER-SUITE.md:28` enumerates exactly which config files a wired worktree materializes and that enumeration becomes incomplete, so it is doc site 8 below. "The suites need no change" is true operationally and false textually; both halves are stated so neither is mistaken for the other.
- Rejected: declaring the case out of existence (narrower than it looks — it turns on whether any weft in use has a committed `_lyx/config/` predating this task, which was not verified; this worktree has no `_lyx/config/` at all, so it proves nothing either way); having this task run reconcile and commit `batcher.yaml` into the weft as part of landing (commits generated config from a task branch and puts config materialization inside a code task).
- Relationship to no-migration-path: distinct concerns. That decision is about the *orphaned* `batcher:` key in `webster.yaml`, which needs no code. This one is about the *newly required* `batcher.yaml` file, which needs an operator action. Neither implies the other.

### verbs-test-select-helper

- Decision: `internal/webstercli/verbs_test.go:221`'s direct `batcher.Select("")` call stays as-is. Only its explanatory comment at `:218–220` is rewritten.
- Rationale: `Select` remains exported and its `"" → DefaultName` behaviour is unchanged, so the call still compiles and still produces exactly the batchifier a real `PersistentPreRunE` would have stored on `c.batcher`. Routing this hand-built `*websterCLI` literal through `Active` instead would require the test to materialize an `_lyx/config/` tree for no gain — the test is deliberately bypassing `PersistentPreRunE`, not exercising it. The comment is what goes stale: it currently says the resolution is "exactly what `PersistentPreRunE` would have resolved and stored on `c.batcher`", which must now name `Active`-resolved `c.batcher` as the thing being stood in for.
- Rejected: switching the call to `Active` (needs a config tree for a test whose whole point is bypassing config load); deleting the stand-in and letting `c.batcher` be nil (the verbs under test call `c.batcher.Batch`).

### persistentprerun-ordering

- Decision: the `batcher.Active` call stays exactly where `batcher.Select` sits today — `internal/webstercli/cli.go:158`, after `websterengine.LoadConfig` at `:151`.
- Rationale: the position decides which module's not-found error a half-reconciled worktree surfaces first. Leaving it in place preserves today's precedence (shuttle → reed → webster → batcher, `cli.go:137–163`), so no existing test's expected error message changes. Moving it earlier is possible — `Active` no longer depends on `websterCfg` — but buys nothing and silently reorders operator-visible failure output.
- Rejected: moving it ahead of the three `LoadConfig` calls (reorders error precedence for no benefit and would need existing fixture expectations re-checked).

### doc-amendments

- Decision: nine doc sites, each its own step. The four from the manifest plus five found during exploration and review.
- Rationale: each site states something this task falsifies, so each belongs to this task under the repo's own ownership rule.
- The sites:
  1. `internal/batcher/doc.go` — the package comment must stop saying batching is "100% webster's own execution-policy decision" (lines 5–6) and instead say it is a standalone step webster consumes today and `Shed` will drive as producer #8 once built. The "chosen via webster.yaml's batcher: config key" paragraph (lines 11–14) must name `batcher.yaml`'s `active:` key and `Active` instead.
  2. `CONSTRAINTS.md:331–336`, the Batcher Registry+Config Invariant — both the ownership claim and the config-key pin at `:335` ("the `batcher:` webster.yaml config key") move to `batcher.yaml`'s `active:` key.
  3. `docs/overview.md:282` — the batcher module-table entry, which pins the key to `webster.yaml`. `docs/overview.md:278` (the webster entry) also describes batcher as "its own config-selected `internal/batcher` registry" and needs the same correction.
  4. `docs/reference/plan-format.md:20–28`, the "Batch is gone / the card is the unit" section — the card stays the plan's unit, but the "entirely internal to webster" framing at `:22`/`:24` goes.
  5. `internal/websterengine/doc.go:23–29` — the whole batcher paragraph, not just the config-key clause. Two separate claims here go stale, and fixing only one leaves the two package docs contradicting each other: `:25–26` says the batchifier is "selected once at config-load time via webster.yaml's `batcher:` key" (now `batcher.yaml`'s `active:` key), and `:27` carries the identical "Batching is 100% webster's own execution-policy decision" sentence that site 1 removes from batcher's own doc. Reword `:27` here the same way site 1 rewords it there — a standalone step webster consumes, not webster's own policy — while keeping the "never the plan's / never an LLM's" clauses at `:28–29`, which stay true. Not in the manifest's list; the manifest names `doc.go:12` and `:25–27`, which have since drifted.
  6. `docs/reference/webster-contract.md:14` — "webster groups a plan's cards into execution batches via a config-selected batcher". Ambiguous today, actively wrong-reading once the config is not webster's; a one-clause fix.
  7. `internal/websterengine/master-template.md:37` — "`lyx webster` groups this flat list into execution batches via the plan's configured batchifier". Not in the manifest's list. This is an embedded agent prompt, and the line is wrong *today*: the batchifier is not the plan's, it is config's. This task makes it wrong in a second way by moving the config owner, so it is this task's to fix under the same ownership rule the other six use. Correct it to name `batcher.yaml`'s configured batchifier, leaving the surrounding "you drive the loop by BATCH number, not by reasoning about grouping yourself" instruction untouched.
  8. `tools/sandbox/SANDBOX-WEBSTER-SUITE.md:28` — "**Wired worktree required.** `lyx webster` requires a worktree wired by `lyx fabric clone`/`lyx fabric add` — which materializes `_lyx/config/webster.yaml`, plus `shuttle.yaml`/`reed.yaml` since webster branches off shuttle directly". The enumeration is complete today and incomplete the moment `batcher.yaml` becomes required, so it is falsified by this task even though the suite still runs green (a fresh `clone` materializes the new file automatically). Add `batcher.yaml` to the list. Not in the manifest; found in review round 2.
  9. `internal/planparser/doc.go:4` — "every consumer (webster's batcher, master, and fork prompt rendering) goes through planparser.ParsePlan". The possessive is the same ownership claim this task removes everywhere else, so it is in scope under the same rule: reword to "the batcher, webster's master, and fork prompt rendering". A one-word fix; listed rather than swept silently because the eight-site list above claims completeness for "sites whose claim this task falsifies", and leaving an unlisted ninth would falsify that claim instead.

## Technical context

**What `internal/batcher` is today** (`batcher.go`, `registry.go`, `identity.go`, `doc.go`, plus tests):
`Batch{Cards []planparser.Card}` and the `Batcher` interface (`Batch(cards) []Batch`, `Name() string`); an unexported `registry map[string]Batcher` with `register`/`lookup`; `DefaultName = "identity"`; and `Select(name string) (Batcher, error)`, which maps `""` to `DefaultName` and errors `batcher: unknown batcher %q` otherwise.
`identityBatcher` self-registers in `init()`.
The package's only non-stdlib import is `internal/planparser`. Adding `internal/configengine` introduces no cycle: `configengine` imports `envsource`, `lyxdirs`, `yamlengine` only, and nothing in that set imports `batcher` or `configreg`.

**The config-module pattern to copy** — `internal/websterengine/template.go` is the tightest example: `import _ "embed"`, `//go:embed template.yaml`, `var configTemplate string`, `func ConfigTemplate() string`.
`internal/websterengine/config.go`'s `LoadConfig` shows the load shape: `configengine.Load(baseDir, module, []byte(ConfigTemplate()))`, then `yaml.Unmarshal` into the struct.
**Two distinct error paths, both decided — do not conflate them.** `Active` surfaces one of two different messages depending on what is missing:
1. **No `_lyx/` tree at all** (`configengine.FindBaseDir` fails). `configengine.Load` returns "not initialized: `_lyx`/ directory not found". `websterengine.LoadConfig` rewrites that to `not initialized here; run "lyx fabric reconcile"` by matching on `strings.Contains(err.Error(), "not initialized")`, matching `perchengine`/`reedengine`/`shuttleengine`. **Decision: `Active` does the same rewrite, with the same `lyx fabric reconcile` text.** The worktree is unwired; a config-level reconcile cannot help.
2. **`_lyx/` exists but `batcher.yaml` does not.** `configengine.Load` returns `config file <path> not found; run "lyx config reconcile"` and no module rewrites it. **Decision: `Active` passes this through unchanged.** This is the message the reconcile-required-for-pre-registry-wefts decision depends on, and it already names the fixing command.
The two are not interchangeable: `fabric reconcile` wires a worktree, `config reconcile` materializes a module's file. TDD candidate 2 asserts path 2 specifically, over a base dir with an `_lyx/` directory and no config file.
Note `Active` takes only `baseDir` — the module name `"batcher"` is batcher's own constant, not a parameter, since unlike `LoadConfig(baseDir, module)` there is no second module this entry point serves.

**`configengine.Load` semantics** (`internal/configengine/config.go`): requires `<baseDir>/_lyx` to exist (`FindBaseDir`), requires the config file to exist, then checks `yamlengine.MissingKeys(template, fileBytes)` — *missing* keys only, never extra ones — then resolves `${env:…}` via `yamlengine.Resolve`. An unknown key in a config file is not an error at any layer.

**`yamlengine.Reconcile`** (`internal/yamlengine/reconcile.go:21`) returns `(merged, added, removed, err)`. `merged` is built from the *template*'s node tree with existing values overlaid, so a key absent from the template is dropped from `merged` and simultaneously reported in `removed`. `configsync.ReconcileAll` surfaces that as `Result.Removed`, and `configcli.go:284` renders it into the reconcile JSON envelope. This is the machinery that makes the no-migration-path decision safe.

**`configreg` consumers** are registry-driven throughout (`configcli.go:42,70,99,154,246,319`, `menu.go:27`, `configsync.go:80`, `configcli_test.go:463,497`, `configcli_integration_test.go:39`). The only hardcoded module list is `internal/configreg/configreg_test.go:17`'s `want` slice, which pins both membership and alphabetical order. `batcher` sorts first, before `board`.

**`internal/lyxtest.SeedConfig`** takes an explicit `map[string]string` of module name to YAML content and commits the result, so adding a configreg module does not implicitly change any existing fixture. Only fixtures that need `batcher.yaml` on disk must add it.

**The two live call sites:**
- `internal/websterengine/runlevel.go:327` — `active, err := batcher.Select(deps.Config.Batcher)`, wrapped as `fmt.Errorf("webster: %w", err)`, immediately followed by `active.Batch(plan.Cards)` and the zero-batch refusal. Under this task's decision the `Select` call and its error wrap both go; `deps.Batcher.Batch(plan.Cards)` replaces `active.Batch(...)`. The zero-batch refusal stays exactly as-is.
- `internal/webstercli/cli.go:158`, inside `PersistentPreRunE` — resolves after `websterengine.LoadConfig` and stores the result on `c.batcher` (`:193`). `c.batcher` is consumed by `awaitbatch.go:62`, `recordbatch.go:75`, `beginbatch.go:63`, and `recoverbatch.go:85`, all unchanged. Only the resolution line changes, from `batcher.Select(websterCfg.Batcher)` to `batcher.Active(layout.AnchorPath())`. It can also move earlier in `PersistentPreRunE`, since it no longer depends on `websterCfg`.

**`RunDeps` plumbing** — `internal/webstercli/run.go:57` constructs `websterengine.RunDeps` field-by-field; add `Batcher: c.batcher`. `RunDeps.WorktreeRoot` is already `c.layout.AnchorPath()`, the same base dir `Active` would take, which is why the alternative looked cheap before the fixture evidence.

**Test sites that break by construction:**
- `internal/webstercli/verbs_test.go:684`'s `seedPersistentPreRunFixture` seeds `shuttle`/`reed`/`webster` only. Once `PersistentPreRunE` calls `Active`, it must also seed `batcher`. Its doc comment (`:677–683`) explains that webster.yaml's content is caller-supplied "so a test can override its batcher: key" — that rationale moves to the batcher config.
- `internal/webstercli/verbs_test.go:696–732`, the gate-test pair `TestPersistentPreRunE_UnknownBatcherFailsFast` and `TestPersistentPreRunE_DefaultBatcherResolves`. Both `strings.Replace` against `websterengine.ConfigTemplate()`'s `batcher: ""` literal, so both break the moment the key leaves that template. Taken whole, per the manifest — the pair moves to replacing `active: ""` in `batcher.ConfigTemplate()`, and the fixture helper's parameter becomes the batcher config rather than the webster config. Their *behaviour* is preserved: an unknown name still aborts before any verb's `RunE`, proven via the `status` verb, which never touches the batcher.
- `internal/webstercli/verbs_test.go:221–223` — a hand-built `*websterCLI` literal calls `batcher.Select("")` directly to stand in for what `PersistentPreRunE` would have resolved. `Select` still exists and still behaves this way, so this compiles untouched; only its explanatory comment (which names `c.batcher` and `PersistentPreRunE`) needs a light re-read for accuracy.
- `internal/websterengine/config_test.go:125` — `cfg.Batcher != "identity"` stops compiling with the field. It moves to `internal/batcher` as an `Active`-level assertion.

**Line-number drift to expect** — the manifest was written against an older tree. `docs/overview.md:267`/`:271` are now `:278` and `:282`; `internal/websterengine/doc.go:12` and `:25–27` are now the claim at `:25–26`; and the manifest's `runlevel.go:332` is `:327` today — that last one was carried into this document unchecked in an earlier draft and corrected only after a citation-accuracy review pass, which is the concrete reason for the rule that follows. Re-derive by grep rather than trusting the manifest's numbers; the same applies to every line reference in this file.

## Constraints

From `CONSTRAINTS.md`:

- **Batcher Registry+Config Invariant** (`:331–336`) — this task's own invariant, and this task amends it. The invariant's substance survives intact: batching is still selected by `internal/batcher`'s name-keyed registry plus a config key with default `identity`, still with no plan-supplied batching and no batch grouping in the plan format. Only the file the key lives in changes. Both the ownership claim and the `webster.yaml` pin at `:335` must be updated in the same commit.
- **Cwd Resolution Invariant** — relevant and satisfied by construction: `batcher.Active` takes an already-resolved `baseDir` from the caller and never calls `lyxcwd` itself. `webstercli/cli.go` resolves cwd once via `lyxcwd.Getwd`/`lyxcwd.Resolve` and passes `layout.AnchorPath()`. `batcher` declares no path segment of its own — its config lands at `_lyx/config/batcher.yaml` via `configengine.ConfigFile`, exactly like every other module — so there is no per-module subdirectory to own.
- **CLI / Cobra Invariant** — does **not** apply. Nothing is registered on the cobra root; there is no `Command()`, no `Short`, no help-tree entry.
- **Sandbox Suite Coverage** — does **not** apply, and must not be satisfied. `cmd/lyx/sandbox_coverage_test.go:38–47` enumerates `newRoot().Commands()`, i.e. cobra registration, not `configreg`. Adding a `**Covers:** batcher` tag would fail that test's drift assert. Verified against the current file during exploration.
- **Documentation Lifecycle** — applies: this task changes observable config behaviour and a module's shape, so `docs/overview.md`, the affected reference docs, and `CONSTRAINTS.md` all land in the same commit as the code.
- **Test Tier Purity Invariant** (`:288–299`) — applies to the new `internal/batcher` tests and must not be tripped. All three existing test files there carry the header "Tier-1 (pure logic, no git, no TestMain)". An untagged test file must not call `gitexec.RunGit`, `exec.Command`/`exec.CommandContext`, or `lyxtest.Copy*`. The seeding decision below keeps `internal/batcher` untagged and Tier-1. Enforced by `cmd/lyx/tierpurity_test.go`.
- **Hermetic Git Test Environment Invariant** (`:301–309`) — would apply if the new tests seeded config via `lyxtest.SeedConfig`, since that helper is named in the invariant's own git-spawning list and would oblige `internal/batcher` to add a `TestMain` calling `lyxtest.HermeticGitEnv()`. The seeding decision below avoids that entirely, so `internal/batcher` stays without a `TestMain`. Enforced by `cmd/lyx/hermeticenv_test.go`.

Discovered during discussion:

- `manifest/designs/loom.md:56` (producer table row 8) names `webster.yaml`'s `batcher:` key and is now falsified by this task — but it is explicitly task E's to write, per `shed-followups.md`'s Scope section. Do not edit it. Task E is sequenced after this one precisely so it can reflect what this task lands.

## Testing

**Seeding mechanism for every new `internal/batcher` test — decided, not left to the plan writer.**
Config fixtures are written with a plain-filesystem helper: `os.MkdirAll(configengine.ConfigDir(baseDir), 0o755)` then `os.WriteFile(configengine.ConfigFile(baseDir, "batcher"), …)`, over a `t.TempDir()` base.
Copy the shape of `internal/websterengine/config_test.go:21–32`'s `seedConfig`, whose own doc comment already states the reason: it is "a plain-filesystem stand-in for `lyxtest.SeedConfig`, deliberately avoiding that helper's git spawn since `configengine.Load` never needs a repository."
`configengine.FindBaseDir` checks only that `<baseDir>/_lyx` exists — no git repository is involved at any point in `Active`'s path.
Do **not** use `lyxtest.SeedConfig`: it spawns git, which drags `internal/batcher` under the Hermetic Git Test Environment Invariant (a `TestMain` calling `lyxtest.HermeticGitEnv()`) and out of Tier-1, contradicting the "Tier-1 (pure logic, no git, no TestMain)" header all three existing test files carry.
`internal/batcher` gains no `TestMain` and no build tag as a result of this task.

Existing evidence that must keep passing untouched: `internal/batcher/batcher_test.go`, `registry_test.go`, and `identity_test.go`. They are the proof that only the configuration source moved and the batching itself did not. If any of them needs editing, something outside this task's scope has changed.

TDD candidates, in the order they are worth writing:

1. **`internal/batcher` — `Active` resolves from `batcher.yaml`.** The core new behaviour. Seed `_lyx/config/batcher.yaml` under a `t.TempDir()` base per the seeding mechanism above, call `Active(baseDir)`, assert the returned `Batcher`'s `Name()`. Cover: the shipped-template default (`active: ""` → `identity`), an explicit `active: "identity"`, and an unknown name (error naming the bad value, reusing `Select`'s existing message). This absorbs the `cfg.Batcher == "identity"` assertion moving out of `internal/websterengine/config_test.go:125`.
2. **`internal/batcher` — absent config is a hard error.** `Active` against a base dir with an `_lyx/` tree (created with `os.MkdirAll`, no config file) returns an error naming the file and pointing at `lyx config reconcile`. Guards the absent-config-is-a-hard-error decision against a later silent-default regression, and is the unit-level counterpart of the operator step recorded in reconcile-required-for-pre-registry-wefts.
3. **`internal/configreg` — `batcher` is in the module list, in sort order.** Extend `configreg_test.go:17`'s existing `want` slice rather than adding a parallel test; that test already pins order as user-visible.
4. **`internal/webstercli` — the `PersistentPreRunE` gate still fails fast.** The relocated pair from `verbs_test.go:696–732`, driving `Command()`'s real `PersistentPreRunE` (never a hand-built `*websterCLI`), with the bad name now injected into `batcher.yaml`. Behaviour asserted is unchanged: exit 1, `"ok":false`, and the unknown-batcher message naming the bad key, proven via the `status` verb.

Not written, per the no-migration-path decision: any test asserting reconcile reports a leftover `webster.yaml` `batcher:` key.

Scenarios that must be covered somewhere, whatever the final test shape:

- `websterengine.Run` uses the injected `RunDeps.Batcher` and does no config I/O — proven by the 15 existing `TestRun_*` tests passing after a one-line `Batcher:` injection into `newRunFixture`, still against a `WorktreeRoot` that has no `_lyx/` tree. The injection is expected; a *seeded config file* is not. If those tests start needing one, the runlevel-call-site decision has been implemented wrongly.
- The `deps.Batcher == nil` guard refuses with its typed error rather than panicking — its own test, since nothing else would ever exercise the branch (`webstercli` always populates the field).
- `webstercli`'s four `c.batcher` consumers (`awaitbatch`, `recordbatch`, `beginbatch`, `recoverbatch`) are unaffected — existing tests cover them and must not need edits.

Acceptance: `go build ./...` and `go test ./...` pass. The config relocation is the only behavioural change.

## Q&A log

- **Q:** How does `websterengine.Run` get its batcher once `Config.Batcher` is gone? **A:** Initially "runlevel calls `batcher.Active` itself"; reversed to a `RunDeps.Batcher` field after exploration turned up concrete evidence (all 15 `TestRun_*` tests break; `Run` becomes the first engine entry point requiring a materialized `_lyx/` tree). The user's reasoning for reversing: this is new, concrete evidence that changes the calculation, not a one-off fixture cost to pay and forget.
- **Q:** What is the key inside `batcher.yaml`, and its default? **A:** `active: ""`.
- **Q:** How does reconcile report the leftover `webster.yaml` `batcher:` key? **A:** Initially "rely on the existing generic `Removed` path, add a test"; superseded — no test, no code. See the next entry.
- **Q:** Where does the migration test live? **A:** Nowhere. Pre-split worktrees do not exist, so that state does not need supporting.
- **Q:** "Hard error" on what, exactly? **A:** Only the absent-`batcher.yaml` case, which is `configengine.Load`'s standard behaviour. No strict unknown-key validation in `configengine` — the state it would catch does not exist.
- **Q:** File layout inside `internal/batcher`? **A:** Mirror every other config module — separate `template.yaml`, `template.go`, `config.go`.
- **Q:** [review round 2 gap] A zero-value `RunDeps.Batcher` is a nil interface where `Config.Batcher`'s zero value used to resolve safely to `identity`. What is the contract? **A:** `Run` guards `deps.Batcher == nil` with a typed error beside the zero-batch refusal, pinned by its own test. A nil-means-default fallback was rejected as the same invisible-divergence hazard the whole task exists to remove. Standing instruction from this point: take the recommended option on every review gap without asking.
- **Q:** [review round 1 gap] A weft whose `_lyx/config/` predates this task gets no `batcher.yaml`, so `lyx webster` aborts until reconcile is run. How is that handled? **A:** Record it as an accepted one-step operator action, together with the fact that `fabric clone` reconciles automatically so fresh hubs and the sandbox suites are unaffected. Clarified during the exchange that "predates" is about the weft's *committed config content*, not worktree age — a worktree created after this lands is still affected if it branches off such a weft.
- **Q:** [review round 1 gap] How do the new `internal/batcher` tests seed `_lyx/config/batcher.yaml`? **A:** Plain filesystem, no git, copying `internal/websterengine/config_test.go:21–32`'s `seedConfig` — keeps the package Tier-1 with no `TestMain`, avoiding both the Test Tier Purity and Hermetic Git invariants.
- **Q:** Entry-point name, and what makes a batcher "active"? **A:** `Active`. The word is the package's own existing vocabulary (`registry.go:27`, `doc.go:11`, `webstercli/cli.go:54`,`:158`); it names the one registry entry this worktree's config selects, against the registered-but-idle rest.
