# tmux (psmux on Windows) + claude TUI behavior -- empirical findings

> Provenance: copied from `millhouse/doc/psmux-tui-behavior.md` (sessions
> `replace-claude-p-with-psmux`, `smoke-test-psmux`). 100% relevant to the Loomyard mux
> design — it is the prior empirical record of driving a real `claude` TUI inside a
> psmux pane on this machine. The mux design and [`mux-exploration.md`](mux-exploration.md)
> draw on it directly. Bug-table line numbers refer to millhouse's Python implementation.

Observed 2026-05-31. Session: `replace-claude-p-with-psmux`.
Updated 2026-06-01. Session: `smoke-test-psmux`.
Terminal: `tmux new-session -x 220 -y 60 -- C:/Code/tools/powershell7/pwsh.exe` (on Windows, via psmux)
Claude version: 2.1.158 (initial), 2.1.159 (smoke-test run), Sonnet 4.6.

---

## Shell startup

`tmux new-session -- pwsh` (on Windows via psmux) resolves `pwsh` via PATH. On this machine,
`C:\Users\hanf\AppData\Local\Microsoft\WindowsApps\pwsh.exe` is a 0-byte App
Execution Alias stub. The session spawns but the shell is dead. Any `send-keys`
go into a dead pane; `_wait_for_marker_in_pane` polls until timeout (60s) and
returns False -- which causes the hang described in the proposal.

Working path: `C:/Code/tools/powershell7/pwsh.exe`.

Fix: add `llm.claude.tmux.shell_path` config key (default `pwsh` on Windows, `bash` on POSIX).

---

## Boot / idle screen layout (60-row terminal)

```
 ▐▛███▜▌   Claude Code v2.1.158
▝▜█████▛▘  Sonnet 4.6 with high effort · Claude Max
  ▘▘ ▝▝    C:\...

────────────────────────────────────────────────────────────────────────────────
❯ Try "fix lint errors"
────────────────────────────────────────────────────────────────────────────────
  ? for shortcuts · ← for agents
```

Key markers:
- `❯` input prompt is ALWAYS present in the separator sandwich -- at boot,
  during processing, and when idle. Do NOT use `❯` as an idle signal.
- Status bar (last line): `? for shortcuts · ← for agents` when idle.

---

## Processing screen layout

```
❯ <submitted message text>

✽ Concocting…(6s · ↓261 tokens)
 ⎿  Tip: Ask Claude to create a todo list...

────────────────────────────────────────────────────────────────────────────────
❯
────────────────────────────────────────────────────────────────────────────────
esc to interrupt
```

Key markers:
- Spinner line: `✽ Verb…(Xs · ↓N tokens)` or `· Verb…(...)`. Spinner character
  alternates `✽` / `·`; verb varies (`Concocting`, `Hashing`, `Crunching`,
  `Brewing`, etc.).
- `⎿  Tip: ...` line always appears below the spinner.
- Status bar: `esc to interrupt`.
- `❯` is still present in the separator sandwich (empty, no text).

The current `_wait_for_idle_stable` checks for ANY line starting with `❯`. Since
`❯` is present in all states, this function ALWAYS returns True on the second
poll -- it never actually waits for the response to finish.

---

## Idle screen layout (after response)

```
❯ <submitted message text>

● <response text line 1>
<response continues...>

✻ Verb for Ns

────────────────────────────────────────────────────────────────────────────────
❯
────────────────────────────────────────────────────────────────────────────────
  ? for shortcuts · ← for agents
```

Key markers:
- `● ` (bullet + space): prefix on the FIRST line of Claude's response.
  **Note (2026-06-01):** the space after `●` may be non-ASCII on this machine
  (same non-ASCII-space issue as the status bar). Use `startswith("●")` rather
  than `startswith("● ")` when detecting the bullet in capture output, then
  strip the bullet char and any following whitespace with `.lstrip()`.
- Completion marker: `✻ Verb for Ns` where Verb varies (`Cogitated`,
  `Crunched`, `Brewed`, `Churned`, `Cooked`, etc.).
