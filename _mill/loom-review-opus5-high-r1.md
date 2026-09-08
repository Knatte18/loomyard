# `loom` crucible review — glyph-hardening campaign, ROUND 1 (opus5-high)

> Independent review per `_mill/loom-review-prompt.md`. Clean-room: findings below were formed without reading any prior review file in this worktree.
> Status: REVIEW COMPLETE. Job 1 closed before any production or test file was touched; fixes follow in the fixer report.

## Executive summary

**Merge readiness: NOT MERGE-READY as landed.** The glyph plan alphabet is individually sound — every mechanism I drove in isolation behaves as its design doc says — but the *integration* into loom's phase machine, which nothing had ever driven live before this round, is broken in a way that stops the pipeline dead.

**The headline: no multi-batch plan containing a `Create`, `Delete` or `Rename` card can complete through Webster.** Driven live, a two-card plan whose first card creates a symbol through a `plan:` handle — the exact shape `contracts/specs/loom-plan-spec.md` prescribes — reaches `begin-batch 2` and is refused, twice over, with no operator recourse but hand-editing the plan:

1. Webster's own `BindHandles` rewrite trips webster's own plan-fingerprint staleness guard (**F4**). Its advised recourse, `lyx webster run --fresh`, restarts the same plan into the same wall.
2. Past that, the same rewrite has stripped the `plan:` prefix out of the declaring card's *declaration* bullet, so the plan no longer parses as one — blocking `handle-malformed` plus `card-field-empty` (**F3**, **F17**).

Beneath those sits one structural gap with three separate manifestations (**F5/F6/F7**): the dispatch-boundary re-resolution validates the **whole** plan against the **post-change** tree on every batch, so any card whose work already landed necessarily contradicts reality — a completed `Delete` target reads `glyph-not-found`, a completed `Rename`'s `Old` side reads `glyph-not-found` *and* `rename-old-unresolved`, a completed bare-glyph `Create` reads `create-already-exists`.

Three more independent blockers:

- **F2** — a declared `Rename` card's own expected outcome is misclassified as drift, because gate one compares a `plan:`-prefixed handle against a bare glyph and can never match for any plan that passes its own validator. Live, this clobbered the card's `Old` side and wrote a false `Tier: exact` amendment.
- **F9** — `renameDeclSource` derives the new declaration with a first-*substring* replace, so `func (c *Counter) Count() int` becomes `func (c *Counter.Tallyer) Count() int`. **No method can be renamed through the glyph alphabet at all.**
- **F1** + **F18** — the evidence tier is dead twice over: its `informational` finding is folded into a blocking error by `RecordBatch`, and even fixed, every candidate is unconditionally accompanied by a blocking `plan-references-deleted-symbol`.
- **F15** — the declaration-head example in the stencil `Plan-Write` actually reads (`type RowJSON struct{...}`) does not parse as Go, so `quarry.Name` rejects it and blocks the plan — and `Plan-Validate`'s `Stuck` carries an empty pointer, so the respawned planner is never told why.

**What works, proven live:** handle canonicalization including the plan-wide rewrite to non-declaring cards; binding of *referencing* cards; the Create inversion's blocking half and its new-unit branch; both containment tiers; exact-tier drift auto-repair; the plain-blocking deleted-and-referenced case; the `language: "none"` opt-out; and the infrastructure-error disposition (`ErrQuarryUnavailable` surfaces as "quarry could not answer", never as a plan finding — the specific disaster the design names is genuinely prevented).

**Counts:** 13 BLOCKING, 5 MEDIUM, 3 LOW, 1 NIT — 22 findings.
(F21 and F22 were found while fixing, and are recorded in full below alongside the rest.)
All four mission scenarios were driven live to completion; see the per-scenario table in "What was tested".

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

**Run 5 — a declared `Rename` card, driven through `record-batch` (scenario 2).**

Plan: card 1 `**Rename:** internal/alpha#ToRename -> plan:internal/alpha#RenamedThing` (with the required `## Rename mechanic` section); card 2 `**Edit:** internal/beta#BetaOne`, `**Uses:** plan:internal/alpha#RenamedThing`. Validated clean.
`begin-batch 1` ok. The rename was then performed for real as a pure identifier rename (AST-exact), committed as `ad1c8e928`, report written, `record-batch 1` run.

