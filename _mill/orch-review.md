# Orchestrator review: hubforge-parallel-chdir discussion.md

Reviewed against the task description (`status.md`: "Unblock t.Parallel on hub-fixture tests that currently t.Chdir") and against the current `main` codebase.
Phase is `discussing`, no plan yet — this is a pre-plan sanity check of the discussion doc itself.

## Verdict

Sound. No blocking defects found.
The document is unusually well-grounded — every file:line citation I checked against the live `main` tree resolved exactly, including several non-obvious ones (`configcli.go:384`'s nested `fabriccli.RunCLI` call, `restoreCwd`'s two call sites in `preflight_integration_test.go`, the exact 228/395 `t.Parallel()` split in `fabricengine`, the `sink.go:79` short-circuit that makes `sink.go:88`'s `Getwd()` dead in tests).
Ready to move to `mill-plan` as-is, modulo the two trivial citation fixes below.

## Verified against `main`

- All ~12 "sites listed under Technical context" resolve: the "~12" figure in Scope is the PersistentPreRunE-group (7 literal `Getwd()` calls + `weft_verbs.go:52`) plus scoutcli's 4 `RunE`-body calls = 12 — it deliberately excludes the 4 plain-handler sites and `preflight.go`, which are separate Scope bullets. Not a miscount; the two numbers just answer different questions. Worth a one-line footnote in Scope so a plan-phase reader doesn't do the same double-take I did.
- Every `lyxcwd.Getwd()` production call site (17 total, `sink.go:88` correctly excluded as unreachable under `testing.Testing()`) matches the Technical Context table exactly, including all 4 `scoutcli` line numbers and the `weft_verbs.go:52`/`fabric.go:398` split (the former is a `PersistentPreRunE` that calls the latter — it's a real second migration touch-point, not a double-count of the same site, since it needs `resolveWarpLocation(cmd.Context())` threading independent of the `Getwd()` swap inside `resolveWarpLocation` itself).
- `.Chdir`/`t.Setenv` per-file counts in the Technical Context table are exact for every file checked (`fabriccli/cli_test.go`: 18 `t.Chdir` calls, 3 more mentions live only in comments; `configcli_integration_test.go`: 3 calls, 1 comment-only mention) — a naive `grep -c` overcounts both files by including comment lines, so whoever built this table filtered those out correctly.
- `fabricengine`: 228/395 `t.Parallel()` — exact. `coalesce_integration_test.go`'s two `.Chdir` occurrences are both inside `TestCoalescePushBothAt_EmptyWarpPath_PushesWeftFromUnrelatedCwd` (chdir + its own restore) — exact, and the "removing it deletes the test's meaning" call is correct on inspection: the test's assertion is specifically about `gitrepo.New("")` resolving through the *unrelated* process cwd.
- `WithCwd`/`CwdFrom`/`ExecuteIn`/`RunCLIIn` do not exist anywhere in the tree yet — confirmed this is genuinely new surface, not a rediscovery of dead work.
- `perchcli/cli_integration_test.go`'s `../../escaped` assertion (`os.Stat(filepath.Join("..","..","escaped"))`) is exactly as described — relative to the chdir'd cwd, and the doc's proposed fix (derive an absolute path from the hub) is the correct minimal rewrite, not a weakening.
- `restoreCwd` at `preflight_integration_test.go:109` is used only by the two named tests (`TestPreflight_NotAGitRepo`, `TestPreflight_SubpathAnchoredHubIsNotRejectedForItsAnchor`) — the "becomes dead and should be deleted" claim holds structurally: no third caller would be left stranded.
- `CONSTRAINTS.md:8` (Cwd Resolution Invariant) and `:70` (hubforge Fabric-Fixture Invariant, "safe under concurrent use" is the literal line-79 text) match the doc's characterization precisely, including the `root`-means-worktree-root / `cwd`-means-cwd distinction the Decisions section is careful never to violate.

## Minor issues (non-blocking, fix opportunistically)

1. **Two off-by-N line citations in the "already safe" appendix**, both outside the task's actual touch surface so they don't affect scope or the plan, but worth correcting since precision is otherwise this doc's strength:
   - `internal/hubforge/hub.go:58` (the leaked `os.MkdirTemp` template) is actually at **line 56**.
   - `docs/benchmarks/running-tests.md:29` (the smoke-tier live-session requirement) is actually at **line 30**.
2. **`weft_verbs.go:52`'s classification is slightly compressed.** It's grouped under "Cobra `PersistentPreRunE`" alongside sites that call `lyxcwd.Getwd()` directly, but it doesn't call `Getwd()` itself — it calls `resolveWarpLocation()` (the real `Getwd()` call already listed separately under "Plain handler functions"). The distinct migration action there is threading `cmd.Context()` into that call, which the "plain-handlers-take-the-context" decision does cover — so nothing is missing from Scope, but a reader skimming only the Technical Context table could double-count it as an 18th `Getwd()` site. A parenthetical ("threads context into `resolveWarpLocation`, not a direct `Getwd()` call") would close the gap.

## Not re-verified (inherently unverifiable from a static read)

- The timing numbers (3.3s/15.5s/4.97s/131.7s, the ~26× Windows multiplier) — these require running the suite on the cited hardware, which this review didn't do. The document is honest about the small measured payoff and doesn't lean on the timing numbers to justify scope, so this doesn't gate anything.
- Windows-specific claims (junction-removal LIFO ordering, the Cortex-XDR timing) — no Windows environment available here.

## Scope check against the task description

Task description is exactly "Unblock t.Parallel on hub-fixture tests that currently t.Chdir." The Scope/Out-of-Scope split (integration tier only, smoke tier and the `WEFT_SKIP_*` env seam explicitly deferred with reasons, `coalesce_integration_test.go` explicitly left untouched with a correct rationale) matches the brief's literal scope without silently expanding into the smoke tier or quietly re-litigating `hubforge` itself, which `lyxtest-real-hubs` already landed and pinned in `CONSTRAINTS.md`. Good discipline on not re-opening `internal/hubforge` even though this task's title implies "hub-fixture tests."
