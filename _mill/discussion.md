# Discussion: Fork-based cluster review in burler

```yaml
task: Fork-based cluster review in burler
slug: burler-fork-cluster
status: discussing
parent: main
```

## Problem

burler's single-reviewer round catches too little. The cluster design (`ClusterN > 0` on
the round profile) has been gated behind `ErrClusterUnsupported` since burler shipped,
waiting on mux own-window anchoring (roadmap milestone 24). The
session-fork-diversity-spike (2026-07-16, `docs/research/session-fork-spike.md`) de-risked
a better mechanism that removes that dependency entirely: Claude Code's **built-in fork
subagents** give N parallel reviewers that inherit one explorer's full context at ~17k
compute marginal per reviewer (vs ~209k cold), with perfect prompt-cache sharing measured
at N=8 and no coverage loss versus independent cold reviewers. Forks run in-pane as
background tasks — no extra tmux windows needed.

This task lifts the cluster gate using that mechanism: the burler handler explores the
target, fans out N lens forks, does its own holistic review while they run, consolidates
everything into one severity-ordered review, and feeds the existing fix phase.

## Scope

**In:**

- Lift the `ClusterN` gate in `internal/burlerengine`: replace the `ClusterN int` profile
  field with `cluster-fan: <name>` (a string naming a fan); delete `ErrClusterUnsupported`.
- New `burler` config module (configreg entry + seeded `burler.yaml`): `lenses:` (name →
  emphasis prose) and `fans:` (name → list of lens names). Standard library seeded.
- Cluster block in the burler round prompt template (new stencil marker(s), Go-composed,
  non-empty in both cluster and non-cluster rounds).
- Shuttle/claudeengine support: a Spec-level fork-mode knob; conditional PreToolUse(Agent)
  hook (allow unnamed forks only) replacing the blanket Agent deny for cluster runs;
  `CLAUDE_CODE_FORK_SUBAGENT=1` on the launch line for cluster runs only; a new
  `internal/shell` env-prefix primitive.
- Provider-invariant fork audit on the shuttle Engine seam; claudeengine implements it by
  reading `~/.claude/projects/<enc>/<session-id>/subagents/*.jsonl`; burlerengine applies
  the policy (exact-N enforcement, Agent-call hard error).
- Review-file contract extension: optional per-finding `origin:` frontmatter key; prose
  `## Rejected` section convention.
