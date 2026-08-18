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

### Scenario 3 — tmux SERVER killed DURING a live run

Long-turn run live (strand `8636c54b…`, agent 38 s into its first python wait).
`tmux -L lyx-hub2… kill-server` at 16:45:27.292 → the run EXITED at 16:45:39.314 (12.0 s, i.e. two
liveness ticks) with:

```
{"ok":false,"guid":"8636c54b…","sessionId":"41506282-…","runDir":"…",
 "error":"shuttle: reed status failed 2 times consecutively: reed status: no reed session (1 strands persisted); run \"lyx reed resume\" to rebuild, or \"lyx reed up\" for a bare substrate"}
```

Verdict: **CLEAN.** Prompt detection (no hang against a dead server), honest `ok:false` mechanism failure
rather than a fabricated classification, R1-F2's identity triple intact, and reed's own remedy text passed
through. Teardown after: tmux servers 0, claude 4 (baseline).

### Scenario 6 — `lyx reed down` + `lyx reed up` WHILE a run believes its pane exists

Long-turn run live (strand `622ddcf4…`, pane `%0`). At 16:46:57.3 a second process ran `lyx reed down`
immediately followed by `lyx reed up` (complete in 0.9 s — a full session rebuild inside one liveness tick).

- `lyx reed status` after the cycle: session up, `strands: []`.
- `lyx shuttle interrupt 622ddcf4…` → `ok:false`,
  `shuttle: strand "622ddcf4…" is not tracked by reed — either its run completed and was cleaned up, or reed's
  strand table was reset under it (a reed remove/down, or a lost or rebuilt reed.json); check "lyx reed status"`
- `lyx shuttle send …` → identical.
- The in-flight run exited 8.5 s later with `{"ok":true,"outcome":"died", …}`.

Verdict: **no wrong-pane addressing** — the rebuilt session's fresh `%0` was never mistaken for the run's pane
(reed's `PaneGeneration` clear does its job and shuttle inherits it correctly), and the out-of-process verbs are
honest. The `died` here happens to be factually correct (the `down` killed the agent), but it is produced by the
same undiscriminating branch as scenarios 1 and 2b — this scenario is what makes the asymmetry visible in one
frame: for the SAME reed state ("guid absent from reed's table"), `interrupt`/`send` say "reed's strand table was
reset under it" while `Wait` says "died".

### Scenario 4 — cross-worktree contamination under REAL concurrent live agent load

Hub `<scratch>/hub2`, worktrees `wt-a` and `wt-b`, both `reed up` on the shared socket `lyx-hub2-2fbe4ed5`.
Two long-turn runs started simultaneously — **exactly 2 real `claude` processes** (`pgrep -xc claude` = 6 = baseline 4 + 2),
the round's authorized ceiling. `wt-a` → strand `880a882f…` pane `%0`; `wt-b` → strand `d3c405c5…` pane `%2`.

Both confirmed mid-flight, then at 16:48:35.4: `cp wt-a/.lyx/reed.json wt-b/.lyx/reed.json` — reed's own
R5-F4 shape, now with two live agents rather than fixtures.

- `wt-b`: `lyx reed status` → `ok:false`, the foreign-session refusal naming `wt-a`, the socket, and the
  `kill-session` remedy.
- `wt-b`: `lyx shuttle interrupt <wt-a's guid>` → `ok:false`,
  `shuttle: "880a882f…" is not a shuttle strand: shuttle: no run found for strand "880a882f…"` —
  **shuttle's OWN `FindRun` guard refuses before reed is touched at all**, because `wt-b`'s run-dir root holds no
  such run. Same for `send`.
- `wt-b`'s in-flight run exited 9.4 s later with `ok:false`, reed's refusal verbatim, and **`wt-b`'s own**
  guid/sessionId/runDir — not `wt-a`'s.
- `wt-a` throughout and after: still live, `reed status` unchanged, pane capture still on its own prompt with
  **no trace of the text sent from `wt-b`**. Output file untouched.

Verdict: **CLEAN, two independent layers deep.** shuttle adds no new cross-contamination risk at its own layer
and correctly inherits reed's; an operator watching `wt-a` saw zero bleed from `wt-b`.
Teardown: both `down` → tmux servers 0, claude 4 (baseline).

### Sweep probe — `Start`'s opportunistic orphan sweep vs. an absent `reed.json` (no LLM cost)

Aged run dir `…/.lyx/shuttle/deadbeef…` (mtime 2 h old) holding a `run.json` whose `strandGuid` is tracked,
in `wt-a`, with the session DOWN so `Start` fails at `AddStrand` and no `claude` is ever spawned — the sweep
still runs (it precedes `AddStrand`), so the three state shapes are separable at zero live-substrate cost:

| `reed.json` state | `lyx shuttle run` outcome | aged run dir |
| --- | --- | --- |
| present, guid tracked | `add strand: no reed session (1 strands persisted)…` | **PRESENT** (correct) |
| corrupt (`{"socket":"x`) | `add strand: no reed session, and reed's persisted state could not be read…` | **PRESENT** (correct — the documented skip) |
| **absent** (`rm .lyx/reed.json`) | `add strand: no reed session; run "lyx reed up"` | **GONE — swept** |

Verdict: see finding R3-F2.

### Not verified this round (stated honestly)

- **Subpath-anchored geometry.** Still `AnchorRel = "."` everywhere; none of the joint scenarios naturally
  constructed a subpath-anchored fixture, and the brief says not to spend a dedicated scenario on it.
- **R2-F9's two silent startup paths** (a trust prompt whose dismissal never takes; a pane that fails every
  capture). No scenario produced either — every run this round reached `StartupReady` promptly, and reed's
  `CapturePane` never failed. Still deadline-bound by construction (`classifyStartupWindow` is called from
  every not-yet-started exit), but unexercised live.
