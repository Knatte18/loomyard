# Session-fork cluster review — spike findings

**Task:** `session-fork-diversity-spike` (retry of `cluster-fork-spike`). **Date:** 2026-07-16.
**Verdict up front: the fork-based cluster design is viable and profitable.** M1 and M2
pass cleanly, fork-cluster coverage is on par with (or better than) independent cold
reviewers, and a forked reviewer costs less than half of a cold one. One design surprise:
hard "ignore everything else" lenses were *not* load-bearing — identical-prompt forks
diverged strongly on their own and out-covered the lens arm.

Rig: `tools/fork-poc/` (prompts, spawn/wait/harvest helpers). Raw session outputs:
`tools/fork-poc/results/`. All live sessions ran as `lyx mux` strands in the sandbox hub
(`~/Code/lyx-test-HUB/lyx-test`), watched live by the operator via `lyx mux attach`.

## Step 0 — transcript persistence (attempt 1's blocker)

**Resolved, and the fullscreen hypothesis is refuted.** A plain interactive `claude`
spawned as a mux strand persisted its `.jsonl` within ~10 s — with `"tui": "fullscreen"`
still set in `~/.claude/settings.json`. The actual root cause of attempt 1's "no
transcript ever appears": a claude launched from inside a Claude Code session inherits
`CLAUDECODE`/`CLAUDE_CODE_*` and treats itself as a nested child, silently not
persisting its transcript. `muxengine.CleanClaudeEnv` (the env-hygiene chokepoint on
server spawn) is the fix, already proven end-to-end by
`internal/muxcli/smoke_resume_test.go:TestSmokeClaudeResumeRecallsCodeword`.
**Every resume/fork-based design in lyx inherits this constraint: agent-spawning paths
must scrub `CLAUDECODE`/`CLAUDE_CODE_*` (mux already does).**

## Method

- **Explorer:** one session reads all of `internal/modelspec/` (11 files, ~1.2k lines)
  plus a nonce; its session id is preassigned via `claude --session-id <uuid>` so the
  transcript path is deterministic.
- **Forks:** `claude "<prompt>" --resume <explorer-id> --fork-session --session-id <uuid>`
  in a fresh mux strand. Prompt must be the FIRST positional arg — variadic
  `--add-dir <dirs...>` swallows a trailing prompt (cost one debugging round).
- **Measurement:** forked transcripts copy the parent's full history, so all usage and
  tool-call accounting splits at the fork point (first user message matching the fork's
  prompt). `usage` blocks in the `.jsonl` carry full token columns per assistant message;
  `compute = input + cache_creation + output` is the headline (tokens processed fresh).
- **Done-signal:** transcript-quiet alone false-fires twice over (launch gap before the
  first flush; long thinking pauses mid-turn). The reliable signal is transcript quiet
  AND the pane no longer showing claude's "esc to interrupt" spinner. Claude Code Stop
  hooks are the robust long-term mechanism (per the CLAUDE.md agent-execution design);
  the pane-grep was sufficient for the spike.
- All sessions ran `claude-sonnet-5` (operator default), except M2's fork (opus).

## M1 — does a fork inherit explored context? PASS

Fork probed with tools forbidden: nonce, `Parse` signature, `Spec` fields,
`LoadRegistry` missing-file behaviour. **4/4 correct, 0 post-fork tool calls.**
Post-fork cost: 38k compute (mostly cache re-creation of the inherited context) vs the
explorer's 157k exploration — the fork skipped the entire explore phase.

## M2 — fork + model switch in one invocation? PASS

Same probe with `--model opus`: ran as `claude-opus-4-8` (parent was sonnet-5),
**4/4 correct, 0 tool calls**. The model-per-fork axis is real; this doubles as the
minimal B3 signal.

## Q2 — diversity (union coverage)

Three arms, N=3 each, reviewing `internal/modelspec`:
**B1** forks with an identical generic review prompt; **B2** forks with one hard lens
each (correctness / error-handling / test-gap, each told to ignore the other
categories); **A** cold sessions with the same three lenses, each exploring from
scratch. Raw outputs in `tools/fork-poc/results/`; findings deduped into clusters by
the orchestrating session (rubric: same file + same underlying defect = one cluster,
production-code and test-gap clusters counted separately).

| arm | raw findings | distinct clusters (prod + test) |
|-----|-------------:|--------------------------------:|
| B1 fork, identical prompt | 14 + 17 + 12 = 43 | **25** (17 + 8) |
| B2 fork, lens per fork | 2 + 7 + 12 = 21 | **20** (7 + 13) |
| A cold, lens per fork | 0 + 10 + 11 = 21 | **20** (9 + 11) |

- **Criterion "no diversity loss" holds:** B2 = A on union coverage (20 = 20), and B1
  beats both. All six forks united cover 36 clusters vs cold's 20.
