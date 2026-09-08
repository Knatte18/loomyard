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
