MILL_REVIEW_BEGIN
# Review: Adopt quarry's glyph alphabet as the plan alphabet — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.x-class model (Anthropic), self-reported as "Opus 5" by the harness
reviewed_file: plan/
date: 2026-09-06
```

## Findings

### [BLOCKING:scope] webster's own Run gate never moves to planglyph
**Location:** batch 5 (cards 23–24), overview `## All Files Touched`
**Issue:** `internal/websterengine/runlevel.go:334` calls `planparser.Validate(plan, deps.Geom.WorktreeRoot)` inside `Run`'s pre-flight — verified in source — yet that file appears in no card's `Edits:` and not in `## All Files Touched`; batch 5 moves only `loomshed.planValidate.Call`, `loomcli` `validate-plan` and `webstercli validate`, while its own Batch Scope justifies moving `webstercli validate` precisely because it mirrors "the identical gate `websterengine.Run` runs before forking an implementer".
**Fix:** add a card moving `runlevel.go`'s gate onto `planglyph.Validate` with the same severity and `ErrQuarryUnavailable` disposition, or state explicitly why the real gate stays on `planparser` while its CLI mirror does not.

### [BLOCKING:scope] RecordDeps carries no Plan for the three record-batch consumers
**Location:** batch 7, cards 33, 34, 36
**Issue:** `DoneChecks(plan, cards, worktreeRoot)`, `BindHandles(plan, planDir, delta, cards)` and `DetectDrift(plan, planDir, delta, sha, now)` are all called from `internal/websterengine/recordbatch.go`, but `RecordDeps` (recordbatch.go, lines 46–56) has no `Plan *planparser.Plan` field — unlike `BeginDeps`, which does — and no card says to add one or to parse the plan inside `RecordBatch`; `planDir` is reachable via `deps.Geom.PlanDir`, `plan` is not.
**Fix:** state in card 33 that `RecordDeps` gains a `Plan *planparser.Plan` field populated from the `planparser.ParsePlan(c.geom.PlanDir)` call `internal/webstercli/recordbatch.go` already performs.

### [BLOCKING:decision] directory-target can never fire after card 5
**Location:** batch 2, cards 4 and 5
**Issue:** card 4 defines `directory-target` over a `refKindPath` entry with no file extension, but card 5 canonicalizes **every** `isPathRef` entry — extensionless directory paths included — into `glyph.Self(lang, ref).String()` at parse time, so by validation time no such entry survives in a `language: go` plan; the discussion's `directory-classifier-rule` and `package-spelling` ("a bare directory path as a target is a hard finding") are instead silently satisfied by rewriting the bare directory into a valid unit self glyph, the same "canonicalizing makes guessing rewarded" failure `bare-symbol-is-hard-finding` rejects.
**Fix:** state in card 5 that canonicalization is gated on a path carrying a file extension, leaving extensionless path refs untouched so card 4's `directory-target` has something to classify.

### [BLOCKING:consistency] the RunCLIIn recount instruction produces a wrong number
**Location:** batch 6, card 29
**Issue:** source shows eleven packages with `func Command()`+`func RunCLI` and ten with `RunCLIIn` (`internal/selfreportcli` has none), and `cmd/lyx/seamsignature_test.go`'s own doc comment says "the eleven existing RunCLI … and the ten RunCLIIn" — so CONSTRAINTS' current "eleven of twelve" is already wrong, and card 29's prescribed method (count entries in `root.AddCommand(...)`, which includes the non-module `loomcli.RunAliasCommand()`) yields "twelve of thirteen", wrong again.
**Fix:** have card 29 derive both numbers from the `Command()`/`RunCLIIn` package sets `seamsignature_test.go` pins, not from `root.AddCommand` arity, and note the current sentence is being corrected rather than merely incremented.

### [BLOCKING:scope] quarrycli escapes the seam-signature guard
**Location:** batch 6, card 27
**Issue:** `cmd/lyx/seamsignature_test.go` pins each module's `RunCLI`/`RunCLIIn` through two hand-maintained blank-identifier var blocks; adding `quarrycli` leaves the guard compiling and passing while covering nothing, so card 27's "run the package's own guards … and fix what they report" discharges nothing, and the file is in no card's `Edits:` and absent from `## All Files Touched`.
**Fix:** add `cmd/lyx/seamsignature_test.go` to card 27's `Edits:` and to `## All Files Touched`, requiring `quarrycli.RunCLI`/`quarrycli.RunCLIIn` entries and the doc comment's counts updated.

### [NIT:scope] the new chokepoint guard may trip the tier-purity guard
**Location:** batch 3, card 15
**Issue:** every sibling source-scanning guard in `cmd/lyx` resolves its scan root via `exec.Command("go", "env", "GOMOD")` and therefore carries an `allowedSpawners` entry in `cmd/lyx/tierpurity_test.go` (see the twelve entries there); card 15 says to build `constraintchokepoint_test.go` "the way this repository's sibling guards do" but neither adds an allowlist entry nor pins the non-spawning `runtime.Caller(0)` root resolution `registration_test.go` uses.
**Fix:** have card 15 name `runtime.Caller(0)` root resolution explicitly, or add `cmd/lyx/tierpurity_test.go` to its `Edits:` for the allowlist entry.

### [NIT:scope] no batch verify runs the markdown link guard
**Location:** batches 4, 5, 6, 7 (cards 22, 25, 29, 39)
**Issue:** the Markdown Link Integrity guard lives in `internal/lyxcwd/docslink_test.go`, and those cards rewrite `docs/overview.md`, `manifest/designs/quarry-glyph-plan-alphabet.md` and `manifest/roadmap.md` — exactly its scan sources — yet no batch `verify:` includes `./internal/lyxcwd/`, so a broken link surfaces only at the hub's end-of-task done gate.
**Fix:** add `./internal/lyxcwd/` to the verify of whichever batches edit `manifest/` or `docs/` content.

### [NIT:completeness] card 4's two new checks have no named function or dispatch slot
**Location:** batch 2, card 4
**Issue:** cards 3, 10 and 14 each name their check function and its position in `validate`'s fixed dispatch list; card 4 names only the two check IDs `bare-symbol-target` and `directory-target`, leaving the function names and ordering unstated.
**Fix:** name the two check functions and their dispatch position, as the sibling cards do.

## Verdict

REQUEST_CHANGES
Two gate surfaces and one dependency struct are unaccounted for; one check is unreachable.
MILL_REVIEW_END
