---
verdict: APPROVED
findings:
  - id: F1
    severity: MEDIUM
    class: design
    origin: lens:guard-reach
    location: _lym/plan/06-shed-keys-in-shed-yaml-alone.md
    summary: D13's local-layer refusal is bypassed on start and resume, because the hub reconcile prunes a retired key from a pair's local loom.yaml or darn.yaml before any load reads it, so the run silently takes shed.yaml's driver
    fix: In card 6, make the reconcile keep a retired key of a category other than CategoryNone in a LayerLocal file, so the next load refuses it through ValidateLocal; add internal/configsync#reconcileSparseFile to card 6's Edit list for that (checked with validate-plan --overlay; it adds no blocking finding). Add a test row that reconciles a pair whose local loom.yaml holds driver_kind and then loads it, wanting the refusal that names the file, the key and the pair's local shed.yaml. If the parent would rather accept the silent prune, record that as an Added after Discussion entry through the parent, and have card 6's Intent state it.
  - id: F2
    severity: MEDIUM
    class: consistency
    origin: lens:affected-set
    location: _lym/plan/27-drop-tapestry-preflight.md
    summary: card 27 deletes CheckSeed and the Tapestry-Preflight row but misses part of the text that names them, a pattern background file and a pinned spec among them
    fix: "Add three targets to card 27's Edit list, each with an Intent line. pattern/PATTERN-cwd-resolution.md drops loomengine.CheckSeed from its three-tier sentence at line 21. contracts/specs/shed-recipe-spec.md names a loom-specific row that still exists in place of Tapestry-Preflight at line 21. internal/loomcli#waitForCurrentProducer, whose span (207-239, confirmed with lym quarry resolve) holds the comment at internal/loomcli/smoke_test.go:214, stops citing Tapestry-Preflight. Add one Intent line, with no new target, for the file header of internal/loomshed/gatefindings_test.go (lines 4-6): it stops citing Tapestry-Preflight. Card 27 already touches that file through its Delete of TestTapestryPreflight_StuckSurfacesItsFailures, and a file target there is refused by redundant-file-target and edit-test-file. Checked with validate-plan --overlay; it adds no blocking finding."
  - id: F3
    severity: MEDIUM
    class: consistency
    origin: handler
    location: _lym/plan/26-preflight-loads-batcher.md
    summary: card 26 declares Env.LoadsBatcher as a package-level var glyph, plan:internal/shedrecipe#LoadsBatcher -> var LoadsBatcher bool, and does not target shedrecipe's Env, while its Intent adds a field to Env; card 35 uses that same wrong glyph
    fix: In card 26, delete the Create line for plan:internal/shedrecipe#LoadsBatcher and add an Extends section listing internal/shedrecipe#Env. In card 35, replace the Uses line plan:internal/shedrecipe#LoadsBatcher with internal/shedrecipe#Env. Checked with validate-plan --overlay; it adds no blocking finding.
  - id: F4
    severity: MEDIUM
    class: decision
    origin: handler
    location: _lym/plan/00-overview.md
    summary: "The overview keeps tacklerFor and arbiterRecipeDescription branching on shedrun.RecipeDarn. D8 says every remaining RecipeDarn or RecipeTapestry branch in internal/loomcli reads the recipe's rows or keys, and its acceptance says the wiring holds no recipe-name branch. A third loom recipe would get tapestry's description: a discussion, plan and webster run."
    fix: In card 35, add internal/loomcli#tacklerFor and internal/loomcli#arbiterRecipeDescription to the Edit list. Each picks the darn description when recipeHasRow(recipe, loomshed.NameDarnWrite) holds and the tapestry one when the recipe holds a Plan-Write row, with pawl's case still by name under D2. Drop the overview's loom-cli shared-decision bullet that keeps them keyed by name. Alternatively, record the exemption as an Added after Discussion entry through the parent. Checked with validate-plan --overlay; it adds no blocking finding.
  - id: F5
    severity: LOW
    class: design
    origin: handler
    location: _lym/plan/23-darn-record-write-row.md
    summary: "Card 23 sets darn's Drift rows to subject decision-record, but the Drift agents read the subject loomCLI.wire tells them. That stays board-entry for darn until card 35 drops the branch, so card 23's stated effect (the Drift rows read the record) depends on a later card."
    fix: Move the bullet that drops the drift-subject branch from card 35 to card 23, and add internal/loomcli#loomCLI.wire to card 23's Edit list. Card 35 keeps wire for its other bullets. Checked with validate-plan --overlay; it adds no blocking finding.
  - id: F6
    severity: LOW
    class: design
    origin: handler
    location: _lym/plan/19-darn-write-guards.md
    summary: card 19's stop-marker halt skips the discussion guard, and its Intent does not state what bounds that skip, the bound D6 gives
    fix: Add one sentence to card 19's Intent after step 4. The marker halt skips the guard but lets nothing through, since Darn-Write returns Stuck before the run reaches Drift-Verify, whatever earlier spawns of the row committed. Only the writer's own session writes the marker, and the marker is removed before each spawn.
  - id: F7
    severity: LOW
    class: design
    origin: handler
    location: _lym/plan/22-merged-loom-builders.md
    summary: "Cards 22, 28 and 32 leave facts to be found while they run. Card 22 asks whether renameRetiredRows writes on an empty map; it does not, since shedengine.RenameProducers returns before any write when no retired row is named. Card 28 says any other caller a grep finds; Rows and AmendmentTarget are the only callers. Card 32 says a grep joins every old-text assertion; those sit in TestDecisionVerb_Refusals, which card 32 already lists."
    fix: In card 22, replace the conditional len guard with the fact that renameRetiredRows makes no status write on an empty map, so no guard is added. In card 28, drop the clause about any other caller a grep finds, and name Rows, AmendmentTarget and RecordEngine as the only callers. In card 32, replace the grep sentence with the fact that the old texts are asserted only in TestDecisionVerb_Refusals in internal/loomcli/decision_test.go.
  - id: F8
    severity: LOW
    class: consistency
    origin: handler
    location: _lym/plan/33-peers-own-recipe.md
    summary: card 33's Intent has loomRowNumber take the recipe, but its Edit glyph carries no re-sign arrow
    fix: "In card 33's Edit list, change internal/loomcli#loomRowNumber to internal/loomcli#loomRowNumber -> func loomRowNumber(t *testing.T, recipe, row string) int. Checked with validate-plan --overlay; it adds no blocking finding."
  - id: F9
    severity: LOW
    class: design
    origin: lens:guard-reach
    location: _lym/plan/30-loom-only-refusal.md
    summary: card 30's loomPath refusal in armAt fires for every verb the loom pre-run arms, including verbs that address no run, such as fmt, lint-pattern and lint-comments; D2 scopes the refusal to verbs that address a run
    fix: In card 30, apply loomRunRefusal only to the verbs that address a run, through a named set beside verbUsesLightweightWiring, and leave fmt, lint-comments, lint-pattern, validate-description and commit-records unrefused. Add a TestArmAt_LoomPathRefusesNonLoomRecipe row where verb fmt over a batten seed takes no refusal.
  - id: F10
    severity: LOW
    class: consistency
    origin: lens:guard-reach
    location: _lym/plan/14-one-discussion-validator.md
    summary: "Card 14's wireLightweight fallback guards a state the later cards rule out. On a RecordEngine parse error it keeps CitedRecord true and logs a Warn. The only names the parse refuses are unknown recipes, and card 30 refuses those before any wiring call."
    fix: In card 14, drop the fallback. wireLightweight sets CitedRecord from citedRecord and treats its error as a wiring fault (panic naming the recipe), since an unknown recipe is refused before wiring runs.
  - id: F11
    severity: LOW
    class: consistency
    origin: lens:affected-set
    location: _lym/plan/09-move-plan-keys-to-tapestry.md
    summary: card 9 moves the plan keys to tapestry.yaml but misses two places that still place them in loom.yaml, the planWrite entry's comment and a loomrecipe test fixture's Plan-Review ConfigFile
    fix: "Add internal/shedrecipe#planWriteGateSpec to card 9's Edit list. Its doc comment, at internal/shedrecipe/entries_planwrite.go:74 inside the span 73-87 that lym quarry resolve confirmed, names tapestry.yaml's plan_lenses. Add internal/loomrecipe#TestReviewBudget_NegativeRefused (span 187-243, which holds internal/loomrecipe/reviewbudget_test.go:222-223). Its negative-budget row builds the Plan-Review segment with ConfigFile tapestry.yaml and wants tapestry.yaml. Checked with validate-plan --overlay; it adds no blocking finding."
  - id: F12
    severity: LOW
    class: consistency
    origin: lens:affected-set
    location: _lym/plan/22-merged-loom-builders.md
    summary: card 22 deletes TapestryRetiredRows but leaves internal/loomrecipe/loomrecipe.go's file header and the tapestry-recipe.yaml comment at line 59 naming it
    fix: "Add contracts/recipes/tapestry-recipe.yaml to card 22's Edit list, its comment at line 59 saying a retired row is renamed through RetiredRows. Add one Intent line, with no new target, saying internal/loomrecipe/loomrecipe.go's file header names New alone. Card 22 already edits that file through New and Routing, and a file target beside them is refused by redundant-file-target. Checked with validate-plan --overlay; it adds no blocking finding."
