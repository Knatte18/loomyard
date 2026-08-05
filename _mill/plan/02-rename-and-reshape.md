# Batch: rename-and-reshape

```yaml
task: 'fabric: shrink hubgeometry to the minimal illusion primitive (slice 7)'
batch: rename-and-reshape
number: 2
cards: 3
verify: go build -tags integration ./internal/lyxcwd/... && go vet -tags integration ./internal/lyxcwd/...
depends-on: [1]
```

## Rename mechanic


For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the lines that must change after the move (package declaration, import lines, identifier retargeting, seam splits).
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor.
4. Never write the relocated file from scratch and delete the original — that breaks git rename history and inflates review diffs.

## Batch Scope

This batch is the one thing in the task that cannot be split: `git mv internal/hubgeometry internal/lyxcwd`, `Layout` becomes `Location{RepoName, HubPath, WorktreeName, AnchorRel}`, the strict cwd gate replaces the at-or-below gate, and the anchor marker becomes `.lyx-anchor`. The moment the struct changes shape every reader of a removed field stops compiling, and those readers are ~190 files — so this batch deliberately leaves them broken and batches 3 and 4 sweep them.

Batch-local decision — `verify` is **`go build -tags integration ./internal/lyxcwd/... && go vet -tags integration ./internal/lyxcwd/...`**, not the full suite. This is the only batch in the plan whose gate is not repo-wide, and the narrowing is honest rather than convenient: the renamed module must compile and vet cleanly on its own, which is exactly what this batch is responsible for, and nothing wider can be true until batch 4 lands. A `verify: null` would have been the lazy alternative and is rejected — it would leave the batch with no gate at all, when a real one exists.

The rename batch also carries the gate and the marker rename because both are edits to the same three files (`lyxcwd.go`, `anchor.go`, `fabricengine/clone.go`) and separating them would mean opening those files twice for no reviewer benefit.

External interface batches 3 and 4 consume: `lyxcwd.Location`, `(*Location).WorktreePath()`, `(*Location).AnchorPath()`, `Location.HubPath`/`AnchorRel`/`RepoName`, `lyxcwd.ErrCwdOutsideAnchor`, `lyxcwd.AnchorFileName`, `lyxcwd.ResolveWithAnchor`.

## Cards

### Card 5: rename the package to `internal/lyxcwd` and reshape `Layout` into `Location`

