# Discussion: PATTERN directives: move from Go constants to stencil files

```yaml
task: 'PATTERN directives: move from Go constants to stencil files'
slug: pattern-directive-stencils
status: discussing
parent: main
```

## Problem

`internal/pattern.Directive(l, role)` returns one of three hardcoded Go string constants — `implementerDirective`, `reviewFixDirective`, `orchestratorDirective` (`internal/pattern/pattern.go:64-89`).
Each is static prose with no per-call interpolation: a "read `_lyx/PATTERN.md` before you touch anything" checklist, worded once per agent shape.
Four call sites splice the returned string into a producer template's optional `pattern_directive` marker.

This is the single source of truth for that prose — genuinely not duplicated anywhere — but it lives in Go source rather than in `stencils/`.
Editing it means touching Go and rebuilding, which is exactly the friction the `stencils/` reorg removed for every other producer-facing prompt in the tree.
Fifteen stencils already ship as directly-editable `.md` files read at call time through `stencilstore.Read`;
these three are the last prompt-facing content that does not.

The full design was written ahead of this task at `manifest/designs/pattern-directive-stencils.md` and is currently marked **Status: Design — not built**.
This discussion adopts that design with three corrections, all recorded under Decisions below: the read-failure posture (the design doc says fail-silent; this task fails loud), the claim that all four call sites are plumbing-free (two of them are not), and the stamp-banner strip the design doc does not mention at all (without it the relocation is not behaviour-preserving).

## Scope

**In:**

- Three new stencil files under a new `stencils/pattern/` family directory, each carrying one constant's prose **verbatim as its body**, under the conventional explanatory `<!-- … -->` banner every other stencil opens with.
- Three new `//go:embed` vars plus three new `entries` rows in `stencils/stencils.go`.
- Three new lines in the **repo-root `.gitattributes`**, one per new file, pinning each to `text eol=lf`.
  That file enumerates every embed target individually and carries no `stencils/**` glob, so a new stencil that is not listed is silently unpinned.
- `internal/pattern.Directive` gains a `stencilsDir string` parameter and an `error` return, and reads through `stencilstore.Read` — then strips the leading banner via `stencil.StripLeadingComment` — instead of returning a constant.
- The three `const` blocks in `internal/pattern/pattern.go` are deleted, along with the file-header and pre-constant comments that describe them.
- All four call sites updated — two are a one-line change, two need the call hoisted out of a map literal.
- `internal/pattern`'s leaf-invariant allowlist extended by two entries, at all three places the leaf test states it plus `CONSTRAINTS.md`.
- Test migration in `internal/pattern`, `internal/websterengine`, and `internal/loomengine`, plus four new tests: lazy-read, missing-stencil error, banner-strip, and stripped-body equality with the embedded default.
- Five doc updates: `CONSTRAINTS.md`, `internal/pattern/doc.go`, `manifest/designs/pattern-directive-stencils.md`, `manifest/roadmap.md`, and `tools/sandbox/SANDBOX-CORE-SUITE.md`.

**Out:**

- The directive prose itself.
  It moves byte-for-byte **as the stencil body** — the bytes remaining after `stencil.StripLeadingComment` — not as whole-file bytes;
  not one word changes, including the trailing newline each constant carries.
  Whole-file equality is not available to claim: every stencil file carries a leading banner, and `stencilstore.Reconcile` writes a `lyx-stencil:` stamp line into it on seeding.
  What must be identical to today's output is what `Directive` returns, which is the stripped body.
- `pattern.isActive` and its three pinned edge rules (empty file active, directory inactive, non-`IsNotExist` stat error active).
  Untouched.
- Every producer template's marker set.
  All four templates carrying `pattern_directive` (`stencils/loom/loom-template-plan.md`, `stencils/webster/webster-template-master.md`, `stencils/webster/webster-prefix-recovery.md`, `stencils/burler/burler-step-1-explore.md`) keep their existing flat, optional marker exactly as today.
  No `{{if}}` block is added to any of them — "empty when inactive" already gives the opt-in behaviour, and the zero-duplication property comes from the stencil file being the one source, not from template-level conditionals.
- `pattern.File`, `FileHere`, `PathspecFile`, `PathspecDir`, and the `patternFileName` constant.
- The `Role` type and its three members.
- `internal/burlerengine`'s tests.
  `template_test.go:142` passes a hardcoded placeholder string into a values map and never calls `Directive`, so it is unaffected.
- Any new CLI surface.
  The three stencils become visible to `lyx stencil list` and to `stencilstore.Reconcile` automatically by being registered;
  no command changes.

## Decisions

### Extend the Pattern Leaf Invariant rather than route around it

