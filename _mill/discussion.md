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

- Add `configengine.LoadOrTemplate(baseDir, module string, template []byte) ([]byte, error)` — identical to `Load` except that its two refusal branches resolve the caller-supplied `template` instead of erroring, and only on proven absence.
- Add an exported `configengine.ErrNotInitialized` sentinel, wrapped by `FindBaseDir` on its `os.IsNotExist` branch, so absence is distinguishable from a stat failure without matching error prose.
- Refactor `configengine.Load` and `LoadOrTemplate` onto one shared unexported body so the two can never drift.
- Rewrite `internal/configengine/config.go`'s own file-header comment, which today opens "implements strict YAML configuration loading" and names three wrappers (`board.LoadConfig`, `worktree.LoadConfig`, `fabric.LoadConfig`) that no longer describe the caller set.
- Repoint exactly four loaders onto `LoadOrTemplate`: `shuttleengine.LoadConfig`, `reedengine.LoadConfig`, `perchengine.LoadConfigWithRegistry`, `websterengine.LoadConfig`.
- Delete the now-dead `not initialized` rewrap block in each of those four callers, and the `strings` import each one carries solely for it.
- Rewrite the four packages' file-header and `LoadConfig` doc comments, which currently promise an error on an absent `_lyx/`.
- Invert the four `TestLoadConfig_NotInitialized` tests to assert a template-derived config instead of an error, and rework the `TestLoadConfig_ModuleArgIsThreadedThrough` negative half in the three packages that have one.
- Add a **Config Strictness Invariant** to `CONSTRAINTS.md`, enforced by review obligation, naming the future guard as a candidate.
- Add a `LoadOrTemplate` section to `docs/shared-libs/configengine.md`.

**Out:**

- **`configengine.Load`'s observable behaviour is unchanged.** Its four remaining hub-scoped callers — `fabricengine`, `boardengine`, `loomengine`, `batcher` — keep the strict behaviour, where an absent config means a broken hub rather than a standalone invocation. Every existing `Load` test stays as-is and passing, which is what proves it. The one internal change is that `FindBaseDir`'s absent-`_lyx/` error now wraps `ErrNotInitialized`; its message text is unchanged, so the four strict callers' `strings.Contains` rewraps keep working untouched.
- **The four strict callers are not migrated onto `errors.Is`.** They keep their `strings.Contains(err.Error(), "not initialized")` rewraps. The new sentinel makes that migration possible, and the invariant text notes it as available, but doing it here would touch four packages this task has no other reason to open.
- **The three own-loader modules are not touched:** `burlerengine.LoadConfig`, `modelspec.LoadRegistry`, `scoutengine.LoadRegistry`. Each resolves its path with `configengine.ConfigFile` and reads the file itself with its own absent-file fallback, so each already degrades. `modelspec.LoadRegistry`'s `builtins()` fallback is the behavioural precedent this task follows. They are named in the new invariant as a class explicitly outside its guard's subject — see `membership-rule-is-a-standalone-entry-point`.
- **No CLI file changes.** Every call site (`internal/burlercli/cli.go:78,96`, `internal/shuttlecli/cli.go:78,85`, `internal/webstercli/cli.go:137,144,151`, `internal/reedcli/cli.go:76`, `internal/perchcli/cli.go:99,106,125`) keeps passing `layout.AnchorPath()` and its literal module name. This task removes the config gate only — those CLIs still require `lyxcwd.Resolve` to succeed, and that gate is T5/T6/T7's work, not this task's. **A reviewer should not expect any CLI to become runnable outside a git repository as a result of this task.**
- **The strict-when-present half stays strict.** A config file that exists but is missing template keys still errors, including an empty or comments-only file. Only an *absent* `_lyx/` or an *absent* file degrades.
- **`manifest/roadmap.md` does not move.** Its "producers standalone: told-geometry foundations" entry (line 12) bundles T1 + T2 + T3; completing T2 alone does not complete that entry.
- **No new `configreg` entry, no `SeedOnly` flag change.** All four modules already appear in `internal/configreg`'s `Modules()` list with closed key sets, and reconcile's behaviour toward them is unaffected.
- **No machine-enforced guard for the new invariant, and therefore no `cmd/lyx` files at all.** The Config Strictness Invariant lands here as text with review-obligation enforcement; the grep guard that would pin its two caller sets is deferred to T10. No new file under `cmd/lyx/`, no edit to `cmd/lyx/tierpurity_test.go`, and no `cmd/lyx/...` leg in this task's verify command. See the `config-strictness-invariant-text-now-guard-at-t10` decision below.

## Decisions

### shared-body-refactor

