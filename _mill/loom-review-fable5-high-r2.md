# loom review — round 2 safety pass (fable5-high-r2)

Clean-room review of the two refactors (ref-shape registry centralization in `internal/planparser/shape.go`; quarry v0.2.0 `Status.Known()`/`ResolveResult.Rejected()` adoption in `internal/planglyph`), driven live through the real built `cmd/lyx` binary.
Round context: round 1 (`opus5-high-r1`) closed F1–F5 and drove eleven live scenarios; this round is a genuinely independent pass to find anything both it and the orchestrator's verification missed.

## Executive summary

Round 2 safety pass, clean-room. I read both refactors end to end (the `shape.go` kind-policy registry with its four meta-tests and two AST scans; every `Status`/`Known()`/`Rejected()` call site against quarry v0.2.0's actual source contract), ran all hermetic gates (build/vet/`-count=5` — all green), and drove SIXTEEN live scenarios (L1–L16) through the real built `cmd/lyx` binary: `lyx webster validate`/`begin-batch`/`record-batch` in real standalone geometry with real git history and real seeded session transcripts, and `lyx loom validate-plan` in a real fabric hub built with `lyx fabric clone`/`add`.

**No behavior regression found in either refactor.** Every migrated gate dispatched correctly live; both containment tiers, both Create-inversion directions, ambiguous handling, the rename exact-tier auto-bind (prior regression NOT reopened), deliberate drift blocking, undeclared-exact-rename auto-repair with exactly one amendment, evidence-tier candidates staying informational, both quarry-outage dispositions, and the `root:` normalization gate all match `quarry-glyph-plan-alphabet.md`.

Findings: **0 BLOCKING, 0 MEDIUM, 1 LOW, 1 NIT, plus one verification-record correction.**

- Top item of note (not a code defect): round 1's claim that the pre-resolution-rejection branch is structurally unreachable live is WRONG — I produced it live (L4) through a Create group's malformed `plan:` handle, and the code behaved correctly (blocking `glyph-rejected`, accurate rejection detail). The branch is live-proven, which strengthens, not weakens, merge-readiness.
- F-R2-2 (LOW, operability): the ambiguous-candidate detail renders duplicate identical IDs with no per-candidate file — the typical Go ambiguity (same name declared twice in one unit) reads as "ambiguous among: X, X".
- F-R2-1 (NIT): the `.Status` tripwire does not match the `Unit` selector, the last unmatched Status-typed reading surface.

**Merge-readiness: READY.** No new defects in the two refactors; the two findings are enforcement/operability polish, fixed this round.

## Scope assessment

- **Ref-shape registry centralization** (`internal/planparser/shape.go` + `classify.go`/`handle.go`): shipped as specified. All thirteen gates carry complete four-kind policies; both sync meta-tests, the completeness meta-test, and both AST boundary scans are present and honest (seeded self-tests prove the matchers fire). No production dispatch site outside the declared files touches `refKind`/`classifyRef` or open-codes `plan:` surgery (scans green; grep concurs). Live driving exercised eleven of the thirteen gates' dispositions with no panic and no silently skipped ref; the fail-closed panic is unreachable today precisely because the meta-tests force completeness — the intended design.
- **quarry v0.2.0 Status-helper adoption** (`internal/planglyph`): shipped as specified. `Known()` (`answer.go:65`) and `Rejected()` (`answer.go:262`) mean exactly what the call sites assume; the six allowlisted consumers each fail closed; classification is IDENTICAL to the documented pre-refactor policy for all four statuses plus the rejection shape (verified by table tests `status_completeness_test.go` AND live: found/multipart pass, ambiguous blocks with candidates, not_found blocks with the Unit-branched detail, rejection blocks via `glyph-rejected` — L2–L4, L10–L13).
- Nothing shipped beyond scope; no plan-promised behavior missing. Docs (`quarry-glyph-plan-alphabet.md`, `planglyph/doc.go` check-ID list, CONSTRAINTS.md invariants) match the as-built code as read and as driven.

## Code findings

Final severity order: F-R2-2 (LOW), F-R2-1 (NIT). No BLOCKING or MEDIUM findings.

### F-R2-2 (LOW, operability) — ambiguous-candidate details render duplicate identical IDs with no per-candidate file

- Files: `internal/planglyph/create.go:163-171` (the `StatusAmbiguous` arm round 1's F3 added) and its sibling `internal/planglyph/resolve.go:84-95` (`glyph-ambiguous`).
- Scenario (CONFIRMED live, L2b/L3): the one Go ambiguity actually constructible — the same name declared in two files of one unit — yields candidates that share one glyph ID, so the operator-facing detail reads `already resolves ambiguous among existing declarations: internal/greet#Dup, internal/greet#Dup` / `ambiguous among candidates: internal/greet#Dup, internal/greet#Dup`. Two identical strings tell the operator nothing about WHERE the colliding declarations are ("why is it ambiguous with itself?"); `ResolveResult.Candidates[].File` carries exactly the missing half (Resolve fills File because its entries span files).
- Severity: LOW — dispositions are all correct; only the message under-informs, in the case that is the common one live.
- Suggested fix: one shared candidate renderer (ID plus `(file)` when File is non-empty) used by both arms, so the two cannot drift; extend the existing tests' expectations.
- CONFIRMED (reproduced live through the real binary, both arms).

### F-R2-1 (NIT) — `.Status` tripwire does not match the `Unit` selector, the third Status-typed reading surface

- File: `internal/planglyph/status_enforcement_test.go:58` (`statusVocabularySelectors = {Status, Known, Rejected}`).
- Scenario: `quarry.ResolveResult.Unit` is a `Status`-typed field drawing from the same vocabulary (quarry's contract: set only on `not_found`, carrying `found`/`not_found`). A NEW planglyph consumer branching on `r.Unit` alone — e.g. `if r.Unit == quarry.StatusFound { treat member as merely missing } else { treat unit as gone }` — names no `Status`, `Known`, or `Rejected` selector anywhere and therefore ships invisible to the tripwire, exactly the blind-spot class round 1's F2 closed for `Rejected()`. The two existing `Unit` readers (`create.go:174` inside `createFindings`, `resolve.go:99` inside `statusFindings`) are both already inside allowlisted functions, so adding `"Unit"` to the selector set costs zero allowlist churn and closes the last unmatched spelling of the vocabulary.
- Severity: NIT (no current fail-open consumer exists; this is enforcement-machinery completeness, not a behavior defect).
- Suggested fix: add `"Unit": true` to `statusVocabularySelectors`, extend the file's doc comment, and add a seeded self-test for the Unit spelling mirroring `TestStatusHitsIn_CatchesRejectedConsumer`.
- CONFIRMED (traced: grep over planglyph production files shows `.Unit` read in exactly the two allowlisted functions; the tripwire's matcher provably cannot see a bare-Unit consumer since `statusHitsIn` matches only the three named selectors).

## Docs & operability findings

### F-R2-3 (verification-record correction, no code/doc change) — round 1's "structurally unreachable" claim about the `Rejected()` branch is refuted

Live scenario L4 (below) produced the `unreadableStatusDetail` `Rejected()` branch through the real binary: a Create group ref `plan:nounit` (no `#`) is skipped by `CanonicalizeHandles` (no unit to derive), but `createHandleResults` still resolves its `resolveKeyFor` body (`"nounit"`), quarry rejects it pre-resolution (`no_separator`), and `createFindings`' fail-closed default arm renders the blocking `glyph-rejected` with the rejection detail. The same route exists for `DoneChecks` at record-batch (a malformed handle or bare-symbol Delete/Create ref resolves to an unparseable target). The CODE is correct and matches `quarry-glyph-plan-alphabet.md` ("A pre-resolution rejection ... is blocking (glyph-rejected)") — no production doc claims unreachability (grep confirms), so the only fix owed is this corrected record; round 1's report stays untouched as that round's own durable record.

### Docs accuracy

- `internal/planglyph/doc.go`'s check-ID enumeration matches every raiser found by reading and by live driving (all four `glyph-rejected` raisers named — round 1's F5 holds).
- `manifest/designs/quarry-glyph-plan-alphabet.md`'s Create-inversion paragraph (including the `ambiguous` row F3 added) matches live behavior exactly (L2a/L2b/L3).
- `CONSTRAINTS.md`'s Ref-Shape Registry Invariant names both syncs, the completeness meta-test, and the per-scan exempt sets — all exactly as implemented (F1 holds).

## What was tested

Observations appended immediately after each command/scenario returns.

### Live driving — standalone webster substrate (real built binary)

Binary: `go build -o .scratch/lyx ./cmd/lyx` (cgo, gcc present). Fixture: `.scratch/live-r2/repo` — a real git repo (hermetic HOME/XDG_STATE_HOME under `.scratch/live-r2/`) holding a Go module with `internal/greet` (Hello, Goodbye, Counter.Count, Shared, and a deliberate duplicate `Dup` declared in two files — quarry answers `ambiguous` for `internal/greet#Dup`, verified via `lyx quarry resolve`) and `internal/farewell` (Wave). Plan dir: `.scratch/live-r2/plan`, driven via `lyx webster validate --target-dir <repo> --plan-dir <plan>` (standalone mode; reaches `planglyph.ValidateDispatch` → `resolvePass` → `CanonicalizeHandles`/`statusFindings`/`createFindings`/`resolveContainment` through the real binary).

- **L1 — Create card, draft handle canonicalization reaching both cards.** Card 1 `**Create:** plan:internal/greet#praiseFn -> func Praise() string`, card 2 `**Uses:** plan:internal/greet#praiseFn` + `**Edit:** internal/farewell/bye.go`. Result: `{"cards":2,"ok":true,"scope":"whole-plan","valid":true}`, and on disk BOTH card files now spell `plan:internal/greet#Praise` (declaring card's arrow bullet and referencing card's Uses entry). Canonicalization + `RewriteRefs` + Create inversion pass (not_found/unit-found) all correct live. MATCHES SPEC.
- **L2a — Create inversion, existing target.** Card 1 declares `func Hello() string` (exists). Result: one blocking `create-already-exists` — `Create target "plan:internal/greet#Hello" already resolves found`. MATCHES SPEC.
- **L2b — Create inversion, ambiguous target (round 1 F3 regression check).** Card 1 declares `func Dup() string`. Result: blocking `create-already-exists` — `already resolves ambiguous among existing declarations: internal/greet#Dup, internal/greet#Dup` — candidates named, NOT the old "unrecognized resolve status" message. F3 holds. (Observed operability nit: both candidates render as the identical ID with no per-candidate file, see findings.)
- **L3 — ambiguous on a non-Create target + create-new-unit.** Card 2 `**Edit:** internal/greet#Dup`, card 1 creates into brand-new unit `internal/shiny`. Result: blocking `glyph-ambiguous` on card 2 (same duplicate-ID candidate rendering) and informational `create-new-unit` on card 1. MATCHES SPEC (`ambiguous` blocking with candidates; new-unit informational).
- **L5 — both containment tiers, three-way overlap.** Cards: member `internal/greet#Hello`, file self `internal/greet/greet.go#`, unit self `internal/greet#`. Result: all three expected pairings fired blocking — `containment-unit-overlap` (member vs unit-self), `containment-unit-overlap` (file-self vs unit-self, the fable-high-r10 F4 pairing), `containment-file-overlap` (member vs file-self, resolve-backed via `Symbols[].File`). MATCHES SPEC.
- **L6 — registry gates live over wrong shapes.** `shedrecipe.Lookup` → `bare-symbol-target`; `internal/greet` (slashed extensionless) → `directory-target` with self-glyph remedy text; Rename `internal/greet#Goodbye -> LICENSE` → `rename-to-not-handle` (note: `LICENSE` was canonicalized to `LICENSE#` by the parse-time slash-free rule first, so the finding names "a glyph" — still refused, refKindName naming what the side now is); Rename `internal/greet/greet.go# -> plan:internal/greet#Whole` → BOTH `rename-from-not-glyph` (F5's third arm) and the resolve pass's accurate `rename-old-unresolved` ("resolved found but names a file or unit"). All ledger-gated dispatches behaved; no panic anywhere. MATCHES SPEC.
- **L7 — two drafts canonicalizing to one glyph.** `plan:internal/greet#SameA`/`#SameB`, both declaring `func Same() string`, both referenced by card 3. Result: one blocking `handle-canonical-collision` naming both drafts and the shared canonical; on-disk plan bytes UNCHANGED (no arbitrary-canonical rewrite — the fable-high-r10 F6 guard holds live). MATCHES SPEC.
- **L8 — real quarry infrastructure error.** `chmod 000 internal/greet`, then validate: `{"error":"webster: quarry could not answer validating plan: planglyph: quarry could not answer: resolve: engine: read .gitignore ...: permission denied","ok":false}` — reported as the quarry-named infrastructure envelope, never as a plan verdict; after `chmod 755` the same plan validates clean. `ErrQuarryUnavailable` disposition correct live.
- **L4 — pre-resolution rejection reachability probe (round 1's L11 claim).** Card 1 `**Create:** plan:nounit -> func Nounit() string`. Result: pure `handle-malformed` on both cards PLUS blocking `glyph-rejected` — `Create target "plan:nounit" was rejected before resolution: error glyph: parse "nounit" as go: a glyph needs a "#"..., reason "no_separator"`. **Round 1's claim that the `Rejected()` branch is structurally unreachable in live operation does NOT hold**: `createHandleResults` resolves `resolveKeyFor("plan:nounit")` = `"nounit"`, which fails `glyph.Parse` inside quarry and comes back as a per-entry pre-resolution rejection, and `createFindings`' fail-closed default arm renders it via `unreadableStatusDetail`'s `Rejected()` branch. Behavior is CORRECT (blocking, accurate detail, beside the actionable pure finding) — the claim, not the code, was wrong. Recorded under docs/verification findings.

### Live driving — record-batch bracket (real built binary, standalone webster geometry)

Method: seeded `state.json` by hand in the derived standalone webster dir (fingerprint computed exactly as `websterengine.fingerprint` does — verified by begin-batch accepting it), a hermetic `HOME` with a seeded parent transcript (`.claude/projects/<encoded-workdir>/livesess.jsonl`) and one fork transcript (one assistant text line), `assertedModel: sonnet` matching the template's master role so no tmux inject fires. Real `lyx webster begin-batch 1` opened each bracket (recording the real start SHA), a real git commit performed each batch's work, a real report YAML carried the true head SHA, and `lyx webster record-batch 1` drove the full post-batch pass — DoneChecks, real `quarry.DeltaGit`, BindHandles, ScopeGuard, DetectDrift — through the real binary.

- **L9 — Rename card, exact-tier auto-bind (the `renameCardPairs` prior-regression check).** Plan: card 1 `**Rename:** internal/greet#Goodbye -> plan:internal/greet#Farewell`, card 2 Uses the handle. Real rename commit (identical signature/body modulo name). record-batch: `{"ok":true,"status":"done"}`; BOTH the Rename pair's New side and card 2's Uses were bound on disk to `internal/greet#Farewell`; NO amendments.md exists (gate one recognized the declared rename — not misclassified as drift); ScopeGuard correctly reported the out-of-scope `internal/farewell#Wave` edit informationally. The prior `renameCardPairs` regression is NOT reopened. MATCHES SPEC.
- **L10 — deliberate drift: referenced symbol deleted, no matching Rename pair.** Plan: card 1 edits bye.go (the batch), card 2 (pending) edits `internal/greet#Shared`. The batch's commit deletes `Shared` outright. record-batch: `{"card_not_done":true,"error":"...plan-references-deleted-symbol/2-edit-shared[blocking]: card 2 references \"internal/greet#Shared\", which the delta reports deleted with no corresponding rename","ok":false}`; plan bytes UNCHANGED, no amendments.md — the evidence/deleted sweep never auto-repaired. MATCHES SPEC (only the exact tier repairs).
- **L11 — undeclared exact rename auto-repairs.** Same plan shape, batch commit renames `Shared`→`Common` (AST-exact) with no Rename card. record-batch: `{"ok":true,"status":"done"}`; card 2's ref rewritten on disk to `internal/greet#Common`; exactly ONE amendment appended (`Card: 2-edit-shared, OldGlyph: internal/greet#Shared, NewGlyph: internal/greet#Common, Tier: exact, SHA: <head>`); ScopeGuard reported the rename informationally. Post-repair revalidation was reached (introduced glyph resolves found; no spurious findings). MATCHES SPEC.

- **L12 — Create card whose work never landed.** Same bracket method, batch commit touches only bye.go. record-batch: `{"card_not_done":true,"error":"...create-not-done/1-create-zing[blocking]: Create target \"plan:internal/greet#Zing\" still does not resolve","ok":false}`. MATCHES SPEC (DoneChecks block before BindHandles).
- **L13 — evidence-tier rename candidate.** Batch renames `Common`→`Blended` WITH a body change (not AST-exact). record-batch: `{"ok":true,"status":"done"}` with an informational `rename-candidate/2-edit-common` warning carrying the full candidate signals (`signature_identical_modulo_name=true, body_token_similarity=0.7143, ...`); the blocking deleted-symbol sweep correctly suppressed for the candidate-bearing symbol; NO rewrite, NO amendment. MATCHES SPEC (evidence tier never repairs; decision left to a reviewer).

### Live driving — real fabric hub, `lyx loom validate-plan` (real built binary)

Hub built with the real binary: two local bare remotes (`fixt.git` cloned from the fixture repo, empty `fixt-weft.git`), `lyx fabric clone <weft> <warp>` → `fixt-HUB` (full topology: `_board`, junctions, per-worktree module configs), `lyx fabric add r2task` → real warp+weft pair.

- **L14 — `lyx loom validate-plan` parity pair in the hub.** Plan seeded in the pair's `_lyx/plan` (draft handle `plan:internal/greet#praiseFn`, `approved: false`). `lyx loom validate-plan`: `{"ok":true,...}` AND both card files canonicalized on disk to `plan:internal/greet#Praise` (through the weft-linked `_lyx`). `lyx loom validate-plan --require-approved`: exactly one extra blocking finding, `plan-unapproved`. Gate Self-Check Parity holds live through the loom verb. MATCHES SPEC.
- **L15 — quarry outage through the loom verb.** `chmod 000` on the worktree's `internal/greet`: `{"error":"loom: quarry could not answer validating plan at <plan-dir>: planglyph: quarry could not answer: resolve: engine: ... permission denied","ok":false}` — the CLI-side quarry-named disposition, never "the plan is invalid". Restored cleanly.

- **L16 — `root:` normalization gate + malformed glyph.** Plan with `root: internal`; card entries `greet/greet.go` (root-joined, canonicalized, resolves — no finding), `shedrecipe.Lookup` (NOT root-joined — the finding's detail shows the verbatim symbol, so `gateNormalizePath` skipped it: the "single sharpest regression this migration can introduce" did not happen), `internal/greet##` (→ `glyph-malformed` with quarry's parse error). First run of L16 accidentally reused L13's stale `state.json`, and `webster validate` honestly reported `"scope":"pending"` and skipped the completed-by-state card — documented run-progress scoping, not a defect; rerun with clean state produced the two expected blocking findings. MATCHES SPEC.

### Smoke suite (tagged, zero real LLM subprocesses)

- `which tmux` → present (no skip masquerading as a pass). `go test -tags smoke ./internal/loomcli/... -run Smoke -v -count=1`: 9 PASS, 2 FAIL — exactly the two failures the round context names as pre-existing: `TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed` (Discussion-Write shuttle `run outcome timeout`; history did not advance) and `TestSmokeBootstrap_DiedDriverProceedsToHandoverAndLogsWhy` (driver log empty; wants the poisoned-status decode failure named). **Deferred-item re-evaluation:** both sit in loom's driver bootstrap / Discussion-Write shuttle path — neither touches `internal/planparser`/`internal/planglyph` nor any `Status`/ref-shape dispatch — so they are genuinely out of this campaign's scope and stay deferred.

### Teardown

- No tmux server was started by my live driving (all webster/loom verbs ran with `assertedModel` pre-matched, so no inject; no `lyx webster run`/`lyx loom run` was ever invoked). The one live `tmux` process on the host (pid 485544) predates this session by a week (started Sep 2, user's own sessions "0"/"10" on the default socket) and is not mine to kill; the smoke suite's own cleanup left no additional server. Scratch fixtures live under the gitignored `.scratch/live-r2/` and are removed at the end of Job 2.

### Not verified, and why

- Windows-specific path behavior — unreachable from this Linux host; named, never-executed gap.
- A real `quarry.Name` length/echo-mismatch and a batched-Resolve coverage breach — unproducible through a real quarry (its contract always holds); covered by the unit-seam tests (`ensureResolveCoverage`, `matchHandleResults`, `ensurePostRepairCoverage`), which is the only way those guards can be exercised.
- The full LLM-driven phase machine (real Discussion-Write/Burler/Plan-Write) — per the cost declaration, this campaign's mechanics never touch an LLM, and nothing I drove suggested otherwise; no felt need arose.

### Spec/contract reading notes

- quarry v0.2.0 `engine.Status.Known()` (module cache `internal/engine/answer.go:65`): true exactly for the four documented values `found`/`not_found`/`ambiguous`/`multipart`; false for `""` and any other string.
- `ResolveResult.Rejected()` (`answer.go:262`): `r.Status == ""` — the documented pre-resolution-rejection marker (Status and Error never both set).
- These match the call sites' assumptions: `donecheck.go:163` (`!r.Status.Known()` fail-closed before reading the two booleans), `resolve.go:139` (`r.Rejected()` selecting the rejection detail).
- Static call-site sweep (`grep` over `internal/planglyph` production files): every `Status` read is one of `donecheck.go` (Known-gated), `resolve.go`/`create.go`/`containment.go` (exhaustive switch with fail-closed default), `handle.go:94` (`!= StatusFound`, deliberately Found-only per its doc). The one `Rejected()` call is `unreadableStatusDetail`, reached only from the fail-closed arms. No hand-rolled `Status == ""` or `Error != ""` test survives in production code.
- Environment check: gcc, cc, tmux, go1.26.0 all present; no environment gap.

### Hermetic gates (pre-fix baseline)

- `go build ./...` — exit 0.
- `go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/...` — exit 0.
- `go test -count=5` over the eight in-scope packages (`loomengine, loomcli, loomshed, planparser, planglyph, websterengine, webstercli, cmd/lyx`) — all `ok`, exit 0.

### Registry/meta-test reading notes

- `shape_test.go` covers both syncs (enum↔allRefKinds, refGate consts↔ledger keys, values not names), ledger completeness, and the fail-closed panic (both missing-kind and missing-gate).
- `shape_enforcement_test.go` covers both AST scans with seeded self-tests; exempt sets match CONSTRAINTS.md's Ref-Shape Registry Invariant exactly (`classify.go`+`shape.go` for refKind; those plus `planparser/handle.go` for plan:-ops; no planglyph exemption).
