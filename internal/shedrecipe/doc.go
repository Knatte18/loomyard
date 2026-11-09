// Package shedrecipe owns the engine registry:
// the mapping from engine name to shedengine.ShedProducer constructor.
// Each recipe row's Engine field resolves against it.
// It imports the packages that host the producers it constructs.
// None of them imports it, so the dependency runs one way only.
//
// An engine is handed the absolute paths it operates on and derives none of its own,
// so it runs identically inside a lym hub and in a bare directory that is not a git repository.
// Every root this package touches is told, none is derived,
// and the package's only path construction is joining a told root with a recipe-relative value.
//
// Env is the told bundle: the caller-filled roots and injected seams every entry may read from.
// Config is the portable per-row half: the recipe row's own static, already-decoded configuration,
// which never contains an absolute path and which this package never learns the file format of.
//
// # The loom engine sets
//
// LoomRecordEngines, LoomBuildEngines and LoomFinalizeEngine name the engines of a loom recipe's rows:
// the record row, the build row and the landing row respectively.
// shedbuild reads them to check a recipe declaring `contract: loom`, so a new loom recipe needs no change to the check.
//
// # The MultiLLM row
//
// The MultiLLM entry builds a seatengine.Table from its row.
// The row names a role prefix, a loom segment and an ordered list of seats.
// It may also name gates, a timeout, interactivity and table tokens.
// Each seat names its stencil, its model spec and its worktree-relative inputs and outputs.
// A seat may also name skills and values.
// A seat's model spec is parsed and resolved against Env.Models at construction,
// so an unknown alias fails before any call.
// Table.Validate runs at construction against Env.StencilsDir.
// An unreadable stencil, an unresolvable include and a reserved or blank value fail there too.
// The producer drives Env.Seats, the seat runner the caller wires over its shuttle runner;
// the entry reads no other seam.
//
// # The DiscussionWrite row
//
// The DiscussionWrite entry runs the Discussion-Write lead on a MultiLLM producer.
// The producer sits behind the discussion commit decorator.
// The table's advisors, when it has any, run on the same producer.
// Its table is told rather than built from Config.
// Env.DiscussionTable is a closure the caller supplies, evaluated once per call;
// building the table needs a location this package may not import.
// The row's "gates" key is stamped onto each table the closure returns,
// so the gate guards the lead whatever table the closure builds.
//
// # The PartsWrite and PartsPlanWrite rows
//
// The PartsWrite and PartsPlanWrite entries run the two steps of a split plan.
// Each runs on a MultiLLM producer behind a commit decorator.
// PartsWrite takes the "gates" key and runs Env.PartsTable, the lead that splits the task into parts.
// It commits through Env.CommitParts.
// PartsPlanWrite takes two keys: "gates" guards its chair, and "part_gates" guards each part's writer seat.
// It runs Env.PartsPlanTable, handing it the per-part hooks as a PartSeats, and commits through Env.CommitPartsPlan.
// "part_gates" shares the element keys of "gates", and its one legal gate name is `plan-part`.
// The `parts` gate caps the parts file at the tapestry.parts_max setting.
//
// # The RecordWrite row
//
// The RecordWrite entry builds a loom recipe's record row for a run whose task is a board entry.
// Its producer is Go code that writes the decision record and support log with no agent.
// It takes no Config key and reads Env.RecordWrite alone.
// The row refuses when it is built unless every path is absolute and every seam non-nil.
//
// # The RaddleWrite and RaddleFile rows
//
// The RaddleWrite entry builds the raddle recipe's agent row, a seat-table producer behind loomshed.NewRaddleWrite.
// It takes the "gates" key alone and reads Env.RaddleWrite and Env.Seats.
// Its table source reads the run's recorded mode and return reason from the raddle state at each Call,
// and it stamps the resolved gate onto the table.
// The `raddle` gate runs the raddle proposal checks and stores the cold proposal on a pass.
// The RaddleFile entry builds the Go row that files the approved proposal on the board.
// It takes no Config key and reads Env.RaddleFile alone.
// Each refuses a relative path and a nil seam of its Env field when it is built.
//
// # The BeamUp, BeamAwait and BeamDown rows
//
// The BeamUp, BeamAwait and BeamDown entries build the pawl recipe's Go producers.
// They bring a beam up, hold its run until a land request passes the landing re-check, and take it down.
// Each takes no Config key and reads Env.Pawl alone.
// An entry refuses an unset Env.Pawl field its producer uses, naming it in its dotted `Pawl.<Field>` form.
// Watch alone may be nil.
// BeamAwait uses ReadyStands and StampWake beside its other seams.
//
// # The Implementer row
//
// The Implementer entry builds a recipe's implementer row with the engine a setting chooses.
// It is the one place a setting picks a row's engine.
// Its "implementer_setting" key names that setting as `module.key`, read through Env.Setting.
// The setting's value names one implementer from a closed table in this package.
// Every other key of the row's Config passes to that implementer's constructor unchanged;
// the shipped tapestry recipe names `tapestry.implementer`.
// Every implementer builds the same row with the same edges.
// The choice therefore never skips a row or enters one conditionally.
// A row without the key, and a setting holding anything but one implementer name, refuse when the recipe builds.
// contracts/specs/implementer-spec.md pins what every implementer takes in and delivers.
//
// # The Block row
//
// The Block entry builds the row a recipe's block row becomes once its sub-recipe is expanded into it.
// It takes no Config key and reads no Env field;
// its producer returns Done at once, so the row's Done marks the run entering the block.
// Whether the block runs is the row's entry gate, built from a block gate check.
// BlockGates is the closed table of those checks, reached only through LookupBlockGate and BlockGateNames.
// `always` holds with an empty reason.
// `undiscussed` requires and calls Env.SeedDiscussedRecord.
// It holds when the seam seeds nothing.
// When the seam seeded the entry's stored decision record, it passes over the block.
// Its reason then names the discussed board entry.
// A name the table lacks is refused with the names it holds.
// A new check is a row of the table, never code per recipe.
//
// # The PRReview row
//
// The PRReview entry builds the PR-Review row over a copy of the run's landing deps.
// The row takes no config key and carries no `entry_gate`.
// Whether it opens a pull request reads the parent's anchor, a board field and the origin;
// the row decides that in its own call.
// Every key is refused when the row is built, with the way forward in the message.
//
// # The SamplerSession row
//
// The SamplerSession entry builds the sampler recipe's build row.
// The row is one interactive agent session that works with the operator in the run's worktree.
// It takes no Config key and reads Env.Sampler alone.
// The row refuses when it is built unless the record paths are absolute and every seam but Watch is non-nil.
// It is a loom build engine, so a recipe whose build row runs it passes the loom contract.
//
// # The review segment rows
//
// The Bouncer, BurlerReview and BurlerFix rows each take the optional keys "color_segment" and "role_prefix".
// "color_segment" names the loom segment their sessions are colored by.
// "role_prefix" leads their sessions' roles, so reed's strand, window and bar names tell the segments apart.
// The roles are <prefix>-bouncer-seed, <prefix>-bouncer-judge, <prefix>-burler-review and <prefix>-burler-fix.
// A value agentname.ValidateRole rejects is refused when the row is built, before any spawn.
//
// The Bouncer also takes the optional key "subject", whose value is "document" or "code".
// The value selects the Env seams that tell it what its segment judges:
// "document" selects Env.DocumentSubject and Env.RecordHead, and "code" Env.CodeSubject and Env.MergeInOnly.
// A named seam the Env does not carry, and a value outside the two, are refused when the row is built;
// an absent key tells no subject.
// BurlerReview renders the amendment note of its run directory into the rubric's amendment marker.
// BurlerReview and the Bouncer hand Env.RedundancyReport to the rubric's redundancy_report marker.
// The rubric reads it only when it carries the marker.
//
// # The Drift rows
//
// The DriftBaseline, DriftAwait, DriftExpect and DriftVerify entries build regression rows.
// The tapestry, darn and sampler recipes use them.
// Each reads Env.Drift, a whole-struct passthrough to the producers.
// Each fills its Seats and Now from Env.Seats and Env.Now.
// DriftExpect and DriftVerify also fill their GateFindingsFile from Env.GateFindingsFile.
// A caller leaves those three fields of Env.Drift unset.
// Each checks the Env.Drift seams its producer reads, and Env.Seats for the agent rows.
// It refuses a nil one, naming the field.
// None has a switch of its own.
// The recipe leaves a row out through its `entry_gate`.
// That gate is `drift.enabled` in tapestry and darn, and `drift.enabled` with `drift.scope == full` in the sampler.
// Each producer reads `drift.enabled` again at every pass,
// so a resume passes a row already entered once the switch went off.
// DriftBaseline takes a required "subject", `decision-record` or `board-entry`.
// DriftAwait takes no key.
// DriftVerify takes a required "subject" and a "gates" list of exactly one entry, named `judgement`.
// The judge's table runs that gate at each turn end, and the entry refuses "gate_key".
// The Drift-Verify and Drift-Recheck rows both build it.
// DriftExpect takes a required "subject" and a "gates" list of exactly one entry, named `expectation`.
// The agent's table runs that gate at each turn end.
// An unknown key, a missing or unknown subject and a gates list of another shape are refused naming the way forward.
// The `drift` gate, which any gated row may list, runs the drift check over Env.Drift.
// It requires the Env.Drift seams Config, Status, CaptureCurrent, TrailerCommit, FileAt and Compare.
// It refuses a nil one, naming the field.
// The `expectation` gate, which DriftExpect's row lists, checks expected.yaml over Env.Drift.
// It requires the Config, TrailerCommit and FileAt seams.
// The `judgement` gate checks judgement.yaml against the judge request over Env.Drift's Paths.
// The judge's table runs it.
// It requires no seam.
// The shipped darn recipe lists `drift` on Darn-Write after `verify`,
// and the tapestry recipe lists it on Webster, whose run-level gate carries it.
//
// # The gate fixer element
//
// An element of a "gates" list may carry the optional key "fixer", a `<module>.<key>` setting read through Env.Setting.
// The setting holds a model-spec; an empty value turns the fixer off.
// Only the DarnWrite, PlanWrite and DiscussionWrite entries read it, and every other row refuses it as an unknown key.
// At most one element of a list carries it.
// A malformed reference, an unreadable setting and an unparsable value refuse when the row is built.
// A row whose element names a fixer hands its finished gate spec to Env.GateFixer in a GateFixerRequest,
// and it runs the spec the seam returns.
// A nil Env.GateFixer, and an empty setting value, leave the row's gates as written.
//
// # Segment settings and row settings
//
// A Constructor is handed its row's RowIdentity, the row's name and its recipe segment: value.
// Env.Segments holds one SegmentSettings per review segment, keyed by that segment value.
// A segment's Bouncer, BurlerReview and BurlerFix rows share one entry.
// A segment carries the values its rows resolve alike.
// These are the reviewer and fixer round models and the model keys they came from.
// They also include the Bouncer's seed and judge pair, the hard bounce cap and the amendment allowance.
// Each of those entries refuses a row with no segment and a segment with no Env.Segments entry.
// The refusal names the entry, the row and the segment.
//
// A value that is not shared by a whole segment is a setting the row names in its Config.
// A key ending in "_setting" holds a `module.key` reference, split at the first dot.
// The entry reads it through Env.Setting, the told lookup answering a registered config module's resolved value.
// The Bouncer reads reviewer_fan_setting as its per-round reviewer fan list.
// It reads auto_accept_setting as its auto-accept flag.
// BurlerReview reads reviewer_fan_setting and fixer_fan_setting, which become the fan lists its rounds record.
// BurlerFix reads fixer_fan_setting.
// Naming it makes the row require the round's recorded fan names.
// A refusal cites the reference as `<key> in <module>.yaml`.
//
// A fan value is a fan name or a list of fan names;
// a null, an empty string and a list of empty names count as no fan, and any other shape is refused.
// An auto-accept value is a bool, and a null is false.
// Every setting a row names is read when the row is built.
// A malformed reference, a nil Env.Setting, a lookup error or a wrong-shaped value each refuses before any row runs.
//
// The lookup is read-only and answers only template keys of registered config modules, a guard-category key included.
// Only rows of the recipes embedded in the binary name settings,
// so a key that a local layer cannot override is never read from one.
package shedrecipe
