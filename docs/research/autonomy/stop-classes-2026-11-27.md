# Run stops, who settled them, and a proposed settlement table

This study gives the design of unattended runs its facts: which stops runs hit, how often, and who settled them how.
It covers the loomyard hub on one machine, from 2026-11-13 to 2026-11-27.
Nothing was run or probed; every figure is read from records.

## Findings

- Runs entered 131 halted states that no one had asked for.
  lym settled 67 of them, the orch 54, and the records of 10 do not say.
- The driver recorded 62 further stops that left no halted state.
- Most halts were short: the orch and the driver's agents act within minutes.
  The long holds were elsewhere: agents waiting on an answer, dead drivers after a machine crash,
  and agents behind a permission dialog or a hung command.
- Review escalations are the largest halted class (62 of 131), and the arbiter may never take cause `budget`.
- Several classes have not recurred since a fix landed in the window.
- The quiet notice fires for two correct waits it does not look for:
  an operator's approval at `Raddle-Write`, and a held turn end awaiting a creel reply.

## Sources and limits

| Source | Used for |
|---|---|
| `Run-Transition` commits in the hub's records repository, and each commit's `status.json` | halted states, their reasons and how long each held |
| `go-settle.jsonl` per run, and the tackler sessions | which driver rule or agent settled a stop |
| the two orch session transcripts | what the orch and the operator did |
| the creel journal | program notices and agents' questions |

- The sources hold no record dated 11-13, 11-14, 11-20, 11-24 or 11-25. Halted states start on 11-15.
- The creel journal covers only 11-21, 11-22, 11-23, 11-26 and 11-27.
  Before 11-21 notices come from the orch transcripts alone.
- Hold time of a halted state runs from its commit to the run's next `running` commit.
  A resume that failed again counts as a new stop.
- Hold times of stops with no halted state come from notices and transcripts, and are approximate.
- Counts read from transcripts were sorted by hand and are marked "about".
- `beam.autonomy` was off for nearly the whole window; it was switched on in the evening of 11-27.
  The records therefore say almost nothing about how autonomy behaves.
- Times below are UTC unless the row says otherwise.

"lym" below means the driver's rules together with the agents the driver spawns, the tackler and the arbiter.
"orch" is the hub orchestrator, and "operator" the person.

## 1. Stop classes

### Halted states

The 154 recorded halted states include 13 pauses a bench asked for and 10 hub pauses, which are not counted below.

