# Discussion: config degrades to embedded template

```yaml
task: config degrades to embedded template
slug: config-template-fallback
status: discussing
parent: standalone-producers
```

## Problem

Four producer config loaders — `shuttleengine.LoadConfig`, `reedengine.LoadConfig`, `perchengine.LoadConfigWithRegistry` and `websterengine.LoadConfig` — all route `configengine.Load`, which refuses outright on an absent `_lyx/` directory (`internal/configengine/config.go:52`, via `FindBaseDir`) and on an absent module config file (`internal/configengine/config.go:60`).
Each of the four then rewraps the first of those refusals as `not initialized here; run "lyx fabric reconcile"`.
The consequence is that a producer cannot run anywhere except inside a reconciled lyx hub, even though every value it needs is already sitting in its own embedded template — the same bytes `lyx config reconcile` would have written to disk.

This is T2 of the ten-task `producers-standalone` decomposition (see [manifest/designs/producers-standalone.md](../manifest/designs/producers-standalone.md), "Wave 1 — foundations", `slug: config-template-fallback`).
The design's goal is producers that run without a lyx worktree at all;
a mandatory config file is one of several gates standing in the way, and it is the one this task removes.
Wave 1 is three parallel tasks (T1, T2, T3) and this is the only one of them that touches these six files, so it can land independently of the other two.

## Scope

**In:**

- Add `configengine.LoadOrTemplate(baseDir, module string, template []byte) ([]byte, error)` — identical to `Load` except that its two refusal branches resolve the caller-supplied `template` instead of erroring.
- Refactor `configengine.Load` and `LoadOrTemplate` onto one shared unexported body so the two can never drift.
- Repoint exactly four loaders onto `LoadOrTemplate`: `shuttleengine.LoadConfig`, `reedengine.LoadConfig`, `perchengine.LoadConfigWithRegistry`, `websterengine.LoadConfig`.
- Delete the now-dead `not initialized` rewrap block in each of those four callers, and the `strings` import each one carries solely for it.
- Rewrite the four packages' file-header and `LoadConfig` doc comments, which currently promise an error on an absent `_lyx/`.
- Invert the four `TestLoadConfig_NotInitialized` tests to assert a template-derived config instead of an error, and rework the `TestLoadConfig_ModuleArgIsThreadedThrough` negative half in the three packages that have one.
- Add a **Config Strictness Invariant** to `CONSTRAINTS.md`, pinned by a new set-equality grep guard in `cmd/lyx`.
- Add a `LoadOrTemplate` section to `docs/shared-libs/configengine.md`.

**Out:**

- **`configengine.Load` itself is unchanged.** Its four remaining hub-scoped callers — `fabricengine`, `boardengine`, `loomengine`, `batcher` — keep the strict behaviour, where an absent config means a broken hub rather than a standalone invocation. Every existing `Load` test stays as-is.
- **`burlerengine.LoadConfig` is not touched.** It already bypasses `configengine.Load` entirely (strict top-level decode, because `MissingKeys` would misfire on its open-ended lenses/fans key set), so it already degrades and needs no change.
- **`modelspec.LoadRegistry` is not touched.** It never calls `FindBaseDir` and already falls back to `builtins()` on an absent file, which is the behavioural precedent this task follows.
- **No CLI file changes.** Every call site (`internal/burlercli/cli.go:78,96`, `internal/shuttlecli/cli.go:78,85`, `internal/webstercli/cli.go:137,144,151`, `internal/reedcli/cli.go:76`, `internal/perchcli/cli.go:99,106,125`) keeps passing `layout.AnchorPath()` and its literal module name. This task removes the config gate only — those CLIs still require `lyxcwd.Resolve` to succeed, and that gate is T5/T6/T7's work, not this task's. **A reviewer should not expect any CLI to become runnable outside a git repository as a result of this task.**
- **The strict-when-present half stays strict.** A config file that exists but is missing template keys still errors, including an empty or comments-only file. Only an *absent* `_lyx/` or an *absent* file degrades.
- **`manifest/roadmap.md` does not move.** Its "producers standalone: told-geometry foundations" entry (line 12) bundles T1 + T2 + T3; completing T2 alone does not complete that entry.
- **No new `configreg` entry, no `SeedOnly` flag change.** All four modules already appear in `internal/configreg`'s `Modules()` list with closed key sets, and reconcile's behaviour toward them is unaffected.