- Decision: add **two** entries to `internal/pattern`'s allowlist — `github.com/Knatte18/loomyard/internal/stencilstore` (for `Read`) and `github.com/Knatte18/loomyard/internal/stencil` (for `StripLeadingComment`, see the next decision) — in the same commit as the code change.
  The allowlist is stated in three places inside `internal/pattern/leaf_enforcement_test.go`, and all three change together: the file-header comment (lines 1-6), the `allowedImports` map (lines 22-25), and the failure message's parenthetical (line 86, currently "stdlib + lyxcwd + lyxdirs").
  `CONSTRAINTS.md`'s **Pattern Leaf Invariant** section is the fourth site.
- Rationale: the invariant's stated subject is feature packages — its text reads "never `websterengine`, `burlerengine`, `loomengine`, or any other feature package".
  Both additions are shared infrastructure, not feature packages, so admitting them does not weaken what the invariant was written to protect.
  Cycle risk was checked rather than assumed: `stencilstore` production code imports only `internal/stencil` and `internal/logger`;
  `internal/stencil` imports **no** internal package at all (a true zero-import leaf, so it cannot participate in a cycle by construction, exactly the argument the invariant already makes for `internal/lyxdirs`);
  `internal/logger` imports `internal/lyxcwd`, `internal/lyxdirs`, and `internal/proc`.
  Nothing in that closure imports `internal/pattern`, so no cycle is possible.
- Rejected: keeping `pattern` a pure leaf by exporting `IsActive(l)` plus a role→stencil-name accessor and having each of the four call sites do its own read — duplicates the active-check + read + strip + error-wrap four times for no safety gain.
  Also rejected: a new `internal/patterndirective` adapter package holding `Directive` — a whole package for one function.

### `Directive` strips the leading banner after reading

- Decision: `Directive` calls `stencil.StripLeadingComment` on the bytes `stencilstore.Read` returns, and returns the stripped body.
  Strip only — no LF normalisation — so its treatment of a stencil is identical to what every other consumer gets.
- Rationale: this is a correctness requirement, not a refinement, and the pre-written design doc does not mention it.
  `stencilstore.Reconcile` seeds every registered file through `ApplyStamp` (`internal/stencilstore/reconcile.go:133`), which inserts a `<!-- lyx-stencil: sha256=… -->` line into the file's leading banner — or prepends a fresh one-line banner when the file has none.
  `stencilstore.Read` is a plain `os.ReadFile` (`reconcile.go:28-35`) and strips nothing.
  Every existing stencil consumer survives this only because it hands the bytes to `stencil.Fill`/`FillOptional`, whose very first act is `stripLeadingComment` (`internal/stencil/stencil.go:27`).
  `Directive`'s return is different in kind: it is a *value* in a `values` map, never a template, so it never passes through `Fill` and nothing would strip it.
  Without the strip, a real hub would inject the stamp banner into all four producer prompts, and `TestDirective_VariantsBeginWithOwnHeading`'s `## ` prefix assertion would fail — the relocation would not be behaviour-preserving.
  Strip-only rather than strip-plus-normalise keeps `Directive` exactly in step with `Fill`;
  CRLF is already handled upstream by the `*.md text eol=lf` `.gitattributes` `Reconcile` seeds beside the stencils.
- Rejected: adding a `stencilstore.ReadBody(baseDir, name)` helper that reads and strips in one call, keeping `pattern`'s allowlist at one entry — it invents shared API for a single consumer, and splitting the strip away from `Fill`'s own copy invites the two to drift.
  Also rejected: stripping at each of the four call sites (four copies of a rule that belongs in one place);
  and omitting the banner from the three new files (`ApplyStamp` prepends one regardless, so this does not avoid the problem).

### The three new files carry the conventional banner

- Decision: each new stencil opens with an explanatory `<!-- … -->` banner in the house style of the existing fifteen (see `stencils/webster/webster-prefix-recovery.md:1-5`, `stencils/loom/loom-template-plan.md:1-6`), naming which `Role` it serves, which call sites consume it, and that it is injected as a `pattern_directive` marker value rather than being a template of its own.
  The directive prose follows the banner as the file body.
- Rationale: every other stencil has one, and it is the only place a reader of the file learns what consumes it.
  It also gives `ApplyStamp` an existing banner to insert the stamp line into, rather than prepending a second one.
  These files contain no `{{.marker}}` of their own — they are injected *into* other templates — and the banner is where that non-obvious fact gets stated, along with the requirement that they stay marker-free for `stencilstore.Validate`'s sake.
- Rejected: shipping them bare, which would leave three of eighteen stencils undocumented in-file for no gain.

