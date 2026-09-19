MILL_REVIEW_BEGIN
# Review: Shed-generic watchdog for ly-drive and loom's CLI verbs — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Opus 5 (claude-opus-5)
reviewed_file: plan/
date: 2026-09-19
```

## Findings

### [BLOCKING:scope] Generic verb bodies never told to call clihelp.SetExit
**Location:** batch 3, cards 11/12/13/14/15 (and batch 5 card 30)
**Issue:** `internal/clihelp/exec.go`'s `RunRootCtx` returns `es.code`, and `es.code` is written only by `clihelp.SetExit`; `output.Err`/`ErrFields`/`Ok` merely *return* 1/0. Every shipped body wraps them — `clihelp.SetExit(ctx, output.Err(...))` in `internal/loomcli/run.go:63,71,89,...`, `step.go:140,149,155,...`, `status.go:106,112,116,150`, `pause.go:34,46,50`, `internal/lifecyclecli/run.go:43,49,60,79,86,97,102,119`, `status.go:42,48,55`. The plan says only "reports on the error envelope" and never names `SetExit`, so a literal implementation exits 0 on every refusal.
**Fix:** state in each verb-body card that the body records the helper's return through `clihelp.SetExit(cmd.Context(), …)`, exactly as the deleted bodies do.

### [BLOCKING:decision] shedverbs helpers are unexported, yet loomcli tests are retargeted onto them
**Location:** batch 3 cards 12/14 vs batch 4 cards 22/23/28
**Issue:** Card 12 declares `stepEnvelope` and card 14 declares `renderStatusLine`/`unavailableLine`/`printStatusLinesOnChange` unexported. Card 22 then points `internal/loomcli/step_test.go`'s five `stepEnvelope(...)` calls (lines 38, 79, 180, 227) at "`shedverbs.stepEnvelope`'s exported equivalent", and card 23 points `status_test.go`'s `renderStatusLine` (line 58), `statusUnavailableLine` (92–93) and `printStatusLinesOnChange` (112) at label-taking versions — none of which any card creates, and batch 4's own Batch Tests forbids editing `internal/shedverbs`. Card 23's fallback (delete the duplicate table) then contradicts card 28, which requires `status_test.go` to "still pin the rendered watch line's format for the `loom` label".
**Fix:** decide per helper whether `shedverbs` exports it (naming the exported identifier in card 12/14) or whether the loomcli test tables are deleted, and make cards 23 and 28 agree.

### [BLOCKING:scope] Card 22 edits internal/loomcli/cli.go without listing it
**Location:** batch 4, card 21/22
**Issue:** Card 22's Requirements add "a `parentFlag string` field to the `loomCLI` struct", bind it in `Command()`, and "carry the entry observation from `PreRun` to `PostRun` on the `loomCLI` receiver" — the struct and `Command()` both live in `internal/loomcli/cli.go`, which appears in neither card 22's `Edits:` nor its `Context:`.
**Fix:** add `internal/loomcli/cli.go` to card 22's `Edits:`, or move the two field additions into card 21, which already edits that file.

### [BLOCKING:consistency] Byte-for-byte help text has neither a live source nor the claimed guard
**Location:** batch 4, card 24; batch 5, card 32
**Issue:** Cards 22 and 23 delete `runCmd`/`stepCmd`/`statusCmd`/`pauseCmd` in earlier commits, yet card 24 says to copy their `Use`/`Short`/`Long` "byte-for-byte out of the deleted … constructors" — by then only in git history. The stated safety net is false: `cmd/lyx/longlist_test.go` only asserts `root.Long` contains each *registered top-level module name*, and `cmd/lyx/jsonhelp_test.go` exercises only root/board/selfreport, so neither can fail on a reflowed loom or lifecycle `Long`, and card 32's "a `longlist_test.go` failure naming a `loom` or `lifecycle` verb" cannot occur.
**Fix:** have card 24 (or card 21) lift `loomVerbTexts` verbatim *before* the deletions, and drop the claim that an existing `cmd/lyx` guard machine-checks the copy.

### [BLOCKING:design] Card 31's run/step parity cases have no stated fixture arm or substrate bound
**Location:** batch 5, card 31
**Issue:** `lyx loom run`'s pre-flight reaches `c.reed.Up()` (`internal/loomcli/run.go:88`) and `step`'s PreStep reaches `ensureStatusStrand`, which also calls `c.reed.Up()` (`internal/loomcli/sharedbootstrap.go:185`) — a real tmux session; every existing loomcli test touching that substrate is `//go:build smoke` (`smoke_test.go`, `smoke_bootstrapwiring_test.go`, `smoke_attachprobe_test.go`, `smoke_operatorstrand_test.go`). Card 31 places run/step parity in the `integration` tier over a hubforge hub without saying which arm of each verb the fixture exercises or how the case stops short of reed/tmux and of `shed.Step`'s producer call.
**Fix:** pin per verb which refusal the fixture lands on (e.g. `run` against an unseeded status file, which refuses before `reed.Up()`), and state explicitly that no parity case may reach reed, tmux, or an LLM producer.

### [NIT:consistency] Card 6's "no string in this package names loom" exceeds its own Edits
**Location:** batch 2, card 6
**Issue:** `internal/lifecycleshed/doc.go:2` and `teardown.go:1,14` also name the loom session, and `teardown.go` is in neither `Edits:` nor `Context:`; the discussion scopes the neutralization to the inner-run producer's own log and stuck-reason strings.
**Fix:** narrow the closure clause to "no string in the moved file or in `InnerRunDeps` names loom".

### [NIT:consistency] Card 39's grep is narrower than the rule it claims to prove
**Location:** batch 6, card 39 vs card 36 rule (a)
**Issue:** Rule (a) covers every `lyx loom <verb>`, but the confirmation greps only for `lyx loom step`; `plugins/ly/skills/ly-drive/SKILL.md` also carries `lyx loom status` (lines 37, 92), `lyx loom start` (41) and `lyx loom run` (116).
**Fix:** grep for the `lyx loom ` prefix rather than the single `lyx loom step` literal.

## Verdict

REQUEST_CHANGES
Five blocking gaps: exit codes, unexported helpers, an unlisted edit, help text, parity tier.
MILL_REVIEW_END