`record-batch 1` returned **ok/done**, and left behind:

- card 1 rewritten to ``- `internal/alpha#RenamedThing` -> `plan:internal/alpha#RenamedThing` `` — its `Old` side **clobbered**, leaving a self-referential pair that no longer records what the card renames;
- `amendments.md` created with, verbatim:
  `- Timestamp: 2026-09-06T16:24:48Z, Card: 1-rename-torename, OldGlyph: internal/alpha#ToRename, NewGlyph: internal/alpha#RenamedThing, Tier: exact, SHA: ad1c8e928…`
  — a "repair" amendment for a rename that was the card's own declared, expected outcome.

→ **F2 confirmed live.** Gate one never fired; the plan's own working-as-designed rename was recorded as drift and auto-"repaired".
`begin-batch 2` then failed with `ErrFingerprintMismatch` again — **F4 confirmed a second time, with no `Create` card anywhere in the plan**, driven purely by the drift rewrite and the new `amendments.md`.

**Run 6a — undeclared exact-tier drift (scenario 3, first sub-scenario). WORKS.**

Plan: card 1 `**Edit:** internal/gamma#Greet`; card 2 `**Edit:** internal/alpha#ToRename`. Batch 1's commit edited gamma **and**, out of band, renamed `ToRename` → `DriftedName` as a pure identifier rename.

`record-batch 1` → **ok/done**, and:

- card 2's target auto-rewrote to `internal/alpha#DriftedName`, with no human intervention;
- exactly one amendment appended: `Card: 2-edit-torename, OldGlyph: internal/alpha#ToRename, NewGlyph: internal/alpha#DriftedName, Tier: exact, SHA: e413fd02e…`.

**Exact-tier auto-repair works live, end to end, exactly as designed** — gate one correctly did not match (no `Rename` card pairs it), gate two correctly did.

**Run 6b — evidence-tier drift (scenario 3, second sub-scenario). BROKEN, two ways.**

Same plan; batch 1 renamed `DriftedName` → `Wobbled` *and* changed its body enough that quarry's token streams differ in length, so quarry classified it a `RenameCandidate` rather than an exact `Renamed` (observed `body_token_similarity=0.4000`, `body_tokens_before=4`, `body_tokens_after=10`).

`record-batch 1` → **refused**, verbatim:

```
webster: record-batch's done-checks reported a blocking finding:
plan-references-deleted-symbol/2-edit-torename[blocking]: card 2 references
"internal/alpha#DriftedName", which the delta reports deleted with no corresponding rename;
rename-candidate/2-edit-torename[informational]: card 2 references "internal/alpha#DriftedName",
deleted with 1 evidence-tier rename candidate(s) — mechanical evidence only, the
rename-versus-genuine-delete decision is the reviewer's, never the pipeline's:
internal/alpha#Wobbled (file=internal/alpha/alpha.go, signature_identical_modulo_name=true,
body_token_similarity=0.4000, body_tokens_before=4, body_tokens_after=10, doc_identical=false)
```

→ **F1 confirmed live**: the `[informational]` finding is folded into the blocking error and fails the batch.
→ and **F18** (new, below): the *same* symbol simultaneously produced the blocking `plan-references-deleted-symbol`, whose text ("deleted with no corresponding rename") directly contradicts the candidate finding printed beside it.

**Run 6c — a genuine delete, still referenced (scenario 3, third sub-scenario). WORKS.**

Batch 1 removed `internal/alpha#Wobbled` outright with no successor. `record-batch 1` → refused with exactly one finding:
`plan-references-deleted-symbol/2-edit-torename[blocking]: card 2 references "internal/alpha#Wobbled", which the delta reports deleted with no corresponding rename`
**The plain-blocking case behaves correctly and does block.**

### Per-scenario verdict — was it driven live to completion?

