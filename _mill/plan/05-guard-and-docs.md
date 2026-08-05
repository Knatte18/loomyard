# Batch: guard-and-docs

```yaml
task: 'fabric: shrink hubgeometry to the minimal illusion primitive (slice 7)'
batch: guard-and-docs
number: 5
cards: 6
verify: go vet -tags "integration smoke scout" ./... && go test ./internal/lyxcwd/... ./cmd/lyx/...
depends-on: [4]
```

## Rename mechanic

For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the lines that must change after the move (package or module declaration, imports, identifier retargeting, seam splits).
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor.
4. Never write the relocated file from scratch and delete the original — that breaks git rename history and inflates review diffs.

## Batch Scope

Final consolidation. The enforcement guard has been converging batch by batch, so what remains is collapsing the last transitional scaffolding into the finished per-token ownership map, and the twelve-file documentation set the Documentation Lifecycle requires in step with the code.

The doc set is **twelve files, not four**, and the eight secondary ones are not cosmetic. Four of them (`plan-format`, `builder-contract`, `discussion-format`, `status-schema`) say something strictly stronger than a stale package name: they state that no package other than `hubgeometry` constructs those paths, which is the exact claim this slice inverts. `pattern.md` is the worst case — it describes an ownership split between `hubgeometry` and `fabricengine` that this slice collapses, so it needs a real edit rather than a rename. The same-commit rule is not satisfied by updating the doc that *defines* the invariant while eight others keep asserting the retired version by name.

## Cards

### Card 41: collapse the guard onto the finished ownership map

- **Context:**
  - `internal/configengine/config.go`
  - `internal/weftname/weftname.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/pattern/pattern.go`
  - `internal/lyxcwd/anchor.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `cmd/lyx/main.go`
- **Edits:**
  - `internal/lyxcwd/enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Remove the transitional `internal/lyxcwd` co-owner from the `_lyx` row and delete the private `lyxDirName` const batch 1 card 2 left in `internal/lyxcwd/lyxcwd.go` — by now every function that used it has left the module, so it is dead. `_lyx` is owned by `internal/configengine` alone, as its sole declarer. The finished map is exactly: `-weft` → `internal/weftname`; `_portals`, `_launchers`, `_raddle` → `internal/fabricengine`; `_pattern` → `internal/pattern` and `internal/fabricengine`; `_lyx` → `internal/configengine`; `_board` → `internal/lyxcwd` and `internal/fabricengine`; `-HUB` → `internal/lyxcwd` and `internal/fabricengine`. Collapse whatever allowlist scaffolding remains into that single map, keeping the `predicate` sub-test shape (synthetic positive/negative Go snippets parsed with `go/parser`, whole-token equality after `strconv.Unquote`, so `_boardroom` and `-weft-bare` stay negatives) and the `scanned_non_empty` sanity sub-test. `TestEnforcement` (the `os.Getwd`/`--show-toplevel` ban) keeps its shape with the allowlist path at `internal/lyxcwd` and `cmd/lyx/main.go`. `.lyx` stays **unpoliced**, as it is today: batch 2 spread it to `logger`, `scoutengine`, `reedengine`, `burlerengine` and `shuttleengine` per the anchoring table, and slice 9 — which registers `.lyx` as a pathspec junction and removes `crossModuleMachineLocalExcludes` — is where it gets an owner. Adding it now would have to be undone one slice later; the omission is deliberate, not an oversight, and must say so in a comment.
- **Commit:** `test(lyxcwd): collapse the geometry guard onto the finished ownership map`

### Card 42: rewrite the module doc

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxcwd/anchor.go`
  - `internal/lyxcwd/gate_test.go`
  - `cmd/lyx/main.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `docs/shared-libs/README.md`
  - `docs/shared-libs/configengine.md`
  - `docs/shared-libs/envsource.md`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `docs/shared-libs/hubgeometry.md` -> `docs/shared-libs/lyxcwd.md`
- **Requirements:** `git mv` the module doc, then rewrite it to the three-operation contract: `Getwd()`; `Resolve(cwd)` / `ResolveWithAnchor(cwd, anchor)` / `ResolveWorktree(root)`; and the `Location{RepoName, HubPath, WorktreeName, AnchorRel}` coordinates with `WorktreePath()` and `AnchorPath()`. The whole departing API comes out. Three things must be stated that the old doc had no reason to say: `ResolveWithAnchor` is a **bypass** — not a general-purpose resolver, and a caller reaching for it to escape a gate failure is misusing it, the correct fix being to stand in the anchored directory; the module's import ceiling is stdlib plus `internal/gitexec` only, which is what keeps `fabricengine` → `logger` → `lyxcwd` acyclic; and cwd resolution staying below Fabric is a documented **infrastructure exception** to "every module asks Fabric", with the intended follow-up recorded — move resolution into `fabricengine` and inject the log directory into `logger` from `cmd/lyx/main.go`, which eliminates the module entirely but pulls `logger` initialization rework in and is out of scope here. `docs/shared-libs/README.md:11` renames the file, the package and the role — "sole owner of cwd/root math" is no longer true. `configengine.md` gains `ConfigDir`/`ConfigFile`/`LyxDirName`; `envsource.md` gains `DotEnv` and the note that `envsource` is now a stdlib-only leaf.
- **Commit:** `docs(shared-libs): rewrite the module doc as lyxcwd`

