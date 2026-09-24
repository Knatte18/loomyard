# batten — review round opus-medium-r4

Round 4 of the batten follow-up crucible campaign.
Worktree `crucible-batten-followup`, seeded at `b1e5ab9af`.
Clean-room: no `_mill/**/batten-review-*` file other than the prompt was opened before this report's findings were complete.

## Executive summary

(pending)

## Scope assessment

(pending)

## Code findings

(provisional, severity ordering pending)

### F1 (provisional) — Worktree-Create reports done over a pair whose provenance record was never written; the child's bootstrap then refuses forever

- Where: `internal/fabricengine/fabric.go` `PairComplete` (checks sibling + junction health only); consumed by `internal/battencli/wire.go` `taskWorktreeComplete` and `childSeedParams`.
- Scenario (CONFIRMED live, windows A6/A7/A8): SIGKILL of `lyx batten step <slug>` anywhere after `seedLyxJunction` and before `WriteOrigin` in `Topology.Add`.
  Resume: create row reports done (PairComplete passes), Seed-Child writes `{recipe: loom, driver: go}` with NO `params.parent` (no origin record), commits and pushes it.
  Run-Shed's spawn then fails on every resume: `loom: no recorded parent branch for this worktree pair; pass --parent once to record it`.
  Following that text from inside the child (`lyx loom start --parent main --no-attach`) is refused: `shedrun: disagreeing seed ... {Recipe:loom Driver:go Params:map[]}; refusing to overwrite with {... Params:map[parent:main]}`.
  The run cannot reach done without hand-editing the child's committed seed.

### F2 (provisional) — the incomplete-pair remedy does not work when followed verbatim from prime

- Where: `internal/battencli/wire.go` `incompletePairRemedy` and the `CreateWorktree` closure's `present && !complete` arm.
- Scenario (CONFIRMED live):
  (a) window A3+ (sibling created): the remedy's `git worktree remove --force <hub>/<slug>-weft` run from prime fails `fatal: ... is not a working tree` — the sibling is a worktree of the other repository; the next resume then refuses with fabric's `weft worktree directory already exists`.
  (b) windows A4/A5: the remedy never names the portal or the launchers; the next resume refuses `link already exists — remove it first: <hub>/_portals/<slug>`, and that refused create's rollback strands the warp branch (known fabric item), costing a second remedy loop.
  (c) hub with `branch_prefix: r4/` (fixture B): the remedy names `git branch -D pa1`; the branch is `r4/pa1`, so `error: branch 'pa1' not found`, and the next resume refuses with createRefusal's leftover-branch text.
- Verified replacement: `lyx fabric remove --force <slug>` from prime removes every Add leftover in one command (portal, launchers, junctions, task worktree, sibling, sibling branch) at A1/A2 (sibling absent, fixture A `ka2`) and A5 (fixture B `pa3`), then `git branch -D <branch_prefix><slug>`; resume reaches Seed-Child.

### F3 (provisional) — Worktree-Teardown reports done while the pair's other-side branch survives, silently