### Fail loud on a read failure, not silent

- Decision: `Directive` returns `(string, error)`.
  An active PATTERN whose directive stencil cannot be read is an error, propagated to the caller, wrapping `stencilstore.Read`'s own error (which already names the stencil and the base directory).
- Rationale: this overrides step 3 of `manifest/designs/pattern-directive-stencils.md`, which specified `logger.Warn` + return `""`.
  Three things point the other way.
  `stencilstore.Read`'s own doc comment states the contract: "a missing file is reported as an error, not silently substituted, per the missing-board-is-a-hard-error Shared Decision" (`internal/stencilstore/reconcile.go:24-26`).
  Every other stencil consumer in the repo already hard-errors on a read failure — `internal/burlerengine/prompt.go:42`, `internal/loomengine/plan.go:36`, `internal/websterengine/render.go:153`.
  And `internal/pattern`'s own documented posture on ambiguity is fail-loud, not fail-silent: `doc.go` argues that "resolving that ambiguity by silently disabling five agents' constraints is worse than resolving it by handing the agent the directive anyway".
  Returning `""` when PATTERN *is* active would strip the constraints from an agent prompt invisibly — the precise failure that paragraph exists to prevent.
  The fail-loud choice also avoids a second allowlist entry: no `internal/logger` import is needed.
- Rejected: the design doc's `logger.Warn` + `""` (adds an import for strictly worse behaviour);
  a bare silent `""` (fully invisible failure).

### Lazy read — the stencil is read only when PATTERN is active

- Decision: `Directive`'s control flow keeps its existing guard order and only then reads.
  The full return matrix is:
  - `l == nil` → `("", nil)`, no read attempted.
  - PATTERN inactive → `("", nil)`, no read attempted.
  - active, unknown or zero `Role` → `("", nil)`, no read attempted.
  - active, known role, read succeeds → `(stencil.StripLeadingComment(string(content)), nil)` — the **stripped body**, never the raw `Read` bytes, per the banner-strip decision below.
  - active, known role, read fails → `("", err)`.
- Rationale: preserves all five of today's behaviours bit-for-bit, so every existing test's expectation still holds.
  It also confines fixture churn to tests that actually activate PATTERN — an eager read would make every inactive-PATTERN call site depend on a seeded stencils directory, forcing changes across `burlerengine`, `websterengine`, and `loomengine` tests that have nothing to do with this task.
- Rejected: erroring on an unknown role (turns today's documented, tested "unknown role renders nothing" into a hard failure — a behaviour change outside this task's contract);
  reading eagerly before the active check.

### `Role` stays an `int` enum

- Decision: `type Role int` and the three `iota`-based members are unchanged.
  Only the body of `Directive`'s `switch` changes — each case yields a stencil *name* string instead of a directive constant, and the read happens once after the switch.
- Rationale: zero API churn at the four call sites, and it keeps the change inside the task's "no behaviour change" contract.
- Rejected: replacing `Role` with a string type keyed on the stencil name — collapses the mapping but is unrelated API churn.

### Naming and registry placement follow the existing family convention

- Decision: files at `stencils/pattern/pattern-directive-implementer.md`, `stencils/pattern/pattern-directive-review-fix.md`, `stencils/pattern/pattern-directive-orchestrator.md`.
  Go vars `PatternDirectiveImplementer`, `PatternDirectiveReviewFix`, `PatternDirectiveOrchestrator`.
  Registered names are the filenames minus `.md`.
  The three `entries` rows are appended as a trailing `pattern` family block, after the `webster` block.
- Rationale: identical `<family>/<family>-<type>-<role>.md` shape to the fifteen existing stencils, and `entries` order is the `lyx stencil list` print order, so a new family reads naturally appended.
- Rejected: inserting the `pattern` block first in `entries`.

### All five doc updates land in this commit

- Decision: five docs change in the same commit as the code.
  - `CONSTRAINTS.md` — Pattern Leaf Invariant allowlist, two entries.
  - `internal/pattern/doc.go` — the module doc.
  - `manifest/designs/pattern-directive-stencils.md` — status flip, plus corrections to **both** step 3 and step 4, plus the banner-strip step the design doc omits entirely, plus the "Related" bullet at line 57.
  - `manifest/roadmap.md` — item complete.
  - `tools/sandbox/SANDBOX-CORE-SUITE.md:232` — "all fifteen registered stencils" becomes eighteen.
