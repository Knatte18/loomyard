# Logging inventory (read-only)

## Method and caveats

- Sources: trace logs in `_board`, `loomyard`, `go-driver-assistant`, `run-window-bugs`, `fan-lab-weft` (`.lyx/logs/*.log`).
- The `*-weft/.lyx/logs` dirs of loomyard, go-driver-assistant and run-window-bugs hold the same files as their pair dirs (identical line counts), so they were counted once.
  Naive glob total is 34194 lines; deduplicated it is 30758 lines, of which 30111 carry `level=` (the rest are the `command=` header lines, 647 trace files).
- Volume is 8.8 MB. WARN 26897 (89.3%), INFO 3214 (10.7%), DEBUG 0, ERROR 0.
- One file dominates: `_board/.lyx/logs/trace-20261008T181526Z-f2e3f7df172f3d30-3854.log` has 26486 lines, 7.7 MB (a `reed watchdog` process).

## 1. Top 30 by count (share of 30111 level lines)

| # | Count | Share | Level | Message |
|---|---|---|---|---|
| 1 | 25234 | 83.80% | WARN | reed: resize re-apply failed |
| 2 | 1104 | 3.67% | WARN | claudeengine: could not read the session name |
| 3 | 681 | 2.26% | INFO | fabric: mutation |
| 4 | 494 | 1.64% | INFO | orch: injection held |
| 5 | 415 | 1.38% | WARN | battenshed: producer stuck |
| 6 | 324 | 1.08% | INFO | reed: watchdog lock already held, exiting |
| 7 | 189 | 0.63% | INFO | battenshed: session state at notice |
| 8 | 179 | 0.59% | INFO | boardengine: spawning detached board sync |
| 9 | 152 | 0.50% | INFO | shuttle: session state |
| 10 | 117 | 0.39% | INFO | reed: promoting resize watchdog to signal mode |
| 11 | 98 | 0.33% | INFO | webster: handle bind |
| 12 | 79 | 0.26% | WARN | orch: session state disagrees with the idle probe |
| 13 | 55 | 0.18% | INFO | shuttle: run started |
| 14 | 55 | 0.18% | INFO | claudeengine: resolved the prompt-cache TTL |
| 15 | 48 | 0.16% | INFO | shuttle: run finished |
| 16 | 33 | 0.11% | INFO | shed: step |
| 17 | 33 | 0.11% | INFO | shed: loop child process started |
| 18 | 31 | 0.10% | INFO | shed: step done |
| 19 | 31 | 0.10% | INFO | shed: loop child exited |
| 20 | 30 | 0.10% | INFO | shuttle: turn end waiting on background work |
| 21 | 29 | 0.10% | INFO | battenshed: inner shed run wait complete |
| 22 | 29 | 0.10% | INFO | battencli: spawning loom session |
| 23 | 29 | 0.10% | INFO | battencli: loom session wait complete |
| 24 | 27 | 0.09% | WARN | gitrepo: push refused, retrying |
| 25 | 26 | 0.09% | INFO | orch: session start signal read |
| 26 | 26 | 0.09% | INFO | orch: compaction boundary found |
| 27 | 23 | 0.08% | INFO | gate test: spawning go test |
| 28 | 23 | 0.08% | INFO | gate test: go test ended |
| 29 | 23 | 0.08% | INFO | gate test: CPU watchdog started |
| 30 | 23 | 0.08% | INFO | gate test: CPU watchdog decision |

The top 12 are 98.6% of all lines; rank 13 and below are each under 0.2% and are lifecycle lines that the policy wants.

## 2. Call sites and verdicts

Policy applied (`internal/logger/logger.go:123-140`): Warn is a notable-but-recoverable failure; Info is a real process spawn/teardown, a recorded durable state mutation or a shed step boundary; Debug is everything else; no Warn in a loop that can iterate more than about 10 times without a state change.
`PATTERN.md:85` and `pattern/PATTERN-spawn-observability.md:4` add: Info for a lifecycle spawn, Debug for a spawn inside a polling probe.
PATTERN.md has no other logging rule.

1. **reed: resize re-apply failed** - `internal/reedengine/watchloop.go:437`. Verdict (c) and (a Warn-in-loop violation): it is a Warn on every failing tick, with no dedupe and no back-off.
   The same error text (`select-layout: have 3 panes but need 2`) repeats thousands of times per session (4527 / 4195 / 3453 per session for the top three).
   Log once per distinct error (or per state change) and count repeats; the abandon line at `watchloop.go:439-441` already shows the intended once-per-outage shape, and the dormant branch at `watchloop.go:415-421` states that same rule.
