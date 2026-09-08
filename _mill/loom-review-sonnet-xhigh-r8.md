# `loom` review — round 8 (sonnet-xhigh-r8)

Independent, clean-room review. Findings below are formed BEFORE reading any prior round's
review/fixer report or the campaign handoff, per the "Clean-room review constraint".

Status: IN PROGRESS — appended incrementally per the "Log as you go" requirement.

## What was tested

### Setup

- Confirmed worktree: `/home/knatte/Code/loomyard/wts/crucible-loom-glyph-hardening`, branch
  `crucible-loom-glyph-hardening`, HEAD `034152421`.
- `which lyx` -> `/home/knatte/go/bin/lyx`; redeployed via `CGO_ENABLED=1 go run ./tools/deploy`
  to confirm PATH agreement with HEAD `034152421` before any live driving.

### TOP PRIORITY — live-driving the provider-startup seam on fresh fixtures

Per this round's explicit top-priority mandate, this was attempted FIRST, before the rest of the
review.

**Fixture #1** (`/home/knatte/Code/lyx-r8-hub-parent/lyx-test-HUB/r8-sonnetxhigh-live`) — a
brand-new `lyx fabric clone https://github.com/Knatte18/lyx-test-weft` into a parent directory
(`/home/knatte/Code/lyx-r8-hub-parent`) never used by any prior round or by this host's claude
installation, plus `lyx fabric add r8-sonnetxhigh-live` for a fresh task worktree.

- Authored a minimal, hand-written, valid one-card plan (`language: none`, a single `Create` card)
  directly into `_lyx/plan/`, validated clean via `lyx webster validate` (`{"cards":1,"ok":true,...}`).
- `lyx reed up` -> `{"ok":true,"session":"r8-sonnetxhigh-live","socket":"lyx-lyx-test-HUB-d9072c31","strands":0}`.
- `lyx webster run --fresh` (backgrounded, foreground-waited via polling) -> spawned a REAL Master
  session (`lyx reed status` showed `master::d2c7ff44` live on pane `%2`).
- `tmux -L lyx-lyx-test-HUB-d9072c31 capture-pane -p -t %2` at t+~4s already showed Master past
  BOTH gates: input box `❯` ready marker present, footer `⏵⏵ bypass permissions on (shift+tab to
  cycle) · esc to interrupt · ← for agents`, and the agent already mid-turn ("Running 1 shell
  command…", "✽ Nesting…").
- The run completed END TO END: Master forked exactly one implementer, the implementer created
  `r8-smoke-marker.txt` and committed it (`1aef516`, `"1: smoke-file"`), the integration-verify
  fork ran and reported OK, Master wrote `outcome.yaml` (`outcome: done`, `batches_done: 1`) and
  `summary.md`. `lyx webster run`'s own envelope: `{"batches_done":1,"cycles":[],"fabricCommitted":true,"ok":true,"outcome":"done",...}`.
- This is a genuine, live, end-to-end confirmation that BOTH the trust-this-folder gate AND the
  Bypass-Permissions modal were correctly detected and dismissed by the real
  `claudeengine.Startup`/`TrustDismissSequence` machinery on a path claude had never seen before —
  a mishandled gate would have either killed claude outright (the R5-2 failure shape) or wedged the
  run at the startup deadline / full run timeout (the R5-7/R6-1/F1 failure shapes), none of which
  happened; the run instead did genuine agentic work (forked a sub-agent, wrote a file, committed,
  ran an integration check) and finished cleanly.

**Fixture #2** (`/home/knatte/Code/lyx-r8-hub-parent-2/lyx-test-HUB/r8b-gatecapture`) — a SECOND,
independently fresh `lyx fabric clone` into a different-again parent directory
(`/home/knatte/Code/lyx-r8-hub-parent-2`), specifically to attempt catching the RAW gate frames
via tight-interval `tmux capture-pane` polling (every 300ms) starting immediately after
backgrounding `lyx webster run --fresh`.

- Attempted a direct `lyx reed add --cmd "claude --dangerously-skip-permissions" ...` raw probe
  (bypassing webster/plan entirely) to observe the gate with even less overhead — THIS SESSION'S
  OWN PERMISSION CLASSIFIER BLOCKED IT ("Blocked by classifier"). Did not attempt to route around
  the block (e.g. via an alternate tool); fell back to the sanctioned `lyx webster run` path, which
  the classifier did NOT block on retry (one subsequent `lyx fabric add` call was also transiently
  blocked, then succeeded on immediate retry — noted here per the "say so plainly" instruction, but
  it did not prevent completing the scenario).
- Tight polling (40 x 300ms captures) did NOT catch the raw gate render — by the very first poll,
  the pane was already past both gates and mid-turn. This is a STATED LIMIT: the gap between
  separate tool-call round-trips in this harness (on the order of 1-3s) is coarser than however
  fast Master's real startup-to-ready transition is in this environment (evidently well under a
  couple of seconds, including reed's own tmux split-window + claude process spawn + this round's
  `claudeengine.Startup`/`TrustDismissSequence` poll-and-dismiss cycle, which polls at
  `defaultPollIntervalMS` = 500ms). Could not visually inspect the exact live gate wording as a
  result; relying instead on (a) the successful end-to-end functional outcome as proof the
  classification/dismissal worked, and (b) static code reading against the two gates' documented
  needle sets.