- **Context:**
  - `_mill/discussion.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/fabricengine/hostlayout.go`
  - `internal/fabricengine/clone.go`
  - `internal/lyxtest/lyxtest.go`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/hubgeometry/anchor.go` -> `internal/lyxcwd/anchor.go`
  - `internal/hubgeometry/anchor_test.go` -> `internal/lyxcwd/anchor_test.go`
  - `internal/hubgeometry/discussionpath_test.go` -> `internal/lyxcwd/discussionpath_test.go`
  - `internal/hubgeometry/enforcement_test.go` -> `internal/lyxcwd/enforcement_test.go`
  - `internal/hubgeometry/geometry_test.go` -> `internal/lyxcwd/geometry_test.go`
  - `internal/hubgeometry/hubgeometry.go` -> `internal/lyxcwd/lyxcwd.go`
  - `internal/hubgeometry/hubgeometry_test.go` -> `internal/lyxcwd/lyxcwd_test.go`
  - `internal/hubgeometry/hubgeometry_unit_test.go` -> `internal/lyxcwd/lyxcwd_unit_test.go`
  - `internal/hubgeometry/loomstatus_test.go` -> `internal/lyxcwd/loomstatus_test.go`
  - `internal/hubgeometry/pattern_test.go` -> `internal/lyxcwd/pattern_test.go`
  - `internal/hubgeometry/planpath_test.go` -> `internal/lyxcwd/planpath_test.go`
  - `internal/hubgeometry/raddle_guard_test.go` -> `internal/lyxcwd/raddle_guard_test.go`
  - `internal/hubgeometry/scoutdaemon_test.go` -> `internal/lyxcwd/scoutdaemon_test.go`
  - `internal/hubgeometry/testmain_test.go` -> `internal/lyxcwd/testmain_test.go`
  - `internal/hubgeometry/webstergeom_test.go` -> `internal/lyxcwd/webstergeom_test.go`
  - `internal/hubgeometry/weft_test.go` -> `internal/lyxcwd/weft_test.go`
  - `internal/hubgeometry/worktreelogs_test.go` -> `internal/lyxcwd/worktreelogs_test.go`
- **Requirements:** The rename is **one** command — `git mv internal/hubgeometry internal/lyxcwd` — plus three follow-up `git mv` calls for the files whose basename also changes (`hubgeometry.go`, `hubgeometry_test.go`, `hubgeometry_unit_test.go`). The per-file `Moves:` pairs above document that operation's full effect so the plan's move accounting is complete; they are **not** seventeen separate commands, and the `## Rename mechanic` section's per-pair instruction is satisfied by the directory move for the fourteen files whose basename is unchanged. Run the directory move first, then rename the package clause to `lyxcwd` in every moved file and rewrite the package godoc: the module is no longer a geometry owner — constructing paths from structural tokens is precisely what it stops doing — it is the entry gate that converts "the process started somewhere" into "these are the coordinates of a legal lyx worktree, or here is why this is not one". Rename the type `Layout` to `Location` and replace its fields with, in this order, `RepoName string`, `HubPath string`, `WorktreeName string`, `AnchorRel string`. The order is outermost identity first: a repo has hubs, a hub has worktrees, a worktree has an anchored subpath. `Cwd` stops being a field — it remains the parameter to `Resolve(cwd)`, because it is the only thing the process knows at startup, but under the strict gate it is provably equal to `AnchorPath()` after a successful resolve and storing it would duplicate a derivable value. The worktree path is likewise not stored: the worktree is a direct child of the hub by construction (`hub := filepath.Dir(worktreeRoot)` today), so `WorktreeName` suffices. Add `func (l *Location) WorktreePath() string` returning `filepath.Join(l.HubPath, l.WorktreeName)` and `func (l *Location) AnchorPath() string` returning `filepath.Join(l.WorktreePath(), l.AnchorRel)`. `resolveCore` sets `HubPath` from `filepath.Dir(workTreeRoot)`, `WorktreeName` from `filepath.Base(workTreeRoot)`, `AnchorRel` from the recorded marker falling back to `"."` — **not** to the cwd-derived relative path, which would make the new name a lie and makes `_lyx` resolve to a different place depending on where the user happened to stand — and `RepoName` from `strings.TrimSuffix(filepath.Base(HubPath), HubSuffix)`. Note the behaviour change: for a non-hub layout (an ordinary clone, or a `lyxtest` synthetic hub) `RepoName` now yields the parent directory's name rather than the prime worktree's; both are heuristics and the new one costs no subprocess. Rewrite every in-module reference to a removed field: `l.WorktreeRoot` becomes `l.WorktreePath()`, `l.Hub` becomes `l.HubPath`, `l.RelPath` becomes `l.AnchorRel`, and the `_lyx`-durable constructors (`PlanDir`, `LoomStatusFile`, `LoomStatusLock`, `DiscussionDir`, `LyxDir`, `DotLyxDir`) rebase from `l.Cwd`/`l.WorktreeRoot` onto `l.AnchorPath()`, while `WorktreeLogsDir`, `ScoutDaemonStateFile` and `ScoutDaemonLock` stay on `l.WorktreePath()` and `HubLogsDir` stays on `l.HubPath` — see the anchoring table in the overview's Shared Decisions, and do not collapse the three groups onto one base. The moved test files convert in this same card, not in batch 4: no batch-4 sweep card covers `internal/lyxcwd`'s own tests, and this batch's `go vet -tags integration ./internal/lyxcwd/...` gate type-checks them — so every one of their synthetic `Layout` literals and removed-field reads is rewritten to `Location` form here, under the same rules cards 13-17 apply elsewhere. Two of those files carry a change beyond the mechanical one. First, `raddle_guard_test.go:48` skips the scanned file by the literal basename `"hubgeometry.go"`; update the literal (and the comment at `:45`) to `"lyxcwd.go"`, or the guard scans the renamed file, finds the `_raddle` literals still in it (`:317`, `:518` pre-rename — they leave only in batch 6, cards 31 and 34), and fails at batch 4's first `go test ./...`. Second, the vet gate also type-checks the moved tests' dependency `internal/lyxtest`, whose `lyxtest.go` still names the old module — so this card retargets it too, pulled forward from card 10: the import path, the two `hubgeometry.Resolve` calls at `:473` and `:530` (and their error strings), and the exported `PairedFixture.Layout` field, which keeps its name and changes type to `*lyxcwd.Location` — the seam every fixture-consuming test reads through in cards 13-17. `lyxtest.go` reads no `Layout` fields, so the retarget is qualifier-and-type only; its own `lyxtest_test.go` waits for card 15, since nothing this batch's gate compiles depends on it. Rewrite `fabricengine/hostlayout.go`'s card-4 inline construction and `clone.go`'s step-5 hook literal into `&lyxcwd.Location{...}` form. Cards 6 and 7 then land the gate and the marker rename against the renamed paths, and cards 8-17 sweep the consumers.
- **Commit:** `refactor(lyxcwd)!: rename hubgeometry to lyxcwd and reshape Layout into Location`

