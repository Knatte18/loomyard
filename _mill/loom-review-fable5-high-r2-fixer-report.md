# loom fixer report — round 2 safety pass (fable5-high-r2)

Companion to `_mill/loom-review-fable5-high-r2.md`. Every recorded finding is closed; nothing deferred except what is explicitly recorded below with reasons.

## Implemented

### F-R2-2 (LOW) — ambiguous-candidate details locate each declaration by file — commit `ecec6c181`

- `internal/planglyph/resolve.go`: new shared renderer `candidateList` — each candidate's ID, with its declaring file appended in parentheses when the Resolve answer carries one; `statusFindings`' `glyph-ambiguous` arm delegates to it.
- `internal/planglyph/create.go`: `createFindings`' `create-already-exists` ambiguous arm delegates to the same renderer (one implementation, the two details cannot drift); dropped the now-unused `strings` import.
- Tests: new table-driven `TestCandidateList` (same-ID-two-files, no-file, mixed); `TestStatusFindings/Ambiguous` and `TestCreateFindings_AmbiguousIsAlreadyExists` extended to require each candidate's file in the detail (with a fixture-assumption guard so an empty File fails loudly rather than vacuously passing).
- Live verification (rebuilt binary FIRST): re-drove the L2b/L3 duplicate-declaration fixture through `lyx webster validate` — both arms now render `internal/greet#Dup (internal/greet/dup1.go), internal/greet#Dup (internal/greet/dup2.go)`.
- No doc change owed: neither `manifest/designs/quarry-glyph-plan-alphabet.md` nor `internal/planglyph/doc.go` pins the detail's text — both say "listing every candidate", which still holds (and now locates them).

### F-R2-1 (NIT) — `.Status` tripwire matches the `Unit` selector — commit `b05000491`

- `internal/planglyph/status_enforcement_test.go`: `"Unit"` added to `statusVocabularySelectors` (`ResolveResult.Unit` is Status-typed — the last unmatched spelling of the vocabulary); doc comments updated (FOUR spellings, the F-R2-1 rationale); `CanonicalizeHandles` added to `allowedStatusConsumers` as a reviewed name-shape hit (it reads `NameResult.Unit`/`Declaration.Unit`, the batched Name echo pair, not the status vocabulary — the name-shape scan cannot tell, so the reviewed function earns an allowlist row, mirroring how the refKind scan treats name collisions); new seeded self-test `TestStatusHitsIn_CatchesUnitConsumer`.
- Test-only change; no binary behavior moved, no doc change owed.

### F-R2-3 (verification-record correction) — no code/doc change, closed by the review report itself

Round 1's claim that the pre-resolution-rejection branch is structurally unreachable live is refuted by live scenario L4 (a Create group's `plan:nounit` handle reaches quarry through `createHandleResults`' `resolveKeyFor` key and comes back as a per-entry rejection, rendered by `createFindings`' fail-closed arm). The code is correct and matches the design doc, no production doc claims unreachability (grep verified), and round 1's report stays untouched as that round's own durable record — the corrected record lives in this round's review report. Nothing further to change.

## Deliberately deferred (with reasons)

- The two pre-existing smoke-test failures (`TestSmokeBootstrap_DiedDriverProceedsToHandoverAndLogsWhy`, `TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed`) — reproduced this round, re-evaluated per the prompt, and confirmed to sit in loom's driver bootstrap / Discussion-Write shuttle path, touching neither `internal/planparser`/`internal/planglyph` nor any Status/ref-shape dispatch. Out of this campaign's scope; not fixed here (one-concern-per-round, matching round 1's disposition).

## Verification

- `go build ./...` — green (before and after each fix).
- `go vet` over all eight in-scope package trees — green.
- `go test -count=5 ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/... ./cmd/lyx/...` — all `ok` after both fixes.
- `golangci-lint run ./internal/planglyph/...` — clean.
- `goimports -w` on every changed file.
- Rebuilt `.scratch/lyx` after the F-R2-2 source change and re-drove the ambiguous fixture live (new rendering confirmed), plus a final-binary spot check of the L1 canonicalization scenario (`{"cards":2,"ok":true}` and both card files rewritten to `plan:internal/greet#Praise`).
- Teardown: `.scratch/live-r2` fixtures and the dev binary removed; git tree clean; the only live tmux server on the host predates this session by a week (user's own) — zero stray processes attributable to this round.

## Changed files

- `internal/planglyph/resolve.go`
- `internal/planglyph/create.go`
- `internal/planglyph/resolve_test.go`
- `internal/planglyph/create_test.go`
- `internal/planglyph/status_enforcement_test.go`
- `_mill/loom-review-fable5-high-r2.md` (review report, committed incrementally through Job 1)
- `_mill/loom-review-fable5-high-r2-fixer-report.md` (this file)
