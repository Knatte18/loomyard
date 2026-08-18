# shuttle — independent review, round 3 (`opus-medium-r3`)

Reviewer: Opus 5, medium effort. Clean-room: findings below were formed before reading any
`_mill/shuttle-review-*` file or `_mill/reed-shuttle-HANDOFF.md`.

Primary mandate this round: **reed×shuttle joint adversarial testing** — real live shuttle runs
(`--model haiku`) with reed-side hardened failure modes triggered MID-FLIGHT.

## Environment / baseline

- Host: Linux, tmux 3.6 (`/usr/bin/tmux`), `claude` at `/home/knatte/.local/bin/claude`.
- Worktree: `/home/knatte/Code/loomyard/wts/reed-shuttle-crucible-hardening`, `AnchorRel = "."`.
- Baseline BEFORE any driving: `tmux: server` processes = **0**; `pgrep -xc claude` = **4**
  (four unrelated long-lived agent sessions, none tmux-hosted).
- Build: `./deploy-dev` @ `06719ef2`, `go build ./...` green.

## What was tested

(appended live, in order)

### Hermetic gates (before any driving)

- `go build ./...` — green.
- `go vet ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/shuttlecli/...` — green.
- `go vet -tags smoke ./internal/shuttlecli/...` — green.
- `go test -count=5 ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/shuttlecli/... ./cmd/lyx/...` — all `ok`.

### Live fixture

Hub `<scratch>/hub1` with worktree `svc-orig` (plain git repo, `AnchorRel = "."`), `lyx reed up` booted,
socket `lyx-hub1-e4bceb4c`. Every real `claude` pinned to `--model haiku`.
Long-running prompt used for every mid-flight scenario: "Use the Bash tool to run exactly `sleep 150` …
then write DONE to `<out>`", `--timeout 8m` — so the reed-side trigger lands while the run is genuinely
polling, not already finished.

### Scenario 1 — worktree rename DURING a live run (`Wait` is the reed caller)

1. `lyx shuttle run --prompt <sleep-150 prompt> --output-file <wt>/out.txt --model haiku --timeout 8m` (backgrounded, pid 3366435).
2. Polled `lyx reed status` until `live:true` (guid `bf1a871a…`); confirmed pane capture shows the
   Claude TUI actively working ("Prestidigitating… (7s)"); confirmed one real `claude` process (pid 3366453).
3. `mv <hub>/svc-orig <hub>/svc-moved` at 16:35:46.586.
4. `lyx shuttle run` EXITED at 16:35:52.714 — 6.1 s after the rename — with:

```
{"guid":"bf1a871a…","lastAssistantMessage":"","ok":true,
 "outcome":"died","runDir":"…/svc-orig/.lyx/shuttle/b7afdd76…","sessionId":"46cf10e9-…"}
```

5. `pgrep -xa claude` immediately afterwards: the agent process 3366453 was **still alive and working**.
6. `<hub>/svc-orig` had been RE-CREATED as a phantom directory containing only `.lyx/reed.lock` and
   `.lyx/reed.json.lock` (reed's `withOpLock` `MkdirAll` on the told anchor path, which no longer exists);
   the real `reed.json` — still naming session `svc-orig`, strand `bf1a871a…`, pane `%0` — travelled with
   the directory to `svc-moved`.

Verdict: **shuttle reported `ok:true, outcome:"died"` for an agent that was demonstrably alive.**
See finding R3-F1.

### Scenario 5 — out-of-process `interrupt`/`send` against a reed that is mid-refusal

Immediately after scenario 1's rename, from the RENAMED worktree (`svc-moved`), with the agent still live:

- `lyx shuttle interrupt bf1a871a…` → `ok:false`, error =
  `shuttle: check strand liveness: this worktree's reed state was recorded against tmux session "svc-orig",
  which is STILL RUNNING on socket "lyx-hub1-e4bceb4c" … Tear the old session down with "tmux -L … kill-session -t '=svc-orig'" …`
- `lyx shuttle send bf1a871a… "hello"` → identical refusal, `ok:false`.
- `lyx reed status` → the same refusal bare.

Verdict: **CLEAN composition.** Both out-of-process verbs inherit reed's foreign-session refusal verbatim,
report `ok:false`, name the session/socket/remedy, and destroy nothing. Nothing was silently addressed on the
wrong pane. This is the exact same underlying reed situation as scenario 1 and it is handled honestly here —
which is what isolates R3-F1 as a `Wait`-side classification defect rather than a missing reed guard.

Teardown after scenarios 1+5: renamed back, `lyx reed down` → `ok:true`; `tmux: server` count 0, `pgrep -xc claude` 4 (= baseline).

### Scenario 2 — `reed.json` corrupted / deleted DURING a live run

**2a — truncated mid-run.** Long-running run started, strand `42c4b9e2…` live, agent mid-flight;
`reed.json` truncated to `{"socket":"lyx-hub1-e4bce` at 16:38:30.9.
The run itself exited 6 s later on an ordinary `asking` classification (the agent had ended its turn),
so this attempt did not read the liveness path — but the out-of-process verbs, issued while the
corrupt file was in place and the agent still live, were clean:

- `lyx shuttle interrupt <guid>` → `ok:false`, `shuttle: check strand liveness: reed state file …/reed.json is
  unreadable: unmarshal state: unexpected end of JSON input — the tmux session it describes may still be running …`
- `lyx shuttle send <guid> …` → identical.
- `lyx reed status` → the same, bare.

Verdict: **CLEAN.** reed's `unreadableStateError` reaches the operator intact through shuttle's one-line
`check strand liveness:` prefix, nothing is destroyed, and the message names both remedies.

**2b — `reed.json` DELETED mid-run.** This is not a synthetic corruption: it is verbatim the second remedy
reed's own `unreadableStateError` recommends ("delete `<path>` by hand to keep the session (its panes and
their processes keep running, untracked)"), and it is also what a `git clean -xdf` does — a sanctioned
operator action under `CONSTRAINTS.md`'s Durable-vs-Ephemeral State Invariant.

