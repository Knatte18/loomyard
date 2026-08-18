# reed — independent review, round 5 (`opus-high-r5`)

Scoped round: **state-loss / state-corruption recovery only**, per `_mill/reed-review-prompt.md`'s "Scope".
Clean-room: findings below were formed without reading any `_mill/reed-review-*` file or `_mill/reed-shuttle-HANDOFF.md`.
Model/effort: Opus / High.

## Substrate

- Host Linux, `tmux 3.6` at `/usr/bin/tmux`, resolved via `PATH`.
- Dev binary deployed with `./deploy-dev` (`.dev-bin/lyx`), redeployed after every source change.
- Fixtures built by hand under the scratchpad: `<scratch>/r5/<name>-HUB/<worktree>` — a plain `git init` worktree
  one level under a `-HUB` parent, which is all `lyxcwd.Resolve` needs (anchor falls back to `"."`, reed's config
  degrades to its embedded template).
- Teardown evidence uses `ps -eo comm | grep -cx 'tmux: server'` and `tmux -L <socket> ls`, never `pgrep -x tmux`.

## What was tested

(appended as each command/scenario returned — see the per-scenario sections below)

### Baseline (pre-scenario sanity)

```
$ ps -eo comm | grep -cx 'tmux: server'      -> 0            (clean start)
$ go build ./...                              -> OK
$ ./deploy-dev                                -> Deployed lyx @ a3d2dec7
$ tmux -V                                     -> tmux 3.6
```

Fixture `kappahub-HUB/{svc-alpha,svc-beta}`; in `svc-alpha`:

```
$ lyx reed up
{"ok":true,"session":"svc-alpha","socket":"lyx-kappahub-HUB-64c9b3a1","strands":0}
panes: %1 dead=0 top=0 h=25 (header)   %0 dead=0 top=26 h=24
$ lyx reed add --cmd 'sleep 100000' --name alpha1   -> guid 6deee84a…, pane %0 (adopted)
$ lyx reed add --cmd 'sleep 100001' --name alpha2   -> guid 44ebc2b3…, pane %2
$ lyx reed status -> both live:true
```

## Findings

(filled in below, per scenario)
