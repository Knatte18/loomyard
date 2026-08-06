# Batch: structural-dirs-and-never-committed-routing

```yaml
task: .lyx hygiene -- relocate transients out of _lyx, fix .lyx junction geometry (slice 9)
batch: structural-dirs-and-never-committed-routing
number: 7
cards: 8
verify: go test ./internal/fabricengine/... ./internal/fabriccli/... ./internal/configsync/... && go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/...
depends-on: [6]
```

## Batch Scope

This batch makes `_lyx` and `.lyx` **structural** — injected by `internal/fabricengine` in code rather than read from `fabric.yaml`'s `pathspec` — and installs the never-committed routing that keeps `.lyx` out of every git invocation fabric issues.
Concretely: two declared sets (`structuralCommittedDirs`, `structuralNeverCommittedDirs`), dedup-preserving unions for the pathspec/commit-routing set and the slug-reservation set, a third bucket in `classifyPaths` plus a hard error in `Commit`, the `HubReservedNames` split into a slug-reservation set and a junction-wiring block set, `--exclude-standard` on `weftPathspecFilter`'s probe, and `template.yaml`'s default shrinking from `_lyx _pattern` to `_pattern`.

It is one batch because the set composition is a single coherent decision whose consumers cannot be updated piecemeal: the moment `_lyx` leaves the template default, `internal/fabriccli/weft_verbs.go:100` must already build from the routing set or a freshly-initialised repo silently stops syncing `_lyx` altogether — the single most breakage-prone edit in the task.

**External interface batch 8 consumes:** `structuralCommittedDirs`, `structuralNeverCommittedDirs`, the wiring-block name set, and `pathspecNames`/`slugReservedNames` helpers.

**Batch-local decision — `WiredNames`/`RepoWiredNames` gain `structuralCommittedDirs` only, NOT `structuralNeverCommittedDirs`.**
Folding `.lyx` into the wired name-set is deliberately deferred to batch 8, where the content-adoption branch lands in the same commit range.
Doing it here would make the very next `lyx fabric reconcile` hard-error in every worktree that already holds a real `.lyx` — which is all of them — and no fixture-based test would catch it, because fixtures start clean.
Every other consumer of `structuralNeverCommittedDirs` (routing exclusion, slug reservation, `classifyPaths`) is wired here.

**Batch-local decision — `HubReservedNames()` keeps its name and its exact current value.**
It becomes the *junction-wiring block set*, which is what `filterHubReserved` and `scanOnDiskJunctionNames` need and what they already get today, so neither call site changes behaviour.
The new, wider slug-reservation set is a separate private helper consumed only by `IsReservedHubName`.
Naming it the other way round would silently make `scanOnDiskJunctionNames` skip `.lyx`, rendering it invisible to `Unwire`'s sweep and to `applyStaleRemoval` — wired forever, never torn down.

**Batch-local decision — deployed `fabric.yaml` files still parse and are not migrated.**
An existing `weft:main` config keeps `pathspec: _lyx _pattern`, so `_lyx` arrives from two sources.
Every set below is therefore a **deduplicated** union, not a concatenation, ordered structural-names-first then config names in `Dirs()` order, first occurrence wins.
No value-level config migration is attempted;
`lyx config reconcile` propagates new *keys* only, and the structural decision removes the need.

## Cards

### Card 36: declare the two structural directory sets and their unions

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/fabricengine/config.go`
  - `internal/fabricengine/doc.go`
- **Edits:**
  - `internal/fabricengine/junctionnames.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** declare two package-level vars in `junctionnames.go`:
  `structuralCommittedDirs = []string{lyxdirs.LyxDirName}` and `structuralNeverCommittedDirs = []string{lyxdirs.DotLyxDirName}`.
  Their godocs must state that geometry is structural and never config/env-overridable (Cwd Resolution Invariant), that both directories must always exist because every lyx module fails without them, and that a `fabric.yaml` dropping `_lyx` from `pathspec` would tear away the durable tree while one omitting `.lyx` would leave machine-local scratch unwired — which is why neither is read from config.
  Add a private `dedupUnion(groups ...[]string) []string` helper preserving first-occurrence order, documented as load-bearing rather than tidy: a deployed `pathspec: _lyx _pattern` means `_lyx` arrives from two sources, and without dedup duplicate names reach `HostJunctions`, `ScopedPathspec` and status output.
  Add two exported-where-needed name-set helpers, both taking a loaded `Config`:
  `pathspecNames(cfg Config) []string` = `dedupUnion(structuralCommittedDirs, filterHubReserved(cfg.Dirs()))`, documented as the pathspec/commit-routing set that **never** contains a `structuralNeverCommittedDirs` entry;
  and `slugReservedNames(cfg Config) []string` = `dedupUnion(structuralCommittedDirs, structuralNeverCommittedDirs, cfg.Dirs(), hubSlugReservedNames())`, documented as taking `cfg.Dirs()` **raw** — unfiltered — because a worktree slug must be refused for every one of these names regardless of wiring.
  Export `PathspecNames(baseDir string) ([]string, error)` as the out-of-package entry point (loading the config via `LoadConfig` exactly as `WiredNames` does) since `internal/fabriccli` needs it in card 40;
  keep the `Config`-taking form private for in-package callers.
  Change `junctionNames(baseDir)` to return `dedupUnion(structuralCommittedDirs, filterHubReserved(cfg.Dirs()))` so `WiredNames`/`RepoWiredNames` gain `_lyx` structurally, and update `WiredNames`'s godoc to record both that gain and the deliberate omission of `structuralNeverCommittedDirs` until batch 8.
  Update the file-header comment, which currently says the name-set is loaded from "the repo-wide fabric.yaml pathspec" alone.
