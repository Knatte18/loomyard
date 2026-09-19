# reed: strand-based mailbox/addressing system

## The idea

Deliver messages/events to any Strand (agent, operator, `ly-drive` watchdog, orchestrator) by address.

## The send/receive asymmetry

Being a Strand is only required to *receive* mail — an address needs somewhere durable to deliver to. It is **not** required to *send*: anything able to reach the delivery mechanism can send without needing an address of its own. An in-process Claude Code fork subagent is the clearest example — it can never itself become a Strand (it is never an OS/tmux process), but it can still send.

## Dependencies

Deliberately a separate, later task from `ly-drive + orchestrator`'s launch-convention change and [`reed: born-as-strand`](reed-born-as-strand.md), which it depends on **for the receive side only**: everything that should be addressable must already exist as a Strand before addressing it means anything. Also downstream of `worktree spawn/teardown as Shed producers`' headless-orchestration future, since a mailbox is most useful once external actors can reach a worktree without a human ever having opened it first.

## Open items

- The actual delivery mechanism is undecided: written into the pane directly? A separate side-channel?
- Whether it rides the already-Done `reed: watchdog daemon`'s per-tick loop, or needs a new process, is undecided.

## Related

- [reed-born-as-strand.md](reed-born-as-strand.md) — the receive-side prerequisite.
- [reed-header-selvage.md](reed-header-selvage.md) — the watchdog daemon this may end up riding on.