- **The central lens hypothesis is NOT confirmed:** B1 > B2. Identical-prompt forks
  diverged substantially (each contributed 4–7 clusters its siblings missed — sampling
  diversity is strong on Claude). Hard lenses *suppressed* coverage two ways: the
  "ignore other categories" clauses cut cross-category findings, and a lens whose
  category is empty wastes its fork (`modelspec` is clean on correctness — both the
  fork and the cold correctness reviewer returned essentially nothing, a consistent
  negative that itself validates judge agreement).
- Notable convergence: the zero-value-`Spec`/`Resolve` shape hole and the
  incidental multi-bracket rejection were found by every B1 fork; the empty-alias-key
  hole was found by B1 and A but missed by all of B2.

## Q3 — token throughput (compute = in + cache_cr + out)

| session | in | cache_cr | cache_rd | out | compute |
|---|--:|--:|--:|--:|--:|
| explorer | 14,540 | 129,148 | 496,114 | 12,902 | 156,590 |
| b1-1 / b1-2 / b1-3 (post-fork) | 1,676 ea | ~74.7k ea | 54,492 ea | 25–30k | 106,486 / 101,574 / 105,782 |
| b2 corr / eh / tg (post-fork) | 1,676 ea | ~74.8k ea | 54,492 ea | 14–31k | 107,358 / 92,282 / 90,626 |
| a corr / eh / tg (whole session) | ~18.5k ea | ~125–172k ea | ~540–758k ea | 31–40k | 198,066 / 199,098 / 231,056 |

- **Arm totals (N=3):** B2 = 156,590 + 290,266 = **447k**; B1 = **470k**; A = **628k**.
  Criterion "cheaper" holds: the fork arm is ~29% cheaper at N=3.
- **Marginal reviewer:** fork ≈ 97k avg vs cold ≈ 209k avg — **2.16× cheaper per
  reviewer**. The explorer amortizes, so the arm-level saving grows with N
  (extrapolated N=5: ~641k vs ~1,047k, ~39% cheaper) and approaches 2.16× asymptotically.
- **Cache secondary reading (API-relevance):** a fork's first turn shows
  cache_read ≈ 54k and cache_creation ≈ 75k — the fork reuses part of the parent's
  prefix cache but re-creates the rest. On API billing the fork saving would be partial,
  not the full explore margin; on subscription the compute saving above is what counts.
  What the fork fully skips is the explore phase's 12 tool calls / 15 turns.

## Decision

Both criteria hold → **forking is worthwhile as the cluster-review mechanism.**

Recommendations for the eventual burler/perch cluster design:

1. **Fork one explorer into N reviewers** — mechanics proven (M1, M2), cost is less
   than half per reviewer, coverage does not drop.
2. **Do not use hard exclusion lenses.** Identical or lightly-steered prompts
   out-covered strict lenses; if steering is wanted, phrase it as emphasis
   ("pay extra attention to X"), never as "ignore Y". Lens-per-fork also wastes a fork
   when its category is empty on the target.
3. **Model-per-fork is available** as a diversity axis (M2), untested for coverage
   effect at N>1 (B3 proper was skipped; B1's result makes it non-blocking).
4. Preassign `--session-id` per fork; account usage post-fork-point from transcripts;
   detect completion via Stop hooks (pane-idle grep is the stopgap).

## Caveats

One module (small, clean, leaf), one target repo, one run per arm, N=3, one judge
(the orchestrating session; raw outputs committed for audit). The B1-beats-B2 result
in particular deserves a re-check on a buggier / larger target before it hardens into
design doctrine. No wall-clock comparison is claimed (transcript timestamps proved
unreliable for it).

## Incidental findings (outside the spike's questions)

- **mux config schema drift:** the sandbox hub's `mux.yaml` still had the Windows-era
  `psmux:`/`pwsh:` keys; the deployed binary refused with a confusing
  "psmux.exe not found in $PATH". `lyx config reconcile --apply` fixed it — but the
  error message names a binary instead of a config-schema mismatch.
- **mux has no visible pane naming.** Strand names exist only in `lyx mux status`
  JSON; the operator watching `lyx mux attach` cannot tell panes apart. The spike
  labelled panes by hand: per pane `tmux set -p @lyxname <strand-name>` plus global
  `pane-border-status top` and `pane-border-format " [#{@lyxname}] #{pane_title} "`.
  mux setting these itself at add/render time is a cheap, high-value improvement.
- **Mouse mode:** default is `off` by design (mux-mouse-default task); live enable on
  a running server works via raw `tmux set -g mouse on`, while the supported path
  (`mouse: on` / `LYX_MUX_MOUSE=on`) needs a server reboot.
- `internal/modelspec` review findings themselves (the strongest recurring ones: the
  `Resolve` zero-value/shape hole, empty-alias-key acceptance, multi-bracket
  mis-rejection, `builtins()` vs `template.yaml` drift) are raw material for a real
  review pass — see `tools/fork-poc/results/`; they were produced by a throwaway
  measurement, not a burler round, and should not be fixed off this doc alone.
