MILL_REVIEW_BEGIN
# Review: fabric: close the weft-visibility leak (slice 8)

```yaml
verdict: GAPS_FOUND
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic) — exact release string not self-observable
reviewed_file: _mill/discussion.md
date: 2026-08-06
```

## Findings

### [GAP] Committed() is not value-identical to WeftCommitted
**Section:** `commit-result-committed-method` / Constraints ("no behavioural delta")
**Issue:** `classifyPaths` (`classify.go:14-28`) routes a file to warp whenever it is not under the *wired* pathspec, and a narrow `fabric.yaml` pathspec is a supported config (`TestHealthy_NarrowPathspecIsHealthy`); in that case `_lyx` writes land as a warp commit, so today's `committed = res.WeftCommitted` (`buildercli/weft.go:37`, `webstercli/weft.go:32`, `perchcli/run.go:353`) is `false` where `Committed()` would be `true` — a change in the `fabricCommitted` JSON value and in the helper's `(bool, error)` return.
**Fix:** State whether the widening is intended, and either record it as a second behavioural-surface change with a test, or define `Committed()` to preserve the current per-site semantics.

### [GAP] "host" is not in the vocabulary rule but carries the same leak
**Section:** `fabric-vocabulary-rule` / `templates-describe-one-repo`
**Issue:** The ban covers only the tokens `weft`/`warp`, yet the two-repo model is equally taught by "host": `implementer-body.md:31` and `implementer-template.md:37,66-67` say "commit to the HOST repo", and `drift.go`'s five reason strings all begin "host …" and reach loom's operator report — the enforcement test would pass with an agent still told there is a non-host repo.
**Fix:** Decide explicitly whether `host` joins the policed token set (and what the templates/reason strings say instead), or record why it is exempt.

### [GAP] Template rewrite decided for one file, not five
**Section:** `templates-describe-one-repo`
**Issue:** Concrete replacement wording is given only for `master-template.md:20,29,136,140-143`; the remaining occurrences get no decision — `master-template.md:148-149` (never cited at all), `implementer-body.md:31`, `implementer-template.md:37,64,66-67`, `orchestrator-template.md:11,88`, `instruction-3-fix-template.md:2,26,29,31`, two of which are section *headings* that pinned template tests assert on.
**Fix:** Enumerate the five files and state, per occurrence class, whether the line is reworded, folded into the positive `_lyx` rule, or deleted.

### [GAP] HealthReason shape and its display string undecided
**Section:** `healthy-typed-reason`
**Issue:** "a small struct or enum-plus-detail" leaves the type shape unchosen, and nothing says what the fabric-worded branch-mismatch display string becomes — today `drift.go:58` is `"host on %s, weft on %s (want %s)"`, whose entire content is which checkout is on which branch, and `loomengine` prints it verbatim via `report.addFailure`.
**Fix:** Fix the type shape, the ok-case zero value, and give the replacement text for all five display strings, stating what diagnostic detail (if any) is deliberately dropped.

### [NOTE] Sequencing note misfiles burlerengine as comment-only
**Section:** Technical context, "Sequencing note for mill-plan"
**Issue:** `burlerengine` is listed among packages with "comment-only cleanup … independent batch", but it owns `instruction-3-fix-template.md` (4 occurrences) and a pinned template test that must land with the template rewrite.
**Fix:** Move `burlerengine` out of the comment-only batch list, or annotate it as comment-plus-template.

### [NOTE] Enforcement test placement and template discovery
**Section:** `enforcement-test`, `documentation`
**Issue:** A fabric vocabulary rule is hung off the Cwd Resolution Invariant and lives in `internal/lyxcwd/enforcement_test.go`, and "every `//go:embed`-ed `.md` under `internal/`" implies parsing embed directives — a mechanism left unspecified, though all five leak-bearing `.md` files are embedded, so a plain `internal/**/*.md` walk is equivalent today.
**Fix:** Say whether the rule gets its own `CONSTRAINTS.md` section, and pin the template-discovery mechanism (directive parse vs. `.md` walk).

## Verdict

GAPS_FOUND
Four unresolved items: a real value delta, the untreated "host" token, template wording, and the reason type.
MILL_REVIEW_END
