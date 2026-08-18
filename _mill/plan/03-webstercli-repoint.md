# Batch: webstercli-repoint

```yaml
task: "the standalone CLI path"
batch: "webstercli-repoint"
number: 3
cards: 4
verify: go test ./internal/webstercli/... && go test -tags integration ./internal/webstercli/...
depends-on: [1]
```

## Batch Scope

This batch repoints the already-shipped `webstercli` standalone entry from `preflight.HubPresent` onto `preflight.ResolveMode`, so all three standalone-capable CLIs select modes by one rule.
It is deliberately narrow: no verb, no engine wiring, no flag, and no other behaviour in the package changes.
It is its own batch rather than folded into batch 4 or 5 because it is a different package with a different reviewer surface, and because it carries the one hub-visible behaviour correction on the byte-identity list that is not about burler or perch at all — `webstercli` in a subdirectory of a wired worktree stops starting a silent standalone session and refuses instead.

**Batch-local decision:** the refuse case is pinned by a new integration-tagged test (card 12), not by a row in the untagged `wiring_test.go`.
See the overview's "the refuse case is pinned at the integration tier" shared decision for why a literal refuse row in `wiring_test.go` is unwritable.

## Cards

### Card 9: repoint `wire`'s parameter from `hubPresent bool` to `mode preflight.Mode`

- **Context:**
  - `internal/preflight/predicates.go`
  - `internal/preflight/doc.go`