| Class | Count | Example run | Settled by, and how | Held: median / max |
|---|---|---|---|---|
| Review escalation, `decision` | 24 | `adopt-redesign` (11-15) | lym 13 (arbiter resume); orch 11 (`decision add`, then `circling continue` or `accept`) | lym 106 s / 5 min; orch 3.6 min / 41 min |
| Review escalation, `hard-cap` | 20 | `weaver-implementer` (11-16) | lym 17 (arbiter accept); orch 3 (parent decisions, `circling accept`, resume) | lym 71 s / 2.5 min; orch 2 min / 3 min |
| Review escalation, `budget` | 14 | `agents-exit-before-reply` (11-23) | orch 14 (`circling accept` or `continue`, resume) | 73 s / 95 min |
| Review escalation, `circling` | 4 | `codex-same-as-claude` (11-16) | lym 4 (arbiter) | 75 s / 105 s |
| Bounce budget exhausted | 3 | `lint-comments-reads-head` (11-23) | orch 3 (`loom resume`) | 104 s / 34 min |
| Board claim push refused at `Worktree-Create` (held lock, moved ref) | 13 | `lym-retro` (11-16) | lym 13 (tackler `resume`, `goto Worktree-Create`); on two the orch also ran `board sync` and `batten run` | 32 s / 105 s |
| Origin or GitHub unreachable | 4 | `reach-misses-tier1-calls` (11-27) | lym 4 (tackler or driver re-step) | 33 s / 49 s |
| Agent launch failed, provider "never became ready" | 12 | `intake-checks-already-fixed` (11-23) | lym 2 (hand-over to a fresh build); 10 not recorded, two of them handed to the orch | 14 s / 17 min |
| Defect or hub config at a row | 9 | `single-repo-records-branches` (11-19) | orch 8 (fix the cause, resume or `goto`); lym 1 | 34 s / 68 s |
| Webster batch failed or run-exit audit finding | 5 | `config-keys-nested` (11-26) | orch 5 (amend the plan, or remove the stray file, `accept-audit`, resume) | 81 s / 8.5 min |
| Gate attempts exhausted | 4 | `raddle-slug-length-check` (11-19) | orch 4 (`decision add`, resume) | 39 s / 43 s |
| Review file not valid YAML | 4 | `board-view-not-refreshing` (11-18) | lym 3 (tackler or hand-over); orch 1 | 51 s / 82 s |
| Running build older than a board stencil | 4 | `hardener-self-charter` (11-21) | lym 3 (hand-over to a fresh build); orch 1 | 18 s / 24 s |
| Fixer fork failed | 3 | `leftover-pairs-teardown` (11-17) | lym 3 | 15 s / 55 s |
| Agent session timed out | 3 | `weaver-bench` (11-27) | lym 3 (fresh driver) | 13 s / 21 s |
| Strand lost from reed | 3 | `cwb-codex` (11-17) | orch 3 (kill the stale driver, `goto`, or re-seed) | 63 s / 151 s |
| Landing stop | 1 | `pr-modes-no-agent` (11-27) | orch, after the operator reported it stuck (`loom resume`) | 5 min |
| Deploy failed at `Land` | 1 | `pr-review-reask-not-delivered` (11-23) | lym (tackler; a later commit had fixed the build) | 64 s |

A `budget` round that can settle as an accept does so without halting, so those are not in this table.

### Stops with no halted state that the driver settled

The batten counts are tackler sessions; the 62 stops also hold the interrupted loom steps in the last row.

| Class | Count | Example run | Settled by, and how | Held |
|---|---|---|---|---|
| Batten step refused: the code tip was not on origin yet | 35 | `orch-ignores-deny-python` (11-18) | lym: tackler checks origin, `resume` | about 16 s of tackler time |
| Records push refused as "ended on origin" while the commit was on origin | 11 | `scratch-symlink-two-paths` (11-18) | lym: tackler resumed 4, escalated 6 to the arbiter; 1 handed back | about 50 s of tackler time |
| Batten step refused: a pause covers the run | 2 | `redundant-check-fixes` (11-18) | lym: tackler, after the pause lifted | under 1 min |
| Loom step interrupted, or driver replaced by a newer build | 15 | `harden-fabricengine` (11-22) | lym: re-step or hand-over once a driver was back | see "dead driver" below |

### Stops seen only in notices and transcripts

| Class | Count | Example run | Settled by, and how | Held |
|---|---|---|---|---|
| Agent asks its parent and waits | 73 questions (11-21 on) | `board-readme-golden-tests` (11-22) | orch reply, often with `decision add` | 65 answered within 2 min; 5 took 3 to 30 min; 3, one episode, 222 min |
| Raddle proposal waits for approval | about 18 | `fabric-git-free-core` (11-26) | operator, sometimes after an orch review | 1 min to about 9 h (overnight) |
| "dobby could not start the task" (push lock busy, origin unreachable) | about 17 | `keep-landed-records` (11-26) | orch `batten run` | about 1 min |
| Failed-push notice for records or board | about 9 in the orch's mail | prime records (11-26) | orch `fabric push` or `board sync` | about 1 min; once 28 min |
| `loom resume` refused with `driver_not_parked` | 16 orch commands wrap a retry | `scope-and-earlier-checks` (11-27) | orch retry loop | up to 5 min |
| Dead drivers after a machine crash or freeze | about 4 episodes, several runs each | `adopt-build-resplit` (11-17) | orch `batten run` or `loom resume`, mostly after the operator reported the crash | 23 min to overnight |
| Agent API error or internet outage | about 4 episodes | `rc-click-wrong-client-session` (11-26) | revive, orch post, or the operator by hand; one 529 cleared itself | about 40 min; one about 2.5 days, most of it with lym not running |
| Provider permission dialog | at least 5 | `mutation-trial` (11-19) | orch, once by typing Enter into the pane; others unrecorded | about 4 to 9 min |
| Hung command, gate or helper inside an agent | 3 | `config-keys-nested` (11-26) | orch kill or redesign, two after the operator pushed | 18 to 40 min |
| Step only the operator can take | at least 2 | `kde-window-placement-build` (11-27) | operator | over 3.5 h, still open at 19:23 |
| Row waiting with no agent session | 1 defect | a `PR-Review` row (11-27) | operator found it; a fix landed | about 2 h from report to fix |

