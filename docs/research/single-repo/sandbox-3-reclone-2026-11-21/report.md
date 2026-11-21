# One-repository sandbox, trial 3: a second clone, and the namespace hand check

Third of three trials of the one-repository hub layout (57015a52), in the sandbox trials 1 and 2 built.
It clones the sandbox a second time with a plain one-URL clone, lands one task from that clone,
pulls the first hub and checks it sees the landing.
It then checks fabric and `gitrepo` by hand for an origin ref built from a bare `refs/heads/` or `refs/tags/` on a records-store path.
The last section sums up all three trials.

```yaml
date: 2026-11-21
lym_build: 3d3b48127d0b at the clone, e6bc01f58044 from 19:38 (/home/knatte/go/bin/lym, re-executed by the dobbies)
repository: Knatte18/lym-sandbox-onerepo (private, free plan)
first_hub: /home/knatte/Code/lym-sandbox-onerepo-LYMHUB
second_hub: /home/knatte/Code/sbone-clone2/lym-sandbox-onerepo-LYMHUB
shortname: sbone (both hubs)
runs: 1 of a cap of 1, landed
hand_check_commit: 57015a52
result: every check passed; no lym bug found, so no note was filed
```

## Result

- The plain clone `lym fabric clone <url>` detected the one-repository layout and joined it,
  and its board, prime records branch and the five archive tags trial 2 left all read back in the new hub.