- This fixture's run ALSO completed end-to-end successfully (`outcome: done`, `batches_done: 1`),
  a second independent confirmation.
- `claude --version` on PATH: `2.1.236` — NOTE: this is OLDER than the `2.1.263` the startup.go
  comments cite as "today's" wording ("Yes, I trust this folder", "Yes, I accept" — see
  `gateAcceptNeedles`'s doc comment). Both live runs succeeded against 2.1.236's actual rendering,
  so the current needle set does cover 2.1.236's gate wording live — the `2.1.263` comment is
  simply a stale/aspirational version reference (doc drift, not a functional defect on its own).
- Curiosity, not a correctness finding: `~/.claude.json`'s `projects` map only gained a
  `hasTrustDialogAccepted: true` entry for each hub's WARP-PRIME worktree path (`.../lyx-test`),
  never for the actual task-worktree path where Master's pane genuinely ran and the gate was
  genuinely dismissed (confirmed via `/proc/<pid>/cwd`). Did not chase this further — it does not
  affect shuttleengine's own correctness (which reads pane CAPTURES, not `~/.claude.json`), and is
  plausibly an artifact of this reviewing session's own directory-visit bookkeeping rather than
  anything the spawned Master sessions did.

**Teardown**: `lyx reed down` on both task worktrees, plus a residual `lyx reed down` on fixture
#2's warp-prime `lyx-test` (an idle header-pane shell left over from an early aborted probe on
that session, sharing the same hub tmux socket). Confirmed via
`tmux -L <socket> list-sessions` -> `no server running` for both hub sockets, and
`ps aux | grep -iE 'claude --dangerously|claude --session-id'` -> empty. Zero stray substrate
processes from this round's live driving.

**Verdict on high-yield-focus item 1**: genuine, real, end-to-end live confirmation obtained TWICE
independently on brand-new fixtures. Did not manage to construct or naturally trigger the specific
F1 failure shape (an accept-phrase-shaped transcript line landing adjacent to the input-box caret)
live — the agent's own turns in both runs never happened to produce such a line. Per the round's
own stated fallback, the "at minimum" bar (confirm both gates dismiss successfully end-to-end on a
fresh fixture) is met, convincingly, twice.

### Code reading — provider-startup seam (this round's top priority, item 2)

