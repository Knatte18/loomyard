MILL_REVIEW_BEGIN
# Review: Extract scout into its own standalone repo

```yaml
duration_s: 325.0
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.x-class model (Anthropic); exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-20
```

## Findings

### [NIT:decision] `Options.AnchorRoot` has no disposition
**Demoted-from:** BLOCKING
**Section:** `state-path-ownership-moves-to-the-caller`
**Issue:** The decision names `DaemonStateFile`/`DaemonLock` and the `ensureServer`/`ensureSupervised` params, but the value actually reaching them is the exported field `Options.AnchorRoot` (`internal/scoutengine/refs.go:51`, threaded at `refs.go:67`) — public API of the new `quarry` package, and set by three files on the "ported unchanged" list (`refs_integration_test.go:91,206,244`).
**Fix:** State whether `Options.AnchorRoot` becomes `Options.StateDir`, and move those three files off the "ported unchanged (import paths and package clauses only)" list if so.

### [BLOCKING:design] Config half of told-geometry left undecided
**Section:** `config-and-state-paths` / `exact-replacement-shapes` / Constraints
**Issue:** State ownership was resolved to "engine is told a leaf directory", but `LoadRegistry(baseDir)` (`internal/scoutengine/load.go:22-23`) still derives `<baseDir>/_lyx/config/servers.yaml` via `configengine.ConfigFile`; the Constraints section says the new config resolution "belongs in `internal/cli/`, not in the `quarry/` package", which is unsatisfiable while the exported engine function takes a base dir and joins.
**Fix:** Decide `LoadRegistry`'s new signature explicitly — told a resolved absolute file path, or the overlay load moves to `internal/cli/` entirely.

### [NIT:scope] `clihelp` used surface is five symbols, not four
**Demoted-from:** BLOCKING
**Section:** "The exact used surface of the shared packages" / TDD candidate 3
**Issue:** The table (declared "measured, not assumed") omits `clihelp.NewExitContext`, used at `internal/scoutcli/cli_test.go:345,471,510` together with the `es.Code()` method on clihelp's unexported `exitState`; `cli_test.go` is on the "ported unchanged" list and cannot compile without it.
**Fix:** Add `NewExitContext` plus the exit-state type/`Code()` to the clihelp replacement shape and to TDD candidate 3's pinned semantics.

### [NIT:decision] 59 `"scoutengine: "` error prefixes, no stated disposition
**Demoted-from:** BLOCKING
**Section:** `mechanical-move-not-hand-transcription` / Testing step 4
**Issue:** Nine production files carry 59 `fmt.Errorf("scoutengine: …")` literals (`errors.go` 12, `ensureserver.go` 18, `lspclient.go` 8, …) that reach the user through the JSON envelope's error field; the port program rewrites only import paths and package clauses, so they survive verbatim, while renaming them breaks step 4's byte-for-byte envelope comparison.
**Fix:** Decide keep-verbatim (dead vocabulary in quarry) or rename (and carve error-message text out of step 4's equality criterion alongside absolute paths).

### [NIT:consistency] Q&A log contradicts the `test-tier-tags` decision
**Demoted-from:** BLOCKING
**Section:** Q&A log, "Test tiers and CI?"
**Issue:** That entry answers "Inherit the `//go:build integration` / `smoke` separation", which the `test-tier-tags` decision supersedes with a single `lsp` tag — and the decision itself warns that a verify command spelled `-tags integration` would run one file of six while appearing green.
**Fix:** Update the Q&A entry to record the resolved answer, as the `servers.yaml` entry already does.

### [BLOCKING:design] Enumeration method is blind to token-free scout facts
**Section:** "Loomyard removal checklist"
**Issue:** `grep -rli 'scout'` cannot surface facts that become false without containing the word: `CONSTRAINTS.md`'s CLI/Cobra Invariant ("Twelve of the thirteen seam modules", "all thirteen modules"), `docs/overview.md:270`, and `cmd/lyx/seamsignature_test.go:1,2,30,48` all encode a module count that drops by one, and no machine guard checks prose counts.
**Fix:** Add a second, count-oriented sweep rule to the deletion batch (module counts, tier-tag lists, and any "N modules" prose) alongside the token grep.

### [NIT:consistency] Retracted sentence left standing in Technical context
**Section:** "`lookupContext` is the hinge", line 220
**Issue:** "Redirecting those three is the entire state-path change" remains verbatim, although `state-path-ownership-moves-to-the-caller` explicitly names it as understating the change.
**Fix:** Delete or amend the sentence so the Technical context does not read as a live instruction.

## Verdict

REQUEST_CHANGES
Told-geometry resolved for state only; config, `Options.AnchorRoot`, and clihelp surface still open.
_Note: 4 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