---

### [MEDIUM] D13's local-layer refusal is pruned away before it fires

**Location:** `_lym/plan/06-shed-keys-in-shed-yaml-alone.md`

**Issue:** `lym loom start` and `resume` run `hubreconcile.EnsureOrWarn` before arming (`internal/loomcli/arm.go:150-155`).
The reconcile walks each pair (`internal/hubreconcile/walk.go:166`) and reconciles the local layer (`internal/configsync/configsync.go:424`).
That runs `yamlengine.Prune` against the template (`internal/configsync/configsync.go:572`).
Once card 6 drops the seven keys from loom's and darn's templates, a pair's local `driver_kind` is pruned silently.
The load then never meets `ValidateLocal`'s refusal, and the run takes `shed.yaml`'s driver: the silent switch D13 refuses the key to prevent.
Card 6's test row calls `LoadConfig` directly and never passes the reconcile.

### [MEDIUM] Card 27 misses text that names CheckSeed and Tapestry-Preflight

**Location:** `_lym/plan/27-drop-tapestry-preflight.md`

**Issue:** These still name the removed check and row after card 27, and no card lists them:
- `pattern/PATTERN-cwd-resolution.md:21` gives `loomengine.CheckSeed` as the third resolution tier.
- `contracts/specs/shed-recipe-spec.md:21`, a pinned spec, gives `Tapestry-Preflight` as its example of a loom-specific row.
- The comment at `internal/loomcli/smoke_test.go:214`, inside `waitForCurrentProducer`, cites the row.
- The file header of `internal/loomshed/gatefindings_test.go` (lines 4-6) cites it too, and card 27 deletes its `TestTapestryPreflight_StuckSurfacesItsFailures` at line 127.