- Separator line: a row of `─` characters (`─`).
- Status bar: `? for shortcuts · ← for agents`.

Occasionally the TUI fills the input area with an auto-suggested follow-up:
```
────────────────────────────────────────────────────────────────────────────────
❯ show an example using a generator as a context manager
────────────────────────────────────────────────────────────────────────────────
```
This happens after some responses. It means session reuse must CLEAR the
input area before submitting the next prompt (send Escape or Ctrl+C).

---

## Reliable idle detection

Replace `_wait_for_idle_prompt` and `_wait_for_idle_stable` with status-bar checks:

```python
def _is_idle(capture: str) -> bool:
    """Return True if the capture shows the idle status bar."""
    for line in capture.splitlines():
        if "shortcuts" in line:
            return True
    return False

def _is_processing(capture: str) -> bool:
    """Return True if the capture shows the processing status bar."""
    for line in capture.splitlines():
        if "esc to interrupt" in line or "esctointerrupt" in line:
            return True
    return False
```

**IMPORTANT -- non-ASCII spaces (discovered 2026-06-01):** psmux (Windows tmux port) alternate-screen
capture on Windows emits the status bar with non-ASCII space characters between
words (not U+0020 ASCII spaces). When decoded with `encoding="utf-8", errors="replace"`,
these become U+FFFD replacement chars. The result in Python is `"?forshortcuts??foragents"`,
NOT `"? for shortcuts · <- for agents"`. ASCII word tokens (`forshortcuts`,
`shortcuts`, `foragents`) are preserved intact.

Consequence: `"for shortcuts"` (ASCII space) NEVER matches. Use `"shortcuts"` as
the sole idle marker -- it is unique to the idle status bar, absent from
processing-screen text and response content.

The `_is_processing` fallback `"esctointerrupt"` was already added for the same
reason; the pattern is now consistent across both functions.

Two-phase response wait:

1. After sending prompt, wait up to ~10s for `_is_processing()` to become True.
   (Guards against reading a stale capture before the spinner appears.)
2. Then wait (with long timeout) for `_is_idle()` to become True for two
   consecutive polls 1s apart.

Both phases use `capture_pane(..., alternate=True)`. No `-S` scrollback needed
for alternate screen -- psmux always returns exactly `rows` lines for it.

Boot wait (`_wait_for_idle_prompt`): same `_is_idle()` check works, since the
boot screen shows the shortcuts status bar immediately.

---

## Response extraction

Current `extract_response` in `_psmux_capture.py` finds the LAST `❯` line
as the upper boundary and searches backwards for the LAST `● ` as the lower
boundary. This correctly identifies the most recent response.

Problem: the extracted slice includes trailing garbage:
- `✻ Verb for Ns` (completion marker)
- Separator line (`────────...`)

Fix: after finding `bullet_idx`, walk forward but stop before the first
separator line or `✻ ` line. Concretely: the end of response content is the
last line (going forward from `bullet_idx`) that does NOT start with `✻` and
is NOT a separator (all `─` chars).

Alternatively: walk backwards from `idle_idx`, skip separator and `✻` lines,
use the first non-skip line as `content_end_idx`.

---

## Multi-line prompt submission

**`tmux paste-buffer` (via psmux on Windows 3.3.4) does NOT work with Claude's TUI.**
The buffer is loaded correctly (`psmux load-buffer` succeeds and `psmux show-buffer`
shows the content), but `psmux paste-buffer -t session -b buf` silently discards
the content -- Claude's input area is never updated. This was previously documented
as "Verified working" which was incorrect.

Bracketed paste mode (`\e[200~`...`\e[201~`) via `send-keys -l` also does not work:
Claude's TUI does not support bracketed paste mode, so embedded `\n` characters
still trigger submission even within the paste markers.

**Working approach for fresh sessions (multi-line)**: Write a PowerShell script
that reads the prompt from a file and passes it as a positional argument to claude.
PowerShell passes the multi-line string (including newlines) as a single argument.
Claude Code CLI accepts a `[prompt]` positional argument and processes the full
text as the initial user message.