| # | Scenario | Driven live? | Outcome |
|---|---|---|---|
| 1 | `Create` card `plan:` handle → canonicalize → bind | **Yes** | Canonicalization ✓ (plan-wide, reaching non-declaring cards). Binding of *referencing* cards ✓. Binding of the *declaring* card corrupts it (F3/F17), and the run then wedges (F3, F4). |
| 2 | `Rename` to-side handle, exact-tier gate one | **Yes** | Broken: gate one never matches (F2). Method renames impossible at all (F9). |
| 3 | Drift: exact auto-repair / evidence tier / plain delete | **Yes, all three** | Exact-tier auto-repair ✓. Plain-blocking delete ✓. Evidence tier broken (F1, F18). |
| 4 | Create inversion (3 ways) + both containment tiers | **Yes** | `create-already-exists` ✓, `create-new-unit` ✓ for a bare glyph but never for a handle (F14), `containment-unit-overlap` ✓, `containment-file-overlap` ✓. |

On scenario 4's "prevents blind parallel dispatch": **not provable as a prevented race.** `internal/websterengine`'s batch loop is strictly sequential (`manifest/designs/webster-parallel-execution.md`), so no two cards can actually race today. What is proved is that both containment findings fire correctly and block the plan. Stated explicitly rather than overclaimed.

### What could NOT be verified, and why

