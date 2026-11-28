// Package boardengine provides a one-shot, daemonless file-locked task tracker.
// Board is the only entry point callers use.
// Anyone adds notes, the orchestrator curates.
//
// # Entries
//
// Board holds one store, board.json, and every entry in it is a task, a note or a project, named by its kind.
// Only a task is claimable, bar a project that is a beam, and a task or a note carries depends_on.
// A note is an idea or observation that is not yet work to run; Promote turns it into a task once the operator decides.
// A note's depends_on says what it waits for: the board shows it as a task's, and the promoted task keeps it.
// It never makes a sibling, since a note never runs.
// Promote refuses a project, which is made a beam with `lym beam create`.
//
// A project groups entries.
// It carries a type label, and no depends_on, profile, run_settings or priority; its only recipe is `pawl`.
// A write giving it any of those fields, or another recipe, is refused naming the entry and the field to drop.
// A task or a note may depend on any other entry: a task, a note or a project.
// Dependencies judges a dependency met once that entry is done:
// a task or a beam when its status is done, and a plain project when its status is done or every landing member is done.
// A dependency on a note is met only once the note is promoted and lands, or is removed or pruned, whatever its status.
// A landing member is a task or project under the project; a note never lands and holds no project open.
// Removing an entry, like pruning a done one, strips its slug from every other entry's depends_on.
//
// Membership is the project key on a task or note and the order key on a project.
// A write that sets or changes one is judged, and each refusal names the entry, the field and the slug at fault.
// An entry whose project the write leaves unchanged is never refused, so marking a project done refuses nothing.
// A change of project on an entry whose pair exists is refused naming `lym batten cancel <slug> --remove`.
// Config.PairExists reports whether a pair exists, and nil reads as absent for every entry.
// Prune keeps a done project while an entry the same prune does not remove names it in project.
//
// The beam flag marks a project as a beam, and only the beam verbs write it.
// A payload may carry beam only as the entry's stored value, which is dropped as a no-op; any other value is refused.
// The replay does not apply that rule or the owner rule, so a hand edit of board.json can still set the flag.
//
// The front and on_land fields mark a front task, which Board.FileFrontTask alone writes.
// A payload may carry either only as the entry's stored value; any other value is refused.
// A front task's project and kind are fixed, and a run-held entry takes no change to either field.
// SetOwners writes the beam flag through OwnerRequest.Beam, in the one commit that records the owner.
// Board.EndBeam clears it for a beam that ends with no landing, leaving a plain project with its members.
//
// # Labels
//
// Each entry carries labels, validated against the types and labels maps in board.yaml.
// A note carries exactly one type label, and a task carries one or more.
// The sketch type label (SketchType) is built in, since lym itself writes it.
// A write that carries a label in neither map is refused, naming the entry, the label and board.yaml.
// A Board built from a bare path has no outputs configured, cannot write, and validates no label.
//
// # Handles
//
// Every entry carries a handle, a letter and a number such as T4, B2 or E7, stored in board.json as the handle key.
// The letter is T for a task, P for a project, and for a note the upper-cased first letter of its type label.
// LoadConfig refuses type labels that do not begin with a letter, whose letter is T or P, or that share a first letter.
// The board alone assigns handles, and an upsert payload carrying one is refused.
// An entry keeps its stored handle while it lives, and a removed entry's number is given out again.
// A promote or a type change moves the entry to the lowest free number of its new letter.
// A handle is not scope: the run lock and the revised date ignore it.
// Every Board and Store method that takes a selector takes a Handle beside a slug and a numeric id.
//
// # Priority, run settings and profile
//
// A note may carry a priority, high, normal or low; a task carries none.
// A write leaving a task with a priority is refused.
// Priority is part of an entry's scope, so the run lock refuses a change to it on a run-held note.
// An entry may carry run_settings, and a task may carry a profile.
// Neither is scope, so the run lock lets a run-held entry take a change to them.
// RewriteRunSettings rewrites every entry's run_settings in one write.
// It puts board.json back when a step after the save fails.
//
// # Owners
//
// Ownership sits on projects.
// The root project's owner is the board's prime_owner, and a beam project's owner is its owner_machine.
// A task, a grouping project and a note record no owner.
// Owners.Effective resolves an entry's effective owner: a beam project's recorded owner, else its parent's owner.
// An owner change is never a scope change, so the run lock lets a run-held entry take it.
// ParentBeam and Board.ParentBeamOf keep the empty slug for the prime, and every owner read and beam verb takes it so.
// The list view prints the prime as RootSubject, `root`, since an empty value reads as no beam.
//
// SetOwners is the only writer that records an owner or the prime owner.
// It pulls the board level with its upstream, writes one commit, and pushes it.
// Without Takeover, it leaves an entry whose effective owner is another machine untouched.
// The exception is an entry whose recorded owner Replace names, which it reports in OwnerWrite.Lost.
// Only `lym adopt` passes Takeover, and only `lym machine name` passes Replace.
// A competing write origin already holds wins the replay of a rejected push.
// An origin that does not answer is ErrOwnerOriginUnreachable,
// and a push a held lock keeps deferring is refused naming `lym board sync`.
// An upsert payload may carry owner_machine only as the entry's effective owner, which is dropped as a no-op;
// any other value is refused naming `lym adopt`.
// Board.EndBeam is the one other write of an owner: it clears the owner of a beam whose life ends with no landing.
// It takes the ordinary write and detached sync.
// That is enough because the cancel that ends such a beam has already refused one another machine owns.
//
// # Machine registry and holds
//
// board.json carries two more top-level records, `machines` and `holds`, which a load keeps and a save writes back.
// An unknown key inside an entry or a hold survives the rewrite.
// Each record is written only by its own setter, which pushes synchronously.
// Registry.ReadsAsHere is the one comparison of a recorded machine name with this machine.
// RegisterMachine pulls the board level with its upstream and runs Registry.Check before it writes and pushes one commit.
// Check refuses with a *RegistrationError naming the clashing machine and the way forward.
// EnsureIdentity is the one write of the machine id into the machine file, beside `lym machine name`.
//
// Only boardengine's hold setters write the holds.
// Each pulls the board level with its upstream, judges the holds that stand, and pushes its one commit before it returns.
// A hold of another machine stands, and WriteHold returns it in HoldWrite.Standing with nothing written.
// A refusal is a *HoldRefusal naming the other machine, and the way forward.
// A rejected push's replay refuses a commit whole over a hold another machine wrote.
//
// # Slug rule
//
// ValidateEntrySlug is the length limit and root reservation every new slug passes.
// Config.ValidateSlug is how the board applies the pair slug rule to the task and project slugs a write makes.
// A write makes a slug when it adds a task or project, promotes a note, or renames an entry.
// A write that leaves an entry's slug and kind as they were is never judged,
// so a slug the rule refuses keeps taking writes, and a note is never judged.
// A refusal names the entry, the rule's own reason and a slug the rule admits.
//
// # Dates
//
// An entry carries two dates: created, when it was added, and revised, when its scope last changed.
// Each is in whole UTC seconds, and the write critical section stamps them from Config.Clock.
// Scope is what the run lock guards, so a status change revises nothing.
// A promote keeps the note's dates, and a merge gives the upserted entry the oldest created date among its entries.
// No view stores an age; the README and `lym board list --text` compute each as they render, through FormatAge.
//
// # Unknown keys
//
// An entry's and the store's unknown keys are the board.json keys this binary does not declare.
// They are kept as raw JSON and written back after the known fields, in key order, on every write.
// This binary adds, changes or merges none, and a kept value is trusted exactly as far as an entry's body is.
//
// The one exception is a pair of retired entry keys.
// A load folds a task's `pull_request` and `notify_on_land` into the task's `beam` run settings.
// They become `beam.pull_request` and `beam.land_notice` there, and both keys are dropped.
// A `pull_request` of `off` carries no value.
// The old resolver still opened a pull request for it under a required or merge parent.
// A value the run settings already hold wins, and the next save writes the fold.
// A project has no run settings, so its two keys stay among its kept keys and every save writes them back.
// `BeamSettingsByEntry` reads each entry's beam settings from the file as it is:
// the run settings' `beam.pull_request` and `beam.land_notice`, and the retired keys while they remain.
// The hub reconcile uses it to carry them into the worktrees of runs already past their seeding, a project's included.
// It also reports a retired `pull_request` of `off`, which the reconcile flags with its way forward instead of carrying.
//
// # README
//
// The root, the entries whose project is empty, is a project like the others.
// It renders with the handle `P0`, and RootProjectBrief gives it as one listing row with the slug RootSubject.
// Every write renders README.md, one board file per project, board-data.js and board.html.
// Every project, open or done, renders to project-<slug>.md beside it, and the root to project-root.md.
// The split is rendering only: every entry stays in the one store, and handles stay numbered across the hub.
// A render rewrites each board file in place, so an open VS Code markdown preview shows the new content.
//
// RenderToDisk reads the beam modes through the BeamModeReader its Outputs carry.
// Config.Outputs takes that reader from InstallBeamModeReader;
// the lym root installs it, and a test binary installs none.
// With modes, the README's header states the hub-wide default of autonomy and of the pull-request setting.
// The section and file header of the root and of each open beam carry a mode line.
// It always shows autonomy and the pull-request setting.
// A beam's line then names its parent branch and links its recorded pull request.
// The link is the one the landing record names, so GitHub may already have closed or merged that pull request.
// A setting is bold when the project sets it itself.
// Otherwise it is italic with the place it inherits from, the hub-wide default included.
// A beam the reader leaves out of its answer carries a mode line saying its settings were not read.
// A beam whose worktree is not on this machine is one such beam.
// A failed read renders the board with no mode line and logs a warning.
// board-data.js is the board's view projection as `window.BOARD = <JSON>;`.
// Each entry in it carries a revision, the SHA-256 of its stored encoding, which Board.View and the render both take.
// BoardRevision is the SHA-256 of board.json, and "" while the file is absent.
//
// ComputeLayers names each task's section, and the same name is the layer field of `lym board list`:
// Running, Ready, the dependency layers A, B and on, Independent, or Draft.
// A task a run holds, by RunHolds, goes under Running and in no layer.
// An open draft task a run does not hold goes under Draft, and nothing starts it until it is released.
// Ready holds the open, released tasks with no open dependency.
// A dependency on a running task counts, so a task waiting on a run is not Ready.
// A dependency on a project that is not done weighs as one on its deepest open landing member.
// An open project is in none of the task layers; ComputeLayers puts it in its own Project layer.
// RenderOrder is the order of every section, and `lym board list` and `find` follow it.
// A pipe in a cell is escaped and a line break becomes a space, so an entry is always one row.
//
// # Intake
//
// Intake reads the hub's own GitHub repository.
// An issue reaches the board as an imported note or folded into an existing entry, and is then closed.
// An entry records the issues it came from in its issues field, and an issue already recorded is never imported twice.
// boardengine imports nothing GitHub-specific.
// The caller converts a fetched issue into InboxIssue and makes every network call outside the board lock.
// ImportResult reports an issue with a blank body and an issue naming paths under a worktree's `.lym` directory,
// so the caller can flag them.
//
// # Run lock
//
// A run holds a board entry while the entry's status has the run-status form "<state> · <producer>".
// RunStatus composes that form and IsRunStatus recognizes it.
// Before the save, the write critical section refuses any write whose net change on a held entry removes it.
// The same refusal applies to a net change that touches a field other than status.
// The net change runs from the entries as loaded under the board file lock to the entries after the whole mutation,
// so a batch or merge that clears the status early and then edits the entry is still refused, and nothing reaches disk.
// A status-only net change passes.
// The discussed flag is no scope either, and only StoreDecisionRecord writes it.
// MakeSketch's change, ending a Discussion-only run, passes.
// So does a family's project conversion under a Raddle-File run.
// Prune passes, since a held entry's dependency on a done entry that the same write removes is not a change.
// The refusal is a *RunLockedError that matches ErrRunLocked through errors.Is.
// An abandoned run is ended with `lym batten cancel <slug>` from the prime.
//
// # The landing hold
//
// Whether an entry is held and whether its run is in flight are two questions with two readers.
// `RunHolds` answers the first, and `IsRunStatus` the second.
// They differ for a beam project whose pawl run stands at a row before Finalize.
// That project is in flight and holds nothing.
// `boardengine` imports no recipe package, so the recipe's own test pins the rows.
// Such a beam project refuses only its removal and a change of `project`, `kind`, `recipe` or `beam`.
// The refusal is a *BeamRunLockedError that matches ErrRunLocked.
// `ProjectRecordHeld` is true once the pawl run stands past Preflight, and the body then takes no edit.
// The refusal is a *ProjectRecordLockedError that matches ErrRunLocked and names `lym loom decision add <slug>`.
// From Finalize on the full run lock applies.
//
// # Draft
//
// Every new task is `draft` (StatusDraft), by one rule at the write critical section:
// after the mutation, each entry of kind task whose status is empty (IsEmptyStatus) becomes `draft`.
// An entry that was a task before the write keeps an empty status the write left untouched.
// A task is released by a null status, through `lym board set-status`, an upsert or a merge's `set_status`.
// Board.SetStatusReleasingProject, which `lym board set-status` alone calls, also releases a project's family:
// each direct draft member task, validated as its own release, and one failing refuses the whole write.
// No other write releases a member.
// A write may release the tasks it makes: the draft rule skips the slugs it is told.
// The owner write and the replay of a local commit run no draft rule.
//
// # Run agents file notes, never tasks
//
// Only the operator decides what becomes a task, so a run's agent may file a follow-up as a note and never as a task.
// Config.StrandName names the writing session.
// A name carrying a task slug under any role but the orch's is a run agent's; every other name is unrestricted.
// The write critical section refuses a run agent's net change that leaves a task whose slug held no task before.
// The refusal names the session, the entry and the way forward, a note with "kind":"note".
// The guard trusts the name its caller passes, so it stops a mistaken write, not a determined one.
// The replay of a local commit never runs it.
//
// # Rename
//
// Rename changes one entry's slug in one write, and the entry keeps its id, its handle and every other field.
// Every other entry's depends_on, project and order name the new slug in place of the old one.
// The rename revises no entry, and a held task that depends on the renamed entry takes the rewrite.
// The renamed entry itself must not be held by a run, and a beam, which keeps its slug while it lives, is refused.
// An entry whose pair exists on this machine is refused naming `lym batten cancel <slug> --remove`.
// Body and brief text is never rewritten; the result names every entry whose body or brief still holds the old slug.
// The entry's decision record file moves with it.
// A failed move is returned as a *DecisionRecordError after the entry is saved.
//
// # Decision record
//
// An entry whose run's Discussion was approved carries the discussed flag.
// Its decision record is the file `decision-<slug>.md` beside board.json, at the Bolt root.
// The file follows its entry through a rename, and a remove, a prune or a merge that drops the slug removes it.
// The file is no render output, so RenderToDisk's manifest never holds it and a render never deletes it.
// RenderToDisk renders a flagged entry whose file is missing as unflagged, so no board file links a missing record.
// Only lym writes the flag and the file, and a payload may carry discussed only as the entry's stored value.
// Every write of the record file stamps `record_body_sha256` with the sha256 of the body the write leaves.
// A payload naming that key is refused.
// StoreDecisionRecord writes the file before it saves board.json,
// so a crash leaves a file with no flag and never a flag with no file.
// It bypasses the run lock for the flag, the stamp and the file alone.
// It refuses an absent entry and a done entry, and the done refusal matches ErrRecordEntryDone.
// Board.DecisionRecord reads without the lock and answers found only when the entry exists, is flagged and has the file.
// A run of a discussed entry reads its stored record and never its body,
// so Board.UpsertTaskReporting warns when it changes the body of an open entry whose record file exists.
// The write still lands, and the warning is silent when the new body is the one the stamp records.
// LoadConfig refuses a board.design_prefix that would render a design file onto a record file's name.
//
// Board.UpsertTaskWithRecord, behind `lym board upsert --record-file`, replaces an existing entry's stored record.
// It refuses a run agent's session, a new entry, an entry with no stored record, a done entry and an entry a run holds.
// It also refuses an entry whose task worktree stands in the hub or whose weft branch its caller's probe reports live.
// The guard reads no batten liveness, so a run started after its check, or on another machine, is not seen.
//
// # Sketch
//
// An entry whose Discussion-only run ended with its decision record approved and stored is a Sketch.
// A Sketch is a note of the built-in type `sketch`, handle letter S, which never runs.
// Board.MakeSketch makes an entry a Sketch in one write.
// It keeps its title, brief, body, project, issues, Created date and discussed flag.
// It drops its run and recipe fields.
// It writes nothing for an absent, done or already sketched entry.
// An entry that another depends on stays depended on: the dependent waits until the Sketch is promoted and lands.
//
// A Sketch becomes a task only as a raddle run's entry:
// Board.PromoteTo with recipe `raddle`, or with no recipe, makes the task in one write.
// The write keeps the decision record, the discussed flag and the sketch label, and releases the task.
// PromoteTo's depends_on is set in that write, so the dobby never sees the task released without it.
// PromoteTo warns, and still promotes, a Sketch whose body no longer matches its `record_body_sha256`.
// Two write rules close every other way.
// A write that makes a discussed note with a record file a task must carry:
// recipe `raddle`, the discussed flag and the sketch label.
// A write without them is refused with the plain `lym board promote` as the way forward.
// The sketch label is refused on a task whose recipe is not `raddle`, or on a project.
// That refusal applies only to a write that adds the label, makes the entry a task or project, or changes its recipe.
// The Bolt replayer admits an entry through the run lock and the kind and label checks only, never the promotion rule.
//
// A write that gives an entry the arras recipe is refused, naming motif as the way forward:
// motif and the Sketch's raddle run are the one design-first route.
// An entry that already holds the arras recipe keeps taking writes, so a run started under it goes on to its end.
//
// # Raddle store
//
// A raddle run that files a Sketch's entries keeps a store on the Bolt beside board.json.
// The store is the directory `raddle/<sketch slug>/` (RaddleStoreDir).
// `store.json` is written last as the filing's done marker.
// Prune never touches the store, so the store outlives the entries it filed.
// RaddleStore.Holds answers the entry of the Sketch slug, or of the Sketch whose `filed` holds the slug.
// The write critical section refuses a write that makes a slug the store holds, whether or not the pair slug seam is set.
// The exception is a made entry that carries a store entry's Sketch slug and that entry's recorded created stamp.
// A store file that cannot be read refuses only a write that makes a slug.
// The refusal names the file and `git -C <bolt> checkout -- <path>` as the way forward.
// The Bolt replayer never runs this rule.
// A board commit from a machine that had not yet pulled the store can land an entry under a held slug on origin;
// only a raddle entry is caught, by batten's start guard.
//
// A raddle run writes the board through these Board methods, each one write under the board lock.
// FileRaddle files a Sketch's approved entries.
// It writes nothing when its done marker is present.
// Otherwise it re-checks the entries and the store, wrapping a refusal in ErrRaddleRecheck.
// The parts are filed draft, so a release stays the operator's, and the run's own entry stays open.
// A family filing, whose project is the run's own entry, turns that entry into the project instead of adding one.
// Every filing moves each open entry's dependency on the run's entry onto the filed parts no other part depends on,
// and a refusal of that move is wrapped in ErrRaddleRepointRefused.
// EndRaddleEntry marks the run's entry done.
// ClearProjectRunStatus clears a run status from a family's project and releases no member.
//
// # Siblings
//
// A write that gives an open task a depends_on entry it did not have reports its siblings:
// the other open tasks that wait on that same entry while sharing a module label with the written task.
// A module label is a label the vocabulary does not report as a type label.
// SetDeps, UpsertTaskReporting, UpsertTasksBatch and MergeTasks return the siblings.
// The check never refuses, never changes a stored record and returns no siblings for a refused write.
//
// # Concurrency and sync
//
// Board sequences all mutating operations with a file lock:
// lock → load → mutate → save board.json → move or remove decision record files → render → write files.
// After each write but the run claim, the owner write and a landing's `done`, spawn.go starts the board's pusher.
// That process commits and pushes the changes to the remote, and the write returns without waiting for it.
// The three exceptions each pull the Bolt first, commit, wait for their push and spawn no sync.
// Read methods (Get/List) bypass the lock and load directly from disk, persisting nothing.
//
// Board.Expecting returns a copy of the board that expects one entry's revision, which Board.View gave a viewer.
// The write section checks it under the lock.
// A stale check refuses with a *StaleEntryError and writes, renders and syncs nothing.
// It refuses a revision whose entry was removed or changed, and an empty revision, a create, whose slug exists.
// The check covers only the entry it names.
//
// The push path talks to git through fabricengine.Bolt, never hand-rolled gitexec calls.
// The write starts `lym loom pusher --board`, whose Board.PusherSync commits and pushes once per interval in force.
// Its network gate is held under the hub scope, and under a pause it yields, leaving the change for the next start.
// `lym board sync` is the operator's verb for a synchronous backup.
//
// # Storage
//
// Board lives at weft:main, never a separate repo.
// fabricengine pairs every warp branch <branch> with a weft branch <branch>-weft, with no exceptions,
// so the unsuffixed name is never claimed by the pairing convention and is reserved for board.
// The long-lived "prime" worktree is the only worktree that checks out two weft branches at once:
// its own <name>-weft companion and weft:main.
// weft:main has no corresponding warp branch,
// so the Warp-SHA trailer and correspondence-index machinery does not apply to it.
//
// # Replay
//
// FabricReplay is the board's Bolt replayer, named `board`.
// It is also the run ledger through which fabricengine reads the ended set and the owners.
// It claims `board.json`, the render outputs and the `design-<slug>.md` and `decision-<slug>.md` files at the Bolt root.
// It also claims the files directly in a raddle store directory.
// The by-entry merge of fields is `internal/entrymerge`'s.
// Apply re-applies a local board commit onto the board checked out at origin's tip, one change at a time.
// A change passes the board's own checks, the run lock included.
// An added entry takes the next id of origin's board.json and a handle free there, never its local one.
// A decision record the commit writes for an entry origin does not hold is reported lost and not written.
// A path of any other shape is refused.
// The commit's `store.json` may record as filed an entry whose add the replay refused or lost.
// Then the store directory's files are set aside and reported refused.
// That keeps origin from holding the done marker without the entries it marks.
// Origin's side is kept and the local value is reported lost in two cases.
// One is both sides changing the same field or design file since the merge base.
// The other is origin removing the entry.
// A change a check refuses is reported refused, and so is a later change depending on an entry a refused change added.
// A commit whose holds differ from its parent's is refused whole when origin gained a hold of another machine.
// The replay takes neither the board lock nor the push lock, spawns no sync, and writes nothing when no change survives.
package boardengine
