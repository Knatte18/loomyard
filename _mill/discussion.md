# Discussion: Deploy cited spec/design docs to target repos like stencils

```yaml
task: Deploy cited spec/design docs to target repos like stencils
slug: deploy-specs-like-stencils
status: discussing
parent: main
```

## Problem

Agent-facing stencil prompts under `contracts/stencils/**` cite loomyard's own repo-relative doc paths — `contracts/specs/loom-plan-spec.md`, `manifest/designs/plan-card-format.md`, `docs/code-comment-conventions.md`, `internal/shedadapters/bouncerfiles.go` — as bare strings an agent is expected to open.
`lyx` genuinely drives arbitrary TARGET repos, not loomyard's own source tree: the crucible campaign's sandbox drove `Knatte18/lyx-test`, which `docs/sandbox-hub.md` states lives outside loomyard's tree "so it is never mistaken for part of Loomyard itself."
An agent working in such a repo's worktree has none of those paths on disk, so every one of those citations is a dead reference.

Some of the cited docs are NORMATIVE — the agent's own output is mechanically validated against them.
`contracts/specs/loom-plan-spec.md` is the format contract `Plan-Validate`/`Plan-Revalidate` parse a written plan against, and the citing stencil's own text admits the in-stencil version only "summarizes" it.
`manifest/designs/plan-card-format.md` is the sole home of the Verify-model tier definitions, and `contracts/stencils/loom/loom-template-plan.md:143` states outright "this file does not restate them."
An agent that cannot reach those can produce a plan that fails validation for reasons it was never told.
Other citations are background or mis-scoped and need removing or rewording, not shipping.

**Why now:** surfaced by crucible's self-report Tier 2 filing [loomyard#241](https://github.com/Knatte18/loomyard/issues/241).
A one-line fix naming the path explicitly was tried and reverted (`d9d5cc0ca` reverts `2e7d612cc`) once it became clear that naming a path which does not exist in the target repo is not a fix.

## Audit result (the factual basis for every decision below)

Every stencil's leading `<!-- ... -->` banner is stripped by `stencil.StripLeadingComment` before an agent ever sees the text, so banner-internal `internal/...` references are not agent-facing citations at all and are out of scope.
Scanning only post-strip **body** text across all 29 files under `contracts/stencils/**/*.md`, the complete citation set is:

**Normative (must become reachable):**

| Site | Cited path | What the agent needs it for |
|---|---|---|
| `contracts/stencils/loom/loom-template-plan.md:138` | `contracts/specs/loom-plan-spec.md` | Card fields grammar + complete validation-check set |
| `contracts/stencils/loom/loom-template-plan.md:143` | `manifest/designs/plan-card-format.md` | Verify-model tier definitions — explicitly not restated in the stencil |
| `contracts/stencils/loom/loom-rubric-plan-review.md:15` | both | Format contract + Card model |
| `contracts/stencils/loom/loom-rubric-plan-review.md:31` | `contracts/specs/loom-plan-spec.md` | the twenty-eight check IDs `format-unrecognized`..`commit-subject-mismatch` |
| `contracts/stencils/loom/loom-rubric-plan-review.md:38` | `manifest/designs/plan-card-format.md` | per-type table (which types require `ImpactSummary`) |
| `contracts/stencils/loom/loom-rubric-webster-review.md:16` | both | Card model + format contract |
| `contracts/stencils/loom/loom-rubric-webster-review.md:54` | `manifest/designs/plan-card-format.md` | per-type mechanical-check table the reviewer must confirm ran |
| `contracts/stencils/webster/webster-body-implementer.md:27` | `contracts/specs/loom-plan-spec.md` | full target/`Uses:` grammar |

Exactly **two** doc files carry the entire normative class.
No other file under `contracts/specs/` or `manifest/designs/` is cited from any stencil body.

**Non-normative (citation is the thing to fix):**

