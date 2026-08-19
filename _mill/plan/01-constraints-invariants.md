# Batch: constraints-invariants

```yaml
task: "invariants and docs for the told-geometry rule"
batch: "constraints-invariants"
number: 1
cards: 3
verify: go test ./internal/lyxcwd/...
depends-on: []
```

## Batch Scope

This batch lands the two `CONSTRAINTS.md` edits that every other batch links to or depends on: a brand-new `## Told-Geometry Invariant` section (cards 1 and 2) and a reword of the existing `## Cwd Resolution Invariant` (card 3).
It is one batch because both edits touch the same file in the same region and share the same register rules, and because batches 4 and 5 cannot link to `../CONSTRAINTS.md#told-geometry-invariant` until the heading exists.

The external interface the later batches consume is the heading text `## Told-Geometry Invariant`, which anchors as `#told-geometry-invariant`.
That slug must not change.

Several `Context:` entries in this batch are package **directories** rather than files.
Read only what the requirement names — for a directory the requirement is almost always "confirm a file with this name does or does not exist here", not "read every file in it".

Batch-local decision: card 1 and card 2 write two halves of one new section and are separate cards purely so each carries a focused `Context:` allowlist.
Card 1 writes the section heading and its first four rule groups;
card 2 appends the enforcement-basis paragraph and the closing **Enforced by** line to the same section.
Card 2 must therefore run after card 1.

## Cards

### Card 1: Told-Geometry Invariant — tiers, split, adapter direction, mode trigger

