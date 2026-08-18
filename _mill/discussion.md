# Discussion: invariants and docs for the told-geometry rule

```yaml
task: invariants and docs for the told-geometry rule
slug: standalone-docs-and-invariants
status: discussing
parent: standalone-producers
```

## Problem

The producers-standalone line of work (T1–T9, all merged) converted every producer package to *told geometry*: an engine is handed the absolute paths it needs and derives none of its own, so it runs identically inside a lyx hub and in a bare directory that is not a git repository.
The rule that work established is now true of the whole tree, but it is written down nowhere as a rule — it lives scattered across a dozen package doc comments and one design doc that is scheduled for deletion.
A cross-cutting structural rule with no home in `CONSTRAINTS.md` is a rule the next task will break without noticing.

Two further things are wrong today, and this is the task that fixes them.
The **Cwd Resolution Invariant**'s current text leaves room for the exact misreading the whole initiative had to correct — that `lyxcwd.Resolve` proves a worktree is lyx-initialized and Fabric-wired.
It does not: it succeeds in any ordinary git repository run from its root, and hands back a `Location` whose `HubPath` and `RepoName` are fiction in that case.
And `manifest/designs/producers-standalone.md` must be deleted per the Documentation Lifecycle now that its last wave ships, which breaks the five markdown links pointing at it from `manifest/roadmap.md` and would fail `internal/lyxcwd/docslink_test.go`.

**Why now:** the last code wave (`the standalone CLI path`, commit `828f65d3`, plus the optional scout uniformity pass `8aced4cb`) has landed on `standalone-producers`.
T10 was gated on T5 in particular — the three-tier rule is only true once the orchestrator-agnostic preflight exists, and `internal/preflight` now does.

## Scope

**In:**

- A new named invariant in `CONSTRAINTS.md` — the **Told-Geometry Invariant** — stating the three resolution tiers, the producer/orchestrator split, and the one-way geometry-adapter direction, with its enforcement basis named honestly per package.
- A reword of the existing **Cwd Resolution Invariant** in `CONSTRAINTS.md`, adding what `lyxcwd.Resolve` actually validates.
  Substance unchanged: no rule relaxed, no gate loosened.
- `docs/overview.md`: an accuracy sentence plus a pointer in its own Cwd Resolution Invariant section, the three missing packages added to the shared-infrastructure list, and a standalone-mode paragraph in the Execution stack section.
- A `doc.go` audit across the converted packages: add one told-geometry sentence naming the package's tier where absent; leave existing told-geometry prose alone.
- `internal/buildinfo/doc.go`: reword its prose reference to the about-to-be-deleted design doc.
- `manifest/roadmap.md`: move the Planned `producers standalone: invariants and docs` item to the head of Done, and reword the four existing producers-standalone Done entries so none links to the deleted design doc.
- Delete `manifest/designs/producers-standalone.md`.
- Implement the **Config Strictness Invariant**'s set-equality grep guard, which `CONSTRAINTS.md` explicitly names T10 (this task) as the home for, and flip that invariant's **Enforced by** line from review obligation to the new test.

**Out:**

- **Changing what `internal/lyxcwd` owns, or what `Resolve` validates.**
  The reword makes the existing text accurate; it does not relax the gate.
  No production change to `internal/lyxcwd`.
- **Adding new import-allowlist tests to close the enforcement gaps this task documents.**
  The gaps are recorded honestly in the invariant text as review obligations; converting one into a machine guard is its own task with its own risk of false positives.
  The one exception is the Config Strictness guard, and only because `CONSTRAINTS.md` already names this task as its home and carries its full specification.
- **The Stencil Ownership and Durable-vs-Ephemeral State rewords.**
  Those landed in T8's own commit alongside the code that made them true, per T8's brief.
- **Any production Go change.**
  Every code edit in this task is a doc comment or a new `_test.go` guard.
- **Recording a scout deviation.**
  T9 (the optional uniformity pass) shipped, so there is no deviation left to record.
- **Rewriting `manifest/designs/producers-standalone.md`'s rationale into a new durable doc.**
  Its durable content already lives in `internal/preflight/doc.go`, `internal/hubgeom/doc.go`, and `internal/standalonegeom/doc.go`; the rule form goes to `CONSTRAINTS.md`.