- **R2-F11** (`sendVerified` viewport scroll) — out of scope by the brief. Two `send` calls were issued this
  round and both were refused upstream of `sendVerified` (by `requireLiveStrand`), so its known limitation was
  not exercised either way.

## Findings

Two findings, both CONFIRMED, both small. One OUT-OF-CAMPAIGN observation about reed.
No BLOCKING. No NOT-FIXED-THIS-ROUND large finding.

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

### R3-F2 — the orphan sweep deletes a LIVE run's directory when `reed.json` is absent — MEDIUM, CONFIRMED, small

`internal/shuttleengine/run.go:238-258` (`sweepOrphansOpportunistic`).

```go
st, err := reedengine.LoadState(filepath.Join(r.anchorPath, lyxdirs.DotLyxDirName))
if err != nil { logger.Warn(…); return }   // corrupt  -> skip the sweep
guids := map[string]bool{}
if st != nil { for _, s := range st.Strands { guids[s.GUID] = true } }   // ABSENT -> empty guid set
```

`LoadState` has three answers, and this function collapses two of them. A corrupt file returns an error and
correctly SKIPS the sweep — the comment says why: "to avoid sweeping kept diagnosis dirs over an unrelated I/O
problem". An ABSENT file returns `(nil, nil)`, which falls through with an EMPTY live-guid set, so
`sweepOrphans` classifies **every** run directory older than `2 × startup_timeout_s` (180 s by default) as an
orphan and `os.RemoveAll`s it — including the directory of a run whose agent is still working in its pane.

Reproduced (see the sweep-probe table above): identical fixture, identical aged run dir whose `strandGuid` is
genuinely tracked — PRESENT with `reed.json` present, PRESENT with `reed.json` corrupt, **GONE** with
`reed.json` deleted.

Why "absent while agents are live" is a real state, not a contrived one — it is the state reed's own
`unreadableStateError` instructs the operator to create:

> or delete `<path>` by hand to keep the session (its panes and their processes keep running, untracked) and
> lose only reed's strand tracking

and it is also what a `git clean -xdf` produces, which `CONSTRAINTS.md`'s Durable-vs-Ephemeral State Invariant
makes a sanctioned operator action. Scenario 2b drove exactly that state with a live agent.