- **Context:**
  - `internal/lyxcwd`
  - `internal/preflight`
  - `internal/preflight/predicates.go`
  - `internal/preflight/doc.go`
  - `internal/fabricengine`
  - `internal/loomengine`
  - `internal/hubgeom`
  - `internal/hubgeom/doc.go`
  - `internal/standalonegeom`
  - `internal/standalonegeom/doc.go`
  - `internal/treadleengine`
  - `internal/shedengine`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Insert a new top-level section whose heading line is exactly `## Told-Geometry Invariant`, placed immediately after the end of the existing `## Cwd Resolution Invariant` section and immediately before the existing `## Lyxdirs Single-Declarer Invariant` heading.
  The heading text is fixed — it anchors as `#told-geometry-invariant` and batches 4 and 5 link to that slug.

  Open the section with a one-or-two-sentence rule statement in the file's existing style (a bolded/plain rule line directly under the heading, matching how `## Cwd Resolution Invariant` and `## Shed Producer-Seam Invariant` open): an engine is handed the absolute paths it operates on and derives none of its own, so it runs identically inside a lyx hub and in a bare directory that is not a git repository.

  Then state four rule groups, in this order.

  **1. The three resolution tiers**, as a compact markdown table.
  Table cells stay on one line each (the semantic-line-break rule exempts table cells).
  The three rows are:
  tier 1 — geometry — `lyxcwd.Resolve` — cwd is the root of a git worktree, `AnchorRel` is whatever the recorded anchor marker says or `"."` when absent;
  tier 2 — fabric — `preflight.Check` with `fabricengine.Ready`/`Healthy`/`Clean`/`PrimeName` — fabric is wired here, junctions intact, warp and weft in sync, tree clean;
  tier 3 — orchestrator state — `loomengine.Preflight` — tiers 1 and 2 plus this orchestrator's own status seed.
  This is the one place in the section where warp/weft appear, and they appear because tier 2 is exactly where the two sides must be told apart.

  **2. The producer/orchestrator split.**
  A producer requires none of the three tiers.
  An orchestrator requires tier 3 and threads the extracted plain values down through its whole producer list.
  A standalone CLI invocation of a single producer **requires** none of the three, but its pre-run does *attempt* tier 1: `preflight.ResolveMode` calls `lyxcwd.Resolve` unconditionally and treats the result as a mode question rather than a precondition.
  Word this rule as "requires none of the three" and never as "never enters tier 1" — the latter is false in source and contradicts rule group 4.

  State `ResolveMode`'s two-way outcome explicitly rather than claiming a blanket degrade, because it refuses as well as degrades:
  it **degrades** to standalone on a successful resolve with no board-level lyx directory beside it;
  it **degrades** on `ErrNotAGitRepo`;
  it **degrades** on `ErrCwdOutsideAnchor` when the re-probe finds no hub geometry;
  it **refuses**, surfacing the original gated error verbatim, on `ErrCwdOutsideAnchor` inside a wired hub worktree's subdirectory, and on any other error class.
  Close the group with the governing rule the source states outright: a non-nil error means refuse, never a degrade to standalone.

  Do not cite `predicates.go` line numbers in the invariant text — line references rot.
  Verify each of the five outcomes above against `internal/preflight/predicates.go` before writing the group, and correct the plan's wording rather than the source if any of them no longer holds.

  **3. The adapter direction.**
  Where an engine takes a `Geometry` **struct**, `internal/hubgeom` (hub mode) and `internal/standalonegeom` (told mode) are its two sole constructors.
  Both depend on the engines;
  no engine imports either back.
  An engine that gains a `Geometry` struct adds a sibling constructor in each rather than deriving geometry inline at a call site or spawning a per-engine geometry package.

  Scope the rule to `Geometry` structs deliberately and say so, because two shipped packages use the other permitted shape — plain told values: `internal/treadleengine` is told `runDir` and `Profile.GateDir`, and `internal/shedengine` is told `StatusPath`/`LockPath`/`StatusLockPath`, neither through a geometry struct and neither with a `hubgeom`/`standalonegeom` constructor.
  State also that the pair is not symmetric: `standalonegeom.StencilsDir` has no `hubgeom` sibling, because hub mode resolves that directory through `fabricengine` instead.
  An unscoped "the two sole constructors of engine geometry" would be false on the day it lands.

  **4. The mode trigger.**
  `preflight.ResolveMode` is what a standalone-capable CLI's pre-run consults — never `preflight.Wired`, and never a bare `HubPresent`.
  Point at `internal/preflight/doc.go` for why each alternative is wrong rather than restating the reasoning here.

  Keep the whole section in the file's rules-only register: no rationale paragraphs, no incident narrative, no reference to any task number.
  Add a **See also** style cross-link back to the Cwd Resolution Invariant in the same style the gitrepo Client Boundary / gitexec Checked-Call pair already uses, stating that Cwd Resolution says who may resolve and Told-Geometry says who must be told instead.

  Do not add the **Enforced by** line in this card — card 2 writes it as part of the enforcement-basis paragraph.
- **Commit:** `docs(constraints): add the Told-Geometry Invariant — tiers, split, adapter direction, mode trigger`

### Card 2: Told-Geometry Invariant — the enforcement basis, named per package