## Decisions

### Told-Geometry Invariant — name and placement

- **Decision:** a new top-level section `## Told-Geometry Invariant` in `CONSTRAINTS.md`, placed immediately after `## Cwd Resolution Invariant` and before `## Lyxdirs Single-Declarer Invariant`.
- **Rationale:** it is the Cwd Resolution Invariant's generalization — Cwd Resolution says *who may resolve*, Told-Geometry says *who must be told instead of resolving*.
  Adjacency is the map a reader needs; the file's existing order already groups related rules this way (the gitrepo Client Boundary / gitexec Checked-Call pair carries explicit **See also** cross-links for the same reason).
- **Rejected:** folding it into the Cwd Resolution Invariant as a sub-bullet list — it would bury a cross-cutting rule that binds fifteen packages inside a rule about one package.
  Appending it at file end — it would sit beside the Documentation Lifecycle, semantically unrelated.

### Told-Geometry Invariant — content

- **Decision:** the invariant states four things, in `CONSTRAINTS.md`'s rules-only register (no rationale, no incident narrative):
  1. **The three tiers**, as a compact table: tier 1 geometry (`lyxcwd.Resolve` — cwd is the root of a git worktree, `AnchorRel` is whatever the marker says or `"."`), tier 2 fabric (`preflight.Check`/`fabricengine.Ready`/`Healthy`/`Clean`/`PrimeName` — fabric is wired here, junctions intact, warp and weft in sync, tree clean), tier 3 orchestrator state (`loomengine.Preflight` — tiers 1 and 2 plus this orchestrator's own status seed).
  2. **The split:** a producer requires none of the three.
     An orchestrator requires tier 3 and threads the extracted plain values down through its whole producer list.
     A standalone CLI invocation of a single producer never enters tier 1 at all.
  3. **The adapter direction:** `internal/hubgeom` (hub mode) and `internal/standalonegeom` (told mode) are the two sole constructors of engine geometry structs.
     Both depend on the engines; no engine imports either back.
     A new engine adds a sibling constructor in each rather than deriving geometry inline at a call site or spawning a per-engine geometry package.
  4. **The mode trigger:** `preflight.ResolveMode` is what a standalone-capable CLI's pre-run consults — never `preflight.Wired`, and never a bare `HubPresent` (see `internal/preflight/doc.go` for why each alternative is wrong).
- **Rationale:** these are exactly the four facts a future task can violate silently.
  Points 3 and 4 are not in the task brief but are load-bearing parts of the same rule — without 3 the told direction inverts into a cycle, and without 4 a CLI silently relocates a live hub's state into the per-OS standalone state directory.
- **Rejected:** stating only the three tiers per the brief's literal wording — it would leave the two mechanisms that make the tiers real unpinned.

### Enforcement basis — named honestly, per package

- **Decision:** the invariant's **Enforced by** line enumerates the machine-checked set exactly, and names the review-obligation set exactly rather than gesturing at it.

  **Machine-enforced** (an import allowlist that genuinely excludes `internal/lyxcwd`):
  `internal/tokenvocab/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`),
  `internal/pattern/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`),
  `internal/buildinfo/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`),
  `internal/standalonestate/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`),
  `internal/shedengine/seam_enforcement_test.go` (`TestProducerSeamInvariant_AllowlistOnly`),
  `internal/treadleengine/seam_enforcement_test.go` (`TestRunnerSeamInvariant_AllowlistOnly`).

  **Review obligation** (no machine guard for the told-geometry property):
  `internal/planparser`, `internal/configengine`, `internal/shuttleengine`, `internal/reedengine`, `internal/burlerengine`, `internal/perchengine`, `internal/websterengine`, `internal/scoutengine`, `internal/hubgeom`, `internal/standalonegeom`.