- Perch passthrough: `cluster-fan` carried 1:1 into each round's burler profile
  (replacing today's `ClusterN` passthrough), in `perchengine.Profile` and perchcli's
  profile YAML.
- Docs: burlerengine package doc cluster section rewritten as-built; `docs/overview.md`
  module rows; `docs/roadmap.md` milestone 24 decoupled from cluster-N and the
  "Deferred burler enhancements" cluster entry marked done; CONSTRAINTS.md Review Round
  Invariant wording checked (A-before-B holds; consolidation is part of A).
- Tests: unit suite + template pins + one opt-in real-engine smoke at N=2.

**Out:**

- Model-per-fork (forks always run the handler's model — CC constraint; CLI
  `--resume --fork-session` remains the documented fallback if that axis is ever needed).
- Non-Claude engines realizing cluster mode (a second engine must hard-error on a
  fork-mode Spec until it implements it — same pattern as effort validation).
- Mux own-window anchoring (milestone 24) — no longer a dependency of this feature;
  stays on the roadmap as an independent mux enhancement.
- Bulk-mode clusters / provider-side context caching (stays a deferred enhancement).
- Any change to the fix phase (B), the verdict vocabulary, perch's loop/judge logic, or
  the shuttle file contract (forks write no output files; the handler writes the two
  round artifacts exactly as today).
- Degrade-to-solo fallbacks of any kind: a cluster round that cannot fork is a failed
  round, fixed at the infrastructure level.

## Decisions

### cluster-fan replaces cluster-n

- Decision: The round profile's cluster interface is a single key, `cluster-fan: <name>`
  (YAML) / `ClusterFan string` (Go). Naming a fan turns clustering on; N = the fan's
  entry count. Absent/empty = no clustering — forking is never default. The `ClusterN`
  int field, its `cluster-n` YAML key (burler and perch), and `ErrClusterUnsupported`
  are deleted.
- Rationale: One mechanism, no count-vs-list reconciliation rules. "A fan IS cluster-N":
  the operator's unit of intent is the group of lenses, and the fan's length is the
  fan-out. Fail-loud validate replaces the gate: unknown fan name, unknown lens name in
  a fan, or fan length > 16 are hard validate errors.
- Rejected: keeping `cluster-n` as count with lens lists reconciled against it (silent
  truncation or awkward padding rules); `cluster-n` as sole on-switch with inert lens
  keys (naming a fan but forgetting the count would silently review solo — the exact
  quiet-degradation class the operator rejected).

### Single-file lens/fan config: burler.yaml via configreg

- Decision: A new `burler` config module (registered in `internal/configreg`, template
  owned by burlerengine per the existing pattern) seeds `_lyx/config/burler.yaml` per
  repo with the entire standard library visible and editable: `lenses:` maps lens name →
  emphasis prose (YAML block scalars, ~5–15 lines each); `fans:` maps fan name → list of
  lens names, repeats allowed.
- Rationale: A lens is only the emphasis steering — the fixed cluster machinery (report
  contract, no-Agent ban, "prefer inherited context") lives once in the prompt
  template — so one lens is 50–150 words and the whole library is ~100 lines of YAML.
  One file, one edit surface per repo; standard-library evolution reaches existing repos
  via `lyx config reconcile`. Changing what's default is editing one YAML line — that is
  the operator's stated point.
- Rejected: per-lens `.md` files with embedded-standards fallback and a repo override
  directory (plumbing — directory resolution via a new hubgeometry helper, embedded/file
  precedence — not justified at this text volume; a mechanical follow-up if lenses grow);
  lenses inline in each round profile (no shipped defaults, copy-paste per profile).

### Standard library content

- Decision: Standard lenses are the spike's eight — `correctness`, `error-handling`,
  `test-gaps`, `security`, `performance`, `api-design`, `concurrency`,
  `docs-consistency` — plus `generic` (broad review, no emphasis). No language-specific
  lens (no golang) in the standard set; repo-tailored lenses (a CONSTRAINTS-fit lens for
  lyx, C# lenses for other repos) are operator-authored in that repo's `burler.yaml`.
  Seeded fans: `default: [generic, generic, correctness, error-handling, test-gaps]`
  (N=5) and `full` (all eight standard lenses, N=8).
- Rationale: The spike's B1 arm (identical generic prompts) out-covered hard lenses —
  sampling diversity is strong — so the default fan blends two generics with the three
  core lenses. Every lens phrases steering as emphasis, never exclusion ("report anything
  else you notice too"): exclusion lenses measurably suppressed coverage.
- Rejected: shipping a golang lens (operator: not standard); lyx-specific content in the
  generic seed (belongs in lyx's own repo config).

### Three-phase round in one session

- Decision: The cluster round runs as ONE interactive shuttle session (the handler), same
  file contract as today. Phase 1: handler explores the target in full. Phase 2: handler
  spawns all N lens forks in a single message (built-in fork subagents: unnamed,
  `subagent_type: "fork"` per current CC semantics), does its own HOLISTIC review while
  they run (architecture, cross-file invariants, CONSTRAINTS-fit — the level no narrow
  lens covers), prepares ground truths + severity rubric for judging, then consolidates
  returned reports: dedup across lenses, per-finding origin labels, a rejected section
  for false positives (its own findings judged with equal skepticism), severity-ordered,
  written to the review file. Phase 3: today's fix phase (B), unchanged, driven by the
  consolidated review.
- Rationale: Validated end-to-end by the spike's E-arm at N=8 (perfect cache sharing,
  handler-as-judge caught a rogue fork). A-before-B is intact: the consolidated review is
  fully on disk before any target file is touched — consolidation is part of A.
- Rejected: separate reviewer sessions in own tmux windows (the original milestone-24
  design — 5–6× costlier per reviewer, needs unbuilt mux work); reviewers as CLI
  `--resume --fork-session` strands (no cache reuse — session-id-unique system prompt).

### Prompt integration: Go-composed cluster marker

- Decision: The cluster instructions enter `review-prompt-template.md` as new top-level
  stencil marker(s) whose value composePrompt builds in Go — cluster rounds get the full
  block (spawn instructions, the lens emphasis texts resolved from config, the fork
  report contract, the hard ban on Agent calls in fork prompts, holistic-review and
  consolidation rules); non-cluster rounds get explicit "single-reviewer round, no
  cluster" prose. Lens prompts steer with emphasis only.
- Rationale: stencil requires every marker non-empty and forbids conditionals (a
  required marker inside a conditional renders silently blank) — the priorRoundsBlock
  pattern is the established shape. Fork prompts are composed by the handler at runtime
  from the lens texts the template hands it.
- Rejected: a second, cluster-only template file (duplicates the round discipline text
  that template_test pins; drift risk).

### Shuttle Spec fork-mode knob

- Decision: `shuttleengine.Spec` gains a provider-invariant fork-mode field (exact name
  is plan detail; semantics: "this run will spawn in-session fork subagents"). burler
  sets it iff the profile resolved a fan. claudeengine realizes it; any future engine
  that cannot must hard-error (the effort-validation pattern: the engine is the sole
  validator of provider vocabulary).
- Rationale: burler must not know Claude specifics (Provider-Seam Invariant), and shuttle
  config alone cannot carry it — the knob is per-run, not per-hub.
- Rejected: a shuttle.yaml config key (would flip behavior for every run in the hub);
  burler bypassing shuttle to talk to claude (breaks the module chain).

### Conditional Agent hook for cluster runs

- Decision: When the Spec requests fork mode, claudeengine's settings.json emits a
  PreToolUse(Agent) hook that ALLOWS only calls matching an unnamed fork
  (`subagent_type: "fork"`, no `name`) and DENIES everything else with a steer.
  Non-cluster runs keep today's blanket Agent deny unchanged
  (`claude_deny_agent_tool: true` default untouched). The hook command pattern-matches
  the stdin JSON payload with grep-class tools under git-bash (no jq dependency) — same
  execution environment the Stop hook already relies on.
- Rationale: Mechanical prevention beats prompt trust (operator decision). Session-level
  hooks also police Agent calls made INSIDE forks, so a fork attempting any non-fork
  Agent call is denied by the hook, and fork-in-fork is additionally blocked by CC
  itself ("Fork is not available inside a forked worker", empirically confirmed).
  Caveat carried into testing: the spike never confirmed PreToolUse fires inside fork
  subagents — the smoke test must prove it; the Go audit is the trust-but-verify layer
  either way.
- Rejected: omitting the Agent deny entirely for cluster runs (leaves non-fork Agent
  calls unpoliced except by prompt and post-hoc audit); flipping the config key per hub
  (drops the guard for every run).

### Env flag on the launch line

- Decision: `CLAUDE_CODE_FORK_SUBAGENT=1` is set inline on the pane launch command, only
  for cluster runs. `internal/shell` gains an env-prefix primitive (posix: `K=v cmd`;
  pwsh: `$env:K='v'; cmd`) per the Shell Mechanics Seam — claudeengine composes the
  launch line through it, never emitting raw shell syntax.
- Rationale: The mux server env is deliberately scrubbed of `CLAUDECODE`/`CLAUDE_CODE_*`
  at boot (`muxengine.CleanClaudeEnv` — mandatory hygiene, unchanged), so the flag cannot
  ride the server env; the launch line runs after the scrub. Scoping it to cluster runs
  keeps a staged-rollout flag away from runs that never fork.
- Rejected: setting it on every launch (broadens exposure of a rollout flag for no
  benefit); `tmux set-environment` per pane (fights the hygiene story, leaks to later
  pane commands).

### Provider-invariant fork audit on the Engine seam

- Decision: The shuttle Engine seam gains a fork-audit capability. claudeengine
  implements it — it owns the knowledge that fork transcripts live at
  `~/.claude/projects/<encoded-cwd>/<session-id>/subagents/*.jsonl` (path-encoding prior
  art exists in muxcli smoke tests) — and returns provider-invariant per-fork facts:
  fork count, Agent-call count, tool-call counts, whether a report was returned. The
  Runner attaches the audit to `shuttleengine.Result` when the Spec requested fork mode
  (after a done classification). burlerengine applies policy on top.
- Rationale: Transcript layout is Claude-specific (Provider-Seam Invariant); policy —
  what counts as a violation — is burler's domain. "Never trust the narrative — read the
  file contract."
- Rejected: burlerengine reading transcript paths directly (seam violation); a separate
  post-hoc CLI audit verb (decoupled from the round result; perch would never see it).

### Audit policy: split by violation class

- Decision: In burlerengine, on a done-classified cluster round:
  (1) **Exactly N fork transcripts required** — fewer, including zero, is a hard error
  with a typed sentinel (e.g. `ErrClusterForksMissing`) whose message names requested vs
  actual ("requested 5, spawned 3"). Same class as a verdict-parse failure: the round
  fails loud. No degrade-to-solo, no partial-cluster warnings — a shortfall is an
  infrastructure defect (CC version, rollout flag, hook regression) and gets fixed, not
  accepted.
  (2) **An Agent call found inside a fork transcript** is a hard error too — the
  mechanical prevention layer (hook + CC depth block) failed, which is a bug, not a
  judgment call.
  (3) **Format non-adherence and rogue tool sprees** are NOT round-failing: they are
  hook-unpreventable agent behavior, the handler-as-judge phase is the semantic defense
  (spike-proven: it flagged the rogue fork and salvaged its novel findings), and the
  audit records them (tool-call counts, report-returned flags) as visible signal on the
  burler Result and CLI JSON envelope.
- Rationale: Fail-loud where a mechanism broke; visible-but-tolerant where an LLM was
  merely sloppy and a judge already handled it.
- Rejected: everything advisory (contradicts "hooks failed = bug"); everything
  round-failing (a sloppy fork report would kill a round whose consolidation already
  handled it).

### Consolidated review format

- Decision: The review file keeps its exact frontmatter contract. Two additions:
  an OPTIONAL per-finding `origin:` key (free text, convention `lens:<name>` /
  `handler`), carried through the `Finding` struct when present, never required by
  ParseReview; and a `## Rejected` prose section below the frontmatter listing false
  positives with a one-line reason each — rejected items appear nowhere in `findings`,
  so the fix phase and perch never see them.
- Rationale: Backward-compatible (ParseReview already tolerates unknown header keys);
  cheap per-finding provenance for debugging cluster quality; rejected-as-prose keeps
  non-findings out of the machine contract.
- Rejected: strict machine schema (origin required at cluster, rejected list in
  frontmatter — parser rules for data nothing machine-consumes yet); prose-only origin
  (loses cheap provenance).

### Engine wiring for fan resolution

- Decision: burlerengine loads its own config (LoadConfig + ConfigTemplate, the
  shuttleengine pattern) and resolves `ClusterFan` → ordered lens list (name + emphasis
  text) at validate time, fail-loud on unknown names. burlercli/perchcli wire the config
  in like they wire shuttle's today. Exact plumbing (config on Engine construction vs a
  Run parameter) is plan detail.
- Rationale: Resolution must happen before prompt composition and must fail before a
  strand spawns; config loading follows the established module pattern.
- Rejected: resolving in the CLI layer only (perch constructs profiles engine-side via
  buildRoundProfile — resolution must live where both paths pass through).

## Technical context

- **The gate:** `internal/burlerengine/profile.go` — `ClusterN`/`ErrClusterUnsupported`
  (validate rejects >0). Perch passes it through 1:1: `perchengine/profile.go`,
  `perchengine/roundfiles.go` (`buildRoundProfile`), `perchcli/run.go` (profile YAML
  decode). All these change to `ClusterFan string` / `cluster-fan`.
- **Prompt:** `internal/burlerengine/prompt.go` (composePrompt, marker map) +
  `review-prompt-template.md` (embedded via template.go). stencil.Fill requires ALL
  markers non-empty; NO conditionals in the template — Go composes block values
  (priorRoundsBlock is the pattern). `template_test.go`
  (`TestTemplate_StatesRoundDiscipline`) pins round-discipline statements — keep green.
- **Shuttle Spec:** `internal/shuttleengine/spec.go` — plain value + validate;
  Result in `run.go`; outcomes in `engine.go`. The file contract (OutputFiles existence
  = done) is untouched by clustering: forks write no output files.
- **claudeengine:** `settings.go` (buildSettings — Stop hook + PreToolUse denies;
  `steerAgentDeny`; the steer-text forbidden-chars init guard matters when adding new
  steer/hook strings), `command.go` (buildLaunchCmd — launch line composition through
  `internal/shell`). Hook commands run under git-bash on Windows; events path already
  rides shQuote.
- **Shell seam:** `internal/shell` (Quote/Invoke/ReadFile; pwsh + posix). New env-prefix
  method needed. Stdlib-only, provider-invariant — the env primitive is generic
  (key/value), not Claude-aware.
- **Env hygiene:** `internal/muxengine/env.go` (CleanClaudeEnv) scrubs
  `CLAUDECODE`/`CLAUDE_CODE_*` from the mux server env at boot (lifecycle.go). Unchanged
  and mandatory; the fork flag must therefore ride the launch line (post-scrub).
- **Config pattern:** `internal/configreg/configreg.go` (entries: name + ConfigTemplate;
  perch/shuttle are the model), `configengine.Load` strict-validates against the
  template. burler gets its first config module.
- **Verdict/parse:** `internal/burlerengine/verdict.go` — reviewHeader tolerates unknown
  keys (no KnownFields), so `origin:` slots in as an optional Finding field without
  breaking old files.
- **Transcript audit prior art:** muxcli smoke tests derive the
  `~/.claude/projects/<encoded-cwd>/` project dir (`smoke_resume_test.go`,
  `claudeTranscriptFiles`); production derivation belongs in claudeengine.
- **Spike facts to design against** (`docs/research/session-fork-spike.md`): forks must
  stay UNNAMED (named forks silently lose context ≤2.1.206); forks always run the
  parent's model; `CLAUDE_CODE_FORK_SUBAGENT` is staged-rollout, v2.1.117+; CC
  concurrency cap min(16, cores−2), queueing beyond; fork-in-fork hard-blocked by CC;
  `useExactTools` means fork toolsets cannot be stripped — forks keep tools by design
  ("prefer inherited context, fetch only what your lens needs"); handler context grows
  ~20–40k tokens at N=8 from returned reports (bounds N; cap 16 covers this).
- **Cost model (subscription):** ~17k compute marginal per fork reviewer vs ~209k cold;
  arm total at N=3 ≈ 286k vs 628k cold.

## Constraints

- **CONSTRAINTS.md applies in full**; specifically touched: CLI/Cobra Invariant (any new
  config module registration/help text; help-accuracy review on changed `Long` texts —
  burlercli/perchcli `Long` examples show `cluster-n: 0` today and must change), Shuttle
  Provider-Seam Invariant (transcript layout + hook specifics stay in claudeengine; Spec
  stays provider-invariant), Shell Mechanics Seam (env prefix only via `internal/shell`),
  Review Round Invariant (A-before-B intact — consolidation is part of A; template pin
  test must stay green; re-check wording, amend in the same commit if cluster phrasing
  needs recording), Test Tier Purity + Hermetic Git Env (new integration/smoke tests
  follow the tagging + TestMain rules), Sandbox Suite Coverage (burler is presumably
  covered; if scenarios change, update `**Covers:**` tags).
- **Weft Git Invariant:** unchanged — forks and handler never touch weft git; overlay
  fix-scope rules already forbid it.
- **Never headless:** the handler remains an interactive tmux session (CLAUDE.md
  agent-execution constraint); forks are in-session background tasks, which is
  compatible (they ride the handler's interactive session).
- **Forking is never default:** absent `cluster-fan` in every profile schema and every
  seeded example; the seeded burler.yaml defines fans but activates nothing.
- **Version pinning:** document the CC version expectations (flag v2.1.117+, unnamed-fork
  requirement ≤2.1.206) in the burlerengine package doc so a CC upgrade/downgrade has a
  reference point.
- **Documentation Lifecycle:** module behavior docs live in the burlerengine package
  header + overview module entry; roadmap only gets the milestone bookkeeping described
  in Scope.

## Testing

- **burlerengine (unit, fake shuttle):** validate — fan resolution (unknown fan, unknown
  lens in fan, cap >16, empty = no cluster), ClusterFan replaces ClusterN everywhere;
  prompt composition — cluster block present with resolved lens texts at cluster,
  explicit no-cluster prose otherwise, all markers non-empty; audit policy — exact-N
  enforcement returns the typed sentinel with requested/actual counts, Agent-call-in-fork
  hard error, sloppy-fork facts surface on Result without failing; ParseReview — `origin:`
  carried when present, absent-origin files still parse (backward compat).
- **Template pins:** extend `TestTemplate_StatesRoundDiscipline`-class assertions to the
  cluster sequencing statements (consolidated review on disk before B; forks spawned in
  one message; emphasis-never-exclusion phrasing).
- **shuttleengine/claudeengine (unit):** Spec fork-mode validation; buildSettings emits
  the conditional Agent hook in fork mode and the blanket deny otherwise (payload-match
  command shape pinned); buildLaunchCmd carries the env prefix in fork mode only;
  new shell env-prefix primitive (pwsh + posix quoting, injection-hardened like
  Quote); fork-audit parsing against fixture `subagents/*.jsonl` files (count,
  Agent-call detection, tool-call counts, report-returned).
- **configreg/config:** burler template registration (drift/help-tree/longlist pinned
  sets updated in the same commit); seeded burler.yaml strict-decodes; default fans
  reference only defined lenses (a test should enforce this self-consistency).
- **perch (unit, fake burler):** `cluster-fan` passthrough profile → round profile.
- **Smoke (opt-in, real engine, N=2):** one cluster round end-to-end mirroring
  `smoke_round_test.go`: forks actually spawn (audit finds exactly 2 transcripts),
  consolidated review parses with origins, and — explicitly — the PreToolUse(Agent) hook
  fires INSIDE a fork (the unverified assumption from the spike): a fork instructed to
  attempt a non-fork Agent call must be denied. Tagged per Test Tier Purity; hermetic
  git env per the invariant.
- **TDD candidates:** ParseReview origin extension, fan resolution/validate, audit
  policy, shell env-prefix primitive — all pure functions with crisp contracts.

## Q&A log

- **Q:** Where do lens templates live? **A:** Config, easily editable — landed on a
  single seeded `burler.yaml` (see Decisions; evolved via Q15/Q19/Q20 below).
- **Q:** How do count and lens selection relate? **A:** Selection must be configurable
  with a default; landed on fan-only (Q18).
- **Q:** How does a cluster round get past the session-level Agent deny? **A:** Spec-level
  knob; hooks stay active as mechanical prevention (refined in Q11).
- **Q:** Where is `CLAUDE_CODE_FORK_SUBAGENT=1` set? **A:** On the launch line, cluster
  runs only, via a new shell-seam env primitive.
- **Q:** Cap on fan-out? **A:** 16 as an upper cap ("16 sounds like a lot — that many
  lenses isn't realistic, but yes as upper cap").
- **Q:** Where does the Go-side fork compliance check live? **A:** Provider-invariant
  audit on the Engine seam; claudeengine implements; burler applies policy.
- **Q:** What does an audit violation do? **A:** "Can't we have hooks active to prevent
  such things?" — yes: conditional PreToolUse(Agent) hook (allow unnamed forks only)
  for cluster runs; audit becomes trust-but-verify, split by class (Agent-call = hard
  error, sloppiness = recorded signal).
- **Q:** Handler can't fork (rollout flag off / old CC)? **A:** "This is an error. Do NOT
  band-aid such errors. It gets fixed." — hard round failure, no solo degrade.
- **Q:** Partial shortfall (requested 5, spawned 3)? **A:** Same as zero — exactly N or
  typed hard error.
- **Q:** Consolidated review format? **A:** Optional `origin:` frontmatter key + prose
  `## Rejected` section.
- **Q:** Test scope? **A:** Unit suite + one opt-in smoke at N=2.
- **Q:** Perch passthrough? **A:** Yes, mechanical — "and forking is not default!"
  (`cluster-fan` absent = solo, everywhere).
- **Q:** Lens profiles as groups? **A:** Operator: lenses AND named groups ("fans"/
  "profiles") of lenses, both configurable PER repo, with a shipped standard collection —
  e.g. a repo-tailored fan mixing language lenses with a CONSTRAINTS lens.
- **Q:** Lenses in YAML? **A:** Operator first said lenses are too textual for YAML
  (one .md per lens); after sizing (a lens is 50–150 words of emphasis steering — the
  fixed machinery lives in the prompt template), approved the single-file burler.yaml
  with everything in it (Q20: option 1).
- **Q:** What turns clustering on? **A:** "We set cluster-profile — isn't that easiest?
  The default profile is its own thing, with standard stuff. … a profile IS cluster-N" —
  `cluster-fan` alone activates; N = fan length; a `default` fan ships in the seed.
- **Q:** Standard library content? **A:** "Golang is NOT a standard lens. I don't know —
  you'll figure something out. But it must not be hard to CHANGE what's default. That's
  the point." — spike's 8 + `generic`; seeded `default` fan
  `[generic, generic, correctness, error-handling, test-gaps]`; language/repo lenses are
  per-repo additions.
- **Q:** "Fan" or "profile"? **A:** Operator: not important, your call — "fan" (avoids
  double-booking "profile", matches the fan-out framing); docs note "fan (a cluster
  profile)" once.
