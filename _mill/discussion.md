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
- `internal/configreg/configreg.go` — register `{Name: "batcher", Template: batcher.ConfigTemplate}`, first in the alphabetical list.
- `internal/configreg/configreg_test.go:17` — add `"batcher"` to the pinned `want` list.
- `internal/websterengine/config.go` — remove the `Batcher` field from `Config`.
- `internal/websterengine/template.yaml:3` — remove the `batcher: ""` key and its trailing comment.
- `internal/websterengine/runlevel.go` — `RunDeps` gains a `Batcher batcher.Batcher` field; the `batcher.Select(deps.Config.Batcher)` call at `:332` is deleted and `deps.Batcher` used in its place.
- `internal/webstercli/cli.go:158` — `batcher.Select(websterCfg.Batcher)` becomes `batcher.Active(layout.AnchorPath())`.
- `internal/webstercli/run.go:57` — populate the new `RunDeps.Batcher` field from `c.batcher`.
- `internal/websterengine/config_test.go:125` — the `cfg.Batcher == "identity"` assertion moves into `internal/batcher`'s own tests, in the shape of an `Active` test.
- `internal/webstercli/verbs_test.go` — `seedPersistentPreRunFixture` seeds `batcher.yaml`; the gate-test pair at `:696–732` string-replaces against batcher's template instead of webster's; the `batcher.Select("")` helper use at `:221–223` is revisited.
- Four doc amendments (see Decisions → doc-amendments): `internal/batcher/doc.go`, `CONSTRAINTS.md`'s Batcher Registry+Config Invariant, `docs/overview.md`, `docs/reference/plan-format.md`.
- Two further doc sites discovered during exploration and not named in the manifest: `internal/websterengine/doc.go:23–25` and `docs/reference/webster-contract.md:14`.

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
- Rationale: "active" is the package's own existing vocabulary, not a new coinage — `registry.go:31`'s `Select` doc says it "resolves the active batcher by name", `doc.go:12` says "The active batcher is chosen via …", and `webstercli/cli.go:158`/`:54` already name the local `activeBatcher` and document the field as "the load-time-resolved, config-selected batchifier". The registry is a library: N implementations self-register at init and all exist in the process at once, but exactly one is selected per worktree. "Active" names that selection against the registered-but-idle rest. `batcher.yaml`'s `active:` key and `batcher.Active()` accessor then pair one-to-one.
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

- Decision: `websterengine.RunDeps` gains a `Batcher batcher.Batcher` field, populated by `webstercli/run.go` from the already-resolved `c.batcher`. `runlevel.go:332`'s `batcher.Select` call is deleted outright; the engine never loads `batcher.yaml`.
- Rationale: this matches the existing layering exactly — `RunDeps` already carries `Config` and `Roles` pre-resolved by the CLI, and `websterengine.Run` does no config I/O today. The alternative was tried on paper first and produced concrete counter-evidence: `newRunFixture` (`internal/websterengine/runlevel_test.go:250`) builds `RunDeps.Config` as an in-memory literal and points `WorktreeRoot` at a bare scratch git repo with no `_lyx/` tree, so a `batcher.Active(deps.WorktreeRoot)` call inside `Run` fails `configengine.Load`'s `FindBaseDir` check and breaks all 16 `TestRun_*` tests. The one-line fixture fix is cheap, but the standing consequence is not: `Run` would become the first engine entry point requiring a materialized `_lyx/` tree on `WorktreeRoot`, and every future `Run` test would inherit that requirement.
- Rejected: `runlevel` calling `batcher.Active` itself (the literal reading of "both call sites move onto batcher's own entry point" — rejected on the evidence above, and it would also load `batcher.yaml` twice per `run`); a `RunDeps.BatcherName string` field with `Select` still in `runlevel` (preserves the old shape, but the name is meaningless to webster once the key is not webster's).
- Note on manifest fidelity: `shed-followups.md:583–585` says "Both call sites move, not one". Both are still handled — `runlevel.go:332`'s `Select` call disappears entirely and `webstercli/cli.go:158` becomes the single `batcher.Active` call — but only one of the two ends up *calling* batcher's new entry point. This is a deliberate, evidence-driven divergence from the manifest's wording, recorded here so the plan does not read it as an oversight.

### no-migration-path

- Decision: this task ships no migration handling for a pre-split worktree. No orphan-key reporting code, no migration test, no strict unknown-key validation.
- Rationale: **user override, recorded verbatim in intent** — no pre-split worktrees exist, so the state being migrated from does not exist and does not need supporting. What remains is free and requires no code: once `batcher:` leaves `internal/websterengine/template.yaml`, `yamlengine.Reconcile` already reports it as a `Removed` key for module `webster` and prunes it on `lyx config reconcile --apply`; `configengine.Load` validates only *missing* keys, and `yaml.Unmarshal` into a `Config` with no `Batcher` field silently ignores an extra one. So a leftover key is already reported-and-never-honoured by existing generic machinery.
- Rejected: a dedicated orphan-key notice on `configsync.Result` mirroring `MigratedFrom` (invents module-specific migration machinery for a case the generic path covers); a hardcoded batcher-specific hint in `configcli`'s reconcile output (task-specific string in a registry-driven surface); strict unknown-key rejection in `configengine` (cross-cutting, affects all ten modules, far larger than this task); a `configsync` test asserting `Removed` contains `batcher` (re-tests `yamlengine`'s own generic behaviour, pins nothing this task can break).
- Manifest divergence: this drops step 4 "Migration" of `shed-followups.md:593–600`, which requires reconcile to report the leftover key as an orphaned key. The *observable behaviour* that section asks for still holds via the generic path; what is dropped is building or testing anything for it.

