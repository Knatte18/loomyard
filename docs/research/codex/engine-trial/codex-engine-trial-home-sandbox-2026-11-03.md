# Codex engine trial: home, trust, sandbox and refusals

Measurement report, run 2026-11-04 on `codex-cli 0.164.0` against `internal/shuttleengine/codexengine/doc.go`.
Every case ran once on `luna[medium]` (`gpt-6-luna`, effort medium), in the `codex-trial-home-sandbox` worktree.

## Launch path

`lym shuttle run --model luna[medium]` launched a Claude session that stopped with `model_not_found`: `run --model` is a provider id on shuttle.yaml's `engine`, and `run` has no engine choice.
`lym shuttle open --model 'luna[medium]'` picks Codex but is refused: `the engine "codex" cannot open a session with no prompt`.
The cases ran with a local override `engine=codex` in the worktree's `shuttle.yaml` (`lym config shuttle --layer local --set engine=codex`) and `lym shuttle run --model gpt-6-luna --effort medium --prompt-file … --output-file …`; the override was removed after case 7.
Both launch faults are filed as bug notes (below).

## Operator home

`~/.codex/config.toml` sha256 `be2329ce686058f5305eea986faaab7d581869f45b4716613c5cf1a36f0c0e76` before the first Codex launch and after case 7; its mtime (2026-10-28 09:33) and `auth.json`'s are unchanged.
The Codex session's write probes at `~/.codex/` were refused (case 5), and no trial file landed there.

## Cases

### 1. A launch in this worktree

Run: a session printed `$CODEX_HOME`, listed it and printed its `config.toml`.
Seen: `CODEX_HOME=/home/knatte/.lym/codex`; `auth.json -> /home/knatte/.codex/auth.json` and `sessions -> /home/knatte/.codex/sessions`; `config.toml` opens with lym's header, carries `[tui] screen_reader_detection_done = true` and one `trust_level = "trusted"` table for the worktree root and one for the main checkout root (`…/loomyard`).
Matches the package doc.
Codex added `"gpt-6.1-sol" = 1` under `[tui.model_availability_nux]` after startup; the doc names Codex's own write.

### 2. Two launches started together

Run: lym trust record and `config.toml` removed, then two `lym shuttle run` started in one shell command, about 0.1 s apart.
Seen: both runs reached `done`; the two sessions read byte-identical `config.toml`, whole, with both roots trusted; `lym-trust.json` lists the same two roots.
Matches the package doc.
The two launches share the same roots, so the case shows the lock keeps the file whole, not that two different roots merge.

### 3. A recorded root whose folder is removed

Run: a folder under `.scratch/trial/` added to `lym-trust.json` by hand, the folder removed, then a launch.
Seen: the root is absent from `lym-trust.json` and from the `config.toml` the session read; the two live roots remain.
Matches the package doc.

### 4. A regular `auth.json` in the lym home

Run: the `auth.json` link replaced by a copy of the operator's file (same sha256), then a launch.
Seen: Warn `the lym Codex home's entry is not a link to the operator home's; way forward: remove it, and the next Codex launch links it again`, with `path` and `want`; the copy stayed a regular file after the run.
Removing the copy and launching again (case 5) made the link anew.
Matches the package doc.

### 5. Launch line and sandbox

Run: the launch script `.lym/reed/launch/<guid>.sh` of a live session was read, and a session ran four write probes.
Seen: the line is `CODEX_HOME='/home/knatte/.lym/codex' 'codex' --no-alt-screen -a never -s workspace-write -c 'sandbox_workspace_write.writable_roots=["/home/knatte/Code/loomyard-LYMHUB/codex-trial-home-sandbox-weft"]' …`, so the records checkout is the one added root and no approval prompt showed.
`echo ok > .lym/trial-case5-inside.txt` succeeded (exit 0).
`echo no > /home/knatte/trial-case5-outside.txt` and a write to `/home/knatte/.codex/` both failed with `Read-only file system`, and neither file exists.
Matches the package doc.
Inside the Codex sandbox `ps -eo args` lists no process, so the launch line cannot be read from there.

### 6. Committed `.codex`

Run: an untracked `.codex/rules/trial.rules` in the worktree root, then an untracked `.codex/config.toml`.
Seen: with the rules folder, `lym shuttle run` exited 1 with `…/.codex/rules holds committed Codex rules, and an allow rule there lifts the sandbox for its command; way forward: …`, a Warn `refused a Codex launch under committed Codex rules`, and no session.
With the config file the launch went ahead and logged Warn `the worktree commits a Codex config, whose hooks and settings the session runs without review`, naming the path.
Matches the package doc.
The check reads the folder's presence, not whether git tracks it; the trial files were untracked, so "committed" is looser than the doc says.
The refusal's way forward ends in "clear the round's assist model key", which fits a review round and not a plain `lym shuttle run`.

### 7. The folder-trust prompt

Run: a shell loop rewrote the lym home's `config.toml` without any project table every few milliseconds for 40 s, while a launch started, so Codex read a file with no trusted root.
Seen: the run's trace logs `shuttle: dismissed startup gate inputs=1` at launch, a line none of the other cases' traces holds; the run reached `done`; after startup `CheckAfterStartup` logged Warn `the lym Codex home's config lost a trusted root after startup` for both roots.
Matches the package doc.
The trace does not name the gate; that it was the trust prompt rests on the stripped file being the only difference from the other launches.

## Faults filed

| Note | Fault |
|---|---|
| `shuttle-run-no-model-spec` | `lym shuttle run --model` takes no model spec and has no engine choice; a rejected model ran to the timeout. |
| `shuttle-open-refuses-codex` | `lym shuttle open` lists Codex aliases and refuses the codex engine for lack of a prompt. |
| `codex-test-writes-real-home` | `TestPrepare_AppendsStandingPassages` in `codexengine/prepare_test.go` resolves the template's `~/.lym/codex`, so each gate run adds `/tmp` roots to the real trust record; seen in the home before the first launch and again during case 1. |