## Decisions

### shared-body-refactor

- Decision: extract a shared unexported `load(baseDir, module string, template []byte, fallbackOnAbsent bool) ([]byte, error)` in `internal/configengine/config.go`. `Load` calls it with `false`, `LoadOrTemplate` with `true`. The flag is consulted at exactly the two refusal branches (the `FindBaseDir` failure and the `os.IsNotExist` config-file read failure);
  every other step — `MissingKeys`, `envsource.Build`, `yamlengine.Resolve` — is shared verbatim.
- Rationale: one body means the strict and degrading paths cannot diverge on env resolution, key validation, or error wording. The two exported functions stay as the documented API surface.
- Rejected: **duplicating the body** — two ~40-line functions that must be hand-synced forever. **Wrapping `Load`** — `LoadOrTemplate` would have to re-detect *which* failure `Load` hit in order to decide whether to fall back, which means either matching on error strings or re-stat'ing the filesystem; both are fragile and the former couples the fallback decision to error prose.

### fallback-resolves-through-envsource

- Decision: the fallback path resolves the template through `envsource.Build(baseDir)` followed by `yamlengine.Resolve(template, env)` — the same two calls the on-disk path uses. It does **not** return the raw template bytes.
- Rationale: env overrides must keep working in standalone mode, because they are the only remaining way a config-less user pins machine-specific values — `LYX_REED_TMUX`, `LYX_REED_SHELL`, `LYX_SHUTTLE_CLAUDE`, `LYX_SHUTTLE_RUN_DIR`, `LYX_REED_DEBUG`, `LYX_REED_MOUSE`.
  This path cannot fail on a missing environment: `envsource.Build` tolerates both an absent `.env` and an absent `baseDir` (`readDotEnv` returns an empty map on `os.IsNotExist`), and all four templates use *only* the optional `${env:NAME:-default}` form, never the required `${env:NAME}` form that `yamlengine.expandScalar` errors on.
- Rejected: returning the template unresolved — it would leave literal `${env:...}` markers in the config structs, breaking every consumer.

### no-missingkeys-on-the-fallback-path

- Decision: the fallback path skips `yamlengine.MissingKeys` entirely.
- Rationale: the bytes being resolved *are* the template, so a template-versus-itself key comparison is vacuously satisfied. Running it would be dead work whose only possible outcome is a false failure if `MissingKeys` ever grows a non-reflexive edge case.
- Rejected: running it for symmetry.

### fallback-error-messages-name-the-template

- Decision: an error raised on the fallback path names the template, not a config-file path — e.g. `%s config template: %w` keyed on `module`. It must never interpolate `ConfigFile(baseDir, module)`, because on this path no such file exists.
- Rationale: the on-disk path's error prose ("config file `<path>`: …") is a lie on the fallback path and would send an operator looking for a file that was never there.
- Rejected: reusing the on-disk error wording unchanged.

### debug-level-observability

- Decision: `logger.Debug` at the fallback point inside `internal/configengine`, naming the module and which of the two conditions fired (absent `_lyx/` versus absent file). This adds `internal/logger` to `internal/configengine`'s import set.
- Rationale: "why did this run use defaults?" needs an answer available under `-v`, while staying silent by default. No invariant blocks the import — `internal/configengine` is not leaf-capped, unlike `internal/modelspec`, whose stdlib-plus-`configengine`-plus-yaml cap (Modelspec Leaf Invariant) is exactly why `LoadRegistry`'s own fallback is silent.
  `internal/logger` already depends on `internal/lyxcwd`, which is import-capped at stdlib plus `internal/gitexec`, so no cycle is introduced.
