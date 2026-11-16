// Package runplan holds the run-plan state and the batch machinery an implementer runs a plan's batches with.
//
// The run-plan state holds the plan fingerprint, the plan file hashes and the overview frame hash.
// It also holds the recorded partition and the batch records.
// An implementer's own state type embeds State beside its own fields, so the run-plan part keeps one schema.
// A batch-record field only one implementer fills is optional and stays empty under another.
//
// The machinery is the batch report contract and its parser, and the plan fingerprint and its re-baseline.
// It also covers the plan re-resolution at dispatch and its plan-conflict record.
// The post-batch checks cover the done checks, handle binding, the scope guard and drift repair.
// They also cover the renames that rewrite pending cards.
// The suspect-path checks, the batch digest and the card gate rendering belong to it as well.
// So do the bounded wait for a batch report, the recovery strand stop and the verify-failure record.
//
// Every piece is told its paths through Geometry: the implementer's run, reports, prompts and scratch directories,
// and the scratch directory its state-mutation lease sits in.
// Two texts keep webster's names because readers pin them:
// the begin-batch header of the plan-conflict record and the handle-bind log line.
// Nothing else here names an implementer or its verbs.
// A refusal that ends in a way forward takes that text from its caller,
// so each implementer names its own way forward.
package runplan