| Site | Cited path | Disposition |
|---|---|---|
| `contracts/stencils/loom/loom-rubric-webster-review.md:51` | `docs/code-comment-conventions.md` | mis-scoped — reword to the target repo's own conventions |
| `contracts/stencils/bouncer/bouncer-template-judge.md:61,90,116` | `internal/shedadapters/bouncerfiles.go` | background — drop the path |
| `contracts/stencils/bouncer/bouncer-template-seed.md:50` | `internal/shedadapters/bouncerfiles.go` | background — drop the path |
| `contracts/stencils/webster/webster-template-master.md:25` | `CONSTRAINTS.md` (unguarded "in full") | target-repo path, but needs "if present" |
| `contracts/stencils/webster/webster-prefix-recovery.md:20` | `CONSTRAINTS.md` (unguarded "in full") | target-repo path, but needs "if present" |

`contracts/stencils/loom/loom-template-discussion.md:42` and `contracts/stencils/loom/loom-template-plan.md:34` already say "at the repo root **if present**" and are correct as-is — they are the wording model for the two unguarded sites.

## Scope

**In:**

- A new embed site for the two normative docs, and a second `stencilstore.Registry` implementation over them.
- A `SpecsDir` derivation in both `internal/fabricengine` (hub) and `internal/standalonegeom` (standalone), mirroring the existing `StencilsDir` pair.
- Seeding/refreshing the specs directory in the same once-per-process pre-run pass that already reconciles stencils, in both hub and standalone wiring.
- A `{{.specs_dir}}` stencil marker, plumbed from each affected producer's own render call site, and rewriting the eight normative citations to use it.
- Rewording/removing the five non-normative citations per the audit table.
- A new enforcement test in `package stencils` that fails on any newly-introduced bare cross-repo citation in a stencil body.
- `CONSTRAINTS.md` update to the Stencil Ownership Invariant (the `//go:embed` clause), plus the module docs the change touches.

**Out:**

- Any new `lyx spec` CLI subtree, or new verbs on `lyx stencil`.
- Shipping `docs/code-comment-conventions.md`, or any doc beyond the two normative ones.
- Crucible round 1's D-2 finding (two-lyx-build stencil skew) — related subsystem, distinct problem, see Decisions.
- Changing `stencilstore`'s reconcile policy, `Classify` table, `Mode` semantics, or edit-detection behaviour in any way.
- Changing the content of `loom-plan-spec.md` or `plan-card-format.md` themselves.
- Resolving the dangling cross-repo links *inside* the two travelling docs (see Technical context — deliberately accepted, not fixed).
- Renaming the source files `contracts/specs/loom-plan-spec.md` or `manifest/designs/plan-card-format.md`.
- `manifest/roadmap.md` — this closes a filed bug, not a planned roadmap item.

## Decisions

### deploy-not-bake

- **Decision:** Extend the same embed-and-deploy mechanism `contracts/stencils/**` already has, rather than baking spec text into the stencil via `{{.X}}` template substitution.
- **Rationale:** `contracts/specs/loom-plan-spec.md`'s own status line states that the LLM-facing subset is pinned separately in the producer's stencil "so the agent's prompt never duplicates this file and the two cannot drift from being the same doc."
  Baking in inverts a documented design decision and would add 482 lines to every Plan-Write prompt and both rubric prompts.
  Deploy is also nearly free: `stencilstore.Reconcile(baseDir, registry, mode, sourceDir)` is already fully parameterized over both `baseDir` and `Registry`, so a second registry against a second baseDir needs **no change to `reconcile.go` at all**.
