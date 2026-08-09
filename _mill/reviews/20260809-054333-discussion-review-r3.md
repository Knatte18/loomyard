MILL_REVIEW_BEGIN
# Review: Rename the fabric host vocabulary to warp, and name the composite repo Fabric

```yaml
verdict: GAPS_FOUND
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-09
```

## Findings

### [GAP] Renaming non-owner production comments breaks rule (1)
**Section:** Decisions, "Non-owner residue is folded in" **Issue:** `enforcement_test.go:883` fails any *non-owner* dir whose production comment carries a bare `warp` token; swapping `internal/configcli/configcli.go:269` ("the host `_lyx` parent" → "the warp `_lyx` parent") makes `internal/configcli` fail `TestEnforcement_FabricVocabulary` immediately — the discussion treats the residue as pure `wordswap` output. **Fix:** state that non-owner *production* residue must be reworded to a neutral term (or the package added to the owner set), not word-swapped, and enumerate which files that covers.

### [GAP] "Exactly three verb-sense hits" is measurably wrong
**Section:** Technical context, "boundary-case survey" **Issue:** `internal/buildercli/poll.go:321` ("a live pane hosting an idle agent process") and `poll_test.go:212` (same wording) are verb-sense `host`+lowercase in a now-in-scope package, so the survey's "exactly three" and commit (a)'s "five ambiguous occurrences" are both undercounts — the run will report seven, tripping the stated "a sixth is a finding worth pausing on" tripwire on a known-benign hit. **Fix:** re-run the verb-sense survey over the widened scope and restate both counts, or drop the pinned number for commit (a) as already done for (c)/(d).

### [GAP] Undefined resolution for legitimately-unswapped ambiguities
**Section:** Decisions, "Ambiguous compounds are reported" **Issue:** the tool exits non-zero while any ambiguity is "unresolved", the run takes no `-skip`, and the only resolution described is rewording — so a correct verdict of "leave `hosting` alone" has no representation and the run can never exit zero. **Fix:** define the resolution mechanism (re-run with `-skip`, an accept-file, or mandatory rewording of every verb-sense hit) and say which this rename uses.

### [GAP] Final completeness-grep exclusion list contradicts the doc exclusions
**Section:** Testing, "Repo-wide completeness check" **Issue:** it expects "zero hits outside the three named exclusions (`CONSTRAINTS.md` ban list, `enforcement_test.go` ban list, `docs/benchmarks/test-suite-timing.md`)", omitting `docs/benchmarks/fixture-copy.md`, `docs/research/scout-spike.md` and `internal/configengine/config_test.go`, all excluded elsewhere in the same document. **Fix:** make the check's exclusion list identical to the Constraints-section "never run over" list.

### [GAP] Guard-tightening commit rule contradicts the four-commit plan
**Section:** Constraints, "Discovered during discussion" **Issue:** "the guard tightening and the `weftname`/`boardengine` fixes must land in the same commit" conflicts with the commit granularity decision, which puts those fixes in (a) and the tightening in (d). **Fix:** restate as "the fixes must land in or before the tightening's commit", confirming (a)→(d) satisfies it.

### [GAP] Residue enumeration omits `internal/builderengine`
**Section:** Scope, "In" (non-owner residue) **Issue:** `internal/builderengine/spawn.go:446` ("the host commit immediately before") is fabric-sense production residue in a package the list does not name, while `:9`/`:178`/`:236`/`:277` in the same file are machine/verb sense that must stay. **Fix:** add the package with an explicit per-line in/out call, or state that the residue list is illustrative and the sweep is grep-driven.

### [NOTE] `.md` half of the tightening not stated
**Section:** Decisions, "Enforcement guard is tightened" **Issue:** the host-half owner skip appears twice — the Go walk (`:886`) and the `internal/**/*.md` walk (`:903`) — and the decision names only one. **Fix:** say both are tightened (the "no owner dir contains a `.md` file" check confirms it is free).

### [NOTE] Owner-set reconciliation direction unspecified
**Section:** Technical context, "Existing guard mechanics" **Issue:** `CONSTRAINTS.md:164` lists `tools/`/`sandbox/` as owners while `fabricVocabularyOwners` does not; "reconcile the doc against the actual map" does not say whether the doc drops them or the map gains them. **Fix:** name the intended end state.

## Verdict

GAPS_FOUND
Guard interaction with non-owner residue and two miscounted surveys must resolve first.
MILL_REVIEW_END