- Where: `internal/battencli/wire.go` `Teardown.Remove` closure — the `!present` "pair already gone" arm, and the success arm, which discards `RemoveResult.RemoteBranchError`/`RemoteSkippedReason` (only `res.Mutated()` is logged).
- Scenario (CONFIRMED live):
  R5 (killed after the sibling worktree's removal, before its branch deletion): resume reports done; `ka7-weft` survives locally AND on the remote.
  R6 (killed after the local branch deletion, before the remote one): resume reports done; `ka12-weft` survives on the remote only — invisible to `lyx fabric cleanup` (local-only enumeration).
  R4 remedy path (`lyx fabric prune --apply`, as the teardown refusal says): prune removes the sibling worktree only; resume reports done; `ka6-weft` survives locally and on the remote.
  Failing remote deletion (fixture B, weft `origin` pointed at a missing path during `pa3`'s teardown): done, no mention anywhere; `r4/pa3-weft` stranded on the remote, deleted locally.
- Consequence (CONFIRMED on `ka12`): following doneSlugRefusal's abandon path verbatim (run directory, `git branch -D`, `git push origin --delete`, `lyx fabric cleanup --apply --remote`), the re-run's create fails `push weft branch "ka12-weft" failed ... [rejected] (non-fast-forward)` with git's own "use git pull" hint, and the refused create's rollback strands the warp branch again.

### F4 (provisional, NIT) — createRefusal and doneSlugRefusal describe a torn-down pair as keeping "both remote copies"

- Where: `internal/battencli/wire.go` `createRefusal` text and doc comment; `internal/battencli/arm.go` `doneSlugRefusal` ("both sides of the pair").
- Batten's own teardown deletes the other-side branch locally and on the remote (`top.Remove(..., remote: true)`), so a batten-torn-down pair leaves only its task branch, local and remote.

### F5 (provisional, NIT) — stale field comment in shedcli's recipe table

- Where: `internal/shedcli/table.go` `entry.BootstrapVerb` comment: "Nothing reads this field yet in this batch -- that is deliberate, not dead code."
  `internal/shedcli/seed.go:88` reads it (the `--driver llm` validator).

## Focus-1 enumeration (SIGKILL windows in Topology.Add / Topology.Remove)

Enumerated from source by reading every mutating call in `Topology.Add` (`internal/fabricengine/add.go`), `WireJunctionsWith` (`junction.go`), `Topology.Remove` (`remove.go`) and `removeWeftWorktree` (`weftwiring.go`), each boundary instrumented as a throwaway `r4kp("<id>")` self-SIGKILL (reverted before any other drive; see What was tested):

```
grep -n 'rec\.\(Append\|AppendRef\)\|createGitWorktree\|createWeftWorktree\|containedWorktreeAdd\|createPortal\|writeLaunchers\|WireJunctionsWith\|WriteOrigin\|CommitWeftPaths\|"push"\|pushWeftBranch' internal/fabricengine/add.go
grep -n 'seedLyxJunction\|seedWeftArtifactExcludes\|seedGitExclude' internal/fabricengine/junction.go
grep -n 'removePortal\|removeLaunchers\|removeWarpJunction\|removeWarpWorktreeDir\|removeWeftWorktree\|removeGitWorktree\|deleteBranch\|deleteRemoteBranch\|worktree", "prune' internal/fabricengine/remove.go internal/fabricengine/weftwiring.go
```

| Site | State left by the kill | Re-entered row | Reaches done with no hand edit? |
|---|---|---|---|
| A1 after warp `worktree add -b` | task worktree + branch only | Worktree-Create refuses, incomplete-pair remedy | Yes, remedy works verbatim (default prefix); prefixed hub: no, F2c |
| A2 after post-checkout hook | same as A1 | same | same as A1 |
| A3 after sibling worktree | + sibling worktree + sibling branch | refuses, remedy names sibling | No verbatim: sibling command fails from prime (F2a) |
| A4 after portal | + portal | refuses | No verbatim: portal unnamed, then rollback strands branch (F2b + known fabric item) |
| A5 after launchers | + launchers | refuses | same as A4 |
| A6 after junction links, before sibling artifact excludes | junctions wired, no origin | create **done** | No: F1 (child seed without parent, bootstrap refuses forever) |
| A7 before `.git/info/exclude` seeding | same | create done | No: F1 (exclude itself harmless: info/exclude is shared per repository) |
| A8 after wiring, before `WriteOrigin` | same | create done | No: F1 CONFIRMED end to end |
| A9 origin written, uncommitted | sibling `?? _lyx/fabric/` | create done; seed has parent | Yes: loom bootstrap commits the record (traced) |
| A10 committed, nothing pushed | branches local only | create done | Yes: Seed-Child's push publishes the sibling branch; task branch pushed by loom's own landing (traced, PLAUSIBLE) |
| A11 task branch pushed only | sibling branch local only | create done | Yes, as A10 |
| A12 both pushed, before return | complete | create done | Yes (healthy re-entered create reports done) |
| R1 after portal removal | launchers + both worktrees | teardown | Yes, clean |
| R2 after launchers, before junction sweep | both worktrees, junctions | teardown (Shutdown re-runs) | Yes, clean |
| R3 junctions swept, task worktree present | task worktree without `_lyx` | Shutdown loads reed config through the task worktree with `_lyx` gone: degrades to template, `Down` ok | Yes, clean |
| R4 task worktree removed, sibling present | sibling + its branch | refuses, names `lyx fabric prune --apply` | Done after prune, but sibling branch stranded local + remote (F3) |
| R5 sibling removed, branch not deleted | sibling branch local + remote | "pair already gone" → done | Done, branch stranded (F3) |
| R6 local branch deleted, remote not | sibling branch on remote only | done | Done, remote branch stranded, re-run breaks (F3) |
| R7 remote deleted, before `worktree prune` | stale registration at most | done | Yes, clean |
| R8 Remove about to return | nothing | done | Yes, clean (predecessor's F5 re-confirmed) |

What this enumeration cannot see: a kill *inside* one primitive (mid `git worktree add`, between two junctions of the wired set, between two launcher files, inside `state.WriteJSON`'s temp-then-rename, mid `git push`); kills of the Shutdown half (reed `Down`) itself; the Windows junction path (not touched, unreachable from this host).
The site list is derived by reading; a mutation hidden behind a helper not named in the grep patterns would be missed, which is why each helper's body was also read (`createPortal`, `writeLaunchers`, `seedLyxJunction`, `removeWeftWorktree`).

## Focus-2 enumeration (youngest fixes)

Youngest fixes from `git log --format='%h %s' d7ab9eca0..HEAD`; operator-facing texts from `git diff d7ab9eca0..HEAD -- internal/ | grep '^+.*fmt.Errorf\|^+.*errors.New'`.

| Text / fix | Followed verbatim from prime? | Result |
|---|---|---|
| incomplete-pair refusal (`afb5976eb`, `ac3ba6e3e`) | Yes, A1/A3/A4/A5 and on a `branch_prefix` hub | Fails at A3+ and on prefixed hubs (F2); never fires at A6–A8 (F1) |
| completeness check vs a COMPLETE pair | A9–A12 re-entries | All report done; no false "incomplete" found. PLAUSIBLE false-incomplete: a pathspec widened between the kill and the resume adds a wired name the old pair lacks (not driven) |
| teardown deletes the other-side branch on the remote (`a021b016e`) | normal teardown, prefixed hub, broken origin, R4/R5/R6 | Deletes exactly `<prefix><slug>-weft` local + remote; task branch kept local + remote. No origin: skipped. Failing delete: silently done (F3). Landing/PR need only the task branch, which is untouched; a slug re-run needs nothing it deleted, but is broken by what it failed to delete (F3) |
| createRefusal leftover-branch text (`85bf53198`) | A4 path (`ka4`) | Works; `git push origin --delete` errors harmlessly when the branch was never pushed; wording "both remote copies" stale (F4) |
| sibling-remnant teardown refusal (`37e4b4788`) | R4 (`ka6`) | Works to done but strands the sibling branch (F3) |
| doneSlugRefusal abandon path (`d79829eb6`) | `ka12` after R6 | Fails: stranded remote-only sibling branch rejects the new create's push (F3); "both sides" wording stale (F4) |
| absent-worktree refusal (`894f5df7c`) | `pfull` removed by `lyx fabric remove` at Seed-Child | Abandon path followed verbatim: create done again. Deferred gap re-confirmed |
| `lyx shed seed` prime-only / drivable-worktree refusal (`af081a157`, `eefd0dc1f`) | read + batten verbs from task worktree, weft prime, `_board`, sibling | Refused, nothing written |
| Run-Shed spawn retry text (`ef703936c`) | `ka8` | Text accurate (retry does retry), but for F1's state retrying can never succeed |
| Seed-Child refusals (`0be1f57b4`) | Board types `batten`, `bogus` (fixture B) | Blocked at Seed-Child with the recipe named, before any write |

## Docs & operability findings

(pending)

## What was tested

### Fixture hub A (`$S/r4fx-a7q`, `$S` = this session's scratchpad under /tmp)

- `mkfx.sh`: bare warp (go.mod + main.go + .gitignore, `-b main`), empty bare weft (`-b main`), `lyx fabric clone --into <hubs> weft.git warp.git`.
  First attempt without `-b main` on the bare warp: clone refused with its own "remote HEAD names a ref that does not exist" remedy — environment setup, not a defect.
- `fxover.sh`: committed `selfreport: false`, `friction: ""` (loom.yaml) and `require_pr_to_base: []` (landing.yaml) on prime's weft BEFORE any Board task or create; verified all three with grep against `git show HEAD:`.
- Board tasks `ka1`..`ka12` (`type: loom`), `floor` (type empty) via `lyx board upsert`.

### Instrumented kill build (focus 1)

- Throwaway kill points added to `add.go` (A1–A5, A8–A12), `junction.go` (A6, A7), `remove.go` (R1–R4, R8), `weftwiring.go` (R5–R7): each `r4kp("Xn")` self-SIGKILLs when `LYX_R4_KP=Xn`.
  Built to `$S/bin/lyx-kp`, then `git checkout -- internal/fabricengine/ && rm r4kp_tmp.go`; `git diff --quiet` clean.
  `.dev-bin/lyx` (deployed from `b1e5ab9af` before instrumentation) contains 0 occurrences of `LYX_R4_KP`; the kill binary 1.
  Only the killed step runs the kill binary; every resume runs `.dev-bin/lyx`.

### Add windows (driven from prime, `LYX_R4_KP=An lyx-kp batten step ka<n>` then `lyx batten step ka<n>`)

- A1 (after warp `worktree add -b`): exit 137; resume blocked at Worktree-Create with the incomplete-pair remedy naming the task worktree and `git branch -D ka1`.
  Remedy followed verbatim from prime: both commands succeed; resume creates the pair (done), Seed-Child done.
- A2 (after hook install): identical to A1.
- A3 (after sibling created): remedy names `git worktree remove --force <hub>/ka3` AND `git worktree remove --force <hub>/ka3-weft`.
  From prime the second command fails: `fatal: '<hub>/ka3-weft' is not a working tree` (rc 128) — it is a worktree of the sibling repository, not prime's.
  Resume after the rest of the remedy: `weft worktree directory already exists: <hub>/ka3-weft` (fabric's own vocabulary, no remedy).
  `git -C <hub>/ka3-weft worktree remove --force <hub>/ka3-weft` works; resume then creates the pair (adopting the leftover `ka3-weft` branch) and Seed-Child is done.
- A4 (after portal) / A5 (after launchers): with the corrected sibling command, resume is refused `link already exists — remove it first: <hub>/_portals/ka4` — the remedy never names the portal or launchers.
  That refused Add rolls back (removing the stale portal) but keeps the warp branch (known `fabric-rollback-keeps-warp-branch` item); the next resume gets createRefusal's leftover-branch text.
  Following that verbatim (`git branch -D ka4`; `git push origin --delete ka4` fails harmlessly: remote ref does not exist; `lyx fabric cleanup --apply --remote` deleted `ka4-weft` and `ka5-weft`) then resume: create done, Seed-Child done, clean both halves.
- A6 (after junction links, before sibling artifact excludes) / A7 (before `.git/info/exclude` seeding): create reports done; both halves `git status --porcelain` clean (info/exclude is shared by every worktree of a repository, so prime's clone-time entries already cover the junctions).
  Seed-Child writes a seed with no `params.parent` — see F1.
- A8 (after wiring, before `WriteOrigin`): same as A6/A7. Run-Shed: `loom: no recorded parent branch ... pass --parent once`; `lyx loom start --parent main --no-attach` from the child refused as a disagreeing seed — F1 CONFIRMED.
- A9 (origin written, not committed): create done; sibling shows `?? _lyx/fabric/`; Seed-Child writes `params.parent: main` (ReadOrigin reads the uncommitted record).
  loom's bootstrap commits the origin record unconditionally (`bootstrapCommitPaths`), so this state self-heals at Run-Shed (traced, `sharedbootstrap.go` step 3).
- A10 (committed, neither branch pushed) / A11 (warp pushed, sibling not): create done; Seed-Child's `PushSeed` pushed the sibling branch (weft remote shows it after Seed-Child).
- A12 (both pushed, before return): create done — a healthy re-entered create reports done.

### Remove windows (child status hand-set to `done` and committed in the child's sibling as a harness, then `lyx batten step` to reach Worktree-Teardown; kill with `LYX_R4_KP=Rn`)

- Harness note: the child's own `status.json` was written by hand (`state: done`) and committed on the child's sibling so Run-Shed returns Done without spawning an LLM; this exercises the teardown row only, and the floor drive below covers the real Run-Shed path.
  `ka3`/`ka4` additionally carried a live reed session (`lyx reed up` + `lyx reed add --name holder --cmd "sleep 86400"` from the child) before their teardown.
- R1 (`ka1`, after portal removal): resume done; sibling branch deleted local + remote; no leftovers.
- R2 (`ka3`, after launchers, before the junction sweep; live session): the killed step's Shutdown had already ended the session; resume done, clean.
- R3 (`ka4`, junctions swept, task worktree still on disk; live session): resume's Shutdown resolves reed's config through the task worktree with `_lyx` gone — reed's config load degrades to its template (Config Strictness: reedengine is degrading) and `Down` succeeds; Remove completes; done, clean.
- R4 (`ka6`, task worktree removed, sibling on disk): resume blocked with the sibling-remnant refusal naming `lyx fabric prune --apply`; followed verbatim: prune removed the sibling worktree only; resume done — `ka6-weft` left locally and on the remote (F3).
- R5 (`ka7`, sibling worktree removed, branch not deleted): resume done — `ka7-weft` left locally and on the remote (F3).
- R6 (`ka12`, local branch deleted, remote not): resume done — `ka12-weft` left on the remote only (F3); the abandon-path re-run then failed on the push (F3 consequence).
- R7 (`ka9`, remote deleted, before `worktree prune`): resume done, clean.
- R8 (`ka10`, Remove about to return): resume done, clean.
- Branch-prefix teardown (fixture B, `pa1`): deletes exactly `r4/pa1-weft` locally and on the remote; `r4/pa1` kept locally and on the remote (Remove never deletes the task branch).
- Failing remote deletion (fixture B, `pa3`, weft `origin` set to a missing path for the step, restored after): teardown done; only log line is the batten status push warning; `r4/pa3-weft` stranded on the remote (F3).

### Focus-2 remedies on a prefixed hub (fixture B `$S/r4fx-b3p`, `branch_prefix: r4/` committed to `_board/_lyx/config/fabric.yaml`)

- `pa1` killed at A1: remedy names `git branch -D pa1` → `error: branch 'pa1' not found`; resume: createRefusal text naming `r4/pa1` correctly (F2c).

### Floor checks

- PrimeRunLock across two slugs (fixture B): `flock <prime>/.lyx/shed/run.lock sleep 15` held from a shell; `lyx batten step pfull` at Worktree-Create → blocked, stuck reason `prime lock "<hub>/warp/.lyx/shed/run.lock" is already held; another batten producer is creating or tearing down a task worktree`; after release, resume → create done.
- Batten Bookend refusals (fixture B), `lyx batten status pfull` and `lyx batten run pfull2`:
  from the task worktree `pfull` → "runs from the hub's prime worktree only; \"pfull\" is not the prime worktree (\"warp\" is)";
  from the weft prime `warp-weft`, from `pfull-weft`, and from `_board` → refused through `RequireDrivableWorktree` naming the checkout. No seed written anywhere.

### Hermetic baseline (start of Job 1)

- `go build ./... && go vet ./... && go test -count=1 ./...` at `b1e5ab9af`: exit 0, every package `ok`.
- Third self-report path sweep: `ls -d internal/*selfreport* internal/*friction*` plus a repo-wide grep for `CreateIssue`/issue-filing call sites.
  Only Tier 1 (`internal/loomcli/selfreport.go`, wired at `internal/loomcli/arm.go:326`) and Tier 2 (`internal/frictionengine` via `lyx selfreport create` in `internal/selfreportcli`).
  No third path.
- Environment: `which claude tmux gh` all resolve.
  `--child-driver llm`: the `ly@loomyard` plugin is absent on this host, an operator-accepted environment gap (focus 4); not driven this round.
- Windows path behaviour: not touched, unreachable from this Linux host.

## Teardown

(pending)
