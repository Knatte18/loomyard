# One-repository sandbox, trial 1: create the repository and clone the hub

First of three trials of the one-repository hub layout (57015a52) in a new sandbox.
It creates a private GitHub repository seeded with a small Go module,
clones it with `lym fabric clone --single-repo`,
and checks that origin shows only the code branch while the records live under `refs/lym/heads/`.

```yaml
date: 2026-11-21
lym_build: 57015a52cd1f (/home/knatte/go/bin/lym)
repository: Knatte18/lym-sandbox-onerepo (private)
hub: /home/knatte/Code/lym-sandbox-onerepo-LYMHUB
shortname: sbone
result: every check passed
```

## Result

Every check in the task passed.
Origin's branch list shows `main` alone and its tag list is empty;
`git ls-remote` shows the board at `refs/lym/heads/main` and the prime's records branch at `refs/lym/heads/main-weft`.
`lym board list`, `lym fabric status`, `lym fabric pairs`, `lym fabric list` and `lym fabric shortname` all answer `ok: true` in the hub.
No lym bug was found, so no note was filed.
One friction point: the task session cannot run the clone, so the orch ran it (see Friction).

## Commands and outcomes

### 1. Create the repository

```
gh repo create Knatte18/lym-sandbox-onerepo --private --description "lym one-repository hub sandbox"
```

Exit 0; the repository is private with default branch `main`.
No other repository was created.

### 2. Seed the Go module on `main`

Three files went in through the contents API (`gh api -X PUT repos/Knatte18/lym-sandbox-onerepo/contents/<file>`), one commit each,
so no local seed checkout was needed:

| File | Commit | Content |
|---|---|---|
| `go.mod` | `247087f` | module `github.com/Knatte18/lym-sandbox-onerepo`, `go 1.24` |
| `greet.go` | `f03faed` | package `greet`, `Hello(name string) string` |
| `greet_test.go` | `f95da8c` | `TestHelloGreetsTheName`, parallel, exact-equality check |

`main`'s tip is `f95da8c`.

### 3. Clone as a one-repository hub

```
lym fabric clone --single-repo --shortname sbone --into /home/knatte/Code https://github.com/Knatte18/lym-sandbox-onerepo
```

From the task session this was refused (exit 1):

```
lym fabric clone is refused from the task session ly:onerepo-sandbox-setup:darn; build a scratch hub as a hubforge test fixture instead, and when the work itself needs this verb, stop and report status: FAILED so the run ends stuck and the orch runs it
```

The orch (`ly:orch`) then ran the same command.
It reported exit 0, `ok: true`, `partial: false`, no warning,
with remote updates `origin/main-weft -> a99f6c8e` and `origin/main -> 3ac1a6af`.
Those are the clone's first seed pushes: the records store later carries more commits on both branches,
and origin's tips match the local tips (step 4).

The hub holds the code store `lym-sandbox-onerepo`, the records store `lym-sandbox-onerepo-weft`, the board worktree `_bolt`
and `lym-sandbox-onerepo.code-workspace`.
The code worktree's `.lym`, `.scratch` and `_lym` are links into the records store.

### 4. Origin refs

```
gh api repos/Knatte18/lym-sandbox-onerepo/branches --jq '.[].name'
main

gh api repos/Knatte18/lym-sandbox-onerepo/tags --jq '.[].name'
(empty)

git ls-remote https://github.com/Knatte18/lym-sandbox-onerepo
f95da8c0659d33dc38831aa1e027a2ec08fb831b	HEAD
f95da8c0659d33dc38831aa1e027a2ec08fb831b	refs/heads/main
db82e1dd1adb5b501a1abb599968db7e1f70e7e6	refs/lym/heads/main
9f8c6aa43e50b6d0daa2f3e8ba6b8f65e38216af	refs/lym/heads/main-weft
```

The code branch is the only branch GitHub lists; the board and the prime's records branch sit under `refs/lym/heads/`.
No ref under `refs/heads/` other than `main`, and no tag, was pushed.

Local tips match origin:

| Store | Branch | Local tip | Last commit |
|---|---|---|---|
| code | `main` | `f95da8c` | Seed greet_test.go |
| records | `main-weft` | `9f8c6aa` | lym: reconcile config for build 57015a52cd1f |
| records (`_bolt`) | `main` | `db82e1d` | board sync |

### 5. Store configuration

The records store (shared by `lym-sandbox-onerepo-weft` and `_bolt`) carries the layout's refspecs:

```
remote.origin.fetch +refs/lym/heads/*:refs/remotes/origin/*
remote.origin.fetch +refs/lym/tags/*:refs/tags/*
remote.origin.push refs/heads/*:refs/lym/heads/*
remote.origin.push refs/tags/*:refs/lym/tags/*
remote.origin.tagopt --no-tags
lym.primerecordsbranch main-weft
branch.main.merge refs/lym/heads/main
branch.main-weft.merge refs/lym/heads/main-weft
```

The code store keeps git's default `+refs/heads/*:refs/remotes/origin/*` and `branch.main.merge refs/heads/main`.
Its refs are `refs/heads/main`, `refs/remotes/origin/main` and `refs/remotes/origin/HEAD`, with nothing from the records namespace.

### 6. lym in the hub

Run from the code worktree:

| Command | Outcome |
|---|---|
| `lym board list` | `ok: true`, no tasks |
| `lym board labels` | `ok: true`, no labels; the types `sketch`, `bug` and `enhancement` |
| `lym fabric status` | `ok: true`, no changes, no failed pushes, no merge in progress |
| `lym fabric pairs` | one pair, `main` ↔ `main-weft`, `in_sync: true`, `junction_healthy: true` |
| `lym fabric list` | one worktree, `main` at `f95da8c` |
| `lym fabric shortname` | `sbone` |

## Friction

- **The task session cannot run the clone.**
  This task's body has a task session clone a hub, but `lym fabric clone` refuses every task session and names the orch as the one to run it.
  The refusal worked as designed and named its way forward;
  the cost was a round trip to the orch.
  Trials 2 and 3 should expect the same for any hub-creating verb.
- **The clone's summary names its first pushes, not the final tips.**
  The reported `a99f6c8e` and `3ac1a6af` are the first commits the clone pushed;
  the reconcile and board-sync commits after them moved origin to `9f8c6aa` and `db82e1d`.
  Origin and the local tips agree, so the layout is sound,
  but a reader checking origin against the clone's output sees different SHAs.
- **The sandbox board has no labels.**
  `lym board labels` lists none, so trial 2 must create labels before it files a labelled entry.