### [MEDIUM] LoadsBatcher is declared as a package var, not an Env field

**Location:** `_lym/plan/26-preflight-loads-batcher.md`

**Issue:** Card 26's Create glyph `plan:internal/shedrecipe#LoadsBatcher -> var LoadsBatcher bool` names a package-level variable.
Its Intent says `shedrecipe.Env` gains `LoadsBatcher bool`, and card 35 sets `Env.LoadsBatcher`.
Card 26 lists no `internal/shedrecipe#Env` target, unlike cards 14 and 17, which add Env fields through `Env`.

### [MEDIUM] Recipe-description branches stay keyed by recipe name against D8

**Location:** `_lym/plan/00-overview.md`

**Issue:** D8: "Every remaining branch on `RecipeDarn` or `RecipeTapestry` in `internal/loomcli` reads the recipe's rows or keys instead".
The acceptance criterion says "the wiring holds no `tapestry` or darn recipe-name branch".
The overview's loom-cli shared decision keeps `tacklerFor`'s switch (`internal/loomcli/drivewiring.go:119-125`) and `arbiterRecipeDescription` (`internal/loomcli/arbiterprompt.go:18-26`) keyed by name.
No Added after Discussion entry licenses that.
A third loom recipe would fall through to the tapestry description, which names discussion, plan and webster.

### [LOW] Card 23's Drift subject depends on card 35's wire change

**Location:** `_lym/plan/23-darn-record-write-row.md`

**Issue:** The Drift agents' table reads `tables.subject` (`internal/loomcli/driftwiring.go:129`), which `loomCLI.wire` sets to `SubjectBoardEntry` for darn (`internal/loomcli/wiring.go:785-789`).
Card 23 changes only the recipe row's `subject`.
Until card 35 drops the wire branch, darn's Drift agents still read the board entry.

### [LOW] Card 19's stop-marker halt states no bound

**Location:** `_lym/plan/19-darn-write-guards.md`

