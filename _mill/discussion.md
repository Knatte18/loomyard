# Discussion: Investigate the unexplained lyx mux server crash

```yaml
task: Investigate the unexplained lyx mux server crash
slug: mux-server-crash
status: discussing
parent: cluster-fork-spike
```

## Problem

During the cluster-fork-spike session (task #044) the entire tmux/psmux server for the
sandbox Hub died twice while an operator was attached via `lyx mux attach` — not a pane,
the whole server process. Roughly 20 minutes of deliberate reproduction attempts (idle
soak, capture-pane, live sends, a real attach client, 40 rapid-fire messages, window
resizes) under both `strace -f -e trace=signal` and tmux's own `-vv` debug log failed to
reproduce it, and every routine cause (OOM, segfault, systemd scope restart, cron, operator
action) was ruled out with a direct check. The root cause is unproven, so this task ships
mitigations and forensic prep, not a fix:

1. Opt-in tmux verbose logging so the *next* unexplained death leaves evidence — the two
   real crashes happened with zero logging active, which is the single biggest reason the
   investigation stalled.
2. Make a dead server cheap to notice: today the operator only finds out when the next
   `lyx mux` verb fails with `no mux session; run "lyx mux up"`, which doesn't even point
   at `resume` (the verb that actually rebuilds the strands).
3. Fix the shipped default layout so the pane the operator saw garbled — the 1-row
   `anchor:top` band — actually shows something readable. A 1-row pane shows effectively
   nothing; the default must be taller.
4. Land the already-implemented-and-verified per-strand `Display.TopBandRows` override
   (currently sitting uncommitted in the `cluster-fork-spike` worktree's working tree).

Both crashes correlated with a Claude Code TUI squeezed into the 1-row top band rendering
corrupted output. That correlation is suggestive but unproven as a causal link; the render
corruption itself is what item 3–4 eliminate.

## Scope

**In:**

- Re-apply and commit the existing TopBandRows fix (4 files: `internal/muxcli/add.go`,
  `internal/muxengine/render/{rules.go,rules_test.go,types.go}`) in **this** worktree, as
  its own early batch.
- Bump the shipped `top_band_rows` default from 1 to **3** in both config templates
  (`internal/muxengine/template_posix.yaml`, `template_windows.yaml`) and update the
  default-asserting test (`config_test.go`).
- New mux.yaml config key `debug_log: ${env:LYX_MUX_DEBUG:-0}` — level `0|1|2` mapping
  to no flag / `-v` / `-vv` on the server-spawning psmux invocation (string-typed Config
  field; Go helper parses/validates). Existing hubs must run `lyx config reconcile` once
  the template gains the key (strict-template contract).
- Route the tmux/psmux server log file (`tmux-server-<pid>.log`, written to the server
  process's cwd) into `.lyx/logs/` (machine-local, never git-tracked), and prune old logs
  at server boot (keep the newest 3).
- Enrich the shared dead-session error in `requireSessionLocked`
  (`internal/muxengine/lifecycle.go`): when the session is absent **and** persisted
  mux.json exists with ≥1 strand, the error suggests `lyx mux resume` (rebuild) alongside
  `lyx mux up` (bare substrate). All verbs (status/add/remove, attach pre-flight) inherit
  it from the one chokepoint.
- Update CLI help text (`up`'s `Long`, template comments) for the new key — help accuracy
  is a review obligation under the CLI/Cobra Invariant.
- Update `docs/reviews/mux-review-prompt.md` so future mux reviews exercise the new
  behaviors (debug toggle, resume hint, top_band_rows=3 default, per-strand override).

**Out:**

- No further crash-reproduction hunting. ~20 min of targeted repro under strace + `-vv`
  already failed; the debug logging shipped here IS the preparedness for the next
  occurrence.
- No proactive dead-server monitoring (daemons, watchers, notifications) — only better
  error text at the moment a verb already fails.
- `collapsed_strip_rows` and `min_full_rows` defaults unchanged — the operator confirmed
  only the 1-row top band is the legibility problem.
- No multi-column layouts. The operator wants columns eventually; explicitly deferred.
- No new `docs/modules/mux.md`. Per the operator and the repo's Documentation Lifecycle,
  per-module design docs are not kept (they go stale); godoc and cobra `Long` texts carry
  the documentation.
- No `docs/roadmap.md` entry (this is hardening, not a planned milestone — per CLAUDE.md).
- No new sandbox-suite scenarios (`tools/sandbox/SANDBOX-MUX-SUITE.md` untouched; mux
  module coverage already satisfied — unit + smoke tests cover the new behavior).
- No non-Claude/engine work, no psmux upstream changes.

## Decisions

### commit-topbandrows-fix-here

- Decision: Reproduce the uncommitted 4-file diff from the `cluster-fork-spike` worktree's
  working tree (`git -C /home/knatte/Code/loomyard/wts/cluster-fork-spike diff` applies
  cleanly here — verified the branches share the base) and commit it in **this** worktree
  as the task's first batch, standalone from the mitigation work.
- Rationale: This session may not mutate the parent worktree's git state (worktree
  isolation); the fix is already reviewed/verified end-to-end, so landing it verbatim
  first gives every later batch (default bump, review-prompt update) a committed base to
  reference.
- Rejected: Operator committing it on `cluster-fork-spike` directly + merge-in (more
  round-trips); leaving it uncommitted (the task body explicitly asks for a decision).
- **Merge caveat (operator action):** the identical changes remain uncommitted in the
  `cluster-fork-spike` worktree's working tree. Before merging this branch back, discard
  them there (`git -C /home/knatte/Code/loomyard/wts/cluster-fork-spike checkout -- .`),
  or the merge collides with identical local modifications.

### top-band-rows-default-3

- Decision: Shipped `top_band_rows` default 1 → 3, in both `template_posix.yaml` and
  `template_windows.yaml`.
- Rationale: 1 row shows effectively nothing for any command beyond a bare status line.
  3 rows = status line + a wrap + air, matches the `collapsed_strip_rows`/`min_full_rows`
  neighborhood, and costs 2 rows per top band out of a 50-row default window. Strands that
  need more use the per-strand `--top-band-rows` override.
- Rejected: 2 (minimal but cramped); raising strip/min_full too (operator: only the top
  band is the problem).

### debug-log-config-key

- Decision: New mux.yaml key `debug_log: ${env:LYX_MUX_DEBUG:-0}`. The template supplies
  only the key and its env-resolved default; on the Config struct the field is a
  **`string`** (`DebugLog string`), so yaml.Unmarshal never chokes on non-numeric env
  input — the Go level→argv helper owns all parsing and validation (numeric check + 0–2
  range) and emits a clear error quoting the offending value (e.g. `invalid debug_log
  "abc": must be 0, 1 or 2`). `configengine.Load` validates key presence and resolves
  `${env:...}`, it does no type/range validation. Level 0 = off (default), 1 = `-v`,
  2 = `-vv`, appended to the psmux argv in `ensureServerAndSessionLocked`'s
  `spawnSession` (`internal/muxengine/lifecycle.go:174`). Invalid values are an error
  surfaced at boot.
- Migration note: the strict template contract means every already-initialized hub fails
  all `lyx mux` verbs with `missing keys: debug_log; run "lyx config reconcile"` once the
  template gains the key — including the sandbox Hub this logging is for. The plan must
  note the reconcile step for existing hubs, and operator-facing help (`up`'s `Long`)
  should not hide it.
- Rationale: The forensic use case is "armed for weeks in the sandbox Hub until the crash
  recurs" — a durable config value, not a flag someone must remember on the one `up` that
  actually boots the shared per-hub server. The `${env:...}` template mechanism gives the
  one-shot env-var opt-in for free. Supporting both `-v` and `-vv` as levels resolves the
  task body's open question ("is `-v` enough to catch a `fatalx()`?") by not forcing a
  choice: default recommendation is 1 (`-v`, low volume), with 2 available.
- Rejected: env-var only (not durable); `lyx mux up --debug` flag only (only effective on
  the boot-winning `up`, easy to miss); bool key (loses the `-vv` escalation path).

### server-log-under-dotlyx-logs

- Decision: Spawn the server with `cmd.Dir` set to `.lyx/logs/` (created if absent) so
  `tmux-server-<pid>.log` lands there, and pass the session's intended start directory
  explicitly (`new-session -c <cwd>`) so pane default cwd is unchanged. The path is
  `filepath.Join(e.layout.DotLyxDir(), "logs")` — `hubgeometry.Layout.DotLyxDir()`
  already owns the `.lyx` token, and muxengine already joins filenames onto it
  (`mux.json`, `mux.lock`), so no new hubgeometry helper is needed. At each server boot,
  prune `tmux-server-*.log` in that directory down to the newest 3. The cwd change and
  `-c` pinning apply on every boot; the `-v` flags only when `debug_log > 0`.
- Rationale: tmux offers no flag to redirect its `-v` log — it writes to the server's cwd,
  so cwd is the only control point. `.lyx` is machine-local, ephemeral, and **never
  git-tracked** (operator requirement) — the right lifecycle for multi-MB server logs,
  and the same home as mux's other runtime state (mux.json, mux.lock). Forensics happen
  on the machine that crashed, so machine-local is sufficient. Boot-time prune (keep
  newest 3) bounds growth across boots without runtime rotation machinery; `-v` volume
  within one server lifetime is low (the 19MB observation was `-vv` under deliberate
  stress).
- Rejected: `_lyx/logs/` (durable and weft-synced — would commit multi-MB logs into the
  weft repo; logs must not be git-tracked); `.scratch/` (routinely cleaned, bad for
  forensics); size-capped in-place truncation on every mux op (complexity; the file is
  held open by the server; YAGNI until a real week-long `-v` log proves too big); no
  rotation at all (task body explicitly asks for growth control).
- Accepted risk: psmux (Windows) is a tmux-derived clone; `-v`/`-vv` are assumed
  compatible. If a psmux version rejects them, the boot fails loud with the psmux error —
  acceptable for an opt-in debug mode; the live verification in this task runs against
  Linux tmux.

### resume-hint-in-requireSessionLocked

- Decision: `requireSessionLocked` loads persisted state when the session is absent; with
  ≥1 persisted strand the error becomes (shape final wording at plan time, keep both verbs):
  `no mux session (N strands persisted); run "lyx mux resume" to rebuild, or "lyx mux up"
  for a bare substrate`. With no state/strands, today's `no mux session; run "lyx mux up"`
  stays. The decision logic (state × strand count → message) is a pure helper for unit
  testing.
- Rationale: One chokepoint feeds status/add/remove and attach's pre-flight — every verb
  inherits the hint for free. `resume` is the verb that actually recovers from a server
  death; pointing only at `up` (substrate-only, never relaunches strands) sends the
  operator to the wrong verb.
- Rejected: enriching only attach/status (leaves add/remove misleading); proactive
  detection (out of scope).
- Note: tests asserting the exact old error string must be found and updated
  (`no mux session` appears in engine and cli tests) — and so must code comments quoting
  the old wording verbatim: `internal/muxengine/strand.go` (~lines 308, 351, 411) and
  `internal/muxcli/attach.go` (~line 53) cite `no mux session; run "lyx mux up"` in prose
  and go stale when the message is enriched.

### mitigation-only-scope

- Decision: No new reproduction attempts; no root-cause claim anywhere in code comments or
  help text — the crash mechanism is documented as unexplained.
- Rationale: Exhaustive targeted repro already failed while fully instrumented; the render
  corruption (the only known common factor) is eliminated by the committed fix, and the
  debug toggle is precisely the trap for the next occurrence.

## Technical context

- **Module layout:** `internal/muxcli` (cobra verbs, Idiom B envelopes) →
  `internal/muxengine` (engine ops, op lock, psmux subprocess overlay) →
  `internal/muxengine/render` (pure layout leaf: `Rules(strands, box, params, paneOrder)`).
  render never imports muxengine.
- **Server spawn:** `ensureServerAndSessionLocked` (`internal/muxengine/lifecycle.go`) —
  the only place the server process is created (`spawnSession` closure: `psmux -L <socket>
  new-session -d -s <session> -x W -y H <pwsh>`); boot-loop with zombie reaping around it.
  This is where `-v` flags, `cmd.Dir`, `-c <cwd>`, and the boot-time log prune belong.
  `CleanClaudeEnv` already filters the spawn env there — keep that intact.
- **Config:** `internal/muxengine/config.go` (`Config` struct mirrors mux.yaml) +
  `template_posix.yaml`/`template_windows.yaml` (strict template; `${env:VAR:-default}`
  resolution already supported by `configengine`). `config_test.go:70` asserts
  `TopBandRows == 1` — update to 3.
- **Layout policy:** `render/rules.go` — `anchor:top` bands get `p.TopBandRows` each
  (per-strand `Display.TopBandRows` override wins once the fix lands), last top band
  stretches when the below-parent stack is empty; `height.go` handles strip collapse and
  clamping. The default bump is config-only; no render code change beyond the re-applied
  fix.
- **Dead-session chokepoint:** `requireSessionLocked` (`lifecycle.go:698`), shared by
  Status (and via Status, attach's pre-flight in `internal/muxcli/attach.go`), AddStrand,
  RemoveStrand. Persisted state loads via `LoadState(e.layout.DotLyxDir())` (returns nil
  when no mux.json — see `loadOrInitStateLocked` in `spawn.go`).
- **The TopBandRows fix diff** (source of truth: `git -C
  /home/knatte/Code/loomyard/wts/cluster-fork-spike diff`): `render.Display` gains
  `TopBandRows int` (`json:"topBandRows,omitempty"`, 0 = inherit); `rules.go` prefers
  `s.Display.TopBandRows > 0` over `p.TopBandRows` (ordered before the last-top-band
  stretch, which still wins); `add.go` gains `--top-band-rows N` wired into
  `AddSpec.Display.TopBandRows`; two new render tests
  (`TestRulesTopBandRowsOverridePerStrand`, `TestRulesTopBandRowsOverrideIgnoredWhenZero`).
  Note: those tests use `Params{TopBandRows: 3, ...}` already, so they coexist with the
  default bump untouched.
- **tmux logging facts:** `-v`/`-vv` on the server-creating invocation enable server-side
  logging; the server writes `tmux-server-<pid>.log` into its own cwd; there is no
  redirect flag and no built-in rotation. `-vv` grew to 19MB in ~20 min of deliberate
  stress; `-v` is much lower volume. tmux's `new-session -c <dir>` pins the session's
  default start directory for panes independently of the server process cwd.
- **Multiplexer contract:** `internal/muxengine/doc.go` documents the assumed subcommand
  surface and load-bearing quirks; `contract_integration_test.go` canaries it against the
  real binary. `-v` is a server-invocation flag, not a subcommand — decide at plan time
  whether the contract doc's prose needs a sentence.
- **Prior art on Windows quirks:** the server cwd currently is the invoking process's cwd
  (the worktree), and lifecycle comments note the server + `__warm__` helper holding the
  worktree directory busy on Windows. Moving the server cwd to `.lyx/logs/` keeps it
  inside the worktree — no behavior change for the busy-directory concern.
- **`.lyx` vs `_lyx`:** `hubgeometry` deliberately distinguishes `.lyx` (dot: ephemeral,
  machine-bound, never weft-synced/git-tracked; `Layout.DotLyxDir()`) from `_lyx`
  (underscore: durable, weft-synced; `LyxDirName`/`ConfigDir`). mux.json and mux.lock
  live in `.lyx`; the server logs join them there.

## Constraints

From `CONSTRAINTS.md` (read in full this session), the ones this task touches:

- **Hub Geometry Invariant:** the `.lyx` path resolves through
  `hubgeometry.Layout.DotLyxDir()` (muxengine already does this for mux.json/mux.lock);
  no raw `_lyx`/`.lyx` string joins in path construction outside hubgeometry.
  `TestEnforcement_GeometryLiterals` enforces on every `go test`.
- **CLI / Cobra Invariant:** no new commands, but help accuracy — `up`'s `Long` must
  mention the debug toggle; `add`'s flag help ships with the re-applied fix. Errors stay
  on the JSON envelope (`output.Err`); the enriched `requireSessionLocked` message rides
  the existing envelope path unchanged.
- **Test Tier Purity Invariant:** new unit tests (level→argv mapping, prune planning,
  error-message decision) must spawn nothing; the live boot-with-debug test goes behind
  the `smoke` (or `integration`) tag.
- **Hermetic Git Test Environment Invariant:** if any new test file lands in a package
  whose tests spawn git/processes, keep the `TestMain`/`HermeticGitEnv` requirement
  satisfied (the mux test packages already comply).
- **Shuttle Provider-Seam:** untouched — this task adds no provider specifics.
- **Documentation Lifecycle:** no module doc created (deliberate, see Scope Out).

## Testing

- **Re-applied fix batch:** carries its own two render unit tests; `go build ./...`,
  `go vet ./...`, full `go test ./...` must stay green (the fix was already verified
  end-to-end in the parent worktree).
- **Default bump:** update `config_test.go`'s `TopBandRows` expectation 1→3; grep for
  other tests pinning the old default.
- **Debug level → argv mapping:** TDD candidate. Pure helper (level string → extra argv
  slice; numeric parse + 0–2 validation with a clear error quoting the value) with
  table-driven unit tests, untagged.
- **Log prune planning:** TDD candidate. Pure helper (existing filenames+mtimes → files
  to delete, keep newest 3) unit-tested untagged; the caller does the actual `os.Remove`.
- **Resume-hint decision:** TDD candidate. Pure helper (state present? strand count →
  message) unit-tested untagged; update every test asserting the old
  `no mux session; run "lyx mux up"` string.
- **Live verification (one tagged test):** boot with `debug_log: 1` against real tmux and
  assert a `tmux-server-*.log` exists under `.lyx/logs/` and that old logs get
  pruned; follow the existing `smoke_*_test.go` pattern (skip when the binary is absent,
  deadline-based waits, teardown discipline, HermeticGitEnv TestMain already in package).
- **Help/registration guards:** `cmd/lyx` drift/helptree tests must stay green (no new
  subcommand, so no pinned-set update expected).
- **Review prompt:** `docs/reviews/mux-review-prompt.md` gains the new behaviors in its
  invariant/driving lists (debug toggle boots with a log under `.lyx/logs/`, resume hint
  on dead server, top-band default 3, per-strand override) so future adversarial reviews
  drive them.

## Q&A log

- **Q:** What does "all panes show something sensible" mean concretely? **A:** The 1-row
  top-band pane is the problem — you see nothing at 1 line; it must be taller. Strip and
  min-full defaults stay.
- **Q:** How to land the uncommitted TopBandRows fix? **A:** Commit it in this worktree
  (re-apply the diff); operator discards the parent worktree's uncommitted copy before
  merge-back.
- **Q:** Debug toggle mechanism? **A:** Config key with env-template default
  (`debug_log: ${env:LYX_MUX_DEBUG:-0}`), levels 0/1/2 → off/`-v`/`-vv`.
- **Q:** Config field type for `debug_log` — `int` (yaml errors cryptically on
  non-numeric env input) or `string` (Go helper owns parsing + clear error)? **A:**
  `string`; the helper validates and quotes the offending value. (Operator delegated the
  call.)
- **Q:** Log location and growth control? **A:** Machine-local, never git-tracked —
  `.lyx/logs/` via the hubgeometry seam (`DotLyxDir()`), same home as mux.json; prune at
  boot keeping newest 3; no runtime rotation. (Reviewer gap: the first draft said `_lyx`,
  which is weft-synced — operator ruled logs must not be git-tracked.)
- **Q:** Dead-server notice? **A:** Enrich the shared `requireSessionLocked` error — all
  verbs inherit; suggest `resume` when persisted strands exist.
- **Q:** New shipped `top_band_rows` default? **A:** 3. (Multi-column layouts noted as a
  nice-to-have later — explicitly deferred.)
- **Q:** More crash-repro in scope? **A:** No — mitigation and forensic prep only.
- **Q:** Test approach? **A:** Pure unit tests for the new helpers + one tagged live test
  for the log landing under `.lyx/logs/`; also update `docs/reviews/mux-review-prompt.md` to
  cover the new mux behaviors.
- **Q:** Create `docs/modules/mux.md`? **A:** No — per-module docs always go stale;
  godoc + `Long` texts carry documentation (matches the repo's Documentation Lifecycle).