### absent-config-is-a-hard-error

- Decision: `batcher.Active` returns `configengine.Load`'s standard error when `_lyx/config/batcher.yaml` is absent — "config file …/batcher.yaml not found; run \"lyx config reconcile\"". No fallback to `DefaultName`.
- Rationale: identical to every other module's behaviour; no batcher-specific absence path. A module that silently tolerates a missing config is the one module whose config never gets created.
- Rejected: falling back to `DefaultName` when the file is absent — never blocks a run, but hides that the split happened and makes `batcher` the sole exception.

### doc-amendments

- Decision: six doc sites, each its own step. The four from the manifest plus two found during exploration.
- Rationale: each site states something this task falsifies, so each belongs to this task under the repo's own ownership rule.
- The sites:
  1. `internal/batcher/doc.go` — the package comment must stop saying batching is "100% webster's own execution-policy decision" (lines 5–6) and instead say it is a standalone step webster consumes today and `Shed` will drive as producer #8 once built. The "chosen via webster.yaml's batcher: config key" paragraph (lines 12–15) must name `batcher.yaml`'s `active:` key and `Active` instead.
  2. `CONSTRAINTS.md:331–336`, the Batcher Registry+Config Invariant — both the ownership claim and the config-key pin at `:335` ("the `batcher:` webster.yaml config key") move to `batcher.yaml`'s `active:` key.
  3. `docs/overview.md:282` — the batcher module-table entry, which pins the key to `webster.yaml`. `docs/overview.md:278` (the webster entry) also describes batcher as "its own config-selected `internal/batcher` registry" and needs the same correction.
  4. `docs/reference/plan-format.md:20–28`, the "Batch is gone / the card is the unit" section — the card stays the plan's unit, but the "entirely internal to webster" framing at `:22`/`:24` goes.
  5. `internal/websterengine/doc.go:23–25` — "selected once at config-load time via webster.yaml's `batcher:` key". Not in the manifest's list; the manifest names `doc.go:12` and `:25–27`, which have since drifted.
  6. `docs/reference/webster-contract.md:14` — "webster groups a plan's cards into execution batches via a config-selected batcher". Ambiguous today, actively wrong-reading once the config is not webster's; a one-clause fix.

## Technical context