```powershell
# wrapper-SESSION-run.ps1
$prompt = Get-Content -Raw 'C:\path\to\prompt.txt'
claude --model MODEL --tools "" --session-id UUID $prompt
```

Execute with: `psmux send-keys -t session ". 'C:\path\to\script.ps1'" Enter`

**Working approach for reuse sessions (single-line only)**: Send the prompt via
`psmux send-keys -l -t session "text"` then `psmux send-keys -t session Enter`.
Multi-line prompts via the reuse path are NOT supported -- each `\n` triggers
submission. Reuse is typically used for keep-alive scenarios where subsequent
prompts tend to be short.

---

## Parallel sessions

Three simultaneous sessions (`probe-a`, `probe-b`, `probe-c`) each running
their own `claude` TUI: all start and respond independently with no
cross-contamination. No psmux-side limitations observed at 3 sessions.

---

## Terminal size and response length

At 60 rows x 220 cols, a ~300-word response (generator explanation) fits
within the visible alternate screen. The `● ` bullet marker remains visible
when the response is fully rendered.

If responses grow beyond ~50 lines, the `● ` marker will scroll off the
top of the visible area and `extract_response` will raise `MarkerNotFoundError`.
For safety, use at least 100 rows when creating new sessions. Review prompts
can produce long responses.

---

## Alternate screen capture note

`psmux capture-pane -a -S -N -p` with large N does NOT return extra history
for the alternate screen -- it always returns exactly `rows` lines. The `-S`
flag is a no-op for alternate screen. Use `psmux capture-pane -a -p` (no -S).

---

## Config keys needed

In `mill-config.yaml` / template, under `llm.claude.tmux`:

| Key | Type | Default | Purpose |
|-----|------|---------|---------|
| `via_tmux` | bool | false | Route through tmux instead of `claude -p` |
| `shell_path` | str | `pwsh` | Shell binary for `new_session`. Use full path on Windows where PATH stub is broken. |
| `reuse_idle_timeout_s` | int | 10 | Seconds to wait for existing session to become idle before reuse fails. |

---

## Summary of bugs in current implementation

Bugs 1-4 were identified and fixed in commit `baff371f` (`replace-claude-p-with-psmux`).
Bugs 5-6 were discovered and fixed during `smoke-test-psmux` (2026-06-01).
Bugs 7-8 were discovered during integration test verification (2026-06-01).

| # | Location | Bug | Fix | Status |
|---|----------|-----|-----|--------|
| 1 | `millpy-claude-sub.py:213,237` | `shell_argv=["pwsh", ...]` resolves broken PATH stub | Read `shell_path` from config | Fixed (baff371f) |
| 2 | `millpy-claude-sub.py:115-152` | `_wait_for_idle_prompt` and `_wait_for_idle_stable` use `❯` which is ALWAYS present | Use status bar text `"for shortcuts"` instead | Partially fixed -- see bug #5 |
| 3 | `_psmux_capture.py:extract_response` | Extracted text includes `✻ Verb for Ns` and separator line | Walk backwards from idle prompt, skip `✻` and separator lines | Fixed (baff371f) |
| 4 | `millpy-claude-sub.py` | No input-area clear before reuse; auto-suggest text from previous response leaks into next prompt | Send Escape before submitting on reuse path | Fixed (baff371f) |
| 5 | `millpy-claude-sub.py:_wait_for_idle_prompt,_wait_for_idle_stable` | Status bar captured with non-ASCII spaces; `"for shortcuts"` (ASCII space) never matches | Use `"shortcuts"` (no space) as idle marker | Fixed (smoke-test-psmux) |
| 6 | `_psmux_capture.py:extract_response` | `bullet_prefix = "● "` may fail if non-ASCII space follows `●` in capture | Use `startswith("●")` and strip bullet + whitespace separately | Fixed (smoke-test-psmux) |
| 7 | `millpy-claude-sub.py` | `psmux paste-buffer` silently drops content on Windows -- Claude TUI never receives the prompt; Phase 1 detects "esctointerrupt" (no match due to non-ASCII spaces) and times out after 60s; Phase 2 finds boot-screen "shortcuts" and returns prematurely, capturing boot state without `●` bullet | Use PS script (`Get-Content` + positional arg) for fresh sessions; `send_keys` for reuse; fix Phase 1 marker to `"interrupt"`; reduce Phase 1 timeout to 15s | Fixed (2026-06-01) |
| 8 | `doc/psmux-tui-behavior.md` | "Verified working" claim for `psmux paste-buffer` bracketed paste was incorrect: `paste-buffer` silently discards content on Windows psmux 3.3.4; Claude TUI does not receive it | Document the limitation; use PS script instead | Fixed (2026-06-01) |

