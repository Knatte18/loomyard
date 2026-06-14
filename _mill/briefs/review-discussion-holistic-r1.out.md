This surfaces a real gap: `docs/shared-libs/README.md` line 18 describes `internal/config` as "layered YAML config" — stale two-layer-vs-multi-layer wording in the same vein as B6/B7. The discussion's B5 only addresses README.md line 21 (the state.md library entry), and the staleness grep in the Testing section (line 272) searches for `layered YAML` in `docs/` and `internal/` — which would flag README.md line 18 as a "surviving config-layer description," yet no enumerated finding fixes it. This is an inconsistency between the "fix-all-staleness" decision + the grep guard and the per-finding inventory.

The muxpoc.md content claims are all backed by real symbols. The discussion is otherwise highly accurate. Let me compose the review.

MILL_REVIEW_BEGIN
# Review: docs-stale-sweep

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-14
```

## Findings

### [GAP] README.md "layered YAML config" not in inventory
**Section:** Scope B / Per-finding inventory (B5) / Testing (staleness grep)
**Issue:** `docs/shared-libs/README.md:18` describes `internal/config` as "layered YAML config" — the same multi-layer staleness as B6/B7 — but B5 only fixes line 21, and the Testing grep for `layered YAML` would flag this line as surviving config-layer wording, contradicting fix-all-staleness.
**Fix:** Add this line 18 wording to the B5 finding (or a new sub-item) so the enumerated inventory matches the grep guard.

### [NOTE] board.md:153 cli.go subsection line cited as ~151–158
**Section:** Per-finding inventory (A1)
**Issue:** The "configuration is loaded from layered YAML files" text the A1 sub-item targets is at board.md:153, inside the cited ~151–158 range — accurate, but the parallel `internal/board/cli.go` doc-comment at lines 9–11/32–35 still says `<cwd>/.mhgo/board.yaml`, matching A2.
**Fix:** None required; cross-references verified consistent. Recorded only to confirm A1/A2 line spans.

## Verdict

GAPS_FOUND
One staleness instance (README.md:18) is omitted from the inventory yet caught by the task's own grep guard.
MILL_REVIEW_END
