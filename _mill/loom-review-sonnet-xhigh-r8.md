# `loom` review — round 8 (sonnet-xhigh-r8)

Independent, clean-room review. Findings below are formed BEFORE reading any prior round's
review/fixer report or the campaign handoff, per the "Clean-room review constraint".

Status: REVIEW (Job 1) COMPLETE. See the fixer report
(`_mill/loom-review-sonnet-xhigh-r8-fixer-report.md`) for Job 2.

## Executive summary

10 findings: **2 BLOCKING, 4 MEDIUM, 3 LOW, 1 NIT.** All CONFIRMED (traced to source or reproduced
via a throwaway test; none left as merely PLAUSIBLE).

- **SF-1 (BLOCKING)** — the provider-startup seam's OWN needle list (`startupGateNeedles`) is
  missing a needle for the "Yes, proceed" gate wording that its sibling list (`gateAcceptNeedles`)
  already recognizes, letting that specific gate rendering misclassify as `StartupReady` and
  silently disable the startup deadline — the fourth consecutive round to find genuine BLOCKING
  material in this exact seam (after rounds 5, 6, 7), each a narrower recurrence of the same
  "two lists identifying the same gate can silently diverge" defect class.
- **PG-1 (BLOCKING)** — a `#`-shaped glyph ref that fails quarry's own grammar (e.g. a doubled `#`)
  produces ZERO findings anywhere across all 27 plan-validation checks outside a `Prosa` group — a
  silent hole in the format's own "fail loud, no `none` sentinel" discipline, on a surface (the
  glyph alphabet) this campaign exists to harden.
- **CW-1/CW-2 (MEDIUM)** — cliwire's own two custom AST-based enforcement tests (the sole-caller pin
  for `standalonestate.Derive`, and the banned-wiring-helper-redeclaration scan) each have a real,
  demonstrated blind spot (function-value indirection; a `var` holding a func literal instead of a
  `FuncDecl`) — exactly the kind of gap this round's "second model on cliwire" mandate existed to
  find.
- **PG-2 (MEDIUM)** — a Rename card's own `plan:` New-side handle is never bound to its resolved
  glyph by any code path, permanently losing glyph-level containment/resolution checking for every
  later legitimate reference to the renamed symbol.
- **WS-1 (MEDIUM)** — `lyx webster validate`, documented as a side-effect-free lint, can silently
  rewrite the on-disk plan (handle canonicalization) without restamping `state.json`'s fingerprint —
  the one caller of that rewrite path in the whole codebase that omits the restamp every sibling
  caller performs, risking a forced `--fresh` full-plan restart triggered by what looks like a
  read-only command.
- **CW-3/LS-1/WS-2 (LOW)**, **CW-4 (NIT)** — see each finding's own writeup.

**Live-driving (high-yield-focus item 1, this round's top priority): ACHIEVED, twice, independently,
end to end.** Two brand-new fixtures (never seen by claude on this host), each a real
`lyx webster run` spawning a genuine Master session, both cleanly dismissed BOTH the trust-folder
gate and the Bypass-Permissions modal automatically and went on to do real agentic work (forked an
implementer, wrote and committed a file, ran an integration check, reached `outcome: done`). Could
not additionally catch the raw gate-render frames via tight `tmux capture-pane` polling (a stated
tooling-latency limit, not a skipped scenario), and did not manage to naturally trigger the specific
F1 prose-collision shape — the round's own stated fallback ("at minimum confirm both gates dismiss
end to end") is met, convincingly, twice. See "TOP PRIORITY" below for the full transcript of what
was attempted.

**Second-model cliwire review (high-yield-focus item 3): DONE**, and it did NOT come back clean —
2 MEDIUM (CW-1, CW-2), 1 LOW (CW-3), 1 NIT (CW-4), all in the enforcement-test machinery rather than
the core resolution logic, which held up clean under adversarial pressure from an independent model
for the second round running.

**General adversarial sweep (high-yield-focus items 2, 4, 5):** covered via a combination of my own
direct reading (the provider-startup seam and `cliwire` in full) and four parallel, independently
clean-room sub-agents covering the rest of the named surface (loom pipeline/shed core, the glyph
plan surface, webster/standalone material) — every sub-agent finding was independently re-verified
by tracing it to source myself before being adopted into this report, not taken on faith. The loom
pipeline/shed core surface (item 4's other half) came back essentially clean (1 LOW/NIT-shaped
finding, LS-1, in ledger round bookkeeping) — this campaign's pre-glyph pipeline machinery really
does look converged. The glyph surface (seven prior rounds) was NOT clean — PG-1/PG-2 are new,
real, previously-unfound gaps, item 5's mandate paying off exactly where the prompt hoped it might.

**Merge-readiness and convergence verdict:** see the end of this report, after the fixer report
records what actually got fixed.

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

Also read `startup_test.go` and `wait_test.go`'s fixture coverage in full to confirm SF-1 is
genuinely uncovered: the existing `"trust_prompt_older_wording"` fixture
(`Do you trust the files in this folder?\n> 1. Yes, proceed\n  2. No, exit`) always keeps the
prose co-resident with the option line, so no existing test exercises an option-list-plus-footer-only
capture. Confirmed reed's actual pane geometry is wide (`220x50`, observed live via
`tmux list-panes`), which somewhat mitigates (but does not eliminate — height-driven cropping is
still possible, e.g. a long scrollback pushing the dialog's own leading paragraph above the
captured viewport) the "narrow-terminal wrap" angle named in the round's own prompt.

