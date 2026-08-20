# Orchestrator review — discussion.md

Reviewed against `main` (`dce9fde8`, unchanged in this worktree — only `_mill/discussion.md`/`status.md` exist so far) and the actual state of `/home/knatte/Code/quarry/wts/quarry`.

## Citation check

The largest citation surface of the three discussions reviewed so far, spanning two repos. Verified every concrete claim.

**Quarry repo state:** confirmed genuinely empty — `git status` shows "No commits yet," branch `main`, only a `.git` directory. Matches the "Repo state" section exactly.

**Byte/line counts:**

| Claim | Status |
|---|---|
| `lspclient.go` 22 827 bytes | Correct, exact |
| `ensureserver.go` 27 529 bytes | Correct, exact |
| `doc.go` 18 118 bytes | Correct, exact |
| `internal/scoutcli` ~1 900 lines | Correct (actual 1 962) |
| `internal/scoutengine` ~5 700 lines with tests | **Off** — actual total is 7 011 lines across 35 `.go` files (2 737 non-test + 4 274 test), about 23% higher than stated. Not load-bearing for any decision — the Problem section uses it only to establish scale — but worth a quick fix since it's a concrete, checkable number that's simply wrong. |
| `internal/lock` 222, `internal/proc` 329, `internal/output` 256 | Correct |
| `internal/configengine` 1 552, `internal/logger` 1 939, `internal/clihelp` 923, `internal/lyxcwd` 3 028, `gitkit`+`hubforge` 2 192 | Correct, all exact |

**Line-precise code citations — all correct, including several that would be easy to get wrong:**

- `internal/scoutcli/cli.go:453` — exact line of `func lookupContext(cwd, dir string) (...)`, and the quoted code excerpt (lines ~460-476, with `…` eliding the error-handling branch) matches the real function's shape faithfully.
- `internal/scoutengine/daemonstate.go:39,47` — exact lines of `DaemonStateFile`/`DaemonLock`.
- `internal/scoutengine/ensureserver.go:305` — exact line of the socket-path join.
- `cmd/lyx/main.go:32,76,110`; `helptree_test.go:28,107-108`; `seamsignature_test.go:23,41,63`; `hermeticenv_test.go:38`; `notransients_test.go:7,27,82-83`; `constructoranchoring_test.go:7,25,45,106-107`; `configstrictness_test.go:17` — every one checked out exactly, both the line number and the quoted/paraphrased content.
- `CONSTRAINTS.md:195-208` (Scout Engine-Seam Invariant section), `:77` (told-geometry review-obligation list) — correct.
- `docs/overview.md:190,293,431` — correct.
- `manifest/roadmap.md` lines 51, 138, 148, 150, 160, 172-174, 206, 263 — all nine/ten references checked individually; every one is genuinely scout-related and at the stated line.
- The six files in the "audit each and reword" catch-all (`loom-plan-spec.md`, `loomshed.go`, `websterengine/doc.go`, `gitrepo/doc.go`, `fabriccli/clone.go`, `fabricengine/junction.go`) — confirmed all six actually contain "scout" mentions.

**The used-surface table is genuinely measured, not asserted:** confirmed `lyxcwd`/`gitkit`/`hubforge` appear in zero `scoutengine` non-test `.go` files (only as prose inside `doc.go`'s own comments, correctly distinguished from an import), and confirmed `gitkit`/`hubforge` are exclusively referenced from the three named integration-test files. The "engine seam holding" claim is a directly verified fact, not a description of intent.

**One finding outside the discussion's own scope:** `CONSTRAINTS.md:440`, quoted accurately by the discussion, itself cites `manifest/roadmap.md:98` for the `scout-redesign.md` prose-mention example — but that reference now lives at `roadmap.md:138` (roadmap line numbers shifted in the `b01ffc3b` cleanup earlier this session). This is a pre-existing staleness in `CONSTRAINTS.md` unrelated to this discussion's own reasoning; flagging it only because the task already touches this exact line to repoint the example, so fixing the line number is free while there.

## Design read

**The measured-not-assumed dependency split (`dependency-strategy-copy-vs-replace`) is the strongest piece of this discussion.** Nine packages / ~11 500 declared lines collapses to 18 used symbols, and the copy/replace criterion (does the package have its own internal dependencies) is applied consistently — `lock`/`proc`/`output` are genuinely leaf packages, and the five replaced ones each get a named, sized replacement rather than a vague "reimplement as needed."

**`config-and-state-paths` correctly identifies a real conflation in the current code** — `lookupContext`'s `loc.AnchorPath()` serving as both the config base and the daemon `anchorRoot` — and the fix (split onto `os.UserConfigDir()`/`os.UserCacheDir()`, stdlib-native per-OS behavior) is proportionate. The self-flagged Unix-socket-path-length risk (108-byte cap) is a genuine, non-obvious platform constraint worth carrying into the plan as a named test, not just a mention.

**`mechanical-move-not-hand-transcription` is the right call for an 8 900-line port** — a written, throwaway Go program for the import/package rewrite is deterministic and diff-reviewable in a way that transcription isn't, and the rationale correctly separates it from `git filter-repo` (rejected for the right reason: paths don't match the new layout, so grafted history would point at nothing).

**`ordering-quarry-green-before-lyx-deletion` and the testing plan's step 4 (behavioural equivalence against the five benchmark symbols before deletion) are the two decisions that make this task verifiable rather than just plausible** — sequencing the irreversible half last, and pinning a concrete before/after comparison rather than "tests pass in both repos," are both doing real work, not just process theater.

No decision looks wrong. `lyx-side-removal-is-total` (delete outright, no shell-out) is well-argued from the cited benchmark finding, not just asserted. `history-not-preserved` correctly identifies that filtered history would point at paths that no longer exist, which is a real cost, not a hand-wave.

## Verdict

Sound. Nothing here should block moving to Plan. Two small fixes worth folding in while the discussion is still open, neither worth a full round on its own: correct "~5,700 lines" to the actual ~7,000 in the Problem section, and (optional, free while `CONSTRAINTS.md:440` is already being touched) fix its stale internal citation from `roadmap.md:98` to `:138`.
