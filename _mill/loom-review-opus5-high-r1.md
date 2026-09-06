# `loom` crucible review — glyph-hardening campaign, ROUND 1 (opus5-high)

> Independent review per `_mill/loom-review-prompt.md`. Clean-room: findings below were formed without reading any prior review file in this worktree.
> Status: IN PROGRESS — appended incrementally as evidence lands.

## Executive summary

_(written last)_

## What was tested

### Hermetic baseline (before any source change)

Run at review start, on the clean `crucible-loom-glyph-hardening` tree (HEAD `61265b9bc`, derived from `main`).

| Command | Result |
|---|---|
| `go build ./...` | exit 0, clean |
| `go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomrecipe/... ./internal/loomshed/... ./internal/shedengine/... ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/hubgeom/... ./internal/planparser/... ./internal/planglyph/...` | exit 0, clean |
| `go test -count=5 <same 11 pkg sets> ./cmd/lyx/...` | exit 0 — 12 packages `ok`, 0 `FAIL`, 0 "no test files" |

Baseline is green; nothing red before the glyph scenarios started, so no pre-existing-redness finding.

### Live driving — the standalone probe harness

`lyx loom run` cannot be driven without spawning LLM sessions, but the glyph surface itself is reachable through real CLI verbs against a real git repository with real quarry, at zero LLM cost, via webster's **standalone mode** (`--target-dir`/`--plan-dir`, `internal/webstercli/wiring.go`).
`lyx webster validate` calls `planglyph.Validate` — byte-for-byte the function `Plan-Revalidate` (recipe row 10, `require_approved: true`) runs, per the Gate Self-Check Parity Invariant. So every observation below is the real row's own behaviour.

Fixture: a real Go module at `<scratch>/sbx`, real `git init`/commit, base SHA `42878ec60`:

- `internal/alpha/alpha.go` — `func Existing() string`, `func ToRename() int`
- `internal/beta/beta.go` — `func BetaOne() string`

Dev binary deployed with `./deploy-dev` → `/home/knatte/Code/loomyard/wts/crucible-loom-glyph-hardening/.dev-bin/lyx` (built at `a4723e91e`).

**Run 1** — probe plan with a `Create`-in-new-unit handle card, a `Create`-in-existing-unit handle card, a `Create` on an already-existing glyph, and a member-glyph/file-self-glyph card pair:

```
env -C <sbx> .dev-bin/lyx webster validate --plan-dir <sbx>/plan
```

Result (verbatim, 2 findings):

- `create-already-exists` / `3-already-exists-create` — "Create target \"internal/alpha#Existing\" already resolves found", blocking. **Create inversion's blocking half: works live.**
- `containment-file-overlap` / `4-member-edit` — "card 4's member glyph physically overlaps card 5-file-self-edit's own file self glyph naming \"internal/beta/beta.go\"", blocking. **Resolve-backed containment tier: works live**, reading `ResolveResult.Symbols[].File` as designed.
- **No `create-new-unit` finding** for card 1's `plan:internal/gamma#NewThing` in a package that does not exist → finding F14.

The three handle-bearing card files were rewritten on disk by the same call (mtimes advanced), confirming that `Plan-Validate`/`Plan-Revalidate` **mutate the plan directory** as a side effect of validation.

**Run 2** — card 1's draft handle changed to a deliberately non-canonical `plan:internal/gamma#DraftSpelling` (declaration head `func NewThing() string`), card 2's declaration head changed to the stencil's own literal `type AddedHere struct{...}`:

- Card 1's declaration rewrote to `plan:internal/gamma#NewThing`, **and card 2's `Uses` reference to the same handle rewrote with it**. **Scenario 1's canonicalization half — plan-wide rewrite via `planparser.RewriteRefs`, reaching a non-declaring card — works live.**
- New blocking finding `handle-name-failed`: "handle \"plan:internal/alpha#AddedHere\" failed naming: declaration does not parse (parse)" → finding F15.

