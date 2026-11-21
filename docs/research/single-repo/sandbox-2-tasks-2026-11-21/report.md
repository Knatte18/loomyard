# One-repository sandbox, trial 2: run tasks to landing

Second of three trials of the one-repository hub layout (57015a52), in the sandbox hub trial 1 created.
It runs five darn tasks in the sandbox: two one after the other, two in flight at once, and one started and abandoned.
After each landing and the abandon it records origin: the records branches and archive tags under `refs/lym/`, the board, and the branch list GitHub shows.

```yaml
date: 2026-11-21
lym_build: 57015a52cd1f (/home/knatte/go/bin/lym)
repository: Knatte18/lym-sandbox-onerepo (private, free plan)
hub: /home/knatte/Code/lym-sandbox-onerepo-LYMHUB
shortname: sbone
runs: 5 of a cap of 5 (4 landed, 1 abandoned)
result: the layout held at every checkpoint; one lym bug, filed as note B2
```

## Result

Every task reached the end the trial planned for it, and origin matched the layout at every checkpoint.

- While a run is in flight, its code branch sits under `refs/heads/` and shows in GitHub's branch list;
  its records branch sits under `refs/lym/heads/<slug>-weft` and does not.
- Within about a minute of each landing, teardown deletes both branches from origin and pushes the records head as `refs/lym/tags/archive/<slug>/<sha12>`.
  GitHub's tag list stayed empty throughout.
- The abandon (`lym batten cancel --remove`, then `lym board remove`) left the same end state as a landing: neither branch on origin and an archive tag under `refs/lym/`.
- After the last step, origin's branch list shows `main` alone, and the board, `main-weft` and `main` on origin equal the local tips.
  The code store holds no ref from the records namespace and no task branch.

One lym bug: the first landing halted at Finalize on a 403 from GitHub's branch-rules API, which lym reports as a bad token (see [The Finalize halt](#the-finalize-halt)).
It is filed on this hub's board as note B2, `rules-read-plan-gated-403`.

