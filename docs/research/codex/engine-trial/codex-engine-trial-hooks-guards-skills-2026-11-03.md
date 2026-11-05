# Codex engine trial: hooks, shell guards and skills

Project `codex-engine-trial`, parts 3, 4 and 6, run on 2026-11-04 against real Codex sessions launched through `lym shuttle`, as `internal/shuttleengine/codexengine/doc.go` describes them.
Codex CLI 0.164.0, model `luna[medium]` (`gpt-6-luna`), every case in this task's worktree.
Verdict: every case matches the package doc.
Two lym faults turned up outside the doc's claims, filed as board notes B4 and B5.

## Method

- A run is `lym shuttle run --model 'luna[medium]' --keep-pane`, since a done run otherwise removes its run directory, the events file with it.
- An output file the agent writes ends a run as done; a first run that named none held at the turn end until its timeout, as `lym shuttle run --help` says.
- Three cases needed input no shuttle verb gives:
  - The interrupt (case 3) is `Escape` sent to the strand's pane with `tmux send-keys`.
  - The second and third turn (case 4) and the turn of an opened session (case 1) are text typed the same way, the Enter sent apart from the text.
  - Cases 6 and 7 use throwaway builds, `go build -overlay` over a scratch copy of one file, so the worktree stays unchanged.
- Rollouts are read from the `transcript_path` the hook payloads name, under `~/.lym/codex/sessions`.

## Case 1: hooks in a run and in an opened session

Run: `echo hooks-trial-1k`, then a file write, then a reply `done-1k`.

Seen in the run's events file, one stamp line (`lym_stamp`, `lym_at`) before each payload line:

| Hook | Payload keys seen |
|---|---|
| SessionStart | `session_id`, `transcript_path`, `cwd`, `hook_event_name`, `model` (`gpt-6-luna`), `permission_mode` (`bypassPermissions`), `source` (`startup`) |
| UserPromptSubmit | the same keys plus `turn_id` and `prompt`, the launch pointer to `prompt.md` |
| Stop | the same keys plus `turn_id`, `stop_hook_active` (false) and `last_assistant_message` (`done-1k`) |

PostToolUse records no payload, as the doc says: the run's `activity-stamp` file was created and touched during the turn (mtime 15:27:26, inside the turn that ran 15:27:17 to 15:27:27).

Opened session: `lym shuttle open --role trial1o --model 'luna[medium]'` started Codex at its composer with no run directory, no events file and no activity stamp.
A typed turn `python3 --version` ran and printed `Python 3.14.4`, which the python guard denies in a run, so the session carries no hooks and no guard.

Matches the doc: hooks ride a run's launch line, and `OpenLaunch` carries none.

## Case 2: PermissionRequest

Turn: `touch /etc/lym-trial-probe`, a path outside the sandbox.
The events file holds no PermissionRequest.
The command failed with `Read-only file system` and the model reported that.
The run's `turn_context` carries `approval_policy: never` and a `workspace-write` sandbox with `network_access: false`.

Why none is raised: with `-a never` Codex asks for no approval, so a command the sandbox refuses ends as a tool error and no approval request reaches the hook.
The PermissionRequest hook is installed, but this launch line has no way to fire it.
Not tried: a launch with another approval policy, which no lym verb offers.

Matches the doc's launch line (`-a never -s workspace-write`) and its note that no approval prompt shows.

## Case 3: interrupt

Three runs (3a, 3b, 3c), each a turn that runs `sleep 240`, interrupted about a minute in with `Escape`.

| Run | Stop in events file | Newest `turn_aborted` in the rollout |
|---|---|---|
| 3a | none | reason `interrupted`, 14:27:59.847Z, the turn's own id |
| 3b | none | reason `interrupted`, 14:30:12.671Z |
| 3c | none | reason `interrupted`, 14:30:12.699Z |

The pane read `Conversation interrupted` each time.
In 3a and 3b the pane showed `1 background terminal running` at the interrupt, so the tool call was live.
The 3c pane showed no background terminal, so that interrupt may have landed before the command started.

`lym shuttle state` for 3a lists in its history `idle-fresh/session-start`, `busy/turn`, then `idle-stalled/interrupt` since 14:27:59Z, the `turn_aborted` time.
The current state after that entry is `unknown/liveness-unproven`: see Faults.

Matches the doc: an interrupt fires no Stop, and the newest `turn_aborted` marks the turn interrupted.

## Case 4: context size