**What `internal/batcher` is today** (`batcher.go`, `registry.go`, `identity.go`, `doc.go`, plus tests):
`Batch{Cards []planparser.Card}` and the `Batcher` interface (`Batch(cards) []Batch`, `Name() string`); an unexported `registry map[string]Batcher` with `register`/`lookup`; `DefaultName = "identity"`; and `Select(name string) (Batcher, error)`, which maps `""` to `DefaultName` and errors `batcher: unknown batcher %q` otherwise.
`identityBatcher` self-registers in `init()`.
The package's only non-stdlib import is `internal/planparser`. Adding `internal/configengine` introduces no cycle: `configengine` imports `envsource`, `lyxdirs`, `yamlengine` only, and nothing in that set imports `batcher` or `configreg`.

**The config-module pattern to copy** — `internal/websterengine/template.go` is the tightest example: `import _ "embed"`, `//go:embed template.yaml`, `var configTemplate string`, `func ConfigTemplate() string`.
`internal/websterengine/config.go`'s `LoadConfig` shows the load shape: `configengine.Load(baseDir, module, []byte(ConfigTemplate()))`, then `yaml.Unmarshal` into the struct.
Webster additionally wraps a "not initialized" error with a module-specific hint; batcher should do the same for consistency with `perchengine`/`reedengine`/`shuttleengine`.
Note `Active` takes only `baseDir` — the module name `"batcher"` is batcher's own constant, not a parameter, since unlike `LoadConfig(baseDir, module)` there is no second module this entry point serves.

**`configengine.Load` semantics** (`internal/configengine/config.go`): requires `<baseDir>/_lyx` to exist (`FindBaseDir`), requires the config file to exist, then checks `yamlengine.MissingKeys(template, fileBytes)` — *missing* keys only, never extra ones — then resolves `${env:…}` via `yamlengine.Resolve`. An unknown key in a config file is not an error at any layer.

**`yamlengine.Reconcile`** (`internal/yamlengine/reconcile.go:21`) returns `(merged, added, removed, err)`. `merged` is built from the *template*'s node tree with existing values overlaid, so a key absent from the template is dropped from `merged` and simultaneously reported in `removed`. `configsync.ReconcileAll` surfaces that as `Result.Removed`, and `configcli.go:284` renders it into the reconcile JSON envelope. This is the machinery that makes the no-migration-path decision safe.

**`configreg` consumers** are registry-driven throughout (`configcli.go:42,70,99,154,246,319`, `menu.go:27`, `configsync.go:80`, `configcli_test.go:463,497`, `configcli_integration_test.go:39`). The only hardcoded module list is `internal/configreg/configreg_test.go:17`'s `want` slice, which pins both membership and alphabetical order. `batcher` sorts first, before `board`.

**`internal/lyxtest.SeedConfig`** takes an explicit `map[string]string` of module name to YAML content and commits the result, so adding a configreg module does not implicitly change any existing fixture. Only fixtures that need `batcher.yaml` on disk must add it.

**The two live call sites:**
- `internal/websterengine/runlevel.go:332` — `active, err := batcher.Select(deps.Config.Batcher)`, wrapped as `fmt.Errorf("webster: %w", err)`, immediately followed by `active.Batch(plan.Cards)` and the zero-batch refusal. Under this task's decision the `Select` call and its error wrap both go; `deps.Batcher.Batch(plan.Cards)` replaces `active.Batch(...)`. The zero-batch refusal stays exactly as-is.
- `internal/webstercli/cli.go:158`, inside `PersistentPreRunE` — resolves after `websterengine.LoadConfig` and stores the result on `c.batcher` (`:193`). `c.batcher` is consumed by `awaitbatch.go:62`, `recordbatch.go:75`, `beginbatch.go:63`, and `recoverbatch.go:85`, all unchanged. Only the resolution line changes, from `batcher.Select(websterCfg.Batcher)` to `batcher.Active(layout.AnchorPath())`. It can also move earlier in `PersistentPreRunE`, since it no longer depends on `websterCfg`.

**`RunDeps` plumbing** — `internal/webstercli/run.go:57` constructs `websterengine.RunDeps` field-by-field; add `Batcher: c.batcher`. `RunDeps.WorktreeRoot` is already `c.layout.AnchorPath()`, the same base dir `Active` would take, which is why the alternative looked cheap before the fixture evidence.

