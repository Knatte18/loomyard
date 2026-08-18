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

## Findings

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