**Run 3** — a `Rename`-focused plan: card 1 renames the free function `internal/alpha#ToRename`, card 2 renames the method `internal/alpha#Counter.Count`, card 3 is a bare-glyph `Create` in a brand-new unit, cards 4/5 are a member glyph and its own unit self glyph.

- `containment-unit-overlap` / `4-member-edit` — "card 4's member glyph in unit \"internal/beta\" physically overlaps card 5-unit-self-edit's own self glyph naming the same unit", blocking. **Syntactic containment tier: works live.**
- `create-new-unit` / `3-bare-glyph-new-unit` — "Create target \"internal/delta#Fresh\" introduces a new unit", informational, unit named correctly. **Create inversion's new-unit branch: works live** — for a *bare glyph* target (contrast F14).
- `handle-name-failed`, blocking, verbatim:
  `handle "plan:internal/alpha#Counter.Tally" failed naming: glyph: parse "internal/alpha#Counter.Tallyer.Count" as go: member has more components than the language allows (Counter.Tallyer.Count) (member_too_deep)`
  → **F9 confirmed live in its worst form.** `func (c *Counter) Count() int` with `Glyph.Name == "Count"` has its first substring hit inside the receiver type `Counter`, so the derived declaration became `func (c *Counter.Tallyer) Count() int`. **No method can be renamed through the glyph plan alphabet.**
- Card 1's free-function rename produced no finding, and re-running with a deliberately misspelled draft (`plan:wrongunit#RenamedThing`) rewrote it to `plan:internal/alpha#RenamedThing` in the declaring card **and** in card 2's `Uses`. **A `Rename` to-side handle canonicalizes correctly for a free function.**

**Run 4 — the two-batch Webster drive (the headline).**

Standalone `lyx webster run` cannot start Master at all on this host — see F16 — so the bracket verbs were driven directly, exactly as Master drives them. `state.json` was hand-written (schema `internal/websterengine/state.go:101-137`) with the real recomputed plan fingerprint, and the Claude fork transcript was seeded as a fixture. Everything else — the plan, the git repository, the commits, quarry's resolve and delta, and both bracket verbs themselves — is real, and both verbs ran as real `lyx` processes.

Plan (validated clean first: `{"cards":2,"ok":true,"valid":true}`):

- card 1 `greet-helper` — `**Create:** - \`plan:internal/gamma#Greet\` -> \`func Greet() string\``
- card 2 `beta-uses-greet` — `**Edit:** internal/beta#BetaOne`, `**Uses:** plan:internal/gamma#Greet`

Sequence and observed results:

1. `lyx webster begin-batch 1` → **ok**, `start_sha c035ebe16`, no advisories.
2. Batch 1's work performed for real: `internal/gamma/gamma.go` written with `func Greet() string`, committed as `905efb59f`.
3. `lyx webster record-batch 1` → **ok**, `status done`. Warnings included, verbatim:
   `scope-outside-plan[informational]: symbol "internal/gamma#Greet" in file "internal/gamma/gamma.go" was touched outside the completed batch's own target glyphs`
   → **F11 confirmed live**: the symbol the plan explicitly asked to be created is reported as out-of-scope.
4. Plan on disk after `BindHandles`. Card 2's `Uses` bound correctly to `internal/gamma#Greet` — **the second half of scenario 1 works.** But card 1's own declaration became
   ``- `internal/gamma#Greet` -> `func Greet() string` `` — the `plan:` prefix stripped out of the *declaration* bullet.
5. `lyx webster begin-batch 2` → **refused**:
   `webster: on-disk plan fingerprint does not match this run's recorded state: … 65c13544… does not match … 3bc6f3ef…; the plan changed since state.json was created — re-run "lyx webster run --fresh" …`
   → **F4 confirmed live.** Webster's own `BindHandles` rewrite tripped webster's own staleness guard. The advised recourse (`--fresh`) archives state and starts the same plan over, hitting the same wall again.