- Rationale: `doc.go:53-54` currently states the pointer is "a literal relative string baked into the directive constant" — false the moment the text moves to a stencil file, so it cannot be deferred.
  The repo's task-completion rule requires the module doc, the design-doc status, and the roadmap to move with the code that makes them true.
  `manifest/roadmap.md:20` carries this item, so the roadmap does move here — this is a planned item completing, not a bugfix or polish pass.
  The design doc needs four corrections, not one: step 3 specifies the fail-silent posture this task overrides;
  step 4 claims the change is "plumbing-free" because "websterengine's functions already take it as a parameter", which `render.go:181` and `:240` contradict — both derive `fabricengine.StencilsDir(l.HubPath)` internally instead;
  no step mentions the banner strip at all, without which the relocation is not behaviour-preserving;
  and the "Related" bullet at line 57 calls `docs/shared-libs/stencil.md` "the `Fill`/`FillOptional` contract these stencil files render through", which this task's own finding disproves — these three are the first stencils that never pass through `Fill`, being injected as a values-map string instead.
  Leaving a shipped design doc asserting four things the code disproves is worse than having no design doc.
  The sandbox suite's count is a literal number that this task falsifies, in a file whose whole purpose is to be read and executed by a human or agent running the suite.
- Rejected: deferring the design doc and roadmap to a later pass;
  correcting only step 3 of the design doc;
  leaving the sandbox count stale.

## Technical context

### The function today

`internal/pattern/pattern.go`:

- `Directive(l *lyxcwd.Location, role Role) string` (line 102) — nil guard, `isActive` guard, then a three-case `switch` with a documented `default` returning `""`.
- The three constants at lines 64-89.
  Each begins with its own `## ` heading and ends with a trailing newline.
- The comment block at lines 56-63 explains why the literal pointers are *not* built from `PathspecFile`/`PathspecDir`/`lyxdirs.LyxDirName`: they are compared by fixed-string equality and substring match in this package's tests and in consumer template tests.
  That property survives the move — the string is still a fixed literal, now sourced from a `.md` file — but the comment's wording about "the three directive constants below" must be rewritten or removed along with the constants.
- The **file-header comment** at lines 1-2 goes stale for the same reason: it reads "implements the PATTERN active check and the three role-specific directive constants `Directive` selects between".
  It is a separate edit from the lines 56-63 block and is easy to miss;
  both are in scope.
- `isActive` (line 125) and the `statFile` seam (line 97) are untouched.

### The four call sites

Two are already simple assignments and take a one-line change each:

- `internal/burlerengine/engine.go:103` — `directive := pattern.Directive(e.layout, pattern.RoleReviewFix)` inside `Engine.Run`, which returns `(Result, error)`.
  `e.stencilsDir` is already a struct field (`engine.go:33`, set by `New` at line 41) and is already passed to `composePrompt` at line 123.
- `internal/loomengine/plan.go:70` — `directive := pattern.Directive(layout, pattern.RoleImplementer)` inside `PlanSpec`, which returns `(shuttleengine.Spec, error)` and already takes `stencilsDir string` as its second parameter.
  `loomengine` already imports `stencilstore`.

Two are **not** plumbing-free, contrary to the brief and the design doc.
Both are inline map-literal values, and a two-return-value call cannot sit inline as a map value in Go.
Each needs the call hoisted above the `values` map, its error checked, and the local referenced in the map:

- `internal/websterengine/render.go:179` — inside `RenderRecoveryPrompt`, which returns `([]byte, error)`.
- `internal/websterengine/render.go:238` — inside `RenderMasterPrompt`, which returns `([]byte, error)`.

Neither webster function takes `stencilsDir` as a parameter — the brief's claim that "websterengine's functions already take it as a parameter" is wrong.
Both derive it internally as `fabricengine.StencilsDir(l.HubPath)` (lines 181 and 240), for the template read that follows the map.
The simplest shape is to hoist that derivation to a local above the map and use it for both the `Directive` call and the existing template read, rather than calling `fabricengine.StencilsDir` twice.
No signature change is needed at either webster site.

Error-wrapping at the two webster sites should follow the file's existing `fmt.Errorf("webster: <what>: %w", err)` house style;
burler and loom likewise use `burler:` / `loom:` prefixes.

### Stencil mechanics

- `stencilstore.Read(baseDir, name)` (`internal/stencilstore/reconcile.go:28`) does a plain `os.ReadFile` on every call with no caching, so an on-disk edit takes effect on the next call.
  It never falls back to the embedded default, **and it does not strip the leading banner** — see the banner-strip decision above, which is the single most important non-obvious fact in this task.
