MILL_REVIEW_BEGIN
# Review: loom: self-checkable mechanical gates

```yaml
duration_s: 182.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-23
```

## Findings

### [NIT:design] Parity test's verdict mapping is unstated
**Section:** `parity-tests-per-gate` / `## Testing` **Issue:** The producer has three outcomes (`Done`, `Stuck`, returned error) but the verb has two exit states (0/1), so a "pass/fail verdicts agree" assertion collapses `Stuck` and error onto the same side — the exact distinction `findings-not-bool`'s pinned short-circuit order exists to protect. **Fix:** State whether the parity test compares three-way (using the envelope message or `output.ErrFields`' `findings` key to separate a findings failure from an I/O fault) or accepts the binary mapping as sufficient.

### [NIT:consistency] Cross-gate parity rule filed under a package-named invariant
**Section:** `new-constraints-invariant` **Issue:** Bullet four ("a mechanical gate's `ShedProducer` row and its CLI self-check verb call the same package function") binds both gates — including the `planparser`-backed one — yet lives inside a *Discussionparser* Sole-Parser Invariant, where a future gate author would not look for it. **Fix:** Say whether that bullet stays there or becomes its own short section, so the plan writer does not have to guess.

### [NIT:consistency] Scope bullet contradicts the later test decision
**Section:** `## Scope` in-list vs `## Technical context` (registration tests) **Issue:** Scope says "Extend `internal/loomcli/cli_test.go`'s registered-verb assertion to six verbs", while the later decision explicitly rejects "merely extending its four-entry map to six" in favour of a tightened exact-set assertion over `parent.Commands()` with a rename. **Fix:** Reword the Scope bullet to name the tightening, so a plan writer reading Scope alone does not reproduce the blind spot.

### [NIT:consistency] Quote attributed to the wrong file
**Section:** `new-package-named-discussionparser` **Issue:** The quoted "reads `decision-record.md`'s Scope section — the same section-parsing `Discussion-Validate` already does" is attributed to the roadmap's `Plan-Sweep` item, but it is `manifest/designs/loom.md:119`; `manifest/roadmap.md`'s `Plan-Sweep` item only points at that design section. **Fix:** Re-attribute the citation to `manifest/designs/loom.md`.

## Verdict

APPROVE
Source claims verified accurate; only presentational and test-granularity nits remain.
MILL_REVIEW_END