- Rejected: **silent** — matches `modelspec` precisely and costs no import, but leaves the degraded-config case undiagnosable. **`logger.Info`** — visible without `-v`, which is wrong here: standalone operation is intended to be the *normal* case for these four modules, so an Info line would print on essentially every producer run.

### module-threading-tests-become-positive-assertions

- Decision: in `shuttleengine`, `reedengine` and `perchengine`, the second half of `TestLoadConfig_ModuleArgIsThreadedThrough` — which today asserts that loading under the unseeded default module name **errors** — is replaced by a positive discrimination test. Seed the non-default module file with a value that differs from the template default, then assert that loading under the seeded name returns the seeded value *and* loading under the unseeded default name returns the template default.
- Rationale: the original intent is "the `module` argument genuinely selects the file path, it is not a hardcoded literal", and that intent survives the fallback intact — a hardcoded module name would now return the *seeded* value where the template default is expected, so the test still fails on the regression it was written to catch. The mechanism it used (absent file → error) is precisely what this task removes.
- Rejected: **deleting the negative half** — a hardcoded module name would then pass silently by falling back to the template. **Asserting against `configengine.Load` directly inside the engine's own test** — keeps a refusal assertion but exercises the wrong function, leaving the engine's own plumbing uncovered.

### config-strictness-invariant-with-grep-guard

- Decision: add a **Config Strictness Invariant** to `CONSTRAINTS.md` recording both caller sets and the rule that decides membership, enforced by a new set-equality grep guard under `cmd/lyx/`.
- Rationale: after this task `configengine` has two loading policies and nothing records which one a new caller should adopt. The whole point of the change is that all five producer-path loaders converge on one behaviour, and T7/T10 depend on that convergence holding — a strict caller silently added to a producer path would defeat them, and a degrading caller added to a hub path would turn a broken hub into a silently-defaulted one.
  The repo has strong precedent for exactly this guard shape: `cmd/lyx/ghguard_test.go`, `cmd/lyx/checkedcall_test.go`, `cmd/lyx/gitrepoboundary_test.go`, `cmd/lyx/boardguard_test.go`.
- Rejected: **review-obligation-only enforcement** — matches how the Producer Pointer-Rule and Batcher Registry+Config invariants are enforced, and is cheaper, but this split is machine-checkable and the failure mode is silent. **No invariant at all** — violates CLAUDE.md's "Record any new cross-cutting invariant there, same commit".

### membership-rule-is-what-the-config-governs

- Decision: the invariant's membership rule is stated as *what the config governs*, not *which package declares it*. A config whose keys are operator-tunable producer knobs (model specs, timeouts, tool paths, poll intervals) degrades; a config that describes hub state, whose absence means the hub itself is broken, stays strict.
- Rationale: this is the distinction the design doc already uses to justify `websterengine`'s placement in the degrading group over an earlier draft that had put it with the hub-scoped callers, and the same distinction that already keeps `burlerengine` off the strict list. Stating it by package name instead would make the invariant a list with no rule, useless for classifying the next caller.
- Rejected: enumerating packages with no stated rule.

## Technical context

**`internal/configengine/config.go`** is small and self-contained: `FindBaseDir`, `ConfigDir`, `ConfigFile`, `Load`.
Current imports are `fmt`, `os`, `path/filepath`, `internal/envsource`, `internal/lyxdirs`, `internal/yamlengine`.
`Load`'s body is a linear six-step flow — `FindBaseDir`, `os.ReadFile`, `MissingKeys`, `envsource.Build`, `yamlengine.Resolve`, return — with the two branches this task changes at lines 52-55 and 58-61.
The package also has `set.go` (`Set`) and `edit.go` (`Edit`, `DefaultEditor`), both of which call `FindBaseDir` and are **out of scope** — they are write paths, where an absent `_lyx/` genuinely is an error.

**The four callers all share one shape.** Each is `configengine.Load(baseDir, module, []byte(ConfigTemplate()))`, followed by an `if err != nil` block containing a `strings.Contains(err.Error(), "not initialized")` test that rewraps to `not initialized here; run "lyx fabric reconcile"`, then a yaml unmarshal:

- `internal/shuttleengine/config.go:38` — plain `yaml.Unmarshal` into `Config`.
- `internal/reedengine/config.go:42` — plain `yaml.Unmarshal`; `Config` has a nested `Header HeaderConfig`.
- `internal/perchengine/config.go:58` (`LoadConfigWithRegistry`) — strict `decoder.KnownFields(true)` decode, then `ResolveModelSpec` against the registry. Note `perchengine.LoadConfig` at line 90 wraps it, first calling `modelspec.LoadRegistry(baseDir)` — which already degrades — so repointing `LoadConfigWithRegistry` alone covers both entry points. `internal/perchcli/cli.go:125` calls `LoadConfigWithRegistry` directly with a separately-loaded registry.
- `internal/websterengine/config.go:53` — plain `yaml.Unmarshal`, then a `modelspec.Parse` grammar check over the `master` and `recovery` role specs.

In **all four files** `strings` is imported for the rewrap and nothing else, so all four lose the import.
`fmt` stays in all four (used by the unmarshal-error wrap and, in perch/webster, by the model-spec error wraps).

**Templates are `//go:embed`-backed** via a `ConfigTemplate() string` accessor per package: `internal/{shuttleengine,perchengine,websterengine}/template.go` embed `template.yaml`;
`internal/reedengine/template.go` is a bare accessor over a `configTemplate` var supplied by GOOS-selected `template_windows.go` / `template_posix.go` (embedding `template_windows.yaml` / `template_posix.yaml`).
**A reed fallback test therefore asserts against GOOS-varying defaults** — `tmux`/`bash` on POSIX versus `tmux`/`pwsh` on Windows — so it must assert on a GOOS-invariant key (`width: 220`, `collapsed_strip_rows: 3`, `header.height_rows: 1`) or branch on `runtime.GOOS`.

**Template defaults available as fallback assertion targets:**

| Module | GOOS-invariant keys |
|---|---|
| shuttle | `poll_interval_ms: 500`, `liveness_every_n_polls: 10`, `run_timeout_min: 30`, `startup_timeout_s: 90`, `claude_deny_agent_tool: true`, `claude_deny_ask_user_question: true`; `run_dir`/`claude` resolve to `""` |
| reed | `width: 220`, `height: 50`, `collapsed_strip_rows: 3`, `min_full_rows: 3`, `strand_name: '<ROLE>:<ROUND>:<SHORT_GUID>'`, `debug_log: 0`, `mouse: off`, `header.height_rows: 1`, `header.template: ""` |
| perch | `judge_model: haiku`, `round_caps: [5, 8, 10]` |
| webster | `master: sonnet`, `recovery: opus[effort=high]`, `self_fix_cap: 2`, `master_timeout_min: 480`, `recovery_timeout_min: 60`, `poll_wait_s: 480` |

**Perch's fallback runs `ResolveModelSpec` on the template default.** `judge_model: haiku` must resolve through whichever registry the caller supplied. Under `perchengine.LoadConfig` that registry is `modelspec.LoadRegistry`'s `builtins()`, and the design doc notes `haiku` is a built-in, default-free alias — so `JudgeEffort` resolves to `""`, matching an existing test's expectation. The fallback must not skip the model-spec resolution step.

**Webster's fallback runs `modelspec.Parse` on `master: sonnet` and `recovery: opus[effort=high]`**, both of which must parse. This is a real assertion the new test gets for free.

**Existing test infrastructure to reuse.** Each of the four engine test packages already has a `seedLyxConfig(t, tmpDir, module, content)` helper that mkdirs `_lyx/config/` and writes `<module>.yaml` — see `internal/shuttleengine/config_test.go:20`. The fallback tests need *no* helper at all: a bare `t.TempDir()` with nothing in it is the whole fixture. All four test packages are `package <name>_test` (external test package).

**Tests that must change, by file:**

