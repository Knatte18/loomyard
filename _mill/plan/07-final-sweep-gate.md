# Batch: final-sweep-gate

```yaml
task: "Rename hub container suffix from -HUB to -LYXHUB"
batch: "final-sweep-gate"
number: 7
cards: 1
verify: go test ./internal/lyxcwd/...
depends-on: [1, 2, 3, 4, 5, 6]
```

## Batch Scope

A single zero-diff gate that runs after every other batch has landed, confirming the sweep came out exactly as sorted rather than approximately.
It exists because the task's whole risk is a mechanical replace that went too far or not far enough, and neither failure mode is caught by any test: a corrupted sentinel still compiles and still passes, and a missed fixture still compiles and still passes.

It delivers no code.
It depends on every other batch because its entire value is being last.

Batch-local decision beyond `## Shared Decisions`: this batch's card is verification-only and carries no commit, which is why its `Edits:`, `Creates:`, `Deletes:`, and `Moves:` fields are all `none`.

## Cards

### Card 22: Confirm the surviving occurrences are exactly the four sanctioned ones

- **Context:**
  - `internal/fabricengine/clone_reset_guard_test.go`
  - `internal/fabricengine/destructivegaps_integration_test.go`
  - `internal/shuttleengine/claudeengine/startup_test.go`
  - `docs/research/session-fork-spike.md`
  - `internal/reedcli/smoke_teardown_test.go`
  - `CONSTRAINTS.md`
  - `internal/lyxcwd/reponame_test.go`
  - `internal/reedengine/server_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Run a recursive listing of every file still containing the retired token — `grep -rl -- '-HUB' .` from the worktree root — and discard every hit under the mill task-state tree, which is not product source and legitimately keeps discussing the old token in its own transcripts.

  What remains must be exactly seven files, and for the stated reason in each case:
  - `internal/fabricengine/clone_reset_guard_test.go` — the `sentinel` const whose value is an arbitrary recognisable marker containing the token as a substring, never the hub suffix.
    Confirm the file's header comment was substituted and only the const value survives.
  - `internal/fabricengine/destructivegaps_integration_test.go` — the `sentinel` const of the same kind, plus the `AcceptsLegacySuffixedHubDirectory` name-blindness subtest, which deliberately builds its fixture path with the retired suffix to prove `looksLikeHub` never inspects the container's own name.
    Confirm only the const value and that one subtest's literal survive.
  - `internal/shuttleengine/claudeengine/startup_test.go` — a verbatim captured startup log line from a past real run.
    It is a record of what happened, not a claim about current code, and is never retro-edited.
  - `docs/research/session-fork-spike.md` — a dated research note recording where a past live-session spike actually ran.
    Same reason.
  - `CONSTRAINTS.md` — the Hub Suffix Invariant section (added by core-constants) names the retired token by design, as the very thing the invariant declares no code parses, trims, or recognises any more.
    This is documentation of current rule, not stale prose, and core-constants' Card 1 specifies this literal text verbatim.
  - `internal/lyxcwd/reponame_test.go` — `TestBuildLocation_RepoNameSuffixTrimming`'s degradation case (added by core-constants' Card 3) pins that a hub basename still carrying the retired suffix is not trimmed, by design; the literal is the fixture the case exists to exercise, not sweep residue.
  - `internal/reedengine/server_test.go` — two comments (added by peripheral-tests' Card 14, per that card's explicit instruction to record the re-check conclusion) explaining why the socket-key boundary assertions are unaffected by "the -HUB -> -LYXHUB rename"; historical/explanatory prose about this very task, not a claim about a currently-recognised suffix.

  `internal/reedcli/smoke_teardown_test.go` must not appear in the surviving set: its single occurrence was conceptual prose, reworded to lowercase rather than substituted.
  If it does appear, that rewording was missed.

  If any other file appears, the sweep is incomplete — report which file and which line, and do not treat the gate as passed.
  If any of the four is missing, a sanctioned occurrence was overwritten — report which, and do not treat the gate as passed.

  Produce no diff.
  This card confirms earlier work and changes nothing.
- **Commit:** none

## Batch Tests

`verify: go test ./internal/lyxcwd/...` runs the two repository-wide gates one final time, after every batch has landed.

`TestEnforcement_GeometryLiterals` walks the whole source tree and fails if any production file outside the two registered owner directories names the new token in a path-construction context, or if either owner stopped naming it — the machine-enforced half of the invariant batch 1 recorded.
`TestEnforcement_MarkdownLinks` walks every markdown file under the documentation and manifest roots, catching a link or anchor broken by the prose sweep in batches 5 and 6.

The scope is deliberately not repository-wide here.
`pipeline.done_gate` is already configured as `go test ./... && go test -tags integration ./...` and runs from the repository root before the task is marked done, so the full regression net — including every package this plan swept and the whole integration tier — is applied there rather than duplicated at this boundary.

The card itself has no runnable assertion: a grep census of sanctioned residue is a judgement about four specific occurrences, each of which needs the reason read alongside the line.
