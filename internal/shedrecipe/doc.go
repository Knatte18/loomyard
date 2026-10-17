// Package shedrecipe owns the engine registry: the name to shedengine.ShedProducer-constructor
// mapping a future recipe loader resolves each row's Engine field against.
// It is imported by nobody in the four producer-hosting packages it imports (internal/shedengine,
// internal/shedadapters, internal/websterengine, internal/landingshed), so the dependency runs one
// way only.
//
// This package holds PATTERN-told-geometry in its precise form: an
// engine is handed the absolute paths it operates on and derives none of its own, so it runs
// identically inside a lyx hub and in a bare directory that is not a git repository.
// Every root this package touches is told, none is derived, and the package's only path
// construction is joining a told root with a recipe-relative value.
//
// Env is the told bundle: the caller-filled roots and injected seams every entry may read from.
// Config is the portable per-row half: the recipe row's own static, already-decoded configuration,
// which never contains an absolute path and which this package never learns the file format of.
//
// # The MultiLLM row
//
// The MultiLLM entry builds a seatengine.Table from its row: a role prefix, a loom segment, an ordered list of seats, and optional gates, timeout, interactivity and table tokens.
// Each seat names its stencil, its model spec and its worktree-relative inputs and outputs, and optionally its skills and values.
// A seat's model spec is parsed and resolved against Env.Models at construction, so an unknown alias fails before any call.
// Table.Validate runs at construction against Env.StencilsDir, so an unreadable stencil, an unresolvable include and a reserved or blank value fail there too.
// The producer drives Env.Seats, the seat runner the caller wires over its shuttle runner; the entry reads no other seam.
//
// # The DiscussionWrite row
//
// The DiscussionWrite entry runs the Discussion-Write lead, and its advisors when the table has any, on a MultiLLM producer behind the discussion commit decorator.
// Its table is told rather than built from Config: Env.DiscussionTable is a closure the caller supplies, evaluated once per call, because building the table needs a location this package may not import.
// The row's "gates" key is stamped onto each table the closure returns, so the gate guards the lead whatever table the closure builds.
// When the gates hold an enabled "parent-review" entry, the producer's fresh-spawn preparation opens the next review round before any stale output is archived and before any seat starts; a resumed live lead continues the latest round.
//
// # The review segment rows
//
// The Bouncer, BurlerReview and BurlerFix rows each take the optional keys "color_segment" and "role_prefix".
// "color_segment" names the loom segment their sessions are colored by.
// "role_prefix" leads their sessions' roles, as <prefix>-bouncer-seed, <prefix>-bouncer-judge, <prefix>-burler-review and <prefix>-burler-fix, so reed's strand, window and bar names tell the segments apart;
// a value agentname.ValidateRole rejects is refused when the row is built, before any spawn.
//
// # Segment settings and row settings
//
// A Constructor is handed its row's RowIdentity, the row's name and its recipe segment: value.
// Env.Segments holds one SegmentSettings per review segment, keyed by that segment value, so a segment's Bouncer, BurlerReview and BurlerFix rows share one entry.
// A segment carries the values a segment's rows resolve alike: the reviewer and fixer round models, the model keys they came from, the Bouncer's seed and judge pair and the hard bounce cap.
// Each of the three entries refuses a row with no segment, and a segment with no Env.Segments entry, naming the entry, the row and the segment.
//
// A value that is not shared by a whole segment is a setting the row names in its Config.
// A key ending in "_setting" holds a `module.key` reference, split at the first dot, and the entry reads it through Env.Setting, the told lookup answering a registered config module's resolved value.
// The Bouncer reads reviewer_fan_setting as its per-round reviewer fan list and auto_accept_setting as its auto-accept flag.
// BurlerReview reads reviewer_fan_setting and fixer_fan_setting, which become the fan lists its rounds record.
// BurlerFix reads fixer_fan_setting; naming it makes the row require the round's recorded fan names, and the reference, as `<key> in <module>.yaml`, is the source a refusal cites.
//
// A fan value is a fan name or a list of fan names; a null, an empty string and a list of empty names count as no fan, and any other shape is refused.
// An auto-accept value is a bool, and a null is false.
// Every setting a row names is read when the row is built, so a malformed reference, a nil Env.Setting, a lookup error and a value of the wrong shape each refuse before any row runs.
//
// The lookup is read-only and answers only template keys of registered config modules, a guard-category key included.
// Only rows of the recipes embedded in the binary name settings, so a key that a local layer cannot override is never read from one.
package shedrecipe