---

## Verified non-issues (from proposal's secondary suspects)

- `--tools ""` flag: valid. `millpy-claude-sub.py` uses `["--tools", ""]` for
  bulk mode; the claude CLI accepts both `--tools` and `--allowedTools`.
- Boot sleep (1s after `new_session`): sufficient on this machine even with
  Cortex XDR scanning. `CLAUDE_READY` probe returned within 1s.
- `send_keys("Enter", enter=False)`: sends `psmux send-keys -t session Enter`
  which psmux interprets as the Enter key (special key name, not literal text).
  Confirmed working.
- Unicode `❯` codec (2026-06-01 clarification): `capture_pane` returns UTF-8;
  `❯` (U+276F) decodes correctly and `startswith("❯")` compares correctly.
  However, the CHARACTER AFTER `❯` (the space before the suggestion text) is a
  non-ASCII Unicode space that decodes as `?` (U+FFFD). The ASCII-encoded debug
  output showed `??Try...` -- the first `?` is the non-ASCII space, NOT `❯`
  being garbled. `❯` itself is fine. The non-ASCII-space problem is specific to
  status bar text between words (see bug #5).

---

## pipe-pane: does NOT work on Windows (tmux 3.3.4)

`tmux pipe-pane -t session "cat >> logfile"` (on Windows via psmux) and the tmux command
both return exit 0 but pipe no data. Tested with psmux (Windows tmux port) and tmux, with bash
session and pwsh session, with multiple path formats and shell wrappers. Files
are created (0 bytes) or not created at all. This is a known limitation of
Windows ports of tmux -- the pipe mechanism requires OS-level pty forking that
these ports do not fully implement.

**Architectural implication:** streaming output to file via `pipe-pane` is not
available on this machine. The "stream to file then framework reads it"
architecture described in the long-term vision requires an alternative
implementation.

**Working alternative -- polling differ:**
A background Python process polls `capture-pane` every ~0.5s, diffs each
capture against the previous one, and appends genuinely new lines to a log
file. Any downstream consumer (Slack bot, file watcher) tails that file.

```python
# Sketch of polling differ
prev_lines = set()
while True:
    capture = _psmux.capture_pane(session, alternate=True)
    new_lines = [l for l in capture.splitlines() if l.strip() and l not in prev_lines]
    if new_lines:
        with open(logfile, "a") as f:
            f.write("\n".join(new_lines) + "\n")
    prev_lines = set(capture.splitlines())
    time.sleep(0.5)
```

Caveats: does not capture lines that scroll off the visible viewport between
polls; may have ordering issues for rapid output. Suitable for the current
mill use case (responses complete before the next poll cycle) but not for
true real-time streaming.

**Longer-term option:** run the session inside WSL where real tmux `pipe-pane`
works. Then the log file is a genuine VT100 byte stream that requires
ANSI-stripping before use.

---

## capture-pane latency

Measured on this machine (20 consecutive calls, claude TUI active in session):

```
min:  19.6ms
max:  26.9ms
avg:  22.9ms
```

At a 0.5s polling interval, capture-pane overhead is ~4-5% of wall-clock.
Not a bottleneck. A poller daemon running at 500ms intervals is practical.

**Go implementation note:** for the long-term Slack streaming framework, Go
is the right language. One goroutine per tmux session, ticker at 500ms,
`exec.Command("tmux", "capture-pane", ...)` (or `psmux` on Windows), diff logic, Slack webhook push.
23ms per call is negligible. This is a separate submodule/repo from mill.
