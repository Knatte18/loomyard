MILL_REVIEW_BEGIN
# Review: batcher: split out of webster into a standalone configreg module with its own batcher.yaml — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-08-11
```

## Findings

### [BLOCKING:scope] Enumeration method misses `internal/websterengine/template.go`'s stale doc
**Location:** 00-overview.md `doc-site-ownership` decision / batch 03-documentation (cards 9-12)
**Issue:** `internal/websterengine/template.go`'s `ConfigTemplate` doc comment reads "role model-specs, batchifier selection, and Master session configuration" — falsified the moment batch 2 (card 5) deletes `batcher: ""` from `template.yaml`, since webster.yaml's template no longer carries batchifier selection. The line contains neither `batcher.Select`, `batcher:`, nor `batcher.yaml` (it says "batchifier selection"), so the decision's stated grep enumeration would not catch it, and no card in batch 3 touches this file. It is in the review manifest and is `template.go`'s sibling `config.go`'s identical clause IS caught and fixed by card 5 — this is the one file the sweep dropped.
**Fix:** Add a requirement (card 9 or a new card) to reword `internal/websterengine/template.go`'s `ConfigTemplate` doc comment to drop "batchifier selection," matching the fix already applied to `config.go`'s `Config` type doc comment in card 5.

### [BLOCKING:consistency] Card 5 names a webster config test that doesn't exist
**Location:** 02-call-site-migration.md, Card 5 Requirements + Batch Tests section
**Issue:** Card 5 instructs deleting "the `Batcher: "",` line from the `websterengine.Config` literal in `TestLoadConfig_TemplateDefaults`' `want`," and the Batch Tests section repeats the same name. No test named `TestLoadConfig_TemplateDefaults` exists in `internal/websterengine/config_test.go`; the test containing that `want := websterengine.Config{... Batcher: "", ...}` literal is `TestConfigTemplate_RoundTripsThroughLoadConfig`.
**Fix:** Correct both occurrences to `TestConfigTemplate_RoundTripsThroughLoadConfig`.

### [NIT:consistency] "Eleven sites" count in doc-site-ownership doesn't reconcile with batch 3's card scope
**Location:** 00-overview.md, `## Shared Decisions` → doc-site-ownership
**Issue:** The decision states "Eleven sites are enumerated in batch 3" and that `configcli_test.go` is "Site 11," but batch 3's four cards touch 13 distinct files (2+3+4+4), and several of those files carry more than one falsified clause (e.g. `batcher/doc.go` has three, `docs/overview.md` has two) — no plausible per-site count around the stated method reaches exactly eleven.
**Fix:** Recount or drop the specific number; the per-card Requirements are otherwise self-contained and don't depend on the count being accurate.

## Verdict

REQUEST_CHANGES
A real doc site is missed by the stated enumeration method, and one card cites a nonexistent test name.
MILL_REVIEW_END