6. Fingerprint re-stamped by hand to get past F4; `lyx webster begin-batch 2` again → **refused**:
   `webster: plan re-resolution at begin-batch reported a blocking finding: handle-malformed/1-greet-helper[blocking]: card 1 Create: entry "\`internal/gamma#Greet\` -> \`func Greet() string\`" does not match the required \`plan:<handle>\` -> \`<declaration head>\` grammar; card-field-empty/1-greet-helper[blocking]: card 1's **Create:** label carries no targets`
   → **F3 confirmed live**, plus a second consequence not predicted from the read: the Create group's ref list is emptied too, so `card-field-empty` fires as well.

**Conclusion of run 4: a two-card plan whose first card creates a symbol through a `plan:` handle — the exact shape the plan format prescribes — cannot reach its second batch. Webster wedges permanently, twice over, and neither wall has an operator recourse short of hand-editing the plan.**

## Findings

### F1 — `RecordBatch` treats an INFORMATIONAL drift finding as blocking (BLOCKING, CONFIRMED-by-trace)

`internal/websterengine/recordbatch.go:252-262`.

`DetectDrift` returns a mixed severity set: `plan-references-deleted-symbol` is `SeverityBlocking`, `rename-candidate` is `SeverityInformational` (`internal/planglyph/drift.go:135-143`).
`RecordBatch` collects **every** finding into `driftBlocking` with no severity filter and fails the batch with `ErrCardNotDone`.
Every other consumer in the repo filters (`loomshed/planvalidate.go:40`, `loomcli/validate.go:76`, `webstercli/validate.go:32`, `websterengine/runlevel.go:178`, `websterengine/beginbatch.go:190`) — this is the one that does not.

Scenario: batch renames a symbol inexactly enough that quarry classifies it a `RenameCandidate` rather than an exact `Renamed`, and some other card still references the old glyph → `DetectDrift` emits the informational `rename-candidate` → `RecordBatch` returns `ErrCardNotDone` and the batch is rejected, though `drift.go`'s own contract says the evidence tier "never auto-repairs" and the decision "is the reviewer's".
This defeats the entire evidence tier: it can never surface to a reviewer because it kills the batch first.

Fix: filter by `Severity`; route `SeverityInformational` drift findings into `RecordResult.Warnings`, exactly as `ScopeGuard`'s output already is (recordbatch.go:243-245).

### F2 — `DetectDrift` gate one can never match a real plan's Rename pair (BLOCKING, CONFIRMED-by-trace)

`internal/planglyph/drift.go:22-35` and `:84`.

`renameCardPairs` indexes `pairs[p.Old] = p.New` verbatim. Gate one then tests `want == newID` where `newID` is `delta.Renamed[i].To.ID` — a bare quarry glyph.
But the plan format **requires** a symbol `Rename` pair's `New` side to be a `plan:` handle (`contracts/specs/loom-plan-spec.md:218`, check `rename-to-not-handle`), and `CanonicalizeHandles` rewrites it to `plan:<canonical-glyph>` — still `plan:`-prefixed (`internal/planglyph/handle.go:145`).
So `want` is `"plan:sub#New"` and `newID` is `"sub#New"`: gate one never fires for any plan that passes its own validator.

Consequence: the card's OWN declared, expected rename is misclassified as drift. Gate two does not save it (the Rename card itself references `p.Old`, so `refCards[oldID]` is non-empty), so `DetectDrift` queues a `driftRepair`, rewrites `oldID`→`newID` plan-wide — clobbering the Rename card's own `Old` side — and appends a spurious `Tier: "exact"` amendment claiming a repair that was really the plan working as designed.

The existing coverage does not catch this because `internal/planglyph/drift_integration_test.go:68` builds the gate-one fixture as ``**Rename:**\n- `sub#Old` -> `sub#New` `` — a pair the plan format itself rejects under `rename-to-not-handle`.