- **A full `lyx loom run` through the whole seventeen-row recipe with real LLM sessions.** Three compounding environment gaps, not a cost objection:
  1. Standalone `lyx webster run` cannot start Master at all on this host (F16), so the no-hub route is closed.
  2. There is no lyx hub on this machine whose warp repository contains Go source — the only hub present (`/home/knatte/Code/lyx-test-HUB`) has no Go files, so quarry can resolve no glyph in it, and a glyph scenario cannot exist there.
  3. The `lyx` on `PATH` (`/home/knatte/go/bin/lyx`, dated 2026-08-28) **predates the glyph landing entirely.** Every agent loom spawns resolves `lyx` from its own shell PATH (`manifest/designs/loom.md`'s "Agent execution" hazard), so a hub run would have its agents call a pre-glyph binary — and would additionally have that binary rewrite and commit the hub's stencils. A hub run under this condition would produce misleading evidence, not better evidence.
  Mitigation actually taken: the Plan-Validate/Plan-Revalidate rows and the `lyx webster validate` verb call the **identical** `planglyph` functions by the Gate Self-Check Parity Invariant (`CONSTRAINTS.md`), and `begin-batch`/`record-batch` are the same processes Master itself invokes — so every row of the glyph surface was driven by its own real code path.
- **The Claude fork-transcript audit** in runs 4–6 was seeded as a fixture rather than produced by a real Master session. It is upstream of and orthogonal to every glyph mechanism under review; the plan, git history, quarry resolve/delta, and both bracket verbs were all real.

### Post-fix live re-verification (runs 7–10, on a fresh fixture)

Every scenario was re-driven after the fixes, on a rebuilt fixture repository (`<scratch>/sbx2`, base `48ff7a69f`) against a freshly deployed binary. Teardown at the end removed the fixture repos, the three derived standalone state directories, and the seeded transcript fixtures.

**Run 7 — scenario 4, all five branches in one plan.** `lyx webster validate` reported exactly:

| severity | check | card |
|---|---|---|
| informational | `create-new-unit` | `1-new-unit-handle` — **now fires for a handle** (F14) |
| — (no finding) | handle Create in an existing unit | `2-existing-unit-handle` |
| blocking | `create-already-exists` | `3-already-exists-handle` — **now fires for a handle** (F14) |
| blocking | `containment-file-overlap` | `4-member-edit` |
| blocking | `containment-unit-overlap` | `4-member-edit` |

**Run 8 — scenarios 1 and 2, a three-batch plan driven to completion.** Card 1 creates through a `plan:` handle, card 2 is a declared `Rename` whose to-side is a handle, card 3 uses both.

- `begin-batch 1` ok, carrying the informational `create-new-unit` as an **advisory** rather than a refusal.
- `record-batch 1` ok/done. Card 1's declaration collapsed to `` - `internal/gamma#Greet` ``; card 2's `Uses` bound to the same real glyph; **no `scope-outside-plan` false positive** for the created symbol (F11).
- `begin-batch 2` ok — the wall that used to be `ErrFingerprintMismatch` then `handle-malformed`/`card-field-empty` (F3, F4, F17).
- `record-batch 2` ok/done, and **card 2's `Old` side survived intact** with **no amendment file created** — gate one recognised the card's own declared rename (F2).
- `begin-batch 3` ok, `record-batch 3` ok/done. `lyx webster status` reports all three batches `terminal: true, status: done`, and the fixture's `go build ./...` passes.

**This is the sequence that had never completed before.**

**Run 9 — scenario 3, all three sub-cases, sequentially on one fixture.**

- **3a exact-tier auto-repair:** an out-of-band pure identifier rename of a symbol a pending card references → `record-batch` ok/done, the pending card auto-rewritten to `internal/alpha#Drifted`, exactly one `Tier: exact` amendment appended.
- **3b evidence tier:** the same symbol renamed *and* rewritten so quarry classified it a candidate → `record-batch` **ok/done** (was `ErrCardNotDone`), the blocking `plan-references-deleted-symbol` **gone** (F18), and the informational finding surfaced verbatim in `RecordResult.Warnings` (F1):
  `rename-candidate/2-edit-renamed[informational]: card 2 references "internal/alpha#Drifted", deleted with 1 evidence-tier rename candidate(s) — mechanical evidence only, the rename-versus-genuine-delete decision is the reviewer's, never the pipeline's: …`
  The pending card was **not** rewritten and **no** amendment was appended, exactly as the tier's contract requires.
- **3c plain blocking delete:** the symbol removed outright with no successor → `record-batch` refused with exactly one finding, `plan-references-deleted-symbol`. **The blocking case is not weakened.**

Where the informational findings surface: `RecordResult.Warnings` → `internal/webstercli`'s `record-batch` JSON envelope → Master's own session, which is the reader of every bracket verb's output, and thence its summary. `BeginBatch`'s equivalents ride out on `BeginResult.Advisories` into the same envelope. Traced end to end and observed in the live envelopes above.

**Run 10 — the method rename (F9).** The plan that previously failed `handle-name-failed: … member_too_deep (Counter.Tallyer.Count)` now validates `{"cards":2,"ok":true,"valid":true}`. Every declaration-head shape newly documented in the stencil (`type X struct`, `func f(...) T`, `type R interface`, `func (c *Cache) Get(...) (...)`) was separately verified live to canonicalize with no finding.

### Gates after the fixes

| Command | Result |
|---|---|
| `go build ./...` | clean |
| `go vet <the 11 in-scope package sets>` | clean |
| `go test -count=5 <the 11 sets> ./cmd/lyx/...` | 12 packages `ok`, 0 FAIL |
| `go test ./...` (whole repo) | 0 FAIL |
| `go test -tags integration ./internal/planglyph/... ./internal/planparser/... ./internal/websterengine/... ./internal/loomcli/... ./internal/loomshed/...` | all `ok` |
| `go test -tags smoke ./internal/loomcli/... -run TestSmokeBootstrap_BringsUpSessionStrandAndDriver -count=1` | PASS (1.6s) |
| `go test -tags smoke ./internal/loomcli/... -run TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed -count=1` | PASS (62.0s) |
| `go test -tags smoke ./internal/loomcli/... -run TestSmokeBootstrap_CleanlinessOrderingAfterSeedCommit -count=1` | PASS (0.2s) |
| `go test -tags smoke ./internal/loomcli/... -run TestSmokeBurlerRound_AttachesToALiveRoundInsteadOfRespawning -count=1` | PASS (3.9s) |

Each smoke test was named exactly and run one at a time; no bare `-run Smoke` was ever issued, and the banned `internal/burlerengine/smoke_cluster_test.go` tests were not run.

### Teardown

`tmux ls` shows only the operator's two pre-existing sessions (`0`, created 2026-09-02; `10`, created 16:30, both predating this round's first check). No `lyx` process, no loom driver, no orphaned `claude` process with a cwd under the fixture. The three derived standalone state directories, both fixture repositories, and both seeded transcript fixtures were removed; `/home/knatte/.local/state/lyx/11cb0661` remains and predates this round (2026-08-18). The worktree is clean.

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

### F18 — an evidence-tier rename candidate always collides with a blocking deleted-symbol finding (BLOCKING, CONFIRMED LIVE)

`internal/planglyph/drift.go:98-111` iterates `delta.Deleted` and reports blocking `plan-references-deleted-symbol` for every still-referenced entry, with no exclusion for entries that also carry rename candidates.

