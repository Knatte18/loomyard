# Batch: status-line-pins

```yaml
task: "Replace reed's header pane with a status-line and Selvage"
batch: "status-line-pins"
number: 4
cards: 4
verify: go test ./internal/reedengine/ && go test -tags integration ./internal/reedengine/
depends-on: [3]
```

## Batch Scope

This batch moves the identity text out of a pane and into tmux's own status-line: `pinGeometryOptionsLocked` flips from `status off` to `status on` plus `status-position bottom`, a rendered `status-left`, an empty `status-right`, a raised `status-left-length`, and an emptied window-status segment — with a new pure escaping helper in front of them, since `#` is tmux's format-expansion character inside a status string.
It is one batch because all seven `set-option` calls live in one function, share one escape-then-measure ordering that is load-bearing, and are read back by one existing readback (`readStatusRowsLocked`) whose behaviour this batch deliberately does not change.

The external interface batch 5 and 7 consume: `pinGeometryOptionsLocked` now renders the status-line on both the paths it already runs on — boot (`lifecycle.go`) and the attach pre-flight (`attach.go`) — so a session booted by an older lyx picks the new status-line up on the operator's next attach, exactly as the hook install already back-fills.

Batch-local decisions beyond `## Shared Decisions`: the attach chain's reserved-row accounting is **not** changed beyond the pin flip. `reservedRowsFromStatus` already maps `"on"` → 1 and `AttachArgv` already floors `reserved` at `rows-1`, and keeping the readback rather than hard-coding 1 is what makes an operator's multi-line status config degrade gracefully instead of mis-sizing every layout — and is also the mechanism that makes the Windows degrade self-correcting.
The pins are session/window-targeted, never global: a session- or window-scoped value from the operator's own `~/.tmux.conf` silently wins over a global set while `set-option` still exits 0, already live-verified for the existing pins.

## Cards

### Card 24: add the status-left escaping and length helpers

- **Context:**
  - `_mill/discussion.md`
  - `internal/reedengine/watchdog.go`
- **Edits:**
  - `internal/reedengine/windowsize.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add two pure, I/O-free helpers to `internal/reedengine/windowsize.go`, beside the existing `parseWindowSize`/`reservedRowsFromStatus`/`windowSizeAllowsChain` group. `escapeStatusText(s string) string` returns `s` with every `#` doubled to `##`, since tmux expands `#{…}` and `#[…]` inside a status string and a hub path containing `#` would otherwise be interpreted rather than displayed. `statusLeftLength(escaped string) int` returns `max(10, utf8.RuneCountInString(escaped))` — measured in **runes**, because tmux's `status-left-length` limit counts characters rather than bytes, and floored at 10 because that is tmux's own default, which would truncate. Document on `statusLeftLength` that it takes the already-escaped string, and state why that order is load-bearing: a hub path carrying `#` grows by one character per occurrence, so measuring the pre-escape string would truncate exactly those lines. Add the `unicode/utf8` import. Do not build these strings through `internal/shell` — the Shell Mechanics Seam governs pane-shell command strings, and a tmux option value is neither.
- **Commit:** `feat(reedengine): add the status-left escape and length helpers`

### Card 25: render the status-line from pinGeometryOptionsLocked

- **Context:**
  - `internal/reedengine/statusline.go`
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/watchdog.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/reedengine/windowsize.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedengine/windowsize.go`'s `pinGeometryOptionsLocked`, replace the `set-option -t <target> status off` call with the status-line block, leaving the `window-size latest` pin and the whole watchdog-hook half of the function untouched. The block calls `e.StatusLineText()` once; on error it logs via `logger.Warn` naming the socket, the session and the error, and skips the two text-derived options (`status-left` and `status-left-length`) while still issuing the other five — a template that fails to render is already refused loudly at boot by `ValidateStatusLine`, so reaching here means a degraded path, not a normal one. On success it computes `escaped := escapeStatusText(strings.TrimRight(text, "\r\n"))` and issues, each session/window-targeted on the same `target` the existing pins use and each with its own `logger.Warn`-and-continue on failure: `set-option -t <target> status on`; `set-option -t <target> status-position bottom`; `set-option -t <target> status-left <escaped>`; `set-option -t <target> status-right ""`; `set-option -t <target> status-left-length <statusLeftLength(escaped)>`; and, window-targeted with `-w` like the existing `window-size` pin, `set-option -w -t <target> window-status-format ""` and `set-option -w -t <target> window-status-current-format ""`. Suppressing the window-status segment is deliberate rather than left at tmux's default: reed's session has exactly one window, so the default `0:bash*` segment beside the identity text names nothing the operator can act on and would shift position as the window's active pane name changes. Write **no** `runtime.GOOS == "windows"` branch around any of these — per the `windows-status-line-is-an-unbranched-accepted-degrade` Shared Decision they are attempted on every platform and psmux may refuse them. Rewrite the function's doc comment: it today opens "pins this session's window to \"status off\" and \"window-size latest\"" and must instead describe the status-line render plus the `window-size latest` pin, keeping the session-vs-global paragraph, the both-paths (boot and attach pre-flight) paragraph, and the whole watchdog-hook paragraph verbatim. Also rewrite the file's own leading comment, which today reads "the two geometry option pins (status off, window-size latest)".
- **Commit:** `feat(reedengine): render the identity text into tmux's status-line`