Fix: strip `planparser.HandlePrefix` from the New side when indexing, reusing the same normalisation `donecheck.go:34` (`resolveKeyFor`) and `handle.go:222` (`BindHandles`' `expected`) already apply. Re-point the integration test at a spec-legal handle pair.

### F3 — `BindHandles` corrupts the declaring `Create` card's own declaration grammar, wedging every later batch (BLOCKING, CONFIRMED-by-trace)

`internal/planglyph/handle.go:239-250` → `internal/planparser/rewrite.go:122-136` → `internal/planparser/parse.go:678-685` → `internal/planparser/validate.go:580-590`.

`BindHandles` substitutes `plan:X` → `X` plan-wide through `RewriteRefs`. `rewriteBulletLine` matches the `` `old` -> `new` `` shape, which is *also* the `Create` declaration shape, so the declaring card's own sub-bullet is rewritten from
``- `plan:internal/foo#NewThing` -> `func NewThing() *Thing` `` to ``- `internal/foo#NewThing` -> `func NewThing() *Thing` ``.

On the next parse that payload no longer satisfies `splitHandleDeclaration` (no `plan:` prefix) but still contains `" -> "`, so `parseCreateField` drops it into `CreateRaw` — and `checkHandleMalformed` emits a blocking `handle-malformed` for every `CreateRaw` entry.

Each `lyx webster begin-batch` re-parses the plan from disk (`internal/webstercli/beginbatch.go:58`) and runs `planglyph.ValidateFormat`, refusing on any blocking finding with `ErrPlanDrifted` (`beginbatch.go:196-198`).
So: batch 1 lands a `Create` card carrying a handle → `RecordBatch` binds it → **batch 2's `begin-batch` refuses forever**. The plan cannot be un-wedged without a hand edit.

### F4 — Webster's plan-fingerprint gate is incompatible with the glyph surface's own in-flight plan rewrites (BLOCKING, CONFIRMED-by-trace)

`internal/websterengine/beginbatch.go:170-176`, `internal/websterengine/fingerprint.go:22-51`.

`fingerprint` hashes the name+contents of **every** `*.md` in `planDir`. Three glyph-surface code paths legitimately rewrite that directory *while the run is in flight*:

- `CanonicalizeHandles` → `RewriteRefs` (from `resolvePass`, reached by both `BeginBatch`'s own `ValidateFormat` and `Plan-Validate`/`Plan-Revalidate`),
- `BindHandles` → `RewriteRefs` (from `RecordBatch`),
- `DetectDrift` → `RewriteRefs` **and** `AppendAmendment`, which creates `amendments.md` — a brand-new `.md` file in `planDir`, hashed by `fingerprint` (and explicitly a "known non-card file" per `loom-plan-spec.md:222`).

None of them re-stamps `State.PlanFingerprint`. So the very first batch whose `RecordBatch` binds a handle or appends an amendment makes the next `BeginBatch` fail with `ErrFingerprintMismatch`, whose message tells the operator to `lyx webster run --fresh` — which archives state and starts over, hitting the same wall again.

This is independent of F3: it fires for a `Rename` card (drift amendment) and for a `Create` card (bind rewrite) alike, and F3's fix does not touch it.

Fix: re-stamp `State.PlanFingerprint` after webster's own sanctioned rewrites (end of `BeginBatch` and end of `RecordBatch`), so the guard keeps catching *foreign* edits between the two while tolerating the pipeline's own. Also exclude `amendments.md` from the fingerprint, since it is an append-only log, not plan identity.

### F5 / F6 / F7 — the dispatch-boundary re-resolution validates already-executed cards against the post-change tree (BLOCKING, CONFIRMED-by-trace)

`internal/websterengine/beginbatch.go:183` calls `planglyph.ValidateFormat(deps.Plan, …)` over the **whole** plan on every batch dispatch.
A plan describes intended change, so a card whose work already landed necessarily contradicts the current tree:

- **F5 (`Delete` card):** the deleted glyph is still in the card's `Targets`, so `statusFindings` reports blocking `glyph-not-found` (`internal/planglyph/resolve.go:102-116`) from the next batch onward.
- **F6 (`Rename` card):** the `Old` glyph is projected into `Targets` (`loom-plan-spec.md:177`) and no longer resolves → blocking `glyph-not-found`; and `CanonicalizeHandles`' `renameDeclSource` cannot resolve `Old` any more → blocking `rename-old-unresolved` (`internal/planglyph/handle.go:47-56`).
- **F7 (`Create` card with a bare-glyph target rather than a handle):** the created symbol now resolves `found`, so the Create inversion reports blocking `create-already-exists` (`internal/planglyph/create.go:70-77`).

Each of these wedges `begin-batch` with `ErrPlanDrifted` for the remainder of the run.
Together with F3 this means **no multi-batch plan containing a `Create`, `Delete` or `Rename` card can complete through Webster** — which is every non-trivial plan.
Round 2's successful full-pipeline pass does not contradict this: it ran a minimal, single-batch, non-glyph task.

Root cause is one gap, not three: nothing scopes the re-resolution to cards that have **not yet executed**. `deps.State.Batches[n].Terminal` plus `deps.Batches`' card lists already carry exactly the information needed.

### F8 — `resolvePass` silently swallows a post-rewrite plan re-parse failure (MEDIUM, CONFIRMED-by-trace)

`internal/planglyph/planglyph.go:87-90`.

```go
current := plan
if reloaded, rerr := planparser.ParsePlan(plan.Dir); rerr == nil {
    current = reloaded
}
```

`rerr` is discarded. If `CanonicalizeHandles`' own `RewriteRefs` produced a plan directory that no longer parses, the three resolve-backed passes below silently run against the **stale in-memory pre-rewrite** plan and the gate reports a clean-looking verdict over bytes that are no longer on disk.
That is precisely the "a plan looks validated and was not" failure mode `repo.go:28-31` says was rejected by design.
Fix: return the error, wrapped, rather than degrading.

### F9 — `renameDeclSource` derives the new declaration with a first-occurrence string replace (MEDIUM, PLAUSIBLE)

`internal/planglyph/handle.go:67`: `decl := strings.Replace(sym.Signature, sym.Glyph.Name, member, 1)`.

`strings.Replace(..., 1)` hits the *first substring* occurrence, not the declared identifier. For a method whose receiver type name contains the method name as a substring the first hit is in the receiver, so the wrong token is renamed — e.g. `func (c *Canonicalizer) Canonical() error` with `Glyph.Name == "Canonical"` yields `func (c *NewNameizer) Canonical() error`. Same for `func (r *Resolve) Resolve()`.
The derived `Decl` then goes to `quarry.Name`, so the canonical glyph computed for the handle is wrong — and it is wrong *silently*, because `Name` will happily name whatever declaration it is handed.
Marked PLAUSIBLE pending a live check of what `Symbol.Signature` actually contains for a Go method.

### F10 — `resolveContainment` emits findings in nondeterministic order (LOW, CONFIRMED-by-trace)

`internal/planglyph/containment.go:47`: `for target, cards := range byTarget` — a Go map range, so `members` and `selves` are built in random order and the resulting `containment-file-overlap` findings come out in a different order on each run whenever more than one overlap exists.
Every sibling in this package sorts deliberately (`sortedCards`, `sort.Strings(canonicals)`, `sort.Strings(targets)`); this one does not.
Consequence: a gate's rendered findings list is not reproducible, so two runs over an identical plan produce different operator output and any test asserting more than one overlap is inherently flaky.

### F11 — `ScopeGuard` reports every handle-created symbol as out-of-plan (LOW, CONFIRMED-by-trace)

`internal/planglyph/scope.go:18-26` builds the comparison union from `Card.Targets` verbatim.
A `Create` card declaring `plan:internal/foo#NewThing` has exactly that handle-shaped string in `Targets` (`internal/planparser/parse.go:680`), while `delta.Created[i].ID` is the bare glyph `internal/foo#NewThing`.
`union[id]` therefore misses, and `ScopeGuard` emits an informational `scope-outside-plan` finding for **every symbol the plan explicitly asked to be created** — the exact opposite of the check's purpose.
`RecordBatch` runs `ScopeGuard` on `batch.Cards`, which are parsed at CLI entry and so still carry the handle even after `BindHandles`' on-disk rewrite (recordbatch.go:243).
Fix: normalise the union through the same handle-stripping `donecheck.go:34`'s `resolveKeyFor` performs.

### F12 — `planglyph`'s entry points nil-deref on a nil plan, and the glyph landing left a smoke fixture panicking (MEDIUM, CONFIRMED-by-trace)

`internal/planglyph/planglyph.go:115-116`: `resolveLanguage` dereferences `plan.Language` with no nil guard, and `DoneChecks`, `CanonicalizeHandles`, `BindHandles`, `DetectDrift`, `ValidateFormat` and `Validate` all call it first.

`internal/webstercli/smoke_test.go:333-344` constructs `websterengine.RecordDeps` with **no `Plan` field** — nil — and calls `RecordBatch`, which reaches `planglyph.DoneChecks(deps.Plan, …)` at `recordbatch.go:195`. That is an unconditional nil-pointer panic.
The fixture predates PR #230 and the `Plan` field it added to `RecordDeps`; nothing updated it, and the smoke tier is not part of any routine gate, so it went unnoticed.
`internal/websterengine/recordbatch_test.go`'s only `RecordDeps` constructor does pass a plan, which is why the hermetic tier stays green.

Strictly `webstercli`'s test is outside loom's module scope, but the defect is a direct consequence of the surface under review and the nil-guard gap is in `planglyph` itself, so both halves are recorded here and fixed this round.

### F13 — the plan spec's worked example teaches an illegal `Rename` pair (MEDIUM, CONFIRMED)

`contracts/specs/loom-plan-spec.md`'s worked example, card 5:

```
**Rename:**
- `internal/boardengine#MapRow` -> `internal/boardengine#MapRowJSON`
```

The `New` side is a glyph. The same document's own line 218 and its check 16 (`rename-to-not-handle`) require it to be a `plan:` handle, and the golden fixture the example claims to be "byte-consistent with" — `internal/planparser/testdata/goodplan/05-rowmapper-rename.md` — correctly writes `` `internal/boardengine#MapRow` -> `plan:internal/boardengine#MapRowJSON` ``.
So the spec's example is the one artifact in the repo teaching the illegal form, and it is the document `Plan-Review`'s rubric and the plan-format contract both point at.
(`contracts/stencils/loom/loom-template-plan.md:103`, which `Plan-Write` actually reads, states the rule correctly — so this is doc-only, not a live planner hazard.)

### F14 — `create-new-unit` never fires for a handle-declared `Create` (MEDIUM, CONFIRMED LIVE)

`internal/planglyph/create.go:65-68`: `createFindings` looks each Create ref up in the resolve index and `continue`s when absent — "a `plan:` handle target is never looked up here at all".

But a handle is the shape the plan format prescribes for creating something genuinely new (`loom-template-plan.md:90-101`), so in exactly the case the check exists for, it is silently inert.
The design doc's stated purpose — "a misspelled unit cannot silently create a package nobody intended" (`quarry-glyph-plan-alphabet.md:37`) — is therefore unmet for every handle-declared Create.

Confirmed live: card 1 of the probe plan declares `plan:internal/gamma#NewThing` in a package that does not exist on disk, and `lyx webster validate` reported **no** `create-new-unit` finding (transcript in "What was tested" below).
Fix: resolve a handle's canonicalized expected glyph (`strings.TrimPrefix(handle, HandlePrefix)`, the normalisation `donecheck.go:34` already performs) so the inversion applies to handles too.

### F15 — the `Plan-Write` stencil's own `Create` declaration-head example does not parse, and blocks the plan (BLOCKING, CONFIRMED LIVE)

`contracts/stencils/loom/loom-template-plan.md:98` is the one worked example `Plan-Write` reads for the declaration grammar:

```
**Create:**
- `plan:internal/boardcli#RowJSON` -> `type RowJSON struct{...}`
```

`quarry.Name` parses the declaration head as real Go source (`package q\n\n<Decl>\n`, one retry appending `" {}"`). A literal `struct{...}` is not Go: `...` is not a valid struct body. `Name` returns `error: "declaration does not parse"`, `reason: "parse"`, and `CanonicalizeHandles` turns that into a **blocking** `handle-name-failed` (`internal/planglyph/handle.go:137-143`).

Confirmed live against the probe plan — verbatim envelope:

```json
{"card":"","check":"handle-name-failed",
 "detail":"handle \"plan:internal/alpha#AddedHere\" failed naming: declaration does not parse (parse)",
 "severity":"blocking"}
```

A planner copying the stencil's own example therefore produces a plan that `Plan-Validate` blocks — and `Plan-Validate`'s `Stuck` carries an **empty pointer** with the findings only written to the driver log (`internal/loomshed/planvalidate.go:133-134`), so `Plan-Write` is respawned with no idea what was wrong and will reproduce the same shape until the bounce budget escalates to a human.
Two fixes are needed: correct the stencil's example to a head that parses (`type RowJSON struct`), and state the constraint explicitly — the declaration head must be a single, parseable Go declaration head.

Sub-finding (LOW): the `handle-name-failed` finding carries an empty `Card` (`handle.go:130-143`, both branches), unlike every other finding in the package, so the operator is not told which card to look at.

### F16 — standalone `lyx webster run` cannot start Master at all (BLOCKING, CONFIRMED LIVE — out of loom's scope to fix)

```
lyx webster run --plan-dir <plan>          # cwd = a plain git checkout, standalone mode
{"error":"webster: start master: shuttle: NewRunner was told an anchor path
 \"/home/knatte/.local/state/lyx/267a789e\" outside its worktree root
 \"<repo>\": the anchor is always the worktree root or a subdirectory of it,
 so this pair is most likely swapped — …","ok":false}
```

Standalone mode derives its state directory under `$XDG_STATE_HOME/lyx/<hash>` (`internal/standalonestate.Derive`) while `--target-dir` names a wholly separate repository, so `standalonegeom.WebsterGeometry`'s `AnchorRoot` is by construction outside `WorktreeRoot` — and `shuttleengine.NewRunner`'s containment assertion refuses exactly that pair.
The mode is therefore dead on its documented entry point, the one `lyx webster --help` gives as its own example (`internal/webstercli/cli.go:186-187`).
`begin-batch`, `record-batch` and `validate` all work standalone; only `run` (the Master spawn) does not.

This is `webster`/`standalonegeom`'s own bug, on a path loom never takes (loom always runs hub mode), so per "Explicitly OUT of scope" it is recorded rather than fixed here — but it is a real, shipped, blocking defect and it is what forced run 4 to drive the bracket verbs directly rather than through `run`.

### F17 — a `Create` group's ref list is emptied by binding, not only malformed (BLOCKING, CONFIRMED LIVE)

Observed alongside F3 in run 4 step 6: besides `handle-malformed`, `begin-batch 2` also reported
`card-field-empty/1-greet-helper[blocking]: card 1's **Create:** label carries no targets`.

`parseCreateField` (`internal/planparser/parse.go:678-685`) appends to `refs` only on the handle branch and the plain-ref branch; the arrow-but-not-a-handle branch appends to `raw` and to nothing else. A card whose only Create sub-bullet took that branch therefore parses with an **empty** `Refs`, which `checkCardFieldEmpty` reports blocking in its own right.
It shares F3's root cause and F3's fix, but it is a distinct check ID and a distinct message the operator sees, so it is recorded separately.

## Scope assessment

_(written last)_