- **Rationale:** verified against the tree during exploration, and two packages are *not* what the design doc's phrasing implies.
  `internal/shuttleengine/seam_enforcement_test.go` (`TestProviderSeamImportRule`) polices the **provider** seam — Claude specifics confined to `claudeengine` — and contains no `lyxcwd` reference of any kind.
  `internal/scoutengine/seam_enforcement_test.go` (`TestEngineSeamInvariant_BannedImports`) polices a **banned list** of `internal/output`, cobra, `internal/clihelp`, and `internal/*cli`, which does not mention `internal/lyxcwd` either.
  Both packages' `doc.go` files assert `internal/lyxcwd` is absent from their production imports;
  that assertion is true today and unguarded.
  Writing "an import-allowlist test per producer package where one exists" without enumerating is precisely the vague phrasing that let this gap sit unnoticed.
- **Rejected:** adding the missing allowlist entries to close the gap in this task — a code change per package, each needing its own transitive-closure reasoning, and out of scope for a docs-and-invariants consolidation.
  Named as a follow-up candidate in the invariant text instead.

### Cwd Resolution Invariant — the reword

- **Decision:** add one new bullet near the top of the existing `## Cwd Resolution Invariant`, stating what `Resolve` validates, in four sub-points:
  `git rev-parse --show-toplevel` must succeed at `cwd`, else `ErrNotAGitRepo` — this is its only real validation;
  an **absent** anchor marker is not an error (`AnchorRel` falls back to `"."`), only a stale pre-rename marker hard-errors;
  `cwd` must equal `Join(worktreeRoot, AnchorRel)`, which with no marker reduces to "cwd is the git worktree root";
  `HubPath` is `filepath.Dir(worktreeRoot)` **unconditionally**, never verified to be a hub, and `RepoName` is `Base(hubPath)` with `-HUB` trimmed, with no check the suffix was ever there.
  Close with the consequence: `Resolve` succeeds in any ordinary git repository run from its root, and `HubPath`/`RepoName` are fiction in that case — proving initialization is tier 2/3's job, not tier 1's.
  Cross-link to the new Told-Geometry Invariant.
- **Rationale:** the existing bullets are individually accurate but collectively silent on what `Resolve` does *not* prove, which is what readers inferred.
  Stating the negative explicitly is the whole point of the reword.
  The existing bullets about `ErrCwdOutsideAnchor`, the ungated variants, the `"."`/stale-marker split, and the import cap already carry these facts in fragments — the new bullet consolidates the reader-facing conclusion rather than replacing them.
- **Rejected:** a one-sentence caveat — too weak to displace an inference this durable.
  Rewriting the whole section — the design doc explicitly scopes out changing this invariant's substance, and a rewrite invites accidental substance drift.

### `docs/overview.md` — three targeted edits

- **Decision:**
  1. In `## Cwd Resolution Invariant` (line ~63): add the accuracy sentence (`Resolve` proves cwd is the root of a git worktree and nothing more) and a pointer to `CONSTRAINTS.md`'s new Told-Geometry Invariant for the tier map.
  2. In the shared-infrastructure sentence at the end of `## Modules`: add `internal/preflight`, `internal/hubgeom`, and `internal/standalonegeom` to the parenthesised list, each with the same one-clause gloss style the existing entries use.
  3. In `## Execution stack (orchestration layers)`: add a paragraph stating that every layer from `reed` up is told its geometry, that `hubgeom`/`standalonegeom` are the two constructors that tell it, and that a producer verb therefore runs in a non-repository directory.
- **Rationale:** the execution-stack description *did* change — the stack now has two entry modes, and the doc describes only one.
  The shared-infrastructure list is a map, and three packages that exist are missing from it.
- **Rejected:** touching only the Cwd Resolution section — leaves the module map wrong.
  No change at all — the brief's "if the execution stack description changed" condition is met.

### `doc.go` audit — additive, not a rewrite

- **Decision:** for each converted package, confirm its `doc.go` carries one sentence naming which tier it sits in and whether it is told or resolves.
  Add the sentence where absent;
  where told-geometry prose already exists (`shuttleengine`, `reedengine`, `pattern`, `perchengine`, `websterengine`, `hubgeom`, `standalonegeom`, `planparser`, `scoutengine`), leave it alone.
  `internal/configengine`, `internal/webstercli`, and `internal/scoutcli` have no `doc.go` at all — do not create one; their told-geometry status is covered by the invariant, and creating a package doc file is a larger editorial act than this task's brief carries.