One gap in the trial itself: the concurrent pair landed only one code change, because the trivial task had nothing to change (see [Gaps](#gaps)).

## The tasks

The orch filed and drove the tasks, because the sandbox board refuses a task session two steps the trial needs (see [Friction](#friction)).
Each is kind `task`, label `enhancement`, recipe darn, profile light, run_settings `{"darn":{"writer":"opus[medium]"}}`, in the seed module's package `greet`.

| Task | Asks for | Plan | Outcome |
|---|---|---|---|
| `greet-goodbye` | `Goodbye(name string) string` with a test | first, alone | landed by pull request #1 as `aeabf1e` |
| `greet-shout` | `Shout(name string) string`, the upper-cased `Hello`, with a test | second, alone | landed by direct push as `179859c` |
| `greet-many` | `HelloAll(names []string) []string`, with a test | third, alongside `greet-doc` | landed by direct push as `1aff8e5` |
| `greet-doc` | a package doc comment on `greet.go` | fourth, alongside `greet-many` | landed with no change: the seed already had one |
| `greet-abandon` | a const `Greeting = "Hello"` | fifth, abandoned | cancelled at Seed-IDE, entry removed |

The orch chained the dependencies `greet-goodbye` → `greet-shout` → (`greet-many`, `greet-doc`) with `lym board set-deps`,
released the first four with `lym board set-status`,
and brought up the sandbox's dobby and orch with `lym beam start` from the sandbox prime.
After the first landing it set `pull_request` off at the sandbox's default layer, so the later tasks opened no pull request.

`main` after the trial:

```
1aff8e5 Add HelloAll, mapping Hello over names
179859c Add Shout, an upper-case Hello
aeabf1e Add Goodbye farewell beside Hello (#1)
f95da8c Seed greet_test.go
f03faed Seed greet.go
247087f Seed go.mod
```

## Origin at each checkpoint

Times are local (UTC+1).
Each row comes from `git ls-remote https://github.com/Knatte18/lym-sandbox-onerepo`
and `gh api repos/Knatte18/lym-sandbox-onerepo/branches` and `/tags`.

| Checkpoint | `refs/heads/` (GitHub's branch list) | `refs/lym/heads/` | `refs/lym/tags/archive/` | GitHub tags |
|---|---|---|---|---|
| Baseline, 18:42 | `main` `f95da8c` | `main` `db82e1d`, `main-weft` `9f8c6aa` | none | none |
| `greet-goodbye` in flight, 18:44 | `main`, `greet-goodbye` | `main`, `main-weft`, `greet-goodbye-weft` | none | none |
| `greet-goodbye` done, 19:12 | `main` `aeabf1e`, `greet-goodbye`, `greet-shout` | `main` `11b42c0`, `main-weft` `788ae88`, `greet-goodbye-weft`, `greet-shout-weft` | none | none |
| its teardown, 19:13 | `main`, `greet-shout` | `main` `863fe54`, `main-weft` `5fd7923`, `greet-shout-weft` | `greet-goodbye/19f51b469999` | none |
| `greet-shout` done, 19:17 | `main` `179859c`, `greet-shout` | `main` `32d70fb`, `main-weft` `59e6188`, `greet-shout-weft` | `greet-goodbye` | none |
| its teardown, 19:19 | `main`, `greet-many`, `greet-doc` | `main` `e1b1a7f`, `main-weft` `c96ee0d`, `greet-many-weft`, `greet-doc-weft` | + `greet-shout/1fcd86f75eff` | none |
| `greet-many` and `greet-doc` done, 19:24 | `main` `1aff8e5`, `greet-many`, `greet-doc` | `main` `c10797b`, `main-weft`, `greet-many-weft`, `greet-doc-weft` | `greet-goodbye`, `greet-shout` | none |
| their teardown, 19:25 | `main` `1aff8e5` | `main` `a07298e`, `main-weft` `0e805b4` | + `greet-many/fa43bc97ef67`, `greet-doc/7719b1787795` | none |
| `greet-abandon` cancelled and removed, 19:27 | `main` `1aff8e5` | `main` `ab78fad`, `main-weft` `2211878` | + `greet-abandon/6669d7eb5fb3` | none |

Origin also keeps `refs/pull/1/head` (`5b9175e`), the head of pull request #1.
GitHub owns that ref; it is not in the branch list, and lym does not push it.

At the end, the local tips equal origin's:
`_bolt` at `ab78fad`, `main-weft` at `2211878`, the code store's `main` at `1aff8e5`.
The records store holds the five archive tags under `refs/tags/archive/`, mapped from origin's `refs/lym/tags/` by its fetch refspec.
The code store holds only `main`, `origin/main` and `origin/HEAD`.

## Runs

Each run took the darn rows in the same order:
Preflight, Record-Write, the Drift rows (skipped: the sandbox has drift off), Darn-Write, Code-Bouncer, one burler review and fix round, Code-Bouncer again, Describe, PR-Review, Finalize.
Every run took exactly one burler round, and only the halt below stopped a run.

| Task | Started | Done | Note |
|---|---|---|---|
| `greet-goodbye` | 18:43 | 19:12 | PR-Review 18:47 to 19:11 with pull request #1 open; halted at Finalize 19:11:23, resumed 19:11:45 |
| `greet-shout` | 19:12 | 19:17 | PR-Review passed with no pull request; the rules read was logged as a Warn |
| `greet-many` | 19:18 | 19:24:32 | in flight with `greet-doc` from 19:18 to 19:24; landed first |
| `greet-doc` | 19:18 | 19:24:54 | in Finalize beside `greet-many` from 19:24:11; landed 22 s after it, on `1aff8e5`, with no commit |
| `greet-abandon` | 19:26 | cancelled 19:27 | cancelled at Seed-IDE once both its branches were on origin |

## The Finalize halt

`greet-goodbye` halted at Finalize with:

```
landing stop: GitHub refused the token for the call to read GitHub's rules for the parent branch:
pullrequest: read the rules active on branch main:
GET https://api.github.com/repos/Knatte18/lym-sandbox-onerepo/rules/branches/main?per_page=100:
403 Upgrade to GitHub Pro or make this repository public to enable this feature. [];
way forward: make sure the gh user's token (GH_TOKEN, GITHUB_TOKEN, or gh auth login --hostname github.com) is valid and can reach the repository, then run lym loom resume
```

The trigger was the orch's own step: it set `pull_request` off at the sandbox's default layer while the run held pull request #1.
Finalize then resolved "no pull request" and read the parent branch's rules, and a run holding a pull request record halts on any failure of that read.

The bug is the reading of the 403.
GitHub gates repository rules behind a paid plan for a private repository, so the endpoint answers 403 to every token.
`githubHaltReason` in `internal/landingshed/finalize_pr.go` maps every 403 to a token failure,
so the halt blames the token and names a way forward that cannot work: a resume meets the same 403.
The orch set `pull_request` back to on and resumed, and Finalize merged pull request #1.

A run with no pull request record reads the same 403 as no rule, with a Warn, and lands.
`greet-shout`'s PR-Review and Finalize both logged:

```
landingshed: GitHub's rules for the parent branch are unreadable, so the landing goes as beam.yaml and the entry say
```

Note B2 on this hub's board, `rules-read-plan-gated-403`, holds the bug and a proposed fix.

## The abandon

The orch released `greet-abandon` with `lym board set-status`.
Once origin showed `refs/heads/greet-abandon` (`1aff8e5`) and `refs/lym/heads/greet-abandon-weft` (`6669d7e`), with the run at Seed-IDE, it ran from the sandbox prime:

```
lym batten cancel greet-abandon --remove
lym board remove '{"slug":"greet-abandon"}'
```

The cancel answered `ok: true` with `pair_removed`, `records_removed` and `claim_cleared` true, `archive_tags` null and `snapshot_tag` empty.
Origin then held neither branch and held `refs/lym/tags/archive/greet-abandon/6669d7eb5fb3`, the records branch's head.
`archive_tags` is null because that field lists archives of the code branch tip,
and the code branch sat on `main`'s tip, which origin already contains.
The hub directory held no `greet-abandon` pair afterwards, and the board was empty.

## Gaps

- **The concurrent pair landed one code change, not two.**
  `greet-doc` asked for a package doc comment that the seed's `greet.go` already carried, so its writer committed nothing,
  and its Finalize landed an empty branch after `greet-many`.
  The trial shows two runs in flight at once, each with its own branches on origin, and the two landings one after the other;
  it does not show a second landing that merges a change onto a parent another landing just moved.
  The five-run cap rules out a rerun in this trial; trial 3 can cover it.
- **Only one landing went through a pull request.**
  With `pull_request` off, the other three landings pushed `main` directly.

## Friction

- **The sandbox board refuses the task session two steps the trial needs.**
  `lym board upsert` with `"kind":"task"` answers "only the operator decides what becomes a task",
  and a `profile` or `run_settings` field answers that only the operator, the hub orch or the entry's beam orch may set it.
  Starting the sandbox's dobby is the orch's too.
  So the orch filed, released, chained and abandoned every task, and the task session observed.
  Trial 3 should expect the same split.
- **A plan-gated 403 reads as a token failure.**
  Covered above; it cost one halt and one round trip to the orch.