### Hermetic gates — clean baseline BEFORE any fix

- `CGO_ENABLED=1 go build ./...` — clean.
- `CGO_ENABLED=1 go vet` over the full named package set from the round prompt — clean.
- `CGO_ENABLED=1 go test -count=5` over the same set + `./cmd/lyx/...` — 20/20 packages `ok`, no
  FAIL/panic.
- `CGO_ENABLED=1 go test -tags integration` over the named integration set — all `ok`.
- `CGO_ENABLED=1 go test ./...` (whole repo) — all `ok`.

### Delegated parallel review (fresh, non-forked sub-agents, same clean-room constraint)

To cover the full breadth of "What to read" within this round's budget, four fresh sub-agents were
dispatched in parallel, each briefed on the clean-room constraint (no `_mill/loom-review-*` files)
and instructed to report concrete, adversarial, file:line-cited findings back in text (no report
files of their own):

1. Second-model review of `internal/cliwire` (+ `webstercli`/`burlercli` wiring).
2. `internal/loomengine`/`loomcli`/`loomrecipe`/`loomshed`/`shedengine`/`shedadapters`/
   `shedrecipe`/`shedbuild`/`hubgeom` + the loom recipe + `cmd/lyx`'s loom integration.
3. `internal/planparser`/`internal/planglyph` (the glyph surface) against
   `contracts/specs/loom-plan-spec.md`.
4. `internal/websterengine` (beginbatch/recordbatch/fingerprint/render/runlevel) +
   `shuttleengine/run.go`'s `NewRunner`/`NewDetachedRunner` + `standalonegeom`/`standalonestate`/
   `logger/sink.go`.

Their findings, once returned, were independently verified (not taken on faith) before being
folded into this report's findings list below — see each finding's own CONFIRMED/PLAUSIBLE marker
and verification note.

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

### CW-1 — MEDIUM — `TestDeriveCallerSet_CliwireOnly`'s `callsDerive` only matches a direct `pkg.Derive(...)` call expression, missing function-value indirection

`internal/cliwire/callerset_enforcement_test.go:143-165` (`callsDerive`).

Delegated review (dispatched sub-agent, second-model cliwire pass) reported this; I independently
verified it by re-reading `callsDerive` myself: it walks for `*ast.CallExpr` nodes whose `.Fun` is a
`*ast.SelectorExpr` naming `standalonestate.Derive`. A production file outside `internal/cliwire`
that instead writes `var deriveFn = standalonestate.Derive` (capturing the method value) and later
calls `deriveFn(target)` genuinely reaches `Derive` — a real second production caller, exactly what
the Cliwire Sole-Wiring Invariant's Derive-pin half exists to catch — but the capturing assignment is
a bare `*ast.SelectorExpr` inside a `*ast.ValueSpec`, never wrapped in a `CallExpr`, so the walk never
sees it, and the later `deriveFn(target)` call is an `*ast.CallExpr` whose `.Fun` is an `*ast.Ident`
(not a `SelectorExpr`), which also does not match.

Severity: MEDIUM — a genuine, demonstrated hole in the mechanical proof that `internal/cliwire` is
the sole production caller of `standalonestate.Derive`, of the same shape as round 7's F3 (which
hardened the sibling caller-set pin against a dot-import). Requires an unusual code shape (threading
`Derive` through as an injected function value) to trigger — not something ordinary drift produces —
but the whole point of an AST-based enforcement test is to not depend on ordinary drift being the
only threat model.