### What the operator had to push

The operator repeatedly had to tell the orch that a run stood still.
The recurring subjects were: an escalation the orch had answered without releasing the run
(`config-keys-nested-by-area`, `agents-exit-before-reply`),
an agent's question nobody had acted on (`board-readme-golden-tests`),
crashes, and runs that looked idle while they waited behind a gate or a landing lock.

### Classes that stopped recurring

| Class | Last seen | Landed change |
|---|---|---|
| Records push "ended on origin" | 11-18 | not traced |
| Code tip not on origin | 11-23 | `b4236816a` moves pushes to a background pusher |
| Launch failed on an unread input box | 11-23 | `b0a969d6e` |
| Review file not valid YAML | 11-19 | `e6b40df6a` |
| Strand lost at a gate re-prompt | 11-17 | `3746960e1` |

Absence over a few days does not show a class is gone.
Two more changes landed too late to judge: `5ac2b64a4` (network re-step) and `f1311f21d` (machine stops, idle seats).
At the time of writing, the unlanded branch `push-retry-not-halt` covers
the push-lock, failed-push and landing network classes.

## 2. Mechanical or judgment

Mechanical means a bounded retry or a fixed answer settles it.

| Class | Kind | What the judgment is |
|---|---|---|
| Escalation, `hard-cap` | mechanical | The only decision open is accept. Whether an open finding needs a follow-up note can be a fixed rule: file one for each. |
| Escalation, `budget` | mixed | Mechanical when every open finding is one the fixer deferred. Otherwise: accept a blocking finding, or buy one more round. |
| Escalation, `decision` | judgment | A design choice the record does not make, within the task's scope, or a note once scope has closed. |
| Escalation, `circling` | judgment | Whether the judge is stuck on a wrong design or on a finding not worth another round. |
| Bounce budget exhausted | mechanical once | One more budget by resume; a second exhaustion needs the findings read. |
| Board claim push, network, code tip, pause-covered, push lock busy | mechanical | none |
| Records push "ended on origin" | mechanical | none once origin is shown to hold the commit; the check is the fix |
| Launch failed, session timeout, fixer fork, strand lost | mechanical once | none; a repeat points to a defect |
| Review file not valid YAML | mechanical | none: re-prompt the writer with the parse error |
| Stale build | mechanical | none |
| Gate attempts exhausted | judgment | Whether the gate or the change is wrong, and whether a test waiver applies. |
| Webster failure or audit finding | judgment | Whether to amend the plan or to accept a stray write. A friction note in the findings is mechanical (`b7706e5c4`, `20d3c9ee1`). |
| Defect or hub config at a row | judgment | Finding the cause; it is often outside the run. |
| Landing stop, deploy failure | judgment | What the pushed commits or the failed build need. |
| Agent question: test waiver (13 of 73) | judgment | Whether no test can fail on the base. |
| Agent question: entry overtaken or contradicted (11) | judgment | Narrow, cancel or continue. |
| Agent question: a measurement or test the session may not run (6) | mechanical | none: run the named thing and report |
| Agent question: refused or operator-only step (12) | mixed | Mechanical where the step has an allow rule; otherwise only the orch or operator may take it. |
| Agent question: raddle review (7), gate setup fault (3), other design or scope call (21) | judgment | The content of the answer. |
| Raddle approval | judgment | Whether the proposed tasks are wanted. |
| `driver_not_parked` | mechanical | none: wait for the park |
| Dead driver, API error | mechanical | none |
| Permission dialog | mechanical for a known prompt | An unknown prompt needs a reader. |
| Hung command or gate | judgment | Hung or only slow, and what to kill. |