- **Rationale:** these doc comments are the durable home for the rationale the deleted design doc held.
  A blanket rewrite for uniform wording would churn nine files' worth of already-correct prose for no reader gain and would make the diff unreviewable.
- **Rejected:** rewriting for uniformity — cost with no benefit.
  Skipping the pass — the brief names `doc.go` of each converted package explicitly.

### The Config Strictness set-equality guard — implemented here

- **Decision:** implement it, as `cmd/lyx/configstrictness_test.go`, per the specification already written in `CONSTRAINTS.md`'s Config Strictness Invariant.
  Add its `allowedSpawners` entry in `cmd/lyx/tierpurity_test.go`, and flip that invariant's **Enforced by** line from review obligation to the new test while keeping the known-blind-spot bullets.
- **Rationale:** `CONSTRAINTS.md` states outright that the guard has "a set-equality grep guard named as a candidate and **T10 named as its home**", and then records the guard's full shape "so T10 inherits a specification rather than re-deriving one".
  This task *is* T10.
  Deferring would still require editing that invariant to move the guard's home, so the cheaper edit is to build it.
  Flagging the scope note plainly: the wiki brief does not mention this guard;
  `CONSTRAINTS.md` is the authoritative file and it assigns the work here.
- **Rejected:** deferring to a new task — an edit to the same invariant either way, with the guard left unbuilt.
  Dropping the guard — would discard a written specification for no reason.

### Design-doc deletion and the five dangling links

- **Decision:** delete `manifest/designs/producers-standalone.md`, and in the same commit reword all five referencing lines in `manifest/roadmap.md`:
  the Planned item (line 12–14) moves to the head of `## Done` with its `See` line repointed at `CONSTRAINTS.md`'s Told-Geometry Invariant;
  the four existing Done entries (lines ~107, ~110, ~113, ~116) each drop the `See [designs/producers-standalone.md](...) — the doc survives this task because …` clause, repointed at the new invariant and the relevant package documentation.
- **Rationale:** mandatory, not optional — `internal/lyxcwd/docslink_test.go` resolves every inline markdown link's file part under `manifest/` and `docs/`, so five dangling links fail the build.
  The "the doc survives this task because …" clauses are now false statements regardless.
- **Rejected:** keeping the design doc — the Documentation Lifecycle deletes a module-design doc when its module lands, and every wave has landed.
  Deleting only the link text and leaving the prose — leaves prose referring to a file that no longer exists.

### `internal/buildinfo/doc.go`'s stale reference

- **Decision:** reword line 5's `the producers-standalone design doc names` to name the rule rather than the deleted doc (e.g. attribute the `StencilMode()` naming to the earlier design rather than to a file path).
- **Rationale:** a prose mention, not a markdown link, so `docslink_test.go` does not catch it — but it is a pointer to a file that will not exist after this commit.
- **Rejected:** leaving it — a dangling reference either way.
  Deleting the sentence — it carries a real "why the accessor is named this and not that" fact worth keeping.

### Scout's deviation — none to record

- **Decision:** record no scout deviation.
  Note instead, in the Told-Geometry Invariant's enforcement paragraph, that `scoutengine`'s guard is a banned list that does not cover `internal/lyxcwd`, so scout sits in the review-obligation set.
- **Rationale:** T10's brief says "record scout's remaining deviation **if T9 was skipped**".
  T9 was not skipped — commit `8aced4cb` (`scoutengine told-geometry (optional uniformity pass)`) landed it.
  `internal/scoutengine`'s only remaining `lyxcwd` mentions are two comments (`doc.go` explaining the deliberate absence, `toolchain.go` explaining a machine-global toolchain-cache path hand-joined directly);
  a machine-global cache path is not anchor geometry, so it is not a deviation.
- **Rejected:** recording a deviation anyway — would state something false.

## Technical context

**Verified state of the tree** (all figures checked during exploration, not inherited from the design doc):

- **Producer engines carry no `internal/lyxcwd` production import.**
  `planparser`, `reedengine`, `websterengine`, `shuttleengine`, `scoutengine`, `burlerengine` (`geometry.go:10`), `perchengine` (`geometry.go:9`), and `standalonestate` each mention it in comments only — the two `geometry.go` comments say `*lyxcwd.Location, but is deliberately not imported here`, which is the contract statement, not an import.
  `configengine`, `tokenvocab`, `pattern`, `buildinfo`, and `standalonegeom` do not mention it at all.
  Note for anyone re-verifying: a grep for the import path `internal/lyxcwd` misses both `geometry.go` comments, which spell it `*lyxcwd.Location` — grep the bare token `lyxcwd`.