- **Context:**
  - `internal/lyxcwd`
  - `internal/tokenvocab/leaf_enforcement_test.go`
  - `internal/pattern/leaf_enforcement_test.go`
  - `internal/buildinfo/leaf_enforcement_test.go`
  - `internal/standalonestate/leaf_enforcement_test.go`
  - `internal/shedengine/seam_enforcement_test.go`
  - `internal/treadleengine/seam_enforcement_test.go`
  - `internal/shuttleengine/seam_enforcement_test.go`
  - `internal/scoutengine/seam_enforcement_test.go`
  - `internal/planparser`
  - `internal/configengine`
  - `internal/shuttleengine`
  - `internal/reedengine`
  - `internal/burlerengine`
  - `internal/perchengine`
  - `internal/websterengine`
  - `internal/scoutengine`
  - `internal/hubgeom`
  - `internal/hubgeom/hubgeom.go`
  - `internal/hubgeom/webstergeom.go`
  - `internal/standalonegeom`
  - `internal/batcher`
  - `internal/stencilstore`
  - `internal/shedadapters`
  - `internal/logger`
  - `internal/pattern`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Append the enforcement-basis material to the `## Told-Geometry Invariant` section card 1 created, ending with the section's **Enforced by** line in the same bolded style every other section in the file uses.

  **State the membership predicate first**, so a future task can re-derive the two lists below rather than guess at them.
  A package is *bound* by this invariant when it takes the absolute paths it operates on from its caller and has no **direct** production import of `internal/lyxcwd`.
  It is *machine-enforced* when that direct non-import is asserted by a test in its own package that polices its production import set — an allowlist that omits `internal/lyxcwd`, or a banned list that names it.
  Otherwise it is a *review obligation*.

  **The direct/transitive qualifier is load-bearing and must be stated**, matching how the Treadle Runner-Seam and Shed Producer-Seam Invariants already word their own allowlists.
  Two of the six machine-enforced packages reach `internal/lyxcwd` transitively and are still correctly bound: `internal/treadleengine` reaches it through `internal/logger` and `internal/shuttleengine`, and `internal/pattern` reaches it through `internal/stencilstore` then `internal/logger`.
  Phrasing the predicate, or the "genuinely excludes `internal/lyxcwd`" claim, without the direct/transitive qualifier would make both entries wrong.

  **Say plainly that the predicate is what binds and the two lists are not exhaustive.**
  The lists enumerate the packages converted by the producers-standalone waves.
  Name `internal/batcher`, `internal/stencilstore`, and `internal/shedadapters` as packages that satisfy the predicate today by other routes — bound, unconverted by this line of work, and review obligation.

  **The machine-enforced list**, enumerated exactly, each with its test file and test function:
  `internal/tokenvocab/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`),
  `internal/pattern/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`),
  `internal/buildinfo/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`),
  `internal/standalonestate/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`),
  `internal/shedengine/seam_enforcement_test.go` (`TestProducerSeamInvariant_AllowlistOnly`),
  `internal/treadleengine/seam_enforcement_test.go` (`TestRunnerSeamInvariant_AllowlistOnly`).

  **The review-obligation list**, enumerated exactly — no machine guard for the told-geometry property:
  `internal/planparser`, `internal/configengine`, `internal/shuttleengine`, `internal/reedengine`, `internal/burlerengine`, `internal/perchengine`, `internal/websterengine`, `internal/scoutengine`.

  Note within that list, in one clause each, why the two packages a reader would assume are covered are not:
  `internal/shuttleengine/seam_enforcement_test.go` (`TestProviderSeamImportRule`) polices the **provider** seam — Claude specifics confined to `claudeengine` — and references `lyxcwd` nowhere;
  `internal/scoutengine/seam_enforcement_test.go` (`TestEngineSeamInvariant_BannedImports`) polices a **banned list** that does not name `internal/lyxcwd` either.
  Both packages' `doc.go` files assert `internal/lyxcwd` is absent from their production imports;
  that assertion is true today and unguarded.

  **Carve the two geometry adapters out into neither list, explicitly.**
  `internal/hubgeom` and `internal/standalonegeom` are *tellers*, not told packages — they exist to construct the geometry the engines are handed.
  The non-import predicate does not bind them and cannot: `internal/hubgeom` imports `internal/lyxcwd` in production (in `internal/hubgeom/hubgeom.go` and `internal/hubgeom/webstergeom.go`), which is precisely its job as the hub-mode adapter, and `internal/standalonegeom` builds from told strings for the same reason in the other mode.
  What binds both is card 1's adapter-direction rule instead: they depend on the engines and no engine imports either back.
  Say that this direction is a **review obligation** too, and that it is a *different* obligation from the non-import one — conflating them would make the invariant self-contradictory on its own first example.

  Name the follow-up candidate in one clause: adding the missing import-allowlist entries to the eight review-obligation packages is a separate task, each needing its own transitive-closure reasoning.
  Do not add any such allowlist entry in this task.

  Close with the **Enforced by** line naming the six tests above for the machine-enforced half, and stating that the review-obligation half and the adapter-direction rule have no machine check.

  Before writing, verify against the tree that each of the six named test functions exists at the named path and that its allowlist or banned list genuinely omits or names `internal/lyxcwd`, and that the eight review-obligation packages carry no such guard.
  If any claim is false, correct the invariant text to match the tree rather than writing the claim as specified.
