MILL_REVIEW_BEGIN
# Review: Shed-generic watchdog for ly-drive and loom's CLI verbs — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-opus-5
reviewed_file: plan/
date: 2026-09-19
```

## Findings

### [BLOCKING:design] Arm's fixed signature leaves the module receiver unwired
**Location:** batch 4, cards 21 and 25 (and batch 5, card 29)
**Issue:** Card 21 fixes `Arm(cwd, verb, args) (shedverbs.Spec, error)` — the same type card 29 gives `entry.Arm` — then in its own last two lines orders "Restructure `Arm` to return both the spec and the receiver, or to take the receiver as a parameter", which that type forbids; worse, rewriting `resolvePersistentPreRun` to only call `Arm` and assign `*c.spec` leaves the outer `*loomCLI` unwired, and `internal/loomcli/start.go` (13 receiver-field reads) plus `internal/loomcli/validate.go` (`c.env.DecisionRecordPath`, `c.env.SupportLogPath`, `c.env.AnchorPath`, `c.env.WorktreeRoot`) depend on `wire`/`wireLightweight` having populated it.
**Fix:** state the split explicitly — an unexported `arm(c *loomCLI, cwd, verb, args)` that the pre-run calls on its own receiver, with exported `Arm` as a thin constructing wrapper — and say the pre-run still wires `c` for `start`/`validate-discussion`/`validate-plan`.

### [BLOCKING:design] `--parent` has no route from Command() into loom's PreStep
**Location:** batch 4, cards 22 and 24
**Issue:** Card 22 says `--parent` is registered on the returned `step` command and read "by closure into the `PreStep` hook exactly as `parentFlag` is captured today", but today `parentFlag` is a local of `stepCmd` (`internal/loomcli/step.go:105`); after the move the flag variable lives in `Command()` while `PreStep` is built inside `Arm`, which sees only `(cwd, verb, args)` — and `args` in a `PersistentPreRunE` are positional only, never parsed flags.
**Fix:** name the carrier (a `parentFlag` field on `loomCLI` bound by `Command()`, read by `Arm`'s hook) and state the disposition of `--parent`'s absence on `lyx shed step --recipe loom`.

### [BLOCKING:design] loom's run would perform the fabric block twice
**Location:** batch 4, cards 21 and 22
**Issue:** Card 22 puts `fabricengine.Open`/`CurrentBranch`/`OriginURL`/`ReadOrigin`/`resolveLandingParent` and `c.env.Landing = landingDeps(…)` into `PreRun`, while card 21 sets `BuildShed` to `c.buildLoomShed()` — and `internal/loomcli/sharedbootstrap.go:232-274` shows `buildLoomShed` already performs that exact block before calling `loomrecipe.New`, so `lyx loom run` opens the fabric and reads origin twice where it does so once today.
**Fix:** drop those five steps and the `Landing` assignment from `PreRun` and leave them inside `buildLoomShed`, or have loom's `BuildShed` call `loomrecipe.New(c.env, c.shedPaths)` directly.

### [BLOCKING:scope] Batch 1 omits nine ShedPaths call sites
**Location:** batch 1, cards 2, 3 and 4
**Issue:** Card 4 retargets only `wiring_test.go`/`wire_test.go`, but `loomrecipe.ShedPaths`/`lifecyclerecipe.ShedPaths` are also constructed in `internal/loomcli/status_test.go:167`, `cli_test.go:239`, `step_test.go:304`, `sharedbootstrap_test.go:55,91` and `internal/lifecyclecli/run_test.go:36`, and the bare in-package `ShedPaths{` at `internal/loomrecipe/fixture_test.go:664`, `internal/loomrecipe/shape_test.go:139` and `internal/lifecyclerecipe/fixture_test.go:52` stop compiling once cards 2/3 delete those types.
**Fix:** add all nine to card 4's `Edits:` (and the overview's `## All Files Touched`), replacing the two-file hand list with the same repo-wide grep the card already implies.

### [BLOCKING:scope] Batch 2 omits four Env.LoomRun construction sites
**Location:** batch 2, cards 7, 8 and 9
**Issue:** The `Env.LoomRun` → `Env.InnerRun` field rename also breaks `internal/shedrecipe/fixture_test.go:152`, `internal/lifecyclerecipe/fixture_test.go:31`, `internal/lifecyclecli/run_test.go:60` and `internal/lifecyclecli/lifecycle_integration_test.go:42-43` (`c.env.LoomRun.Spawn = …`), none of which appear in any card's `Edits:`; separately `internal/lifecyclerecipe/recipe_test.go:103`'s `want := []string{"LoomRun", …}` engine list must move to `"InnerRun"`, yet card 8 does not list that file and card 9 says "change no existing assertion in `recipe_test.go`".
**Fix:** add the four files to card 7, assign `recipe_test.go:103` unambiguously to card 8 or 9, and revisit batch 4's "deliberately does not depend on batch 2" claim — both batches now edit `lifecyclecli/run_test.go` and `lifecycle_integration_test.go`.

### [BLOCKING:design] Card 31's parity tests cannot be tier 1, and batch 5 runs no tagged tier
**Location:** batch 5, cards 31 and 34 + the batch's `verify:`
**Issue:** Driving both sides through `RunCLIIn` reaches each module's `PersistentPreRunE` → `lyxcwd.Resolve` → `gitexec.Run` (`internal/lyxcwd/lyxcwd.go:149`), which the Test Tier Purity Invariant bans outside `integration`/`smoke`-tagged files, and loom's `run` additionally calls `c.reed.Up()` and `fabricengine.Open`; the cited precedent's own header (`internal/loomcli/parity_test.go:10-11`) states "no test here calls RunCLIIn -- so both stay tier 1". Batch 5's `verify:` chains no `go test -tags integration`, so a tagged parity suite would never compile in-batch.
**Fix:** decide the tier explicitly — tag the `RunCLIIn`-driven cases `integration` and chain `go test -tags integration ./internal/shedcli/...` onto batch 5's `verify:`, or drop `RunCLIIn` and compare envelopes over told receivers as the precedent does.

### [BLOCKING:consistency] The NewShed hoist double-prefixes every recipe error
**Location:** batch 1, cards 1, 2 and 3
**Issue:** Card 1 wraps `NewShed`'s parse and build errors with `"shedbuild: "` while cards 2/3 wrap the delegated error again with `"loomrecipe: "`/`"lifecyclerecipe: "`, so today's `loomrecipe: <err>` (`internal/loomrecipe/loomrecipe.go:91,96`) becomes `loomrecipe: shedbuild: <err>` on `lyx loom run`/`step`'s error envelope.
**Fix:** pick one prefix layer — either `NewShed` returns unwrapped errors, or the two `New` bodies return `NewShed`'s error unwrapped — and say so in card 1.

### [NIT:consistency] lifecycle's found-status envelope is seven keys, not six
**Location:** batch 4, card 26
**Issue:** The card says the four core keys plus `found`/`status_path`/`history` "reproduce today's six-key found-envelope exactly", but `internal/lifecyclecli/status.go:55-63` emits seven keys.
**Fix:** correct the count to seven so an implementer does not pin a six-key closure assertion.

### [NIT:consistency] loomcli.ensureStatusLockDir is left with no caller
**Location:** batch 4, card 23
**Issue:** Deleting `statusCmd` and `pauseCmd` removes the only two callers of `internal/loomcli/sharedbootstrap.go:142`'s `ensureStatusLockDir`, and no card states whether it is deleted or kept.
**Fix:** give it a disposition in card 23, the same way the card already disposes of an emptied `status.go`/`pause.go`.

### [NIT:consistency] Card 34 is a no-file card that instructs a file to be written
**Location:** batch 5, card 34
**Issue:** It says "If `internal/shedcli/parity_test.go` has no such case, add it there rather than here; this card changes no file", with `Edits: none` and `Commit: none`.
**Fix:** move the broken-config lightweight-wiring case into card 31's own `Requirements:` and leave card 34 purely confirmatory.

## Verdict

REQUEST_CHANGES
Four design gaps in the Arm seam and parity tier, plus two incomplete rename inventories.
MILL_REVIEW_END