CONFIRMED (I re-derived the same conclusion independently from the source, not solely from the
sub-agent's report).

Fix: extend `callsDerive` (or add a sibling check) to also flag a bare `*ast.SelectorExpr` outside
`internal/cliwire` whose `X` is the standalonestate alias and whose `Sel.Name == "Derive"`, independent
of whether it sits inside a `CallExpr` — a value-capture is exactly as much "naming the symbol" as a
direct call.

### CW-2 — MEDIUM — `TestBannedDeclarations_CliPackagesCallIntoCliwire` only matches a top-level `*ast.FuncDecl`, missing a package-level `var` holding a func literal under a banned name

`internal/cliwire/bannedecl_enforcement_test.go:103-113`.

Also reported by the delegated sub-agent; independently verified: the scan's inner loop only type-asserts `decl.(*ast.FuncDecl)` for each top-level declaration. A policed package
(`internal/webstercli`/`internal/burlercli`) that re-declares a banned helper as
`var resolveStandaloneTarget = func(cwd, flag string) (string, error) { ... }` produces an
`*ast.GenDecl`/`*ast.ValueSpec` at the top level, not a `FuncDecl`, so it is invisible to this walk —
even though it creates an identically-named, identically-callable package-level symbol, which the
test's own doc comment says is exactly what it exists to catch (the doc comment credits a re-declared
METHOD as the shape that motivated checking receivers alongside plain functions; a `var` holding a
func literal is one further spelling of the same partial-copy-under-a-known-name hazard, and the
`bannedWiringDeclarations` map's own name list explicitly includes names that would need to be
reachable as either a func or a var to be a genuine re-implementation-copy risk).

Severity: MEDIUM — same class of gap as CW-1, on the sibling enforcement test, both closing checks
this round's own "second model on cliwire" mandate was specifically aimed at finding.

CONFIRMED (independently re-derived from the source).

Fix: also walk top-level `*ast.GenDecl` nodes with `Tok == token.VAR` (and `CONST`, for completeness)
and flag any `*ast.ValueSpec.Names` entry whose name is in `bannedWiringDeclarations`, regardless of
what the RHS is.

### CW-3 — LOW — `internal/cliwire` has no dedicated Told-Geometry import-allowlist enforcement test

Delegated sub-agent finding, independently spot-checked. CONSTRAINTS.md's Told-Geometry Invariant
lists `cliwire` among its "Bound packages" (never a direct import of `internal/lyxcwd`), and
`internal/cliwire/doc.go` makes an explicit prose claim about its fixed dependency set, but no test
in the package mechanically pins that claim the way `internal/shedrecipe/seam_enforcement_test.go`
(and several siblings) pin theirs. I confirmed this is a genuine, REPO-WIDE gap, not something the
`cliwire` consolidation introduced or regressed — several other Told-Geometry "Bound packages"
(`reedengine`, `burlerengine`, `websterengine`, `planparser`, `planglyph`, `configengine`) equally
lack a dedicated enforcement test; only packages that also carry their OWN separately-named Leaf
Invariant get one today. I manually confirmed `cliwire`'s actual current imports match `doc.go`'s
claim exactly (no live violation).

Severity: LOW — no live violation, and fixing every Bound package's coverage gap is explicitly a
larger, repo-wide task outside this round's cliwire-specific mandate. Given `cliwire` already carries
two custom enforcement tests (an unusually high bar for a young package) and is this round's specific
second-review target, I will add a small, cliwire-scoped `seam_enforcement_test.go` mirroring
`shedrecipe`'s pattern as part of the fix pass — closing the gap for the one package this round is
actually auditing, without taking on the repo-wide task of auditing every other Bound package (that
remains this campaign's own follow-up, not this round's job).

### CW-4 — NIT — `RepositoryRootOf`/`ResolveToldDir` have no defensive guard against an empty/relative `cwd`

Delegated sub-agent finding, independently spot-checked against the sole production call path
(`resolveStandaloneTarget` → `cwd` sourced from `lyxcwd.CwdFrom`, which errors upstream before `wire`
is ever reached on an unresolvable cwd) — NOT reachable today. `filepath.Join("", ...)` silently
drops the empty element rather than failing, so a hypothetical future caller passing an empty `cwd`
would get a silently-wrong (relative, or process-cwd-relative) result rather than a clear error.

Severity: NIT — no live path reaches it; `standalonestate.Derive` itself still fails loud on a
relative target downstream, so even a hypothetical bad caller would not silently corrupt state, just
fail less informatively than an early guard would. Will add a one-line non-empty assertion to both
functions during the fix pass since it's a trivial, cheap hardening with no behavior change on any
real call path.

### PG-1 — BLOCKING — a `#`-shaped ref that fails quarry's own glyph grammar produces ZERO findings anywhere in the 27-check pipeline (outside a `Prosa` group)

Delegated review (dispatched sub-agent, glyph-surface pass) reported this; independently re-derived
end to end from source (not taken on faith):

- `internal/planparser/classify.go:58-60` (`classifyRef` rule 2): classifies ANY ref containing `#`
  as `refKindGlyph` on shape alone — explicitly, by design, "never calls `glyph.Parse`" (its own doc
  comment, line 9-10).
- `internal/planparser/validate.go:350-367` (`diskPathForRef`): on `parseGlyph` error, returns
  `("", false)`.