- **The orchestrator/CLI tier legitimately imports it:** `internal/preflight` (`predicates.go`, `preflight.go`), `internal/webstercli` (`wiring.go`, `cli.go`), `internal/burlercli` (`wiring.go`, `cli.go`), `internal/perchcli` (`wiring.go`, `cli.go`), `internal/scoutcli` (`cli.go`), `internal/hubgeom`.
  This is exactly the split the invariant describes, so the invariant is stating a fact about the current tree rather than an aspiration.

**Key files:**

- `CONSTRAINTS.md` — 579 lines, 39 `##` sections.
  `## Cwd Resolution Invariant` at line 8;
  `## Lyxdirs Single-Declarer Invariant` at line 47 (the new section goes between them);
  `## Config Strictness Invariant` at line 499.
  File register: rules only, no rationale, no incident narratives, no historical justification — stated in its own header at lines 3–6 and binding on the new section.
- `docs/overview.md` — 435 lines.
  `## Cwd Resolution Invariant` at line 63;
  `## Documentation lifecycle` at line 86;
  `## Modules` at line 266 with the shared-infrastructure sentence at its end;
  `## Execution stack (orchestration layers)` at line 320.
- `manifest/roadmap.md` — 231 lines.
  `## Planned` at line 8 with the single item at 12–14;
  `## Done` at line 105, newest-first.
- `manifest/designs/producers-standalone.md` — 713 lines, deleted.
  Its `## The three resolution tiers` section (lines 18–54) is the source for the invariant's tier table;
  its `### Wave 5 — consolidation` (lines 665–687) is this task's own spec.
- `internal/preflight/doc.go` — already the durable home for tier-1/tier-2 rationale, the report-not-error contract, and the `HubPresent`/`Wired`/`ResolveMode` three-function split.
  The invariant points here rather than restating it.
- `internal/hubgeom/doc.go` and `internal/standalonegeom/doc.go` — already state the one-way told direction explicitly.
  The invariant's adapter-direction point is a rule-form restatement, deliberately, since `CONSTRAINTS.md` is where a reviewer looks.

**The Config Strictness guard's inherited specification** (quoted from `CONSTRAINTS.md`, to be implemented verbatim):
follow `cmd/lyx/gitrepoboundary_test.go`'s pinned-set style;
walk non-test `*.go` files under the module root;
collect every package directory containing a `configengine.Load(` call and every one containing a `configengine.LoadOrTemplate(` call;
compare each collected set against its pinned set;
exclude `internal/configengine` itself as the declaration site;
skip `_test.go` files.
The pinned sets as `CONSTRAINTS.md` records them: degrading is `{shuttleengine, reedengine, perchengine, websterengine, batcher}`, strict is `{fabricengine, boardengine, loomengine}`.
Resolving the scan root through `go env GOMOD` spawns a process, so the guard must be allowlisted in `cmd/lyx/tierpurity_test.go`'s `allowedSpawners` map with a one-line reason in the style of the fourteen entries already there (see `cmd/lyx/tierpurity_test.go:28-43`).
The three own-loader modules (`burlerengine`, `modelspec`, `scoutengine`) call neither entry point and are structurally invisible to a substring scan — they need no exclusion, but the guard's doc comment should say so, since the invariant's own text is careful about it.
**If the collected sets do not match the pinned sets when the guard first runs**, the pinned sets in both the test and `CONSTRAINTS.md` are what is wrong — verify against the tree and correct both in the same commit rather than loosening the assertion.

**Guards this task must not trip:**

- `internal/lyxcwd/docslink_test.go` — every inline markdown link's file part *and* `#anchor` under `manifest/` and `docs/` must resolve.
  Anchor slugs follow GitHub's rule as implemented in `docsLinkSlug`: strip leading `#` run and one space, delete backticks, lowercase, delete every rune that is not a letter/digit/`_`/`-`/space, replace spaces with `-`.
  Note the em-dash consequence: ` — ` leaves two spaces behind and becomes a double hyphen.
  A new `## Told-Geometry Invariant` heading therefore anchors as `#told-geometry-invariant`.
