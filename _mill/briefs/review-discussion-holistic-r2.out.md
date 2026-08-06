MILL_REVIEW_BEGIN
# Review: fabric: close the weft-visibility leak (slice 8)

```yaml
verdict: GAPS_FOUND
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-06
```

## Findings

### [GAP] Unexporting New/Warp/Weft breaks fabricengine's own external tests
**Section:** `one-constructor-open`, Technical context ("45 uses inside `fabricengine`, **zero** … mechanical in-package rename")
**Issue:** `internal/fabricengine/fabric_test.go`, `checkout_index_refresh_test.go`, `warpforward_integration_test.go`, `weftgit_exclude_test.go` are `package fabricengine_test` and call `fabricengine.New(...)` and `f.Warp`/`f.Weft` — an in-package rename does not reach them, and `fabric_test.go:25,44` are exactly the missing-path contract tests `Open`'s TDD item claims to inherit.
**Fix:** State how these four files migrate (move into `package fabricengine`, or restate their contract through `Open` plus a deleted fixture side) and list them under regression coverage.

### [GAP] Healthy's typed-cause set is under-enumerated
**Section:** `healthy-typed-reason`
**Issue:** `drift.go` returns five distinct reason shapes — branch mismatch (`:58`), `"host junction check unavailable: cannot load fabric.yaml"` (`:69`), junction missing (`:83`), not-a-junction (`:94`), points-elsewhere (`:110`) — and all four non-branch shapes today classify as `CheckJunction` + `check3BlocksSeed` only because they contain the substring "junction"; the discussion names three causes and its equivalence test says only "broken/missing junction".
**Fix:** Enumerate all five causes (notably the config-load-failure one, which is an error surfaced as a reason) and require the equivalence test to cover each.

### [GAP] Repo-level docs carrying the vocabulary are unscoped
**Section:** Scope / `documentation` / `enforcement-test`
**Issue:** The enforcement walk is `internal/` + `cmd/` only and the doc list names four files, leaving `README.md:62`, `docs/overview.md:133`, `docs/skills.md:167`, `docs/reference/builder-contract.md:167` and `docs/benchmarks/test-suite-timing.md` describing `_lyx` as "weft-synced" — the most operator-facing surface in the repo. A plan writer could reasonably read these as in or out.
**Fix:** State explicitly whether prose docs outside the four named files are in scope, and why the machine check does not cover them.

### [GAP] Templates drop the warning but RefScanner keeps the penalty
**Section:** `templates-describe-one-repo` vs `refscanner-owns-both-halves`
**Issue:** The scanner still hard-fails a run on `\S*-weft\b` or the sibling path (`audit.go:94`), while the templates will no longer tell an agent that path exists or is forbidden — a sibling is discoverable by an ordinary `ls ..`, so an unwarned agent can fail a whole run on a rule it was never given.
**Fix:** Decide and record how the templates keep an actionable, geometry-free ban (e.g. "only `_lyx/...` paths; never a path outside `{{.worktree_root}}`") sufficient to make the audit's penalty reachable by instruction.

### [GAP] A negative assertion goes vacuous after the reword
**Section:** Testing → Regression coverage
**Issue:** `internal/webstercli/verbs_test.go:633` asserts the envelope does *not* contain `"weft sync failed"`; after `diagnostics-say-fabric-detail-says-weft` the string can never appear, so the test passes forever without checking anything. It is not in the regression list (which names only `buildercli/run_test.go:150` and `configcli_test.go:187`).
**Fix:** Add it to the list with the required update to the new `"fabric sync failed"` wording.

### [GAP] lyxcwd/anchor.go's comments do not reword losslessly
**Section:** Technical context ("reword cleanly")
**Issue:** `lyxcwd/anchor.go:2,4,32,39` document that `.fabric-anchor` sits at the **weft:main root** and is written by fabricengine's weft:main commit choke; `lyxcwd` is not an owner, and substituting "fabric" erases which of the two checkouts physically holds the marker — the "weft-synced `_lyx`" example does not generalise to this file.
**Fix:** Name the files whose comments describe two-repo mechanics rather than sync semantics, and state the rewording strategy (relocate the detail into `fabricengine`, or drop it) for each.

### [NOTE] "Never told" holds only until the first sync failure
**Section:** `diagnostics-say-fabric-detail-says-weft`
**Issue:** The wrapped `%v` keeps weft-level detail, and `master-template.md:143` instructs Master to quote that failure verbatim into `stuck_reason` — so an agent learns the word, and writes it into `_lyx`, on the first failure.
**Fix:** Acknowledge this bound explicitly, or scope the detail fabric exposes through the consumer envelope.

### [NOTE] Guard-test naming drift not covered by the hand-clean
**Section:** Scope → Test files
**Issue:** `cmd/lyx/boardguard_test.go` calls the invariant "Weft Git Invariant" (CONSTRAINTS.md says "Fabric Git Invariant (warp + weft)"), and `cmd/lyx/rawgitmutation_test.go:10,45` names `WarpBisector`/`WarpResetter` in comments this task renames.
**Fix:** Add `cmd/lyx/`'s guard tests to the hand-clean list.

### [NOTE] WEFT_SKIP_GIT literals live in non-owner test files
**Section:** Out list / Scope → Test files
**Issue:** `WEFT_SKIP_GIT` is kept, but its literal appears in `webstercli`, `buildercli`, `perchcli`, `configcli` tests — the "hand-cleaned of vocabulary that is not owner-package API" criterion does not obviously permit it.
**Fix:** State that retained env-var names are an explicit carve-out from the test hand-clean.

## Verdict

GAPS_FOUND
Six gaps: external-test migration, cause enumeration, doc scope, template/audit asymmetry, vacuous assertion, comment fidelity.
MILL_REVIEW_END