- Decision: extract a shared unexported `load(baseDir, module string, template []byte, fallbackOnAbsent bool) ([]byte, error)` in `internal/configengine/config.go`. `Load` calls it with `false`, `LoadOrTemplate` with `true`.
  **Concrete shape, stated so the two paths' actual divergence is unambiguous.** The body has one on-disk path and one fallback tail:
  1. The flag is consulted at exactly two points — `FindBaseDir`'s absence branch and the config-file read's `os.IsNotExist` branch — and at each one **only on proven absence** (see `fallback-only-on-proven-absence`). Any other failure at either point returns unchanged, on both paths.
  2. With the flag set, each of those two branches returns through **one shared fallback tail**, which resolves the template: `envsource.Build`, then `yamlengine.Resolve`, then return. That tail **skips `MissingKeys`** (see `no-missingkeys-on-the-fallback-path`) and **wraps its own failures as `%s config template: %w`** keyed on `module`, never with the on-disk path's `config file %s:` prose (see `fallback-error-messages-name-the-template`).
  3. The on-disk path — file present — is byte-for-byte the current `Load`: `MissingKeys`, `envsource.Build`, `yamlengine.Resolve`, all with the existing `config file %s:` error wording, reached identically whichever exported function was called.
- Rationale: the sharing that matters is the **on-disk path** — a config file that exists is validated, env-resolved and error-reported identically no matter which entry point read it, which is what stops the strict and degrading callers from drifting apart on the case they have in common. The fallback tail is deliberately *not* shared with it: it has no file to validate against and no path to name, so `MissingKeys` and the file-keyed prose are wrong there by construction rather than by choice.
  What both paths do share is the env-marker resolution semantics — the same `envsource.Build`/`yamlengine.Resolve` pair, in that order — so a template default and an on-disk value expand by identical rules.
  The two exported functions stay as the documented API surface.
- Note: an earlier draft of this decision said "every other step — `MissingKeys`, `envsource.Build`, `yamlengine.Resolve` — is shared verbatim", which contradicted the two decisions below; round 4 of review caught it. The step list above supersedes it.
- Rejected: **duplicating the body** — two ~40-line functions that must be hand-synced forever. **Wrapping `Load`** — `LoadOrTemplate` would have to re-detect *which* failure `Load` hit in order to decide whether to fall back, which means either matching on error strings or re-stat'ing the filesystem; both are fragile and the former couples the fallback decision to error prose.

### fallback-only-on-proven-absence

- Decision: `LoadOrTemplate` falls back **only when the thing is provably absent**, never on any other failure of the same step. Absence is distinguished by a typed sentinel, not by error prose:
  - Add an exported `configengine.ErrNotInitialized` sentinel. `FindBaseDir` wraps it with `%w` on its `os.IsNotExist` branch (`internal/configengine/config.go:30-31`) and continues to return its own unwrapped `stat %s: %w` error on any other stat failure.
  - The degrading path falls back only when `errors.Is(err, ErrNotInitialized)` holds, and propagates anything else unchanged.
  - The config-file branch already has a clean discriminator — `os.IsNotExist(err)` at `config.go:59` — and keeps it. A read failure that is not `IsNotExist` (permission, IO) propagates.
- Rationale: `FindBaseDir` returns `stat _lyx: %w` for a permission or IO error with no sentinel, so falling back on "the `FindBaseDir` failure" as a whole would turn a genuinely broken `_lyx` into a silent set of defaults — the exact failure this task must not introduce, since a producer would then run with template values against a hub it could not read.
  The sentinel is worth its small widening of the change for a second reason: it retires the `strings.Contains(err.Error(), "not initialized")` seam, which `docs/shared-libs/configengine.md:127-131` already documents as a substring match callers depend on. The four degrading callers delete their copy of it in this task anyway;
  the four strict callers keep theirs, and this task does **not** migrate them — that is a follow-up, and the invariant text notes it as available rather than done.
- Rejected: **inline `os.Stat` + `os.IsNotExist` inside the shared body** — narrower, touching no exported surface, but it duplicates the stat `FindBaseDir` already performs and leaves the string-match seam standing. **Falling back on any `FindBaseDir` error** — silently converts an unreadable `_lyx` into defaults.

### fallback-resolves-through-envsource

- Decision: the fallback path resolves the template through `envsource.Build(baseDir)` followed by `yamlengine.Resolve(template, env)` — the same two calls the on-disk path uses. It does **not** return the raw template bytes.
- Rationale: env overrides must keep working in standalone mode, because they are the only remaining way a config-less user pins machine-specific values. **Two of the four templates carry env markers at all** — `internal/shuttleengine/template.yaml` (`LYX_SHUTTLE_RUN_DIR`, `LYX_SHUTTLE_CLAUDE`) and `internal/reedengine/template_{posix,windows}.yaml` (`LYX_REED_TMUX`, `LYX_REED_SHELL`, `LYX_REED_DEBUG`, `LYX_REED_MOUSE`);
  `internal/perchengine/template.yaml` and `internal/websterengine/template.yaml` carry none, so for those two the resolve step is a no-op that must still run for uniformity rather than for effect.
  This path cannot fail on a missing environment: `envsource.Build` tolerates both an absent `.env` and an absent `baseDir` (`readDotEnv` returns an empty map on `os.IsNotExist`), and the two templates that *do* use markers use only the optional `${env:NAME:-default}` form, never the required `${env:NAME}` form that `yamlengine.expandScalar` errors on.
- Rejected: returning the template unresolved — it would leave literal `${env:...}` markers in the config structs, breaking every consumer.

### no-missingkeys-on-the-fallback-path