One interactive run of three turns (`turn-one`, `turn-two`, `turn-three`), the last two typed into the pane.
The events file holds three UserPromptSubmit and three Stop payloads.
The rollout's `token_count` events carry `last_token_usage.total_tokens` 14745, 15192, 15258, 15436, 15578 and 15745 in file order, and a cumulative `total_token_usage.total_tokens` of 91954 at the end.

A scratch program called `codexengine.New().ContextTokens` on each Stop payload and read 15745, known, for all three.
That is the newest `token_count`'s last-call value, not the cumulative one, so it matches the doc.
It is the file's newest value at read time, so an engine read of an earlier Stop after later turns reads the later size; a read at the turn end, as the run loop does, gets that turn's.

## Case 5: shell guards

The prompt file of a run opens with seven notices, in table order and in Codex wording (`apply_patch` for edits, `a subagent of your own` for the headless claude row):
python, raw go, refuse raw, shell edit, computed rm, machine sleep, headless claude.
The publish row is not installed, since `lym shuttle run` sets no publish guard.

One run issued one command per installed row, each denied with no retry.
A guard that failed to deny would run each of them harmlessly: `--help` forms for the two that start programs.

| Row | Command | Result |
|---|---|---|
| python | `python3 --version` | blocked, python steer |
| raw-go | `LYM_STRAND_NAME=x true` | blocked, raw go steer |
| refuse-raw | `go test ./internal/shellguard -run XXX_NONE` | blocked, refuse-raw steer |
| shell-edit | `echo x > .scratch/guard-sentinel.md` | blocked, Codex shell-edit steer; the file was not created |
| computed-rm | `rm .scratch/guard-$HOME-none` | blocked, computed-rm steer |
| machine-sleep | `systemctl --help suspend` | blocked, machine-sleep steer |
| headless-claude | `claude --help --print` | blocked, Codex headless-claude steer |

Each result reads `Command blocked by PreToolUse hook: <the row's Codex steer>`, word for word the steer in `internal/shellguard/rows.go`.

Matches the doc.

## Case 6: a guard hook that fails to run

Throwaway build: `go build -overlay` swapping `guards.go` for a copy whose every guard hook command is `cat >/dev/null; echo lym-trial-guard-hook-crash-<row> >&2; exit 3`.
One run issued `echo x > .scratch/guard-sentinel2.md` and `python3 --version`, the two the real guards deny.

Both ran: the sentinel file was created and `python3` printed `Python 3.14.4`.
The model saw no hook text in its tool output and reported no warning.
It shows only in the pane: a `Hook failed` line with `hook exited with code 3` for each installed row on each tool call, 21 lines in the captured pane.
The rollout holds no mention of the crash, and neither does the events file.

Matches the doc: a hook that fails to run fails open, so a guard is a guardrail and not a barrier.
lym has no signal of its own that a guard hook failed.

## Case 7: skills

Throwaway build: a `Spec.Skills` list read from an environment variable, and a template with a second skill root, `.scratch/skillroot`, since `lym shuttle run` has no flag for skills.
Each run's first turn is the load message and its second the prompt pointer.

| Load | First turn | Seen |
|---|---|---|
| `scribe:prose` | the fixed opening plus `.../scribe/scribe/1.1.0/skills/prose/SKILL.md` | a tool call reads that path; reply `ok prose`; no skip warning |
| `trial:ver`, with `ver` at 1.2.0 and 1.10.0 | the opening plus the `1.10.0` path | reply `ok ver NEW-VERSION-1.10.0`; the older file is not read |
| `nosuch:skill`, under no root | `No skill file is to be read now. Do nothing else, and reply only `ok`.` | reply `ok`; warning `shuttle: skill skipped ... cause=unknown` |

The newest version is chosen by number, since 1.10.0 beat 1.2.0.

Matches the doc.

## Faults

Each is a board note, `kind: note`, label `bug` and the module labels.

- B4 `shuttle-hidden-anchor-dies` (shuttle, reed): `lym shuttle run --anchor hidden` ends died on Codex and on Claude, after two start attempts, with `provider never became ready` and no pane capture. Three hidden runs, three deaths. Outside codexengine, found while running case 1.
- B5 `codex-state-always-unknown` (shuttle, codex): `lym shuttle state` reads every Codex run as `unknown/liveness-unproven`, because the session fold lets an unproven liveness win over the derived state, which hides a running turn and the interrupt that case 3 shows in the state's history.

## Observed, not faults

- A done run's `lastAssistantMessage` and `sessionId` read empty in the `lym shuttle run` envelope for Codex. The empty message is documented on `shuttleengine.Result`; the empty session id follows from Codex minting its id at the first turn.
- `lym shuttle run` and `lym shuttle open` give no way to name skills, so skill loading is reachable only from code that builds a `Spec`.
