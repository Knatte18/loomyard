MILL_REVIEW_BEGIN
# Review: reed: header pane's boot sometimes leaves shell/log noise in its scrollback

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:design] Seed pass has a second, mode-independent WARN source
**Section:** Mechanism §3 / Scope "Out" / Testing P2 **Issue:** `stencilstore.Reconcile` also calls `warnPortBackDrift` (`internal/stencilstore/reconcile.go:77-79`, body 170-190), which emits one `logger.Warn` per board-vs-worktree-drifted stencil in **either** mode whenever `sourceDir` is non-empty — so "stops firing the moment those stencils are refreshed ... or the binary is a production build" is false, and the "the two never fire together" WARN/commit exclusivity argument covers only the `StateUntouched` dev warn, not this one. **Fix:** state both WARN sources in Mechanism §3, correct the non-determinism explanation accordingly, and re-state P2's pre-fix expectation (stderr non-empty) without pinning it to "five WARN lines from the dev-mode branch only".

### [BLOCKING:consistency] Untagged RunCLI header assertion breaks tier purity
**Section:** Testing → `internal/reedcli` (untagged) — P3, second bullet **Issue:** "Assert non-blocking mode still returns the JSON envelope unchanged, driven through `RunCLI` as today" — no such untagged test exists today (`header_test.go`'s header states it "never runs RunE/PreRunE"; the only RunCLI-driven reedcli coverage is `cli_integration_test.go`, `//go:build integration`), and `RunCLI` → reed's `PersistentPreRunE` → `lyxcwd.Resolve` spawns `git rev-parse`, which the discussion's own Constraints section forbids in untagged `reedcli` tests. **Fix:** drop the "as today" claim and either move that assertion to the integration/smoke tier or replace it with a non-spawning seam.

### [NIT:consistency] P1's pane_current_command claim contradicts its own rejection
**Section:** `verification-per-fix-not-per-symptom` P1 vs `single-string-shell-command-form` **Issue:** the exec-prefix alternative is rejected on the ground that exec is what would make "the pane's process ... lyx rather than the shell", yet P1 proposes asserting `#{pane_current_command}` names the lyx binary — true only if the executing shell applies the last-command exec optimisation, which is shell-dependent. **Fix:** either qualify the optional assertion as best-effort/shell-dependent or drop it, leaving the argv+zero-`send-keys` assertion as P1's pin.

### [NIT:scope] Gate placement needs a signature change not stated
**Section:** Technical context → "Root pre-run and the seed pass" **Issue:** `seedStencils(ctx context.Context)` is called as `seedStencils(cmd.Context())` (`cmd/lyx/main.go:86`) and never sees the `*cobra.Command`, so "the annotation gate belongs alongside that early return" requires threading the command in. **Fix:** note that `seedStencils` takes the command (or the annotation map) as a parameter, so the plan does not treat it as a body-only edit.

### [NIT:design] Executing shell is tmux default-shell, not `e.cfg.Shell`
**Section:** `single-string-shell-command-form` rationale **Issue:** the precedent cited is `new-session ... e.cfg.Shell` (`lifecycle.go:314`), but a `split-window` shell-command argument is run by tmux's `default-shell`, not `cfg.Shell`, while the line is composed by `shell.ForGOOS()` — a mismatch the rationale reads as settled (risk is unchanged from today, since the typed line already lands in a default-shell pane). **Fix:** record explicitly that the command argument is interpreted by tmux's `default-shell` and that `cfg.Shell` parity is out of scope.

## Verdict

REQUEST_CHANGES
Mechanism understates the seed pass's WARN sources; one untagged test bullet breaks tier purity.
MILL_REVIEW_END
