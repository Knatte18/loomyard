I've verified the load-bearing claims: the initengine assertion (`init_test.go:62`, `configreg.Modules()` exists), the ide-menu bug (`menu.go:40` loads config from `l.Cwd` but never sets `cfg.Path`; `boardcli/cli.go:103` is the reference `hubgeometry.BoardDir(layout.Hub)` pattern; the three red menu tests place the board at `<container>/_board` = `BoardDir(Hub)`), and the re-tiering fail-first/pass-after feasibility (the five named packages are the only untagged banned-token spawners; hubgeometry, gitexec, weftengine, boardtest, initengine, idecli/initcli/weftcli/warpcli are all already integration-tagged; smoke files are smoke-tagged; proc is allowlisted).

MILL_REVIEW_BEGIN
# Review: Fix test-suite regression: slow Tier 1 + 2 red packages + stale benchmarks

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-12
```

## Findings

### [NOTE] Guard token-match rule left unspecified
**Section:** Decisions § test-tier-purity-invariant
**Issue:** The banned list says `lyxtest.Copy`, but the real spawners are `lyxtest.CopyPaired`/`CopyHostHub`/`CopyPairedLocal` (CONSTRAINTS uses `Copy*`); whether the grep is prefix/substring vs whole-token, and whether hits inside comments/strings count, is not pinned — an exact-token implementation would miss perchcli's `CopyPaired` and silently pass the current tree, breaking the fail-first TDD claim.
**Fix:** State in the decision that the match is substring/prefix over source (matching `Copy*`), and note that the self-allowlist of `tierpurity_test.go` exists because matching is raw-substring (so comment/string mentions in other untagged files also trip it).

## Verdict

APPROVE
Scope, decisions, and feasibility are sound and source-verified; only a minor guard-mechanics detail to pin.
MILL_REVIEW_END
