# Discussion: Reconcile stale design docs (stateless + weft model)

```yaml
task: Reconcile stale design docs (stateless + weft model)
slug: reconcile-stale-docs
status: discussing
parent: main
```

## Problem

The `docs/` tree for Loomyard (`lyx`) has rotted and started misleading work. The
root cause is structural, not a one-off slip: the mechanical per-module docs under
`docs/modules/*.md` (and most of `docs/shared-libs/*.md`) were written as
**pre-implementation plan drafts** and then kept around past landing. Once a module
ships, its implementation plus its tests become the source of truth (the *fasit*),
so the prose doc inevitably drifts away from the code. Two concrete drifts already
caused trouble: the docs still describe an **old deferred shared worktree registry**
(slug → path/branch/hub stored in `.lyx/local-state.json`, "lands with mux") even
though the design is now permanently **stateless** with no registry; and they carry
**pre-rename vocabulary** (Container instead of Hub, "main/first worktree" instead of
Prime). The fix is twofold: (1) adopt and write down a documentation-lifecycle
convention so this class of rot stops recurring, and (2) apply it now — delete the
landed mechanical docs, sweep and strengthen the durable docs, and relocate the
research/reference material that doesn't belong in `docs/modules/`.

**Why now:** stale docs already misled a running task thread. Loomyard is mid-flight
through a vocabulary rename (#005: Container→Hub, Prime) and an architecture shift
(stateless model; weft overlay superseding portals), so the docs are maximally
divergent from reality right now.

## Scope

**In:**

- **Delete** the mechanical per-module docs for **landed** modules:
  `docs/modules/board.md`, `docs/modules/worktree.md`, `docs/modules/ide.md`,
  `docs/modules/muxpoc.md`.
- **Delete** the mechanical `docs/shared-libs/*.md` whose content is fully
  reconstructable from code+tests: `git.md`, `lock.md`, `fsx.md`, `gitignore.md`,
  `state.md`.
- **Keep + sweep** the durable docs: `docs/overview.md`, `docs/roadmap.md`,
  `docs/shared-libs/README.md`, `docs/shared-libs/config.md`,
  `docs/shared-libs/paths.md`, `docs/modules/mux.md`.
- **Expand `docs/overview.md`** into the durable "what each module/lib is for" map,
  with a clear **done vs. not-done status marker per module**, and add a new
  **Documentation lifecycle** section stating the delete-on-landing convention.
- **Add a pointer** to that convention from `CONSTRAINTS.md`.
- **Move** the three unlanded mux research logs out of `docs/modules/` into a new
  `docs/research/` folder: `mux-exploration.md`, `mux-hooks-exploration.md`,
  `mux-proposal.md`.
- **Move** the upstream psmux manual into a dedicated reference folder:
  `docs/vendor/psmux_scripting.md` → `docs/reference/psmux_scripting.md`, and make it
  prominently linked as the upstream/vendored psmux command reference.
- **Fix every internal link** broken by the deletions/moves across all kept docs
  (overview.md, roadmap.md, README.md, mux.md, etc.).
- **Improve Go header/package doc-comments** (comments only, no logic) for the four
  modules whose detail docs are being deleted — `internal/board`, `internal/worktree`,
  `internal/ide`, `internal/muxpoc` — so the durable "what is this module for + key
  hazards" lives next to the code (esp. the worktree Windows locked-worktree teardown
  hazard and the stateless-by-design rationale).

**Out:**

- **No code logic changes.** Only Go *doc-comments* are touched; no functions, types,
  or behavior change. No tests are added or modified.
- **Portals code is NOT removed.** `internal/worktree/portals.go` and the
  `paths.Layout` portal methods stay; their removal is task 006 (weft engine). Docs
  describe portals as deprecated-but-present, not gone.
- **No weft code.** There is no `internal/weft` and none is created here.
- `docs/modules/mux.md` is **not deleted** — mux is an unbuilt module, so its plan
  draft legitimately lives in `docs/modules/` until the mux module lands, at which
  point the convention says delete it.
- **Untouched files:** `docs/benchmarks/*.md` (landed-board performance data, not
  plan-drafts), `docs/psmux-tui-behavior.md` (observed-behavior log).
- **Task item (c) is dropped as moot:** `.scratch/proposal-weft-repo.md` does not
  exist anywhere — not in the working tree, not in git history (`git log --all`), not
  in `.scratch/`. It was ephemeral and already consumed into the weft docs.

## Decisions

### doc-lifecycle-convention

