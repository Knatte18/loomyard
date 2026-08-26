MILL_REVIEW_BEGIN
# Review: fabric: clone doesn't commit written module configs

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [BLOCKING:scope] Fixture-breakage sweep found only SeedConfig
**Section:** Scope / `seedconfig-tolerates-empty-stage` / Testing §8
**Issue:** The "fixture helper acquires a new way to fail" class was enumerated by hand (r1 caught `hubforge/seed.go`) and stops there, but the same pattern exists verbatim in two other setups that stage nothing of their own: `internal/preflight/preflight_integration_test.go:51-52` and `internal/preflightshed/preflight_integration_test.go:52-53` both run `gitkit.MustRun(t, h.PrimeWeft(), "git", "add", "-A")` then a bare `git commit`; their only stageable content today is the nine untracked configs (`.lyx` is excluded, the `_extra` junction target is an empty dir git ignores), so after this change `git add -A` stages nothing, `git commit` exits 1, and `gitkit.MustRun` `tb.Fatalf`s.
**Fix:** State the enumeration method for this class (e.g. every `add`/`commit` pair run at `h.PrimeWeft()` with no file written by the fixture itself), and give the resulting sites an explicit disposition in Scope rather than leaving them to the §8 regression sweep.

### [BLOCKING:decision] configengine shared-lib doc has no disposition
**Section:** `docs-in-the-same-commit`
**Issue:** `docs/shared-libs/configengine.md` carries an `## Exported functions` section that enumerates `ConfigDir` and `ConfigFile` (lines 111-125); the new exported `ConfigFileRel` lands beside them, but the Decision enumerates only `CloneAndWire`'s doc comment and `hubforge`'s `hub.go`/`doc.go` and never names this file either way.
**Fix:** Say explicitly whether `docs/shared-libs/configengine.md` gains a `ConfigFileRel` entry in the same commit, per the Documentation Lifecycle rule.

### [NIT:consistency] Backfill remedy overstates its reach
**Section:** `no-backfill-for-existing-hubs`
**Issue:** "After that single fix-up, every subsequently added pair inherits the configs" holds only for pairs added off the fixed prime — `add.go:133-134,170` forks from `WeftBranchName(<the invoking worktree's warp HEAD branch>)`, so a pair added from an existing pre-fix pair still inherits a config-less weft branch.
**Fix:** Qualify the remedy: pairs added from an already-broken existing pair still need `lyx config reconcile --apply`.

## Verdict

REQUEST_CHANGES
Two dispositions missing: sibling fixture breakages and the configengine shared-lib doc.
MILL_REVIEW_END
