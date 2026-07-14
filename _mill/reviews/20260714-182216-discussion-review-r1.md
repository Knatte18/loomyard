MILL_REVIEW_BEGIN
# Review: Investigate the unexplained lyx mux server crash

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-14
```

## Findings

### [GAP] Log dir conflates _lyx (durable) with .lyx (ephemeral)
**Section:** Decisions § server-log-under-lyx-logs (+ Constraints, Testing "Live verification")
**Issue:** The decision says logs land "under `_lyx/`" with a rationale that "`_lyx` survives cleanup" and a "helper mirroring `ConfigDir`" (which is under `_lyx`/LyxDirName), yet names `e.layout.DotLyxDir()` as the seam — but `DotLyxDir()` resolves to `.lyx` (dotLyxDirName), which hubgeometry.go:26-30 documents as deliberately distinct: ephemeral/machine-bound and NOT weft-synced, opposite lifecycle to `_lyx`. The two choices have contradictory semantics for forensics (multi-MB server logs committed into the weft repo vs kept machine-local).
**Fix:** Decide explicitly whether server logs go under `_lyx` (durable, weft-synced) or `.lyx` (machine-local, matching where mux.json already lives via DotLyxDir), correct the rationale to the real lifecycle of the chosen dir, and name the matching hubgeometry helper.

### [NOTE] debug_log 0-2 range validation location overstated
**Section:** Decisions § debug-log-config-key
**Issue:** The decision claims `debug_log` is "strict-validated by the config template like every other key" and "values outside 0-2 are a config error," but configengine.Load only checks missing keys + resolves env (config.go:65-91) — it does no int range/type validation; the Testing section correctly puts range-check "0-2" in a Go helper, so the two sections disagree on where validation lives.
**Fix:** State that the 0-2 range (and non-numeric env input) is validated in the Go level→argv helper, not the template; the template only supplies the key/default.

## Verdict

GAPS_FOUND
One GAP: the log-directory decision conflates durable `_lyx` with ephemeral `.lyx`, which have opposite lifecycles.
MILL_REVIEW_END
