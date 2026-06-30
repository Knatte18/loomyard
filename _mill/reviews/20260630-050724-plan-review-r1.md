MILL_REVIEW_BEGIN
# Review: Harden the Path Invariant: close enforcement hole + fix geometry leaks — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-30
```

## Findings

### [NIT] Stale `path:` seed lines left in board test fixtures
**Location:** Batch 3, Card 15
**Issue:** `seedCwd` (cli_test.go:40 `path: board`) and `TestLoadConfig_HappyPath` (config_test.go:36 `path: _custom_board`) hardcode a `path:` line that is no longer a template key; Card 15 never instructs removing it. `configengine.Load` only checks *missing* keys (config.go:66), so this won't break the build, but it is exactly the hardcoded-fixture drift the Path Invariant calls out (PR #20 anecdote).
**Fix:** Add to Card 15: strip the `path:` line from `seedCwd` and the HappyPath seed (and fix the "all template keys" comment).

### [NIT] HappyPath test risks losing home/sidebar/proposal coverage
**Location:** Batch 3, Card 15
**Issue:** Card 15 lists config_test.go:36 among "path:-resolution cases" to "delete (or repurpose)", but that case is the general happy path that also exercises home/sidebar/proposal_prefix and env resolution. A literal delete drops that coverage.
**Fix:** State that TestLoadConfig_HappyPath must be *repurposed* (remove only the path seed line + path assertion), not deleted.

### [NIT] LoadConfig godoc becomes false after dropping resolution
**Location:** Batch 3, Card 13
**Issue:** Card 13 restates the `Path` field comment but leaves the `LoadConfig` function godoc (config.go:53-56 "Preserves relative-Path resolution …"), which the same card makes untrue by deleting that block. Also: removing the `yaml:"path"` tag does not by itself stop yaml.v3 mapping `path:`→`Path` (it lowercases field names); the real guarantee is Card 14 overwriting `cfg.Path`, so the card's stated rationale is imprecise (harmless).
**Fix:** Have Card 13 also update the `LoadConfig` function godoc; optionally use `yaml:"-"` and correct the rationale.

## Verdict

APPROVE
Plan is sound, complete, and accurately grounded; only minor test-fixture/doc-hygiene NITs remain.
MILL_REVIEW_END
