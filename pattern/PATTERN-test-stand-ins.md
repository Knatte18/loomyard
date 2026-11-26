# PATTERN-test-stand-ins

A test never uses made-up data or behaviour that simulates another module of this repository.
Such a stand-in goes stale the moment the module it simulates changes,
and the test keeps passing against a module that no longer exists.

## Terms

- A **module** is a Go package of this repository.
- A **stand-in** is anything a test writes by hand in place of what production gets from somewhere else:
  a type implementing an interface, a function in a seam field, a record, a config file or an output text.
- The **owner** of a value is whoever produces it in production, or declares and alone writes its format.

## A repository module has no stand-in

- A test never hand-writes another module's state, records, config or output.
- A test that crosses into another module drives the real module.
  When it cannot, the test belongs in that module.
- An interface the module under test declares does not make its fake the module's own:
  when another module fills the seam in production, the fake simulates that module.
- A record of another module is written by that module's own writer,
  and a config file by `configengine.Set` over that module's template.
  The test edits no byte of either.
- An error of another module is provoked in the real module, through the state it reads.
  A test may remove another module's file or make it unreadable, since that writes no content.
  How a module refuses malformed content is tested where its parser lives.
- A file an agent writes and a module parses is that parser's format in the parser's own tests.
  Any other module's test takes it as a recording of the agent, under the rules below.
- A recording or golden of another module's output is a stand-in too, even one another test compares with the module.
- A kit under `internal/testkit/` may build fixtures through a module's real code;
  it holds no stand-in for a module.

What is not a stand-in:

- A module's own state, built by hand for its own rules: values of types it declares and files it alone writes or parses.
- A function seam that production fills from the module under test itself.
- A scripted implementation of an interface many modules fill, such as a shed producer or a gate closure,
  in a test of how the declaring module handles it.
  It scripts no particular module's behaviour.
- A value or file the code under test never interprets, such as a path it only passes on
  or an output file whose presence alone counts.
- A clock, a sleeper and a random or id source: they hand the module a value it reads as data.
- A wrapper that records or counts calls and passes each one to the real module.

## Something outside the repository

An LLM agent session, tmux, git, GitHub, Slack, the operating system
and a library of another repository are outside.
A test may stand in for one, under these rules.

- The stand-in replaces the outside thing alone, below the one module that reaches it in production.
  Every module of this repository above it is real.
- Each outside thing has one stand-in per boundary, shared through a kit under `internal/testkit/<kit>/`
  or kept in the one package that reaches the thing.
  No other package writes its own.
- What it answers is recorded from the real thing, never written by hand.
  A test picks, orders and cuts recordings, and may pick by what the code under test sent;
  it writes no new content into one.
  A failure it answers is a recorded failure.
- A recording names its source beside it: the run or command that produced it, the date and the version of the outside thing.
- The production code that reads the outside thing runs over every recording in a `tier0` test,
  so a reader that drifts from what was recorded fails every gate that tests it.
- A contract test drives the real thing and compares what lym reads from it with the recording:
  the fields, the event kinds and how a screen is classified, never free text.
  It lives in the tier that needs the real thing, as [PATTERN-test-speed](PATTERN-test-speed.md) names them.

## The LLM session

Only `tier3` runs a real LLM, and no gate runs `tier3`, so its stand-in takes these rules too.

- Each kind of recording has a `tier3` contract test that can record it again from a live session.
- A change to how an engine launches its provider, types into the pane, reads an event, a screen or a transcript,
  or a change that needs a new kind of answer from the stand-in,
  records the affected kinds again through those tests in the same task.
  Running `tier3` is the operator's call, so a run asks its parent for it.
- A fault a real session shows adds its recording, and the case that names what went wrong.
- Bound: a provider that changes by itself goes unseen until the next `tier3` run.

`internal/shuttleengine/claudeengine`'s turn-end corpus is the model for a recording and its source lines.

## Standing fakes

Fakes written before this rule remain until sweep tasks remove them.
[The inventory](../docs/research/no-fake-other-modules.md) lists them with a verdict for each.
No change adds a stand-in this rule forbids.

## Enforcement

Review discipline.
No test scans for a stand-in.
