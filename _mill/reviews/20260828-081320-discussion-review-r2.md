MILL_REVIEW_BEGIN
# Review: reed: header pane's boot sometimes leaves shell/log noise in its scrollback

```yaml
duration_s: 116.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:design] ED 3 backstop blinds the verification of record
**Section:** `scrollback-clearing-backstop` + `verification-is-a-real-deterministic-smoke-test`
**Issue:** The smoke test's whole assertion is `capture-pane -S -` showing only the header line, but `ED 3` is emitted by `header --blocking` *after* whatever the shell/pre-run wrote, so the scrollback comes up clean even if the split-window and seed-skip fixes are absent or later regress — the named "verification of record" pins only the backstop, and "fails on the pre-fix code" cannot be shown per-change since all four changes land together.
**Fix:** State how the source fixes are pinned independently (e.g. capture before the header's first write, or designate the untagged no-`send-keys`/annotation tests as the regression pins and demote the scrollback test's claim), and say what evidence counts as pre-fix failure.

### [BLOCKING:design] Untagged `--blocking` output test cannot run as described
**Section:** Testing → `internal/reedcli` (untagged)
**Issue:** "Assert the `--blocking` output begins with ED 2 + ED 3 + cursor-home … `RunCLI` already exposes the writer seam" is false: `header.go:64-67` calls `blockForever()` immediately after the single `fmt.Fprint`, and `internal/reedcli/header_test.go:3` records exactly that ("never invokes the --blocking path, since that path blocks forever") — the test as written hangs the untagged suite.
**Fix:** Decide the seam that makes the clear sequence assertable (extract the composed string into a pure helper, or drop the bullet and rely on the smoke path), and say which.

### [NIT:decision] Stencil Ownership Invariant wording left to the reviewer
**Demoted-from:** BLOCKING
**Section:** Constraints → Stencil Ownership Invariant
**Issue:** "Record the opt-out in the invariant's wording if review judges it a change to that invariant's meaning" punts a `CONSTRAINTS.md` edit to an unnamed later judgement; the invariant currently reads as unconditional ("Seed/refresh runs once per process pre-run"), and this task makes it conditional on an annotation.
**Fix:** Decide now whether `CONSTRAINTS.md` is edited in this commit, and if so what the amended sentence says.

### [NIT:design] Non-interactive-shell claim may not kill noise class 2
**Section:** Mechanism §2 / `single-string-shell-command-form`
**Issue:** The captured `-bash: .../env: No such file or directory` is consistent with `$BASH_ENV`, which a *non-interactive* `bash -c` still sources — so the "noise class 2 is gone" claim is asserted, not established, though `ED 3` would still cover the residue.
**Fix:** Soften the claim to "RC-file noise is removed for login/interactive RC paths; `ED 3` covers any residual `BASH_ENV`-style case".

### [NIT:decision] Non-blocking `lyx reed header` disposition unstated
**Section:** `header-opts-out-of-the-stencil-seed-pass`
**Issue:** A cobra annotation is per-command, not per-flag, so plain `lyx reed header` (the ordinary JSON-envelope preview verb) also stops seeding/committing stencils; the discussion only argues the keepalive's case.
**Fix:** State explicitly that both modes opt out and why that is acceptable.

## Verdict

REQUEST_CHANGES
Verification story and one untagged test are infeasible as written; one constraint edit undecided.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
