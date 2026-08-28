MILL_REVIEW_BEGIN
# Review: reed: header pane's boot sometimes leaves shell/log noise in its scrollback

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:consistency] Seed-gate ordering assertion has no named seam
**Section:** Testing → `cmd/lyx` (untagged), 2nd bullet **Issue:** It demands "assert the ordering inside the extracted predicate's call site ... as a pure, in-process assertion" while the same discussion (Technical Context, and `cmd/lyx/stencilseed.go:36-38`, verified) establishes that `seedStencils` returns under `testing.Testing()` before either the annotation check or `stencilSeedTarget` runs — so no in-process test can observe their relative order, and the one observable form (no `git rev-parse` spawned) is explicitly forbidden two lines later. **Fix:** Either name the concrete seam that makes ordering observable (e.g. a single decision function that both consults the annotation and resolves the target, asserted with an annotated command outside any repo so it returns skip without touching geometry), or drop the ordering assertion and state that the annotation-present + predicate-returns-skip tests plus the Test Tier Purity guard are the whole `cmd/lyx` pin.

### [NIT:scope] `internal/clihelp` absent from the Scope "In" list
**Section:** Scope → In **Issue:** The annotation-key constant is decided to live in `internal/clihelp` (Decisions, Technical Context "Where a shared annotation key belongs"), but the In list names only `reedengine`, `cmd/lyx`, `reedcli`, tests, docs, `CONSTRAINTS.md` — a plan writer working from Scope alone would miss the touched package. **Fix:** Add `internal/clihelp` (one new exported annotation-key constant, no behaviour change) to the In list.

### [NIT:design] `ED 3` support in tmux/psmux is assumed, never stated as assumed
**Section:** Decisions → `scrollback-clearing-backstop` **Issue:** The discussion is explicit that psmux parity for the `split-window` command form is asserted-not-verified, but makes no equivalent statement for `\x1b[3J` — if the multiplexer's emulator does not implement clear-history for CSI 3 J, the backstop is a silent no-op, and B stays green anyway once the source fixes land, so no test would reveal it. **Fix:** Record the same asserted-not-verified disposition for `ED 3` (or state that B, run pre-source-fix, is the one observation that would confirm it).

## Verdict

REQUEST_CHANGES
One unimplementable test assertion contradicts the discussion's own analysis of `seedStencils`.
MILL_REVIEW_END