- `internal/shuttleengine/config_test.go` — `TestLoadConfig_NotInitialized` (line 109) inverted; `TestLoadConfig_ModuleArgIsThreadedThrough` (line 88) negative half at lines 102-106 reworked.
- `internal/reedengine/config_test.go` — `TestLoadConfig_NotInitialized` (line 112) inverted; `TestLoadConfig_ModuleArgIsThreadedThrough` (line 92) negative half reworked.
- `internal/perchengine/config_test.go` — `TestLoadConfig_NotInitialized` (line 184) inverted; `TestLoadConfig_ModuleArgIsThreadedThrough` (line 164) negative half reworked.
- `internal/websterengine/config_test.go` — `TestLoadConfig_NotInitialized` (line 150) inverted. This package has **no** `ModuleArgIsThreadedThrough` test, so only one change here.
- `internal/configengine/config_test.go` — new `LoadOrTemplate` tests added; the existing `TestLoad_*` and `TestFindBaseDir_*` tests are untouched, including `TestLoad_NotInitialized` (line 251) and `TestLoad_AbsentFile` (line 104), which now pin that `Load` did *not* change.

**Tests that do NOT change, verified during exploration:**

- `internal/webstercli/cli_test.go:275` `TestStatusCmd_NotInitialized` — about an absent `state.json`, not config.
- `internal/reedcli/cli_integration_test.go:44` — asserts the error is *not* a config-resolution error; the fallback makes this strictly more true.
- `internal/batcher/config_test.go:129`, `internal/fabricengine/config_test.go:125` — strict-group callers, deliberately unaffected.

**The new `cmd/lyx` guard must allowlist itself for tier purity.** If it resolves its scan root via `exec.Command("go", "env", "GOMOD")` — the pattern `ghguard_test.go`, `checkedcall_test.go`, `boardguard_test.go` and `gitrepoboundary_test.go` all use — it trips the Test Tier Purity Invariant's ban on `exec.Command` in untagged test files.
It must therefore add its own module-relative path to `allowedSpawners` in `cmd/lyx/tierpurity_test.go` (the map at line 24), with a one-line justification matching the style of the existing entries.
**This makes `cmd/lyx/tierpurity_test.go` a required file edit, easily missed.**

**Guard mechanism.** Set-equality over grep results, following `gitrepoboundary_test.go`'s pinned-set shape: walk non-test `*.go` under the module root, collect every package directory containing a `configengine.Load(` call and every one containing a `configengine.LoadOrTemplate(` call, and compare each against its pinned set — strict `{fabricengine, boardengine, loomengine, batcher}`, degrading `{shuttleengine, reedengine, perchengine, websterengine}`.
Exclude `internal/configengine` itself (declaration site) and skip `_test.go` files.
Known blind spot to state in the invariant text, matching how the existing guards state theirs: a substring scan cannot see a call reached through an alias or a function value.

**Doc surface.** `docs/shared-libs/configengine.md` documents `Load`'s six-step flow (lines 30-41), its error cases (lines 138-149), and — at lines 21-22 and 127-131 — states outright that `configengine` errors when `_lyx/` is absent and that typed wrappers rewrap on the `"not initialized"` substring.
All of those claims become partly false and must be updated, not merely appended to.
There is **no** `manifest/designs/configengine.md`; the package's design surface is this shared-libs doc plus its package comment.

## Constraints

From `CONSTRAINTS.md`:

- **Cwd Resolution Invariant** — `LoadOrTemplate` takes a `baseDir string` exactly as `Load` does, and must never accept or construct a `*lyxcwd.Location`. Geometry is structural, never config-overridable, so the fallback must not invent a path from config. `internal/configengine` remains the single declarer of the `_lyx/config` path shape via `ConfigDir`/`ConfigFile`.
- **Lyxdirs Single-Declarer Invariant** — any new path construction uses `lyxdirs.LyxDirName`, never a `"_lyx"` literal. Enforced by `internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_GeometryLiterals`, which matches string literals in path-construction context.
- **Modelspec Leaf Invariant** — `internal/modelspec` production imports are capped at stdlib, `internal/configengine`, and `gopkg.in/yaml.v3`. This task must not push anything new into `modelspec`, and is the reason `modelspec`'s own fallback cannot log.
- **Test Tier Purity Invariant** — every new test is untagged Tier 1: no `git init`, no `exec.Command`, no `hubforge.NewHub`, no `gitkit.Copy*`, no `time.Sleep` ≥ 1s. All four fallback tests are pure `t.TempDir()` and satisfy this naturally. The new `cmd/lyx` guard does *not*, and must allowlist itself in `cmd/lyx/tierpurity_test.go` as described above.
- **Hermetic Git Test Environment Invariant** — not triggered: no new test spawns git, so no new `TestMain` is required.
- **Documentation Lifecycle** — see [docs/overview.md#documentation-lifecycle](../docs/overview.md#documentation-lifecycle).
- **New: Config Strictness Invariant** — added by this task, per the `config-strictness-invariant-with-grep-guard` decision above.

From `CLAUDE.md`:

- Docs land in the same commit as the change: `docs/shared-libs/configengine.md` and `CONSTRAINTS.md` are part of this task's single commit, not a follow-up.
- `manifest/roadmap.md` moves only on completing a planned item — this task does not complete its roadmap entry (which bundles T1+T2+T3), so roadmap stays untouched.
- Markdown uses semantic line breaks: one sentence per line, plus a break at internal independent-clause boundaries. No fixed-column hard wrap. Table cells stay on one line.
- Go comments follow the `golang:golang-comments` skill's godoc rules.

Behavioural constraint discovered during exploration:

- **`envsource.Build` must be called with the same `baseDir` on the fallback path**, not skipped and not passed a different directory. A standalone user may keep a `.env` beside the working directory even with no `_lyx/`, and `readDotEnv` already returns an empty map when that file is absent.

## Testing

**`internal/configengine` — TDD candidate, and the primary one.** `LoadOrTemplate` is a new pure function over a temp directory with no fixtures, so its tests can be written before the implementation. Scenarios:

- Absent `_lyx/` → returns template-derived bytes, no error.
- `_lyx/` present, config file absent → returns template-derived bytes, no error.
- `_lyx/` and config file both present → identical result to `Load` on the same inputs (the fallback must not shadow a real file).
- Config file present but missing a template key → still errors. This is the strict-when-present boundary, and it is the single most important negative test in the task.
- Config file present but empty or comments-only → still errors, for the same reason.
- Fallback path honours env overrides: set an env var referenced by the template's `${env:NAME:-default}` marker, assert the override lands in the returned bytes with no `_lyx/` anywhere on disk.
- Fallback path with an absent `.env` and an absent `baseDir` → no error.
- `Load` regression: the existing `TestLoad_NotInitialized` and `TestLoad_AbsentFile` must keep passing unmodified, which is what proves the shared-body refactor preserved strict behaviour.

**Per-engine loader tests — one new or inverted test per loader, four total.** This is the check T2 names explicitly ("a new test per loader asserting a `baseDir` with no `_lyx/` returns the template-derived config rather than an error"). Each replaces that package's `TestLoadConfig_NotInitialized`:

- `shuttleengine` — bare `t.TempDir()`, assert no error and that `Config` carries template defaults (`PollIntervalMS == 500`, `RunTimeoutMin == 30`, `StartupTimeoutS == 90`, `ClaudeDenyAgentTool == true`).
- `reedengine` — bare `t.TempDir()`, assert no error and **GOOS-invariant** defaults only (`Width == 220`, `CollapsedStripRows == 3`, `Header.HeightRows == 1`). Do not assert on `Tmux`/`Shell`, which differ by GOOS.
- `perchengine` — bare `t.TempDir()` through `LoadConfig` (so the registry also falls back to `builtins()`), assert no error, `JudgeModel` resolved from `haiku`, and `RoundCaps == [5, 8, 10]`. This test covers the fallback *and* the model-spec resolution running on top of it.
- `websterengine` — bare `t.TempDir()`, assert no error, `Master == "sonnet"`, `SelfFixCap == 2`, `MasterTimeoutMin == 480`. The `modelspec.Parse` grammar check runs over the template defaults as a side effect, which is desirable.

Name these for what they now assert, not for the removed refusal — e.g. `TestLoadConfig_UninitializedFallsBackToTemplate`.

**Module-threading tests in `shuttleengine`, `reedengine`, `perchengine` — three reworks.** Each seeds the non-default module name with one key set to a non-default value, then makes two assertions: the seeded name returns the seeded value, and the unseeded default name returns the template default. Both halves are needed — the first alone does not catch a hardcoded module name, and the second alone does not prove the file is read.

**`cmd/lyx` guard.** A table-free walk-and-compare test. Its own failure output must name the offending package and which set it was expected to be in, following the existing guards' message style. Verify it actually fails by temporarily flipping one caller during development.

**Verification commands.**

- Task-specific: `go test ./internal/configengine/... ./internal/shuttleengine/... ./internal/reedengine/... ./internal/perchengine/... ./internal/websterengine/... ./cmd/lyx/...` — the `cmd/lyx/...` leg is not in T2's original list and is required here because of the new guard and the `tierpurity_test.go` edit.
- Baseline: `go test ./...` from the worktree root.
- Structural: confirm `internal/configengine` production code contains exactly two exported load entry points and one shared body;
  confirm `strings` is gone from all four callers' import sets;
  confirm no remaining production `configengine.Load(` call sites outside the four strict packages.

## Q&A log

- **Q:** How should `LoadOrTemplate` share code with `Load` — shared unexported body with a fallback flag, duplicated body, or wrap `Load` and re-detect the failure? **A:** Shared unexported body with a fallback flag. Wrapping `Load` was rejected because re-detecting which refusal fired requires error-string matching or a re-stat.
- **Q:** Should the fallback resolve the template through `envsource.Build`/`yamlengine.Resolve`, or return it raw? **A:** Resolve it. Env overrides are the only way a config-less user pins machine tool paths, and no template uses the required `${env:NAME}` form, so the path cannot fail on a missing variable.
- **Q:** Should the fallback be observable or silent? **A:** `logger.Debug` inside `configengine`, accepting a new `internal/logger` import. Silent was rejected as undiagnosable; `logger.Info` as too loud, since standalone is meant to be the normal case for these four.
- **Q:** How should the three `TestLoadConfig_ModuleArgIsThreadedThrough` negative halves be preserved once the absent-file refusal they rely on is gone? **A:** Convert to a positive discrimination test — seed the other module with a non-default value, assert the unseeded default name yields the template default. Deleting the half was rejected: a hardcoded module name would then pass silently.
- **Q:** Does this need a new `CONSTRAINTS.md` invariant, and should it be machine-enforced? **A:** Yes to both — a Config Strictness Invariant with a set-equality grep guard in `cmd/lyx`, following the `ghguard_test.go` / `gitrepoboundary_test.go` precedent. Review-obligation-only enforcement was rejected because the failure mode is silent and the split is machine-checkable.
- **Q:** How should the invariant state which set a new caller joins? **A:** By what the config governs — operator-tunable producer knobs degrade, hub state stays strict — not by an unexplained package list. This is the same distinction that places `websterengine` in the degrading group and already keeps `burlerengine` off the strict list.
- **Q:** What happens to the four `TestLoadConfig_NotInitialized` tests? **A:** Inverted and renamed to assert a template-derived config rather than an error.
- **Q:** What happens to the dead `not initialized` rewrap blocks? **A:** Deleted in all four callers, along with the `strings` import each carries solely for them.
- **Q:** Does a config file that exists but is broken — missing keys, empty, comments-only — fall back? **A:** No. It still errors. Only an absent `_lyx/` or an absent file degrades.
- **Q:** Which docs must land in this commit? **A:** `docs/shared-libs/configengine.md` (a `LoadOrTemplate` section, plus corrections to its existing claims at lines 21-22 and 127-131) and `CONSTRAINTS.md` (the new invariant). Neither is in T2's original Files list. `manifest/roadmap.md` stays untouched, because its entry bundles T1+T2+T3.