- `internal/planparser/validate.go:374-396` / `:1051-1103` (`checkCardPathMalformed` /
  `checkPathMissing`): both silently `continue`/skip when `diskPathForRef` returns `!ok` — verified
  directly, lines 380-383 and the `checkRef` closure around line 1072-1076.
- `internal/planglyph/planglyph.go:282-300` (`collectGlyphTargets`): `add`'s closure calls
  `glyph.Parse(lang, raw)` and on error just `return`s without adding the ref to `seen` — verified
  directly, lines 288-291. A ref that never enters `seen` never enters the batched
  `quarry.Resolve` call at all.
- `internal/planglyph/resolve.go:79-133` (`statusFindings`): only iterates `results` — the
  `quarry.ResolveResult` answers `Resolve` actually returned. A ref `collectGlyphTargets` never
  submitted has NO corresponding `ResolveResult`, so it can never trigger `glyph-rejected` (fires on
  `r.Status == ""`, i.e. a target quarry ITSELF rejected after being asked), `glyph-not-found`, or
  `glyph-ambiguous` — those three checks all presuppose the ref was actually resolved against.

Net effect: **a malformed-but-`#`-shaped ref is invisible end to end.** Concrete adversarial example
(a plausible copy-paste typo, verified against quarry's actual vendored grammar rejecting anything
with more than one `#`):

```
**Edit:**
- `internal/boardcli#newListCmd#extra`
```

This passes `bare-symbol-target`/`directory-target` (wrong shape — those only fire on
`refKindSymbol`/`refKindPath`), passes `card-path-malformed`/`path-missing` (skipped, `!ok`), passes
`containment-unit-overlap` (same skip pattern in `internal/planparser/containment.go:39-45`), and
never even reaches `planglyph`'s resolve-backed checks. **The plan validates 100% clean** while
carrying a target no execution engine can ever act on — record-batch's own done-check would be the
first thing to ever notice, deep into a batch's execution, not `Plan-Validate` up front where every
other malformed-entry class is caught.

The ONE check that does catch a malformed `#`-shaped ref today is `checkProsaSymbolTarget`
(`validate.go:905-952`) — but only for entries inside a `**Prosa:**` group, since that check's whole
job is "is this a self glyph at all", not "is this glyph well-formed" for the other six type labels.
This asymmetry — a `glyph-malformed` counterpart exists for handle-shaped entries (`handle-malformed`)
but not for glyph-shaped ones — is what makes this read as an oversight rather than a deliberate scope
narrowing; nothing in `classify.go`/`validate.go` documents skipping grammar validation on a
non-Prosa group as intentional the way every other narrowing in this heavily-self-documented codebase
does.

CONFIRMED — traced every hop above directly from source myself (not solely from the sub-agent's
report); `internal/planparser/glyphref_test.go`'s only "malformed glyph" fixture is a `#`-free string
(a different, already-handled code path via `refKindSymbol`), and no test in `validate_test.go`
exercises a doubled-`#` or otherwise-grammar-invalid `#`-shaped target through the full check
pipeline.

Suggested fix: add a new `glyph-malformed` check (pure, `internal/planparser/validate.go`, alongside
`bare-symbol-target`) that fires whenever `classifyRef(ref) == refKindGlyph` but
`parseGlyph(lang, ref)` errors, scoped exactly like `bare-symbol-target` (skipped under
`language: none`, applies to Targets and Uses on every group). One check closes the gap for
`card-path-malformed`, `path-missing`, both containment tiers, and the resolve-backed status policy
simultaneously, since they all currently rely on shape classification alone to decide whether to even
attempt validation.

### PG-2 — MEDIUM — a Rename card's own `plan:` New-side handle is never bound to its real glyph; every later reference to the renamed symbol permanently loses glyph-level validation

Delegated review (dispatched sub-agent) reported this; independently re-derived from source:

- `internal/planparser/parse.go:534-552` (`parseTypeLabelCase`, the `renameLabel` branch): appends
  both pair endpoints to `card.Targets`, never to `card.Declarations`.
- `internal/planparser/parse.go:554-567` (the `createLabel` branch): is the ONLY branch that appends
  to `card.Declarations`.
- `internal/planglyph/handle.go:305-321` (`BindHandles`): `for _, c := range cards { if
  len(c.Declarations) == 0 { continue } ...}` — a pure-Rename card has zero `Declarations`, so it is
  skipped entirely; its own New-side handle is never added to the `subs` substitution map `BindHandles`
  builds and applies via `RewriteRefs`.
