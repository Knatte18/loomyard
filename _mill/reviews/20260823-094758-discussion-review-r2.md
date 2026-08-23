MILL_REVIEW_BEGIN
# Review: landing: parent-fabric resolution chain

```yaml
duration_s: 230.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model; exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-23
```

## Findings

### [NIT:consistency] loomcli may not name PushWarpRebaseFreeAt
**Demoted-from:** BLOCKING
**Section:** Decisions § push-verb-named-by-the-caller (and Constraints § Fabric Vocabulary Invariant)
**Issue:** The bare weft/warp ban is not landingshed-specific: `fabricVocabularyOwners` (internal/lyxcwd/enforcement_test.go:597) is {fabricengine, fabriccli, weftname, gitkit, boardengine, configsync, hubforge}, and `internal/loomcli` is not in it; `fabricVocabularyHits` fails on any `*ast.Ident` whose name contains "warp"/"weft" — which includes a selector's `Sel` — so `fabricengine.PushWarpRebaseFreeAt(...)` written in `drive.go` fails `TestEnforcement_FabricVocabulary`. No non-owner production file names such an identifier today, so there is no precedent to lean on, and the discussion's "CONSTRAINTS.md — no change expected" makes the contradiction explicit.
**Fix:** Decide the neutral spelling the caller actually uses (a vocabulary-neutral `fabricengine` export delegating to `PushWarpRebaseFreeAt`, or an owner-set amendment with the CONSTRAINTS edit it implies) — "the closure body naming `PushWarpRebaseFreeAt` lives in `loomcli`" cannot stand as written.

### [NIT:decision] ReadOrigin's found/error returns have no stated disposition
**Demoted-from:** BLOCKING
**Section:** Decisions § parent-branch-from-origin-record; Technical context § Deps table, `ParentBranch` row
**Issue:** `ReadOrigin` is `(Origin, bool, error)` (internal/fabricengine/origin.go:75) and a false bool is the legacy-worktree case, explicitly *not* an error; the table names only `→ Origin.ParentBranch` and the discussion never says what `drive` does when the record is absent, carries an empty `ParentBranch`, or the read errors. An empty value then flows into the PR base branch and into `OpenParent`'s matcher, while "rejecting an empty `ParentBranch`" is explicitly out of scope — so nothing catches it anywhere.
**Fix:** State drive's behaviour for all three returns, e.g. an up-front envelope refusal mirroring `resolveParentBranch`'s "pass --parent once to record it" message.

### [BLOCKING:design] Self-parent match left for the implementer to decide
**Section:** Testing § `internal/fabricengine` — the matcher, bullet 4
**Issue:** "Decide and pin the behaviour here rather than leaving it implicit" defers the decision itself rather than making it: the discussion never says whether `OpenParent` returning the acting worktree's own pair is allowed-and-correct or an error. It is reachable — `loom run --parent <own branch>` is not rejected by `resolveParentBranch` (internal/loomcli/seedinput.go) — and the result is `Finalize` merging a branch into itself via `parentHandle.Merge`.
**Fix:** Record the choice as a Decision (match-and-proceed vs. refuse in `OpenParent`), so the test pins a decided behaviour rather than inventing one.

## Verdict

REQUEST_CHANGES
One machine-enforced vocabulary conflict plus two undecided items block plan writing.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