- **Rejected:** bake-in via `{{.plan_spec}}` (inverts the spec's own anti-duplication intent, bloats three prompts); hybrid bake-the-small-deploy-the-large (two mechanisms for one class, worst of both).

### specs-dir-mirrors-stencils-dir

- **Decision:** Deployed copies live at `<hub>/_board/_lyx/specs/` in hub mode and at the `standalonegeom` equivalent of `<stateDir>/.../specs` in standalone mode — an exact structural mirror of `StencilsDir`, declared by a new `SpecsDir` function in each of `internal/fabricengine/junctionnames.go` and `internal/standalonegeom`.
- **Rationale:** Same durable/tracked `_lyx` tree as stencils, same told-never-derives discipline, same weft-commit story.
  `"specs"` is not a policed geometry token (same reasoning `stencilsDirName`'s own comment gives for `"stencils"`), so it needs no `geometryTokenOwners` row.
- **Rejected:** a `specs/` subfolder nested inside the stencils dir (collides with `RelPath`'s `<family>/` derivation, and conflates a told stencils override with specs); `<hub>/_board/_lyx/docs/` ("docs" understates that these are pinned normative contracts).

### reuse-reconcile-policy-unchanged

- **Decision:** Deployed specs use `stencilstore`'s existing policy verbatim — absent→seed stamped, untouched-but-stale→refresh (prod) or warn (dev), reconciled→restamp, edited→warn and never overwrite.
  No new `Mode`, no force-sync-normative carve-out.
- **Rationale:** CONSTRAINTS.md's Stencil Ownership Invariant states "A hash-mismatched file is never overwritten," and a second policy for a second doc class would be a second thing to keep correct.
  An operator who edits a deployed spec has deliberately forked it; the existing warn-and-leave-alone response is right.
  Reusing the policy means reusing the code with zero modification.
- **Rejected:** always force-sync normative docs (needs a new `Mode`, contradicts the invariant, and buys nothing — nobody is editing these in practice).

### two-docs-only

- **Decision:** Exactly `contracts/specs/loom-plan-spec.md` and `manifest/designs/plan-card-format.md` travel.
- **Rationale:** The full-tree audit above found no third normative citation.
  Blanket-shipping `contracts/specs/` would carry `llm-model-spec.md`, `loom-status-spec.md`, `webster-spec.md`, and `final-summary-spec.md` — none cited from any stencil body.
- **Rejected:** all of `contracts/specs/` (ships four uncited docs); adding `docs/code-comment-conventions.md` (see `code-comment-conventions-is-misscoped`).

### specs-dir-marker

- **Decision:** Citations are rewritten to `` `{{.specs_dir}}/loom/loom-plan-spec.md` ``, with `specs_dir` filled at render time from the resolved specs directory, told to each producer the same way its other geometry is told.
  `specs_dir` is a **required** marker on every stencil that carries it (plain `stencil.Fill` semantics, or a required name under `FillOptional`) — never optional, since a blank render would silently reproduce exactly the dead-reference bug this task exists to fix.
- **Rationale:** A hub-relative literal breaks in standalone mode for exactly the reason `webster-template-master.md`'s own banner records for `plan_dir`: "the standalone Master proved unable to see a plan it was told about only in hub-relative terms."
  Critically, `--stencils-dir` is a told override (`cliwire.Module.RefuseUnreadableStencilsDir`, `cliwire/standalone.go`) with no sibling guarantee, so the specs dir must be resolved and told independently — it can never be derived as a sibling of whatever stencils dir was told.
- **Rejected:** a hard-coded `_lyx/specs/...` hub-relative string (breaks standalone, breaks under `--stencils-dir`).

### code-comment-conventions-is-misscoped

- **Decision:** `loom-rubric-webster-review.md:51` is reworded to have the reviewer check doc comments against the **target repo's own** conventions (its `CONSTRAINTS.md` and surrounding code), dropping the `docs/code-comment-conventions.md` path entirely.
  The doc is not deployed.
- **Rationale:** That doc is explicitly Go-only and is loomyard's own house style.
  Shipping it into an arbitrary target repo would have a reviewer enforce the *wrong* conventions there — a worse defect than the dangling path.
  This is precisely the task body's "if a stencil cites one of these purely for context, the citation itself is probably the thing to fix" case.
- **Rejected:** deploying it as a third normative doc (imposes loomyard's Go style on non-Go target repos).
- **Gotcha:** `docs/code-comment-conventions.md:5` names `contracts/stencils/rubric_test.go` as the guard for this very citation. Check whether `rubric_test.go` pins the phrase, and update both the test and that doc line if so.

### background-citations-lose-the-path

- **Decision:** The four `internal/shedadapters/bouncerfiles.go` citations in `bouncer-template-judge.md` and `bouncer-template-seed.md` drop the path, keeping the substance ("this format is parsed mechanically — deviating from it fails the parse").
  `webster-template-master.md:25` and `webster-prefix-recovery.md:20` gain "if present" on their `CONSTRAINTS.md` instruction, matching the wording already used in `loom-template-discussion.md:42` and `loom-template-plan.md:34`.
- **Rationale:** Naming a loomyard source file tells an agent in a target repo nothing it can act on; the enforcement fact is the part that changes its behaviour.
  An unguarded "read `CONSTRAINTS.md` in full" is a dead instruction in a repo without one.
- **Rejected:** leaving them (they are the same class of bug, and the enforcement test in `citation-enforcement-test` would flag them anyway).

### citation-enforcement-test

- **Decision:** A new test in `package stencils` (beside `rubric_test.go`) scans every registered stencil's post-`StripLeadingComment` body for repo-relative path tokens under `contracts/`, `manifest/`, `docs/`, and `internal/`, failing unless the occurrence is `{{.specs_dir}}`-prefixed or listed on a named allowlist keyed by `(stencil name, token)` with each entry naming its justification.
- **Rationale:** This is the actual deliverable.
  Fixing nine sites fixes nine instances; the test closes the class, matching the body's "the fix should close the whole normative class at once, not file-by-file."
  `rubric_test.go` is the in-repo precedent for content-pinning tests in this package, and CONSTRAINTS.md's Markdown Link Integrity invariant is the precedent for the `(file, target)`-keyed allowlist-with-owner shape.
- **Rejected:** fix the nine sites and rely on review discipline (this bug reached production once already, and the reverted one-line fix shows review discipline did not catch it).

### no-new-cli-surface

- **Decision:** No `lyx spec` subtree and no new `lyx stencil` verbs.
  Specs reconcile in the same once-per-process pre-run pass that already reconciles stencils.
- **Rationale:** YAGNI for two files.
  The Stencil Ownership Invariant already requires "Seed/refresh runs once per process pre-run, never lazily inside `Read`," so the pass is the natural and only home.
- **Rejected:** mirroring `list`/`diff`/`sync`/`promote` as `lyx spec ...` (four verbs, four help-tree test rows, for two files nobody edits).

### d2-stays-separate

- **Decision:** Crucible round 1's D-2 finding (a stale `lyx` build on PATH downgrading shared stencils) is not folded in.
  The relationship is noted in whichever module doc this change touches, and nothing more.
- **Rationale:** Distinct problems — skew between two lyx builds versus docs absent entirely.
  Because deployed specs reuse `ModeFor`/`ModeDev` unchanged, they inherit exactly the asymmetry D-2 describes; solving it here would mean solving it for stencils too, which is a strictly larger, separate task.
- **Rejected:** folding D-2 in (scope explosion; changes `stencilstore` policy this task deliberately leaves alone).

### registered-names

- **Decision:** Registered names are `loom-plan-spec` and `loom-plan-card-format`, so `stencilstore.RelPath` places both under one `loom/` family: `specs/loom/loom-plan-spec.md` and `specs/loom/loom-plan-card-format.md`.
  Only the *registered name* changes for the design doc; the source file `manifest/designs/plan-card-format.md` is not renamed.
- **Rationale:** `RelPath` derives the family directory from the substring up to the first `-`, so a bare `plan-card-format` would create a one-file `plan/` family dir.
  Both docs belong to loom.
- **Rejected:** `plan-card-format` → `plan/plan-card-format.md` (a lone one-file family directory).

## Technical context

**The embed constraint is the one real structural obstacle.**
`contracts/stencils/stencils.go`'s own file comment states the rule: "`//go:embed` reaches only files at or below its own directory."
`contracts/specs/` and `manifest/designs/` are both outside `contracts/stencils/`, so the existing embed site cannot reach either.
A new embed site is required.
`contracts/specs/loom-plan-spec.md` is at-or-below a new `contracts/specs/specs.go`, but `manifest/designs/plan-card-format.md` is not, and the two have no common ancestor below the repository root.
`//go:embed` patterns additionally may not contain `..`, so a single site "covering both" would have to be a package at the repository root — a placement this repo does not otherwise use.
**The expected resolution is therefore two embed sites, not one**: one under `contracts/specs/`, one under or beside `manifest/designs/`, both feeding the *same* logical registry.
`stencilstore.Registry` is a two-method interface (`Names()`, `Default(name)`) and is indifferent to how many `//go:embed` directives or packages back it, so this is mechanical and low-risk.
The final wiring is a plan-level decision; the embed constraint itself is hard and non-negotiable, and the root-package option should not be re-litigated.

**`stencilstore` needs no modification.**
`Reconcile(baseDir string, registry Registry, mode Mode, sourceDir string)` takes both the directory and the registry as arguments, and `Registry` is a two-method interface (`Names()`, `Default(name)`).
`Path`/`RelPath`/`Classify`/`ApplyStamp`/`BodyHash`/`writeStamped`/`seedGitattributes` are all name- and baseDir-generic.
A second registry over a second baseDir is a pure call-site addition.
Note `Reconcile` also seeds `.gitattributes` (`*.md text eol=lf`) into its baseDir, which is correct and wanted for the specs dir too.

**Stamping mutates the deployed copy.**
`ApplyStamp` prepends or edits a leading `<!-- lyx-stencil: sha256=... -->` banner.
Both travelling docs currently open with a `# Heading` and then a `> **Status: ...**` blockquote, so neither has a leading HTML comment — `ApplyStamp` will prepend a one-line banner plus a blank line.
`BodyHash` strips that banner before hashing, so the round-trip is sound, but the deployed copy is not byte-identical to the source file.
The `lyx-stencil:` key name is now slightly inaccurate for a spec; leave it — changing `stampKey` would invalidate every existing stencil stamp in every hub.

**Existing call sites to extend for seeding.**
Standalone: `internal/cliwire/standalone.go` calls `stencilstore.Reconcile(stencilsDir, stencils.Registry(), stencilstore.ModeFor(buildinfo.IsDev()), "")` around line 95.
Hub: the root pre-run in `cmd/lyx` (named as a consumer in `contracts/stencils/stencils.go`'s `Registry()` doc comment, alongside `internal/stencilcli`).
`internal/stencilcli/cli.go` has `sync` (`ForceRefresh`) and `validate`.
Both the hub and standalone paths need the second reconcile added; `stencil sync`/`validate` covering specs is a judgment call for the plan (the `no-new-cli-surface` decision means no new *verbs*, not necessarily that existing verbs ignore specs).

**Geometry to mirror.**
`internal/fabricengine/junctionnames.go:126` `StencilsDir(hub) = filepath.Join(BoardDir(hub), lyxdirs.LyxDirName, stencilsDirName)` with `stencilsDirName = "stencils"` unexported and documented as not a policed geometry token.
`internal/standalonegeom/stencilsdir.go:25` is the standalone half.
`SpecsDir` mirrors both exactly.

**Render call sites for `{{.specs_dir}}`.**
The four stencils needing the marker are rendered from:
`loom-template-plan.md` → `composePlanPrompt` in `internal/loomengine/plan.go`;
`loom-rubric-plan-review.md` and `loom-rubric-webster-review.md` → read as rubric **values** interpolated into the two Bouncer stencils' `{{.rubric}}` marker (see `rubric_test.go`'s "marker-value-not-template constraint" — **a rubric is a value, not a template, so a `{{.specs_dir}}` marker placed in a rubric body will NOT be rendered by `stencil.Fill`; it must be substituted before the rubric is handed over as a value, or the citation must be resolved another way**);
`webster-body-implementer.md` → composed with `WebsterPrefixFork`/`WebsterPrefixRecovery` by `RenderForkPrompt`/`RenderRecoveryPrompt` in `internal/websterengine/render.go`.
The rubric case is the sharpest trap in this task and must be designed explicitly.

**Told-Geometry Invariant applies.**
An engine is handed absolute paths and derives none.
`specs_dir` must be threaded through the same `Geometry` structs and `hubgeom`/`standalonegeom` constructors that already carry `stencilsDir`, never derived inside an engine.

**The travelling docs carry their own dangling cross-repo links.**
`plan-card-format.md` cites `contracts/specs/loom-plan-spec.md`, `docs/code-comment-conventions.md`, `internal/planparser`, `internal/hubforge`, and a relative link `[code-comment-conventions.md](../../docs/code-comment-conventions.md)`.
`loom-plan-spec.md` cites `internal/planparser`, `internal/websterengine`, `docs/glyph.md`, `contracts/stencils/loom/loom-template-plan.md`, and a relative link to `manifest/designs/webster-parallel-execution.md`.
These are **accepted as-is and deliberately not fixed** — they are reference-doc prose read by a human or a curious agent, not instructions the agent's output is validated against, and rewriting them would fork the docs from their loomyard-side source, which `deploy-not-bake` exists to avoid.
Scope this explicitly so a plan reviewer does not read it as an oversight.
Note the two relative links (`../../manifest/...`, `../../docs/...`) resolve correctly from `manifest/designs/` and `contracts/specs/` but not from `specs/loom/` in a deployed tree.

**Content overlap between the two docs is real but not a reason to ship only one.**
Both carry a "Card types" table, and they deliberately disagree on one cell: `loom-plan-spec.md:124` states `ImpactSummary` is required for `Edit`/`Delete` only and explicitly says it "resolves the design doc's table in favour of the design doc's own prose."
`plan-card-format.md`'s table says `Create` requires it.
The spec is authoritative where they differ, and `plan-card-format.md`'s Verify-model section is the sole home of the tier definitions.
Both must travel.

**Validation findings already carry human detail.**
`internal/planparser/validate.go:49` has a `Detail string` field, and `validate_test.go:70` records "Detail is for humans, Check is the stable contract."
So a plan that fails validation does get told what went wrong — which narrows, but does not close, the gap: `loom-template-plan.md:143`'s "this file does not restate them" for the Verify-model tiers is a genuine hole a validation message cannot fill, since the agent needs it at *write* time.

## Constraints

From `CONSTRAINTS.md`, in order of how directly each binds this task:

- **Stencil Ownership Invariant** — "`//go:embed` in `contracts/stencils` is seed defaults only. `internal/stencilstore` is sole owner of seeding/hashing/reading/validation. A hash-mismatched file is never overwritten. Seed/refresh runs once per process pre-run, never lazily inside `Read`."
  This task adds a second embed site and a second seeded directory, so **this invariant's text must be updated in the same commit**.
- **Told-Geometry Invariant** — an engine derives no paths of its own and never imports `internal/lyxcwd`; `internal/hubgeom` and `internal/standalonegeom` are the only `Geometry`-struct constructors.
  `specs_dir` is told, never derived.
- **Durable-vs-Ephemeral State Invariant** — `_lyx` holds tracked content only; every never-tracked file lives under `.lyx` at the mirrored subpath.
  Deployed specs are durable tracked weft content under `_lyx`, exactly like stencils.
- **Cliwire Sole-Wiring Invariant** — a `<module>cli` never re-implements mode-derived state/plan/stencils resolution; it declares a `cliwire.Module` descriptor and calls in.
  Specs-dir resolution belongs in `internal/cliwire`, not in any `<module>cli`.
- **Markdown Link Integrity** — every inline markdown link in a `.md` under `manifest/` or `docs/` resolves, with an allowlist keyed by `(file, target)` naming its owning task.
  Relevant if the change touches links in `manifest/designs/plan-card-format.md`.
- **CLI / Cobra Invariant** — non-empty `Short` on every command, help-tree tests.
  Binding only if the plan adds a command despite `no-new-cli-surface`.
- **Documentation Lifecycle** — see `docs/overview.md#documentation-lifecycle`.
- **Test Tier Purity Invariant** and **Hermetic Git Test Environment Invariant** — govern where the new tests may live and what they may do.

From `CLAUDE.md`:

- Docs land in the same commit as the change: the module doc under `manifest/designs/`, `docs/overview.md` if the module table or execution stack changes, and `CONSTRAINTS.md` for the invariant edit above.
- `manifest/roadmap.md` does **not** move — this is a filed bug, not a planned roadmap item.
- Markdown uses semantic line breaks, one sentence per line, never fixed-column hard-wrap.
- Building requires `CGO_ENABLED=1` and a C compiler on `PATH` (quarry's tree-sitter grammars).

## Testing

**`package stencils` (`contracts/stencils/`) — the primary TDD candidate.**
Write `citation_enforcement_test.go` first, before touching any stencil body.
It scans every name in the registry, reads its default bytes, applies `stencil.StripLeadingComment`, and fails on any token matching a repo-relative path under `contracts/`, `manifest/`, `docs/`, or `internal/` unless the occurrence is `{{.specs_dir}}`-prefixed or on the `(stencil name, token)` allowlist.
Written first it must fail on all nine current sites, which is the proof it works; it goes green as the rewrites land.
Scenarios: a normative citation correctly rewritten passes; a bare re-added citation fails; an allowlisted entry passes; an allowlist entry whose token no longer appears anywhere fails as stale.

**`contracts/stencils/rubric_test.go` — extend, and check for a break.**
`docs/code-comment-conventions.md:5` claims `rubric_test.go` guards the `code-comment-conventions` citation.
Verify whether it pins that phrase; if so, the `code-comment-conventions-is-misscoped` rewrite breaks it and both the test and that doc line move in the same commit.
Add a pin that each of the four `{{.specs_dir}}`-carrying stencils actually contains the marker, so a future edit cannot silently drop it back to a bare path.

**`internal/stencilstore` — new registry, existing machinery.**
No behavioural change, so no new tests on `Reconcile`/`Classify` themselves.
Add a table row to the existing `RelPath` test asserting `loom-plan-spec` → `loom/loom-plan-spec.md` and `loom-plan-card-format` → `loom/loom-plan-card-format.md`.
Add a specs-registry test mirroring `stencilstore_test.go`'s registry coverage: `Names()` is stable and non-empty, `Default()` returns non-empty bytes for each name and `false` for an unknown one, and every default round-trips `BodyHash(ApplyStamp(c, h)) == BodyHash(c)`.

**Geometry — mirror the existing pairs exactly.**
`internal/fabricengine/specsdir_test.go` modelled on `stencilsdir_test.go`: `SpecsDir("/h")` equals the expected literal, and it is a child of `BoardDir`.
Add a case to `internal/standalonegeom/standalonegeom_test.go` beside `TestStencilsDir`.
Check whether `TestEnforcement_GeometryLiterals`'s `geometryTokenOwners` needs a row — per `stencilsDirName`'s own comment, `"specs"` should not, but assert the decision rather than assuming it.

**Seeding — integration.**
A hermetic test (hub built via `internal/hubforge` per the hubforge Fabric-Fixture Invariant, or `t.TempDir()` where the seam allows) that a fresh hub's pre-run creates `<hub>/_board/_lyx/specs/loom/loom-plan-spec.md` with a parseable stamp and a `.gitattributes`, and that a second run writes nothing.
Same for standalone via `cliwire`.
Key scenario: a **told `--stencils-dir`** still yields a resolvable specs dir — this is the case that motivates `specs-dir-marker` and the one most likely to regress.

**Render — the rubric trap.**
Assert each of the four affected prompts renders with `specs_dir` substituted and no literal `{{.specs_dir}}` surviving in the final prompt text.
For the two rubrics specifically, assert this at the point the composed **Bouncer** prompt is produced, not at rubric-read time — a rubric is interpolated as a value into `{{.rubric}}` and is never itself run through `stencil.Fill`, so a test that only checks the rubric file would pass while the shipped prompt still carried an unrendered marker.
Assert `specs_dir` is treated as required: rendering with it absent or empty errors rather than producing a blank path.

**Whole-repo gate.**
`go build ./... && go test ./...` with `CGO_ENABLED=1`.

## Q&A log

- **Q:** Deploy the normative docs like stencils, or bake their content into the stencil via `{{.X}}` substitution? **A:** [auto-pick] Deploy — new embed site plus a second `stencilstore.Registry`. **Why:** `loom-plan-spec.md`'s own status line states the agent's prompt must never duplicate the file precisely so the two cannot drift; baking in inverts that and adds 482 lines to three prompts, while `Reconcile` is already baseDir- and Registry-parameterized so deploy needs no change to `reconcile.go`.
- **Q:** Where do deployed copies live? **A:** [auto-pick] `<hub>/_board/_lyx/specs/` plus the `standalonegeom` equivalent, via a new `SpecsDir` mirroring `StencilsDir`. **Why:** same durable tracked tree and same told-never-derives discipline as stencils; a `specs/` subfolder inside the stencils dir would collide with `RelPath`'s family derivation and conflate a told stencils override with specs.
- **Q:** Always force-sync a normative spec, or reuse stencilstore's don't-overwrite-an-edit policy? **A:** [auto-pick] Reuse unchanged. **Why:** a second policy needs a new `Mode` and contradicts the Stencil Ownership Invariant's "a hash-mismatched file is never overwritten"; an operator editing a deployed spec has deliberately forked it, and reusing the policy means reusing the code with zero modification.
- **Q:** Which docs travel? **A:** [auto-pick] `loom-plan-spec.md` and `plan-card-format.md` only. **Why:** the full-tree audit of all 29 stencils found no third normative citation; blanket-shipping `contracts/specs/` would carry four docs no stencil body cites.
- **Q:** How does a stencil name the deployed path so it resolves in hub, standalone, and `--stencils-dir` modes? **A:** [auto-pick] A required `{{.specs_dir}}` marker filled at render time. **Why:** a hub-relative literal already failed once in standalone (`webster-template-master.md`'s `plan_dir` banner records it), and `--stencils-dir` is a told override with no sibling guarantee, so the specs dir must be told independently rather than derived from the stencils dir.
- **Q:** Ship `docs/code-comment-conventions.md` as a third doc, or reword the citation? **A:** [auto-pick] Reword — the reviewer checks the target repo's own conventions. **Why:** the doc is explicitly Go-only loomyard house style; shipping it would have a reviewer enforce the wrong conventions in an arbitrary target repo, which is worse than the dangling path.
- **Q:** What about the `bouncerfiles.go` and unguarded `CONSTRAINTS.md` citations? **A:** [auto-pick] Drop the `bouncerfiles.go` path keeping the enforcement substance; add "if present" to both `CONSTRAINTS.md` sites. **Why:** a loomyard source path tells a target-repo agent nothing actionable, and an unguarded "read `CONSTRAINTS.md` in full" is dead in a repo without one; `loom-template-discussion.md:42` already has the correct wording to copy.
- **Q:** Registered names, given `RelPath` derives the family dir from the substring before the first `-`? **A:** [auto-pick] `loom-plan-spec` and `loom-plan-card-format`, both landing under `specs/loom/`. **Why:** a bare `plan-card-format` would create a one-file `plan/` family directory; only the registered name changes, the source file is not renamed.
- **Q:** What stops the next citation from reintroducing this bug? **A:** [auto-pick] A `package stencils` test scanning every stencil body for bare cross-repo path tokens, with an allowlist keyed by `(stencil name, token)`. **Why:** this is the real deliverable — it closes the class rather than nine instances; review discipline already missed this once, and the reverted one-line fix is the evidence.
- **Q:** New `lyx spec` CLI subtree? **A:** [auto-pick] No — specs reconcile in the same once-per-process pre-run pass, no new verbs. **Why:** YAGNI for two files nobody edits, and the Stencil Ownership Invariant already puts seed/refresh in that pass.
- **Q:** Fold in crucible D-2 (two-lyx-build stencil skew)? **A:** [auto-pick] No — keep separate, note the relationship in the module doc. **Why:** distinct problems (build-vs-build skew versus docs absent entirely); deployed specs inherit the `ModeFor` asymmetry that *is* D-2, so solving it here would mean solving it for stencils too — a strictly larger task that changes the reconcile policy this task deliberately leaves alone.
- **Q:** Fix the dangling cross-repo links inside the two travelling docs? **A:** [auto-pick] No — accept them, and say so explicitly in Scope. **Why:** they are reference prose, not instructions the agent's output is validated against, and rewriting them would fork the deployed copies from their loomyard-side source, which is exactly what `deploy-not-bake` exists to prevent.