- `internal/planglyph/drift.go:31-44` (`renameCardPairs`) + `:101-106` (gate one inside
  `DetectDrift`'s loop over `delta.Renamed`): when a real rename matches a declared Rename card's own
  expected old/new pair, `DetectDrift` explicitly `continue`s — deliberately NOT queuing a
  substitution repair for it, on the assumption the Rename card's own bracket-verb flow already
  handles the rewrite. It does not; see above.

So neither of the two places that ever rewrite a `plan:` handle into its resolved glyph spelling
touches a Rename card's own New side. A later card legally referencing the renamed symbol (legal —
`handle-dangling` treats a Rename's `New` side as a valid declaration, matching a Create's) keeps
the `plan:`-prefixed spelling for the rest of the plan's life. Since
`collectGlyphTargets`/`resolveContainment` both exclude anything `plan:`-prefixed by construction, and
both containment tiers require `refKindGlyph`, that reference becomes **permanently invisible** to
`glyph-not-found`/`glyph-ambiguous` resolution AND to both containment checks
(`containment-unit-overlap`/`containment-file-overlap`) — the exact checks whose stated purpose
(`internal/planparser/containment.go:1-8`) is preventing a blind-parallel-dispatch merge conflict.
A card dispatched in parallel with such a reference, against another card editing the same file's
self glyph, is not flagged even though the referenced symbol is by then perfectly real and
resolvable.

CONFIRMED — traced every hop above directly from source; `internal/planglyph/handle_test.go`'s
`TestBindHandles_*` suite never exercises a Rename-declared handle, confirming by omission that this
path has no test coverage today.

Suggested fix: `BindHandles` should also collect every Rename group's own `p.New` handle (matched
against the delta's created/renamed symbols by ID, mirroring `renameCardPairs`'s own
`resolveKeyFor` normalization) and fold it into the same `subs` map/`RewriteRefs` call, so a Rename's
New side loses its `plan:` prefix exactly when a Create's does.

### LS-1 — LOW — a bouncer ledger/focus file's own `round:` frontmatter field is validated (positive int) but never cross-checked against the round number encoded in its filename