- Decision: Adopt and document the rule **"delete each mechanical per-module doc
  (`docs/modules/<module>.md`) when its module lands; the implementation + tests are
  the fasit."** The durable docs are `docs/overview.md` (principles, naming, the
  module/lib map, the weft contract) and the not-yet-landed portion of
  `docs/roadmap.md`. Write the convention as a new **Documentation lifecycle** section
  in `overview.md`, and add a one-line pointer to it from `CONSTRAINTS.md`.
- Rationale: Mechanical per-module prose duplicates what code+tests already specify
  and therefore rots. Stopping the rot requires a written convention, not just a
  one-time cleanup. The user explicitly wants a single good overview file describing
  each module's job; detail docs are the thing that rots.
- Rejected: Keeping and reconciling the per-module docs (just defers the next rot);
  putting the convention only in CONSTRAINTS.md (CONSTRAINTS is for the build-enforced
  path invariant — the lifecycle convention is editorial, so overview.md is its home
  with a CONSTRAINTS pointer).

### delete-landed-mechanical-docs

- Decision: Delete `docs/modules/{board,worktree,ide,muxpoc}.md` and
  `docs/shared-libs/{git,lock,fsx,gitignore,state}.md`.
- Rationale: All describe landed code with tests. The agent map confirmed board,
  worktree, ide, muxpoc are implemented modules; `state`/`fsx` landed in `ba81abf`.
  Deleting them clears the #003 registry staleness and #005 rename staleness for free
  rather than reconciling doomed-to-rot prose.
- Rejected: Reconciling them in place (the brief's explicitly rejected path —
  guarantees re-rot).

### keep-sweep-durable-and-contract-docs

- Decision: Keep and sweep `overview.md`, `roadmap.md`, and the three shared-libs docs
  that encode a *contract* a human/agent must follow: `README.md` (the shared-libs
  index), `config.md` (the `_lyx/config` two-layer + `$env:…? default` grammar), and
  `paths.md` (the geometry + `os.Getwd`/`git rev-parse` enforcement-wall contract).
- Rationale: These three are not mechanical restatements of an API — they document
  authoring rules and invariants that overview.md/CONSTRAINTS.md lean on. The other
  five shared-libs docs are pure mechanics reconstructable from code+tests.
- Rejected: Keeping all eight shared-libs docs (perpetuates rot for the mechanical
  five); deleting all eight (loses the config grammar and the geometry contract index).

### overview-is-the-module-map

- Decision: Expand `overview.md` into the durable map of **what each module and shared
  lib is for**, with an explicit **done / not-done status marker per entry**. Weft is
  marked clearly as *designed, not yet built*. `roadmap.md` remains the milestone-level
  transition tracker (✅ Done markers).
- Rationale: Once the per-module docs are gone, overview.md is the only place a reader
  learns a module's purpose; it must carry that load and must not imply unbuilt things
  exist. The user wants done-vs-not-done legible at a glance, and noted the roadmap
  already does milestone-level done-marking.
- Rejected: Leaving overview.md's terse module list as-is (would leave a hole where the
  deleted docs were).

### weft-framing

- Decision: Keep `overview.md` describing the weft overlay model as the **decided
  target architecture**, but ensure both overview.md and roadmap.md make it
  unambiguous that **weft is not built yet** — portals are still the live mechanism
  until task 006 removes them. Strengthen overview's existing "Status" note. Mark
  modules done/not-done so the reader is never misled that weft code exists.
- Rationale: The design is decided (overview already documents it), but there is zero
  weft code and portals are still active in the implementation. Docs must state the
  decided design without implying it's running. The user's steer: overview should
  clearly mark what is done vs not done, and the roadmap tracks the transition.
- Rejected: Reverting overview to describe portals as canonical (contradicts the
  decided architecture); duplicating a heavy "current vs target" callout block (the
  status markers + roadmap entry are lighter and sufficient).

### code-headers-carry-the-why

- Decision: Move durable rationale that would be lost when deleting per-module docs
  into (a) overview.md (brief module purpose) and (b) the modules' own Go package/file
  **header doc-comments** — what the module is for and its key hazards. Applies to
  `internal/board`, `internal/worktree`, `internal/ide`, `internal/muxpoc`. Comments
  only; no logic.
- Rationale: The user's rule — important "why" belongs next to the code in its header,
  not in a separate doc that drifts. The standout case is `internal/worktree`: the
  *why* of the junction-aware teardown ordering (Windows locked-worktree hazard) and
  the stateless-by-design choice are not obvious from signatures alone.