- The stamp mechanics, for reference: `Reconcile` writes `ApplyStamp(shipped, BodyHash(shipped))` (`reconcile.go:133`);
  `ApplyStamp` (`stencilstore.go:103`) inserts the `lyx-stencil:` line into an existing leading banner or prepends a new one, and never alters the body;
  `BodyHash` (`stencilstore.go:41`) hashes the post-`StripLeadingComment`, LF-normalised body, so the stamp never covers itself.
  This is why the on-disk file's bytes and `Directive`'s return value can never be compared directly.
- `stencil.StripLeadingComment` (`internal/stencil/stencil.go:100`) is the exported form of the same first-`-->`-wins strip `Fill` applies internally at line 27, and is what `BodyHash` itself calls.
  Using it in `Directive` means the three code paths that need a stencil body all use one implementation.
- `stencils/stencils.go` is the only file in the top-level `stencils` package root — `//go:embed` reaches only at-or-below its own directory, so the three new embed vars must live in that file.
- `stencils/registry_test.go` walks the package directory and asserts a bijection between `*.md` files in family subfolders and `entries` rows.
  A new family subfolder needs no test change;
  a file without an `entries` row (or vice versa) fails.
- Seeding into a real hub is automatic: `stencilstore.Reconcile` runs once per process at `cmd/lyx`'s root pre-run and writes any registered file that is absent.
  Existing hubs pick the three files up on the next `lyx` invocation.
  No migration step, no manual seeding.
- Each file's **body** — what remains after the banner is stripped — must be byte-identical to the constant it replaces, trailing newline included.
- Two different `.gitattributes` files protect that byte-exactness, at two different stages, and both matter:
  - The **repo-root `.gitattributes`** covers the *source* tree, and therefore the `//go:embed` bytes baked into the binary.
    Its header states the requirement directly: embed targets "must be deterministic across checkouts regardless of a given machine's `core.autocrlf` setting".
    It lists all fifteen current stencils one line each (lines 7-21) with no glob, so the three new files need three new lines.
    Nothing machine-checks this omission — `stencils/registry_test.go` pins the registry against the on-disk tree but says nothing about `.gitattributes` — which is exactly why it is called out here.
  - The `*.md text eol=lf` file `Reconcile` seeds beside the **board copy** covers the seeded working copy under `<hub>/_board/_lyx/stencils/`.
    It does not cover the source tree, so it is not what makes the embed deterministic.
- These three are the first registered stencils that never pass through `stencil.Fill`, but `stencilstore.Validate` still parses each one's on-disk content *and* its shipped default with `stencil.TopLevelMarkers` (`internal/stencilstore/validate.go:52,56`).
  They must therefore stay marker-free: a stray `{{` anywhere in the body or the banner becomes a `lyx stencil validate` error rather than a harmless literal.
  Zero markers is a valid template — it yields zero findings — so this costs nothing as long as it stays true.
- `stencils/**/*.md` is inside the Fabric Vocabulary enforcement walk (`internal/lyxcwd/enforcement_test.go`).
  The directive prose contains none of the policed `host`/weft/warp phrases, so this is a non-issue — worth knowing only so it is not a surprise if the walk is ever consulted.

### Test-fixture landscape

Three packages already have the exact helper shape needed, all seeding from the `stencils` package's embedded defaults into a `t.TempDir()`:

- `internal/burlerengine/prompt_test.go:22` — `newTestStencilsDir(t)`, seeds burler's four.
- `internal/loomengine/prompt_test.go:20` — `newTestStencilsDir(t)`, seeds loom's two.
- `internal/websterengine/template_test.go:45` — plus `seedHubStencils(t, hub)` declared at line 122, which seeds webster's five under the real `fabricengine.StencilsDir(hub)` location.

Which of them must grow the three new files follows from the lazy-read decision:

- **`internal/loomengine/prompt_test.go`'s `newTestStencilsDir` — must.** `plan_test.go:128-141` writes a real `_lyx/PATTERN.md`, then calls `PlanSpec(layout, newTestStencilsDir(t), cfg, reg)` and asserts `## Constraints` precedes `## Step 1` in the prompt.
  PATTERN is active there, so the read fires;
  without seeding, that test hard-errors under the fail-loud posture.
- **`internal/websterengine/template_test.go`'s `seedHubStencils` — must.** `patternActiveLayout` (line ~165) plants a real `_lyx/PATTERN.md` under the hub's worktree subdirectory specifically to exercise the active branch.
  `testLayout` (line ~145) never creates the worktree subdirectory, so PATTERN stays inactive there and those tests would pass either way — but both fixtures call the same `seedHubStencils`, so seeding once covers both.