- `greet-reclone` ran from the second hub to landing with no halt, landed as `1c74466`, and tore down as trial 2's landings did.
- `lym fabric pull` in the first hub brought its code store and records store level with the landing.
  The first hub's board caught up only at `lym board sync`, because the pull moves the pair and not the board
  (see [The first hub after the pull](#the-first-hub-after-the-pull)).
- The hand check found no origin ref built from a bare `refs/heads/` or `refs/tags/` on a records-store path.

As in trials 1 and 2, the orch ran the steps the task session is refused: the clone, filing and releasing the task, and the dobby start.

## 1. The second clone

```
lym fabric clone --into /home/knatte/Code/sbone-clone2 https://github.com/Knatte18/lym-sandbox-onerepo
```

The orch ran it: exit 0, `ok: true`, `partial: false`, no warning.
It reported `shortname_recorded: false` and `warp_binding_recorded: false`, since origin already carried both,
and no refusal or warning for a second hub of the shortname `sbone` on this machine.
The `--into` directory differs from the first hub's, because a hub's directory is always `<warp-name>-LYMHUB`.

The new hub holds the code store, the records store, `_bolt` and the workspace file, as the first does.
Its records store carries the layout's configuration with no flag given:

```
remote.origin.fetch +refs/lym/heads/*:refs/remotes/origin/*
remote.origin.fetch +refs/lym/tags/*:refs/tags/*
remote.origin.push refs/heads/*:refs/lym/heads/*
remote.origin.push refs/tags/*:refs/lym/tags/*
remote.origin.tagopt --no-tags
lym.primerecordsbranch main-weft
```

Its code store holds `main`, `origin/main` and `origin/HEAD` and nothing from the records namespace.
The records store holds trial 2's five archive tags, mapped to `refs/tags/archive/<slug>/<sha12>`.
Each reads back with the landed run's records; `archive/greet-shout/1fcd86f75eff`, for one,
holds that run's decision record, change description, review rounds, shed status and usage records.

| Command, in the second hub | Outcome |
|---|---|
| `lym board list` | `ok: true`; the board origin holds, with `greet-reclone` once filed |
| `lym fabric status` | `ok: true`, no changes, no failed pushes |
| `lym fabric pairs` | `main` ↔ `main-weft`, `in_sync: true`, `junction_healthy: true` |
| `lym fabric list` | `main` at `1aff8e5`, trial 2's last landing |
| `lym fabric shortname` | `sbone` |

## 2. A task landed from the second clone

The orch filed `greet-reclone` in the second hub (kind `task`, label `enhancement`, profile light),
asking for `func Greeting() string` returning `"Hello"` in package `greet` with a test,
and started that hub's dobby with `lym beam start`.
`pull_request` was off, from the sandbox's default layer, as trial 2 left it.

| Time (UTC+1) | Event |
|---|---|
| 19:33:51 | released |
| 19:33:58 | Worktree-Create; the pair was made in the second hub only |
| 19:38:59 | Finalize, after Seed-IDE, Record-Write, the Drift rows, Darn-Write, Code-Bouncer, one burler round and Describe |
| 19:39:19 | done, with no halt and no pull request; landed as `1c74466` |
| 19:40:44 | teardown finished |

`1c74466 Add Greeting, the bare greeting word` sits on `1aff8e5` and changes `greet.go` and `greet_test.go`.
Teardown deleted `refs/heads/greet-reclone` and `refs/lym/heads/greet-reclone-weft` from origin
and pushed `refs/lym/tags/archive/greet-reclone/e421ed44a33f`.

Origin afterwards:

```
1c74466a4a3e1e2c567db27c12b642c0f8a5b796	refs/heads/main
81bf47453492d99d3144cec1be9a0052c79160bf	refs/lym/heads/main
2aa71aef3bbb2dba74d973da70c26e4984ada744	refs/lym/heads/main-weft
...	refs/lym/tags/archive/<slug>/<sha12>, one per trial 2 task and greet-reclone
5b9175e7cb4164c50eedda919e8f9b80806927d2	refs/pull/1/head
```

GitHub's branch list shows `main` alone and its tag list is empty.

## 3. The first hub after the pull

The orch ran `lym fabric pull` from the first hub's prime:
exit 0, `ok: true`, `warp_fetched`, `warp_advanced` and `weft_pulled` true, `reconciled` and `rewrite_detected` false.
It advanced the records store to `26be249` and the code store and prime worktree to `1c74466`.

| What | First hub after the pull | Origin |
|---|---|---|
| code `main` | `1c74466` | `1c74466` |
| records `main-weft` | `2aa71ae` | `2aa71ae` |
| archive tags | six, `greet-reclone` among them | six |
| board (`_bolt`) | `57fa4e1`, ahead 1 and behind 5 | `81bf474` |

The code and the records saw the landing.
The board did not: `lym board list` in the first hub still showed `greet-reclone` as `running · Describe`.
`lym fabric pull` pulls the pair and leaves the board to the board's own sync,
and the first hub had written no board entry since the landing.

The one commit ahead was the first hub's own.
At 19:38:33 both hubs' dobbies re-executed onto the new lym binary, and each seeded the same stencil change into its `_bolt`:
the second hub as `e732462`, pushed at 19:38:30, and the first as `57fa4e1`, kept local.
`lym board sync` in the first hub answered `ok: true`, dropped `57fa4e1` as already upstream,
and left `_bolt` at origin's `81bf474` with an empty board.

The first hub's records store, which `_bolt` shares, also keeps `origin/greet-reclone-weft` at `1bc06ae`,
a tracking ref fetched while the run was in flight;
origin has since deleted the branch.
A dry-run `lym fabric cleanup` lists no candidate,
and `lym fabric add` drops such a ref when a later pair reuses the name.

## 4. Hand check: bare origin ref names on records-store paths

Method: `git grep -E 'refs/(heads|tags)/' 57015a52 -- 'internal/fabric*' internal/gitrepo ':!*_test.go'`.
The search returned 82 lines in `internal/fabricengine` and `internal/gitrepo`; `internal/fabriccli` and `internal/fabricvocab` have none.
From the landing commit to `3d3b4812` the only change at these sites removes two local-ref lines from `anchormarker.go`.

Every site that sends a ref name to origin, or reads one back from it:

| Site | What reaches origin | Store | Wrong? |
|---|---|---|---|
| `fabricengine/add.go:498` | fetch source `refs/heads/<code branch>` | code | no: code branches live under `refs/heads/` |
| `fabricengine/add.go:576` | fetch `ns.Branch(<records branch>):refs/heads/<records branch>` | records | no: the origin side is namespaced, the bare side is local |
| `fabricengine/archive.go:225` | push `refs/tags/<tag>:ns.Tag(tag)` | both | no: the destination goes through the namespace |
| `fabricengine/clone.go:842`–`844` | the records store's tag fetch and both push refspecs | records | no: the bare sides are the local sides |
| `fabricengine/cleanupremotewarp.go:246`, `293`, `318` | branch and tag names listed on origin | records | no: compared after `ns.LocalRef` maps them |
| `fabricengine/remoteleftover.go:281`–`301` | tag names listed on origin | records | no: compared after `ns.LocalRef` |
| `fabricengine/originreconcile.go:543` | branch names listed on origin | records | no: compared after `ns.LocalRef` |
| `fabricengine/recordslayout.go:117` | `ls-remote` of `refs/heads/lym-bolt` | origin, before a clone | no: it probes the retired layout's board branch, which sat there |
| `fabricengine/recordslayout.go:126`, `139` | origin's `HEAD` symref; namespaced branch names | origin | no: `HEAD` names the code default; the names go through `ns.LocalRef` |
| `fabricengine/cleanupremotewarp.go:332` | origin's `HEAD` symref | code | no: `HEAD` names the code default branch |
| `fabricengine/remoteteardown.go:187` | way-forward text: push `refs/tags/<archive tag>` with no destination | code | no: the code store's namespace is the identity |
| `fabricengine/remoteteardown.go:203` | way-forward text: push `refs/tags/<tag>:ns.Tag(tag)` | records | no: the destination is spelled out |
| `fabricengine/remoteteardown.go:484`, `remove.go:676` | fetch `refs/heads/<code branch>` | code | no |
| `gitrepo/push.go:154` | push source `refs/heads/<branch>`, destination `ns.Branch(branch)` | both | no: the bare side is local |
| `gitrepo/pushretry.go:45`–`99` | matches a `! [remote rejected] <src> -> <dst>` line | both | no: a destination is also matched through `ns.LocalRef` |

The remaining lines are local ref names passed to `for-each-ref`, `rev-parse`, `rev-list`, `update-ref` or `RefSHA`,
the worktree list's branch prefix, the namespace's own definition in `gitrepo/refnamespace.go`, comments and message text.

**Result: no site is wrong.**

One observation outside the check's scope.
A discarded task branch's archive tag is pushed from the code store,
whose namespace is the identity, so it lands at origin's plain `refs/tags/archive/<slug>/<sha12>` and shows in GitHub's tag list.
The two-repository layout behaves the same.
Trial 2's abandon pushed no such tag, because the task branch sat on `main`'s tip.
No trial exercised a discard with unlanded code, and none is filed as a bug.

## The three trials

| Trial | What it did | Outcome |
|---|---|---|
| [1](../sandbox-1-setup-2026-11-21/report.md) | created `Knatte18/lym-sandbox-onerepo`, seeded a Go module, cloned it with `--single-repo` | origin showed `main` alone, the board and prime records under `refs/lym/heads/`, every lym status verb `ok` |
| [2](../sandbox-2-tasks-2026-11-21/report.md) | ran five darn tasks: two in turn, two at once, one abandoned | four landings and the abandon left origin in the layout at every checkpoint; one bug, note B2 |
| 3 | cloned a second hub with a plain one-URL clone, landed a task from it, pulled the first hub, checked fabric and `gitrepo` by hand | the clone joined the layout, the landing tore down cleanly, the first hub caught up; no wrong site |

Across the three trials, across six landings, one abandon and two hubs:

- **Origin held the layout throughout.**
  GitHub's branch list showed `main` plus each in-flight code branch, and its tag list stayed empty.
  Every records branch, the board and every archive tag stayed under `refs/lym/`.
  No code store ever held a ref from the records namespace.
- **The clone works both ways.**
  `--single-repo` seeds the namespace, and a plain clone of a seeded origin joins it, with the same store configuration.
- **Teardown is uniform.**
  A landing and an abandon both delete the pair's two branches from origin and push the records head as `refs/lym/tags/archive/<slug>/<sha12>`,
  within about a minute and a half of the landing.
- **The code namespaces the origin side at every records-store site** the hand check reached.

One lym bug was found, in trial 2, and it does not belong to the layout:
**B2, `rules-read-plan-gated-403`.**
A private repository on GitHub's free plan answers 403 to the branch-rules read,
and a run holding a pull request record halts at Finalize blaming the token.

Friction the trials share:

- **A task session cannot run the trials' setup.**
  `lym fabric clone`, `lym board upsert` of a task, the run-settings fields and the dobby start are each the orch's,
  so every trial needed the orch for those steps.
- **A pull leaves the board behind.**
  After `lym fabric pull`, a hub that wrote no board entry since the landing reads a stale board until `lym board sync` runs.
- **Two hubs of one board both seed stencils on a binary change.**
  The duplicate commit was dropped at the next board sync with no conflict.
- **The concurrent-landing gap trial 2 left stays open.**
  No trial landed a code change onto a parent another landing had just moved.
