# reed: strand-based mailbox/addressing system

## The idea

Deliver messages/events to any Strand (agent, operator, `ly-drive` watchdog, orchestrator) by address.

## The send/receive asymmetry

Being a Strand is only required to *receive* mail — an address needs somewhere durable to deliver to. It is **not** required to *send*: anything able to reach the delivery mechanism can send without needing an address of its own. An in-process Claude Code fork subagent is the clearest example — it can never itself become a Strand (it is never an OS/tmux process), but it can still send.

## Addressing (settled in a 2026-09-20 design discussion)

The address format is **`name@slug[.hub]`** — the email shape, deliberately: "deliver to *name* at *place*" needs no explanation for humans or agents, and the grammar is collision-free because slugs (`[a-z][a-z0-9-]*`) can contain neither `@` nor `.`, so the single `@` splits unambiguously and the optional `.hub` suffix reads exactly like an unqualified-vs-qualified hostname (omitted = own hub).
The resemblance to real email is a known confusion risk, accepted on purpose — it buys the mental model.
Real external channels never ride on it: a raw email address is never a valid recipient, and creel never guesses whether something "is" email.
Instead — **later, not creel v1** — external channels are **connector domains**: the rule is that the rightmost element decides the router — a known hub (or nothing) means internal post, while a reserved connector domain (`slack`, `email`, …) hands *everything left of it* to that connector module to interpret (`user@slack`; possibly `orch@slug.hub.slack` if each slug gets its own Slack channel — the connector owns that mapping, creel's core never parses it).
Each connector module owns its own list of valid left-side names, and that configured list *is* the allowlist: a confused agent structurally cannot reach an arbitrary third party.
Inbound goes the same way: a reply arriving from Slack enters creel as ordinary mail with the connector address as `from`, so the receiving agent just replies to `from` and never knows Slack exists.
Connector domains are excluded from the hub-name grammar, so no hub can ever shadow one.
Resolution stays heuristic-free everywhere: a recipient resolves to a strand, or ends in a connector domain, or is refused with the list of names that exist.
Creel v1 reserves the grammar only; no connector ships with it.
(The `@` file-mention in Claude Code's interactive input is a UI autocomplete trigger only — an address in a shell argument is just text — so the overlap costs a cosmetic popup when a human types an address in the prompt box, nothing more.)

- **The strand guid is the identity; names are the address.**
  Every strand has a guid for its whole life, pane or no pane; creel resolves an address at *delivery* time (slug → session, name → strand → guid) and delivers to the guid.
  Names are what humans and agents write, because names are role-stable while guids are instance-bound:
  a torn-down-and-respawned orchestrator has a new guid but the same name, and mail to `orchestrator@task-a` should reach whoever holds the role now.
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
  A small canonical vocabulary (`orchestrator`, `driver`, `operator`, …) lets an agent address `driver@<slug>` with no directory lookup at all; a listing verb covers the rest.
- **Multi-hub is out of scope, but the grammar reserves the door.**
  Operating across several repos at once would not need one reed server spanning hubs: each hub keeps its own server and per-hub daemon, and cross-hub mail becomes daemon-to-daemon relay — the optional `hub:` prefix (omitted = own hub) is the entire forward-compatibility cost, paid here in grammar only.

## Dependencies

Deliberately a separate, later task from `ly-drive + orchestrator`'s launch-convention change and the shipped `reed: born-as-strand` item, which it depends on **for the receive side only**: everything that should be addressable must already exist as a Strand before addressing it means anything. Also downstream of `worktree spawn/teardown as Shed producers`' headless-orchestration future, since a mailbox is most useful once external actors can reach a worktree without a human ever having opened it first.

## Delivery (settled in the same 2026-09-20 discussion)

Transport and log are two different jobs, and the terminal is bad at the second one (scrollback dies with the pane, is unsearchable, and mixes mail into everything else) — so delivery is **pull, not full-content injection**, with a durable store carrying the audit trail.

- **Every message is an envelope in the creel store**: `from`, `to`, timestamp, `kind`, status (`sent` → `delivered` → `read`).
  `from` is filled in by the send verb itself, never free-typed: derived from `$LYX_STRAND_NAME` + the cwd slug when it can be (env is inherited down the whole process tree, so Go code running inside a strand signs with the strand's address for free), falling back to the `$TMUX_PANE`-to-state join.
  Outside reed there is no address, and creel does not pretend otherwise: `from` becomes a reserved not-addressable marker (`operator`), meaning a reply is impossible and the message must stand on its own — no reply-routing is configured for the marker.
  So a receiver always knows exactly where the reply goes (or that it can't), and a sender can neither forge nor forget its identity.
  Replying is just sending to `from`.
- **Two kinds.**
  `mail` is stored and *pulled*: the receiver is notified with one short line and reads at its own pace (`lyx creel inbox` for the count, a get-one verb that flips status to `read`).
  Pulled content arrives as tool output the agent asked for — better context than a giant injected user-message — and a short fixed notification line can never break on escaping the way multi-line `send-keys` can.
  `inject` is typed verbatim into the receiving pane instead: its body is a command to *execute*, not content to read, so it never sits in an inbox — the first use case is `lyx reed self-compact`, sugar for sending `/compact` to one's own address.
  Self-injection is free; injecting into *another* strand's pane is the driver privilege, restricted to privileged senders, never something any strand can do to any other.
- **The daemon delivers, at idle.**
  Both kinds ride the per-hub daemon's existing per-tick loop (resolving the old open item — no new process): the notification line or injected body is sent-keys into the receiving pane only when the receiver is idle, not mid-turn.
- **Everything is journaled, regardless of kind.**
  An `inject` is not mail but is still logged with sender, receiver, body, and delivery time; `lyx creel log` shows all traffic hub-wide — who sent, who read, when — searchable and durable, which is strictly better backtracking than terminal scrollback ever was.
  The live-watching human still sees traffic as it happens: the notification line carries sender + subject (`2 new: watchdog@prime "loom stuck at Gate"`), bodies stay in the store.
- **Plain CLI verbs, no MCP layer.**
  Creel needs no streaming or client-side session state, so it stays ordinary `lyx` verbs like every other module; an MCP server could be layered *on top of* the same verbs later if an agent outside a terminal ever needs access — deferred until someone does, not decided against.

## Related

- the shipped `reed: born-as-strand` item — the receive-side prerequisite.
- `internal/reedengine`'s package documentation — the shipped watchdog daemon this may end up riding on.
