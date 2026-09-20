# reed: strand-based mailbox/addressing system

## The idea

Deliver messages/events to any Strand (agent, operator, `ly-drive` watchdog, orchestrator) by address.

## The send/receive asymmetry

Being a Strand is only required to *receive* mail — an address needs somewhere durable to deliver to. It is **not** required to *send*: anything able to reach the delivery mechanism can send without needing an address of its own. An in-process Claude Code fork subagent is the clearest example — it can never itself become a Strand (it is never an OS/tmux process), but it can still send.

## Addressing (settled in a 2026-09-20 design discussion)

The address format is **`[hub:]slug:name`** — colon as hierarchy separator, safe because the slug grammar (`[a-z][a-z0-9-]*`) can never contain one.

- **The strand guid is the identity; names are the address.**
  Every strand has a guid for its whole life, pane or no pane; creel resolves an address at *delivery* time (slug → session, name → strand → guid) and delivers to the guid.
  Names are what humans and agents write, because names are role-stable while guids are instance-bound:
  a torn-down-and-respawned orchestrator has a new guid but the same name, and mail to `taskA:orchestrator` should reach whoever holds the role now.
  A reply can carry the guid when pinning one specific instance matters (detecting "the one I was talking to is gone").
- **Self-identification is injected, not looked up.**
  Reed owns every spawn/relaunch command line, so it injects `LYX_REED_ID` (the guid) and `LYX_STRAND_NAME` into every process it starts.
  The full address is *derived*, never injected: worktree slug comes from the cwd resolution the process already lives under, so `<slug>:<$LYX_STRAND_NAME>` composes at the point of use — a third stored copy could only drift.
  Fallback and verification: `$TMUX_PANE` (the `%`-id, server-unique, never renumbered) joins against the state's `PaneID` field; env and join disagreeing is an anomaly worth surfacing, not just a fallback order.
  Agents should treat the injected name as their own: ly-drive and sibling stencils read `$LYX_STRAND_NAME` at session start and use it in self-reference and as sender.
- **Names mirror to pane titles for humans, authority stays in state.**
  Reed sets the tmux pane title (`select-pane -T`) to the strand name at spawn/relaunch; tmux titles are display-only and enforce nothing, so the state's name field remains the single lookup key — the same volatile-view/durable-truth split `PaneID` already has.
  Reed owns the title channel alone: `allow-set-title off` on reed-managed panes, because any program in a pane can otherwise retitle it via the OSC 0/2 escape at any time (Claude Code does, continuously), and a cooperating-agent scheme is exactly the drift vector to close, not a feature.
  The per-hub daemon's tick adds the safety net: compare each tracked pane's actual title against the strand name, rewrite on divergence, and log that it happened — reconcile, same as the rest of reed's repair machinery.
  And the hard rule that makes display drift harmless rather than merely unlikely: **delivery never resolves through a pane title** — routing is always name → state → guid, so a stale title can mislead an eye but never a message; a send to a name the state doesn't know refuses with the list of names that exist (the same refuse-with-list idiom shed's run addressing uses), which itself surfaces the drift.
  A listing verb (`lyx reed list`: name, guid, session/worktree, pane-id, actual pane title, alive/dormant, drift flagged) doubles as the address directory and the diagnostic for exactly this concern.
  "Every pane has a name" is then nearly free: everything reed spawns is a strand (the shipped born-as-strand item makes even the operator's attach pane one), every strand has a name, every name mirrors to its title; panes outside reed are outside the system and never routed to.
- **Rename is a relaunch.**
  Env is frozen at process start, so the name is a birth attribute set at `AddStrand` and never mutated in place — renaming a role means tearing down and respawning its strand.
- **Well-known role names make addresses guessable.**
  A small canonical vocabulary (`orchestrator`, `driver`, `operator`, …) lets an agent address `<slug>:driver` with no directory lookup at all; a listing verb covers the rest.
- **Multi-hub is out of scope, but the grammar reserves the door.**
  Operating across several repos at once would not need one reed server spanning hubs: each hub keeps its own server and per-hub daemon, and cross-hub mail becomes daemon-to-daemon relay — the optional `hub:` prefix (omitted = own hub) is the entire forward-compatibility cost, paid here in grammar only.

## Dependencies

Deliberately a separate, later task from `ly-drive + orchestrator`'s launch-convention change and the shipped `reed: born-as-strand` item, which it depends on **for the receive side only**: everything that should be addressable must already exist as a Strand before addressing it means anything. Also downstream of `worktree spawn/teardown as Shed producers`' headless-orchestration future, since a mailbox is most useful once external actors can reach a worktree without a human ever having opened it first.

## Open items

- The actual delivery mechanism is undecided: written into the pane directly? A separate side-channel?
- Whether it rides the already-Done `reed: watchdog daemon`'s per-tick loop, or needs a new process, is undecided.

## Related

- the shipped `reed: born-as-strand` item — the receive-side prerequisite.
- [reed-header-selvage.md](reed-header-selvage.md) — the watchdog daemon this may end up riding on.
