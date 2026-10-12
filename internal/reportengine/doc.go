// Package reportengine computes the read-only report behind `lyx report convergence`: per finished run and per ISO week, what woke the orch, what stopped and repaired runs, and how the parent branch's landings fared.
// Every figure comes from records lyx already writes, each read from git objects through a tolerant parse in the package that owns its format.
// A record a run lacks is reported absent, never as zero.
//
// # Told geometry
//
// The package never imports internal/lyxcwd and derives no path of its own.
// Geometry carries the absolute checkout paths of the code, records and board repositories and their three branch names, and the engine writes nothing, takes no lock and makes no commit.
// It names no fabric side and no run-record directory: record paths come from shedrun and the record packages, and the archive tag namespace and task-commit trailer come through fabricengine's neutral exports.
//
// # One ref snapshot
//
// Open resolves each branch tip and lists the archive tags once.
// Every later read names those tips and tags, so a report sees one history while runs land concurrently.
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
package reportengine
