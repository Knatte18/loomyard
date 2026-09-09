# loom review — round 2 safety pass (fable5-high-r2)

Clean-room review of the two refactors (ref-shape registry centralization in `internal/planparser/shape.go`; quarry v0.2.0 `Status.Known()`/`ResolveResult.Rejected()` adoption in `internal/planglyph`), driven live through the real built `cmd/lyx` binary.
Round context: round 1 (`opus5-high-r1`) closed F1–F5 and drove eleven live scenarios; this round is a genuinely independent pass to find anything both it and the orchestrator's verification missed.

## Executive summary

(written last)

## Scope assessment

(written last)

## Code findings

(provisional entries appended as formed)

### F-R2-1 (provisional) — `.Status` tripwire does not match the `Unit` selector, the third Status-typed reading surface

- File: `internal/planglyph/status_enforcement_test.go:58` (`statusVocabularySelectors = {Status, Known, Rejected}`).
- Scenario: `quarry.ResolveResult.Unit` is a `Status`-typed field drawing from the same vocabulary (quarry's contract: set only on `not_found`, carrying `found`/`not_found`). A NEW planglyph consumer branching on `r.Unit` alone — e.g. `if r.Unit == quarry.StatusFound { treat member as merely missing } else { treat unit as gone }` — names no `Status`, `Known`, or `Rejected` selector anywhere and therefore ships invisible to the tripwire, exactly the blind-spot class round 1's F2 closed for `Rejected()`. The two existing `Unit` readers (`create.go:174` inside `createFindings`, `resolve.go:99` inside `statusFindings`) are both already inside allowlisted functions, so adding `"Unit"` to the selector set costs zero allowlist churn and closes the last unmatched spelling of the vocabulary.
- Severity: NIT (no current fail-open consumer exists; this is enforcement-machinery completeness, not a behavior defect).
- Suggested fix: add `"Unit": true` to `statusVocabularySelectors`, extend the file's doc comment, and add a seeded self-test for the Unit spelling mirroring `TestStatusHitsIn_CatchesRejectedConsumer`.
- CONFIRMED (traced: grep over planglyph production files shows `.Unit` read in exactly the two allowlisted functions; the tripwire's matcher provably cannot see a bare-Unit consumer since `statusHitsIn` matches only the three named selectors).

## Docs & operability findings

(provisional entries appended as formed)

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
