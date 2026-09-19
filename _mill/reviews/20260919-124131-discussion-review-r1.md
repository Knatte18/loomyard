# Review: Shed-generic watchdog for ly-drive and loom's CLI verbs

```yaml
verdict: REQUEST_CHANGES
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [NIT:consistency] lifecycle PreRun: Decision contradicts Technical context
**Demoted-from:** BLOCKING
**Section:** `optional-hooks-for-module-specific-work` vs Technical context ¶ `internal/lifecyclecli`
**Issue:** The Decision says "`lifecyclecli` fills `PostRun` only … and leaves the rest nil", but Technical context says the seed-when-absent behaviour "is lifecycle's `PreRun` and must not leak into the generic body" — with `PreRun` nil, the generic `run` body has no host for lifecycle's status decode, `StateDone` refusal, unrecognized-state refusal, and seeding (all verified present in `internal/lifecyclecli/run.go`), and the module's-own-`RunE`-wrapper alternative is explicitly rejected.
**Suggested fix:** Correct the Decision: `lifecyclecli` fills `PreRun` (decode/refusals/seed) and `PostRun` (`abandonedSession`), leaving `PreStep`/`InterruptPolicyFor` nil.

### [BLOCKING:design] Per-module flags and positional args unaddressed in shedcli's constructor
**Section:** `shedcli-owns-the-verb-bodies` / `named-recipe-table-arming` / Testing ¶ Parity
**Issue:** `lyx loom step` carries a `--parent` flag feeding the bootstrap (verified in `internal/loomcli/step.go`), and every lifecycle verb takes a `<slug>` positional arg with `cobra.ExactArgs(1)` (verified in `internal/lifecyclecli/run.go`/`status.go`) that lifecycle's arming needs before it can fill `Env`/`ShedPaths` — yet `Verbs(spec)` and the `Hooks` signatures offer no stated way for a module to register extra flags/args or for hooks to read them, and the parity test `lyx lifecycle run` vs `lyx shed run --recipe lifecycle` presupposes an answer for where the slug goes.
**Suggested fix:** State the mechanism — e.g. the arming module decorates the returned commands with its own flags/`Args` validators and hooks capture them by closure, and `lyx shed`'s per-recipe arming declares its positional-arg contract.

### [NIT:decision] ErrShedBusy treatment is a hedged recommendation, not a Decision
**Section:** Technical context ¶ `internal/loomcli/run.go`
**Issue:** The three-way `ErrShedBusy` divergence is real (verified), but its resolution — "the cleanest route is a told busy-message/kind on the arming spec" — is phrased as a suggestion inside Technical context rather than settled as a Decision the plan must follow.
**Suggested fix:** Promote it to a Decision (told busy-message/kind on the arming spec) or fold it into `optional-hooks-for-module-specific-work`.

## Verdict

REQUEST_CHANGES
Two blockers: the lifecycle PreRun contradiction and the unspecified per-module flag/arg surface; everything else verified clean.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