2. **claudeengine: could not read the session name** - `internal/shuttleengine/claudeengine/sessionname.go:28` and `:40`. Verdict (c) plus (b): all 1104 lines are the `path=` variant, a transcript not yet written, which the doc comment at `sessionname.go:21-23` calls an expected state; it is polled.
   Log once per session id at Debug, or Warn only when the file is absent past a deadline.
3. **fabric: mutation** - `internal/fabricengine/mutation.go:158` and `:216`. Verdict (a): the policy names recorded durable mutations as Info.
   Kinds: commit_created 161, branch_pushed 147, file_written 85, path_removed 52, link_created 42, link_removed 37.
   `file_written` and `path_removed` may be the weakest candidates for Debug; otherwise right.
4. **orch: injection held** - `internal/orchengine/watcher.go:535` (the sibling `held by the session` is at `:466`). Verdict (c): doc says once per change of reason, but 409 of 494 carry `reason="the input box holds a draft: "` followed by the draft text, which varies, so the reason string changes per keystroke and defeats the dedupe.
   Key the dedupe on the reason class, not the string; also see section 4.
5. **battenshed: producer stuck** - `internal/battenshed/stuck.go:48`, called from `innerrun.go:690` (and `create.go:61,74`, `seamchild.go:77-123`, `land.go:138`, `teardown.go:106,122`). Verdict (b) and (c): "inner shed run still running; checking it every 2s" is a normal wait, not a failure, yet it is Warn and repeats each poll; "paused", "blocked", "waiting" and "automatic Publish verify retry waits" are the same.
   Make the polled waiting reasons Debug (or Info on change); keep Warn for real blocks. Dedupe on (producer, reason).
   `internal/landingshed/stuck.go:21` has the same shape.
6. **reed: watchdog lock already held, exiting** - `internal/reedcli/watchdog.go:705`. Verdict (b): 324 short-lived processes each write a whole trace file (header plus this line) to exit on contention, which the code comment calls "one short-lived process and nothing else".
   Debug; better, avoid opening a durable trace file at all on this path (the sink is set at `watchdog.go:~690` before the lock is tried).
7. **battenshed: session state at notice** - `internal/battenshed/notice.go:413`, wrapper `:408-415`. Verdict (d) or Debug: the doc comment says "shadow logging: no notice, stamp or finding depends on the state fields"; one line per run per notice, and notices repeat.
8. **boardengine: spawning detached board sync** - `internal/boardengine/spawn.go:31`. Verdict (a): a real process spawn at Info per policy. 179 lines is volume from `board upsert/get`; keep.
9. **shuttle: session state** - `internal/shuttleengine/waitshadow.go:160`. Verdict (a): already logs on state change only (`hasLogged` and name/cause compare at `:157`). Shadow state, so Debug would also be defensible.
10. **reed: promoting resize watchdog to signal mode** - `internal/reedengine/watchloop.go:468`. Verdict (a): a state change, but 117 lines against 325 watchdog processes means the mode restarts per process; fine as Info, or Debug.
11. **webster: handle bind** - `internal/websterengine/recordbatch.go:778` and `:781`. Verdict (c): one line per handle, built to be "countable by grep" (`:773`). A per-batch summary line (bound N, failed M, with failed handles listed) would carry the same content.
12. **orch: session state disagrees with the idle probe** - `internal/orchengine/watcher.go:619`. Verdict (c): Warn on a probe disagreement that is observation of a shadow classifier, recoverable and expected during transitions; dedupe on (state, idle) change, and Debug unless it persists.
13. **shuttle: run started / run finished** - `internal/shuttleengine/wait.go:1615` (finished). Verdict (a): lifecycle.
14. **claudeengine: resolved the prompt-cache TTL** - `internal/shuttleengine/claudeengine/claudeengine.go:110`. Verdict (d)/Debug: a computed config value, not a spawn or mutation; equal for most runs.
15. **shed: step / step done / loop child process started / loop child exited** - `internal/shedverbs/loopchild.go:38`. Verdict (a): the policy lists shed step boundaries and process spawn/teardown.
16. **shuttle: turn end waiting on background work** - `internal/shuttleengine/wait.go:910`. Verdict (c) if inside the wait loop; check it logs on change only (30 lines, low urgency).
17. **battenshed: inner shed run wait complete, battencli: spawning loom session, loom session wait complete** - Verdict (a): process lifecycle.
18. **gitrepo: push refused, retrying** - `internal/gitrepo/pushretry.go:87`. Verdict (a): a bounded retry (`pushMaxAttempts`), exactly the Warn case; 27 lines.
19. **orch: session start signal read, compaction boundary found** - `internal/orchengine/watcher.go:598`, `:270`, `:1259`. Verdict (a): once per new time/boundary, per doc.
20. **gate test: spawning go test, go test ended, CPU watchdog started, CPU watchdog decision** - `internal/gatecli/run.go:110` (decision). Verdict (a) for spawn/end; "CPU watchdog started" and "decision" are Debug candidates, 4 lines per gate run.