### Card 6: strict cwd gate, path comparator and `ResolveWithAnchor`

- **Context:**
  - `internal/configengine/config.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/lyxcwd/anchor.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Creates:**
  - `internal/lyxcwd/gate_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extract two named helpers, both callable without spawning git so the tests stay untagged — the gate must be its own function rather than inline logic inside `Resolve`, which spawns `git rev-parse --show-toplevel` and could therefore only be exercised by a tagged test. `func samePath(a, b string) bool` normalizes each side through `filepath.EvalSymlinks` then `filepath.Clean`, falling back to `Clean`-only on whichever side `EvalSymlinks` fails for (the path may not exist yet), and compares byte-exact on Linux/macOS, `strings.EqualFold` on Windows. Normalization is not optional: the worktree side comes from `git rev-parse --show-toplevel` while cwd comes from `os.Getwd`, and the two disagree routinely — macOS `/tmp` is a symlink to `/private/tmp`, `lyxtest` fixtures live under symlinked temp dirs, and Windows/macOS filesystems are case-insensitive while Go string compare is not. `func checkCwdAnchorGate(cwd, anchorRel, worktreePath string) error` returns `nil` when `samePath(cwd, filepath.Join(worktreePath, anchorRel))` and otherwise wraps `ErrCwdOutsideAnchor` in a message naming both sides and the marker file. Replace the at-or-below gate at `internal/lyxcwd/lyxcwd.go` (pre-rename `hubgeometry.go:118-127`) with a call to it, and hoist the call out of the `if anchor, found := readRecordedAnchor(hub); found` block so it runs for unanchored repos too. Today `filepath.Rel(anchorAbs, cwd)` returning `internal/foo` passes, so cwd may sit arbitrarily deeper than the anchor and lyx then dies further downstream at `configengine.FindBaseDir` with `not initialized: _lyx/ directory not found`; strict equality turns a confusing late failure into an immediate accurate one. With the `"."` fallback from card 5 this also means lyx is accepted only at the worktree root for an unanchored repo, never in a subdirectory — a user-visible behaviour change that must be called out in the commit message. Add `func ResolveWithAnchor(cwd, anchor string) (*Location, error)`, resolving exactly as `Resolve` does but taking the anchor as a parameter and applying **no** gate; document it as a **bypass** — not a general-purpose resolver, and a caller reaching for it to escape a gate failure is misusing it, the correct fix being to stand in the anchored directory. It must stay ungated because both its callers stand somewhere the gate would reject: clone passes the freshly-cloned worktree root while the anchor may be a non-`"."` subpath, and `lyxtest` injects anchors into synthetic hubs. `ResolveWorktree` keeps applying no gate, unchanged. `gate_test.go` is untagged: a table over `(cwd, anchorRel, worktreePath)` triples asserting exact match resolves, a subdirectory errors, a parent errors and a sibling errors; a `samePath` table covering trailing separator, `.`/`..` segments, mixed separators, a symlinked path resolving to its target (temp dir) and a case-differing path that must match on Windows and must not on Linux; and one row per entry point asserting the same triple that makes the gate return `ErrCwdOutsideAnchor` is accepted without error by `ResolveWithAnchor`, so a later "consistency" change cannot quietly gate the bypass and break clone.
- **Commit:** `feat(hubgeometry): require cwd to equal the anchored directory exactly`