**Issue:** Step 4 runs ahead of the guard, so a present marker skips D5's guard.
The Intent does not carry D6's bound: the halt lets nothing through, since Darn-Write returns Stuck before Drift-Verify.

### [LOW] Three cards leave a fact to be found at run time

**Location:** `_lym/plan/22-merged-loom-builders.md`

**Issue:** Card 22: "guard the call with `len(...) > 0` if it would".
`shedengine.RenameProducers` returns before any write when `namesRetiredRow` finds no retired row (`internal/shedengine/rename.go:41-42`).
Card 28: "any other caller a grep for `parseEmbeddedRecipe(` finds".
The only callers are at `internal/loomrecipe/names.go:72` and `internal/loomrecipe/names.go:86`.
Card 32: "A grep for `Darn-Write session` and `board note titled` … each joins this card".
The assertions sit in `internal/loomcli/decision_test.go:401-431`.

### [LOW] loomRowNumber changes its signature with no re-sign arrow

**Location:** `_lym/plan/33-peers-own-recipe.md`

**Issue:** The Intent says "`loomRowNumber` takes the recipe".
Its definition is `func loomRowNumber(t *testing.T, row string) int` (`internal/loomcli/peers_test.go:21`), and the Edit glyph has no arrow.

### [LOW] The loom-only refusal also refuses verbs that address no run

**Location:** `_lym/plan/30-loom-only-refusal.md`

**Issue:** Every verb but the group, `checklist` and the review groups reaches `arm` (`internal/loomcli/cli.go:220-232`).
So `fmt`, `lint-pattern`, `lint-comments`, `validate-description` and `commit-records` would be refused in a worktree whose seed names a non-loom recipe other than pawl.
D2 refuses only "every `lym loom` verb that addresses a run".
The fork could not confirm whether a batten worktree holds a self seed that `lym loom` reads, so how often this bites is uncertain.

### [LOW] wireLightweight's parse-error fallback guards an unreachable state

**Location:** `_lym/plan/14-one-discussion-validator.md`

**Issue:** `parseEmbeddedRecipe` refuses only unknown names (`internal/loomrecipe/names.go:94`).
After card 30 they are refused in `armAt` before wiring, and on the `lym shed` path before arming.
The fallback that keeps `CitedRecord` true with a Warn is reached by no input and no test.

### [LOW] Card 9 misses two places that put plan keys in loom.yaml

**Location:** `_lym/plan/09-move-plan-keys-to-tapestry.md`

**Issue:** `internal/shedrecipe/entries_planwrite.go:74` says "loom.yaml's plan_lenses".
`internal/loomrecipe/reviewbudget_test.go:222-223` builds the Plan-Review segment with `ConfigFile: "loom.yaml"` and wants `loom.yaml` in the refusal.
Card 9 moves both keys to `tapestry.yaml` and lists neither site.

### [LOW] Card 22 leaves the deleted TapestryRetiredRows named

**Location:** `_lym/plan/22-merged-loom-builders.md`

**Issue:** `internal/loomrecipe/loomrecipe.go:1` reads "loomrecipe.go implements New and TapestryRetiredRows".
`contracts/recipes/tapestry-recipe.yaml:59` says a retired row is renamed "through TapestryRetiredRows".

## Rejected

- lens:repo-invariants finding 1 (card 18's conformance row lacks `CommitDiscussion`): `newTestEnv` fills `CommitDiscussion: func() error { return nil }` (`internal/shedrecipe/fixture_test.go:199`), so the row reaches the unknown-key refusal.
- lens:affected-set finding F1's member `internal/loomcli/bootstrap.go:142`: that comment is the doc of `dispositionForHandshake`, whose span (136-162, confirmed with `lym quarry resolve`) card 27 already edits, naming its doc among the re-pointed docs.
  The finding's other members are kept in F2, merged with lens:repo-invariants findings 2 and 3.

## Notes

- Fidelity: the plan carries D1 to D14.
  - The pinned-spec updates are in cards 1–3, 10–11 and 27.
  - D7's rubric darn form is in card 23.
  - D3's `--amends-plan` re-keying and D4's reworded `decisionWay*` refusals are in card 32.
  - The PATTERN declarations in cards 7 and 36 match the record's declared text.
- Verify lines: no card carries a `**Verify:**` line, so none names a raw test command.
- Board scope: no card edits this task's board entry, files a task or promotes a note. `lym board get psr-rv` found no entry for the run's slug.