- Decision: the fallback path skips `yamlengine.MissingKeys` entirely.
- Rationale: the bytes being resolved *are* the template, so a template-versus-itself key comparison is vacuously satisfied. Running it would be dead work whose only possible outcome is a false failure if `MissingKeys` ever grows a non-reflexive edge case.
- Rejected: running it for symmetry.

### fallback-error-messages-name-the-template

- Decision: an error raised on the fallback path names the template, not a config-file path — e.g. `%s config template: %w` keyed on `module`. It must never interpolate `ConfigFile(baseDir, module)`, because on this path no such file exists.
- Rationale: the on-disk path's error prose ("config file `<path>`: …") is a lie on the fallback path and would send an operator looking for a file that was never there.
- Rejected: reusing the on-disk error wording unchanged.

### info-level-observability

- Decision: `logger.Info` at the fallback point inside `internal/configengine`, naming the module and which of the two conditions fired (absent `_lyx/` versus absent config file). This adds `internal/logger` to `internal/configengine`'s import set.
- Rationale: stated against the levels' real semantics, which are not what a reading of "Debug is the quiet one" would suggest:
  - `logger.SetVerbosity` (`internal/logger/logger.go:379-385`) maps `count <= 0` to Warn, `count == 1` to Info, `count >= 2` to Debug. So **Info is silent on stderr at the default threshold** and appears at `-v`; Debug needs `-vv`.
  - `durableHandler`'s `Enabled` is unconditional at Info and above (`internal/logger/logger.go:281-285`), never gated by `levelVar`. So **an Info record reaches the durable trace-file sink at any verbosity — whenever the sink is armed at all** — while **a Debug record never reaches it under any condition**.
  - **The sink's arming condition, stated because it bites here.** `ensureDurableSink` (`internal/logger/sink.go:75-103`) disarms itself (`sinkOK = false`) in three cases: under `go test` without `LYX_TRACE=1`; when `lyxcwd.Getwd()` fails; and when `lyxcwd.Resolve(cwd)` fails. The third is exactly a config-less invocation standing outside an anchor. So durable capture requires a resolvable `AnchorPath()`, and the claim is **not** "always".
  Info therefore gives the wanted shape wherever it can be had: nothing on a normal run's stderr, and a durable record of which config a run actually used. "Why did this run use defaults?" is a question asked *after* the run, from the trace file — which is precisely where Debug would never be.
- **Accepted limitation, not an oversight.** In true standalone mode — outside any anchor, which is the mode T5-T8 build toward — the sink is disarmed, so the fallback line reaches neither stderr (default threshold is Warn) nor any file. That is accepted here rather than solved, for two reasons: no log level rescues it, since Debug is disarmed in that case *and* excluded from the sink unconditionally, so the choice between levels is a wash there and Info still wins everywhere else; and the fix is not this task's — it is anchor-independent log-sink geometry, which belongs with the told-geometry work in T3/T5 (`reedengine`'s `LogsDir` becomes a told value in T3 for the same underlying reason). **Recorded as a watch item for T5-T8:** whichever task first makes a producer genuinely runnable outside an anchor should decide where that run's trace lands, and this fallback line is one of its consumers.
  The case the record *does* cover is the hub one — an anchor resolves but `_lyx/config/<module>.yaml` is missing, or `_lyx/` itself is — which is also where a degraded config is most surprising and most worth a durable trace.
- **No test asserts the log line.** Under `go test` the sink is disarmed unless `LYX_TRACE=1`, and stderr is above the default threshold, so a test asserting on the fallback's observability would be asserting on the harness's env rather than on the code. The fallback's *behaviour* is fully tested (see Testing); its logging is not.
  No invariant blocks the import, on two separate grounds that must both be checked:
  - **No cycle.** `internal/configengine` is not itself leaf-capped, and `internal/logger` imports only `internal/lyxcwd`, `internal/lyxdirs` and `internal/proc` — none of which reaches `internal/configengine`. (`internal/modelspec`'s stdlib-plus-`configengine`-plus-yaml cap, the Modelspec Leaf Invariant, is exactly why `LoadRegistry`'s own fallback must stay silent.)
  - **No allowlisted importer is tripped.** Two allowlist-capped packages import `internal/configengine` — `internal/modelspec` (Modelspec Leaf Invariant) and `internal/gitkit` (gitkit Leaf Invariant) — so widening `configengine`'s imports widens their transitive closure with `logger` and `proc`. Neither invariant is violated, because both enforcing tests are **direct-import** allowlists, not transitive ones: `internal/modelspec/leaf_enforcement_test.go` and `internal/gitkit/leaf_enforcement_test.go` each walk their own package's files with `parser.ParseFile(..., parser.ImportsOnly)` and check only the import declarations they find there. This is stated rather than left to be derived, since a transitive reading would make the import look like a violation.
- Rejected: **`logger.Debug`** — reaches stderr only at `-vv` and never reaches the durable sink, so the one place the answer would be looked for is the one place it would not be. An earlier draft of this decision chose Debug on the false premises that it surfaces at `-v` and that Info prints by default; both are contradicted by the source cited above. **Silent** — matches `modelspec` precisely and costs no import, but leaves the degraded-config case with no record anywhere.