## 3. Proposed rule table

Three outcomes: lym settles, the orch settles, or a severe halt that waits for the operator and reaches them on Slack.
The orch column is split in two:

- **Today** is autonomy as the orch's role text defines it.
- **Overnight** is a stricter level in which the orch never asks the operator.

"—" means the class does not reach that column. A bound is per run and row.

| Class | lym settles | Orch, today | Orch, overnight | Severe |
|---|---|---|---|---|
| Board claim push refused | sync and re-step, bounded | after the bound | same | third failure |
| Network, GitHub API, code tip not on origin, push lock busy | wait and re-step; no notice below the bound | after the bound | keep waiting, report once | credentials refused |
| Pause covers the run | hold until lifted | — | — | — |
| Launch failed, session timeout, fixer fork, strand lost | one fresh start | on a repeat | on a repeat | third halt at the row |
| Review file not valid YAML | re-prompt the writer | on a repeat | on a repeat | third halt at the row |
| Stale build | hand over (as today) | when no newer build exists | deploy or wait | a deploy it cannot run |
| Escalation `hard-cap` | accept, file each open finding as a note | — | — | — |
| Escalation `budget`, only deferred findings | accept, file a note | — | — | — |
| Escalation `budget`, open blocking findings | — | accept or continue | accept with a note, or one continue | — |
| Escalation `decision`, `circling` | arbiter (as today) | what it hands on | what it hands on; pick the narrower reading and note it | third halt at the row |
| Bounce budget exhausted | one resume | on the second | on the second | on the third |
| Gate attempts exhausted | one fresh spawn | on the second | on the second | third halt at the row |
| Webster failure or audit finding, defect or config at a row | — | settle | settle, and file the bug | third halt at the row |
| Landing stop, deploy failure | one deploy retry on a newer tip | settle | settle | a GitHub or machine step |
| `driver_not_parked` | the verb waits | — | — | — |
| Dead driver, API error | respawn or revive, bounded | after the bound | after the bound | machine cannot host the run |
| Permission dialog | answer a listed prompt | read and answer | read and deny when unsure | — |
| Hung command or gate | end it past its time limit | judge | judge | — |
| Agent question: run a named measurement | run it and post the result | when not runnable | same | — |
| Agent question: waiver, overtaken entry, design call | — | answer | answer; never relay | a cancel or a scope change it cannot note |
| Entry overtaken, cancel proposed | — | relay (severe today) | pause the run, note it, go on with other work | the cancel itself |
| Scope change | — | relay (severe today) | file a note and finish within the record | the run cannot finish within the record |
| Raddle approval | — | operator | operator | yes: it creates tasks |
| `PR-Review` waiting on the operator | — | operator | operator | yes |
| Owner halt, takeover | — | operator | operator | yes |
| Step only the operator can take | — | operator | operator | yes |
| No settling action open | — | operator | operator | yes |

Three points the design must still decide:

- Today's text makes a repeating stop severe at the third halt at a row.
  lym's own retries must count toward that, or a retry loop hides a repeating stop.
- A raddle approval and a pull request review are waits by design, not faults.
  Whether the overnight level sends them as imperative Slack messages or as quiet ones is open.
- "Accept with a note" for open blocking findings lands known defects unattended.
  The narrower alternative holds the run and lets other work continue.

## 4. Where each lym-settled class would be handled