- `internal/lyxcwd/enforcement_test.go` (`TestEnforcement_FabricVocabulary`) — the `weft`/`warp` vocabulary walk covers `.md` files.
  The new invariant text should say Fabric/warp/weft only where the two sides genuinely must be told apart (tier 2's description), per the Fabric Vocabulary Invariant.
- `cmd/lyx/tierpurity_test.go` — the new guard spawns `go env GOMOD` and must be allowlisted, or it fails the Test Tier Purity Invariant.
- The new guard file will itself contain the literal tokens `configengine.Load(` and `configengine.LoadOrTemplate(` as scan data, which is harmless (it is a `_test.go` file and the guard skips those) but should be noted in its doc comment, matching how the other guards document the same self-reference.

## Constraints

From `CONSTRAINTS.md` and `CLAUDE.md`, binding on this task:

- **Documentation Lifecycle** — module-design docs under `manifest/designs/` are deleted when their module lands;
  durable rationale moves to the Go package header comment beside the code.
  This is what licenses deleting `producers-standalone.md` and what dictates that its content lands in `doc.go` files and `CONSTRAINTS.md`, not in a replacement design doc.
- **Markdown Link Integrity** — enforced by `internal/lyxcwd/docslink_test.go`;
  see Technical context above.
- **Cwd Resolution Invariant** — reworded by this task, substance unchanged.
- **Config Strictness Invariant** — its named guard is built by this task and its **Enforced by** line flipped.
- **Fabric Vocabulary Invariant** — binds the new `.md` prose.
- **Test Tier Purity Invariant** — binds the new guard's `allowedSpawners` entry.
- **CLAUDE.md: task completion — docs land in the same commit.**
  `CONSTRAINTS.md` gets the new cross-cutting invariant in the same commit as everything else;
  `docs/overview.md` moves because the module table and execution stack change.
- **CLAUDE.md: `manifest/roadmap.md` moves only on completing or adding a planned item.**
  Completing a planned item is exactly this case, so the roadmap move is correct here.
- **CLAUDE.md: markdown semantic line breaks** — one sentence per line, plus a break at internal independent-clause boundaries.
  No fixed-column hard wrap.
  Table cells and blockquotes stay on one line.
  This binds every `.md` line this task touches, including lines edited inside existing paragraphs.
- **CLAUDE.md: worktree isolation** — all work stays in `wts/standalone-docs-and-invariants`;
  no push to `main`.
- **`CONSTRAINTS.md`'s own register** — rules only.
  The new invariant states rules;
  the *why* stays in `internal/preflight/doc.go` and the geom packages' doc comments, referenced by pointer.

## Testing

No production Go changes, so there is no unit-test surface to grow except the one new guard.

**TDD candidate — the only one:** `cmd/lyx/configstrictness_test.go`.
Write the guard against the two pinned sets before reconciling them with the tree, so the first run either confirms `CONSTRAINTS.md`'s recorded sets or reveals drift.
Both outcomes are useful;
a guard written to match whatever the tree currently says would assert nothing.

**Scenarios the guard must cover:**

- A package directory calling `configengine.Load(` that is not in the strict pinned set → fail, naming the package.
- A package directory calling `configengine.LoadOrTemplate(` that is not in the degrading pinned set → fail, naming the package.
- A pinned-set member with no matching call anywhere → fail, so a removed call site cannot silently shrink a set.
- `internal/configengine` itself excluded as the declaration site.
- `_test.go` files skipped, including the guard's own file.
- Clean skip when the go toolchain is absent from `PATH`, mirroring `cmd/lyx/crosscompile_test.go` and `cmd/lyx/tierpurity_test.go`.

**Documentation verification** — these are the real gates for the rest of the task, and every one is an existing test:

- `go test ./...` — the task-wide verify command.
- `internal/lyxcwd/docslink_test.go` — link and anchor integrity across every reworded doc, and the gate that proves the five roadmap references were fixed before the design doc was deleted.
- `internal/lyxcwd/enforcement_test.go` (`TestEnforcement_FabricVocabulary`) — the vocabulary walk over the new `.md` prose.
- `cmd/lyx/tierpurity_test.go` — proves the new guard's `allowedSpawners` entry is correct.

**Manual review obligations**, not machine-checkable:

- Every claim in the new invariant's enforcement paragraph is true of the tree at commit time — specifically, that the six named tests really do exclude `internal/lyxcwd` and that the ten review-obligation packages really have no such guard.
- No `doc.go` sentence added by the audit contradicts prose already in that file.

## Q&A log

- **Q:** What should the new invariant be named, and where in `CONSTRAINTS.md` does it go? **A:** [auto-pick] `## Told-Geometry Invariant`, immediately after the Cwd Resolution Invariant. **Why:** it is that invariant's generalization — Cwd Resolution says who may resolve, Told-Geometry says who must be told instead — and the file already uses adjacency plus **See also** cross-links to pair related rules.
- **Q:** How should the enforcement basis be stated, given the brief's "import-allowlist test per producer package where one exists"? **A:** [auto-pick] Enumerate both sets exactly — six machine-enforced packages named with their test functions, ten review-obligation packages named individually. **Why:** exploration found `shuttleengine`'s and `scoutengine`'s seam tests do not police `internal/lyxcwd` at all (provider seam and a banned list respectively), so the unenumerated phrasing would have carried a false implication into an authoritative file.
- **Q:** How far should the Cwd Resolution Invariant reword go? **A:** [auto-pick] Add one bullet stating the four facts about what `Resolve` validates, plus the consequence sentence; change no rule. **Why:** the design doc explicitly scopes out changing the invariant's substance, and the misreading is about what `Resolve` does *not* prove — which only an explicit negative fixes.
- **Q:** Does `docs/overview.md` change, and how much? **A:** [auto-pick] Three targeted edits — accuracy sentence plus pointer in its Cwd section, three missing packages added to the shared-infrastructure list, a standalone-mode paragraph in the Execution stack. **Why:** the stack genuinely has two entry modes now and the doc describes one; `internal/preflight`, `internal/hubgeom`, and `internal/standalonegeom` exist and are absent from the module map.
- **Q:** How much `doc.go` work? **A:** [auto-pick] Additive audit — add a tier-naming sentence where absent, leave existing told-geometry prose untouched, create no new `doc.go` for the three packages that lack one. **Why:** nine of the converted packages already carry correct prose;
  rewriting for uniformity would churn the diff for no reader gain.
- **Q:** The design doc must be deleted — what about the five `manifest/roadmap.md` links to it? **A:** [auto-pick] Move the Planned item to the head of Done and reword all four existing Done entries in the same commit, repointing at the new invariant. **Why:** mandatory rather than a preference — `internal/lyxcwd/docslink_test.go` fails the build on a dangling link, and the "the doc survives this task because …" clauses become false statements.
- **Q:** `internal/buildinfo/doc.go` mentions the design doc in prose. **A:** [auto-pick] Reword to name the rule rather than the file. **Why:** `docslink_test.go` does not catch a prose mention, so it would survive as a silent dangling reference;
  the sentence's actual content (why the accessor is `IsDev()` and not `StencilMode()`) is worth keeping.
- **Q:** `CONSTRAINTS.md` names T10 — this task — as the home for the Config Strictness set-equality grep guard, but the wiki brief does not mention it. Build it here or defer? **A:** [auto-pick] Build it here, and flip that invariant's **Enforced by** line. **Why:** `CONSTRAINTS.md` is the authoritative file, it assigns the work here, and it already records the guard's full specification "so T10 inherits a specification rather than re-deriving one";
  deferring would require editing the same invariant anyway, for strictly less value. Flagged plainly as a scope addition beyond the wiki brief.
- **Q:** Does scout's remaining deviation need recording? **A:** [auto-pick] No deviation to record;
  note instead that scout's guard is a banned list not covering `internal/lyxcwd`, placing it in the review-obligation set. **Why:** the brief conditions that record on T9 being skipped, and T9 landed in commit `8aced4cb`;
  scout's two remaining `lyxcwd` mentions are comments, and a machine-global toolchain-cache path is not anchor geometry.