- **Edits:**
  - `internal/webstercli/wiring.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `wire`'s signature from
  `func (c *websterCLI) wire(loc *lyxcwd.Location, hubPresent bool, cwd, stencilsDirFlag, planDirFlag, targetDirFlag string) error`
  to
  `func (c *websterCLI) wire(loc *lyxcwd.Location, mode preflight.Mode, cwd, stencilsDirFlag, planDirFlag, targetDirFlag string) error`,
  and change its body's `if hubPresent {` to `if mode == preflight.ModeHub {`.
  Add the `internal/preflight` import.
  `wireHub` and `wireStandalone` keep their exact current signatures and bodies — this card changes the dispatch parameter only.

  Rewrite `wire`'s doc comment, which is written throughout in `hubPresent` terms:
  the "computes hub-or-standalone mode from loc/hubPresent -- the *lyxcwd.Location and boolean preflight.HubPresent(cwd) already returned" opening now describes a told `preflight.Mode` from `preflight.ResolveMode`;
  the "hubPresent true selects hub mode ... hubPresent false selects standalone mode, which covers BOTH a plain downloaded git repository and an unresolvable cwd -- preflight.HubPresent returns a nil Location for each" paragraph now says `ModeHub` and `ModeStandalone`, with `ResolveMode` as what returns a nil Location for each standalone cause.
  Add a sentence stating that `wire` never sees the refused case at all: a `ResolveMode` error aborts upstream in `resolvePersistentPreRun`, because a refusal is a resolution verdict rather than a wiring choice — which is why `mode` carries only two reachable values here and no third `Mode` value exists for refusal.

  Preserve the existing `preflight.Wired is deliberately never consulted` paragraph in substance: its `fabricengine.Ready` paired-sibling argument and the three healthy hub locations are unchanged by this repoint.
  Preserve the closing "wire performs no cwd resolution and spawns no process" paragraph verbatim in substance — it is the Test Tier Purity claim card 11's test rests on.
  Also update the file-header comment's "once it has resolved cwd and the preflight.HubPresent probe" clause and its "with a told HubPresent result" clause, both of which name the old probe.
- **Commit:** `refactor(webstercli): take a told preflight.Mode rather than a hubPresent bool`

### Card 10: repoint `resolvePersistentPreRun` onto `ResolveMode` and add the refuse branch

- **Context:**
  - `internal/preflight/predicates.go`
  - `internal/webstercli/wiring.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/webstercli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `resolvePersistentPreRun`, replace `loc, hubPresent := preflight.HubPresent(cwd)` with a three-value call to `preflight.ResolveMode(cwd)`.
  A non-nil error is the refuse case: surface it verbatim via `output.Err(out, err.Error())`, then `clihelp.Abort(ctx, 1)` and `return nil` — the same shape the existing `lyxcwd.CwdFrom` error branch directly above already uses.
  Do not wrap, prefix, or re-word the error: every sentinel `ResolveMode` surfaces is already self-describing, and the gated `ErrCwdOutsideAnchor` message naming both paths and the marker file is the whole reason the refuse path exists.
  Then pass the resolved `mode` to `c.wire(loc, mode, cwd, c.stencilsDirFlag, c.planDirFlag, c.targetDirFlag)`.

  Repoint the two doc comments that name the old probe.
  The file-header comment at the top of `cli.go` says the parent command's `PersistentPreRunE` "runs one preflight.HubPresent probe, and delegates to c.wire"; `resolvePersistentPreRun`'s own doc comment says it "resolves cwd, probes preflight.HubPresent(cwd), and delegates the mode decision".
  Both must name `preflight.ResolveMode` and state that its error return aborts the pre-run rather than selecting a mode.
  This is a rewrite of both comments' surrounding sentences, not a token substitution: each must read correctly as prose about a three-way resolver.
  Add to `resolvePersistentPreRun`'s doc comment that the refusal deliberately stays here rather than moving into `wire`, because it is a resolution verdict, not a wiring choice.

  Leave the group-command guard (`if cmd.Name() == "webster"`) exactly as it is — it returns before any resolution and must keep doing so.
  Change no flag, no `Long`, and no verb registration in this file.
- **Commit:** `fix(webstercli): refuse a wrongly-entered hub instead of silently degrading to standalone`

### Card 11: update the untagged wiring truth table for the new parameter

- **Context:**
  - `internal/webstercli/wiring.go`
  - `internal/webstercli/cli_integration_test.go`
  - `internal/preflight/predicates.go`
- **Edits:**
  - `internal/webstercli/wiring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Update every `c.wire(...)` call in this file to pass `preflight.ModeHub` or `preflight.ModeStandalone` in place of the `true`/`false` second argument, adding the `internal/preflight` import.
  The mapping is exact: every existing `true` becomes `preflight.ModeHub`, every existing `false` becomes `preflight.ModeStandalone`.
  No test case's assertions change — this is a parameter-shape update to a table that already covers the right rows.

  Rename any test function or subtest whose name embeds `HubPresent` (for example `TestWire_HubPresentTrueSelectsHubMode`) so it names the mode rather than the retired probe, and update the file-header comment's "with a told preflight.HubPresent result" clause the same way.

  Add a comment to the file header recording two structural facts a later reader would otherwise try to "fix":
  first, that no `(loc non-nil, ModeStandalone)` row exists because no caller can produce one — `ResolveMode` returns a nil Location for both standalone causes;
  second, that the refuse case is deliberately absent from this file because `wire` never receives it (the error aborts upstream in `resolvePersistentPreRun`) and manufacturing one would require driving the real pre-run, which spawns git and breaches the Test Tier Purity Invariant — the exact invariant `wire`'s extraction exists to satisfy.
  Name `internal/webstercli/cli_integration_test.go` as where the refusal is pinned instead.
- **Commit:** `test(webstercli): drive wire's truth table with a told preflight.Mode`

### Card 12: pin the refusal end-to-end in the integration suite

- **Context:**
  - `internal/webstercli/cli.go`
  - `internal/preflight/predicates.go`
  - `internal/hubforge/hub.go`
  - `internal/lyxcwd/anchor.go`
  - `internal/webstercli/testmain_test.go`
- **Edits:**
  - `internal/webstercli/cli_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `TestRunCLIIn_WronglyEnteredHub_Refuses` to this already-`//go:build integration`-tagged file.
  Build a real wired hub via `hubforge.NewHub`, create an ordinary subdirectory inside the prime worktree, and drive `RunCLIIn(<that subdirectory>, &out, []string{"status"})`.
  Assert a non-zero exit code and that the output carries the gated cwd-anchor error — the message naming both paths and the anchor marker file — rather than a standalone session's own output.
  `status` is chosen because it drives the full pre-run without requiring a seeded plan directory, matching the reasoning the file's existing `TargetDirectoryUnchanged` test already records for the same choice.

  Add a second assertion that no standalone state tree was created for that subdirectory: this invocation must refuse before `wireStandalone` ever calls `standalonestate.Derive`.
  Redirect `XDG_STATE_HOME` and `LOCALAPPDATA` to `t.TempDir()` values before the call and assert both remain empty afterwards; without the redirect the assertion would be made against the operator's real state directory.
  The redirect means this test must not be marked `t.Parallel()` — `t.Setenv` panics under a parallel test.

  Document in the test's own doc comment that this is deviation four on the plan's hub byte-identity list: `internal/webstercli/cli.go` discarded `Resolve`'s error class before this task, so this invocation started a silent standalone session and now refuses.
  It is a deliberate correction of shipped behaviour, which is why it is pinned rather than merely allowed.
  The file's two existing tests keep their outcomes — a temp directory outside any git repository still resolves to standalone under `ResolveMode`, so neither moves — but both need one addition.
  `TestRunCLIIn_StandalonePreRun_ReachesRunsOwnValidationGate` and `TestRunCLIIn_StandalonePreRun_TargetDirectoryUnchanged` each reach `wireStandalone` today and redirect `XDG_STATE_HOME` only, so on Windows `standalonestate.Derive` still reads the operator's real `LOCALAPPDATA`.
  Add `t.Setenv("LOCALAPPDATA", t.TempDir())` alongside each one's existing `XDG_STATE_HOME` redirect, so both `Derive` branches land inside the test's own temp tree on every platform.
  This is the overview's "every test reaching `wireStandalone` redirects the state root" decision applied to its pre-existing half, matching what batch 4 card 16 and batch 5 card 23 do for their own packages; leaving it out would make that decision's "new or pre-existing" wording false in the one package that shipped standalone first.
  Change nothing else in either test: their assertions, fixtures and doc comments are unaffected by this repoint.
- **Commit:** `test(webstercli): pin the wrongly-entered-hub refusal end-to-end`

## Batch Tests

`verify:` runs `go test ./internal/webstercli/...` followed by `go test -tags integration ./internal/webstercli/...`.
Both invocations are load-bearing here.
The untagged run covers card 11's truth table plus the package's large existing untagged surface (`cli_test.go`, `verbs_test.go`, `wiring_test.go`), which is the regression coverage proving cards 9 and 10 changed the mode trigger and nothing else.
The tagged run is the only one that compiles card 12's new refusal test and the file's two existing standalone tests, whose continued passing is what proves the repoint did not turn the genuine not-a-repository case into a refusal.

The package already carries a hermetic `TestMain` (`internal/webstercli/testmain_test.go`), so no new test-main wiring is needed for card 12's git-spawning fixture.
