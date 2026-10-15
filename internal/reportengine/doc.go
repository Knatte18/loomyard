// Package reportengine computes the read-only report behind `lyx report convergence`: per finished run and per ISO week, what woke the orch, what stopped and repaired runs, and how the parent branch's landings fared.
// Every figure comes from records lyx already writes, each read from git objects through a tolerant parse in the package that owns its format.
// A record a run lacks is reported absent, never as zero.
// ComputeTiming is the second compute entry, over the same snapshot: where each run's time went, per phase and per day or week.
//
// # Told geometry
//
// The package never imports internal/lyxcwd and derives no path of its own.
// Geometry carries the absolute checkout paths of the code, records and board repositories and their three branch names, and the engine writes nothing, takes no lock and makes no commit.
// It names no fabric side and no run-record directory: record paths come from shedrun and the record packages, and the archive tag namespace and task-commit trailer come through fabricengine's neutral exports.
//
// # One ref snapshot
//
// Open resolves each branch tip, lists the archive tags and lists the records repository's local branches (Sources.Branches) once, then stamps the UTC time the snapshot was taken (Sources.SnapshotAt).
// Every later read names those tips, tags and branches, so a report sees one history while runs land concurrently.
// Compute reads neither Branches nor SnapshotAt.
// Compute works over Sources, whose Reader per repository a test replaces with an in-memory fake.
//
// # The run unit
//
// Every archive tag whose slug parses is one run, so a reused slug yields one run per tag.
// A tag committed before Options.Since is left out entirely; a tag whose slug does not parse is a Finding.
// At the tag's commit the engine reads the loom seed and status through their tolerant parses.
// The run is full when the seed names the loom recipe and the status history holds a Finalize entry; any other tag is a PartialRun that enters no per-run figure.
// A record that exists but does not decode is a Finding naming the tag and the file, and the tag is partial.
// A run's time is its tag's committer date in UTC, and its week is the ISO week with Monday as the first day.
//
// # The batten source
//
// Batten's status file for a slug is read from the records branch's history, one walk over the commits touching the run records directory.
// A snapshot is a fresh seed when its history opens with the worktree-create row and the previous commit touching the file held no file or a longer history.
// A run's own fresh seed is the newest one committed after the slug's previous tag and at or before this tag; its span runs to the slug's next fresh seed, or to the branch tip when none follows.
// The run keeps every snapshot of its span, oldest first, the last being its batten record, so a run directory that batten cleaned up still reads from the last commit that held it.
// No fresh seed in that interval leaves the batten source absent.
//
// # Wakes and automatic steps
//
// A run's wakes are counted per source in RunRow.Wakes, a source the run lacks being nil under its name.
// The batten source classifies the batten snapshots and the loom history at the tag under the row names batten is told in production.
// Each wake counts under its kind, each automatic step under RunRow.Automatic by kind, and each paused change under RunRow.Paused, none of them a wake.
// A halt at a Bouncer row counts as a review escalation when the row's review segment, or an archived generation of it, holds an escalation file.
// The parent-review source counts each round that reached the reviewer without an applied alignment verdict; keep-alive pings sum into RunRow.ParentReviewPings and are never wakes.
// The alignment source counts an applied approve that superseded entries as a wake, and every attempt in RunRow.Alignment per point and verdict word.
// The land-ready-tasks source is always absent, since its records are ephemeral.
//
// # Stops
//
// The halt-note source is present when the tag's tree holds the friction directory or an archived sibling of it.
// Each batten change into blocked or failed is matched, in order, to the first unmatched halt note with the same producer and state, and the pair is one Stop classed by the note's anomaly.
// A change with no note is a no-note stop, a note left unmatched is a stop of its own class without a change, and a change into awaiting is an awaiting stop and no failure.
// A stop carrying a note has a failure class: its producer and the reason's leading clause with quoted strings, paths and digit runs normalised.
// Without the source, Stops and Failures are absent whatever the batten changes.
//
// # Repairs by actor
//
// Automatic steps are batten's own and count in RunRow.Automatic.
// Arbiter repairs count per verdict word in RunRow.Arbiter, with how many carry the applied mark.
// Every other repair is an LLM step, counted per source in RunRow.LLMSteps since the records do not tell the loom driver from the orch:
// the loom history's goto and reroute entries, and the batten record's goto and reroute entries and its resumes from a halt.
// An entry or resume that an automatic step or an applied arbiter attempt accounts for is not one; a verify retry accounts for the goto it sends and the first resume after its halt.
// A source a run lacks has no key.
//
// # Review rounds and decisions
//
// RunRow.Segments maps each review segment, and the parent review, to its round count and the outcome of its last round.
// A Bouncer re-seed's archived generation adds its rounds to its segment and is no segment of its own.
// A segment some selected run holds and another lacks is nil in the run lacking it.
// RunRow.Decisions counts the entries added after the discussion per author, and RunRow.DriveReports the driver attempts reported.
//
// # Landings
//
// Report.Landings lists the first-parent commits of the parent branch committed at or after Options.Since, oldest first, from one walk that reaches Options.FreshDays further back to date the owners of blamed lines.
// The exact join reads each selected tag's task commit from the newest task-commit trailer in the first-parent history below the tag, walked in growing pages.
// The landing is the first-parent commit that is the task commit (a fast-forward), has it as a parent (a merge) or holds its tree (a squash).
// A fast-forwarded branch's own commits, back to the nearest older merge or exact-joined landing, belong to its landing and are neither landings nor unmapped.
// The time join serves a tag the exact join found no task commit for, and a slug whose pair has checkpoints after its newest tag:
// its window opens at the newest landing checkpoint of the run's interval and closes at the first board commit that shows the slug's entry done, and the landing is the first commit inside it that no exact join claimed.
// Two runs claiming one commit within a join are both left unmapped, and the clash is a finding.
// A landing no join maps is of kind unmapped; any other takes its kind from the slug's labels in the newest board commit at or before it: bug is fix, feature and enhancement are feature, the rest other.
// An exact-joined tag whose loom history holds no done Finalize entry was landed by hand, and a full run so landed counts one landing LLM step.
// RunRow.Landing points at a full run's own landing, and its LandIncomplete says a finished Finalize never reached Land in the batten record.
//
// # Fresh-code fixes
//
// For each fix and unmapped landing the lines its commit modifies or deletes, outside test, docs and markdown files, are blamed in its first parent.
// A line is fresh when the landing that brought its blamed commit, the fast-forward landing for a task-branch commit, was committed within Options.FreshDays before it.
// The landing is fresh when more than half its lines are, aged otherwise, and has no pre-image when no such line remains.
// A feature or other landing is never blamed.
//
// # Weekly aggregates
//
// Report.Weeks holds one Week per ISO week that holds a run, a partial run or a landing, ascending.
// A run buckets by its tag's week, a partial run counts in its week's Partial alone, and a landing buckets by its own commit's week.
// A week sums a figure over the runs holding its source and states in Lacking how many of its runs do not; a figure no run holds, and a mean or share nothing carries, is nil and never 0.
// The wake mean covers the runs holding every wake source but land-ready-tasks, which is absent for every run.
// A class's Runs counts the week's runs it occurred in.
// A share is fresh over fresh plus aged, separately for fix and unmapped landings.
// Report.Absent counts, per source, the selected runs lacking it.
//
// # Timing
//
// ComputeTiming is the second compute entry, over the same Sources snapshot.
// It is told its options and runs no process and reads no clock: its window runs from TimingOptions.Since to Sources.SnapshotAt, and a Since after the snapshot gives an empty report with no bucket.
// A record that does not decode is a Finding, and a figure a run's records lack is nil, never zero.
//
// A landed run is exactly a full run as run selection picks it, read once with its seed, status and batten snapshots; a partial run enters neither the run table nor any bucket.
// A run in flight is a pair records branch of Sources.Branches: a branch other than the records branch whose name is the one fabricengine.RecordsBranchName forms from a slug.
// A branch whose tip is the commit of one of its slug's archive tags is landed and skipped; one whose slug TimingOptions.Live names is in flight; any other is abandoned and left out with a finding.
// A pair not seeded yet, or seeded to another recipe, is left out silently.
// A run in flight takes the batten snapshots committed after its slug's newest archive tag, so a reused slug never reads an earlier run's.
// A run enters the report when its span overlaps the window.
//
// A run's span opens at the first entry of its batten record, else at the seed's started_at, else at the first loom row with a finding saying so.
// The loom history and the batten record's history merge in stamp order, each keeping its own order, a row's stamp being its last_at when set, else its at.
// Each row's interval runs from the latest stamp so far, the span's opening for the first row, to its own stamp, so a batten row between two loom rows splits their gap.
// A stamp earlier than the latest, or none, gives a zero interval and a finding.
// The span closes at the last stamp of either history, or at Sources.SnapshotAt for a run in flight, whose trailing interval counts under the phase of its loom status's current producer.
//
// Each interval counts under one phase of a closed table.
// setup holds every batten row before batten's first Run-Shed entry and loom's Preflight and Loom-Preflight; discussion every Discussion row; plan every Plan row and Batchifier; webster every Webster row and Describe; merge every Merge row; publish-finalize Publish, PR-Align, PR-Gate, PR-Rework, every Darn row, Finalize and Friction-Reflect; land every batten row other than Run-Shed after batten's first Run-Shed entry.
// A Run-Shed entry records the child's state changes and counts under the phase of the producer its change names, else of the latest loom row before it, else under setup.
// Any other row counts under other and adds a finding.
// Without the batten source land is nil and setup holds the loom rows alone.
//
// TimingOptions.Sleeps, unioned and clipped to the span, is the run's sleep, and every row interval loses the part a sleep covers, so the phases plus the sleep sum to the span.
//
// A wait is time the run spent on someone else, an interval of one kind, listed here in the order an overlap is credited to them:
// parent_review, a parent-review round from its request's opened_at to its verdict's recorded_at;
// pr_gate, a batten change of the child into awaiting at PR-Gate;
// escalation, a change into awaiting at any other row;
// halt, a change into blocked or failed.
// A change's wait runs from batten's entry for it to the next row of either history stamped after it.
// A batten-row halt is an entry at one of batten's own rows, never Run-Shed, that is an awaiting outcome or a stuck outcome no OnStuck edge bounced (its routed_to is empty); it is a wait of kind escalation when awaiting and halt otherwise, from its entry to the next row.
// A batten stuck entry written before routed_to existed has it empty even when an edge bounced it, so an older batten record reads such a bounce as a batten-row halt.
// A wait with no later row and no verdict runs to the span's close, which is Sources.SnapshotAt for a run in flight, so an open wait there runs to the snapshot with no finding; a parent-review round with no request opens no wait.
// Each wait is clipped to the span, sleep comes out first, and then each kind's union loses every earlier kind's, so overlapping waits count once under the kind listed first and a sleep inside a wait counts as sleep alone.
// Waits stay inside their phase's time; a run's active seconds are its span less sleep less every net wait, so active, the waits and sleep sum to the span.
//
// An attempt is one spawn: every arbiter attempt, every PR-Gate alignment attempt and every parent-review round's alignment record.
// Its time runs, net of sleep, from its spawned_at to the first row of either history stamped after it, or to the span's close in a run in flight.
// In a landed run an attempt spawned outside the span, or with no row after it, has its time absent and adds a finding naming its file; its spawn still counts.
// A figure's seconds are the summed time of the attempts whose time is known, nil when none is, and an attempt's time lies inside the wait it serves and is reported beside the waits, never added to them.
//
// Buckets are UTC days or ISO weeks from Since, none opening before it.
// Each interval, wait and sleep is clipped to every bucket it overlaps, and a bucket's time sums over runs, so two parallel runs add two hours to an hour.
// Each attempt spawn counts in the bucket its spawned_at falls in and its time is clipped to every bucket it overlaps.
// TimingReport.Phases holds each phase's median over the landed runs holding it whose archive tag is committed in the window; the median of an even count is the mean of the two middle values rounded down, and a run in flight never enters one.
//
// # Outputs
//
// The JSON envelope is the Report itself: selection, runs, partial, landings, weeks, absent and findings.
// RenderMarkdown renders the same fields as the selection and one table per board question: wakes per run, recurring stop and failure classes, the fresh-code fix share, and review rounds per segment.
// A nil figure renders as no data.
package reportengine
