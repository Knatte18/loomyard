// Package boardengine provides a one-shot, daemonless file-locked task tracker.
// Board is the only entry point callers use.
// Anyone adds notes, the orchestrator curates.
//
// # Entries
//
// Board holds one store, board.json, and every entry in it is a task or a note, named by its kind.
// Only a task is claimable, and only a task carries depends_on.
// A note is an idea or observation that is not yet work to run: one note per entry, and a relation between notes is a sentence in a body.
// Promote turns a note into a task once the operator decides to build it.
//
// # Labels
//
// Each entry carries labels, validated against two maps in board.yaml, each from a label to its one-line description.
// The types map holds the type labels: a note carries exactly one of them,
// and a task carries one or more.
// The labels map holds every other label, such as an area.
// LoadConfig reads both from the yaml node tree,
// so Config holds them as Label values in file order.
// A description may be empty or null,
// and the older shape, a list of names, still loads read-only with empty descriptions.
// ConfigOpenMaps names the two keys as open maps,
// so configengine carries a repository's own entries whole through reconcile and --set.
// Vocabulary answers both questions, and Board copies it from its Config and sets it on every store it builds.
// A write that carries a label in neither list is refused, naming the entry, the label and board.yaml.
// A Board built from a bare path has no outputs configured, cannot write, and validates no label.
//
// # Migration
//
// A board.json entry in the old shape, with a numeric tier and a type, converts in memory on load and persists in the new shape on the next write.
// Tier 1 becomes a task and every other tier a note, and the type becomes a label: bug stays bug, design becomes enhancement and undecided, and every other type and an absent one become enhancement.
// Each leading bracketed prefix of the brief becomes a label too, and is stripped from the brief unless it names a type.
// A note's former depends_on moves into the body as a closing line, and a task's dependency on an entry that became a note is dropped.
// A board directory that still holds the pre-upgrade tasks.json and notes.json migrates the same way and persists to board.json on the first write,
// and a pre-upgrade binary's later done marks are folded into the store, so a long-running old driver keeps working until the legacy files are retired.
//
// # README
//
// Every write renders README.md, with a Tasks section, a Notes section grouped by type label in the order of the types list with the remainder under Other, and a Done section.
// Tasks splits into Running, Ready, the dependency layers A, B and on, and Independent for an isolated task.
// A task a run holds, by IsRunStatus as the run lock decides it, goes under Running and in no layer; Running is omitted when no task runs.
// Ready holds the open tasks with no open dependency, and is always written, as `_None._` when empty.
// A dependency on a done task does not count, and one on a running task does, so a task waiting on a run is not Ready.
// Layer A holds the tasks that wait only on Running or Ready entries, Layer B those that wait on something in Layer A, and so on; ComputeLayers names each task's subsection, and the same name is the layer field of `lyx board list`.
// Each subsection is one markdown table numbered from 1: the bold title with the brief as a `• ` bullet under it in the same cell, the slug linked to its design doc when the entry has a body, and the labels that are not type labels.
// Running adds an At column, the run status without the state when the state is `running`, and Ready and the layers add an After column, the open entries named in depends_on.
// A pipe in a cell is escaped and a line break becomes a space, so an entry is always one row.
//
// # Intake
//
// The board is the one list, and GitHub is the inbox that selfreport files issues to.
// An issue reaches the board in three steps: list the open inbox issues no entry records, import one as a note or fold it into an existing entry, then close it.
// An entry records the issues it came from in its issues field, and an issue already recorded is never imported twice.
// A merge carries the removed entries' issues: the upserted entry's issues are its own followed by each removed entry's, in remove order, without duplicates.
// Import writes the board first and then comments with a pointer to the entry and closes the issue, and close alone ends a noise issue with a stated reason and touches no entry.
// boardengine imports nothing GitHub-specific: the caller converts a fetched issue into InboxIssue and makes every network call outside the board lock.
//
// # Run lock
//
// A run holds a board entry while the entry's status has the run-status form "<state> · <producer>", which RunStatus composes and IsRunStatus recognizes.
// Nil, empty, "done", "abandoned" and a hand-set word are not run statuses, and lock nothing.
// A held entry is the scope its run executes, so the write critical section refuses, before the save, any write whose net change on it removes it or touches a field other than status.
// The net change runs from the entries as loaded under the board file lock to the entries after the whole mutation,
// so a batch or merge that clears the status early and then edits the entry is still refused, and nothing reaches disk.
// A status-only net change passes: set-status, a merge's set_status and an upsert that differs only in status.
// set-status is itself unguarded, so a live run's status can be cleared by hand.
// Prune passes, since a held entry's dependency on a done entry that the same write removes is not a change; every other depends_on change is refused.
// The refusal is a *RunLockedError that matches ErrRunLocked through errors.Is.
// Its message names the entry, its status and both ways forward: a finding goes to a note of its own, and an abandoned run's entry is unlocked by clearing its status once `lyx batten status` shows no run holds it.
//
// # Concurrency and sync
//
// Board sequences all mutating operations with a file lock: lock → load → mutate → save board.json → render → write files.
// After each write, a detached background sync process (see sync.go) is launched to commit and push
// changes to the remote.
// The write returns immediately without waiting for the sync.
// Read methods (Get/List) bypass the lock and load directly from disk, persisting nothing.
//
// The detached sync path talks to git through fabricengine.Bolt, never hand-rolled gitexec calls,
// under board's own board.lock/board.push.lock write and push locks.
//
// # Storage
//
// Board lives at weft:main, never a separate repo.
// fabricengine enforces one uniform branch-naming scheme with no exceptions: a warp branch <branch>
// is always paired with weft branch <branch>-weft.
// That means no task's weft branch can ever be named exactly the warp's own default branch (every
// paired weft branch carries the -weft suffix) — which is what makes the unsuffixed name
// permanently unclaimed by the pairing convention and reserved exclusively for board.
// This repo's earlier design considered and rejected two alternatives before landing here: a
// separate third repo for board is extra git-identity overhead for something that doesn't need its
// own identity;
// and GitHub wiki rendering (an intermediate idea) requires whichever repo holds the wiki to be
// public on GitHub's free tier — in the old separate-repo model that meant board's own repo, never
// the warp/warp repo — disqualifying for private consulting work, where the warp repo's
// wiki-serving repo would have had to go public just to render board's front page.
//
// The long-lived "prime" worktree is the only worktree with a reason to check out two weft branches
// simultaneously: its own ordinary <name>-weft companion (the standard pairing rule, unchanged),
// plus weft:main for board access — never paired with any warp branch.
// No other worktree checks out weft:main directly.
//
// Consequence for fabric: weft:main has no corresponding warp branch, so the Warp-SHA trailer /
// correspondence-index machinery (fabricengine.RecordCorrespondence / WeftSHAForWarpSHA) does not
// apply to it — board's reads/writes to weft:main are a standalone concern, not routed through
// fabric.Commit.
//
// # loomyard's own area labels
//
// Before deploying the landed binary, write this list into the hub's live <hub>/_board/_lyx/config/board.yaml, then delete this paragraph.
// The migration turns every bracketed brief prefix on loomyard's live board that is not a type into a label, and the board refuses writes until board.yaml lists them.
// The PR description must quote this snippet verbatim, because the hub copies the list from the PR description, not from the diff.
//
//	labels: [undecided, agents, batcher, batten, board, bouncer, burler, cli, config, cost, discussion, docs, engines, fabric, finalize, github, glyph, ide, landing, loom, messaging, models, orch, parent-review, perf, permissions, plan, plugins, process, quarry, recipes, reed, review, search, shed, shuttle, slack, speed, testing, ui, vocabulary, webster, windows]
package boardengine
