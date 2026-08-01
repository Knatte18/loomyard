# Batch: hubgeometry-recorded-anchor

```yaml
task: 'fabric: clone-does-everything + subpath-in-weft + init dissolution'
batch: hubgeometry-recorded-anchor
number: 1
cards: 5
verify: go test -tags integration ./internal/hubgeometry/...
depends-on: []
```

## Batch Scope

This batch is the structural spine of the whole task: it makes `hubgeometry` resolve `RelPath` from the recorded `.fabric-anchor` marker on `weft:main` instead of trusting cwd positionally. It adds the anchor-read primitive (a new `anchor.go`), rewires both `Resolve` (with the cwd at-or-below hard gate + absent fallback) and `SiblingLayout` (same anchor read, no gate), updates the machine-pinned `TestSiblingLayout_EquivalentToResolve`, and records the extended invariant in `CONSTRAINTS.md`. Because ~27 call sites get `RelPath` transparently through `Resolve`/`SiblingLayout`, fixing both fixes them all — this is the primary TDD target and the risk is concentrated here.

External interface the later batches consume: the exported constant `hubgeometry.FabricAnchorName` (clone writes this file; batch 4) and the corrected `RelPath` resolution (every wiring/commit path in batches 2, 4, 5). `hubgeometry` must stay YAML-free (stdlib + `gitexec` only) — the marker is a plain single-line file read with `os.ReadFile`+`TrimSpace`.

Batch-local decision: the anchor-read helper is unexported (`readRecordedAnchor`) and shared by `Resolve` and `SiblingLayout`; only the filename constant and a new error sentinel are exported.

## Cards