Also seen at lower counts: `configengine: degrading to embedded template` (15, `internal/configengine/config.go:189,260`; probably (a)), and `shuttle: could not type the segment color` (WARN, 3 lines over 1500 chars, error is a retryable "a turn is running" - (b), Debug).

## 3. Test noise

- No `.lyx/verify/verify-*.log` exists in any pair or anywhere under `/home/knatte/Code/loomyard-LYXHUB` (searched by name; the files are named by `internal/verifytree/verify.go:37,231`), so the counts you asked for could not be taken, and no test was run.
- No trace file comes from a test binary either: `command=` header lines are only real `lyx` invocations (reed watchdog 325, board upsert 110, shed step 41, board get 36, batten run 26, gate test 23, ...).
- Static view instead. Logger default is stderr at Warn (`internal/logger/logger.go:344-346`), so tests leak Warn (and Error) lines, never Info/Debug, unless `LYX_LOG_LEVEL` is set. `logger.SetOutput` (`logger.go:435`) is the discard seam.
- 56 `SetOutput`-style calls exist in tests. 38 packages that contain `logger.Warn(` or `logger.Error(` call sites have zero test files that call `SetOutput`, among them: shuttleengine (34 test files), loomcli (77), shedadapters (32), battencli (29), landingshed (24), loomshed (23), gitrepo (23), planglyph (20), loomengine (18), shedverbs (17), reedcli (26), orchengine (13), webstercli (13), boardengine (15), fabriccli (15), configengine (6).
- 355 `TestMain` functions are in use (e.g. `internal/verifyrun/testmain_test.go`, via `tmuxkit.Main`). The tests can route all logging to discard with one `logger.SetOutput(io.Discard)` in `tmuxkit.Main` or a shared testkit helper called from TestMain, which covers every package at once; tests asserting log output keep their own `SetOutput`.
- To get real counts: after a run, `grep -o 'level=WARN msg="[^"]*"' verify-N.log | sort | uniq -c | sort -rn`; the likeliest leaks are the Warn sites of items 1, 2, 5, 12 and 24 above (reedengine watchloop, sessionname, stuck.go, gitrepo pushretry) because their tests drive the failing path on purpose.

## 4. Whole payloads and long fields

- **battencli: create worktree** - field `mutations=` is a Go `%+v` dump of the whole mutation struct (hub root plus every entry); lines exceed 1500 chars. Redundant with `fabric: mutation`, which already logs each entry. Log the count only.
- **orch: injection held** - `reason` embeds the draft text of the input box (`reason="the input box holds a draft: ..."`), 409 lines, user content in the trace. Log a constant reason plus the draft length.
- **battenshed: producer stuck** - reasons embed full `error="..."` strings and a parent-review sentence; two lines exceed 1500 chars.
- **reed: resize re-apply failed** - `err=` carries the full tmux layout string; 26486-line file is 7.7 MB because of it.
- **claudeengine: could not read the session name** - full transcript path plus the wrapped open error repeats the path twice.
- **shuttle: could not type the segment color** - full wrapped error with strand GUID, over 1500 chars on 3 lines.
- **shuttle: turn end waiting on background work** - `outstanding` joins all task descriptions with `; `.
- **webster: handle bind** - one line per handle with card, handle and glyph (handles repeat as glyph).
- Longest line overall is 5150 chars; only 5 lines exceed 1500.

## Estimated removal

- Fixing items 1 (once per distinct error), 2, 4, 5, 6, 7 removes about 27450 of 30111 level lines, roughly 91% (1: ~25200, 2: ~1100, 4: ~400, 5: ~300, 6: 324, 7: 189, less a few dozen kept).
- Excluding the single 26486-line `_board` file, 4877 level lines remain across the other 646 traces; the same fixes remove about 2200 of them, roughly 45%. The bytes share is larger because the resize lines are long.
- Adding the Debug demotions (items 8-11, 14, 20) would take out another 400 or so (about 1.5% of all, about 10% of the remainder).