What the sweep destroys is not inert: `events.jsonl` is the file the provider's Stop hook is still appending to
(deleting the directory loses the run's completion signal permanently), `settings.json` and `prompt.md` are the
live process's own artifacts, and `run.json` is the ONLY thing that maps the strand guid back to a run — so
after the sweep `lyx shuttle interrupt/send <guid>` answers `"<guid>" is not a shuttle strand`. That is
verbatim the outcome `findRunByStrand`'s own doc comment says must be avoided ("sends them away from a running
agent"); it was hardened there against a TRUNCATED run.json and is reintroduced here by deleting the file
outright.

Note also that a `Start` reaching this branch is almost always a `Start` that is about to FAIL: `AddStrand`
needs a live reed session, and a live session means `reed up` has already written a `reed.json`. So the sweeps
this branch performs are, in practice, either useless (the run fails a moment later at `AddStrand`) or
destructive (the hand-deleted-while-session-live case). Skipping them costs nothing real: after the next
`lyx reed up` a present-but-empty `reed.json` restores ordinary sweeping.

Suggested fix: treat "no state file" the same way as "unreadable state file" — skip this sweep and log it,
with the reasoning stated in the comment (an absent table is not evidence that any run is an orphan; it is
absence of evidence, and reed itself documents deleting it as the way to KEEP a live session).

Size: small — one branch in one function, plus a `rundir_test.go`/`run_test.go` case.

### OUT-OF-CAMPAIGN (reed) — an in-process engine silently resurrects a vanished anchor path

Not a shuttle defect and NOT fixed here (reed's campaign is closed); recorded because it is the mechanism that
turns scenario 1's rename into R3-F1's false `died`, and the operator decides whether it warrants reopening reed.

`reedengine.withOpLock` (`lock.go:87-94`) does `os.MkdirAll(e.stateDir())` on the told `AnchorPath` before every
op. When a long-lived in-process engine outlives a rename of its worktree, that path no longer exists — and reed
RE-CREATES it. Observed in scenario 1: after `mv svc-orig svc-moved`, the shuttle process's next `Status()` left
behind a phantom `<hub>/svc-orig/` containing only `.lyx/reed.lock` and `.lyx/reed.json.lock`, then answered
`ok:true` with zero strands, because `LoadState` found no `reed.json` there.

`validateToldAnchorPath` (`server.go:226`) checks that the told anchor is non-empty and absolute — the two
shapes that "succeed against the wrong tree" — but not that it still EXISTS. For an out-of-process caller that
distinction never matters (the path is resolved a moment earlier). For a long-lived in-process engine it is the
difference between reed's designed foreign-session refusal firing and reed cheerfully reporting an empty world.
A possible reed-side answer would be to refuse an anchor path that has ceased to exist rather than re-creating
it; that is reed's call, not this round's.

## Post-clean-room reconciliation with rounds 1-2 (read only after the above was written and committed)

- **R3-F1 is new.** Round 2's "Named residual — `Wait` cannot tell 'reed is gone' from 'reed refuses'" is a
  DIFFERENT axis: it concerns two shapes of reed ERROR, both of which already take `Wait`'s honest
  identity-plus-error exit. R3-F1 is the case where reed SUCCEEDS — `Status` returns `ok` with a strand table
  that no longer contains the guid — and `Wait` manufactures a terminal `died` from it. Neither round reached
  it, because neither drove a reed-side state reset against a run that was still polling. I take no position on
  the named residual; it is unchanged and still correctly out of campaign.
- **R3-F2 was examined by round 2 and deliberately NOT recorded** ("Assessed and deliberately NOT recorded as
  findings", first bullet). Round 2 saw the same asymmetry and the same reed advice, and declined on the grounds
  that "blocking the sweep on absence would break the ordinary post-`down` cleanup path".

  I re-examined that reasoning and **disagree**, on a fact round 2 did not weigh: the sweep runs BEFORE
  `AddStrand`, and `AddStrand` cannot succeed without a live reed session, and a live session means `lyx reed up`
  has already written a `reed.json`. So the post-`down` cleanup path is not broken by skipping the sweep on
  absence — it is merely deferred to the next run after the next `up`, which is the next run that can succeed at
  all. Concretely: `down` (file deleted, dirs orphaned) → `up` (file recreated, zero strands) → `shuttle run`
  → the sweep runs with a PRESENT-but-empty table and collects exactly what it would have collected before.
  The only sweeps forgone are those on a `Start` that is about to fail at `AddStrand` anyway — plus the one
  dangerous case. Round 2's "the age guard is the correct and sufficient protection" does not hold either: the
  age guard protects a run that is STARTING, not one that has been running longer than 180 s, which is precisely
  the run a live-agent sweep destroys.

  Flagging the disagreement explicitly so the orchestrator can adjudicate rather than discover it.