- **Commit:** `feat(fabricengine): declare structural committed and never-committed dir sets`

### Card 37: split HubReservedNames into wiring-block and slug-reservation sets

- **Context:**
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/add.go`
- **Edits:**
  - `internal/fabricengine/junctionnames.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** keep `HubReservedNames()` returning exactly `{BoardDirName, portalsDirName, launchersDirName, "_raddle"}` and rename its role in the godoc to the **junction-wiring block set**: the hub-structural names that must never wire a per-worktree junction, consumed by `filterHubReserved` and by `scanOnDiskJunctionNames`.
  Its doc must state explicitly that `.lyx` is deliberately **not** a member: adding it here would make `filterHubReserved` delete `.lyx` from the wired names so the per-worktree junction is never created, and would make `scanOnDiskJunctionNames` skip it so `Unwire`'s sweep and `applyStaleRemoval` could never see it — wired forever, never torn down.
  Add `hubSlugReservedNames() []string` returning `append(HubReservedNames(), lyxdirs.DotLyxDirName)` (built without mutating the returned slice — allocate a fresh one), documented as the slug-reservation set: names a worktree slug may never claim, `.lyx` included because a worktree named `.lyx` would collide with the hub-level `<hub>/.lyx` batch 8 recognises.
  Change `IsReservedHubName(name string, junctionNames []string) bool` to iterate `hubSlugReservedNames()` instead of `HubReservedNames()`, and to additionally treat every `structuralCommittedDirs` and `structuralNeverCommittedDirs` entry as reserved, so `Topology.Add`'s existing `IsReservedHubName(slug, t.cfg.Dirs())` call site in `add.go` needs no change and still refuses `_lyx` and `.lyx` even for a config naming neither.
  Update `IsReservedHubName`'s godoc to describe the full union.
- **Commit:** `refactor(fabricengine): split hub-reserved names into wiring-block and slug-reservation sets`

### Card 38: give classifyPaths a third bucket for never-committed paths

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/fabric.go`
- **Edits:**
  - `internal/fabricengine/classify.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** change `classifyPaths(relPath string, wiredNames []string, files []string) (warp, weft []string)` to `classifyPaths(relPath string, routingNames, neverCommittedNames []string, files []string) (warp, weft, neverCommitted []string)`.
  Evaluate the never-committed prefixes **first**: a path under a `neverCommittedNames` prefix (same `ScopedPathspec` + `isUnderAnyWeftPrefix` segment-boundary matching the weft check already uses) goes to the third bucket and is not considered for either of the other two;
  otherwise the existing weft-then-warp fallthrough is unchanged.
  Keep the function pure and I/O-free, and keep input order and original path spellings within each bucket (significant on Windows).
  Rewrite the file-header comment and the function godoc to state why omission is not enough and is actively worse: `classifyPaths` is a strict split where everything not matching a weft prefix falls through to **warp** — the user's own repo — where `git add` on an ignored path fails the whole invocation with exit 1 and stages nothing, taking every legitimate `_lyx` file named in the same call down with it.
  State that classification stays policy-free: the third bucket is reported, and turning it into an error is `Commit`'s job.
- **Commit:** `feat(fabricengine): route never-committed paths to a third classify bucket`

### Card 39: make Commit reject a never-committed path by name

- **Context:**
  - `internal/fabricengine/classify.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/weftgit.go`