| Class | Package | Today |
|---|---|---|
| Board claim push refused | `internal/sheddriver` rule; refusal in `internal/boardengine`, claim in `internal/battencli` | tackler agent resumes |
| Network, code tip | `internal/sheddriver` (`driverules.go`), `internal/battencli` (`codetipsettle.go`), `internal/runpusher` | network waits exist |
| Push lock busy at start | `internal/dobbyengine` catch-up | refused start with a notice and backoff |
| Pause covers the run | `internal/sheddriver` | tackler agent resumes |
| Launch failed, session timeout, fixer fork, strand lost | `internal/sheddriver`; causes in `internal/seatengine` and `internal/shuttleengine` | tackler, hand-over or orch |
| Review file not valid YAML | `internal/burlerengine` gate | the round fails |
| Stale build | `internal/sheddriver` | hand-over exists |
| Escalation accepts for `hard-cap` and deferred-only `budget` | `internal/shedadapters` (the bouncer's settle) | arbiter or orch |
| Escalation `budget` admission, if the arbiter is to take it | `internal/arbiterstop`, `internal/shedarbiter` | refused by rule |
| Bounce budget, gate attempts | `internal/sheddriver` | handed back to the orch |
| Deploy retry | `internal/battenshed` (`Land`) | tackler |
| `driver_not_parked` | `internal/loomcli` (resume) | refused |
| Dead driver | `internal/dobbyengine` (run notices), `internal/battenshed` (revive) | notice; revive only for a halted child |
| API error | `internal/dobbyengine`, `internal/tendercli` | revive exists |
| Permission dialog | `internal/shuttleengine/claudeengine` | notice to the parent |
| Hung command or gate | `internal/dobbyengine` (gate-hold notice), `internal/gatecli` | notice only |
| Named measurement for an agent | `internal/gatecli`, asked through `internal/creelengine` | orch runs it |

The batten run is scheduled by dobby and stepped by the driver in-process,
so a rule in `internal/sheddriver` covers both layers.
Rules there would replace the tackler for the classes it almost always resumed:
of 65 tackler sessions it resumed 57, escalated 7 and handed back 1.

## 5. Notices that fired for a correct wait

The quiet notice (`[dobby notice] ... agents idle for 30m`, `internal/dobbyengine/runnotice.go`) already skips
a live gate or verify run, a parent-lock wait, background shells, and the rows in `loomshed.OperatorWaitRows`
(`readAgentActivity` in `internal/battencli`).
Some of those skips landed during the window, so a case below may no longer fire.

| Waiting on | Cases | Examples | The notice should name |
|---|---|---|---|
| Operator approval at `Raddle-Write` | 6 | `codex-same-as-claude` (11-16), `kde-window-placement` (11-27) | "waiting for the operator's approval of `<proposal path>`, posted `<time>`". `Raddle-Write` is not in `OperatorWaitRows`; key on a proposal that passes its checks, so a dead raddle agent still shows. |
| A creel reply or an operator step | 6 | `kde-window-placement-build` (11-27, four notices), `harden-fabric-undriven-cases` (11-23) | "waiting on the reply to creel #`<id>` since `<time>`: `<first line>`", or the operator step. The held turn end is deliberately not counted as a wait; `creelengine.Store.AwaitsReply` answers the question. |
| A long build or gate the agent started | about 5 | `redundancy-confirmed-survey` (11-16, 11-17), `test-time-measure` (11-27) | "waiting on `<command or gate run>`, running `<time>`, last output `<time>`" |
| `Finalize` with no agent | 2 | `gatemap-passedalone-flake`, `restep-prior-work-none` (11-27) | "Finalize is running `<step>`" with the lock holder or GitHub check. Both moved on by themselves; what they waited on is inferred. |
| Nothing: the agent was working | 1 | `per-task-settings-one-lookup` (11-22) | no notice; it was reading code |

Idle notices that were right:

- a stalled fixer (`godocreflow-worsens-findings`);
- a hung shell (`memory-cap-not-enforced`);
- a helper the operator had stopped (`claude-code-mode`);
- an approval given but never relayed (`board-viewer-editor`).

Other notices that added no information:

- "ended a turn without its output files and is held" repeats the agent's own question, sometimes after the answer.
- The creel chase "read and has not been answered" fired four times on 11-21 for matters already settled or moving.
- "the verify gate has run past its budget" reports a running gate, not a stuck one;
  it should name the failing runs, if any.
- The repeated-edit "loop" notice was noise until it was turned off on 11-17.