The file's own contract argues this "never collides with the exact-tier handling above: quarry's own delta engine removes an exact pair's constituents from Deleted entirely" (`drift.go:66-69`). That reasoning is correct for the **exact** tier and false for the **evidence** tier: quarry deliberately leaves an evidence-tier candidate's endpoints in `Created`/`Deleted`, because "suppressing either for a candidate quarry has not resolved would be a silent pick in disguise" (quarry's `internal/engine/delta.go`).

So every evidence-tier candidate is accompanied by a blocking finding, and the two contradict each other in the same message — confirmed live in run 6b:

```
plan-references-deleted-symbol/…[blocking]: card 2 references "internal/alpha#DriftedName",
  which the delta reports deleted with no corresponding rename;
rename-candidate/…[informational]: card 2 references "internal/alpha#DriftedName",
  deleted with 1 evidence-tier rename candidate(s) …
```

This is deeper than F1: even with `RecordBatch`'s severity filter fixed, the evidence tier could never be non-blocking, so the "review-surfaced, never auto-repaired" tier the design describes does not exist in practice.

Fix: exclude from the `plan-references-deleted-symbol` sweep any deleted symbol that appears as a `RenameCandidates` entry ID — that symbol's disposition is the candidate finding's, not the delete check's — and say so in the function's own contract comment.

### F21 — a `Delete` card's own successful deletion is reported as drift against itself (BLOCKING, CONFIRMED)

Found while fixing F5/F6/F7, and it is the same root cause one step further in.

`internal/websterengine/recordbatch.go` passed the **whole** plan to `DetectDrift`, including the cards of the batch currently being recorded. `DetectDrift`'s signal is `delta.Deleted` intersected with the plan's own references (`drift.go:98-111`), and the batch being recorded is exactly what the delta describes — so a `Delete` card whose batch has just removed its target still references that target, and the card's own success comes back as a blocking `plan-references-deleted-symbol` against the very card that asked for it.

**No `Delete` card could ever be recorded at all.** The function's own contract already says the right thing — "the delta's deleted symbols intersected with the *remaining* plan's own references" — but nothing implemented "remaining".
Reproduced by `TestRecordBatch_DeleteCardDeletingItsOwnTargetIsNotDrift`, which fails on the unfixed code.

### F22 — standalone mode writes trace logs into the target repository, unexcluded (LOW, CONFIRMED LIVE — out of loom's scope to fix)

Every standalone `lyx` invocation writes `<target>/.lyx/logs/trace-*.log` into the repository named by `--target-dir`. In hub mode `.lyx` is held out of git by `fabricengine`'s `.git/info/exclude` seeding; standalone mode has no fabric and seeds no exclude, so the files show as untracked in the operator's own repository — and `RecordBatch`'s dirty-worktree probe reports "worktree is dirty after batch N's own commits" on every single call because of them.

Observed throughout runs 4–6, and a plain `git add -A` in the fixture committed eleven of them.
This is `standalonegeom`/`logger` scope on a path loom never takes (loom is always hub mode), so it is recorded rather than fixed here, alongside F16.

### F19 — `Plan-Validate`/`Plan-Revalidate` mutate the plan on disk, which no doc says (MEDIUM, CONFIRMED LIVE)

`manifest/designs/loom.md`'s producer table describes row 8's output as "pass/fail, also callable standalone as `lyx loom validate-plan`" and row 10's as "pass/fail — no artifact, a gate signal only".

Both rows in fact rewrite the plan directory in place: `planglyph.ValidateFormat`/`Validate` → `resolvePass` → `CanonicalizeHandles` → `planparser.RewriteRefs` (`internal/planglyph/planglyph.go:82`). Confirmed live — probe run 1 advanced the mtime of every handle-bearing card file, and run 2 rewrote a draft handle's spelling plan-wide.
The same is true of the standalone verbs `lyx loom validate-plan` and `lyx webster validate`, whose help text ("lint the plan without running anything", "checks it… reports the result") gives no hint that they write.

A validator that silently rewrites its subject is a genuine operability surprise, and "no artifact, a gate signal only" is now simply false for both rows. The mechanism is deliberate and correct; only the documentation is wrong.

### F20 — `handle-name-failed` carries no card attribution (NIT, CONFIRMED LIVE)

`internal/planglyph/handle.go:130-143` — both branches construct the finding with no `Card` field, so the rendered envelope reads `{"card":"", …}` while every other finding in the package names its card. Observed verbatim in probe runs 2 and 3.
`declSource` already carries only the handle; the card is available at the collection site (`handle.go:95-113`) and simply is not threaded through.

### Additional live confirmations (no finding)

- **`language: "none"` opts out correctly.** The same plan that reports a blocking `glyph-not-found` under `language: go` validates `{"cards":2,"ok":true,"valid":true}` under `language: none` — every glyph-aware check is a genuine no-op and no repository is opened.
- **The infrastructure-error disposition holds.** Two induced quarry failures both surfaced as gate/infrastructure errors, never as plan findings:
  - unreadable unit directory (`chmod 000 internal/alpha`) → `webster: quarry could not answer validating plan: planglyph: quarry could not answer: resolve: engine: read .gitignore … permission denied`
  - absent worktree root → `webster: quarry could not answer … open "/nonexistent-abc" … no such file or directory`
  Neither degraded into a `not_found` that would, under the Create inversion, silently mark a Create card done — the specific disaster `quarry-glyph-plan-alphabet.md`'s "infrastructure-error disposition" section exists to prevent. `DoneChecks` propagates the same wrapped error to `RecordBatch`, which returns it rather than passing the batch (traced, `recordbatch.go:195-198`).
- **`glyph-not-found`'s unit branching is correct**: a missing member in an existing unit reported "unit exists but the member is missing", not the misspelled-unit wording.

## Scope assessment

**Design intent vs. shipped.** `manifest/designs/quarry-glyph-plan-alphabet.md` describes eight mechanisms. Measured against what loom's phase machine actually reaches:

| Mechanism | Shipped as designed? |
|---|---|
| Glyph/self-glyph/path alphabet, `language:` selector | Yes — verified live including the `none` opt-out |
| Package-ownership seam (`planparser` pure, `planglyph` owns `quarry.Repo`) | Yes — no check implemented twice; the parity pair holds |
| Handle lifecycle: draft → canonicalize | Yes — verified live, plan-wide |
| Handle lifecycle: → bind | **Partially** — binds referencing cards correctly, corrupts the declaring card (F3/F17) |
| Resolve status policy | Yes — verified live, correct unit branching |
| Create inversion | **Partially** — blocking half and new-unit branch verified live, but `create-new-unit` is inert for the handle shape the format prescribes (F14) |
| Both containment tiers | Yes — both verified live |
| Infrastructure-error disposition | Yes — verified live in two ways |
| Drift detection (later batch, same task) | **No** — gate one unreachable (F2), evidence tier unreachable (F1, F18); only the exact-tier undeclared-drift path and the plain-delete path work |

**Deferred-that-should-be-fixed.** Nothing was deferred by the landing task that should have shipped. The gap is not omission but *un-exercised integration*: every defect above lives at the seam between `planglyph` and `websterengine`, and the landing task's own suite tests the two sides separately. `internal/planglyph/drift_integration_test.go:68` is the sharpest illustration — it pins gate one against a `Rename` pair shape (`sub#Old -> sub#New`) that the plan format itself rejects, so the test passes while the production path cannot.

**Shipped-beyond-scope.** None found. `quarrycli`, `donecheck.go`, `drift.go` and the `lyx quarry` verb group are named in the design doc's own "Deliberately out of scope" as later batches of the same task, so their presence is scope-tracked, not scope-creep.

**Docs accuracy.** `manifest/designs/loom.md` does not describe the glyph surface at all: its rows 8/10 still read "pass/fail — no artifact, a gate signal only" for producers that now rewrite the plan directory in place (F19), and its "Plan-Validate detail" section names the `planglyph` pair without mentioning canonicalization. `contracts/specs/loom-plan-spec.md`'s worked example teaches an illegal `Rename` shape (F13). The `Plan-Write` stencil teaches an unparseable declaration head (F15). The design doc `quarry-glyph-plan-alphabet.md` itself is accurate throughout — it describes the intended behaviour correctly; the code is what diverges.

**Operability.** Two real gaps beyond the findings: `Plan-Validate`'s `Stuck` carries an empty pointer, so a planner respawned after a `handle-name-failed` is told nothing (this is a pre-existing, documented residual, but F15 makes it bite in a newly common case); and `ErrFingerprintMismatch`'s advised recourse (`--fresh`) is, under F4, a loop rather than a repair.