### module-threading-tests-become-positive-assertions

- Decision: in `shuttleengine`, `reedengine` and `perchengine`, the second half of `TestLoadConfig_ModuleArgIsThreadedThrough` — which today asserts that loading under the unseeded default module name **errors** — is replaced by a positive discrimination test. Seed the non-default module file with a value that differs from the template default, then assert that loading under the seeded name returns the seeded value *and* loading under the unseeded default name returns the template default.
- Rationale: the original intent is "the `module` argument genuinely selects the file path, it is not a hardcoded literal", and that intent survives the fallback intact — a hardcoded module name would now return the *seeded* value where the template default is expected, so the test still fails on the regression it was written to catch. The mechanism it used (absent file → error) is precisely what this task removes.
- Rejected: **deleting the negative half** — a hardcoded module name would then pass silently by falling back to the template. **Asserting against `configengine.Load` directly inside the engine's own test** — keeps a refusal assertion but exercises the wrong function, leaving the engine's own plumbing uncovered.

### config-strictness-invariant-text-now-guard-at-t10

- Decision: add a **Config Strictness Invariant** to `CONSTRAINTS.md` recording both caller sets and the rule that decides membership, **enforced by review obligation**. Its `Enforced by` line names a set-equality grep guard as a candidate and points at T10 as its home. This task builds no guard and touches no file under `cmd/lyx/`.
- Rationale: after this task `configengine` has two loading policies and nothing records which one a new caller should adopt, so the text is required — CLAUDE.md mandates recording a new cross-cutting invariant in the same commit. Enforcement is a separate question, and three things settle it against building the guard here:
  1. **T1, this wave's sibling task, made the opposite call on the same grounds.** `planparser-plan-dir` explicitly considered and rejected machine-enforcing its own reworded invariant, reasoning that building one is scope the design did not ask for. Two tasks in one wave independently reaching opposite defaults on "should a reworded/new invariant get a guard" is worse than either answer applied consistently.
  2. **`producers-standalone.md` places its new-invariant work at T10**, the consolidation task, which introduces the three-tier geometry/fabric/orchestrator-state rule. A guard themed with cross-cutting caller-set discipline belongs alongside it, not bolted onto a six-file loader change in wave 1.
  3. **It closes a real contention gap.** The design's file-contention analysis enumerates every file shared across all ten tasks, and it never anticipated a task adding a *new* guard file — so a `cmd/lyx` guard here would silently invalidate that analysis. Deferring keeps the analysis true and keeps this task's file set inside its decomposed brief.
  Review-obligation enforcement is well-precedented for exactly this kind of policy statement: the Producer Pointer-Rule and Batcher Registry+Config invariants are both enforced that way.
- Rejected: **the guard in this task** — mechanically well-precedented (`cmd/lyx/ghguard_test.go`, `checkedcall_test.go`, `gitrepoboundary_test.go`, `boardguard_test.go`, and at least nine `allowedSpawners` entries of this exact shape), and the split genuinely is machine-checkable with a silent failure mode, which is why the guard is *scheduled* rather than dropped. But wave-1 consistency and the intact contention analysis outweigh landing it three waves early. **No invariant at all** — violates CLAUDE.md's same-commit recording rule and leaves the two-policy split undocumented for the very tasks that depend on it.

### membership-rule-is-a-standalone-entry-point

- Decision: the invariant's membership rule is **whether the module has, or is slated to have, a standalone entry point** — a way to be invoked outside a lyx hub. A module with one degrades, because a config-less invocation is a supported mode. A module that only ever runs inside a hub stays strict, because there an absent config means the hub is broken.
  Current classification: degrading `{shuttleengine, reedengine, perchengine, websterengine}` — standalone entries arrive in T5-T8;
  strict `{fabricengine, boardengine, loomengine, batcher}` — no standalone entry, `loomengine` explicitly deferred to Someday.
- Rationale: **the design doc gave both rationales, and only this one survives contact with the source.** `producers-standalone.md:271` argues Webster's placement on governs-what grounds — "`webster.yaml` is an operator-tunable producer config — role/model-spec settings — not hub state, the same distinction that keeps `burlerengine.LoadConfig` off the strict list already" — and `:272` adds the standalone-entry reason, that it "does not belong there once Webster has a standalone entry (T7)". The two rationales agree on Webster and disagree on `loomengine`, so the design's own line 271 is the surviving contradiction this decision resolves rather than a claim it inherits.
  A "what the config governs" phrasing cannot survive contact with the source: `internal/loomengine/config.go:90`'s `Config` is two role model-specs plus two timeout ints (`discussion`, `discussion_timeout_min`, `plan`, `plan_timeout_min`), and `internal/loomengine/template.yaml` is four lines of exactly that — structurally indistinguishable from `webster.yaml`, which the same phrasing would place in the degrading set while T2 pins loom strict. `internal/batcher`'s single `active: ""` key with a registry default is the same problem in weaker form.
