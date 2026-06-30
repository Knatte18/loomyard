MILL_REVIEW_BEGIN
# Review: Harden the Path Invariant: close enforcement hole + fix geometry leaks

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-30
```

## Findings

### [GAP] Literal-match semantics unspecified; -weft-bare sites omitted
**Section:** Decisions — "enforcement test: AST scan" + "lyxtest.go geometry routed through paths"
**Issue:** The scan's matching rule (whole-token equality vs. substring) is never stated, yet `lyxtest.go` builds `base+"-weft-bare"` at lines 207 and 481 (`filepath.Join(tmpDir, base+"-weft-bare")`) — two `+`-operand sites the conversion list omits (it names only the three `base+"-weft"` sites, 185/475/541). Under substring matching these fail the new scan, falsifying "tree-scan must pass once converted"; under exact-token matching they pass but the rule must say so. The synthetic negatives (`Long:` field literal) only pin context-scoping, not token-matching.
**Fix:** State the detector matches whole geometry tokens (not substrings), and explicitly confirm lines 207/481 are out of scope — or, if substring, add them (e.g. `paths.WeftSiblingPath(tmpDir, base) + "-bare"`).

### [NOTE] warpcli/clone.go HubSuffix site not enumerated
**Section:** Decisions — "warp routes every geometry site" / Technical context
**Issue:** `internal/warpcli/clone.go:51` does `filepath.Join(cwd, name+warpengine.HubSuffix)`; deleting `warpengine.HubSuffix` breaks its compilation, but warpcli is absent from the In-scope conversion list (only warpengine files listed) and the target (`paths.HubPath(cwd, name)` vs `paths.HubSuffix`) is undecided.
**Fix:** Add warpcli/clone.go:51 to the conversion list and pick `paths.HubPath` (matches the zero-literal goal).

### [NOTE] board Resolve error discarded
**Section:** Decisions — "board data dir is paths-owned" / Technical context line 256
**Issue:** `layout, _ := paths.Resolve(cwd)` swallows the error; if Resolve fails while `_lyx` exists, `cfg.Path = filepath.Join("", "_board") = "_board"` is set silently instead of surfacing a clean error.
**Fix:** Handle the Resolve error via the JSON envelope rather than discarding it.

## Verdict

GAPS_FOUND
Unspecified literal-match semantics leave two omitted `-weft-bare` sites that could fail the deliverable scan.
MILL_REVIEW_END