### Card 26: correct the attach chain's status-off premise

- **Context:**
  - `internal/reedengine/windowsize.go`
- **Edits:**
  - `internal/reedengine/attach.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedengine/attach.go`, rewrite the comment above the `e.pinGeometryOptionsLocked()` call, which today reads "The ordering is load-bearing: the told box is only correct once \"status off\" has landed, since that is what makes the post-attach window equal the client's rows rather than rows - 1." The ordering is still load-bearing but for the opposite reason: the told box is only correct once the status-line pins have landed **and been read back**, because `readStatusRowsLocked` a few statements later is what turns whatever `#{status}` actually became into the reserved-row count the box is computed from. Also update the comment on the `readStatusRowsLocked` call, which today reads "A #{status} that reads back as something other than \"off\" does NOT suppress the chain" — with `status on` now the intended value, that sentence should state the rule positively: the reserved-row count is taken from whatever value reads back, and only an unrecognized value (`ok == false`) suppresses the chain. Change **no** logic in this file: `reserved` still comes from the readback, the `reserved > rows-1` floor stays, `box` is still `render.Box{X: 0, Y: 0, W: cols, H: rows - reserved}`, and both `anyPlacedStrand` guards stay. This is the whole of the `reserved-row-accounting-follows-the-pin` decision — the machinery for a status-line consuming rows is already written and tested, and the only thing that changes is which value the readback returns.
- **Commit:** `docs(reedengine): correct the attach chain's status-off premise`

### Card 27: pin the status-line options and the escaping rules

- **Context:**
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/attach.go`
  - `internal/reedengine/statusline.go`
  - `internal/reedengine/overlay.go`
- **Edits:**
  - `internal/reedengine/windowsize_test.go`
  - `internal/reedengine/attachgeometry_integration_test.go`
  - `internal/reedengine/attach_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedengine/windowsize_test.go`, add a table-driven test for `escapeStatusText` covering a string with no `#`, one `#`, several `#`, and a `#` at each end; and a table-driven test for `statusLeftLength` covering a string shorter than 10 runes (expects 10), exactly 10, longer than 10, and a multi-byte string whose rune count differs from its byte count (expects the rune count). Add one asserting the escape-then-measure order: for a string whose escaped form crosses the 10-rune floor only because of the doubling, `statusLeftLength(escapeStatusText(s))` exceeds `statusLeftLength(s)`. Add a test driving `pinGeometryOptionsLocked` against `TmuxCmd`'s `execHook` seam that records every issued argv and asserts the seven options are issued with the expected targets and values, that `status-left` carries the escaped rendered text, and that a `StatusLineText` error skips only `status-left` and `status-left-length` while the other five are still issued. Assert too that no call's failure stops the calls after it — script the hook to fail one option and assert the remainder are still attempted, and that `pinGeometryOptionsLocked` returns nothing and surfaces no error. Update every existing assertion in the file that expects `status off`. Retarget `internal/reedengine/attachgeometry_integration_test.go`'s expectations for the flipped pin, keeping its subject — the told box equals `rows - reserved` where `reserved` comes from the `#{status}` readback — unchanged, since that is precisely the self-correcting property the Windows degrade rests on.
- **Commit:** `test(reedengine): pin the status-line options and the escape-then-measure order`

## Batch Tests

`verify: go test ./internal/reedengine/ && go test -tags integration ./internal/reedengine/` covers the one package this batch edits, at both the tier that owns the new pure helpers (`windowsize_test.go`, table-driven, no tmux) and the tier that owns the attach chain's end-to-end reserved-row behaviour (`attachgeometry_integration_test.go`).
The `execHook` seam on `TmuxCmd` is what makes the seven new `set-option` calls assertable without a live server, so the option-issuing test stays in the untagged tier where it belongs — it spawns no process and needs no tmux.
The batch deliberately adds no test asserting what psmux does with these options: that is an unverified open item this plan carries into the shipped design doc (card 47) rather than a claim any test here can make, and the Windows half of the status-line smoke — asserting the self-correcting reserved-row property rather than the option values — is written in batch 7 with the rest of the smoke tier.