### Card 1: Add the `.fabric-anchor` read primitive

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/hubgeometry/anchor.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/hubgeometry/anchor.go` in `package hubgeometry`, importing only stdlib (`os`, `path/filepath`, `strings`, `errors`). Add three exported/unexported symbols:
  1. `const FabricAnchorName = ".fabric-anchor"` — the plain marker filename at the `weft:main` root. Document it as the recorded lyx-anchor subpath marker (a structural geometry artifact, not config).
  2. `var ErrCwdOutsideAnchor = errors.New("cwd is outside the recorded fabric anchor subtree")` — the hard-error sentinel `Resolve` returns when cwd is not at or below the anchored subtree.
  3. `func readRecordedAnchor(hub string) (anchor string, found bool)` — reads `filepath.Join(BoardDir(hub), FabricAnchorName)` with `os.ReadFile`; on any error (absent board, absent marker, unreadable) returns `("", false)`; on success returns `(strings.TrimSpace(string(data)), true)`. An empty/whitespace-only marker after trim returns `("", false)` (treated as absent — never anchors to empty). This helper is YAML-free and does not spawn git.
  Do not modify `Resolve`/`SiblingLayout` yet — cards 2 and 3 wire them.
- **Commit:** `feat(hubgeometry): add .fabric-anchor read primitive`

### Card 2: `Resolve` reads the recorded anchor with a cwd at-or-below hard gate

- **Context:**
  - `internal/hubgeometry/anchor.go`
- **Edits:**
  - `internal/hubgeometry/hubgeometry.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `Resolve(cwd string) (*Layout, error)` (hubgeometry.go:107), after `hub := filepath.Dir(workTreeRoot)` and the existing `relPath, _ := filepath.Rel(workTreeRoot, cleanCwd)` (line 127), consult the recorded anchor before building the Layout:
  - Call `anchor, found := readRecordedAnchor(hub)`.
  - When `found`: set `relPath = anchor` (record wins). Then validate cwd is at or below `<workTreeRoot>/<anchor>`: compute `anchorAbs := filepath.Join(workTreeRoot, anchor)` and `rel, err := filepath.Rel(anchorAbs, cleanCwd)`; if `err != nil` OR `rel == ".."` OR `rel` begins with `".."+string(filepath.Separator)`, return `nil, fmt.Errorf("%w: cwd %s is not at or below %s", ErrCwdOutsideAnchor, cleanCwd, anchorAbs)`. When `rel` is `"."` or any descendant path (no leading `..`), the gate passes.
  - When `!found`: leave `relPath` as the existing cwd-derived value (today's behavior) — the absent-marker fallback for mid-clone, lyxtest synthetic hubs, and non-hub repos.
  Keep the gate scoped to this entry `Resolve(cwd)` only; do not add it to `SiblingLayout`. Update `Resolve`'s doc comment (the numbered "Steps" list, hubgeometry.go:93-106) to state step 6 now reads `.fabric-anchor` (record wins), applies the cwd at-or-below gate, and falls back to cwd when the marker is absent.
  **Also add a gate-free companion resolver** for internal callers that resolve *another* worktree's geometry from its root (not an acting cwd): factor the shared body into `func resolveCore(cwd string, applyGate bool) (*Layout, error)` (the git rev-parse + anchor-read + RelPath logic), define `Resolve(cwd) = resolveCore(cwd, true)`, and add exported `func ResolveWorktree(worktreeRoot string) (*Layout, error) = resolveCore(worktreeRoot, false)`. `ResolveWorktree` reads the same recorded anchor for `RelPath` but applies NO cwd at-or-below gate — its input is definitionally a worktree root providing geometry (which sits above a subpath anchor), so the gate would spuriously fire. Document that `ResolveWorktree` is the resolver `fabricengine.hostLayoutFor`'s non-sibling fallback must use (batch 2 rewires that call site), matching the discussion's gate-scope caveat ("the cwd hard-error gate applies only to the entry `Resolve(cwd)`, never to internal sibling-layout construction above a subpath anchor").
- **Commit:** `feat(hubgeometry): resolve RelPath from recorded anchor with cwd gate`

### Card 3: `SiblingLayout` reads the recorded anchor without the cwd gate

- **Context:**
  - `internal/hubgeometry/anchor.go`
- **Edits:**
  - `internal/hubgeometry/hubgeometry.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `func (l *Layout) SiblingLayout(worktreeRoot string) *Layout` (hubgeometry.go:186), replace the hardcoded `RelPath: "."` with the recorded anchor when present. Before building the returned `&Layout{...}`, compute `relPath := "."`; call `anchor, found := readRecordedAnchor(l.Hub)`; if `found`, set `relPath = anchor`. Use `relPath` in the returned struct's `RelPath` field. Do NOT apply the cwd at-or-below gate here — `SiblingLayout` derives another worktree's geometry from its root (which is above a subpath anchor) and must never hard-error on the gate (this is the `hostLayoutFor` fast path on the Reconcile/Status hot paths). Update `SiblingLayout`'s doc comment (hubgeometry.go:169-185) to state that `RelPath` now follows the recorded anchor (not always `"."`), staying byte-equivalent to `Resolve(worktreeRoot)` for a hub sibling, and that no cwd-legitimacy check is applied here.
- **Commit:** `feat(hubgeometry): SiblingLayout follows recorded anchor, no cwd gate`

### Card 4: Anchor resolution tests (Resolve gate + SiblingLayout equivalence)

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/anchor.go`
  - `internal/hubgeometry/siblinglayout_test.go`
  - `internal/hubgeometry/hubgeometry_test.go`
  - `internal/hubgeometry/testmain_test.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:**
  - `internal/hubgeometry/siblinglayout_test.go`
- **Creates:**
  - `internal/hubgeometry/anchor_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/hubgeometry/anchor_test.go` (first line `//go:build integration`, matching the other git-spawning hubgeometry tests) with table tests for `Resolve`:
  - root anchor (`.` written to `<BoardDir>/.fabric-anchor`) resolves `RelPath="."` from any cwd under the worktree root;
  - subpath anchor (`backend`) resolves `RelPath="backend"` when cwd is at `<root>/backend` and when at `<root>/backend/deeper`;
  - cwd outside the anchored subtree (a sibling `frontend/`, and the repo root above a `backend` anchor) returns an error wrapping `ErrCwdOutsideAnchor` (assert via `errors.Is`);
  - marker absent falls back to today's cwd-derived `RelPath` (no error).
  Build synthetic hubs with `lyxtest` helpers (respect the lyxtest Leaf Invariant — no feature-package imports); construct the `_board` worktree directory and write `.fabric-anchor` into it via `hubgeometry.FabricAnchorName`/`hubgeometry.BoardDir`. Add a `ResolveWorktree` case to `anchor_test.go` asserting the gate-free resolver returns `RelPath="backend"` for a subpath-anchored hub when called with the worktree root (which is ABOVE the anchor) and does **not** return `ErrCwdOutsideAnchor` — this is the exact geometry `hostLayoutFor`'s fallback hits, and its gate-free behavior is what distinguishes it from `Resolve`. In `internal/hubgeometry/siblinglayout_test.go`, extend `TestSiblingLayout_EquivalentToResolve` so it also seeds a non-root anchor (`backend`) and asserts `SiblingLayout(worktreeRoot).RelPath == "backend"` and stays byte-equivalent to `ResolveWorktree(worktreeRoot)` (the gate-free resolver, since a hub-sibling `SiblingLayout` and the internal fallback must agree; `Resolve(worktreeRoot)` with the gate would instead error for a subpath-anchored hub because the worktree root sits above the anchor — assert that gated-error case separately to pin the intended split). Keep existing equivalence assertions green for the root case (where `Resolve`, `ResolveWorktree`, and `SiblingLayout` all agree at `RelPath="."`).
- **Commit:** `test(hubgeometry): cover recorded-anchor RelPath resolution and gate`

### Card 5: Record the extended Hub Geometry Invariant in CONSTRAINTS.md

- **Context:**
  - `internal/hubgeometry/anchor.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `CONSTRAINTS.md`'s `## Hub Geometry Invariant` section (the bullet list under line 5), add a bullet recording that `RelPath` is now resolved from the recorded `.fabric-anchor` marker (read from `BoardDir(Hub)` via `readRecordedAnchor`), with cwd demoted to a validated at-or-below gate (`ErrCwdOutsideAnchor` when cwd is outside the anchored subtree) and a cwd-derived fallback when the marker is absent. State explicitly that the marker is a structural geometry artifact (a fixed per-repo anchor set once at create), NOT a config/env override — so the existing "geometry is structural, never config/env-overridable" rule still holds — and that `hubgeometry` stays YAML-free (the marker is a plain single-line file). Do not weaken or reword the existing bullets; append the new one. Keep one-line-per-paragraph markdown (no hard-wrap).
- **Commit:** `docs(constraints): record recorded-anchor RelPath resolution + cwd gate`

## Batch Tests

`verify: go test -tags integration ./internal/hubgeometry/...` runs the whole hubgeometry package (the `-tags integration` superset includes both the untagged unit tests — `hubgeometry_unit_test.go`, `geometry_test.go`, `enforcement_test.go`, `raddle_guard_test.go` — and the integration tests that spawn git: the new `anchor_test.go`, the edited `siblinglayout_test.go`, and `hubgeometry_test.go`). The whole-package scope is justified: this batch edits the single most-depended-on file in the repo (`hubgeometry.go`), and `enforcement_test.go`/`raddle_guard_test.go` are the machine guards that must stay green after touching geometry — a scoped single-file run would miss them. No cross-package regressions are expected at this batch (callers consume `RelPath` transparently); the repo-wide `done_gate` (`go test ./...`) is the final backstop.