### Card 43: rewrite the two primary design surfaces

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/reconcile.go`
- **Edits:**
  - `docs/overview.md`
  - `manifest/designs/fabric-unified-view.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `docs/overview.md`, rewrite the "Hub Geometry Invariants" section to the narrower contract and update the module table and execution-stack rows for the rename. Its **"Junction model" section is already accurate and must not be touched**. In `manifest/designs/fabric-unified-view.md`, correct the wording that says a module's durable subdirectory is joined onto `cwd` — the anchoring table in the overview's Shared Decisions is the truth, and `CONSTRAINTS.md` carries the identical wording, already fixed in batch 1 card 18; the two must not be allowed to disagree, so re-read both after editing. Record the `_board` junction: an operator-convenience link at `<AnchorPath>/_board`, wired outside the `pathspec` mechanism, unmonitored by `Healthy`, re-wired idempotently by reconcile, and read by no lyx code path — that last point is a hard rule to carry into the doc, not a default a later caller may quietly opt out of. Record `internal/weftname` as the owner of the `-weft` convention and why `lyxtest` cannot instead build fixtures by calling real `fabric clone`/`fabric add`: `lyxtest` → `fabricengine` is a compile-time cycle, and 19 of the 25 in-package `fabricengine` test files need unexported access so they cannot be converted to `package fabricengine_test`; doing it properly needs an export-for-test shim across `fabricengine` first, which is a slice of its own.
- **Commit:** `docs(design): record the post-shrink geometry, weftname and the _board junction`

### Card 44: correct the four reference docs that invert the invariant

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/loomengine/plan.go`
  - `internal/websterengine/report.go`
  - `internal/planparser/parse.go`
- **Edits:**
  - `docs/reference/plan-format.md`
  - `docs/reference/builder-contract.md`
  - `docs/reference/discussion-format.md`
  - `docs/reference/status-schema.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** These four are wrong on substance, not just on the package name — each asserts that paths resolve through `internal/hubgeometry` and that no other package constructs them, which is the exact claim `per-module-constructors` inverts. `plan-format.md:38` names the Hub Geometry Invariant and says "no other package constructs them": `loomengine` is the constructor now, and `internal/planparser` owns the `_lyx/plan` relative token. `builder-contract.md:170` says `_lyx/webster/` resolves "via its own `hubgeometry` helpers": those helpers are `websterengine.Dir`/`ReportsDir`/`PromptsDir`. `discussion-format.md:14` and `status-schema.md:9` carry the same wording for the discussion pair and `_lyx/status.json`: both are `loomengine` now. Correct the technical claim in each, not merely the package name.
- **Commit:** `docs(reference): correct the four docs asserting hubgeometry constructs module paths`

### Card 45: correct the remaining secondary references

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/pattern/pattern.go`
  - `internal/fabricengine/pull.go`
  - `internal/modelspec/load.go`
  - `internal/loomengine/preflight.go`
- **Edits:**
  - `docs/reference/model-spec.md`
  - `manifest/designs/loom.md`
  - `manifest/designs/pattern.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `model-spec.md:32` names `hubgeometry.ConfigFile`, which is `configengine.ConfigFile`. `loom.md` has three references — preflight's geometry check at line 38, the module table row at 103, and "launcher geometry already in `internal/hubgeometry`" at 129, which is `fabricengine` now. `pattern.md` has four at lines 3, 18, 47 and 62, asserting that `hubgeometry` declares the `_pattern` junction record and owns every `_pattern` path literal — contradicted twice over: the record moved to `fabricengine` and the constructors to `internal/pattern`. Note for the implementer: `loom.md:38` and `pattern.md:3,47` were written as status narrative ("built and merged", "✅ Done"); the edit is to the technical clause inside them, **not** to the status claim. Do not rewrite the status.
- **Commit:** `docs: correct the remaining hubgeometry references in loom, pattern and model-spec`

### Card 46: mark slice 7 shipped on the roadmap

- **Context:**
  - `manifest/designs/fabric-unified-view.md`
  - `docs/overview.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `manifest/roadmap.md:9` currently reads "Slices 7-10 … are next" and enumerates all four. Slice 7 is a planned roadmap item and it is now complete, so the entry moves: state that slice 7 has shipped — `internal/hubgeometry` shrunk to `internal/lyxcwd`, the ~20 per-module constructors moved to their owning modules, the `Weft*`/junction plumbing moved to `fabricengine`, the weft-visibility leak closed at all seven call sites, and the `cwd`-reachable `_board` junction wired — and that slices 8, 9 and 10 remain. Do not touch the roadmap for anything else in this task; per the repo's task-completion rule the roadmap moves only on completing or adding a planned item, and the rest of this slice's work is covered by git history and the module docs.
- **Commit:** `docs(roadmap): mark fabric slice 7 shipped`

## Batch Tests

`verify: go vet -tags "integration smoke scout" ./... && go test ./internal/lyxcwd/... ./cmd/lyx/...` — narrow on purpose. Card 41 is the only code change in this batch and it is confined to `internal/lyxcwd/enforcement_test.go` plus the dead const it removes; `cmd/lyx` is included because it hosts the tier-purity, hermetic-env and help-tree guards that a stray edit would trip. Cards 42-46 are documentation with no runnable surface, which is why the scope does not widen for them.

The guard itself is the test for this batch: `TestEnforcement_GeometryLiterals` passing against the finished ownership map is the machine-checked proof that batches 1-4 **moved** ownership of every geometry token rather than copying it, and `TestEnforcement` passing with the `internal/lyxcwd` allowlist is the proof that the `os.Getwd`/`--show-toplevel` ban survived the rename. The repo-wide `done_gate` (`go test ./...`) runs before mill-go marks the task done and is the backstop for packages no batch verify covered.
