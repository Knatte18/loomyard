# shuttle — independent review, round 4 (`opus-high-r4`)

Reviewer: Opus 5, high effort.
Scope this round is the operator's two named focus items, not an open sweep:
close **R2-F11** (`sendVerified`'s viewport-scroll false negative) with a designed, live-validated fix,
and drive the **three joint-composition scenarios** round 3 named as untested successors.
Per the round-4 brief this is explicitly NOT clean-room: `_mill/shuttle-review-r2.md` (R2-F11) and `_mill/shuttle-review-r3.md` were read in full before starting.

## Environment / baseline

- Host: Linux, tmux 3.6 (`/usr/bin/tmux`), `claude` at `/home/knatte/.local/bin/claude`, `LYX` = `.dev-bin/lyx`.
- Worktree: `/home/knatte/Code/loomyard/wts/reed-shuttle-crucible-hardening` @ `b5c972d1`, `./deploy-dev` green.
- Baseline BEFORE any driving: `ps -eo comm | grep -cx 'tmux: server'` = **0**; `pgrep -xc claude` = **2**
  (two unrelated long-lived operator sessions; more unrelated ones came and went during the round, so my own
  processes are tracked by argv instead — every one of mine carries `--model haiku` and a `--settings <scratch>/…` path).
- Every real `claude` this round: `--model haiku`, one at a time, no exceptions.

## Hermetic gates (before any driving)

- `go build ./...` — green.
- `go vet ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/shuttlecli/...` — green.
- `go vet -tags smoke ./internal/shuttlecli/...` — green.
- `go test -count=5 ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/shuttlecli/... ./cmd/lyx/...` — all `ok`.

## What was tested

(appended live, in order)

### Focus item 1 — R2-F11, live reproduction against a real Claude TUI

**Fixture.** Hub `<scratch>/r4/hubA`, worktree `wt-scroll` (plain git repo, `AnchorRel = "."`), `lyx reed up`
on socket `lyx-hubA-541459e6`. Strand pane geometry as reed configures it: **220x48**, header pane 220x1.

**Instrumentation.** A read-only recorder (`tmux -L <sock> capture-pane -p -t %2`, every 250 ms, straight to files)
ran continuously beside the driving, so every claim below is measured against **recorded real frames**, not inferred.
1195 frames carrying the needle were captured across the session.

**Process 1 (discarded fixture attempt).** A 25-step Bash-call run wrote its output file early, so `Wait` classified
`done` and cleaned up the strand and run dir before any `send` could be issued (`lyx shuttle send` then answered
`"…" is not a shuttle strand: no run found`, correctly — `FindRun` reads the run dir the cleanup had just removed).
Not a defect: this is the documented `done`-cleanup contract. Re-run with `--keep-pane`, which preserves strand,
pane and `run.json` after a `done` outcome, giving one long-lived idle Claude TUI to drive sends into.

**Process 2 (the reproduction).** Strand `a0733c0cfabd0190e36d5508e47e7cb3`, pane `%2`, live and idle.

1. Two priming sends established the needle in the transcript and pushed it up the viewport.
   Note a substrate fact worth recording: Claude's TUI **collapses** tool calls ("Ran 30 shell commands" is one line),
   so 30 Bash calls churn the viewport by ~4 lines — churn has to come from rendered assistant TEXT, not tool volume.
2. Priming send `reply with the numbers 1 to 38 one per line and nothing else` (needle = its 48 normalized chars,
   i.e. the whole string). After the agent's 38-line reply the message sat at **line index 1** of a 48-line viewport —
   the exact R2-F11 precondition: viewport full, the earlier copy at its top, `baseline = 1`.
3. First repeat of the same text: succeeded in 0.43 s. The frames show why, and it is a mitigating fact R2 did not have —
   the text is briefly visible in the TUI's **input box** while the transcript copy is also still on screen, so the poll
   caught `count = 2` inside a window of under 266 ms. **The false negative is a race, not a certainty.**
4. Second repeat, with the text extended past the 220-column pane width so its rendered message **wraps to two lines**
   (the needle's first 48 normalized characters are unchanged, so it is the same needle). That widens the delivery-time
   push from ~2 lines to ~3, which is enough to evict a copy sitting at index 1 in the SAME redraw:

```
$ lyx shuttle send a0733c0c… "reply with the numbers 1 to 38 one per line and nothing else, and take care to …"
{"error":"shuttle: Send: sent text never appeared in the pane after 2 attempt(s) — the provider TUI likely
 swallowed the input; the send was NOT delivered","ok":false}
real 0m11.530s
```

Recorded frames, relative to the send's start (`count` = the very count `sendVerified` computes):

| t (s) | count | needle line idx | what happened |
| --- | --- | --- | --- |
| −1.46 … +0.39 | 1 | 1 | baseline state: one copy, at the top of the viewport |
| **+0.658** | **1** | **38** | the new copy arrived at the bottom **and the old copy at index 1 scrolled off in the same redraw** |
| +0.66 … +5.98 | 1 | 38 → 26 → 0 | all 20 polls see `count == baseline`, neither `>` nor `<` — the ambiguity |
| **+6.51** | **1** | **39** | `sendVerified` REPLAYED the whole choreography — the agent received the message a SECOND time |
| +11.53 | 1 | 0 | `Send` returned `ok:false`, "the send was NOT delivered" |

**Verdict: R2-F11 is CONFIRMED live, and its consequence is worse than round 2's PLAUSIBLE writeup assumed.**
Round 2 predicted a duplicate agent turn. What actually happens is a duplicate agent turn **and** a hard
`ok:false` "the send was NOT delivered" returned to the caller for a send that was in fact delivered twice.
See finding R4-F1.

### Focus item 1 — measuring the candidate signal against the same recorded frames

`reed.CapturePane` is `capture-pane -p` (`internal/reedengine/io.go:151`) — the visible viewport, no scrollback and
no `-S`, so a scrollback-based fix is not reachable from shuttle without changing reed's API (out of campaign).
A provider-specific echo marker is barred by the Shuttle Provider-Seam Invariant (`sendVerified` lives in the
provider-invariant package). That leaves R2-F11's own suggested direction: **position**.

Measured over the whole recorded corpus (1195 frames carrying a needle, two different needles, across four sends,
an idle pane, a streaming pane and a tool-calling pane), tracking for each frame the count and the number of lines
BELOW the last occurrence:

- the capture is always exactly **48 lines** (a pane-height frame — `capture-pane` returns the viewport, padded),
- and `linesBelow` for a fixed occurrence **never once decreased** while the count stayed equal…
- …**except at exactly the three frames where a new copy was genuinely delivered** (46→8, 46→9, 47→8).

That is the empirical basis for the fix: **in this TUI a pane only ever appends at the bottom, so an existing
occurrence's distance from the bottom is monotonically non-decreasing until it scrolls off.**
A copy that now sits strictly closer to the bottom than every copy the baseline counted therefore CANNOT be one of
the baseline's copies — it is new, which is exactly the fact a bare count cannot express.

### Focus item 2, scenario 1 — subpath-anchored worktree geometry under joint stress

**Fixture (the campaign's first non-trivial anchor).** Hub `<scratch>/r4/hubS` with `_board/.lyx-anchor` recording
**`srv/api`**, worktree `wt-sub`, so `AnchorPath = <hub>/wt-sub/srv/api` and `WorktreeRoot = <hub>/wt-sub` are
genuinely different directories. `lyx reed up` from the anchor: session `wt-sub` (derived from the WORKTREE basename,
not the anchor), socket `lyx-hubS-91c0e0d7`.

Every one of the four things the brief named resolved correctly, each checked against the live fixture rather than read:

1. **`validateToldPaths`' containment check** — passes and stays silent: the anchor IS a subdirectory of the worktree
   root, which is the shape the clause was written to allow, and no run was ever refused.
2. **The run-dir root and reed's state** both sit under the ANCHOR: `wt-sub/srv/api/.lyx/shuttle/<runID>` and
   `wt-sub/srv/api/.lyx/reed.json`. Confirmed by `ls` and by the `runDir` field of every envelope below.
3. **A relative `--output-file` resolves against the WORKTREE ROOT**, exactly as the run verb's help promises, and
   NOT against the anchor. Proven without spending a process, using the pre-existing-file guard as an oracle:
   with `<wt-sub>/out-sub.txt` present, `--output-file out-sub.txt` is refused naming
   `…/wt-sub/out-sub.txt`; with `<wt-sub>/srv/api/out-anchor.txt` present, `--output-file out-anchor.txt` is NOT
   refused. The two bases are demonstrably distinguished.
4. **The fork audit's transcript directory** (`claudeProjectDirFor(anchorPath)`, `claudeengine/audit.go:97-112`)
   points where the transcript actually lands. After a real run, `~/.claude/projects/` held BOTH
   `…-hubS-wt-sub-srv-api` and `…-hubS-wt-sub`, and the session's `4f2abc3b-….jsonl` transcript was in the
   **anchor-derived** one; the worktree-root-derived one held only a `memory/` directory. Deriving from the anchor
   (the pane's own cwd, which reed sets via `new-session -c AnchorPath`) is correct, and deriving from the worktree
   root would have found an empty directory.

**Then round 3's scenario 2b re-run against this geometry** — `reed.json` deleted while a run is genuinely mid-turn
(agent inside a 90 s blocking `python3` call):

- delete at 21:14:45.4 → the run exited at 21:14:53.5 (8.0 s, two liveness ticks) with **`ok:false`** and R3-F1's
  mechanism-failure message, the identity triple intact and `runDir` correctly under the anchor. Not the pre-R3-F1
  `ok:true, outcome:"died"`.
- the agent was still alive afterwards, and **both** run directories were still on disk.
- `lyx shuttle interrupt`/`send` from the anchor: `ok:false`, `requireLiveStrand`'s not-tracked message. Honest.
- A subsequent `Start` (made to fail at `Prepare` via `--effort bogus`, so the orphan sweep runs but no process is
  ever spawned) logged R3-F2's skip naming the ANCHOR path, and both aged run dirs survived.

**Verdict: CLEAN.** Subpath anchoring changes nothing about shuttle's behaviour, and both of round 3's fixes hold
identically under it. No finding.

### Focus item 2, scenario 2 — a `PaneGeneration` mismatch reed CLEARS, with a live run attached

**The clear-not-refuse condition, read off `generation.go`:** `adoptPaneGenerationLocked` clears when the recorded
stamp `Recorded()` and is NOT `SameIncarnation(live)` AND `refuseLiveForeignSessionLocked` returns nil — and per
`classifyRecordedSessionLocked:203` that last part is immediate (`recordedSessionAbsent`) whenever
`recorded.SessionName == e.SessionName()`. So the CLEAR path is precisely: **this worktree's own session name, a
different incarnation** — reed's own doc calls it "simply a reed.json older than the session now running".

**Construction.** Reusing the subpath fixture, a fresh long-turn run was started and confirmed mid-turn
(strand `12334dbf…`, pane `%3`, agent inside a blocking `python3` call), then `reed.json`'s
`paneGeneration.created` was rewritten to an older value by an atomic write+rename — one field, nothing else, the
minimal mutation that isolates the clear condition while the session, the pane, the strand table and the agent all
stay genuinely real. (The fully-organic routes to the same state — kill the server and re-`up`, or restore a backup
taken before a rebirth — all kill the pane with the session, which would forfeit the "live run attached" half of the
scenario.)

Trigger at 21:17:10.8. Reed logged the clear, and 4.0 s later:

```
WARN reed: persisted pane bindings were minted against a different tmux session incarnation, clearing them
     recordedTmuxSession=$0 recordedServerPID=3524614 liveTmuxSession=$0 liveServerPID=3524614
{"guid":"12334dbf…","ok":true,"outcome":"died","runDir":"…","sessionId":"7b19df4a-…"}
```

State captured immediately afterwards:

- `lyx reed status` → `{"guid":"12334dbf…","live":false,"name":"12334dbf","paneId":""}` — the strand is **still in
  reed's table**, with an EMPTY pane binding.
- `tmux list-panes` → `%3 claude dead=0`, and the capture still showed `esc to interrupt`: the agent was working.
- `lyx shuttle interrupt`/`send` → `ok:false`, `"has no live pane — its run already reached a terminal outcome or
  its pane died; keys would be silently dropped"` — refused (good) but on two claims that were both false.
- Restoring the correct stamp made `reed status` report the same strand `live:true` on `%3` again, and
  `lyx reed remove` then killed a genuinely running agent — proof the pane was alive throughout and that the
  `died` verdict was purely a bookkeeping artefact.

**Verdict: this is the third shape the brief asked about, and it is a real defect — see finding R4-F2.**
It is neither R3-F1's ABSENT case (the guid IS in the table) nor the original `died` case (reed holds no pane for
this strand at all, so there is no pane that "is not alive"). It lands in the one branch R3-F1's fix left untouched.

### Focus item 2, scenario 3 — `Interrupt` and `Wait` genuinely racing a mid-refusal reed

**Fixture.** Hub `<scratch>/r4/hubC`, worktree `wt-orig`, `AnchorRel = "."`, with a real
`_lyx/config/shuttle.yaml` (the module template with `poll_interval_ms: 100` and `liveness_every_n_polls: 1`) so
`Wait`'s liveness tick fires every 100 ms and its two-strike mechanism-failure exit lands ~200 ms after any trigger —
shrinking the race window to something two external processes can genuinely collide inside.

Long-turn run live (strand `7c050def…`, pane `%0`, agent inside a blocking `python3` call). Then, in one shell:
two `lyx shuttle interrupt` processes pre-armed with a 50 ms head start, and the worktree renamed out from under the
running `Wait`. Everything below happened inside a **164 ms** window:

| t | who | result |
| --- | --- | --- |
| 21:20:20.614 → .652 | `mv wt-orig wt-moved` | rename completes |
| .6678 | both `interrupt` processes start (23 µs apart) | — |
| .702 | `interrupt` #1 | `ok:false` — reed's foreign-session refusal, naming `wt-orig`, the socket, and the `kill-session` remedy |
| .717 | `interrupt` #2 | `ok:false` — byte-identical refusal |
| .778 | the in-flight `Wait` | `ok:false` — R3-F1's `reed did not track strand … on 2 consecutive liveness checks`, identity triple intact |

- **No double-fire and no double-log:** two concurrent interrupts serialized through reed's op lock and produced two
  identical refusals; neither reached `playInputs`, so no key was delivered to any pane, once or twice.
- **No inconsistent pair:** all three verdicts are `ok:false` mechanism failures, none claims the run is dead or
  cleanable, and nothing was destroyed. They differ in VOCABULARY, and correctly so — they are answering from two
  different geometries: the in-process engine still holds the pre-rename anchor path (so it sees an empty table),
  while each CLI process resolves the post-rename one (so it sees reed's foreign-session refusal). Both statements
  are true of what each can observe.
- The agent was still alive and working afterwards; reed's phantom `<hub>/wt-orig/.lyx` was re-created exactly as
  round 3's OUT-OF-CAMPAIGN reed note describes (unchanged, still reed's call, not fixed here).

**Verdict: CLEAN.** No finding. Worth recording as an observation, not a defect: the `runDir` in `Wait`'s error names
the pre-rename path, which no longer exists — the directory travelled with the worktree. No code can hold a path that
was renamed under it, and the identity triple is still what lets an operator find the run.

## Findings

Three findings: one MEDIUM closing R2-F11, one MEDIUM new, one LOW.
No BLOCKING. Nothing deferred — no NOT-FIXED-THIS-ROUND finding.

### R4-F1 — `sendVerified` reports a delivered send as "NOT delivered" and replays it into the pane (closes R2-F11) — MEDIUM, CONFIRMED, small

- **Where:** `internal/shuttleengine/run.go:476-507` (`sendVerified`).
- **Severity:** MEDIUM (round 2 filed it LOW/PLAUSIBLE; the live reproduction shows a second, worse consequence).
  **Status: CONFIRMED, live-reproduced end to end** (frames above). **Size: small** — one function, one file.
- **Scenario:** the pane is full, an earlier copy of the same text sits at the top of the viewport, and the newly
  delivered copy's arrival pushes that earlier copy off in the same redraw. The count is then unchanged — neither
  `>` baseline (deliver) nor `<` baseline (R2-F6's re-baseline) — so every one of the 20 polls fails, the whole
  choreography is replayed into a pane that already received it, and `Send` finally returns
  `ok:false, "the send was NOT delivered"`.
- **Why it matters, concretely:** both halves are harmful, and the second one is new information.
  The duplicate turn is what round 2 named. The `ok:false` is what a caller acts on: `loom`/`burler` see a failed
  `send`, and the honest reaction to "the send was NOT delivered" is to send it again — a THIRD copy of the same
  instruction, or to abort a round that was in fact progressing. The verb's own contract ("verifies delivery")
  is inverted here: it reports non-delivery for a double delivery.
- **Why round 3 did not see it:** its two `send` calls were refused upstream by `requireLiveStrand`, so
  `sendVerified` never ran. Round 2 could not reproduce it for the same class of reason.
- **Suggested fix (validated against the recorded frames, see the measurement above):** keep the count check exactly
  as it is — it is the live-proven swallowed-send detector and its failure path must not move — and ADD one
  acceptance path in the branch that is ambiguous today. Track, alongside the baseline count, the number of lines
  below the LAST baseline occurrence; on a poll where the count is EQUAL and non-zero, accept delivery when the last
  occurrence now has strictly fewer lines below it (by a small margin) than the baseline's did. The position is
  computed over the SAME normalized concatenation the count already uses, with a per-character line-provenance map,
  so a needle that straddles a wrap boundary is still matched exactly as today — no change to what counts as a match.
  This is a strict narrowing: with no scroll it is byte-for-byte today's behaviour, it never turns a success into a
  failure, and when nothing is delivered no copy can appear below the baseline's, so the swallowed-input failure
  path is untouched.
- **Residual, stated honestly:** if the pane churns hard enough that the delivered copy is evicted between two
  250 ms polls, no viewport-only check can see it at all. That residual is inherent to `capture-pane -p` having no
  scrollback, is far narrower than the reproduced case, and is named in the code comment rather than papered over.

### R4-F2 — a strand whose pane BINDING reed cleared is classified `died` while its agent is still working — MEDIUM, CONFIRMED, small

- **Where:** `internal/shuttleengine/wait.go:272-277` (`checkLivenessTick`'s `if !strand.Live` branch).
- **Severity:** MEDIUM. **Status: CONFIRMED, live-reproduced** (scenario 2 above). **Size: small** — one branch,
  one file, plus a `wait_test.go` case against the existing `fakeReed`.
- **Scenario:** reed's `adoptPaneGenerationLocked` decides this worktree's persisted pane bindings were minted
  against a different session incarnation and CLEARS them (`clearAllPaneBindings` sets every `PaneID` to `""`).
  Reed's `Status` then reports the strand with `PaneID: ""` and, because `aliveIDs[""]` is false,
  `Live: false` (`lifecycle.go:1171`). Shuttle reads that as "the strand's pane is not alive" and returns
  `OutcomeDied` — `ok:true`, a successful terminal classification — for an agent that is demonstrably still
  working in a pane tmux reports as `dead=0`.
- **Why it matters:** this is R3-F1's hazard exactly, reached through the branch R3-F1's fix did not cover.
  `wait.go`'s own header states the rule — "died is reserved for a strand reed STILL TRACKS whose pane is not
  alive" — and a strand whose binding was cleared does not satisfy it: reed holds NO pane for it, so there is no
  pane whose liveness was assessed. The consequence is the same duplicate-agent hazard: an unattended caller
  (loom, burler, CI) reads `died` as "gone, retry" and spawns a second agent against the same worktree while the
  first keeps working unreachably.
- **The distinction IS available to shuttle**, which is what makes this small: `reedengine.StrandStatus` already
  carries `PaneID`, so "tracked, but reed holds no pane binding" is directly observable and needs no reed change.
  Shuttle's own `wait.go` already draws the neighbouring distinction for the not-tracked case (`errStrandNotTracked`).
- **Suggested fix:** in `checkLivenessTick`, before the `!strand.Live` branch, treat a tracked strand with an empty
  `PaneID` as a mechanism failure in `errStrandNotTracked`'s style (a new sentinel with its own message naming the
  cleared binding), so `Wait` takes its identity-preserving exit through the existing `statusFailures` two-strike
  counter rather than manufacturing a terminal outcome. Keep the satisfied-file-contract short-circuit ahead of it,
  exactly as the other two negative answers already do.
- **One case must be excluded, or the fix would misfire:** a run started `--anchor hidden` is never given a pane at
  all, so its strand legitimately carries an empty `PaneID` from the moment `AddStrand` persists it
  (`strand.go:280-322` realizes a pane "unless added anchor:hidden"). The new branch must therefore be gated on
  `run.spec.Display.Anchor != render.AnchorHidden`, leaving today's behaviour for hidden runs untouched.
- **Behaviour change worth stating:** on a GENUINE server rebirth the clear also fires and the agent really is gone;
  that case now reports a mechanism failure (`ok:false`, identity preserved, no cleanup) instead of
  `outcome:"died"`. That is the same trade R3-F1 made deliberately: for a state where the agent's fate is unknown,
  an honest mechanism failure is safe in both directions, while a confident `died` is wrong in one of them.

### R4-F3 — the no-live-pane refusal asserts two things that are both false when the binding was cleared — LOW, CONFIRMED, small

- **Where:** `internal/shuttleengine/run.go:436` (`requireLiveStrand`'s `!s.Live` message).
- **Severity:** LOW. **Status: CONFIRMED** (quoted verbatim from scenario 2). **Size: small.**
- **Scenario:** with the binding cleared as above, `lyx shuttle interrupt`/`send` answer
  `strand "…" has no live pane — its run already reached a terminal outcome or its pane died; keys would be
  silently dropped`. The refusal itself is right and nothing is destroyed, but both offered causes are false: the
  run had not reached a terminal outcome and the pane had not died — reed simply no longer holds a pane id for the
  strand. An operator following that message looks for a dead agent and finds a live one.
- **Why it is worth fixing rather than tolerating:** this is the same class of defect the campaign already fixed
  twice in `requireLiveStrand`'s sibling message (round 3's R3-F1, and the not-tracked message's own rewrite,
  which explicitly stopped naming only the completed-and-cleaned-up cause because it "would be wrong more often
  than right"). The cleared-binding case is now known to be reachable with a live agent, so the same reasoning
  applies to this branch.
- **Suggested fix:** split the empty-`PaneID` case out of the `!s.Live` branch with its own message — reed tracks
  the strand but holds no pane binding for it (a binding cleared as stale, or a strand added `anchor:hidden`), the
  agent may still be running in a pane reed can no longer address, check `lyx reed status`.

## Assessed and deliberately NOT recorded as findings

- **`Wait`'s error names a `runDir` that no longer exists after a worktree rename** (scenario 3). The directory
  travelled with the worktree; no code can hold a path renamed under it, and the guid/sessionId still locate the run.
- **The two concurrent `interrupt` processes produced identical output with no ordering guarantee between them.**
  Round 2 already recorded inter-process ordering as inherent (P1); this round adds that it is also harmless in the
  refusal path, since both processes refuse before touching the pane.
- **Reed re-creating a vanished anchor path** (`withOpLock`'s `MkdirAll`) — reproduced again in scenario 3, still
  OUT-OF-CAMPAIGN and still reed's call, recorded unchanged from round 3.
- **The `--anchor hidden` shuttle run has no pane and therefore cannot ever satisfy its file contract.** Reachable
  from the CLI, but it is a caller error rather than a shuttle defect, and R4-F2's fix deliberately preserves
  today's behaviour for it rather than quietly changing it.

## Scope assessment — plan-promised vs shipped

Nothing this round found shuttle short of what `internal/shuttleengine/doc.go` and `docs/overview.md` promise.
The subpath-anchored fixture is the first in the campaign to put real distance between `AnchorPath` and
`WorktreeRoot`, and every one of the four consumers the `validateToldPaths` comment enumerates resolved against the
base its own documentation names.
All three findings are cases where a shipped check is more confident than its evidence supports —
a count that cannot see position (R4-F1), and a liveness bool that cannot see a missing binding (R4-F2/R4-F3) —
not cases of missing scope.

## Convergence assessment

**R2-F11 is closed, and closing it needed exactly the live step the brief insisted on.**
Two rounds could only call it PLAUSIBLE from reading. Driving it produced three facts neither round had:
the failure is a race the input-box echo usually wins (so it is rarer than round 2 assumed), its consequence is
worse than round 2 assumed (a hard `ok:false` on a double delivery, not merely a duplicate turn), and the position
signal that fixes it is measurably stable over 1195 real frames (so the fix rests on measurement rather than on a
plausible argument about terminals).

**reed×shuttle joint composition — converged as far as this axis is worth pushing.**
Round 3 named three untested successors and predicted a fourth round would find less than its own two findings.
That held: two of the three came back CLEAN (subpath geometry, the concurrency race), the third found one defect of
exactly the shape round 3's own finding had — shuttle reading a reed bookkeeping fact as an agent fact — which is
now closed on both of its remaining branches. The composition surface has been exercised across ten distinct
scenarios over two rounds, and the residue is concentrated in a single, now-fixed conceptual seam rather than
spread across the module.

**shuttle-alone correctness — converged, four rounds agree.** Nothing outside the two focus items surfaced at the
normal bar while driving, and the normal flow (`done`, `asking`, `interrupt`, `send`, `--keep-pane`, the orphan
sweep) behaved exactly as documented on every fixture, including the subpath-anchored one.

## Merge readiness

**Merge-ready.** See the fixer report (`_mill/shuttle-review-r4-fixer-report.md`) for per-fix verification;
the merge bar is correctness in the normal single-instance flow, and every fix here narrows a branch that only
fires when the substrate has already gone sideways (a scrolled-off needle, a cleared pane binding), leaving the
normal path byte-for-byte unchanged.