**Test sites that break by construction:**
- `internal/webstercli/verbs_test.go:684`'s `seedPersistentPreRunFixture` seeds `shuttle`/`reed`/`webster` only. Once `PersistentPreRunE` calls `Active`, it must also seed `batcher`. Its doc comment (`:677–683`) explains that webster.yaml's content is caller-supplied "so a test can override its batcher: key" — that rationale moves to the batcher config.
- `internal/webstercli/verbs_test.go:696–732`, the gate-test pair `TestPersistentPreRunE_UnknownBatcherFailsFast` and `TestPersistentPreRunE_DefaultBatcherResolves`. Both `strings.Replace` against `websterengine.ConfigTemplate()`'s `batcher: ""` literal, so both break the moment the key leaves that template. Taken whole, per the manifest — the pair moves to replacing `active: ""` in `batcher.ConfigTemplate()`, and the fixture helper's parameter becomes the batcher config rather than the webster config. Their *behaviour* is preserved: an unknown name still aborts before any verb's `RunE`, proven via the `status` verb, which never touches the batcher.
- `internal/webstercli/verbs_test.go:221–223` — a hand-built `*websterCLI` literal calls `batcher.Select("")` directly to stand in for what `PersistentPreRunE` would have resolved. `Select` still exists and still behaves this way, so this compiles untouched; only its explanatory comment (which names `c.batcher` and `PersistentPreRunE`) needs a light re-read for accuracy.
- `internal/websterengine/config_test.go:125` — `cfg.Batcher != "identity"` stops compiling with the field. It moves to `internal/batcher` as an `Active`-level assertion.

**Line-number drift to expect** — the manifest was written against an older tree. `docs/overview.md:267`/`:271` are now `:278` and `:282`; `internal/websterengine/doc.go:12` and `:25–27` are now the paragraph at `:23–25`. Re-derive by grep rather than trusting the manifest's numbers; the same applies to every line reference in this file.

## Constraints

From `CONSTRAINTS.md`:

- **Batcher Registry+Config Invariant** (`:331–336`) — this task's own invariant, and this task amends it. The invariant's substance survives intact: batching is still selected by `internal/batcher`'s name-keyed registry plus a config key with default `identity`, still with no plan-supplied batching and no batch grouping in the plan format. Only the file the key lives in changes. Both the ownership claim and the `webster.yaml` pin at `:335` must be updated in the same commit.
- **Cwd Resolution Invariant** — relevant and satisfied by construction: `batcher.Active` takes an already-resolved `baseDir` from the caller and never calls `lyxcwd` itself. `webstercli/cli.go` resolves cwd once via `lyxcwd.Getwd`/`lyxcwd.Resolve` and passes `layout.AnchorPath()`. `batcher` declares no path segment of its own — its config lands at `_lyx/config/batcher.yaml` via `configengine.ConfigFile`, exactly like every other module — so there is no per-module subdirectory to own.
- **CLI / Cobra Invariant** — does **not** apply. Nothing is registered on the cobra root; there is no `Command()`, no `Short`, no help-tree entry.
- **Sandbox Suite Coverage** — does **not** apply, and must not be satisfied. `cmd/lyx/sandbox_coverage_test.go:38–47` enumerates `newRoot().Commands()`, i.e. cobra registration, not `configreg`. Adding a `**Covers:** batcher` tag would fail that test's drift assert. Verified against the current file during exploration.
- **Documentation Lifecycle** — applies: this task changes observable config behaviour and a module's shape, so `docs/overview.md`, the affected reference docs, and `CONSTRAINTS.md` all land in the same commit as the code.

Discovered during discussion:

- `manifest/designs/loom.md:56` (producer table row 8) names `webster.yaml`'s `batcher:` key and is now falsified by this task — but it is explicitly task E's to write, per `shed-followups.md`'s Scope section. Do not edit it. Task E is sequenced after this one precisely so it can reflect what this task lands.

