// Package reportengine computes the read-only reports behind `lym report`: convergence, timing, cost, raddle and replies.
// Compute reports, per finished run and per ISO week:
// what woke the orch, what stopped and repaired runs, and how the parent branch's landings fared.
// Every figure comes from records lym already writes, read from git objects.
// Each is read through a tolerant parse in the package that owns its format.
// A record a run lacks is reported absent, never as zero.
// ComputeTiming, ComputeCost and ComputeRaddle work over the same snapshot as Compute.
// ComputeReplies reads creel log entries instead of a snapshot.
//
// # Told geometry
//
// The package never imports internal/lymcwd and derives no path of its own.
// Geometry carries the absolute checkout paths of the code, records and board repositories, and their branch names.
// The engine writes nothing, takes no lock and makes no commit.
// It names no fabric side and no run-record directory:
// record paths come from shedrun and the record packages,
// and the archive tag namespace and task-commit trailer come through fabricengine's neutral exports.
//
// # One ref snapshot
//
// Open reads each branch tip, the archive tags and the records repository's local branches once.
// It stamps the UTC time of the snapshot.
// Every later read names those tips, tags and branches, so a report sees one history while runs land concurrently.
// Compute works over Sources, whose Reader per repository a test replaces with an in-memory fake.
//
// # The run unit
//
// Every archive tag whose slug parses is one run, so a reused slug yields one run per tag.
// A tag committed before Options.Since is left out; a tag whose slug does not parse is a Finding.
// A run is full when the seed at the tag names the tapestry, jacquard, darn or hardener recipe;
// its status history must also hold a Finalize entry.
// Any other tag is a PartialRun that enters no per-run figure.
// A record that exists but does not decode is a Finding naming the tag and the file, and the tag is partial.
// A run's time is its tag's committer date in UTC, and its week is the ISO week with Monday as the first day.
//
// # The batten source
//
// Batten's status file for a slug is read from the records branch's history,
// so a run directory that batten cleaned up still reads from the last commit that held it.
// A run's batten source starts at the newest fresh seed committed after the slug's previous tag.
// That seed is at or before this tag.
// The span ends at the slug's next fresh seed or the branch tip.
// The run keeps every snapshot of that span, the last being its batten record.
// No fresh seed in that interval leaves the batten source absent.
//
// A report walks the records branch's first-parent line for the commits touching a run's directory.
// The walk opens a week before the start of the parent-branch walk.
// That start lies FreshDays before the window for the convergence report and at the window for the timing report.
// The walk widens, doubling its lookback, only while a run's fresh seed may lie before it.
// It never widens past the slug's previous tag, or a week before the run's seed stamp when there is no previous tag.
// The landing checkpoints come from a walk of their own that no run's seed changes.
// It opens at the parent walk's start.
// It opens at the opening of the oldest pair a tag found no task commit for when that is older.
// A run's status file is read once at each commit of its span and at the commit before its seed,
// and no parsed status is kept beyond the run's own snapshots.
//
// # Wakes, stops and repairs
//
// RunRow.Wakes counts a run's wakes per source and kind; a source the run lacks is nil under its name.
// Automatic steps (RunRow.Automatic) and paused changes (RunRow.Paused) are not wakes.
// Keep-alive pings sum into RunRow.ParentReviewPings and are never wakes.
// The land-ready-tasks source is always absent, since its records are ephemeral.
//
// Each batten change into blocked or failed takes the first unmatched halt note.
// It forms one Stop classed by the anomaly.
// A change with no note is a no-note stop; an unmatched note is a stop of its own class without a change.
// A change into awaiting is an awaiting stop and no failure.
// Without the halt-note source, Stops and Failures are absent whatever the batten changes.
//
// Automatic steps are batten's own.
// Arbiter repairs count per verdict word in RunRow.Arbiter, with how many carry the applied mark.
// Every other repair is an LLM step, counted per source in RunRow.LLMSteps;
// an entry or resume that an automatic step or an applied arbiter attempt accounts for is not one.
//
// # Review rounds and decisions
//
// RunRow.Segments maps each review segment and the parent review to its round count and the outcome of its last round.
// A Bouncer re-seed's archived generation adds its rounds to its segment.
// A segment some selected run holds and another lacks is nil in the run lacking it.
//
// # Landings
//
// Report.Landings lists the parent branch's first-parent commits committed at or after Options.Since, oldest first.
// The exact join maps a run to its landing through the task-commit trailer of its pair;
// a run whose history holds no trailer of its own, such as a darn run, finds no task commit.
// The landing is the first-parent commit that is the task commit, has it as a parent or holds its tree.
// The time join serves a tag the exact join found no task commit for,
// and a slug whose pair has checkpoints after its newest tag.
// A pair's checkpoints are read back to the checkpoint walk's start,
// so a pair with no archive tag whose checkpoints all lie before it opens no window.
// Two runs claiming one commit within a join are both left unmapped, and the clash is a finding.
// A landing no join maps is of kind unmapped.
// Any other takes its kind from the slug's labels on the board: bug is fix, feature and enhancement are feature.
// A full run landed by hand counts one landing LLM step.
//
// The board's history is read once, together with the board in effect at its start.
// It starts at the parent walk's start or the oldest time-join window's opening, whichever is older.
// Each board commit is read at most once.
// A landing's slug that the board in effect at the start lacks takes its labels from older commits.
// Those are read newest first until its entry is found.
// Only each slug's done spans and labels are kept from it, never a parsed board.
//
// # Fresh-code fixes
//
// For each fix and unmapped landing, the lines its commit modifies or deletes are blamed in its first parent.
// Test, docs and markdown files are left out.
// A line is fresh when the landing that brought its blamed commit was committed within Options.FreshDays before it.
// The landing is fresh when more than half its lines are, and aged otherwise.
// A feature or other landing is never blamed.
//
// # Weekly aggregates
//
// Report.Weeks holds one Week per ISO week that holds a run, a partial run or a landing, ascending.
// A run buckets by its tag's week, a landing by its own commit's week, and a partial run counts in Partial alone.
// A week sums a figure over the runs holding its source and states in Lacking how many of its runs do not.
// A figure no run holds, and a mean or share nothing carries, is nil and never 0.
// Report.Absent counts, per source, the selected runs lacking it.
//
// # Timing
//
// ComputeTiming is told its options, runs no process and reads no clock.
// Its window runs from TimingOptions.Since to Sources.SnapshotAt.
// A record that does not decode is a Finding, and a figure a run's records lack is nil, never zero.
// A landed run is exactly a full run as run selection picks it; a partial run enters no bucket.
// A run in flight is a pair records branch not landed or abandoned:
// a branch whose slug TimingOptions.Live names is in flight, and an abandoned one is left out with a finding.
// A run enters the report when its span overlaps the window.
//
// A run's span runs from its first batten or loom entry to its last stamp, or to Sources.SnapshotAt in flight.
// Each interval between rows counts under one phase through a closed table per recipe;
// a row the table lacks counts under other and adds a finding, so a new row shows up rather than vanishing.
// The jacquard table is tapestry's without its Discussion rows.
// TimingOptions.Sleeps, clipped to the span, is the run's sleep and comes out of every interval,
// so the phases plus the sleep sum to the span.
//
// A wait is time the run spent on someone else, of kind parent_review, escalation or halt.
// Overlapping waits count once under the kind listed first, and a sleep inside a wait counts as sleep alone.
// An open wait in a run in flight runs to the snapshot with no finding.
// A run's active seconds are its span less sleep, every net wait and the paused phase.
//
// An attempt is one spawn: every arbiter attempt and every parent-review round's alignment record.
// Its time is reported beside the waits, never added to them.
// An attempt whose time cannot be fixed adds a finding naming its file, and its spawn still counts.
//
// A stop is counted under one of three kinds: stop as convergence lists it, bounce, or batten-halt.
// Each stop's settler comes from the Go driver's settle records, read through shedrun.ParseSettleRecords,
// and the history's caller labels.
// A settle record matching no stop is a finding, except a loom file of a run without the halt-note source.
// A stop with no entry after it in a run in flight reads open, and one whose moving entry carries no by reads unknown.
//
// Buckets are UTC days or ISO weeks from Since, none opening before it.
// A bucket's time sums over runs, so two parallel runs add two hours to an hour.
// A phase's median is over the landed runs of that recipe holding it whose archive tag is committed in the window,
// and a run in flight never enters one.
//
// # Cost
//
// ComputeCost is told its options, runs no process and reads no clock.
// It takes every archive tag committed on or after CostOptions.Since, whatever its recipe.
// It reads their usage files through shedrun.ParseRunUsage, the one reader of those files.
// A tag with no run record, or with a usage file that does not decode, is skipped and a Finding.
// A cancelled or torn-down run is left out.
// A run's dollars are absent when it has no session or any session's cost is unknown; such a run still enters the hours.
// Runs group into cells by recipe, profile and stored-record flag.
// A tapestry run with the stored-record flag set groups under jacquard, whose rows it walked;
// the tapestry cell with that flag set stays empty.
// A cell with fewer runs than CostOptions.MinRuns carries the note "too few runs" and no figure.
// A cell's least-squares fit needs two sized runs of distinct sizes and is absent otherwise.
//
// # Raddle
//
// ComputeRaddle reports, per Sketch a raddle run filed, its predicted cost against what its filed parts cost.
// It reads each store.json through boardengine.ParseRaddleStoreEntry.
// A part with no archive tag is listed as unlanded.
// The actual cost is partial when a run holds a session of unknown cost, no session or an undecodable usage directory.
// It is nil when no session cost is known.
// A Sketch whose store files are missing or do not decode is skipped, and a Finding names it.
//
// # Replies
//
// ComputeReplies is pure over creel log entries and groups by the receiver's role:
// a strand's role, a program's upper-case name.
// It counts what this hub's receivers were asked, so an `outbound` post and an `inbound` reply count nothing.
// A reply whose reply-to names no marked message, and a name that is neither a strand nor a program, count nowhere.
// Roles come out in name order.
//
// # Outputs
//
// The JSON envelope is the Report itself.
// Every duration the renderers print is spelled through durfmt.FormatSeconds.
// A nil figure renders as no data.
package reportengine
