// Package reportengine computes the read-only report behind `lym report convergence`.
// It also computes the timing, cost, raddle and replies reports.
// Compute reports, per finished run and per ISO week:
// what woke the orch, what stopped and repaired runs, and how the parent branch's landings fared.
// Every convergence and timing figure comes from records lym already writes.
// Each is read from git objects through a tolerant parse in the package that owns its format.
// A record a run lacks is reported absent, never as zero.
// ComputeTiming, ComputeCost and ComputeRaddle work over the same snapshot as Compute.
// ComputeTiming reports where each run's time went, per phase and per day or week.
// ComputeCost reports what landed runs cost in dollars and hours, from the usage records each run keeps.
// ComputeRaddle reports, per Sketch a raddle run filed, its predicted cost against what its filed parts cost.
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
// Open reads each branch tip, the archive tags and the records repository's local branches (Sources.Branches) once.
// It then stamps the UTC time the snapshot was taken (Sources.SnapshotAt).
// Every later read names those tips, tags and branches, so a report sees one history while runs land concurrently.
// Compute reads neither Branches nor SnapshotAt.
// Compute works over Sources, whose Reader per repository a test replaces with an in-memory fake.
//
// # The run unit
//
// Every archive tag whose slug parses is one run, so a reused slug yields one run per tag.
// A tag committed before Options.Since is left out entirely; a tag whose slug does not parse is a Finding.
// At the tag's commit the engine reads the seed and status of the tapestry run directory through their tolerant parses.
// The run is full when both hold:
// the seed names the tapestry or the darn recipe, each of which writes that directory;
// and the status history holds a Finalize entry.
// Any other tag is a PartialRun that enters no per-run figure.
// RunRow.Recipe is the recipe the seed names.
// Every figure reads a darn run's records as it reads a tapestry run's.
// A record that exists but does not decode is a Finding naming the tag and the file, and the tag is partial.
// A run's time is its tag's committer date in UTC, and its week is the ISO week with Monday as the first day.
//
// # The batten source
//
// Batten's status file for a slug is read from the records branch's history.
// One walk covers the commits touching the run records directory.
// A snapshot is a fresh seed when both hold:
// its history opens with the worktree-create row;
// the previous commit touching the file held no file or a longer history.
// A run's own fresh seed is the newest one committed after the slug's previous tag and at or before this tag;
// its span runs to the slug's next fresh seed, or to the branch tip when none follows.
// The run keeps every snapshot of its span, oldest first, the last being its batten record,
// so a run directory that batten cleaned up still reads from the last commit that held it.
// No fresh seed in that interval leaves the batten source absent.
//
// # Wakes and automatic steps
//
// A run's wakes are counted per source in RunRow.Wakes, a source the run lacks being nil under its name.
// The batten source classifies the batten snapshots and the loom history at the tag.
// It uses the row names batten is told in production.
// Each wake counts under its kind.
// Each automatic step counts under RunRow.Automatic by kind.
// Each paused change counts under RunRow.Paused.
// Neither is a wake.
// A halt at a Bouncer row counts as a review escalation in this case:
// the row's review segment, or an archived generation of it, holds an escalation file.
// The parent-review source counts each round that reached the reviewer without an applied alignment verdict;
// keep-alive pings sum into RunRow.ParentReviewPings and are never wakes.
// The alignment source counts an applied approve that superseded entries as a wake.
// It counts every attempt in RunRow.Alignment per point and verdict word.
// The land-ready-tasks source is always absent, since its records are ephemeral.
//
// # Stops
//
// The halt-note source is present when the tag's tree holds the friction directory or an archived sibling of it.
// Each batten change into blocked or failed takes, in order, the first unmatched halt note with its producer and state;
// the pair is one Stop classed by the note's anomaly.
// A change with no note is a no-note stop.
// A note left unmatched is a stop of its own class without a change.
// A change into awaiting is an awaiting stop and no failure.
// A stop carrying a note has a failure class:
// its producer and the reason's leading clause with quoted strings, paths and digit runs normalised.
// Without the source, Stops and Failures are absent whatever the batten changes.
//
// # Repairs by actor
//
// Automatic steps are batten's own and count in RunRow.Automatic.
// Arbiter repairs count per verdict word in RunRow.Arbiter, with how many carry the applied mark.
// Every other repair is an LLM step, counted per source in RunRow.LLMSteps.
// The records do not tell the loom driver from the orch, so the sources are:
// the loom history's goto and reroute entries;
// the batten record's goto and reroute entries and its resumes from a halt.
// An entry or resume that an automatic step or an applied arbiter attempt accounts for is not one;
// a verify retry accounts for the goto it sends and the first resume after its halt.
// A source a run lacks has no key.
//
// # Review rounds and decisions
//
// RunRow.Segments maps each review segment and the parent review to its round count and the outcome of its last round.
// A Bouncer re-seed's archived generation adds its rounds to its segment and is no segment of its own.
// A run's `code` review directory is the Code-Review segment.
// An older run's `webster` directory is read as the same segment.
// A `merge` directory is not read.
// A segment some selected run holds and another lacks is nil in the run lacking it.
// RunRow.Decisions counts the entries added after the discussion per author.
// RunRow.DriveReports counts the driver attempts reported.
//
// # Landings
//
// Report.Landings lists the parent branch's first-parent commits committed at or after Options.Since, oldest first.
// One walk produces them, reaching Options.FreshDays further back to date the owners of blamed lines.
// The exact join reads each selected tag's task commit from a task-commit trailer.
// It takes the newest trailer in the first-parent history below the tag, walked in growing pages.
// The walk ends at the commit that opened the run's pair, named by fabricengine's origin-record subject:
// a trailer below it is the parent's own and names a parent landing,
// so a run whose history holds no trailer of its own, such as a darn run, finds no task commit.
// The landing is the first-parent commit that does one of these:
// is the task commit (a fast-forward), has it as a parent (a merge) or holds its tree (a squash).
// A fast-forwarded branch's own commits belong to its landing and are neither landings nor unmapped.
// They run back to the nearest older merge or exact-joined landing.
// The time join serves two cases:
// a tag the exact join found no task commit for, and a slug whose pair has checkpoints after its newest tag.
// Its window opens at the newest landing checkpoint of the run's interval.
// It closes at the first board commit that shows the slug's entry done.
// The landing is the first commit inside the window that no exact join claimed.
// Two runs claiming one commit within a join are both left unmapped, and the clash is a finding.
// A landing no join maps is of kind unmapped.
// Any other takes its kind from the slug's labels in the newest board commit at or before it:
// bug is fix, feature and enhancement are feature, the rest other.
// An exact-joined tag whose loom history holds no done Finalize entry was landed by hand.
// A full run so landed counts one landing LLM step.
// RunRow.Landing points at a full run's own landing.
// RunRow.LandIncomplete says a finished Finalize never reached Land in the batten record.
//
// # Fresh-code fixes
//
// For each fix and unmapped landing, the lines its commit modifies or deletes are blamed in its first parent.
// Test, docs and markdown files are left out.
// A line is fresh when the landing that brought its blamed commit was committed within Options.FreshDays before it.
// For a task-branch commit that landing is the fast-forward landing.
// The landing is fresh when more than half its lines are, and aged otherwise.
// It has no pre-image when no such line remains.
// A feature or other landing is never blamed.
//
// # Weekly aggregates
//
// Report.Weeks holds one Week per ISO week that holds a run, a partial run or a landing, ascending.
// A run buckets by its tag's week.
// A partial run counts in its week's Partial alone.
// A landing buckets by its own commit's week.
// A week's figures sum over its runs of every recipe, and Week.Recipes counts its runs per recipe.
// A week sums a figure over the runs holding its source.
// It states in Lacking how many of its runs do not.
// A figure no run holds, and a mean or share nothing carries, is nil and never 0.
// The wake mean covers the runs holding every wake source but land-ready-tasks, which is absent for every run.
// A class's Runs counts the week's runs it occurred in.
// A share is fresh over fresh plus aged, separately for fix and unmapped landings.
// Report.Absent counts, per source, the selected runs lacking it.
//
// # Timing
//
// ComputeTiming works over the same Sources snapshot.
// It is told its options, runs no process and reads no clock.
// Its window runs from TimingOptions.Since to Sources.SnapshotAt.
// A Since after the snapshot gives an empty report with no bucket.
// A record that does not decode is a Finding, and a figure a run's records lack is nil, never zero.
//
// A landed run is exactly a full run as run selection picks it, of either recipe.
// It is read once with its seed, status and batten snapshots.
// A partial run enters neither the run table nor any bucket.
// A run in flight is a pair records branch of Sources.Branches:
// a branch other than the records branch whose name is the one fabricengine.RecordsBranchName forms from a slug.
// A branch whose tip is the commit of one of its slug's archive tags is landed and skipped;
// one whose slug TimingOptions.Live names is in flight;
// any other is abandoned and left out with a finding.
// A pair not seeded yet, or seeded to a recipe other than tapestry or darn, is left out silently.
// TimingRun.Recipe is the recipe the run's seed names.
// A run in flight takes the batten snapshots committed after its slug's newest archive tag,
// so a reused slug never reads an earlier run's.
// A run enters the report when its span overlaps the window.
//
// A run's span opens at the first entry of its batten record, else at the seed's started_at.
// Failing both, it opens at the first child row, with a finding saying so.
// The loom history and the batten record's history merge in stamp order, each keeping its own order.
// A row's stamp is its last_at when set, else its at.
// Each row's interval runs from the latest stamp so far, the span's opening for the first row, to its own stamp.
// A batten row between two loom rows therefore splits their gap.
// A stamp earlier than the latest, or none, gives a zero interval and a finding.
// The span closes at the last stamp of either history, or at Sources.SnapshotAt for a run in flight.
// A run in flight's trailing interval counts under the phase of its loom status's current producer.
// Instead it counts under paused when the last row is the batten record's entry for the child's change into paused.
//
// Each interval counts under one phase, and the child's rows map to phases through a closed table per recipe.
// Every run's setup holds every batten row before batten's first Run-Shed entry;
// land every batten row other than Run-Shed after batten's first Run-Shed entry;
// paused every Run-Shed entry whose change is from paused,
// so the time the child stood paused at a producer boundary counts apart from the row it paused at.
// A tapestry run's setup holds Preflight and Drift-Expect;
// discussion the Motif block row and every Discussion row;
// plan every Plan row, Parts-Write, Parts-Plan-Write, Drift-Baseline and Drift-Await;
// webster every Webster row and every Code row (Code-Bouncer, Code-Burler-Review, Code-Burler-Fix);
// webster also Drift-Verify, Drift-Recheck and Describe;
// webster also Batchifier, a row only a past run's history holds;
// merge Parent-Align and every Merge row;
// publish-finalize PR-Review, Finalize and Friction-Reflect.
// The table also maps, by the literals the report declares, rows that only a past run's history holds:
// Publish, PR-Align, PR-Gate and PR-Rework count under publish-finalize;
// Loom-Preflight counts under setup;
// the Merge-In, Merge-Bouncer and Merge-Burler rows count under merge;
// the Webster-Bouncer and Webster-Burler rows count under webster.
// A darn run's setup also holds Preflight, Record-Write, Drift-Baseline, Drift-Await and Drift-Expect;
// write Darn-Write and Describe, and each row the darn recipe retires counts under its successor's phase;
// review every Code-Review row (Code-Bouncer, Code-Burler-Review, Code-Burler-Fix), Drift-Verify and Drift-Recheck;
// review also the Darn-Bouncer, Darn-Burler-Review and Darn-Burler-Fix rows a past run's history holds;
// merge the Merge-In row a past run's history holds;
// publish-finalize PR-Review, Finalize and Friction-Reflect, and the Publish, PR-Align and PR-Gate rows of a past run.
// Any other Run-Shed entry records a change of the child's state.
// It counts under the phase the run's recipe table gives the producer its change names;
// failing that, under the latest child row before it, else under setup.
// Any other row counts under other and adds a finding,
// so a row a recipe gains before its table does shows up rather than vanishing.
// Without the batten source land is nil and setup holds the loom rows alone.
//
// TimingOptions.Sleeps, unioned and clipped to the span, is the run's sleep.
// Every row interval loses the part a sleep covers, so the phases plus the sleep sum to the span.
//
// A wait is time the run spent on someone else, an interval of one kind.
// The kinds, in the order an overlap is credited to them:
// parent_review, a parent-review round from its request's opened_at to its verdict's recorded_at;
// pr_gate, a batten change of the child into awaiting at PR-Gate, a row only a past run holds;
// escalation, a change into awaiting at any other row;
// halt, a change into blocked or failed.
// A change's wait runs from batten's entry for it to the next row of either history stamped after it.
// A batten-row halt is an entry at one of batten's own rows, never Run-Shed.
// It is an awaiting outcome, or a stuck outcome no OnStuck edge bounced (its routed_to is empty).
// It is a wait of kind escalation when awaiting and halt otherwise, from its entry to the next row.
// A batten stuck entry written before routed_to existed has it empty even when an edge bounced it,
// so an older batten record reads such a bounce as a batten-row halt.
// A wait with no later row and no verdict runs to the span's close, which is Sources.SnapshotAt for a run in flight.
// An open wait in a run in flight therefore runs to the snapshot with no finding.
// A parent-review round with no request opens no wait.
// Each wait is clipped to the span, and sleep comes out first.
// Then each kind's union loses every earlier kind's.
// Overlapping waits therefore count once under the kind listed first, and a sleep inside a wait counts as sleep alone.
// Waits stay inside their phase's time;
// a run's active seconds are its span less sleep less the union of every net wait and the paused phase's time.
// Active, the waits, the paused phase and sleep so sum to the span when no wait overlaps a pause.
//
// An attempt is one spawn: every arbiter attempt and every parent-review round's alignment record.
// Its time runs, net of sleep, from its spawned_at to the first row of either history stamped after it.
// In a run in flight it runs instead to the span's close.
// In a landed run, an attempt spawned outside the span or with no row after it has no time.
// It adds a finding naming its file, and its spawn still counts.
// A figure's seconds are the summed time of the attempts whose time is known, nil when none is.
// An attempt's time lies inside the wait it serves and is reported beside the waits, never added to them.
//
// A stop is counted under one of three kinds.
// stop is a halt or awaiting stop as convergence lists it, so its count matches convergence's:
// the k-th stop with a change takes the time of the k-th batten change into blocked, failed or awaiting,
// and a note-only stop carries no time and is counted in the bucket of the run's close;
// without the halt-note source convergence lists no stop, so the kind is absent and its settler counts are left out.
// bounce is a loom stuck entry an OnStuck edge bounced, its routed_to set; batten-halt is a batten-row halt as above.
//
// Each stop's settler comes from the Go driver's settle records and the history's caller labels.
// A run's records are two files read through shedrun.ParseSettleRecords, each at its run directory:
// the tapestry run's at the archive tag, or at the pair branch tip for a run in flight;
// batten's at the commit holding the run's batten record.
// Only a landed run read by ComputeTiming and a run in flight read their settle files, so convergence reads none.
// An absent file holds no record.
// A record not committed at that tip is not there, and its stop reads open.
// A line that does not decode is a finding naming the file and the line.
// A record's stop key holds the history length the driven history had when the driver saw the halt,
// so its last counted entry is the entry that halted.
// A batten-file record names a batten-row halt when its producer and state equal the halt's,
// and its history length is the 1-based position of the halt's entry in batten's history.
// It names nothing else.
// A loom-file record names a child stop with a change when its producer and state equal the stop's,
// and its history length is the greatest among such records whose last counted entry is stamped at or before the change.
// A halt the Go driver restepped into a second halt before batten observed it therefore reads the second record,
// and the earlier record folds into the stop with no finding.
// A note-only stop takes the earliest record of its producer and state no other stop took.
// A change whose producer is empty matches no record.
// Any other record matching no stop is a finding naming the file and the key.
// A loom file of a run without the halt-note source is excepted, since it lists no stop to match.
// The settler is the first of these that holds:
// judge for a bounce;
// shed when a batten automatic step accounts for the stop;
// shed also when its record is a go-rule restep or a past run's verify-retry;
// tackler for a tackler restep record;
// arbiter when an applied arbiter attempt for the stop's producer and state was spawned after it;
// alignment when an applied parent-review alignment record was spawned after the stop;
// else the by of the first entry after the stop in the history that moved it on:
// the loom's for a child stop and batten's for a batten-row halt, with the caller label shed read as shed.
// A stop whose moving entry carries no by reads unknown, as every entry written before the label existed does.
// A stop with no entry after it in a run in flight reads open.
// A go-rule record of action moved, busy-exit, running-exit or hand-over settles nothing by itself.
// Neither does a spawn record, a parent hand-back record or any arbiter record.
//
// Buckets are UTC days or ISO weeks from Since, none opening before it.
// Each interval, wait and sleep is clipped to every bucket it overlaps.
// A bucket's time sums over runs, so two parallel runs add two hours to an hour.
// Each attempt spawn counts in the bucket its spawned_at falls in and its time is clipped to every bucket it overlaps.
// Each stop counts in the bucket its time falls in under its kind and settler.
// TimingReport.Phases holds, per recipe, each phase a run of it can hold.
// That is the phases its table names plus setup, land, paused and other.
// Each phase's median is over the landed runs of that recipe holding it whose archive tag is committed in the window;
// the median of an even count is the mean of the two middle values rounded down,
// and a run in flight never enters one.
//
// # Cost
//
// ComputeCost is told its options, runs no process and reads no clock.
// It takes every archive tag committed on or after CostOptions.Since, whatever its recipe.
// It reads each tag's usage files at the tag's commit through shedrun.ParseRunUsage, the one reader of those files.
// It never reads a review round's usage record.
// A tag with no run record, or with a usage file that does not decode, is skipped and a Finding.
// A run is landed when its usage holds a landed-size record.
// For a recipe with no Finalize row, a run is landed when its status at the tag is done.
// A cancelled or torn-down run holds neither and is left out.
// A run's dollars are the sum of its sessions' costs, absent when it has no session or any session's cost is unknown.
// A run with no dollar figure still enters the hours.
// A run's hours are its active seconds from ComputeTiming, joined by tag, for a run the timing report times;
// any other run's hours are the union of its sessions' started-to-ended spans, overlaps counted once,
// and a run with neither has no hours.
// Runs group into cells by recipe, profile and stored-record flag.
// A cell reports its run count, and the mean dollars and mean hours over the runs holding each.
// Over the runs holding a landed-size record it also reports a least-squares fit in dollars and in hours:
// a fixed cost plus a cost per 100 changed lines, each with its correlation.
// A fit needs two sized runs of distinct sizes and is absent otherwise.
// A cell with fewer runs than CostOptions.MinRuns carries the note "too few runs" and no figure.
//
// # Raddle
//
// ComputeRaddle lists the store directories of the board branch's tip, one per Sketch a raddle run filed.
// It reads each one's store.json through boardengine.ParseRaddleStoreEntry.
// A Sketch's predicted cost is its approved proposal's expected cost, nil when the proposal states none.
// Its proposal is changed when the hash of the approved proposal differs from the cold proposal's.
// A filed part is a filed slug other than the filed project.
// Its runs are the archive tags of that slug, each read through shedrun.ParseRunUsage.
// A part with no archive tag is listed as unlanded.
// The actual cost sums the session costs of the landed parts' runs.
// It is partial when a run holds a session of unknown cost, no session or a usage directory that does not decode.
// It is nil when no session cost is known.
// A store directory without a store.json holds nothing.
// A Sketch whose store files are missing or do not decode is skipped, and a Finding names it.
//
// # Replies
//
// ComputeReplies is pure over creel log entries.
// It groups by the receiver's role: a strand's role, a program's upper-case name.
// Per role it counts these figures:
// Expected is the `posted` and `inbound` entries marked as expecting a reply, by their receiver.
// Replied, by the reply's sender, is the sum of two entry kinds:
// the `posted` reply entries whose reply-to names a message the entries show as marked;
// the `outbound` entries whose `Answers` names one.
// Chased is the `chased` entries, by their receiver.
// Unanswered is the `unanswered` entries, by their holder.
// An `outbound` post and an `inbound` reply count nothing, since the report counts what this hub's receivers were asked.
// A reply whose reply-to names no marked message counts nowhere.
// A name that is neither a strand nor a program counts nowhere.
// Roles come out in name order.
//
// # Outputs
//
// The JSON envelope is the Report itself: selection, runs, partial, landings, weeks, absent and findings.
// RenderMarkdown renders the same fields as the selection and one table per board question:
// wakes per run, with the week's runs per recipe;
// recurring stop and failure classes;
// the fresh-code fix share;
// review rounds per segment.
// A nil figure renders as no data.
// RenderTimingMarkdown renders a TimingReport as the selection and sleep note, then these tables:
// runs (recipe, state, span, active, each wait kind, sleep, each phase and the attempt counts with their time);
// phase medians per recipe;
// time per bucket;
// stops per settler per bucket.
// Every duration is spelled through durfmt.FormatSeconds, with seconds below a day and as 2d03h from a day on.
// RenderCostMarkdown renders a CostReport as the selection and one table of cells.
// A figure the cell lacks renders as no data.
// RenderRaddleMarkdown renders a RaddleReport as the selection and one table of Sketches.
package reportengine
