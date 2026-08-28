# Batch: scrollback-backstop-and-composite-smoke

```yaml
task: "reed: header pane's boot sometimes leaves shell/log noise in its scrollback"
batch: "scrollback-backstop-and-composite-smoke"
number: 3
cards: 4
verify: go test ./internal/reedcli/ && go test -tags smoke -run 'TestSmokeHeaderPayloadClearsPaneScrollback|TestSmokeHeaderPaneScrollbackIsClean' ./internal/reedcli/
depends-on: [1, 2]
```

## Batch Scope

This batch adds the defence-in-depth backstop and the composite end-to-end check, and it lands **after** batches 1 and 2 for a reason recorded in the discussion: `ED 3` runs after anything the shell or the pre-run wrote, so once it exists the pane comes up clean whether or not the source fixes do.
Landing it first would erase the ability to demonstrate each source fix independently.
Both `depends-on` edges are real: batch 3 edits `internal/reedcli/header.go`, which batch 2 also edits, and B asserts a composite outcome that only holds once batch 1's launch change and batch 2's seed-skip are both in the tree.

Three things land here.
`headerCmd`'s `--blocking` path prints `\x1b[2J\x1b[3J\x1b[H` instead of `\x1b[2J\x1b[H`, so the scrollback buffer is cleared and not only the visible screen.
Those bytes move into a pure, unexported helper so they are assertable in the fast untagged suite: the `--blocking` path calls `blockForever()` immediately after its single `fmt.Fprint`, so any untagged test that drives `RunCLI --blocking` hangs the suite forever, and the writer seam does not help because the process never returns to the test.
And the composite `capture-pane -p -S -` assertion lands as B, documented in its own file header as pinning the composite outcome and none of the individual fixes.

`ED 3`'s efficacy against a real multiplexer is proved directly by card 14 rather than inferred from landing order — see the overview's "`ED 3`'s efficacy is proved directly" Shared Decision for why this replaces the discussion's ordering-dependent observation.
On a Windows/psmux host it remains unverified, for the same reason every Windows claim in this task is: this worktree is Linux and cannot execute a Windows run.
That gap is a follow-up, not a blocker.

Batch-local decision beyond the overview's Shared Decisions: cards 14 and 15 share one new smoke file rather than two, because both are scrollback assertions reading the same `capturePaneScrollback` helper and one file header can state plainly which of the two pins what.

## Cards

### Card 12: clear scrollback as well as screen on the header's blocking render

- **Context:**
  - `internal/reedengine/headerpane.go`
  - `internal/reedengine/header.go`
- **Edits:**
  - `internal/reedcli/header.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an unexported pure helper `headerBlockingPayload(text string) string` to `internal/reedcli/header.go`, returning the exact bytes the `--blocking` mode writes: the `ED 2` + `ED 3` + cursor-home sequence `"\x1b[2J\x1b[3J\x1b[H"` followed by `strings.TrimRight(text, "\r\n")`.
  Replace the blocking branch's `fmt.Fprint(out, "\x1b[2J\x1b[H"+strings.TrimRight(text, "\r\n"))` with `fmt.Fprint(out, headerBlockingPayload(text))`, leaving the following `blockForever()` call and everything else in `RunE` unchanged.
  Document on the helper that it exists so the byte sequence stays assertable without entering the blocking path, and that this is the same composition-split-from-side-effecting-call-site shape `internal/reedengine/headerpane.go` already uses for exactly this reason.
  Document why `ED 3` is there: `ED 2` clears the visible screen only and does not touch scrollback, which is precisely why the observed noise survived where the operator eventually saw it;
  `ED 3` is a backstop that guarantees the pane is clean at the moment the header renders regardless of what any future code path, shell, or terminal wrote before it, and it is not the pin for any individual source fix.
  Update the file's header comment and the command's `Long` text where they describe the boot mechanics, so neither still implies the pane's text is typed into a shell that survives it.
- **Commit:** `fix(reedcli): clear scrollback as well as screen on the header's blocking render`

### Card 13: pin the header blocking payload's clear sequence

- **Context:**
  - `internal/reedcli/header.go`
- **Edits:**
  - `internal/reedcli/header_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `TestHeaderBlockingPayload`, asserting `headerBlockingPayload`'s return value byte-for-byte: for an input carrying trailing `"\r\n"` it returns `"\x1b[2J\x1b[3J\x1b[H"` followed by the input with its trailing carriage returns and newlines stripped, and for an input with no trailing newline it returns the same prefix followed by the input unchanged.
  Include a case pinning that interior newlines are preserved, so the trim can never widen into a full-text normalisation.
  Assert on the helper only.
  Do not drive `RunCLI` with `--blocking` anywhere in this file: that path calls `blockForever()` right after its single `fmt.Fprint`, so a test reaching it never returns and hangs the untagged suite.
  Do not add an untagged assertion on the non-blocking JSON-envelope path either — driving it through `RunCLI` reaches reed's `PersistentPreRunE` and therefore `lyxcwd.Resolve`, which spawns `git rev-parse`, banned here by the Test Tier Purity Invariant.
  Update the file's header comment so its existing statement about never invoking the `--blocking` path now also records that the payload is asserted through the pure helper instead, and why.
- **Commit:** `test(reedcli): pin the header blocking payload's clear sequence`

### Card 14: prove the header payload clears real pane scrollback

- **Context:**
  - `internal/reedcli/header.go`
  - `internal/reedcli/smoke_test.go`
  - `internal/reedcli/smoke_header_keepalive_test.go`
