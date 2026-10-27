// Package shedrecipe owns the engine registry: the name to shedengine.ShedProducer-constructor
// mapping a future recipe loader resolves each row's Engine field against.
// It is imported by nobody in the four producer-hosting packages it imports (internal/shedengine,
// internal/shedadapters, internal/websterengine, internal/landingshed), so the dependency runs one
// way only.
//
// This package holds PATTERN-cwd-resolution in its precise form: an
// engine is handed the absolute paths it operates on and derives none of its own, so it runs
// identically inside a lym hub and in a bare directory that is not a git repository.
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
//
// # The BeamDescribe row
//
// The BeamDescribe entry builds the beam recipe's description row, a Go producer that writes a beam's change description from its board entry and its commits with no agent.
// It takes no Config key, and reads Env.BeamDescribe alone, whose every field is required; a zero field refuses when the row is built.
//
// # The BeamUp, BeamAwait and BeamDown rows
//
// The BeamUp, BeamAwait and BeamDown entries build the pawl recipe's Go producers that bring a beam up, hold its run until a land request passes the landing re-check, and take it down.
// Each takes no Config key and reads Env.Pawl alone.
// An entry refuses a field of Env.Pawl its producer uses and the wiring left unset, naming it in its dotted `Pawl.<Field>` form; Watch alone may be nil.
//
// # The Implementer row
//
// The Implementer entry builds a recipe's implementer row with the engine a setting chooses, the one place a setting picks a row's engine.
// Its "implementer_setting" key names that setting as `module.key`, read through Env.Setting, and its value names one implementer from a closed table in this package.
// Every other key of the row's Config passes to that implementer's constructor unchanged.
// Every implementer builds the same row with the same edges, so the choice never skips a row or enters one conditionally;
// a row without the key, and a setting holding anything but one implementer name, refuse when the recipe builds.
// contracts/specs/implementer-spec.md pins what every implementer takes in and delivers.
//
// # The PRReview row
//
// The PRReview entry builds the PR-Review stub, the seat the human pull-request review will take over.
// Its one required key, "pr_review_setting", is a `module.key` reference to a bool setting that says whether the stub logs on each call; every other key is refused.
// A missing key and a setting that is not a bool each refuse when the row is built, with the way forward in the message.
//
// # The review segment rows
//
// The Bouncer, BurlerReview and BurlerFix rows each take the optional keys "color_segment" and "role_prefix".
// "color_segment" names the loom segment their sessions are colored by.
// "role_prefix" leads their sessions' roles, as <prefix>-bouncer-seed, <prefix>-bouncer-judge, <prefix>-burler-review and <prefix>-burler-fix, so reed's strand, window and bar names tell the segments apart;
// a value agentname.ValidateRole rejects is refused when the row is built, before any spawn.
//
// The Bouncer also takes the optional key "subject", "document" or "code", which selects the Env seams that tell it what its segment judges:
// "document" selects Env.DocumentSubject and Env.RecordHead, and "code" Env.CodeSubject and Env.MergeInOnly.
// A named seam the Env does not carry, and a value outside the two, are refused when the row is built; an absent key tells no subject.
// BurlerReview renders the amendment note of its run directory into the rubric's amendment marker.
//
// # The Drift rows
//
// The DriftBaseline, DriftAwait and DriftVerify entries build the three regression rows of the loom and darn recipes.
// Each reads Env.Drift, a whole-struct passthrough to the producers, and fills its Seats and Now from Env.Seats and Env.Now, and DriftVerify its GateFindingsFile from Env.GateFindingsFile for the row, so a caller leaves those three unset.
// Each checks the Env.Drift seams its producer reads, and Env.Seats for the two agent rows, and refuses a nil one naming the field.
// None has a switch of its own: the recipe leaves a row out through its `entry_gate: drift.enabled`.
// DriftBaseline takes a required "subject", `decision-record` or `board-entry`, and an optional "capture", `detach` (the default) or `wait`.
// DriftAwait takes no key.
// DriftVerify takes a required "subject", the optional "gate_key" and a "gates" list of exactly one entry, named `verify`, which the agent's table runs once the session committed code.
// An unknown key, a missing or unknown subject, a capture outside the two and a gates list of another shape are refused naming the way forward.
//
// # Segment settings and row settings
//
// A Constructor is handed its row's RowIdentity, the row's name and its recipe segment: value.
// Env.Segments holds one SegmentSettings per review segment, keyed by that segment value, so a segment's Bouncer, BurlerReview and BurlerFix rows share one entry.
// A segment carries the values a segment's rows resolve alike: the reviewer and fixer round models, the model keys they came from, the Bouncer's seed and judge pair, the hard bounce cap and the amendment allowance.
// Each of the three entries refuses a row with no segment, and a segment with no Env.Segments entry, naming the entry, the row and the segment.
//
// A value that is not shared by a whole segment is a setting the row names in its Config.
// A key ending in "_setting" holds a `module.key` reference, split at the first dot, and the entry reads it through Env.Setting, the told lookup answering a registered config module's resolved value.
// The Bouncer reads reviewer_fan_setting as its per-round reviewer fan list and auto_accept_setting as its auto-accept flag.
// BurlerReview reads reviewer_fan_setting and fixer_fan_setting, which become the fan lists its rounds record.
// BurlerFix reads fixer_fan_setting; naming it makes the row require the round's recorded fan names, and the reference, as `<key> in <module>.yaml`, is the source a refusal cites.
//
// A fan value is a fan name or a list of fan names; a null, an empty string and a list of empty names count as no fan, and any other shape is refused.
// An auto-accept value, like the PRReview flag, is a bool, and a null is false.
// Every setting a row names is read when the row is built, so a malformed reference, a nil Env.Setting, a lookup error and a value of the wrong shape each refuse before any row runs.
//
// The lookup is read-only and answers only template keys of registered config modules, a guard-category key included.
// Only rows of the recipes embedded in the binary name settings, so a key that a local layer cannot override is never read from one.
package shedrecipe
