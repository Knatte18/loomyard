// Package boardengine provides a one-shot, daemonless file-locked task tracker.
// Board is the only entry point callers use.
// Anyone adds notes, the orchestrator curates.
//
// # Entries
//
// Board holds one store, board.json, and every entry in it is a task, a note or a project, named by its kind.
// Only a task is claimable, bar a project that is a beam, and only a task carries depends_on.
// A note is an idea or observation that is not yet work to run.
// A note is one idea per entry, and a relation between notes is a sentence in a body.
// Promote turns a note into a task once the operator decides to build it,
// and it refuses a project, which is made a beam with `lym beam create`.
//
// A project groups entries.
// It carries a type label as a task does, and it carries no depends_on, profile, run_settings or priority.
// Its only recipe is `pawl`.
// A write giving it any of those fields, or another recipe, is refused naming the entry and the field to drop.
// Its project, order and beam keys are the membership and the beam flag.
// A project carries order and beam, and no task or note does.
// A binary that does not declare those keys keeps all three as unknown keys.
// A task may depend on any entry that lands, a project included.
// Dependencies judges the dependency met once that entry is done:
// a task or a beam when its status is done, which a beam's landing into its parent sets,
// and a plain project when its status is done or every landing member of it is done.
// A landing member is a task or project under the project; a note never lands and holds no project open.
// A dependency on a project waits on the project and its landing members not yet done.
// The write's cycle check follows a project to its members,
// so a member cannot wait on a task that waits on its own project.
// The layers and the siblings check read a project dependency the same way.
//
// Membership is the project key on a task or note and the order key on a project, both accepted by an upsert.
// A write that sets or changes one, an entry new to the board counting as set, is judged.
// It is refused when project names an absent entry, a non-project, the entry itself or a done project.
// It is refused too when project names a project a run holds or a chain of projects with a cycle.
// It is refused as well when order names an entry whose project is not this project or names one entry twice.
// A change of project on an entry whose pair exists is refused naming `lym batten cancel <slug> --remove`.
// Config.PairExists reports whether a pair exists, and nil reads as absent for every entry.
// Each refusal names the entry, the field and the slug at fault.
// An entry whose project the write leaves unchanged is never refused, so marking a project done refuses nothing.
// Prune drops the pruned slugs from every order,
// and it keeps a done project while an entry the same prune does not remove names it in project.
// MergeTasks gives an upserted entry with no project the first removed entry's.
// It rewrites every order naming a removed slug to name the upserted slug once in its place.
// It refuses to remove a project another surviving entry still names, naming those entries as the ones to move first.
// A promote keeps a note's project.
//
// The beam flag marks a project as a beam, and only the beam verbs write it.
// An upsert, batch or merge payload may carry beam only as the entry's stored value, false for a new entry.
// That value is dropped as a no-op.
// Any other value is refused.
// True names `lym beam create <slug>`, for a project, and for a task a project entry made under a slug of its own.
// A refusal of false on a beam says the flag stays set for the beam's life.
// The life ends when `lym beam land <slug>` lands the beam and marks the project done.
// The replay does not apply that rule or the owner rule, so a hand edit of board.json can still set the flag.
//
// The front and on_land fields mark a front task, a task lym files ahead of its worktree's other starts.
// Its on_land field holds the key of the action the front-task package runs when it lands.
// Board.FileFrontTask is their one writer, and the task it files is draft until a caller releases it.
// An upsert, batch or merge payload may carry either only as the entry's stored value, false and empty for a new entry.
// That value is dropped as a no-op, and any other value, or a value of the wrong type, is refused.
// A front task's project and kind are fixed too: a payload naming another is refused, and so is a rename of it.
// The way forward is to cancel its run and remove the entry.
// A run-held entry takes no change to either field.
// SetOwners writes the flag through OwnerRequest.Beam, in the one commit that records the owner.
// OwnerEntry carries IsProject and Beam for `lym adopt`'s beam admission.
//
// # Labels
//
// Each entry carries labels, validated against two maps in board.yaml, each from a label to its one-line description.
// The types map holds the type labels: a note carries exactly one of them, and a task carries one or more.
// The labels map holds every other label, such as an area.
// LoadConfig reads both from the yaml node tree, so Config holds them as Label values in file order.
// A description may be empty or null.
// A list of names in place of a map loads read-only with empty descriptions.
// ConfigOpenMaps names the two keys as open maps,
// so configengine carries a repository's own entries whole through reconcile and --set.
// The sketch type label (SketchType) is built in, since lym itself writes it.
// LoadConfig puts it ahead of board.yaml's types unless the file lists it.
// Vocabulary answers whether a label is a type label and whether it is configured.
// Board copies it from its Config and sets it on every store it builds.
// A write that carries a label in neither list is refused, naming the entry, the label and board.yaml.
// A Board built from a bare path has no outputs configured, cannot write, and validates no label.
//
// # Handles
//
// Every entry carries a handle, a letter and a number such as T4, B2 or E7, stored in board.json as the handle key.
// The operator names an entry by its handle without its slug.
// The letter is T for a task, P for a project, and for a note the upper-cased first letter of its type label.
// So the hub's board.yaml types decide the note letters, and a Sketch takes S.
// LoadConfig refuses type labels that do not begin with a letter or whose letter is T or P.
// It refuses type labels that share a first letter, the built-in sketch type label's included.
// The board alone assigns handles, and an upsert payload carrying one is refused.
// One assigner runs over the whole entry list on load, after each write's mutation and before the replay's save.
// An entry keeps a stored handle of its own letter that no entry of lower id holds.
// Every other entry, in id order, takes the lowest number of its letter not in use.
// So a handle stays while its entry lives, and a removed entry's number is given out again.
// A promote or a type change moves the entry to the lowest free number of its new letter.
// A board.json without handles gets them in id order on load, persisted by the next write.
// A note's letter cannot be decided when its type label is not configured or the board is path-only.
// Such a note keeps its stored handle unless an entry of lower id holds it, which clears it.
// A handle is not scope: the run lock and the revised date ignore it.
// `TitleWithHandle` opens the name of a run's `runs` window with its run's handle, `T4: <name>`.
// It reads the local board file.
// It leaves the name alone when the board names no handle for the slug or cannot be read.
// `IDEWindowTitle` and `TerminalWindowTitle` give a console window's title through `internal/consoletitle`.
// The title reads `<shortname>: <P#>: <T#>: <slug>`, from the hub shortname and the board at the call.
// <P#> is the handle of the nearest open beam above the entry, else of its project.
// The prime's and a beam's own windows carry a label in place of the slug.
// The label is `prime` or `beam` in VS Code, and `orch` in a terminal.
// `RecipeWindowTitle` gives the title of a run's status window.
// It is the worktree's console title with the recipe in place of the slug.
// Every Board and Store method that takes a selector takes a Handle beside a slug string and a numeric id.
// A handle's letter matches case-insensitively.
//
// # Priority
//
// A note may carry a priority, high, normal or low, and a note without one is normal; a task carries none.
// A normal priority is stored as no key, so board.json names a priority only on a high or low note;
// a write of "normal" clears the field, and a write of any other value is refused.
// A write that leaves a task with a priority is refused, and Promote clears the note's priority.
// A task stored with a priority loses it on load, so the next write drops it.
// A run-held task stored with one is still writable.
// The README shows a note's priority as H or L in the Prio column of the Notes tables, and no view orders by it.
// Priority is part of an entry's scope, so the run lock refuses a change to it on a run-held note.
//
// # Run settings
//
// An entry may carry run_settings: an object of module name to a mapping of that module's config keys.
// The mapping nests as in the module's YAML file.
// Batten's Seed-Child writes it into the pair's local config layer.
// A write of an empty object clears the field, which board.json then omits.
// A run_settings change is never a scope change, so the run lock lets a run-held entry take it.
// `lym board intake` never sets the field.
//
// A task may also carry a profile, the name of a run profile in the hub's `profiles.yaml`.
// `internal/runprofile` reads that file.
// Seed-Child writes the profile's block for the task's recipe under the task's run_settings.
// A write of an empty string clears it, and the task then takes its recipe's default profile.
// The README entry cell names only a profile the task carries.
// Like run_settings, a profile change is never a scope change.
//
// # Owners
//
// Ownership sits on projects.
// The root project's owner is the board's prime_owner, and a beam project's owner is its owner_machine.
// An owner_machine names the machine that may start, resume and land the runs under the beam.
// A task, a grouping project and a note record no owner, and board.json omits the key while none is recorded.
// A load drops an owner key on any entry but a beam project.
// The prime_owner key is a top-level record of board.json.
// The board replay merges it with the same origin-wins rule as a beam's owner.
// The README states it under its render time, and Board.PrimeOwner reads it for the list view.
// The effective owner of an entry is what Owners.Effective resolves:
// a beam project's recorded owner, else its parent's owner, and for every other entry its parent's owner.
// The parent resolves through Config.ParentOf, and the root's owner is the prime owner.
// A run status on an entry changes nothing about whose owner it reads.
// A board with no prime owner reads the root as owned by no other machine.
// A slug the board does not hold reads the same.
// Owners.AdoptSubject names what `lym adopt` takes for an entry:
// the beam project itself, else its nearest beam, else RootSubject.
// A project's recipe is ProjectRecipe, `pawl`, or none, which a beam project reads as `pawl`.
// No task carries it, and a write giving a project another recipe, or a task `pawl`, is refused naming the fix.
// Board copies Config.ParentOf onto every store it builds, so reads and writes resolve parents alike.
// A nil ParentOf reads every parent as the prime.
// ParentBeam is the board half of the parent resolver.
// It walks an entry's project links upward and returns the nearest open project that is a beam, "" meaning the prime.
// The list view carries it as parent_beam, taken from Config.ParentOf when that seam is set.
// Otherwise it comes from ParentBeam over the board's own entries, and it is empty when the seam fails.
// Board.ParentBeamOf answers it for one slug.
// Board.BeamUnder answers it for an entry not yet written, from the project link it will carry.
// ReadTasks reads the entries of a local board.json for a reader outside a Board.
// An owner change is never a scope change, so the run lock lets a run-held entry take it.
//
// SetOwners is the only writer of an owner or the prime owner.
// It pulls the board level with its upstream, writes one commit "board: owner <machine> for <slugs>", and pushes it.
// The commit sets each named beam project's owner, the prime owner when asked, and an optional claim status on its slug.
// A named task or grouping project gets no owner.
// A Beam request sets the beam flag and `recipe: pawl` on a named project in the same commit.
// With a BeamColor picker it also sets the beam's `beam_color` when it has none.
// The picker receives the colours the other open beams hold.
// `beam_color` is no upsert key, and only a beam project keeps one.
// A claim writes the run status alone, never an owner, and lands only on a task or on a project whose beam flag is set.
// Any other entry keeps its status.
// Without Takeover, SetOwners leaves an entry whose effective owner is another machine untouched.
// It reports that entry in OwnerWrite.Lost.
// Only `lym adopt` passes Takeover.
// A competing write origin already holds wins the replay of a rejected push,
// and the owners re-read from the local board.json after the push decide Recorded and Lost.
// An origin that does not answer is ErrOwnerOriginUnreachable,
// and a push a held lock keeps deferring is refused naming `lym board sync`.
// ReadOwners reads the owners the local board.json records.
// The list and get views carry each task's and project's effective owner as owner_machine.
// They never carry an owner recorded on a task.
// Board.EffectiveOwner resolves one slug's effective owner.
// A list whose parent resolver fails leaves the cell empty and still succeeds.
//
// An upsert payload may carry owner_machine only as the entry's effective owner, which is dropped as a no-op.
// Any other value is refused naming `lym adopt`.
// The rule runs from the store's upsert, batch and merge paths, never from the replay.
// The replay applies an owner change through its own entry writes.
//
// # Slug rule
//
// Config.ValidateSlug is how the board applies the pair slug rule to the task and project slugs a write makes.
// A nil ValidateSlug checks nothing.
// Board copies it, and the write critical section runs it after the mutation.
// A write makes a slug when it adds an entry of kind task or project, as an upsert, a batch or a merge does.
// It makes one when a promote makes a note a task, and when a rename gives an entry its new slug.
// A write that leaves an entry's slug and kind as they were is never judged.
// So a slug the rule refuses keeps taking writes, and a note is never judged.
// A task that becomes a project keeps a slug that already names a pair.
// The owner setter runs outside the critical section, makes no slug and checks none.
// A refusal names the entry, the rule's own reason and a slug the rule admits.
// The suggested slug is the slug without its records suffix, or the slug with -task appended.
// That suggestion goes through the seam and against the slugs of the entries the write leaves before it is offered.
// None is offered when it fails the rule or names an existing entry.
// An upsert under an existing slug edits that entry, so the refusal then asks for another slug the rule admits.
//
// # Dates
//
// An entry carries two dates: created, when it was added, and revised, when its scope last changed.
// Each is in whole UTC seconds, and board.json names neither while it is unknown.
// The write critical section stamps them after the mutation, against the entries as loaded.
// It uses the time Config.Clock gives, the wall clock by default:
// a new entry gets that time for whichever date it does not carry,
// and an entry whose scope changed gets it as revised unless the write set revised itself.
// Scope is what the run lock guards, so a status change revises nothing.
// Prune's dropping of a removed done dependency revises nothing either.
// A note becoming a task revises nothing, so a promote keeps the note's dates.
// An existing entry without a created date keeps none, since no write knows when it was added.
// An upsert may carry either date as an RFC 3339 time, which is how a backfill writes them.
// The run lock lets such a write through on a held entry, since the dates are not scope.
// A merge gives the upserted entry the oldest created date among it and the entries it removes.
// No view stores an age; the README and `lym board list --text` compute each as they render, through FormatAge.
// An age is one compact unit such as `3d` or `5w`.
//
// # Unknown keys
//
// An entry's and the store's unknown keys are the board.json keys this binary does not declare.
// They are kept as raw JSON and written back after the known fields, in key order, on every write.
// A binary built without this guarantee still drops them.
// Retiring a board field means naming it in the decode shape so it is consumed and dropped, as tier and type are.
// A field a binary stops declaring is kept forever.
// The only keys that pass through are those already in board.json.
// They come from another binary, a hand edit or a git merge of the records repository.
// This binary adds, changes or merges none.
// A kept value has no size or shape limit and is trusted exactly as far as an entry's body is.
//
// # Migration
//
// A board.json entry in the old shape, with a numeric tier and a type, converts in memory on load.
// It persists in the new shape on the next write.
// Tier 1 becomes a task and every other tier a note.
// The type becomes a label: bug stays bug, design becomes enhancement and undecided.
// Every other type and an absent one become enhancement.
// Each leading bracketed prefix of the brief becomes a label too, and is stripped from the brief unless it names a type.
// A note's former depends_on moves into the body as a closing line,
// and a task's dependency on an entry that became a note is dropped.
// A board directory that holds the legacy tasks.json and notes.json migrates the same way.
// It persists to board.json on the first write.
// A done mark that a binary using the legacy files writes later is folded into the store,
// so a long-running old driver keeps working until the legacy files are retired.
// A board without the top-level `draft_status` key is marked once.
// The next write sets `draft` on every open task whose status is empty.
// It writes the key `draft_status: true` in the same save, and a board holding the key is never migrated again.
// The store's on-disk version stays 1, and a binary that does not declare the key keeps it as an unknown one.
// A replayed commit that set the key sets it on origin's board.
// So a replay cannot leave origin to migrate released tasks again.
// The accepted bound is that a task an older binary creates after the migration has no status.
// It starts without a release until every machine runs the new binary.
// A task left in status `discussed` by a binary's Discussion-only run becomes a Sketch at the start of the next write.
// It keeps the status as a hand-set word when the write would refuse it as a Sketch.
// One such case is another entry depending on it.
//
// # README
//
// The root, the entries whose project is empty, is a project like the others.
// It renders with the handle `P0` and the title `Root: entries under no project`.
// Board project handles start at 1, so `P0` names no board entry.
// Every write renders README.md, the overview of every project, and one board file per project.
// Every project, open or done, renders to project-<slug>.md beside it, and the root to project-root.md.
// The split is rendering only: every entry stays in the one store, and handles stay numbered across the hub.
// So a file's handles have gaps, and layers, After cells and owners read the whole board.
// README.md opens with the UTC date and time it was rendered, so its ages read as of that moment, and the prime owner.
// A project file opens with its handle and title and the render time.
// It then has a link up to its parent's file, which is README.md for the root and for a project under no other.
// Its facts line and brief follow, then for a beam the word `beam`, and then the project's body when it has one.
// The intro and the entry sections come after the body.
// The body's ATX headings outside fenced code blocks shift down together until the shallowest is `##`,
// so the body adds no second `#` heading.
// Its design file and the facts line's link to it stay.
// A project's facts line is its parts joined by a middle dot.
// The first part is its slug as a code span.
// The slug links to its design file when the project has a body, as a table's Slug cell does.
// Then come `Age` and its created age when its created date is known.
// The last part is `Owner` and its effective owner, or none recorded.
// That owner is its recorded owner_machine for a beam, else its parent's.
// The parent resolves through ParentBeam over the rendered entries, with the prime owner as the root's.
// The root's facts line is `Owner` and the prime owner alone.
// The root has no board entry to give it a slug, an age or a design file.
// The Running table's Owner column shows a task's effective owner the same way.
// Every project, the root included, renders through one project section at a given heading level:
// its heading links its handle and title to its file and counts its open members, and its facts line and brief follow.
// Under the heading come a section per open project directly under it, one heading level down.
// Each holds its own sub-projects the same way, in the parent's order key and then by handle.
// The project's own entries follow.
// A project with no open sub-project has its own entries one level down.
// A project with an open sub-project has them under an `Own entries` heading after its sub-projects, at their level.
// A meaning line under it keeps its own entries from reading as the last sub-project's.
// The root's projects stand beside it rather than under it.
// Its section and its file hold its own entries alone, and its open members are its open tasks and notes.
// Heading levels nest with the project depth and stop at six, so a deep project still renders.
// An open project under a done project counts as under the root, since a done project has no section.
// README.md holds a section per open project under the root, by handle, and the root's section comes last.
// Each is a `<details open>` block the reader can collapse, open by default.
// Its `<summary>` stands in for a `##` heading and carries what that heading would, as an HTML link.
// Markdown does not render inside a summary, so the link is HTML.
// The section's contents keep their levels below `##`.
// So every open project shows there with its own Running, Ready, Tasks, Notes and Done.
// A project file's header stands as its project's heading.
// After it come the project's sub-projects at `##`.
// Its own entries follow at `##`, or under `## Own entries` at `###` when it has open sub-projects.
// A project's own entries are a Tasks section, a Notes section grouped by type label, and a Done section.
// The Notes section lists Sketches first, under the heading `Sketches`.
// Research comes next, under the heading `Research`.
// The other types follow in the order of the types list, and the remainder goes under Other.
// An open project is in none of the Tasks layers.
// ComputeLayers puts it in its own Project layer, a run-held one in Running and a done one in the done layer.
// RenderOrder places open projects by handle between open tasks and open notes.
// It places a done project between done tasks and done notes.
// No project is a row of Tasks or Notes.
// A done project is a row of its parent file's Done table, its title linked to its file.
// Every section heading but Own entries ends in the number of entries listed under it, in parentheses.
// Examples in a project file are `## Tasks (10)` and `### Ready (0)`.
// The number is never the highest handle number, since handles have gaps.
// Tasks splits into Running, Ready, and the dependency layers A, B and on.
// Independent holds an isolated task, and Draft an unreleased one.
// A task a run holds, by RunHolds as the run lock decides it, goes under Running and in no layer;
// Running is omitted when no task runs.
// An open draft task a run does not hold goes under Draft, whatever its dependencies or isolation.
// Nothing starts a draft task until it is released.
// Draft follows Independent and is omitted when no task is draft.
// A draft title cell opens with a `draft` code span before the bold title.
// Ready holds the open, released tasks with no open dependency, and is always written, as `_None._` when empty.
// A dependency on a done task does not count, and one on a running task does, so a task waiting on a run is not Ready.
// A dependency on a project that is not done counts too, and weighs as one on its deepest open landing member.
// So a task waiting on a project sits one layer below that member.
// It sits in Layer A when the project's open members are all running, ready or draft, or when it has none.
// Layer A holds the tasks that wait only on Running, Ready or Draft entries.
// Layer B holds those that wait on something in Layer A, and so on.
// ComputeLayers names each task's subsection, and the same name is the layer field of `lym board list`.
// Each subsection, Done included, is one markdown table in RenderOrder's order.
// A task subsection and a Notes type subsection are ordered by handle.
// Done lists its tasks before its notes, each by handle, so `lym board list` and `find` follow the same order.
// A row opens with the entry's handle.
// A Notes row adds the Prio column after it: H for a high priority, L for a low one and blank for normal.
// The title cell holds the bold title, with the brief as a `• ` bullet under it.
// It then holds one `• ` bullet per run setting, sorted, so the settings stack in height and add no column.
// A run setting is a code span such as `loom: code_max_bounces 1`, with a nested key as its dotted path.
// The slug cell links the slug to its design doc when the entry has a body.
// A `Sketch` link to its decision record file sits under the slug when the entry carries the discussed flag.
// The `Age<br>Revised` cell holds the entry's age above the time since its last revision, blank for an unknown date.
// The last cell holds the labels that are not type labels.
// The Running, Ready, layer and Draft tables add a Recipe column after the slug.
// It shows the task's recipe, or the board's `default_recipe` (`darn` in the template) when the recipe field is empty.
// The recipe is followed by ` · ` and the run profile when the task names one, such as `tapestry · light`.
// A task naming no profile takes its recipe's default profile and shows the recipe alone.
// The Notes and Done tables have no Recipe column.
// Running adds an At column, the run status without the state when the state is `running`.
// Running also adds an Owner column, the entry's effective owner machine, blank when none is recorded.
// Ready, the layers and Draft add an After column.
// It holds the handles of the entries named in depends_on that are not done, one per line.
// A project counts as not done the way Dependencies judges it.
// An entry with no handle or not on the board is named by its slug.
// A pipe in a cell is escaped and a line break becomes a space, so an entry is always one row.
//
// # Intake
//
// The board is the one list, and intake reads the hub's own GitHub repository.
// An issue reaches the board by listing the open issues of that repository no entry records.
// It is then imported as a note or folded into an existing entry, and then closed.
// An entry records the issues it came from in its issues field, and an issue already recorded is never imported twice.
// A merge carries the removed entries' issues.
// The upserted entry's issues are its own followed by each removed entry's, in remove order, without duplicates.
// Import writes the board first and then comments with a pointer to the entry and closes the issue.
// Close alone ends a noise issue with a stated reason and touches no entry.
// boardengine imports nothing GitHub-specific.
// The caller converts a fetched issue into InboxIssue and makes every network call outside the board lock.
// An imported or folded body carries the issue's comments after the issue body, the ones the caller kept, oldest first.
// Each comment is a heading naming its UTC creation date, one level below the body's own heading.
// The comment follows verbatim, its author not recorded.
//
// # Pull request
//
// A task or project may carry a `pull_request` override of "on", "off" or "merge-in".
// Any other non-empty value is refused, and empty means none and stores no key.
// "merge-in" marks a merge task, which lands with no pull request by a true merge.
// The field is scope for the run lock, so a held entry takes no change to it.
// A landing reads it against the parent's beam.yaml `pull_request` mode.
// The two decide whether the landing goes through a pull request.
//
// # Landing notice
//
// A task or project may carry `notify_on_land`.
// True asks the dobby that drives its run for a creel line once the run lands or ends without landing.
// The dobby's beam.yaml `land_notice` governs that line.
// False stores no key, and a note carrying it is refused.
// The field is not scope, so the run lock and the revised date ignore it and a run-held entry takes it.
// A done entry takes it too: its run already landed, and its parent's dobby sends the line at its next tick.
// Prune keeps a done entry while it carries the field,
// so the request outlives the landing until the dobby has sent its line.
// ClearNotifyOnLand clears it once the line went out, and writes nothing when the entry is gone or does not carry it.
//
// # Run lock
//
// A run holds a board entry while the entry's status has the run-status form "<state> · <producer>".
// RunStatus composes that form and IsRunStatus recognizes it.
// Nil, empty, "draft", "done", "abandoned" and a hand-set word are not run statuses, and lock nothing.
// A held entry is the scope its run executes.
// Before the save, the write critical section refuses any write whose net change on a held entry removes it.
// It refuses a write too when that net change touches a field other than status.
// The net change runs from the entries as loaded under the board file lock to the entries after the whole mutation,
// so a batch or merge that clears the status early and then edits the entry is still refused, and nothing reaches disk.
// A status-only net change passes: set-status, a merge's set_status and an upsert that differs only in status.
// The discussed flag is no scope either, so a held entry takes it, and only StoreDecisionRecord writes it.
// A held task whose net change is becoming a Sketch with no status and no other scope change passes.
// That change ends its Discussion-only run.
// MakeSketch writes it, and a hand write making the same change passes too, as set-status does.
// A write that also changes the entry's title, body or any other scope field is refused.
// So is one that makes it a note without the sketch label.
// A held task whose net change turns it into the project of a family the same write adds passes too.
// That holds while its status names the Raddle-File producer, whatever its state.
// The family's parts are the discussed draft tasks the write adds under the entry, each after the parts it depends on.
// The entry must become exactly their project.
// It has no status and the run fields cleared.
// Its order and part list name the parts in their sequence, and the write makes no other scope change.
// set-status is itself unguarded, so a live run's status can be cleared by hand.
// Prune passes, since a held entry's dependency on a done entry that the same write removes is not a change;
// every other depends_on change is refused.
// The refusal is a *RunLockedError that matches ErrRunLocked through errors.Is.
// Its message names the entry, its status and both ways forward.
// A finding goes to a note of its own, and an abandoned run is ended with `lym batten cancel <slug>` from the prime.
// That command clears the status.
//
// # The landing hold
//
// Whether an entry is held and whether its run is in flight are two questions with two readers.
// `RunHolds` answers the first and is true for every entry `IsRunStatus` accepts, with one exception.
// The exception is a beam project whose producer half is one of these rows:
// `Worktree-Create`, `Preflight`, `Record-Write`, `Beam-Up` and `Beam-Await`.
// A pawl run stands at those rows before it reaches Finalize, and a halt at any of them counts.
// `boardengine` imports no recipe package.
// The pawl recipe's own test pins that list against the claim row's and the recipe's row names.
// The readers that ask whether an entry is held use `RunHolds`.
// The readers that ask whether a run is in flight use `IsRunStatus`, which sees the pawl run at every row.
// A beam project whose run status stands but holds nothing keeps its scope fields editable.
// It refuses only its removal (a prune, a remove or a merge that leaves it absent).
// It also refuses a change of `project`, `kind`, `recipe` or `beam`.
// The refusal is a *BeamRunLockedError that matches ErrRunLocked and names `lym batten cancel <slug>`.
// `ProjectRecordHeld` is true once the pawl run stands past Preflight:
// the run has written, or is writing, the project record from the body.
// The body then takes no edit.
// The refusal is a *ProjectRecordLockedError that matches ErrRunLocked.
// It names `lym loom decision add <slug>`, the one way the project's scope changes after that.
// The pawl recipe's own test pins the record-free rows, the claim row and Preflight, against its row names.
// From Finalize on the full run lock applies.
//
// # Draft
//
// Every new task is `draft` (StatusDraft), by one rule at the write critical section.
// After the mutation, the rule gives `draft` to each entry of kind task whose status is empty.
// An empty status is nil or the empty string (IsEmptyStatus).
// These are `draft` even when the payload sets `status` to null:
// a create of any verb, a batch or merge create, a note or project upserted to a task, and a promoted note.
// An entry that was a task before the write keeps an empty status the write left untouched;
// so an edit of a released task leaves it released.
// A task is released by a null status.
// `lym board set-status '{"slug":"<slug>","status":null}'` writes it.
// So does an upsert setting `status` to null on a task, and so does a merge whose `set_status` sets null.
// Board.SetStatusReleasingProject, which `lym board set-status` alone calls, also releases a project's family.
// A null status on a project whose status is empty releases, in the same write, each direct member task in draft.
// A member already released, run-held or done, a nested project and its members, and every other entry stay as they are.
// Each member is validated as its own release, and one failing refuses the whole write.
// A null status on a project whose status is a run status or done is refused, naming each draft member's release.
// No other write releases a member: Board.SetStatus, an upsert, a batch and a merge release only the entry they name.
// An empty string written to a task whose loaded status differs is `draft`, not a release.
// A write may release the tasks it makes: the draft rule skips the slugs it is told.
// That is the promoted task of a promotion with recipe `raddle`.
// The owner write and the replay of a local commit run no draft rule:
// the first never makes a task, and the second copies entries a critical-section write already normalised.
//
// # Run agents file notes, never tasks
//
// Only the operator decides what becomes a task, so a run's agent may file a follow-up as a note and never as a task.
// Config.StrandName names the writing session.
// A name carrying a task slug under any role but the orch's is a run agent's;
// an empty name, the hub orch's slug-free name and an orch thread's name are unrestricted.
// The write critical section judges a run agent's write before the save.
// It refuses a net change that leaves a task whose slug held no task in the loaded entries:
// a new task, a promote, a note upserted to kind task and a merge upserting a new task.
// A rename is judged with the loaded slug renamed and creates none.
// The refusal names the session, the entry and the way forward, a note with "kind":"note".
// The guard trusts the name its caller passes, so it stops a mistaken write, not a determined one.
// The replay of a local commit never runs it.
//
// # Rename
//
// Rename changes one entry's slug in one write.
// The entry keeps its id, its handle, its position in board.json and every other field, kept keys included.
// Every other entry's depends_on, project and order name the new slug in place of the old one.
// A reference to the same entry under a new name is no scope change.
// So the write critical section judges the dates and the run lock against the loaded entries with the rename applied:
// the rename revises no entry.
// A held task that depends on the renamed entry, or a held project that orders it, takes the rewrite.
// The renamed entry itself must not be held by a run, and the refusal is the run lock's *RunLockedError.
// A beam is refused, since its slug stays fixed while it lives and its title is changed with `lym board upsert`.
// So is an entry whose pair exists on this machine, and that refusal names `lym batten cancel <slug> --remove`.
// A target that is empty, longer than MaxSlugLength or already an entry's slug is refused too.
// Body and brief text is never rewritten;
// the result names every entry whose body or brief still holds the old slug with no letter, digit, '-' or '_' beside it.
// The render after the write drops the old slug's design doc and writes the new one,
// and the entry's decision record file moves with it to the new slug's name.
// A failed move is returned after the entry is saved.
// The error is the rename's *DecisionRecordError, which names the file left at the old name.
// Board.Rename returns its result beside that error.
// A flagged entry with no file removes a file at the new slug's name, so it never reads an orphan as its own record.
//
// # Decision record
//
// An entry whose run's Discussion was approved carries the discussed flag.
// Its decision record is the file `decision-<slug>.md` beside board.json, at the Bolt root;
// the file follows its entry through a rename, and a remove, a prune or a merge that drops the slug removes it.
// The file is no render output, so RenderToDisk's manifest never holds it and a render never deletes it.
// A flagged entry's Slug cell in every board table links it as `Sketch`.
// So does a `Decision record:` line under the metadata line of the entry's design doc.
// RenderToDisk renders a flagged entry whose file is missing as unflagged, so no board file links a missing record.
// Only lym writes the flag and the file: Board.StoreDecisionRecord is the one writer.
// A payload may carry discussed only as the entry's stored value, false for a new entry, which is dropped as a no-op;
// any other value is refused.
// StoreDecisionRecord writes the file before it saves board.json.
// A crash therefore leaves a file with no flag and never a flag with no file.
// It writes nothing when the entry is flagged and the file already holds the bytes.
// It bypasses the run lock for the flag and the file alone, so every other field of a held entry stays locked.
// It refuses an absent entry and a done entry, since a landed entry keeps the record it last stored.
// The done refusal matches ErrRecordEntryDone.
// Board.DecisionRecord reads without the lock.
// It answers found only when the entry exists, carries the flag and the file exists;
// an absent entry, an unflagged one and a flagged one with no file each answer not found.
// A merge that removes a flagged slug and upserts it again keeps the flag.
// LoadConfig refuses a design_prefix that would render a design file onto a record file's name:
// a prefix of `decision-` or one starting with it, the empty prefix included.
//
// # Sketch
//
// An entry whose Discussion-only run, a motif run, ended with its decision record approved and stored is a Sketch.
// A Sketch is a note of the built-in type `sketch`, handle letter S, which never runs.
// It is the stage before triage, which breaks it down into tasks, each with its own type and recipe, or into a project.
// Board.MakeSketch makes an entry a Sketch in one write.
// The result is a note whose labels are `sketch` followed by the entry's labels that are not type labels.
// It has no status, depends_on, isolated, recipe, profile, run_settings, pull_request or notify_on_land;
// its title, brief, body, project, issues, Created date and discussed flag stay.
// The write stamps Revised like any scope change.
// It writes nothing for an absent, done or already sketched entry.
// It refuses while another entry depends on the entry, since a note cannot be depended on.
// The README lists Sketches under their own `Sketches` heading, the first Notes subsection.
// `lym board list --text` lists them after every other entry under a `sketches:` line.
//
// A Sketch becomes a task only as a raddle run's entry:
// Board.PromoteTo with recipe `raddle`, or with no recipe, which gives a Sketch `raddle`, makes the task in one write.
// The write keeps the decision record, the discussed flag and the sketch label.
// It restores the label when a note write dropped it, and it releases the task, so its parent's dobby starts the run.
// Board.Promote is PromoteTo with no recipe.
// Two write rules close every other way.
// The first rule covers a discussed note whose record file exists.
// A write that makes it a task must carry recipe `raddle`, the discussed flag and the sketch label,
// or the write is refused with the plain promote, `lym board promote '{"slug":"<slug>"}'`, as the way forward;
// the rule keys on the flag and the stored record, never on the label.
// The second rule refuses the sketch label on a task whose recipe is not `raddle`, or on a project.
// It is judged only for a write that adds the label, makes the entry a task or project, or changes its recipe,
// so a legacy task that already carries the label keeps taking writes that do none of these.
// The label rule cannot see the record file.
// So a discussed note whose record file is missing is led to the plain promote as well.
// The plain promote gives it recipe raddle when it still carries the label.
// The Bolt replayer admits an entry through the run lock and the kind and label checks only, never the promotion rule.
// So a promotion in a board commit from another machine can land on origin.
//
// # Raddle store
//
// A raddle run that files a Sketch's entries keeps a store on the Bolt beside board.json.
// The directory `raddle/<sketch slug>/` (RaddleStoreDir) holds these files:
// `proposal.yaml`, the approved proposal;
// `cold-proposal.yaml`;
// `decision-record.md`, the Sketch's record;
// `store.json`.
// store.json records these fields:
// `created`, the Sketch's created stamp;
// `hash`, the approved proposal's hash;
// `filed`, every filed slug with the project's included;
// `project` for a project or beam shape, which is also in `filed`.
// Prune never touches the store, since it reads only board.json entries and decision files.
// The store therefore outlives the entries it filed.
// ReadRaddleStore reads every store.json under the store root, and an absent root holds none.
// RaddleStore.Holds answers the entry of the Sketch slug, or of the Sketch whose `filed` holds the slug.
// The write critical section reads the store once under the lock, before the mutation,
// so the slug rule judges the store as it stood before the write.
// It then refuses a write that makes a slug the store holds, whether or not the pair slug seam is set.
// The exception is a made entry that carries a store entry's Sketch slug and that entry's recorded created stamp,
// so a live raddle run's own Sketch is never refused.
// A store file that cannot be read refuses only a write that makes a slug.
// The refusal names the file and `git -C <bolt> checkout -- <path>` as the way forward;
// a write that makes no slug goes on.
// A refusal names a slug both the seam and the store admit.
// A note made a task or project keeps its slug.
// Its refusal names `lym board rename` of the note to such a slug before the promotion.
// These inputs reach a refusal:
// `lym board upsert` creating a task or project under a filed slug;
// `lym board rename` to a Sketch slug the store holds;
// `lym board promote` of a Sketch under a filed slug.
// The Bolt replayer adds an entry through the run lock and the kind and label checks only, never this rule,
// so a board commit from a machine that had not yet pulled the store can land an entry under a held slug on origin;
// only a raddle entry is caught, by batten's start guard.
//
// A raddle run writes the board through these Board methods, each one write under the board lock:
// FileRaddle files a Sketch's approved entries.
// The done marker is a directory the store already holds for the Sketch slug with the filing's created stamp and hash.
// FileRaddle writes nothing when that marker is present.
// Otherwise it runs the filing's re-check over the entries and the store as they stand.
// A refusal is wrapped in ErrRaddleRecheck.
// It then adds the project the shape files and the parts as discussed entries.
// It writes each entry's decision record file before the save.
// After the save it writes the store directory with store.json last, so the write leaves its own done marker.
// The filing's Project is the Sketch's own project field, so every filed entry's parent beam equals the Sketch's;
// the parts are members of the new project when the shape files one.
// The parts are filed draft, so a release stays the operator's.
// The run's own entry stays open.
// A part's dependency on the run's entry, or on a slug no longer on the board, is dropped.
// A family filing, whose project is the run's own entry, turns that entry into the project instead of adding one.
// When the write fails, the entry's former decision record is written back.
// Every filing moves each open entry's dependency on the run's entry onto the filed parts no other part depends on.
// A refusal of that move is wrapped in ErrRaddleRepointRefused and names the refusing entry.
// EndRaddleEntry marks the run's entry done and clears its notify_on_land.
// The run lock admits both changes, since neither is scope.
// It writes nothing for an absent or done entry.
// ClearProjectRunStatus clears a run status from a family's project and releases no member.
//
// # Siblings
//
// A write that gives an open task a depends_on entry it did not have reports its siblings:
// the other open tasks that wait on that same entry while sharing a module label with the written task.
// A dependency on a project waits on the project and each of its landing members not yet done.
// So a task waiting on a project and one waiting on a member of it wait on the same entry.
// A module label is a label the vocabulary does not report as a type label.
// SharedModuleLabels answers the overlap, in the written task's label order.
// The written task's baseline is its own depends_on as loaded plus the depends_on of every entry the write removed,
// so a merge warns only on dependencies none of the merged entries had.
// A dependency listed twice counts once.
// A dependency that is met is not added, and a done written task is skipped.
// A sibling that is done or abandoned is not one; a running sibling is.
// A pair reached from both sides, as in a batch, is reported once with the dependencies of both sides.
// SetDeps, UpsertTaskSiblings, UpsertTasksBatch and MergeTasks return the siblings.
// They are computed under the write lock from the entries as loaded and as the mutation left them.
// The check never refuses, never changes a stored record and returns no siblings for a refused write.
//
// # Concurrency and sync
//
// Board sequences all mutating operations with a file lock:
// lock → load → mutate → save board.json → move or remove decision record files → render → write files.
// After each write, sync.go launches a detached background process that commits and pushes the changes to the remote.
// The write returns immediately without waiting for the sync.
// Read methods (Get/List) bypass the lock and load directly from disk, persisting nothing.
//
// The detached sync path talks to git through fabricengine.Bolt, never hand-rolled gitexec calls.
// It runs under the board write and push locks, which sit in the board checkout's never-tracked lock directory.
// The swap lock that fences readers of board.json sits there too,
// and the board's .gitignore ignores only the render manifest.
// The detached sync is spawned as `lym board sync --deferrable` in the board directory,
// so its network gate is held under the hub scope.
// Under a pause it defers instead of waiting:
// it exits with one Info line before it commits or pushes anything.
// When the pause came later, it exits at its push's network call instead.
// Either way the change is left for the next sync.
//
// # Storage
//
// Board lives at weft:main, never a separate repo.
// fabricengine pairs every warp branch <branch> with a weft branch <branch>-weft, with no exceptions.
// No task's weft branch can therefore be named exactly the warp's default branch,
// so the unsuffixed name is never claimed by the pairing convention and is reserved for board.
//
// The long-lived "prime" worktree is the only worktree that checks out two weft branches at once:
// its own <name>-weft companion, and weft:main for board access, which is paired with no warp branch.
// No other worktree checks out weft:main directly.
//
// weft:main has no corresponding warp branch,
// so the Warp-SHA trailer and correspondence-index machinery does not apply to it.
// That machinery is fabricengine.RecordCorrespondence and WeftSHAForWarpSHA.
// Board's reads and writes to weft:main are a standalone concern, not routed through fabric.Commit.
//
// # Replay
//
// FabricReplay is the board's Bolt replayer, named `board`.
// It is also the run ledger through which fabricengine reads the ended set and the owners.
// The lym root pre-run registers it with `fabricengine.RegisterBoltReplayer`.
// It claims these paths:
// `board.json` and `README.md`;
// the `project-<slug>.md` files, of which `project-root.md` is the root project's and not a project's;
// the `design-<slug>.md` and `decision-<slug>.md` files at the Bolt root;
// the files directly in a raddle store directory, `raddle/<sketch>/<name>`.
// Its ledger file is `board.json`:
// `Runs` decodes the entries with `Ended` set for a `done` one, and `CommitRuns` names the entries a commit changes.
// The by-entry merge of fields is `internal/entrymerge`'s, and its fields include the `discussed` flag.
// Apply re-applies a local board commit onto the board checked out at origin's tip, one change at a time.
// The commit is read out of the files the request holds by path.
// A change is one of these:
// an added or removed entry;
// a changed entry's fields, as one change;
// a design file or decision record the commit touches.
// A decision record goes after the entry changes:
// one the commit writes for an entry origin does not hold is reported lost and not written.
// Every other record file, a removal included, lands by the design-file rule.
// So a replayed removal or rename leaves no old record file on origin.
// A change passes the board's own checks, the run lock included.
// The checks run against the board as the kept changes before it left it.
// An added entry takes the next id of origin's board.json and a handle free there, never its local one.
// A raddle store file lands after the entry changes, by the design-file rule:
// a file origin does not hold lands.
// So a concurrent board write from another machine, which writes no file of that Sketch's store, replays beside it.
// A path of any other shape is refused.
// A store directory's files are set aside and reported refused in this case:
// the commit's `store.json` records as filed an entry whose add the replay refused or lost.
// So origin never holds the done marker of a filing without the entries it marks.
// The report names the Sketch.
// It says that its staged store and part records stay in the raddle run's own committed `_lym/raddle/` records.
// The operator re-files the parts that did not land with `lym board upsert` once the clash is settled.
// The replay itself files nothing.
// Origin's side is kept and the local value is reported lost in two cases:
// both sides changed the same field or design file since the merge base;
// origin removed the entry.
// A change a check refuses is reported refused, and so is a later change depending on an entry a refused change added.
// A commit that set `draft_status` sets it on origin's board too, and neither side ever clears it.
// It takes neither the board lock nor the push lock, spawns no sync, and writes nothing when no change survives.
//
// # loomyard's own area labels
//
// Before deploying the landed binary, write this list into the hub's live `_lym/config/board.yaml` in the Bolt checkout, then delete this paragraph.
// The migration turns every bracketed brief prefix on loomyard's live board that is not a type into a label, and the board refuses writes until board.yaml lists them.
// The PR description must quote this snippet verbatim, because the hub copies the list from the PR description, not from the diff.
//
//	labels: [undecided, agents, batcher, batten, board, bouncer, burler, cli, config, cost, discussion, docs, engines, fabric, finalize, github, glyph, ide, landing, loom, messaging, models, orch, parent-review, perf, permissions, plan, plugins, process, quarry, recipes, reed, review, search, shed, shuttle, slack, speed, testing, ui, vocabulary, webster, windows]
package boardengine