Delegated review (loom pipeline/shed core pass) reported this; independently verified:
`internal/shedadapters/bouncerfiles.go:81-98` (`recordedVerdict`) reads `ledgerPath(runDir, round)`
(a path built FROM the caller's `round` argument) and calls `parseLedger(ledgerRaw)`, but discards
the returned `ledgerFile.Round` value entirely (`if _, err := parseLedger(ledgerRaw); err != nil`) —
never comparing it against `round`. `parseLedger` (`:136-149`) and `parseFocus` (`:201-214`) each
validate only that their own `round:` field is a positive integer, never that it matches the
filename's `round-<N>-...` segment. A judge writing `round: 1` inside a file actually named
`round-3-bouncer-ledger.md` would pass validation silently.

Severity: LOW — confirmed nothing in the reviewed code branches on the parsed `Round` value for
control flow (`recordedVerdict`'s presence/parseability check is what matters, and that is correctly
keyed off the filename via `ledgerPath(runDir, round)`, independent of the frontmatter's own claim);
the field is otherwise only round-tripped by `renderFocus`. This is a narrow instance of the same
soft spot `internal/shedadapters/doc.go` already names and explicitly accepts (ledger carry-forward
is enforced by the judge prompt alone, not diffed mechanically).

Suggested fix: have `recordedVerdict`/`readRoundFocus` assert `parsed.Round == round` and treat a
mismatch as unparsed (fail-safe to Stuck/re-judge), matching this file's own posture toward every
other malformed-input shape.

### WS-1 — MEDIUM — `lyx webster validate` can silently rewrite the on-disk plan (handle canonicalization) without restamping `state.json`'s fingerprint, unlike every other caller of the same rewrite path

Delegated review (webster/standalone pass) reported this; independently verified end to end:

- `internal/webstercli/validate.go:143-154` (`scopedValidate`): calls `planglyph.Validate` (no run
  yet / no terminal batch) or `planglyph.ValidateDispatch` (mid-run) — neither followed by any
  fingerprint restamp or state mutation of any kind.
- `internal/planglyph/planglyph.go:37-42` / `:65-79`: both call `resolvePass`, which (`:155-185`)
  calls `CanonicalizeHandles(pending, planDir, results)` — verified this genuinely rewrites
  `_lyx/plan/*.md` on disk via `planparser.RewriteRefs` whenever a pending card carries a
  not-yet-canonical `plan:` handle, and reloads the plan from disk when it does (`rewrote` bool,
  lines 187-205).
- `internal/websterengine/beginbatch.go:213-239`: the SAME `ValidateDispatch` call, from the SAME
  package, is immediately followed by `restampFingerprint(deps.State, deps.Plan.Dir)` — verified
  directly, with an explicit comment naming exactly why ("otherwise state.json keeps the pre-rewrite
  fingerprint while the plan on disk carries this run's own sanctioned edit, and every later
  begin-batch refuses it as a foreign one"). `recordbatch.go` and `runlevel.go` follow the identical
  restamp-immediately-after pattern at their own `ValidateDispatch`/`BindHandles`/`DetectDrift` call
  sites.

`webstercli/validate.go` is the one caller of this rewrite-capable pass that never restamps. Concrete
scenario: a run is in progress (`state.json` holds a fingerprint from the last bracket-verb restamp).
A pending card's `plan:` handle is not yet canonical. The operator runs `lyx webster validate` —
exactly the documented "lint-without-run pre-flight for a Planner or human" workflow the command's
own `Long` text recommends, and which a live scenario like PG-2 above (a Rename card whose New-side
handle is never bound by any OTHER path) would keep genuinely pending indefinitely. `validate`
canonicalizes the handle, rewrites the plan file on disk, and reports `{"valid": true, ...}` — while
`state.json`'s `PlanFingerprint` is left stale. The next bracket verb (`begin-batch`/`record-batch`/
`recover-batch`/`run`) computes a fresh fingerprint that no longer matches the recorded one and
refuses with `ErrFingerprintMismatch`, whose only recourse is `--fresh` — archiving `state.json` and
`reports/` and restarting the ENTIRE plan from batch 1, discarding a live run's progress bookkeeping
over a self-inflicted, silent side effect of what the command's own docs call a read-only lint.

CONFIRMED — verified `validateCmd`'s full `RunE` (`validate.go:188-231`) contains no restamp call of
any kind, and no test exercises this: `TestValidateCmd_ScopeFollowsRunProgress`'s mid-run subtest
saves a `State` with `PlanFingerprint` left at its zero value and never re-checks it after calling
`validate`, and its fixture cards carry no `plan:` declarations, so it never reaches the rewrite path
at all.

Severity note: the most direct way to LAND a not-yet-canonical handle mid-run is somewhat narrow on
its own (an out-of-band plan edit, or a Rename card per PG-2's own gap above) — but `validate`
silently masking the true state while claiming `"valid": true` is a violation of this codebase's own
otherwise-exceptionless "restamp immediately after every sanctioned on-disk rewrite" discipline, with
no comment anywhere justifying the omission the way every other narrowing decision in this codebase is
justified.

Fix: mirror `persistPlanFingerprintRebaseline` (`internal/webstercli/cli.go:283-296`) inside
`validateCmd`'s `RunE` — under `AcquireStateMutation`, if `LoadState` returns a non-nil state,
recompute the fingerprint after `scopedValidate` returns and persist it if it changed. Also correct
`validate`'s own doc text ("it never spawns anything") to acknowledge it can rewrite plan files via
handle canonicalization, since that claim currently implies no side effects at all.

### WS-2 — LOW — stale/incorrect doc comment on `websterengine.Geometry.WorktreeRoot`

Delegated review (webster/standalone pass) reported this; independently verified:
`internal/websterengine/geometry.go:22-24` claims "webster's is the anchor-anchored value every one
of its CLI call sites passes today" — true for hub mode
(`internal/hubgeom/webstergeom.go:25`: `WorktreeRoot: anchorPath`) but demonstrably false for
standalone mode (`internal/standalonegeom/webstergeom.go:34`: `WorktreeRoot: target`, deliberately
DISJOINT from the anchor/state directory — confirmed by that file's own correct, contradicting
comment at line 28). "Every one of its CLI call sites" overstates a claim that holds for hub mode
only.

Severity: LOW — comment-only; both individual constructors are themselves correct and correctly
documented. Worth fixing precisely because this campaign's fingerprint bugs have repeatedly grown
from a false assumed invariant like "WorktreeRoot == AnchorRoot always" — cheap to fix, so fixed
this round.

Fix: scope the claim to hub mode explicitly, or drop the generalization in favor of pointing at each
geometry constructor's own (correct) comment.

## Fixing — summary

All 10 findings FIXED (2 BLOCKING, 4 MEDIUM, 3 LOW, 1 NIT), one commit per fix, on top of `034152421`.
Full table with commits, tests, and change descriptions: `_mill/loom-review-sonnet-xhigh-r8-fixer-report.md`.
Zero findings deferred; zero NOT-FIXED-THIS-ROUND. Every fix ships with a new or extended regression
test that fails against the pre-fix code (confirmed for every code-shaped fix by tracing the pre-fix
behavior directly, not merely asserted) and a doc update in the same commit where the finding touched
documented behavior (`loom-plan-spec.md`'s 27→28-check renumbering for PG-1; the judge stencil's own
marker fix for LS-1; `validate`'s help text and `Geometry.WorktreeRoot`'s comment for WS-1/WS-2).

One fix (LS-1) surfaced a genuine, separate, pre-existing production defect while being implemented —
a judge-stencil template-marker conflation that a narrower fix would have turned into a live
regression (silently discarding every real judge's own focus-file targeting) — and both were fixed
together rather than shipping the narrower, unsafe version. See LS-1's own writeup and the fixer
report's Notes section for the full trace.

**Final hermetic gate re-run, cold, after all 10 fixes, before writing this verdict:**
- `CGO_ENABLED=1 go build ./...` — clean.
- `CGO_ENABLED=1 go vet` over the round's full named package set — clean.
- `CGO_ENABLED=1 go test -count=5` over the same set + `./cmd/lyx/...` — 20/20 packages `ok`, zero
  FAIL, zero panics across 5 runs each.
- `CGO_ENABLED=1 go test -tags integration` over the named integration set — all `ok`.
- `CGO_ENABLED=1 go test ./...` (whole repo) — all `ok`.
- `CGO_ENABLED=1 go run ./tools/deploy` — redeployed clean at final HEAD, confirming the binary this
  round's own live-driving used reflects every fix (though the two live-driving runs themselves
  happened during Job 1, against `034152421`, before any fix — none of the 10 findings were on a code
  path either live run exercised in a way that would have behaved differently post-fix: SF-1 is a
  narrow needle-list gap the live runs' actual gate renderings did not happen to hit, and the other 9
  are glyph/cliwire/bouncer-surface findings outside the smoke-test plan's own one-card scope).

**Teardown, confirmed clean at the end of Job 1 and again now:** zero stray `tmux`/`claude` processes
from this round's own live driving (`ps aux` scoped to this round's fixtures, and both hub tmux
sockets confirmed to have "no server running" via `tmux -L <socket> list-sessions`); the one
pre-existing `tmux` process on this host (pid from Sep 2, this session's own outer terminal) and the
handful of pre-existing `claude` sessions predate this round entirely and are not this round's to
tear down.

## Merge-readiness verdict

**READY**, with the same honest limits every round of this campaign has carried forward (see below).
All 10 findings this round found are fixed, tested, and verified against cold hermetic gates. The
provider-startup seam — this round's top priority, and the site of BLOCKING material in 3 of the
previous 4 rounds — was both live-driven successfully (twice, independently, end to end) AND given a
genuine adversarial reading that found and closed one more real gap (SF-1), the fourth consecutive
round to do so, but this time closed with a STRUCTURAL fix (deriving the outer needle list from the
inner one) rather than another point patch, specifically to break the "narrower recurrence" pattern
the prior three rounds each represented.

## Convergence verdict

**NOT MET**, continuing the campaign's own honest count: this is the FIFTH consecutive round (5, 6,
7, 8, and this one makes it not just "in a row" but "every round since the seam opened") to find and
fix genuine, non-trivial material — including a fourth straight round finding real material in the
provider-startup seam specifically (following R5-2/R5-7, R6-1, F1, now SF-1). Per the campaign's own
bar (a safety pass finding nothing severe, not a safety pass finding-and-fixing severe things however
well verified afterward), this round does not close the campaign on its own.

That said, three things distinguish this round from a simple continuation of the pattern, worth
weighing rather than glossing over:

1. **The provider-startup seam's own defect shape has now visibly narrowed AND changed kind across
   rounds 5→6→7→8**: R5-2/R5-7 were "the mechanism doesn't exist at all" (no gate recognized, or a
   recognized gate confirmed the wrong option); R6-1 was "the mechanism exists but has no positive-
   evidence requirement"; F1 was "positive evidence exists but has no adjacency requirement"; SF-1 is
   "adjacency exists and is correct, but ONE OF TWO SEPARATELY-MAINTAINED NEEDLE LISTS drifted from
   the other" — a narrower, more mechanical class of gap than any of its three predecessors, and one
   this round's own fix eliminates STRUCTURALLY (the two lists can no longer diverge, by construction)
   rather than by patching the one instance found. Round 7's own read — "each iteration is a strict
   narrowing" — continues to hold, now across four iterations instead of three.
2. **Live confirmation of the seam, which round 7's own verification explicitly named as "the single
   most valuable thing a live-capable session could still add," was obtained this round** — twice,
   independently, end to end, on brand-new fixtures. This closes a real, previously-open gap in the
   campaign's own evidence base: F1's fix (and, transitively, the whole R5→R6→R7 lineage it rests on)
   has now been proven not just by sabotage-proof (a strong but indirect method) but by a genuine live
   claude pane clearing both gates and doing real agentic work.
3. **Everything OUTSIDE the provider-startup seam did NOT come back clean this round** — unlike round
   7's own general sweep, which found the rest of the surface sound. This round's PG-1/PG-2 (the glyph
   surface, seven prior rounds deep) and CW-1/CW-2 (cliwire's own enforcement tests, second review) are
   all genuine, non-trivial, previously-unfound gaps outside the startup seam. This is the more
   sobering half of this round's own evidence: the "everything else has converged" read that rounds 6
   and 7 both offered was itself not fully accurate — a general adversarial sweep by a sufficiently
   different combination of eyes (a genuinely fresh model, PLUS parallel dedicated sub-agent passes
   with independent verification, a method this campaign has not used in this combination before) still
   finds real material outside the seam too. Whether that reflects this round's own method being more
   thorough than rounds 6/7's, or genuine remaining surface area, is a fair question for the operator
   to weigh — this report does not resolve it either way.

## High-yield-focus items — what was achieved

1. **Live-driving the provider-startup seam on a fresh fixture (TOP PRIORITY): ACHIEVED**, twice,
   independently, end to end. See "TOP PRIORITY" above for the full transcript. Could not additionally
   catch the raw gate-render frames via tight polling (stated tooling-latency limit) and did not
   naturally trigger the specific F1 prose-collision shape; the round's own stated fallback ("at
   minimum confirm both gates dismiss end to end") is met, convincingly, twice.
2. **Adversarial re-examination of the provider-startup seam by reading: ACHIEVED**, finding and fixing
   SF-1 (BLOCKING). Attempted but could not further narrow the "one rendering quirk wide" residual
   (the no-blank-line-before-the-input-box scenario) named by round 7 — neither confirmed nor
   disproven live this round; still an open, explicitly-stated residual, not newly closed.
3. **Second-model adversarial review of cliwire: ACHIEVED**, and did not come back clean — 2 MEDIUM
   (CW-1, CW-2), 1 LOW (CW-3), 1 NIT (CW-4), all fixed. Convergence-across-models evidence for cliwire
   itself is now genuinely mixed: round 7 found the core resolution logic sound (still true — nothing
   this round found was in module.go/paths.go/standalone.go's core logic), but found real gaps in
   BOTH of cliwire's own enforcement tests, the exact mechanism round 7 itself hardened once already.
4. **General adversarial sweep over everything else: ACHIEVED**, via a combination of direct reading
   and four independently-verified parallel sub-agent passes. Found real, previously-unfound material
   (PG-1 BLOCKING, PG-2 MEDIUM) on the glyph surface specifically — NOT a clean pass, unlike rounds 6
   and 7's own general sweeps.
5. **Whatever this round's own combination (Sonnet/xhigh, first deployment on this seam; four parallel
   independently-verified sub-agent passes, a method not used in this combination before) turns up
   that seven prior passes didn't: ACHIEVED** — SF-1, PG-1, PG-2, CW-1, CW-2, LS-1 (6 of the 10
   findings) are all genuinely new material no prior round recorded, confirmed by this round's own
   independent verification of every sub-agent claim before adopting it, not taken on faith.

## Honest limits (this campaign's own, carried forward, plus this round's own)

- **Windows path behavior**: never reachable from this Linux host across all eight rounds; not
  reasoned about as if driven, per this round's own explicit out-of-scope instruction.
- **`burlercli`'s standalone reed bring-up**: remains live-unverified, as in every prior round. Fair
  game for a future round, not required.
- **`DetectDrift`'s exact-tier auto-repair path live through a real Webster fork**: still never
  triggered by any round's live driving (open since round 3).
- **The "one rendering quirk wide" residual** round 7 named (no blank line between the transcript and
  the input box, in some future rendering) is neither confirmed nor disproven this round — still an
  open, stated residual, not a new finding and not newly closed.
- **The raw gate-render frames were not captured live this round** — a stated tooling-latency limit
  (the gap between separate tool-call round-trips in this harness is coarser than the seam's own
  gate-render-to-dismiss window), not a skipped scenario; the END-TO-END functional outcome (both
  gates dismissed, real agentic work followed) is the evidence obtained instead, and is judged
  sufficient by this report for the "confirm live" mandate, but a future round with tighter live-loop
  control could still add the visual confirmation this round could not.
- **The ~45-item residue from round 6's sweep over loom's pre-glyph pipeline machinery**: remains
  recorded but deliberately out of this campaign's declared scope, per this round's own seed — a
  separate mill-wiki task, not folded in here. This round's own general sweep over that SAME surface
  (loomengine/loomcli/loomrecipe/loomshed/shedengine/shedadapters/shedrecipe/shedbuild/hubgeom) found
  it essentially clean beyond LS-1 — corroborating, not contradicting, that the pre-glyph pipeline
  machinery is in good shape independent of the ~45-item residue's own disposition.
- **The `~/.claude.json` trust-registration curiosity** noted during live-driving (both hub sessions'
  trust acceptance registered against the WARP-PRIME path, never the actual task-worktree path where
  Master's pane genuinely ran) was not chased to a root cause — plausibly an artifact of this
  reviewing session's own directory-visit bookkeeping, not evidence of anything shuttleengine itself
  does wrong (which reads pane CAPTURES, never `~/.claude.json`), but stated rather than silently
  dropped.