- **Edits:** none
- **Creates:**
  - `internal/reedcli/smoke_headerscrollback_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create a `//go:build smoke` file holding `TestSmokeHeaderPayloadClearsPaneScrollback`, a direct proof that the `ED 3` backstop takes effect against a real multiplexer rather than being a silent no-op.
  Resolve tmux via `tmuxBinaryPath`, which skips the test on a machine without it.
  Write a payload file into `t.TempDir()` whose contents are fifty distinct, greppable junk lines followed by `headerBlockingPayload("hub: <the temp dir>")`, then start a private harness tmux server on a test-unique `-L` socket with a detached `new-session` whose shell command is `sh -c 'cat <payload file>; sleep 300'`, and tear the server down with `reapHarnessServer` in a `t.Cleanup`.
  Poll with `capturePaneScrollback` until the header line appears, then assert the captured full scrollback contains the header line and contains **none** of the fifty junk lines.
  On failure, print the whole capture so the failure is diagnosable from the output alone.
  Give the file a header comment stating that this first test proves `ED 3` actually clears scrollback on this host's multiplexer — the one thing the composite backstop can never show, since it goes green either way once the source fixes land — and that the same claim on a Windows/psmux host is asserted, not verified, because this worktree cannot execute a Windows run.
- **Commit:** `test(reedcli): prove the header payload clears real pane scrollback`

### Card 15: assert the live header pane's scrollback is clean end to end

- **Context:**
  - `internal/reedcli/smoke_test.go`
  - `internal/reedcli/smoke_lifecycle_test.go`
  - `internal/reedcli/smoke_headerseed_test.go`
  - `internal/reedcli/header_test.go`
  - `internal/reedengine/lifecycle_test.go`
  - `internal/reedengine/state.go`
  - `internal/hubforge/hub.go`
- **Edits:**
  - `internal/reedcli/smoke_headerscrollback_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `TestSmokeHeaderPaneScrollbackIsClean`, the composite backstop B.
  Build a dev-stamped binary via `buildLyxBinaryWithLDFlags` with the same `buildinfo.Channel=dev` ldflags card 7 uses, forge a hub with `hubforge.NewHub`, release it with `deferHubRelease`, and plant the same stale-but-untouched board stencil card 7 plants, so the arrangement that made the noise non-deterministic in the field is forced rather than hoped for.
  Run `lyx reed up` as a real subprocess of the built binary with its working directory set to `hub.PrimeWorktree()` — an in-process `RunCLI` cannot be used, because the header pane boots `os.Executable()` and that is the test binary.
  Read `HeaderPaneID` via `reedengine.LoadState`, poll until the rendered `hub: <hub path>` line appears, then capture the pane's entire scrollback with `capturePaneScrollback` and assert it holds that header line and no other non-empty line.
  Then assert the same property survives two further transitions in the same test: after running `lyx reed resume` as a subprocess, and after a heal — `kill-pane` the header pane through tmux directly, run `lyx reed up` again, re-read the fresh `HeaderPaneID` from state, and capture again.
  The heal path is the one most likely to regress, because it re-runs the same launch code from a different entry point.
  Tear the session down in a `t.Cleanup` the way `TestSmokeHeaderPaneDisplaysRenderedHeaderText` does.
  Every assertion must name the offending captured line in its failure message, since the original bug was only ever caught by an operator eyeballing a pane.
  Extend the file's header comment to state plainly that this second test pins the **composite** end-to-end outcome and **none** of the individual fixes, because `ED 3` runs after everything else and would keep it green if a source fix regressed — P1, P2, and P3 are the pins for the three source changes, and they live in `internal/reedengine/lifecycle_test.go`, `internal/reedcli/smoke_headerseed_test.go`, and `internal/reedcli/header_test.go` respectively.
- **Commit:** `test(reedcli): assert the live header pane's scrollback is clean end to end`

## Batch Tests

`verify:` runs two commands.
The untagged half, `go test ./internal/reedcli/`, covers card 12's source change and card 13's P3 pin, and re-runs the package's existing untagged guards (`header_test.go`, `cli_test.go`) — the package is small and hermetic, so whole-package scope costs nothing here.
The tagged half, `go test -tags smoke -run 'TestSmokeHeaderPayloadClearsPaneScrollback|TestSmokeHeaderPaneScrollbackIsClean' ./internal/reedcli/`, runs exactly this batch's two new smoke assertions.
It is deliberately filtered: the unfiltered `-tags smoke` package drives real `claude` sessions and real transcript persistence, which no per-round verify loop can afford.
Both filtered tests skip cleanly on a machine without tmux, via `tmuxBinaryPath`.

New coverage this batch adds:

- `internal/reedcli/header_test.go` — `TestHeaderBlockingPayload`, the P3 pin, untagged and pure.
- `internal/reedcli/smoke_headerscrollback_test.go` — `TestSmokeHeaderPayloadClearsPaneScrollback`, the direct `ED 3` efficacy proof, and `TestSmokeHeaderPaneScrollbackIsClean`, the composite backstop B covering boot, resume, and heal.

Existing coverage that must pass unchanged: `internal/reedcli/header_test.go`'s `TestHeaderCmd_UseAndShort` and `TestHeaderCmd_BlockingFlagRegistered`, and — outside this batch's verify scope but not to be broken — `internal/reedcli/smoke_lifecycle_test.go`'s `TestSmokeHeaderPaneDisplaysRenderedHeaderText`, whose `capture-pane` viewport assertion is unaffected because `capturePane` is not edited by this task.

At the end of the task, run the untagged suite and then the `smoke` tag explicitly, as the discussion's evidence discipline requires.
A green untagged run is not sufficient evidence for this task, and neither is B going green — B cannot fail for the reasons this task cares about once `ED 3` exists.