- **A third class exists and the invariant must name it: own-loader modules.** Some modules never call `configengine.Load` at all — they resolve the path with `configengine.ConfigFile` and read the file themselves, with their own absent-file fallback. Three exist today, all already degrading:
  - `internal/burlerengine/config.go:63` — `burler.yaml`, absent file returns a zero `Config`. Bypasses `Load` because `MissingKeys` would misfire on its open-ended lenses/fans key set.
  - `internal/modelspec/load.go:24` — `models.yaml`, absent file returns `builtins()`. Cannot call a logging `Load` anyway, being leaf-capped.
  - `internal/scoutengine/load.go:23` — `servers.yaml`, absent file returns `builtins()` on `errors.Is(err, os.ErrNotExist)`. Its own file header says it deliberately mirrors `modelspec`'s shape.
  **Disposition: all three stay exactly as they are.** None is repointed, and none joins either pinned set. They already have the behaviour this task is giving the other four, and routing them through `LoadOrTemplate` would either break them (burler's key set) or violate a leaf cap (modelspec).
- **The invariant text must state that this class is outside its guard's subject.** A `configengine.Load(` / `LoadOrTemplate(` set-equality grep is structurally blind to all three own-loaders — they contain neither token — so without an explicit clause the invariant would read as though the two pinned sets enumerate every module config in the repo, which they do not. The clause is what stops T10 from either "discovering" the three as violations or believing its guard is total. State the class, name its three members, and mark it out of subject.
- **Watch item for T7/T10, stated rather than assumed:** `batcher` sits on the strict side because it has no standalone entry of its own, but its config is read on webster's batching path. If T7's standalone Webster turns out to reach `batcher.Active`, `batcher` moves to the degrading side and this invariant's pinned sets change with it. That is T7's finding to make, not this task's to pre-empt — recorded here so it is a decision then rather than a surprise.
- Rejected: **"what the config governs"** — falsified by `loomengine` above. **Moving `loomengine` to the degrading set** to rescue that phrasing — contradicts T2's brief, which names `loomengine` among the four strict callers where "an absent config means a broken hub". **Enumerating the two sets with no general predicate** — leaves T10, and every later caller, with a list and no way to classify against it.

## Technical context

**`internal/configengine/config.go`** is small and self-contained: `FindBaseDir`, `ConfigDir`, `ConfigFile`, `Load`.
Current imports are `fmt`, `os`, `path/filepath`, `internal/envsource`, `internal/lyxdirs`, `internal/yamlengine`;
this task adds `errors` (for the sentinel) and `internal/logger`.
`Load`'s body is a linear six-step flow — `FindBaseDir`, `os.ReadFile`, `MissingKeys`, `envsource.Build`, `yamlengine.Resolve`, return — with the two branches this task changes at lines 52-55 and 58-61.
`FindBaseDir`'s own two error branches are at lines 30-31 (`os.IsNotExist`, which gains the `ErrNotInitialized` wrap) and 32-34 (any other stat failure, unchanged and never a fallback trigger).
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
- `internal/configengine/config_test.go` — new `LoadOrTemplate` tests added; the existing `TestLoad_*` and `TestFindBaseDir_*` tests are untouched, including `TestLoad_NotInitialized` (line 251) and `TestLoad_AbsentFile` (line 104), which now pin that `Load` did *not* change. One comment edit besides the additions: `TestLyxDirNameConstant`'s doc comment at lines 383-385 (see "Doc surface" above).

**That is the complete test-file list — five files.** No `cmd/lyx` test is added or edited by this task.

**Tests that do NOT change, verified during exploration:**

- `internal/webstercli/cli_test.go:275` `TestStatusCmd_NotInitialized` — about an absent `state.json`, not config.
- `internal/reedcli/cli_integration_test.go:44` — asserts the error is *not* a config-resolution error; the fallback makes this strictly more true.
- `internal/batcher/config_test.go:129`, `internal/fabricengine/config_test.go:125` — strict-group callers, deliberately unaffected.

**No guard is built here** (see the `config-strictness-invariant-text-now-guard-at-t10` decision), so `cmd/lyx/` is untouched and `cmd/lyx/tierpurity_test.go`'s `allowedSpawners` map needs no new entry.

**Guard shape to describe in the invariant's `Enforced by` line, for T10 to build.** Set-equality over grep results, following `cmd/lyx/gitrepoboundary_test.go`'s pinned-set shape: walk non-test `*.go` under the module root, collect every package directory containing a `configengine.Load(` call and every one containing a `configengine.LoadOrTemplate(` call, and compare each against its pinned set — strict `{fabricengine, boardengine, loomengine, batcher}`, degrading `{shuttleengine, reedengine, perchengine, websterengine}`.
It would exclude `internal/configengine` itself (declaration site) and skip `_test.go` files, and — resolving its scan root via `exec.Command("go", "env", "GOMOD")` like its four siblings do — it would have to allowlist itself in `cmd/lyx/tierpurity_test.go`'s `allowedSpawners` map (declared at line 28, under a doc comment starting at line 24) to satisfy the Test Tier Purity Invariant.
Blind spot the invariant text should state, matching how the existing guards state theirs: a substring scan cannot see a call reached through an alias or a function value.
Record this shape in the invariant so T10 inherits a specification rather than re-deriving one.

**Doc surface.** `docs/shared-libs/configengine.md` carries the claims below, which become partly false and must be **updated, not merely appended to**. Each is cited once here, so the enumerations elsewhere in this file cannot drift from it:

| Lines | Claim that goes stale |
|---|---|
| 21-22 | "`_lyx/` presence is what makes a directory 'initialised'; if it is absent, `configengine` errors" — true of `Load` only |
| 30-41 | The Resolution-model six-step flow, described as the package's only loading path |
| 47 | "**Errors are strict**: missing template keys, absent files … cause hard errors" — the *absent files* half stops being true for `LoadOrTemplate` |
| 124-131 | `FindBaseDir`'s error messages plus the "Note on error rewrapping" block, which documents the `"not initialized"` substring match callers depend on — this is where `ErrNotInitialized` gets documented |
| 87 | "Typed wrappers (`board.LoadConfig`, `worktree.LoadConfig`, `weft.LoadConfig`) unmarshal this…" — same stale wrapper naming this task fixes in `config.go`'s own header; no such three wrappers exist |
| 136 | Calls `Load` a "five-step flow" while the same file's Resolution model above it lists six steps |
| 138-149 | `Load`'s per-error-case list, which needs a `LoadOrTemplate` sibling section rather than edits in place |

The table is the single authority for these line ranges — every other section, including the Q&A log, defers to it rather than restating a count or a range.

**Three further claims in the same file are already false today, independent of this task, and are fixed in the same pass** — they sit inside or beside the ranges above, so leaving them while editing around them would be the more deliberate act:

| Lines | Pre-existing falsehood |
|---|---|
| 99-102 | A `### LyxDirName (constant, "_lyx")` section asserting `internal/configengine` exports the token and is "the single declarer of this token". Source has no such export — `config.go` uses `lyxdirs.LyxDirName` — and the claim contradicts the **Lyxdirs Single-Declarer Invariant**, which names `internal/lyxdirs` as sole declarer. This one is not optional: a doc asserting the opposite of a live invariant is worse than a stale one. |
| 124 | Quotes `FindBaseDir`'s error as `not initialized: _lyx/ directory not found in <dir>`; `config.go:31` emits no `in <dir>` suffix. |
| 127-131 | The rewrap note's example remedy is `run "lyx init"`; every caller emits `run "lyx fabric reconcile"`. |

Fixing these is the cheap half of the doc pass and stops the file from carrying an invariant contradiction forward.

**The same falsehood appears once more, in Go, in a file this task already edits — fix it in-pass.** `internal/configengine/config_test.go:383-385`'s comment on `TestLyxDirNameConstant` reads "moved here from lyxcwd's own unit test now that configengine is the single declarer of the `_lyx` token", while the assertion three lines below it reads `lyxdirs.LyxDirName`. It contradicts itself and the Lyxdirs Single-Declarer Invariant in the same breath. Reword the comment to name `internal/lyxdirs` as the declarer;
the test body and name are correct as they stand and do not change. This gets a stated disposition rather than being swept under the out-of-file clause, precisely because it is neither outside this file's package nor outside this task's edits.

Any staleness *outside* `docs/shared-libs/configengine.md` and the six Go files this task edits is out of scope.

There is **no** `manifest/designs/configengine.md`; the package's design surface is this shared-libs doc plus `internal/configengine/config.go`'s own file-header comment, which is itself in scope (see Scope/In).

## Constraints

From `CONSTRAINTS.md`:

- **Cwd Resolution Invariant** — `LoadOrTemplate` takes a `baseDir string` exactly as `Load` does, and must never accept or construct a `*lyxcwd.Location`. Geometry is structural, never config-overridable, so the fallback must not invent a path from config. `internal/configengine` remains the single declarer of the `_lyx/config` path shape via `ConfigDir`/`ConfigFile`.
- **Lyxdirs Single-Declarer Invariant** — any new path construction uses `lyxdirs.LyxDirName`, never a `"_lyx"` literal. Enforced by `internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_GeometryLiterals`, which matches string literals in path-construction context.
- **Modelspec Leaf Invariant** — `internal/modelspec` production imports are capped at stdlib, `internal/configengine`, and `gopkg.in/yaml.v3`. This task must not push anything new into `modelspec`, and is the reason `modelspec`'s own fallback cannot log.
- **Test Tier Purity Invariant** — every new test is untagged Tier 1: no `git init`, no `exec.Command`, no `hubforge.NewHub`, no `gitkit.Copy*`, no `time.Sleep` ≥ 1s. Every test this task adds is pure `t.TempDir()` and satisfies this naturally, so no `allowedSpawners` entry is needed anywhere.
- **Hermetic Git Test Environment Invariant** — not triggered: no new test spawns git, so no new `TestMain` is required.
- **Markdown Link Integrity** — `docs/shared-libs/configengine.md` is a scanned source for `TestEnforcement_MarkdownLinks` (`internal/lyxcwd/docslink_test.go`), and it contains **zero inline links today**, so every link the doc pass adds is newly exposed to the guard. The line-102 correction naturally wants a link to `CONSTRAINTS.md`'s Lyxdirs Single-Declarer Invariant: any such link must resolve in both halves — the relative file path *and* the `#anchor` — since the guard resolves anchors on `.md` targets wherever they land, including outside `docs/`. Prefer a link over a bare mention there, but a bare mention is acceptable if the anchor cannot be made to resolve.
- **Documentation Lifecycle** — see [docs/overview.md#documentation-lifecycle](../docs/overview.md#documentation-lifecycle).
- **New: Config Strictness Invariant** — added by this task as text with review-obligation enforcement, per the `config-strictness-invariant-text-now-guard-at-t10` decision above.

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
- **Absence-only discrimination — the negative test the `fallback-only-on-proven-absence` decision exists for.** A `_lyx/` that exists but cannot be stat'd must error rather than fall back. Construct it by chmod'ing the containing directory to `0o000` on a POSIX-only test guarded by `runtime.GOOS`, skipping on Windows and when running as root (where the mode is not enforced); assert the returned error is *not* `errors.Is(err, ErrNotInitialized)` and that no template-derived bytes come back.
  **The test must restore the mode in a `t.Cleanup` registered immediately after `t.TempDir()` and before the chmod.** Cleanups run LIFO, so a restore registered after `t.TempDir()` runs before `TempDir`'s own `RemoveAll`; without it the unreadable directory makes teardown fail and the test fails even when its assertion passes.
  If a portable construction proves impossible, the fallback trigger must still be unit-tested directly at the sentinel level: assert `FindBaseDir` on an absent dir satisfies `errors.Is(err, ErrNotInitialized)` and that a hand-constructed non-sentinel error does not.
- `ErrNotInitialized` is wrapped, not returned bare: assert `errors.Is` holds *and* that the message still contains `"not initialized"`, since the four strict callers' `strings.Contains` rewraps depend on that text.
- `Load` regression: the existing `TestLoad_NotInitialized` and `TestLoad_AbsentFile` must keep passing unmodified, which is what proves the shared-body refactor preserved strict behaviour.

**Per-engine loader tests — one new or inverted test per loader, four total.** This is the check T2 names explicitly ("a new test per loader asserting a `baseDir` with no `_lyx/` returns the template-derived config rather than an error"). Each replaces that package's `TestLoadConfig_NotInitialized`:

- `shuttleengine` — bare `t.TempDir()`, assert no error and that `Config` carries template defaults (`PollIntervalMS == 500`, `RunTimeoutMin == 30`, `StartupTimeoutS == 90`, `ClaudeDenyAgentTool == true`).
- `reedengine` — bare `t.TempDir()`, assert no error and **GOOS-invariant** defaults only (`Width == 220`, `CollapsedStripRows == 3`, `Header.HeightRows == 1`). Do not assert on `Tmux`/`Shell`, which differ by GOOS.
- `perchengine` — bare `t.TempDir()` through `LoadConfig` (so the registry also falls back to `builtins()`), assert no error, `JudgeModel` resolved from `haiku`, and `RoundCaps == [5, 8, 10]`. This test covers the fallback *and* the model-spec resolution running on top of it.
- `websterengine` — bare `t.TempDir()`, assert no error, `Master == "sonnet"`, `SelfFixCap == 2`, `MasterTimeoutMin == 480`. The `modelspec.Parse` grammar check runs over the template defaults as a side effect, which is desirable.

Name these for what they now assert, not for the removed refusal — e.g. `TestLoadConfig_UninitializedFallsBackToTemplate`.

**Module-threading tests in `shuttleengine`, `reedengine`, `perchengine` — three reworks.** Each seeds the non-default module name with one key set to a non-default value, then makes two assertions: the seeded name returns the seeded value, and the unseeded default name returns the template default. Both halves are needed — the first alone does not catch a hardcoded module name, and the second alone does not prove the file is read.

**No guard test.** Deferred to T10 per the `config-strictness-invariant-text-now-guard-at-t10` decision, so no test outside the five files listed above.

**Verification commands.**

- Task-specific: `go test ./internal/configengine/... ./internal/shuttleengine/... ./internal/reedengine/... ./internal/perchengine/... ./internal/websterengine/...` — exactly T2's own Verify line, unextended.
- Baseline: `go test ./...` from the worktree root.
- Structural: confirm `internal/configengine` production code contains exactly two exported load entry points and one shared body;
  confirm `strings` is gone from all four callers' import sets;
  confirm no remaining production `configengine.Load(` call sites outside the four strict packages.

## Q&A log

- **Q:** How should `LoadOrTemplate` share code with `Load` — shared unexported body with a fallback flag, duplicated body, or wrap `Load` and re-detect the failure? **A:** Shared unexported body with a fallback flag. Wrapping `Load` was rejected because re-detecting which refusal fired requires error-string matching or a re-stat.
- **Q:** Should the fallback resolve the template through `envsource.Build`/`yamlengine.Resolve`, or return it raw? **A:** Resolve it. Env overrides are the only way a config-less user pins machine tool paths, and neither template that carries markers (shuttle, reed) uses the required `${env:NAME}` form — perch and webster carry no markers at all — so the path cannot fail on a missing variable.
- **Q:** Should the fallback be observable or silent? **A:** Observable — `logger.Info` inside `configengine`, accepting a new `internal/logger` import. Silent was rejected as leaving no record anywhere. For which level and why, see the dedicated Q&A entry below and the `info-level-observability` decision; an earlier draft of this entry answered `logger.Debug` on premises round 1 of review disproved.
- **Q:** How should the three `TestLoadConfig_ModuleArgIsThreadedThrough` negative halves be preserved once the absent-file refusal they rely on is gone? **A:** Convert to a positive discrimination test — seed the other module with a non-default value, assert the unseeded default name yields the template default. Deleting the half was rejected: a hardcoded module name would then pass silently.
- **Q:** Does this need a new `CONSTRAINTS.md` invariant, and should it be machine-enforced? **A:** The invariant, yes — the guard, no, deferred to T10. This reverses an earlier call in this discussion, on evidence from the orchestrator review (`_mill/orch-review.md`): T1, this wave's sibling, rejected machine-enforcing its own reworded invariant on the same grounds; `producers-standalone.md` places its new-invariant work at T10; and adding a new guard file here would silently invalidate the design's ten-task file-contention analysis. The invariant text records the guard's exact shape so T10 inherits a specification.
- **Q:** Is deferring the guard consistent with CLAUDE.md? **A:** Yes. The rule is "record any new cross-cutting invariant there, same commit" — recording, not enforcing. Review-obligation enforcement is the same level the Producer Pointer-Rule and Batcher Registry+Config invariants carry.
- **Q:** How should the invariant state which set a new caller joins? **A:** By whether the module has, or is slated to have, a standalone entry point. An earlier answer here said "by what the config governs — operator-tunable knobs degrade, hub state stays strict"; round 1 of discussion review falsified it against source, since `internal/loomengine`'s config is two model-specs plus two timeouts, structurally identical to `webster.yaml` yet pinned strict by T2. The design doc gave both rationales for Webster (`:271` governs-what, `:272` standalone-entry); they agree on Webster and disagree on `loomengine`, and only the standalone-entry one survives. `batcher`'s placement is recorded as a T7/T10 watch item rather than assumed permanent.
- **Q:** Does `LoadOrTemplate` fall back on any `FindBaseDir` failure, or only on absence? **A:** Only on proven absence, via a new exported `ErrNotInitialized` sentinel that `FindBaseDir` wraps on its `os.IsNotExist` branch;
  a permission or IO stat failure propagates. Falling back on the whole error would turn an unreadable `_lyx/` into a silent set of defaults. The config-file branch keeps its existing `os.IsNotExist` discriminator.
- **Q:** Which log level for the fallback, given `internal/logger`'s actual semantics? **A:** `logger.Info`. An earlier answer chose `logger.Debug` on two premises round 1 of review disproved: `SetVerbosity` maps `-v` to Info and only `-vv` to Debug, and `durableHandler` is unconditional at Info-and-above, so Debug never reaches the durable trace file while Info stays off stderr at the default Warn threshold.
- **Q:** Round 3 showed the durable sink is not unconditional — `ensureDurableSink` disarms outside an anchor and under `go test` without `LYX_TRACE=1`, which is the standalone case itself. Does that change the level? **A:** No, it changes what is claimed. Keep `logger.Info`, state the arming condition, and accept the standalone-unobservability as a limitation no level fixes: Debug is disarmed there too *and* excluded from the sink unconditionally, so Info still wins everywhere the sink is armed. Recorded as a T5-T8 watch item, since anchor-independent trace geometry is told-geometry work, not this task's. Also recorded: no test can assert the log line.
- **Q:** What happens to the four `TestLoadConfig_NotInitialized` tests? **A:** Inverted and renamed to assert a template-derived config rather than an error.
- **Q:** What happens to the dead `not initialized` rewrap blocks? **A:** Deleted in all four callers, along with the `strings` import each carries solely for them.
- **Q:** Does a config file that exists but is broken — missing keys, empty, comments-only — fall back? **A:** No. It still errors. Only an absent `_lyx/` or an absent file degrades.
- **Q:** Which docs must land in this commit? **A:** `docs/shared-libs/configengine.md` (a `LoadOrTemplate` section, plus corrections to every stale claim enumerated in the Technical context's "Doc surface" tables — those tables are the single authority for the line ranges and the count) and `CONSTRAINTS.md` (the new invariant). Neither is in T2's original Files list; the doc update is nonetheless mandatory, since leaving those claims stale would violate the Documentation Lifecycle rule outright rather than merely under-apply it. `manifest/roadmap.md` stays untouched, because its entry bundles T1+T2+T3.
- **Q:** Does this task's final file set stay inside T2's decomposed brief? **A:** Yes, once the guard is deferred. Six production/test files from the Files list, plus the four engines' `config_test.go` (required by T2's own Verify line, which the Files list is terse about), plus two docs. Nothing under `cmd/lyx/`, so the design's file-contention analysis across T1-T10 stays accurate.