1. Run started with a prompt holding ONE turn open for ~5 minutes (three blocking `python3 -c 'import time;
   time.sleep(100)'` Bash calls — note the `claude` CLI refuses standalone `sleep`, so this shape was needed).
   Strand `5f14bfe5…`, pane `%2`, live; pane capture confirmed the agent was 50 s into its third python wait.
2. `rm .lyx/reed.json` at 16:43:09.294.
3. `lyx shuttle run` EXITED at 16:43:15.204 — 5.9 s later — with
   `{"ok":true,"outcome":"died","guid":"5f14bfe5…","sessionId":"a9bab1ab-…","runDir":"…"}`.
4. Immediately after: `claude` pid 3379261 still alive; pane capture still showed
   `⎿ $ python3 -c 'import time; time.sleep(100); print(3)' (1m 10s)` — still in the SAME turn, still working;
   the output file was still absent 10 s after the "died" verdict.

Verdict: **second, cleaner reproduction of R3-F1**, with no rename involved and via an operator action reed
itself recommends.

Teardown after scenario 2: `reed.json` restored, `lyx reed down` → `ok:true`; tmux servers 0, claude 4 (baseline).

### Normal-flow confirmations gathered along the way (no defect)

- Happy path: 30-Bash-call run → `{"ok":true,"outcome":"done"}`, output file written, strand removed from
  `reed status`, run dir deleted, claude count back to baseline. R1-F1's model pin (`--model haiku`) honored.
- `asking` path: a run whose agent ended its turn without writing the output file → `{"ok":true,"outcome":"asking"}`
  carrying the real `lastAssistantMessage`, pane and run dir deliberately kept.

## Findings

(appended as spotted)

### R3-F1 — `Wait` classifies a LIVE agent as `died` when reed's strand table goes empty mid-run — MEDIUM, CONFIRMED, small

`internal/shuttleengine/wait.go:219-231` (`checkLivenessTick`).

`checkLivenessTick` derives `live` from a single linear scan of `status.Strands` and then treats the two
structurally different negative answers identically:

- the guid IS in reed's table and its pane is not alive → the run genuinely died;
- the guid is ABSENT from reed's table → reed's strand tracking was lost or reset under the run. This says
  nothing about the agent process, which is very often still running.

Both fall into the same `if !live { … return OutcomeDied }` branch.

Shuttle's own `requireLiveStrand` (`run.go:403-421`) already draws exactly this distinction one file over,
with two different messages — the absent case explicitly reads "not tracked by reed — … or reed's strand table
was reset under it (a reed remove/down, or a lost or rebuilt reed.json)". `Wait` does not.

Concrete failure (reproduced live, see Scenario 1): rename the worktree while a run is in flight. The in-process
reed engine keeps the geometry it was constructed with, so it never reaches reed's foreign-session refusal —
instead `withOpLock` re-creates a phantom `<old-anchor>/.lyx`, `LoadState` finds no `reed.json`, `Status`
succeeds with zero strands, and shuttle answers `ok:true, outcome:"died"` 6 seconds later while the agent works on.

The same shape is reachable without a rename, by an action the repo explicitly sanctions: `CONSTRAINTS.md`'s
Durable-vs-Ephemeral State Invariant makes `git clean -xdf` (which deletes `.lyx`) an ordinary operator action,
and reed's own round-4 header-rebuild retry exists precisely to recover from "a lost `.lyx/reed.json` while a
session stays up". Reed treats that state as recoverable; shuttle answers it with a false terminal outcome.

Why it matters more than a cosmetic mislabel: `outcome:"died"` is reported with `ok:true`, i.e. as a
successful classification. An unattended caller (loom, burler, a CI pipeline) reads "died" as "this agent is
gone, retry it" — and retrying spawns a SECOND live agent against the same worktree while the first keeps
running unreachably. That is the same duplicate-agent hazard reed's own R5-F5 refusal was built to prevent,
reintroduced one layer up.

Suggested fix: in `checkLivenessTick`, distinguish "found in the table" from "absent from the table".
A strand present-but-not-live keeps today's `OutcomeDied` (with the existing output-files-exist → `OutcomeDone`
race guard). A strand ABSENT from the table is a mechanism failure, not a classification: return an error so
`Wait` takes its `identity()` exit (guid/sessionId/runDir preserved, no cleanup, R1-F2's contract), with a
message in `requireLiveStrand`'s vocabulary — reed no longer tracks this strand, its agent may still be live in
its pane, check `lyx reed status`. Because it goes through `Wait`'s existing `statusFailures` retry counter, a
one-tick blip does not trip it.

Size: small — one function, plus a hermetic `wait_test.go` case against the existing `fakeReed`.