- Rejected: Docs-only with no code-comment changes (loses the rationale, or defers it
  to a follow-up task the user did not want); rewriting only worktree (the other three
  also lose their purpose statement when their doc is deleted).

### relocate-research-and-reference

- Decision: Move the three unlanded mux research logs (`mux-exploration.md`,
  `mux-hooks-exploration.md`, `mux-proposal.md`) from `docs/modules/` to a new
  `docs/research/` folder. Move the upstream psmux manual from `docs/vendor/` to a
  dedicated `docs/reference/` folder (`docs/reference/psmux_scripting.md`) and link it
  prominently as the upstream/vendored psmux command reference. `docs/modules/mux.md`
  stays put (unbuilt module's design draft).
- Rationale: `docs/modules/` should hold module docs (built or actively-building),
  not background research. The mux logs are research that fed the design. The psmux
  manual is a pure upstream usage reference and deserves a clearly named home that's
  easy to find. `mux.md` is the planned module's design, not research, so it remains a
  module doc (to be deleted when mux lands).
- Rejected: Leaving research logs in `docs/modules/` (clutters the module namespace);
  deleting them (they're unlanded design/research, which the convention preserves);
  moving `mux.md` to research (it's a module design, not research).

### drop-proposal-item-c

- Decision: Drop task item (c) (fix `.scratch/proposal-weft-repo.md`); record as
  resolved-moot.
- Rationale: The file does not exist in the working tree, git history, or `.scratch/`.
- Rejected: Recreating it (no source; the weft design now lives in overview.md).

## Technical context

Loomyard is a Go toolkit; binary `lyx` (`github.com/Knatte18/loomyard`). Pure-docs task
plus Go doc-comment edits. Key facts mill-plan needs (verified by exploration):

- **Landed modules (code + `*_test.go`):** `internal/board`, `internal/worktree`,
  `internal/ide`, `internal/muxpoc`, plus shared libs `internal/{config,git,lock,output,paths,state,fsx,gitignore}`.
- **Unlanded:** no `internal/mux`, no `internal/weft`. The three `mux-*` research docs
  and `mux.md` are design/research only.
- **`internal/state` + `internal/fsx` already landed** in commit `ba81abf`
  ("Extract internal/fsx and build generic internal/state (#7)"). `internal/state` is a
  generic `WriteJSON[T]`/`ReadJSON[T]` helper with **no consumer yet** (only its own
  test imports it). → roadmap milestone 3 ("Deferred to land with mux") is **stale**:
  the lib exists; it just has no consumer. Reword, don't keep the "deferred" framing.
- **Portals still exist in code:** `internal/worktree/portals.go`
  (`createPortal`/`removePortal`, wired into `add.go`/`remove.go`), and
  `internal/paths/paths.go` methods `PortalsDir()` (~L120), `PortalLink(slug)` (~L131),
  `PortalTarget(slug)` (~L140). `paths.md` should keep documenting these as
  **deprecated-but-present** (removal = task 006), not delete them.
- **No worktree registry / `local-state.json` in code.** `lyx worktree list` is a thin
  wrapper over `git worktree list --porcelain` (`internal/worktree/list.go` →
  `internal/paths/worktreelist.go`). Every doc mention of a registry / `local-state.json`
  / "lands with mux" is stale and must go from kept docs.

Known stale spots in the **kept** docs that must be swept (from the exploration map):

- `docs/shared-libs/paths.md`: **L60** struct comment `// path to the main/first
  worktree checkout` → Prime vocabulary; **L58** `// top-level container directory` and
  L3/L14/L41 prose use lowercase "container" → Hub; the three portal methods stay,
  tagged deprecated.
- `docs/shared-libs/config.md`: layout diagram references `mux.yaml` (unlanded) and
  `.lyx/local-state.json` (L19-20, no code reads it) → remove/flag as unlanded; "config
  container" wording (L10).
- `docs/shared-libs/README.md`: opening line lists modules as "board, worktree, mux"
  though only muxpoc is shipped → reflect the landed set + status.
- `docs/overview.md`: Modules + "Other docs" sections link to the four module docs and
  five shared-libs docs being deleted, and to the three research docs being moved →
  rewrite those link blocks; the "Path Invariants" section and the weft section are
  current but the module map must gain explicit status markers.
- `docs/roadmap.md`: milestone 3 (state/fsx — now landed), milestone 4 (registry
  "deferred with internal/state", thin `list` wording), milestone 8 ("laid out from the
  worktree registry") → drop registry/lands-with-mux language; links to
  `modules/{board,worktree,ide,muxpoc}.md` and `modules/mux-hooks-exploration.md` →
  fix to deleted/moved targets.
- `docs/modules/mux.md`: terminology sweep (Container→Hub, Prime, drop registry); fix
  links to moved research docs and to the relocated psmux reference.

Files to create: `docs/research/` (3 moved files), `docs/reference/` (1 moved file).
Prefer `git mv` to preserve history.

## Constraints

From `CONSTRAINTS.md` (the one build-enforced invariant — unaffected by this task but
must not be contradicted by edits):

- All worktree/hub geometry resolves through `internal/paths` (`paths.Getwd()`,
  `paths.Resolve()`); raw `os.Getwd` and `git rev-parse --show-toplevel` are banned
  outside `internal/paths` and `cmd/lyx/main.go`, enforced by
  `internal/paths/enforcement_test.go`. Doc edits must keep describing this accurately;
  `paths.md` is kept partly because it documents this contract.

Task-specific constraints:

- **Doc-comments only in `.go` files** — no logic, signature, or test changes. After
  edits, `go build ./...`, `go vet ./...`, and `go test ./...` must still pass
  unchanged (comments don't affect them, but verify).
- The "Documentation lifecycle" convention written into overview.md must match the
  decision recorded here (delete-on-landing; durable = overview.md + unlanded roadmap).

## Testing

This is a docs + comments task — "testing" means verification, not new test code:

- **No new tests; no test edits.** Per `internal/paths/enforcement_test.go` and the
  rest of the suite, run `go build ./...`, `go vet ./...`, `go test ./...` after the
  doc-comment edits to confirm nothing broke (comment-only changes must not affect them).
- **Broken-link check:** after deletions/moves, grep all kept `docs/**/*.md` (and
  `CONSTRAINTS.md`, `README` files) for links pointing at deleted files
  (`modules/board.md`, `modules/worktree.md`, `modules/ide.md`, `modules/muxpoc.md`,
  `shared-libs/{git,lock,fsx,gitignore,state}.md`) and at moved files (the three
  `mux-*` logs, `vendor/psmux_scripting.md`). Expect zero stale targets.
- **Stale-term check:** grep the kept docs for `local-state.json`, `lands-with-mux` /
  "lands with mux", "main/first worktree", and the worktree-"registry" framing —
  expect zero in kept files (portals references are allowed only where tagged
  deprecated-but-present). Confirm capitalized old name "Container" (as the name for
  Hub) does not appear.
- **Manual read-through** of overview.md to confirm: every landed module is listed with
  a done marker; weft is marked not-built; the psmux reference and research folder are
  linked; the Documentation lifecycle section states the convention.

## Q&A log

- **Q:** Which `shared-libs/*.md` survive? **A:** Keep `README.md` + `config.md` +
  `paths.md`; delete `git.md`, `lock.md`, `fsx.md`, `gitignore.md`, `state.md`. The
  durable artifact the user wants is one good overview file of each module's job;
  detail docs rot.
- **Q:** `worktree.md` carries rationale (Windows teardown hazard, stateless-by-design)
  not in code prose — what happens to it? **A:** Move the durable "why" into overview.md
  *and* into the code's own header comments, then delete `worktree.md`. Important "why"
  belongs in the code header.
- **Q:** Where to write the doc-lifecycle convention? **A:** overview.md (new section) +
  a pointer from CONSTRAINTS.md.
- **Q:** How to frame weft when no weft code exists and portals are still live? **A:**
  Overview must clearly mark what is done vs not done; weft is the decided design but
  flagged not-built; the roadmap tracks the transition.
- **Q:** `proposal-weft-repo.md` (item c)? **A:** Drop it — the file doesn't exist
  anywhere; record as moot.
- **Q:** Include Go code-header doc-comments given the "pure docs" scope? **A:** Yes —
  package/file header comments for board, worktree, ide, muxpoc (comments only).
- **Q:** What to do with the three mux research logs? **A:** They don't belong in
  `docs/modules/` — move them to a separate `docs/research/` folder (background
  research).
- **Q:** Does `mux.md` move with the research? **A:** No — it's not research, it's a
  module that isn't built yet; it stays in `docs/modules/` and is deleted when the mux
  module is built.
- **Q:** The vendor psmux file? **A:** It's a pure upstream usage reference; give such
  reference files their own folder (`docs/reference/`) and make it prominent.
- **Q:** Benchmarks / psmux-tui-behavior? **A:** Their own thing — leave untouched.