- **Commit:** `docs(constraints): name the Told-Geometry Invariant's enforcement basis per package`

### Card 3: Cwd Resolution Invariant — state what `Resolve` validates

- **Context:**
  - `internal/lyxcwd`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxcwd/anchor.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add one new bullet near the top of the existing `## Cwd Resolution Invariant` section — after the opening rule line and among the first few bullets, where a reader meets it before the per-token ownership detail.
  Change no existing rule: no gate is relaxed, no bullet is deleted, and the section's substance is unchanged.

  The bullet states, in four sub-points, what `lyxcwd.Resolve` validates:

  1. `git rev-parse --show-toplevel` must succeed at `cwd`, else `ErrNotAGitRepo` — the only validation `Resolve` makes of the **repository** itself.
     Keep the word "only" scoped to the repository;
     sub-points 2 and 3 are genuine checks too, but they are checks about the anchor marker and the caller's position, not about whether the repository is a lyx worktree.
  2. An **absent** anchor marker is not an error — `AnchorRel` falls back to `"."`.
     Only a stale pre-rename marker hard-errors, with `ErrStaleAnchorMarker`.
  3. `cwd` must equal `Join(worktreeRoot, AnchorRel)`, else `ErrCwdOutsideAnchor`;
     with no marker this reduces to "cwd is the git worktree root".
  4. `HubPath` is `filepath.Dir(worktreeRoot)` **unconditionally** — never verified to be a hub — and `RepoName` is `Base(hubPath)` with a `-HUB` suffix trimmed, with no check the suffix was ever there.

  Close the bullet with the consequence, which is the whole point of the reword: `Resolve` succeeds in any ordinary git repository run from its root, and `HubPath`/`RepoName` are fiction in that case.
  Proving a worktree is lyx-initialized and Fabric-wired is tier 2's and tier 3's job, not tier 1's.

  Cross-link to the new `## Told-Geometry Invariant` section for the tier map.

  Verify all four sub-points against `internal/lyxcwd/lyxcwd.go` and `internal/lyxcwd/anchor.go` before writing.
  If a sub-point does not match the source, write what the source does and flag the divergence in the commit body rather than writing the specified text.
- **Commit:** `docs(constraints): state what lyxcwd.Resolve validates, and what it does not prove`

## Batch Tests

`verify: go test ./internal/lyxcwd/...` runs the two guards that can react to a `CONSTRAINTS.md` edit: `internal/lyxcwd/docslink_test.go` (`TestEnforcement_MarkdownLinks`), which resolves the file part and `#anchor` of every inline markdown link under `manifest/` and `docs/` — including the existing `docs/overview.md` links that point into `CONSTRAINTS.md` — and `internal/lyxcwd/enforcement_test.go`, whose geometry-literal and Fabric-vocabulary walks confirm nothing in this batch's prose trips a guarded token in the files those walks do cover.

`CONSTRAINTS.md` is not itself a link **scan source** (the scan sources are `manifest/` and `docs/` only), so this batch adds no new outgoing-link coverage of its own;
what the guard proves here is that the new `## Told-Geometry Invariant` heading is present and correctly slugged for batches 4 and 5 to link to, and that no existing inbound link into `CONSTRAINTS.md` was broken by the edit.

The overview's module-wide `verify: go build ./...` catches nothing in this batch (no Go file is touched) and is a no-op cost of a second or two.