- **`internal/burlerengine/prompt_test.go`'s `newTestStencilsDir` — seed it anyway.** No burler test activates PATTERN today, so nothing there fails without it;
  `template_test.go:142`'s `pattern_directive` value is a hardcoded placeholder in a values map, not a `Directive` call.
  The default is nevertheless to add the three files, for symmetry with the loom and webster helpers and so the first burler test that ever does activate PATTERN does not fail for an unrelated reason.
  It must not be presented as a fix for a failure — there is no failure there — and it must not be presented as required by anything in this task.

`internal/pattern`'s own tests currently import only `lyxcwd` and `lyxdirs` (`pattern_test.go:15-16`) and need a new package-local `newTestStencilsDir(t)` helper importing `stencils`.
That import is test-only, and `leaf_enforcement_test.go` skips `*_test.go` files, so the leaf guard is unaffected.
No cycle: `stencils` imports only `stencilstore`, which does not reach `pattern`.

### Tier purity

`internal/pattern/pattern_test.go`'s header comment declares the file untagged Tier 1 — "it uses only `os.Stat` (via the package's `statFile` seam) and `t.TempDir`, and spawns nothing".
The new helper adds `os.MkdirAll` + `os.WriteFile` into a `t.TempDir()` and spawns nothing, so `cmd/lyx/tierpurity_test.go` stays satisfied.
The header comment should be updated to mention the seeded stencils directory so it stays accurate.

## Constraints

From `CONSTRAINTS.md`:

- **Pattern Leaf Invariant** — the one this task deliberately amends.
  Enforced by `internal/pattern/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`), an allowlist check over non-test `.go` files in the package.
  The invariant text plus all three in-test statements of the allowlist (header comment, `allowedImports` map, failure message) gain `internal/stencilstore` and `internal/stencil`, and nothing else.
  The `CONSTRAINTS.md` amendment should carry the cycle-freedom reason the way the existing text already does for `internal/lyxdirs`, since `internal/stencil` is a zero-import leaf on the same footing.
- **Stencil Ownership Invariant** — every producer prompt is read at call time from `<hub>/_board/_lyx/stencils/`, never from embedded bytes;
  `//go:embed` carries seed defaults only and is never a live read path.
  This rules out any fallback-to-embedded-default on read failure, independently reinforcing the fail-loud decision.
  `internal/stencilstore` stays the sole owner of seeding, hash-stamping, edit detection, reading, and validation.
- **Fabric Vocabulary Invariant** — binds `stencils/**/*.md`, which the three new files join.
  No policed phrase appears in the directive prose.
- **Test Tier Purity Invariant** — untagged test files perform no expensive spawns.
  Covered above.
- **Documentation Lifecycle** and the repo's task-completion rule — the module doc, design doc, and roadmap move with the code.
- **Cwd Resolution Invariant** — not engaged.
  `Directive` gains a *told* `stencilsDir` and derives no geometry of its own, matching how `burlerengine`, `loomengine`, and `treadleengine` are already told theirs.

Discovered during discussion, not from `CONSTRAINTS.md`:

- `internal/pattern/doc.go`'s "Why the pointer stays relative" section makes a claim about "the directive constant" that this task falsifies.
  It is the module doc, so it is a required update, not an optional one.

## Testing

