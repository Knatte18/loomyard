// Package shedrecipe owns the engine registry:
// the mapping from engine name to shedengine.ShedProducer constructor.
// Each recipe row's Engine field resolves against it.
// It imports the packages that host the producers it constructs.
// None of them imports it, so the dependency runs one way only.
//
// An engine is handed the absolute paths it operates on and derives none of its own,
// so it runs identically inside a lym hub and in a bare directory that is not a git repository.
// The package's only path construction is joining a told root with a recipe-relative value.
//
// Env is the told bundle: the caller-filled roots and injected seams every entry may read from.
// Config is the portable per-row half: the recipe row's own static, already-decoded configuration,
// which never contains an absolute path and which this package never learns the file format of.
// An entry refuses at construction, naming the field, a relative path or a nil seam of the Env fields it reads.
// An unknown Config key is refused with the way forward.
//
// # The loom engine sets
//
// LoomRecordEngines, LoomBuildEngines and LoomFinalizeEngine name the engines of a loom recipe's rows.
// shedbuild reads them to check a recipe declaring `contract: loom`, so a new loom recipe needs no change to the check.
//
// # The Implementer row
//
// The Implementer entry is the one place a setting picks a row's engine.
// Its "implementer_setting" key names that setting as `module.key`, read through Env.Setting.
// The setting's value names one implementer from a closed table in this package.
// Every implementer builds the same row with the same edges, so the choice never skips a row or enters one conditionally.
// contracts/specs/implementer-spec.md pins what every implementer takes in and delivers.
//
// # The Block row
//
// The Block entry builds the row a recipe's block row becomes once its sub-recipe is expanded into it.
// Whether the block runs is the row's entry gate, built from a block gate check.
// BlockGates is the closed table of those checks, reached only through LookupBlockGate and BlockGateNames.
// A new check is a row of the table, never code per recipe.
//
// # The review segment rows
//
// The Bouncer, BurlerReview and BurlerFix rows take the optional keys "color_segment" and "role_prefix".
// A Constructor is handed its row's RowIdentity, the row's name and its recipe segment: value.
// Env.Segments holds one SegmentSettings per review segment, which the segment's three rows share.
// A review row with no segment, or a segment with no Env.Segments entry, is refused.
//
// A value that is not shared by a whole segment is a setting the row names in its Config.
// A key ending in "_setting" holds a `module.key` reference, split at the first dot and read through Env.Setting.
// Every setting a row names is read when the row is built,
// and a malformed reference, a nil Env.Setting, a lookup error or a wrong-shaped value refuses before any row runs.
// The lookup answers only template keys of registered config modules, a guard-category key included.
// Only rows of the recipes embedded in the binary name settings,
// so a key that a local layer cannot override is never read from one.
//
// # The Drift rows
//
// The DriftBaseline, DriftAwait, DriftExpect and DriftVerify entries build regression rows over Env.Drift.
// A caller leaves the Seats, Now and GateFindingsFile fields of Env.Drift unset; the entries fill them.
// None has a switch of its own: the recipe leaves a row out through its `entry_gate`,
// and each producer reads `drift.enabled` again at every pass, so a resume passes a row already entered.
//
// # The red-first gate
//
// The `red-first` gate holds the handoff of a run whose board entry is labelled `bug`.
// It holds until a test the diff adds or changes fails on the merge base.
// The parent may instead record a decision titled `No test seam`, for a bug no test can reach.
// A decision titled `No bug fixed` passes it too, for a change that only adds tests pinning existing behaviour.
// A hub whose scoper is not go passes it, since the check cannot judge the tests there.
// A diff that adds no test holds the run without spending an attempt.
// The hold lasts until a new commit or that record, so its entry is MayHold.
// An entry without the label passes at once.
//
// # The gate fixer element
//
// An element of a "gates" list may carry the optional key "fixer", a `<module>.<key>` setting read through Env.Setting.
// The setting holds a model-spec; an empty value turns the fixer off.
// Only the DarnWrite, PlanWrite and DiscussionWrite entries read it, and every other row refuses it as an unknown key.
// A row whose element names a fixer hands its finished gate spec to Env.GateFixer and runs the spec the seam returns.
// A nil Env.GateFixer leaves the row's gates as written.
package shedrecipe