- **Edits:**
  - `internal/fabricengine/commit.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `Fabric.Commit`, replace the `RepoWiredNames(l)` lookup used for classification with the repo-wide **routing** set — load it via the new `PathspecNames(repoWideFabricBase(l))`-equivalent path (use the in-package `Config`-taking helper against `repoWideFabricBase(l)` rather than re-deriving a base) — and pass `structuralNeverCommittedDirs` as the third argument to `classifyPaths`.
  Immediately after classification and **before** taking any lock or committing anything, return a hard error naming the first offending path when the never-committed bucket is non-empty, e.g. `fabricengine: refusing to commit %q: paths under %s are never committed`.
  Do not silently drop them: a caller passing a `.lyx` path is a bug and must be told.
  Extend `Commit`'s godoc with this behaviour and with the reason it is an error rather than a filter, and note that the check precedes the lock so a rejected call mutates nothing and spawns no push.
- **Commit:** `feat(fabricengine): reject never-committed paths in Commit with a hard error`

### Card 40: build fabric's own sync pathspec from the routing set

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/config.go`
- **Edits:**
  - `internal/fabriccli/weft_verbs.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** change the `pathspec = fabricengine.ScopedPathspec(l.AnchorRel, cfg.Dirs())` line in the persistent pre-run to build from the exported routing set instead of raw `cfg.Dirs()`: obtain the names via `fabricengine.PathspecNames(fabricengine.BoardDir(l.HubPath))` — the same base the `LoadConfig` call two lines above already uses — and surface a load failure through the same `output.Err` + `clihelp.Abort(ctx, 1)` shape the neighbouring error paths use.
  Add a comment stating why this cannot stay on `cfg.Dirs()`: with `_lyx` no longer in `template.yaml`'s default, a freshly-initialised repo's `pathspec` names only `_pattern`, so a raw-`Dirs()` sync would silently stop syncing `_lyx` entirely, and that `.lyx` can never appear here because the routing set excludes `structuralNeverCommittedDirs` by construction.
  Leave the `cfg` variable itself in place — the other verbs still read it — and leave the three `fab.Commit` call sites unchanged.
- **Commit:** `fix(fabriccli): build the weft sync pathspec from fabric's routing set`

### Card 41: add --exclude-standard to the weft pathspec probe

- **Context:**
  - `internal/fabricengine/commit.go`
- **Edits:**
  - `internal/fabricengine/weftgit.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `entryMatchesWeft`, add `--exclude-standard` to the `git ls-files --cached --others -- <entry>` argv, and update the error strings that echo the command so they still name the invocation accurately.
  Extend `weftPathspecFilter`'s and `entryMatchesWeft`'s godocs to record the latent bug this closes: without the flag, `git ls-files --cached --others` *matches ignored files*, so any stray ignored file matching a pathspec entry makes the filter forward a doomed entry, and `git add` then fails the entire invocation with exit 1 rather than skipping it — toppling a commit that had legitimate content in the same call.
  With the flag, such an entry is filtered out, `positive` stays false for it, and the commit degrades to a clean no-op.
- **Commit:** `fix(fabricengine): pass --exclude-standard to the weft pathspec probe`

### Card 42: shrink the template pathspec default to _pattern

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/template.go`
  - `internal/configsync/configsync.go`
- **Edits:**
  - `internal/fabricengine/template.yaml`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** change the `pathspec:` default value from `_lyx _pattern` to `_pattern` and rewrite its inline comment: the key now names **optional** per-repo directories only, because `_lyx` and `.lyx` are structural and injected in code by `internal/fabricengine`, never read from here.
  Do not remove the key — `_pattern` is genuinely per-repo optional, so the field survives with a narrower job.
  Do not attempt any value-level migration of already-deployed configs: an existing `pathspec: _lyx _pattern` still parses and still resolves correctly, because every set is a deduplicated union.
- **Commit:** `refactor(fabricengine): shrink the pathspec default to _pattern`

### Card 43: cover the structural sets, the third bucket and the routing set

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/classify.go`
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/template.yaml`
  - `internal/fabriccli/weft_verbs.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/fabricengine/classify_test.go`
  - `internal/fabricengine/junctionnames_test.go`
  - `internal/fabricengine/template_test.go`
  - `internal/fabricengine/add_test.go`
  - `internal/fabricengine/hostjunction_test.go`
  - `internal/fabricengine/config_driven_junctions_integration_test.go`
  - `internal/fabricengine/weftgit_exclude_test.go`
  - `internal/fabriccli/cli_test.go`
  - `internal/configsync/configsync_test.go`