## Testing

Existing evidence that must keep passing untouched: `internal/batcher/batcher_test.go`, `registry_test.go`, and `identity_test.go`. They are the proof that only the configuration source moved and the batching itself did not. If any of them needs editing, something outside this task's scope has changed.

TDD candidates, in the order they are worth writing:

1. **`internal/batcher` — `Active` resolves from `batcher.yaml`.** The core new behaviour. Seed a worktree with `_lyx/config/batcher.yaml`, call `Active(baseDir)`, assert the returned `Batcher`'s `Name()`. Cover: the shipped-template default (`active: ""` → `identity`), an explicit `active: "identity"`, and an unknown name (error naming the bad value, reusing `Select`'s existing message). This absorbs the `cfg.Batcher == "identity"` assertion moving out of `internal/websterengine/config_test.go:125`.
2. **`internal/batcher` — absent config is a hard error.** `Active` against a worktree with an `_lyx/` tree but no `batcher.yaml` returns an error naming the file and pointing at `lyx config reconcile`. Guards the absent-config-is-a-hard-error decision against a later silent-default regression.
3. **`internal/configreg` — `batcher` is in the module list, in sort order.** Extend `configreg_test.go:17`'s existing `want` slice rather than adding a parallel test; that test already pins order as user-visible.
4. **`internal/webstercli` — the `PersistentPreRunE` gate still fails fast.** The relocated pair from `verbs_test.go:696–732`, driving `Command()`'s real `PersistentPreRunE` (never a hand-built `*websterCLI`), with the bad name now injected into `batcher.yaml`. Behaviour asserted is unchanged: exit 1, `"ok":false`, and the unknown-batcher message naming the bad key, proven via the `status` verb.

Not written, per the no-migration-path decision: any test asserting reconcile reports a leftover `webster.yaml` `batcher:` key.

Scenarios that must be covered somewhere, whatever the final test shape:

- `websterengine.Run` uses the injected `RunDeps.Batcher` and does no config I/O — implicitly proven by the 16 existing `TestRun_*` tests continuing to pass against a `WorktreeRoot` that has no `_lyx/` tree. If those tests start needing a seeded config, the runlevel-call-site decision has been implemented wrongly.
- `webstercli`'s four `c.batcher` consumers (`awaitbatch`, `recordbatch`, `beginbatch`, `recoverbatch`) are unaffected — existing tests cover them and must not need edits.

Acceptance: `go build ./...` and `go test ./...` pass. The config relocation is the only behavioural change.

## Q&A log

- **Q:** How does `websterengine.Run` get its batcher once `Config.Batcher` is gone? **A:** Initially "runlevel calls `batcher.Active` itself"; reversed to a `RunDeps.Batcher` field after exploration turned up concrete evidence (all 16 `TestRun_*` tests break; `Run` becomes the first engine entry point requiring a materialized `_lyx/` tree). The user's reasoning for reversing: this is new, concrete evidence that changes the calculation, not a one-off fixture cost to pay and forget.
- **Q:** What is the key inside `batcher.yaml`, and its default? **A:** `active: ""`.
- **Q:** How does reconcile report the leftover `webster.yaml` `batcher:` key? **A:** Initially "rely on the existing generic `Removed` path, add a test"; superseded — no test, no code. See the next entry.
- **Q:** Where does the migration test live? **A:** Nowhere. Pre-split worktrees do not exist, so that state does not need supporting.
- **Q:** "Hard error" on what, exactly? **A:** Only the absent-`batcher.yaml` case, which is `configengine.Load`'s standard behaviour. No strict unknown-key validation in `configengine` — the state it would catch does not exist.
- **Q:** File layout inside `internal/batcher`? **A:** Mirror every other config module — separate `template.yaml`, `template.go`, `config.go`.
- **Q:** Entry-point name, and what makes a batcher "active"? **A:** `Active`. The word is the package's own existing vocabulary (`registry.go:31`, `doc.go:12`, `webstercli/cli.go:54`,`:158`); it names the one registry entry this worktree's config selects, against the registered-but-idle rest.
