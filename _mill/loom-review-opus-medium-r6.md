# `loom` — independent review — ROUND 6 (opus-medium-r6) — SAFETY PASS

> Clean-room round 6 of the quarry-glyph-plan-alphabet crucible campaign.
> Worktree: `/home/hanf/Code/loomyard/wts/crucible-loom-glyph-hardening`, branch `crucible-loom-glyph-hardening`, HEAD at round start `12f865288`.

## Status

IN PROGRESS — Job 1 (clean-room review) under way. This section is replaced by the executive summary when the findings list is complete.

## What was tested

(appended incrementally, immediately after each command/scenario returns)

### Environment / gates (round 6)

- `git rev-parse --show-toplevel` → `/home/hanf/Code/loomyard/wts/crucible-loom-glyph-hardening`; `git branch --show-current` → `crucible-loom-glyph-hardening`. Confirmed, not assumed.
- `claude --version` → `2.1.263 (Claude Code)` — the same provider build round 5 transcribed its two gate captures from.
- `CGO_ENABLED=1 go build ./...` → clean.
- `CGO_ENABLED=1 go vet` over the full round-6 package set → clean.
- `CGO_ENABLED=1 go test -count=5` over the round-6 package set + `./cmd/lyx/...` → all green (exit 0).

### Live substrate — BLOCKED IN THIS ENVIRONMENT (stated plainly, not skipped)

This round could NOT drive live substrate. Every live-substrate entry point is refused by this session's own permission
classifier, not by the code and not by my choice:

- `tmux new-session ...` → denied ("Blocked by classifier").
- `lyx reed up` → denied ("Blocked by classifier").
- `CGO_ENABLED=1 go run ./tools/deploy` → denied ("Blocked by classifier"), twice.

Consequence, recorded honestly: no `lyx loom run`, `lyx webster run`, `lyx shuttle run`, crash-kill, or fresh-fixture
gate transcription was possible. Round 6's item 2 was therefore driven by READING plus hermetic tests only. The
never-hand-driven-fixture discipline the prompt makes BLOCKING for any live scenario was honoured vacuously: no live
scenario was attempted at all, so no immunized fixture was reused either.

The PATH binary (`/home/hanf/go/bin/lyx`, built 12:14) predates HEAD by two commits (`7025ffb1c`, `187161faa`,
14 lines across `internal/{burler,webster}cli/wiring.go`); it DOES contain both round-5 startup fixes
(`1640a59bb`, 12:13:52). It could not be refreshed because deploy is blocked. No live conclusion is drawn from it.


### Provider-startup seam — driven by reading + a hermetic probe (item 2)

Ran a throwaway probe test over `claudeengine.Startup` / `TrustDismissSequence` with realistic
working-pane captures (test file deleted again afterwards; the permanent regression tests land with the fix):

| capture | `Startup` | `TrustDismissSequence` |
|---|---|---|
| `● I'll start by reading the files in this folder.` + `❯` + bypass footer | `StartupTrustPrompt` | `[]` (nothing pressed) |
| `● You asked whether to trust this folder; I'd say yes.` + `❯` + `? for shortcuts` | `StartupTrustPrompt` | `Up, Up, Enter` — **keys played into a live agent's pane** |
| `● Reply with "Yes, I accept" to continue.` + `❯` + `? for shortcuts` | `StartupTrustPrompt` | `Up, Up, Enter` — **keys played into a live agent's pane** |
| `● Reading loom.md` + `❯` + bypass footer | `StartupReady` | `[]` |
| `Do you trust the files in this folder?` / `❯ 1. Yes, proceed` / `2. No, exit` | `StartupTrustPrompt` | `[]` (**a gate the code recognizes but can never dismiss**) |

