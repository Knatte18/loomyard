# `fabric merge` — independent review (round `opus-medium-r1`)

Reviewer: crucible round agent, clean-room, round 1 of the `fabric merge` campaign.
Surface under review: the merge primitive as shipped by `a2bf44e2`.

> Sections below are appended as work proceeds (log-as-you-go).
> The executive summary and the final severity ordering are written last.

## What was tested

(appended per command/scenario)

## Findings

(appended as formed)

### Gates (baseline, committed tree)

- `go build ./...` — **green**.
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — **green** (no output).
- `go test -count=1 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...` — **green** (tag: none / hermetic).
- `go test -tags integration -count=1 -timeout 30m ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — **green** (tag: `integration`; fabricengine 23.8 s, fabriccli 2.4 s, gitrepo 1.6 s).
- `./deploy-dev` — deployed `lyx @ 125757f4` to `.dev-bin/lyx`; all live driving below runs that binary.

### Live substrate

Scratch hubs are built outside the worktree, under the session scratchpad, by
`mkhub.sh` (bare warp remote with a seeded `main`, empty bare weft remote, then
`lyx fabric clone <weft-bare> <warp-bare>`) and `mkscen.sh <name> <mode>` which adds a
`task1` pair and creates a `clean` / `conflict` / `weftonly` divergence.
Each destructive scenario gets its own freshly built hub. `GIT_CONFIG_GLOBAL` is
pinned per hub so the host's git config never leaks in.

### Scenario L1 — plain two-sided `merge` that fast-forwards both sides (hub `h1`)

`lyx fabric merge task1` after both branches advanced ahead of `main`/`main-weft`:

```
{"already_up_to_date":false,"committed":true,
 "mutations":[{"kind":"merge_staged","target":"warp-bare","detail":"d140036…"},
              {"kind":"merge_staged","target":"warp-bare-weft","detail":"e824beb…"}],
 "ok":true,"partial":false}
```

Both sides fast-forwarded (warp log shows `task1: warp f1` as HEAD, no merge commit).
Observations: `committed: true` although no conclude-commit landed on either side, and
no `merge_committed` entry appears. `KindMergeStaged`'s godoc does say it covers the
fast-forward case, so the mutation kind is intentional — the `committed` flag is not
documented anywhere and is what a script would read. See finding **F6**.

### Scenario L2 — `merge-in` conflict → sibling refusals → `--continue` (hub `s_conflict`)

`lyx fabric merge-in task1` with a conflicting `shared.txt` on warp and
`_lyx/shared.txt` on weft:

```
{"conflicts":["_lyx/shared.txt","shared.txt"],
 "error":"merge produced conflicts; resolve them, then run \"lyx fabric merge --continue\"",
 "mutations":[{"kind":"merge_staged",…}×2],"ok":false,"partial":false}   exit 1
```

Path unification is correct (weft `_lyx/…` is a wired name at anchor `.`, so it maps by
identity). `fabric-merge.json` in the weft gitdir records
`warp_outcome=conflicted, weft_outcome=conflicted`. Both checkouts show `AA` for the
conflicted path — record and git agree.

Sibling verbs while the record exists (all run from the prime warp worktree):

| verb | result |
| --- | --- |
| `fabric commit` | refused, `ErrMergeInProgress` |
| `fabric pull` | refused, `ErrMergeInProgress` |
| `fabric push` | refused, `ErrMergeInProgress` (commit half) |
| `fabric sync` | refused, `ErrMergeInProgress` |
| `fabric checkout task1` | refused, `ErrMergeInProgress` |
| `fabric remove task1` | **SUCCEEDED** — removed the merge's source pair and deleted branch `task1-weft` mid-merge (finding **F5**) |
| `fabric add task2` | refused, but with the unrelated `source worktree has uncommitted changes` message |
| `fabric cleanup --apply` | ran unguarded (no-op here) |
| `fabric prune --apply` | ran unguarded (no-op here) |
| `fabric reconcile` | ran unguarded (no-op here) |

Resolving both sides and running `lyx fabric merge --continue` concluded correctly:
two `merge_committed` entries, a merge commit on each side, record deleted.
Weft merge-commit subject is `Merge commit '<sha>' into main-weft` — SHA-labelled, no
branch name leaked, matching `doc.go`'s claim.

### Scenario L3 — crash between the warp and weft `MergeStart` (hub `s_crash`) — CONFIRMED DEFECT

State reconstructed exactly as a kill in that window leaves it: `git merge --no-commit
<task1-sha>` run in the warp checkout, weft untouched, and `fabric-merge.json` written
with `warp_outcome:"staged"`, `weft_outcome:""` (this is byte-for-byte what
`merge.go:141-148` persists before it calls the weft `MergeStart`).

From a fresh process:

```
$ lyx fabric merge --continue
{"error":"fabricengine: merge conclude did not finish; run MergeContinue again",
 "mutations":[{"kind":"merge_committed","target":"warp-bare","detail":"21835d5…"}],
 "ok":false,"partial":true}
```

- The warp merge commit **landed**.
- The weft conclude then failed (`git commit --no-edit` on a clean tree), so the pair is
  now out of correspondence and no correspondence entry was recorded.
- Every subsequent `--continue` fails identically with zero mutations — the instruction
  in the error message ("run MergeContinue again") can never succeed.
- Only `--abort` recovers; it correctly reset both sides to `warp_start`/`weft_start`
  and deleted the record.

See finding **F1**.