- **Creates:**
  - `internal/fabricengine/structuraldirs_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** create `structuraldirs_test.go` (untagged — pure set arithmetic over a hand-built `Config`) asserting the trio the naive one-list implementation fails:
  `WiredNames`-equivalent output contains `_lyx` even for a `Config` whose `Pathspec` names neither structural directory;
  a deployed `Pathspec: "_lyx _pattern"` yields **no duplicate** `_lyx` in the wired set, the routing set, or the slug-reservation set;
  the routing set contains `_lyx` but **never** `.lyx`;
  `.lyx` is refused as a worktree slug via `IsReservedHubName` even with an empty `junctionNames` argument;
  and `HubReservedNames()` still returns exactly the four hub-structural tokens with `.lyx` absent — the guard that keeps `scanOnDiskJunctionNames` able to see it.
  In `classify_test.go`, update every call to the widened signature and add cases proving a `.lyx`-prefixed path lands in **neither** weft nor warp — asserting only "never routed to weft" would pass on the dangerous implementation where it falls through to the user's own repo — plus segment-boundary cases (`.lyxfoo` is not under `.lyx`) and a `relPath != "."` case.
  Add a `Commit`-level case (in whichever of the package's commit tests already has a `Fabric` fixture available) asserting a `.lyx` path produces an error naming that path and that nothing was committed on either side.
  In `template_test.go`, rewrite `TestConfigTemplate_PathspecResolvesToLyxAndPattern` for the new default: the resolved `pathspec` whitespace-splits to exactly `["_pattern"]`.
  Rename the test accordingly.
  In `junctionnames_test.go`, update the `HubReservedNames()` sanity block's commentary and add the `.lyx`-is-not-in-it assertion.
  In `add_test.go`, add a case for a slug of `.lyx` being refused.
  In `hostjunction_test.go`, update the "default two-name pathspec" commentary and expectations now that `_lyx` arrives structurally rather than from config.
  In `config_driven_junctions_integration_test.go`, keep the narrow-pathspec-is-healthy proof but re-base it: a `pathspec: _lyx` config is now redundant rather than narrow, so use a config whose pathspec names neither structural directory and assert `_lyx` is still wired and `Healthy` still reports true, and keep the `_extra`-appended future-module proof intact.
  In `weftgit_exclude_test.go`, add the `--exclude-standard` regression: an entry matching only ignored files is filtered out and reported non-positive, so the commit is a clean no-op instead of a hard `git add` failure — this one has a verified pre-fix failure to reproduce, so write it to fail against the pre-card-41 code.
  In `internal/fabriccli/cli_test.go`, keep its `pathspec: _lyx` seeded config working (it must, by dedup) and add the regression guard for card 40: `lyx fabric sync` still commits `_lyx` content when the repo-wide `fabric.yaml` names only `_pattern`, and the pathspec it builds never names `.lyx`.
  In `internal/configsync/configsync_test.go`, update the two assertions that expect the template-default fallback to contain `_lyx` (`"only warp.yaml present, pathspec falls back to template default"` and the migrated-value case's expectations, keeping the migrated `pathspec: _lyx custom-dir` case asserting the *existing value is preserved*, which is the behaviour that must not change).
  Keep every `//go:build integration` line first in its file.
- **Commit:** `test(fabricengine): cover structural sets, the third bucket and the routing set`

## Batch Tests

`verify: go test ./internal/fabricengine/... ./internal/fabriccli/... ./internal/configsync/... && go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/...` — the three edited packages, with a tagged run because card 43 edits four `//go:build integration` files (`config_driven_junctions_integration_test.go`, `weftgit_exclude_test.go`, `internal/fabriccli/cli_test.go`) and because the sync-still-commits-`_lyx` regression is only expressible against a real paired fixture.

Covered files: `internal/fabricengine/structuraldirs_test.go` (new), `classify_test.go`, `junctionnames_test.go`, `template_test.go`, `add_test.go`, `hostjunction_test.go`, `config_driven_junctions_integration_test.go`, `weftgit_exclude_test.go`, `internal/fabriccli/cli_test.go`, `internal/configsync/configsync_test.go`, plus the package's remaining reconcile/status/commit suites re-run as regression cover.

Three assertions carry the batch.
The `.lyx`-lands-in-neither-bucket case is the one that distinguishes the correct implementation from the dangerous one: a `.lyx` path silently falling through to warp reaches the user's own repo and fails there on the same exit-1 ignored-path error, taking the whole commit with it.
The sync-still-commits-`_lyx` regression guards the single most breakage-prone edit in the whole task — the `weft_verbs.go` pathspec source — where a miss is silent, not loud.
And the `HubReservedNames()`-does-not-contain-`.lyx` assertion, paired with the wired-set and slug-reservation assertions, is precisely the trio a naive one-list implementation fails;
any two of the three can pass while the third is broken.