### Card 7: rename the anchor marker and simplify clone's step-5 resolve

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/hook.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:**
  - `internal/buildercli/weft_integration_test.go`
  - `internal/fabriccli/cli_test.go`
  - `internal/fabriccli/fabric.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/clone_adopt_test.go`
  - `internal/fabricengine/commit_integration_test.go`
  - `internal/fabricengine/doc.go`
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/unwire_test.go`
  - `internal/fabricengine/weftgit_exclude_test.go`
  - `internal/lyxcwd/anchor.go`
  - `internal/lyxcwd/anchor_test.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/perchcli/run_integration_test.go`
  - `internal/webstercli/weft_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two changes to the clone path, both small and both landing before the rename. First, rename the constant `FabricAnchorName` to `AnchorFileName` in `internal/lyxcwd/anchor.go` and its value from `.fabric-anchor` to `.lyx-anchor`, with **no** compatibility fallback read — the marker anchors the whole weft repo, not the fabric module, and `fabricengine/doc.go:110` already calls the value "the lyx-anchor subpath". In `internal/fabricengine/clone.go`, the step-8 marker path uses the new constant, and a leftover `.fabric-anchor` in `_board` with no `.lyx-anchor` beside it is a hard error naming re-clone as the remedy; without that detection the break would be silent, because an old clone would fall back to `AnchorRel = "."` and resolve `_lyx` to the wrong place for a subpath-anchored repo. Add that case to `clone_adopt_test.go`. Every listed test file writes or asserts the marker filename and takes the new constant. Second, at `clone.go:112` replace `hubgeometry.Resolve(hostWorktreePath)` with a direct struct construction from `hubPath` and `name`, already in scope at `:103`, because `InstallPostCheckoutHook` reads exactly one field (`hook.go:59`) — it needs a path, not a resolution. This is a **simplification, not a correctness fix**: step 3 aborts the clone if the hub already exists and step 4 creates it fresh, so at `:112` the hub is provably empty, `<hubPath>/_board` cannot exist, the marker read returns not-found, and the call succeeds today and would keep succeeding under the strict gate. Delete the now-unreachable non-fatal `else` branch at `:116-118`, which reads as a real failure path and is not one. `fabricengine/doc.go:110` names `.fabric-anchor`, `hubgeometry.BoardDir(Hub)`, `hubgeometry.Resolve`, the anchor file's pre-rename package path, `SiblingLayout` and `RelPath` in one sentence: correct the marker name, drop the `SiblingLayout` clause, and rename the package qualifiers and path to their `lyxcwd` forms (`lyxcwd.BoardDir(Hub)`, `lyxcwd.Resolve`, `internal/lyxcwd/anchor.go`, `AnchorRel`) — the qualifier rename is honest at this point because `BoardDir` still lives in `lyxcwd` until card 34, which re-points the wording to `fabricengine` when the symbol actually moves. The moved `internal/lyxcwd/lyxcwd.go` carries one more stale marker name this card owns: the `resolveCore` doc comment (pre-rename `hubgeometry.go:76`) says "reading the recorded `.fabric-anchor` marker for `RelPath`" — correct it to `.lyx-anchor`/`AnchorRel`, which is why the file is in this card's `Edits:`. Two references outside the write/assert set also carry the old name and no later card touches them: `fabriccli/fabric.go:266` names `.fabric-anchor` in a cobra `Long` — user-facing help whose accuracy is a review obligation under the CLI invariant — and `fabricengine/unwire.go:5` names it in a godoc comment; correct both to `.lyx-anchor` here.
- **Commit:** `feat(fabric): rename the anchor marker to .lyx-anchor with a stale-marker guard`


## Batch Tests

`verify: go build -tags integration ./internal/lyxcwd/... && go vet -tags integration ./internal/lyxcwd/...` — deliberately narrow, and the only narrow gate in this plan. The rest of the tree does not compile until batch 4 completes the consumer sweep, so a wider command would fail by construction rather than by defect. What this gate does prove is the thing this batch is responsible for: the renamed module and its own tests compile, vet cleanly, and the new `gate_test.go` type-checks.

The untagged `gate_test.go` created here is the load-bearing new coverage for the whole task — the strict-gate table (exact match resolves; a subdirectory, a parent and a sibling all error), the `samePath` normalization table (trailing separator, `.`/`..` segments, mixed separators, a symlinked temp dir, and a case-differing path that must match on Windows and must not on Linux), and the per-entry-point rows pinning `ResolveWithAnchor` and `ResolveWorktree` as ungated so a later "consistency" change cannot quietly gate the bypass and break clone. It runs for real at batch 4's `go test ./...`; here it only has to compile. The `-tags integration` flag is not decoration: `internal/lyxcwd`'s own `anchor_test.go` and `lyxcwd_test.go` are integration-tagged, so without it the gate would silently skip type-checking two of the module's test files. The tagged test edits this batch makes *outside* `internal/lyxcwd` — the marker-filename changes in `fabricengine`, `fabriccli`, `buildercli`, `webstercli` and `perchcli` — cannot be type-checked here, because those packages do not compile until batch 3 finishes the production sweep; they are covered by batch 4's repo-wide `go vet -tags "integration smoke scout" ./...`. `internal/lyxtest` is the one out-of-module package this gate does reach: the module's own tests import it, so the vet stage type-checks it as a dependency — which is exactly why card 5 retargets `lyxtest.go` in this batch rather than leaving it to the batch-3 sweep. The stale-`.fabric-anchor` detection lands in `internal/fabricengine/clone_adopt_test.go`, which is integration-tagged because it needs a real clone, and is likewise first executed under batch 4.
