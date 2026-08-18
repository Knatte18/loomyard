# done_gate fix r1 — TestEnforcement_FabricVocabulary regression

## Root cause

`internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_FabricVocabulary` (tree-scan
subtest, see the test body starting at line 758, the bare-token predicate at line 654
`bareVocabularyToken`, and the owner set `fabricVocabularyOwners` around line 590-608)
fails any non-test `.go` file under `internal/` or `cmd/` — outside a small owner set
(`internal/fabricengine`, `internal/fabriccli`, `internal/weftname`, `internal/gitkit`,
`internal/boardengine`, `internal/configsync`, `internal/hubforge`) — that contains the
bare, case-insensitive substring "weft" or "warp" anywhere: identifier, string literal,
or comment.

The `webster-told-geometry` task's batch 3 (preflight doc correction) and batches 5/8
(webstercli two-mode hub/standalone pre-run) edited three non-owner files and, in each
case, described `fabricengine.Open`'s stat check or `fabricengine.Ready`'s probe using
the phrase "the weft sibling" instead of the neutral "the paired sibling" phrasing the
rest of the non-owner codebase already uses for the identical concept — e.g.
`internal/preflight/predicates.go:23` ("probes the paired sibling of the current
worktree, not the hub") and `internal/preflight/doc.go`'s own line 54, which says the
same thing about the same call two lines below the offending line 43. This was purely
an accidental vocabulary leak in prose, not a deliberate reference to fabric internals
that the owner set was meant to carve out — the plan's "All Files Touched" list never
flagged it because it's a leaf regression in comment wording, not a functional gap.

## Offending sites and fix

- `internal/preflight/doc.go:43` — `probes the paired weft sibling of the current` →
  `probes the paired sibling of the current`, now textually identical to line 54's
  sentence about the same `fabricengine.Ready` call.
- `internal/webstercli/cli.go:71` — `fabricengine.Open stat-checks the weft sibling and
  would fail` → `fabricengine.Open stat-checks the paired sibling and would fail`.
- `internal/webstercli/wiring.go:120-121` — same rewording as `cli.go`, in the
  `wireHub` comment describing why `openFabric` stays a lazy closure.

All three are pure comment reword, zero behavior change. No production code, test, or
fixture was touched, and the owner set (`fabricVocabularyOwners`) was left unmodified —
these three files do not legitimately belong in it; they were simply using the banned
word where the established non-owner idiom ("paired sibling") already covers the same
meaning without invoking fabric's weft/warp vocabulary.

## Verification

Ran the full `pipeline.done_gate` command from the worktree root after the fix:

```
go test ./... && go test -tags integration ./...
```

Both halves passed clean (exit 0, no FAIL lines), including
`internal/lyxcwd`'s `TestEnforcement_FabricVocabulary` and every other package.

## Commit

Single atomic commit on `webster-told-geometry` (not pushed — push left to the
Builder's finalize step per the task brief):

- `74b579d9e7309832e3047de72e6658b29bd760a8` — "Fix fabric-vocabulary leak: reword bare
  "weft" mentions outside owner set"

`golangci-lint run` was run whole-project per the golang-build skill; its findings are
all pre-existing and in files unrelated to this fix (internal/lock, internal/fslink,
internal/shedengine, internal/scoutengine, internal/treadleengine, internal/websterengine,
internal/boardengine, internal/webstercli/cli_test.go, internal/treadleengine/handoff.go,
cmd/lyx/drift_test.go) — none touch `internal/preflight/doc.go`,
`internal/webstercli/cli.go`, or `internal/webstercli/wiring.go`, so per the "targeted
regression fix, not a design change" instruction they were left alone. `goimports -w` on
the three changed files made no further changes. Codeguide is not initialized for this
repo (`resolve.py --json` returned `found: false`), so the codeguide-update step was
skipped per the git-commit skill's own conditional.

## CONSTRAINTS.md / manifest/designs update

Not needed. This was a wording-only fix inside the invariant the Fabric Vocabulary
Invariant (`CONSTRAINTS.md` line 246) already documents; the invariant's own text was
correct and unaffected — the three files were simply out of compliance with it, not
carrying a design decision that needed documenting.

```json
{"status":"success","commit_sha":"74b579d9e7309832e3047de72e6658b29bd760a8"}
```