Verify command for the plan: `go build ./... && go test ./...`.
Not a scoped package list — the load-bearing guards for this change live in at least four unrelated packages (`internal/pattern`'s leaf test, `stencils/registry_test.go`, `internal/lyxcwd`'s vocabulary and geometry walks, `cmd/lyx`'s tier-purity guard), and a hand-enumerated list is one omission away from false confidence.

### `internal/pattern` — the TDD candidate

This is the package whose contract changes, and every behaviour is already pinned by an existing test, so it is the natural TDD target: change the signature, watch the suite fail, then make it pass.

Migrate in place (all in `pattern_test.go`, all currently calling `Directive(l, role)`):

- `TestDirective_ActiveWithFile`, `TestDirective_InactiveWithoutFile`, `TestDirective_EmptyPatternFileIsActive`, `TestDirective_PatternFileAsDirectoryIsInactive`, `TestDirective_NilLayout`, `TestDirective_UnknownRole`, `TestDirective_VariantsArePairwiseDistinct`, `TestDirective_VariantsBeginWithOwnHeading`, `TestDirective_RelPathNestedSubdirectory`, `TestDirective_NonNotExistStatErrorIsActive`.

Each gains the new argument and an error assertion.
The substring assertions in `TestDirective_VariantsArePairwiseDistinct` (`_lyx/PATTERN.md` present, `_lyx/pattern/` present, `_pattern/` absent) and the `## ` prefix assertion in `TestDirective_VariantsBeginWithOwnHeading` must keep asserting the same things — they are what proves the prose survived the move intact.

Add four new tests, one per distinct property:

1. **Lazy read.** PATTERN inactive plus a deliberately bogus `stencilsDir` returns `("", nil)`.
   This is what pins the decision that limits fixture churn;
   without it, a future eager-read refactor would silently break `burlerengine`/`websterengine`/`loomengine` fixtures with nothing local failing.
2. **Missing-stencil error.** PATTERN active plus a `stencilsDir` lacking the file returns a non-nil error whose message names the missing stencil.
   `internal/loomengine/discussion_test.go:128-146` is the existing precedent for this shape, including the "names the missing stencil" assertion.
3. **Banner is stripped.** PATTERN active against a fixture whose files carry a realistic leading banner **including a `lyx-stencil:` stamp line** returns text that begins at the `## ` heading, with no `<!--` anywhere in it.
   This is the regression guard for the correctness bug that would otherwise ship: `Read` does not strip, and `Directive`'s value never passes through `Fill`.
   It is the reason the fixture below must be stamped rather than raw.
4. **Verbatim move.** Each role's returned text equals `stencil.StripLeadingComment` of the corresponding `stencils` embedded default.
   State the assertion in those terms, not as whole-file byte equality — the on-disk file carries a banner and a stamp that the return value never does.

A caveat mill-plan must not paper over: test 4 is a weak relocation guard on its own.
The three constants are deleted by this task and referenced nowhere else afterwards, so the assertion effectively compares the stencil file against itself.
What actually pins the relocation is the migrated `TestDirective_VariantsArePairwiseDistinct` and `TestDirective_VariantsBeginWithOwnHeading` — the `_lyx/PATTERN.md` and `_lyx/pattern/` substrings present, `_pattern/` absent, `## ` prefix, three variants pairwise distinct — plus `loomengine/plan_test.go`'s end-to-end ordering assertion.
Test 4 earns its place as a cheap tripwire against a future edit to a stencil file drifting from the embedded default, not as proof the move was faithful.

Plus a new package-local `newTestStencilsDir(t)` helper.
It is modelled on the three existing ones (`t.TempDir()`, seeded from the `stencils` embedded defaults) with one deliberate difference: it writes each file through `stencilstore.ApplyStamp(content, stencilstore.BodyHash(content))` rather than raw `os.WriteFile`, so the fixture matches what `Reconcile` actually puts on disk in a real hub.
The three existing helpers seed unstamped bytes, which is harmless for them because `Fill` strips either way — but for this package a raw fixture would make test 3 vacuous and let the strip bug pass green.
That difference is intentional and should be stated in the helper's own comment so a future reader does not "fix" it into consistency with the other three.

### `stencils`

`registry_test.go`'s bijection check covers the three new rows and files with no test change.
Confirm it actually runs rather than assuming — a missing `entries` row is exactly the silent failure that test exists to catch.

### `internal/websterengine`

Extend `seedHubStencils` with the three files.
`patternActiveLayout`-based tests then assert real directive content instead of the old constant;
`testLayout`-based tests keep asserting an empty `pattern_directive`.
Both `RenderRecoveryPrompt` and `RenderMasterPrompt` need their new error path reachable — a test that makes PATTERN active against a hub whose stencils dir lacks the pattern files, asserting a non-nil error, is worth having at one of the two sites at least, since these are the two call sites with genuinely new control flow.

### `internal/loomengine`

Extend `newTestStencilsDir` with the three files.
`plan_test.go`'s PATTERN-active ordering assertion (`## Constraints` precedes `## Step 1`) must keep passing unchanged — it is the end-to-end proof that the stencil-sourced directive still lands in the right place in a real producer prompt.

### `internal/burlerengine`

Seed the three files into `newTestStencilsDir` for symmetry with the loom and webster helpers.
Nothing here fails without it — no burler test activates PATTERN — so this is a consistency change, not a fix, and must not be reported as one.

### Scenarios that must be covered

- All three roles render their own distinct, heading-led text when PATTERN is active.
- Every inactive path returns empty with a nil error and touches no stencil file.
- A missing or unreadable directive stencil while PATTERN is active surfaces as an error at all four call sites, not as a silently empty prompt section.
- The stamp banner never reaches a producer prompt — the returned text starts at `## ` and contains no `<!--`.
- The rendered prose is byte-identical to what shipped before this task.
- `pattern_directive` still renders in the correct position within a real producer prompt.

## Q&A log

- **Q:** How does `internal/pattern` reach `stencilstore.Read` given the Pattern Leaf Invariant? **A:** Extend the invariant's allowlist rather than route around it. (Answered as one entry, `internal/stencilstore`; the banner-strip finding later in this log raised it to two by adding `internal/stencil` — two is the live figure, in the test's three statement sites and in `CONSTRAINTS.md`, same commit.) Verified independently that `stencilstore` imports only `internal/stencil` and `internal/logger`, neither of which reaches `pattern`, so there is no cycle risk. Rejected: four duplicated read+error-wraps at the call sites; a one-function adapter package.
- **Q:** What happens when the stencil read fails but PATTERN is active? **A:** `Directive` returns `(string, error)` and fails loud. Overrides step 3 of the pre-written design doc, which specified `logger.Warn` + `""`. Verified `stencilstore.Read`'s doc comment states the missing-board-is-a-hard-error contract verbatim, and that all four enclosing functions already return an error.
- **Q:** Are all four call sites really plumbing-free? **A:** No. `burlerengine/engine.go:103` and `loomengine/plan.go:70` are simple assignments and are trivial. `websterengine/render.go:179` and `:238` are inline map-literal values, and a two-return-value call cannot sit inline as a map value in Go, so both need the call hoisted above the map with an error check. Recorded explicitly so it does not surface as a surprise mid-implementation.
- **Q:** Should the read be lazy or eager? **A:** Lazy — read only when PATTERN is active and the role is known. Preserves all five existing behaviours bit-for-bit and confines fixture churn to the two test packages that actually activate PATTERN.
- **Q:** Should `Role` become a string type keyed on the stencil name? **A:** No. Keep `type Role int`; only the `switch` body changes, from yielding a constant to yielding a stencil name. Unrelated API churn against a task whose contract is "no behaviour change".
- **Q:** File names, Go var names, and registry order? **A:** Follow the existing family convention exactly — `stencils/pattern/pattern-directive-{implementer,review-fix,orchestrator}.md`, vars `PatternDirective{Implementer,ReviewFix,Orchestrator}`, appended to `entries` as a trailing `pattern` family block.
- **Q:** Which new tests beyond migrating the existing ones? **A:** Four, each pinning a distinct property: lazy-read (inactive + bogus `stencilsDir` → `("", nil)`); missing-stencil error naming the stencil; banner-strip (a stamped fixture's directive returns text starting at `## ` with no `<!--`); and equality with `stencil.StripLeadingComment` of the `stencils` embedded default — stripped-body equality, never whole-file byte equality, since the on-disk file carries a banner and stamp the return value never does.
- **Q:** Which docs land in this commit? **A:** All five — `CONSTRAINTS.md`, `internal/pattern/doc.go`, `manifest/designs/pattern-directive-stencils.md`, `manifest/roadmap.md`, and `tools/sandbox/SANDBOX-CORE-SUITE.md:232` (its "all fifteen registered stencils" becomes eighteen). `doc.go:53-54` literally says the pointer is "baked into the directive constant", which goes stale the moment the text moves, so it cannot be deferred. The design doc needs four corrections, not one — steps 3 and 4 are both false, the banner strip is absent from it entirely, and its line-57 "Related" bullet claims these files render through `Fill`, which they never do.
- **Q:** Verify command? **A:** `go build ./... && go test ./...`. The invariant guards span four-plus unrelated packages; a scoped list is one omission away from false confidence.
- **Q:** Do the three new files need repo-root `.gitattributes` lines? **A:** Yes, three of them, one per file. That file pins every embed target individually with no `stencils/**` glob, and nothing machine-checks an omission. The `*.md text eol=lf` that `Reconcile` seeds is a different file covering the board copy, not the source tree, so it does not make the embed deterministic.
- **Q:** These stencils never pass through `stencil.Fill` — does anything else parse them? **A:** Yes, `stencilstore.Validate` parses every registered file and its shipped default with `stencil.TopLevelMarkers`. The three must stay marker-free; zero markers is valid and yields zero findings, but a stray `{{` would become a validate error rather than a rendering no-op.
- **Q:** `stencilstore.Read` returns the stamp banner and `Directive`'s return never passes through `stencil.Fill`, so nothing strips it — how is that handled? **A:** `Directive` calls `stencil.StripLeadingComment` on the bytes it reads, which makes the leaf-invariant amendment two entries (`internal/stencilstore` + `internal/stencil`) rather than one. Verified independently: `Reconcile` stamps via `ApplyStamp` at `reconcile.go:133`, `Read` is a bare `os.ReadFile` at `reconcile.go:28`, and `FillOptional` strips at `internal/stencil/stencil.go:27` — which is exactly why every other consumer is unaffected and this one is not. `internal/stencil` imports no internal package, so the second entry is cycle-free by construction. Rejected a new `stencilstore.ReadBody` helper (shared API for one consumer, and it would fork the strip away from `Fill`'s copy).