Read in full: `internal/shuttleengine/claudeengine/startup.go`, `doc.go`,
`internal/shuttleengine/wait.go`, `internal/shuttleengine/engine.go`, `internal/shuttleengine/run.go`
(Start/Wait/Interrupt/Send/Inject/requireReadyAgentPane/requireLiveStrand/playInputs/sendVerified/
scanPaneForNeedle/deliveredBelowBaseline), `internal/shuttleengine/claudeengine/startup_test.go`
(fixtures only, to understand what's already covered — this is a test file, not a review report,
so it's in scope pre-clean-room).

Found and CONFIRMED (via a throwaway unit test, since removed) a real gap — see Finding SF-1 below.

## Findings

Severity-ranked. CONFIRMED = traced/reproduced exactly. PLAUSIBLE = strong suspicion, not fully
traced/reproduced.

### SF-1 — BLOCKING — `startupGateNeedles` is missing a needle for the "Yes, proceed" gate wording, unlike `gateAcceptNeedles`

`internal/shuttleengine/claudeengine/startup.go:34` (`startupGateNeedles`) vs
`internal/shuttleengine/claudeengine/startup.go:225` (`gateAcceptNeedles`).

`gateAcceptNeedles = {"trustthisfolder", "yes,itrust", "yes,iaccept", "yes,proceed"}` identifies a
gate's ACCEPTING OPTION line (used by both `isGateAcceptOptionLine`/`locateGateLines` and, through
them, `TrustDismissSequence`'s caret walk). `startupGateNeedles =
{"trustthisfolder", "filesinthisfolder", "yes,iaccept"}` is the SEPARATE, independent needle list
`Startup` requires a hit on (`containsAnyNeedle(normalized, startupGateNeedles)`) before even
looking at `gateIsRendered`'s adjacency evidence.

Three of `gateAcceptNeedles`'s four entries survive the outer check today only as an accident of
substring containment once whitespace is stripped: "Yes, I trust this folder" normalizes to
`yes,itrustthisfolder`, which happens to CONTAIN the substring `trustthisfolder` (from
`startupGateNeedles`), and "Yes, I accept" is trivially in both lists already. **"Yes, proceed"
has no such accident** — `yes,proceed` shares no substring with any of
`trustthisfolder`/`filesinthisfolder`/`yes,iaccept`.

Consequence: if a gate renders with its accepting option worded "Yes, proceed" (the doc comment on
`gateAcceptNeedles` itself calls this "the older trust-gate wording Startup's own fixture set has
always treated as a recognized gate" — i.e. explicitly still a live-supported case, not a retired
one) AND the capture does not ALSO separately contain "trust the files in this folder" (or another
`startupGateNeedles` hit) — e.g. because the dialog's leading prose paragraph has scrolled out of
the captured viewport (`reed`'s `capture-pane` carries no `-S`, so it is viewport-only, per
`internal/shuttleengine/run.go:659`'s own doc comment) while the option list + footer remain
visible — `containsAnyNeedle(normalized, startupGateNeedles)` is FALSE, so `Startup` skips the
trust-prompt branch entirely, falls through to the ready check
(`strings.Contains(capture, gateCaretMarker)`), sees the gate's own selection caret, and returns
**`StartupReady`** for a pane that is NOT actually ready — the exact `opus-medium-r5` R5-7 failure
shape ("an unrecognized gate is classified StartupReady, the startup deadline stops applying, and
the run parks on the dialog for the whole master timeout... rather than dying fast"), reintroduced
through the SEPARATE needle list rather than through a missing needle in the SAME list R5-7 fixed.

CONFIRMED live via a throwaway unit test (added then removed, will be reinstated properly during
the fix):

```go
capture := "❯ 2. No, exit\n  1. Yes, proceed\n\nEnter to confirm · Esc to cancel"
c := &Claude{}
got := c.Startup(capture) // == shuttleengine.StartupReady (1), NOT StartupTrustPrompt (2)
```

`go test` output: `Startup(...) = 1 (StartupReady=1 StartupTrustPrompt=2 StartupPending=0)` —
confirms the pane misclassifies as ready.

Suggested fix: stop maintaining `startupGateNeedles` as an independently-hand-typed list that can
silently drift from `gateAcceptNeedles` (this is structurally the same class of bug R6-2 already
named once — "Missing that third spelling was not a missing nicety either"). Derive it from
`gateAcceptNeedles` plus the prose-only needles that never appear in an accepting-option line
(`filesinthisfolder`, which the trust gate's own explanatory paragraph carries but no option label
ever does):

```go
var startupGateNeedles = append([]string{"filesinthisfolder"}, gateAcceptNeedles...)
```

This makes every future addition to `gateAcceptNeedles` (a new accepting-option wording) automatically
close the outer gate simultaneously, closing off this whole class of bug rather than this one instance
of it.

Add a regression test mirroring the "older Yes, proceed trust-gate wording is dismissable" fixture
already in `startup_test.go`, but for a capture that carries ONLY the option list + footer (no
"trust the files in this folder" prose) — the exact shape a cropped/scrolled viewport would produce.
