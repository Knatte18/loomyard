// spec.go declares the arming contract every verb body reads: Spec (the resolution-dependent
// values an arming module's PersistentPreRunE fills), Hooks (the module-specific extension points a
// verb body calls at fixed points in its own sequence), VerbTexts (the build-time help text cobra
// needs before any pre-run has run), and AbsentDisposition (status's told behaviour over a status
// file that does not exist).

package shedverbs

import (
	"context"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// AbsentDisposition tells the generic status body what to report when its status file does not
// exist. Refuse true means status reports RefuseMessage on the error envelope -- loom's case, where
// a never-bootstrapped task is a refusal naming the bootstrap remedy. Refuse false means status
// reports "found: false" plus "status_path" on the success envelope instead -- lifecycle's case,
// where a slug that has never run on this machine is a determined answer, not an error.
type AbsentDisposition struct {
	// Refuse selects which of the two absent-file treatments status applies.
	Refuse bool
	// RefuseMessage is the error envelope's message when Refuse is true. It is ignored when Refuse
	// is false.
	RefuseMessage string
}

// Hooks carries the module-specific extension points the generic verb bodies call at fixed
// points in their own sequence. Every field is nil-by-default and skipped when nil, so a module
// that needs none of them may leave the whole struct at its zero value.
type Hooks struct {
	// PreRun runs at the very start of run's RunE, before BuildShed and before shed.Run. A non-nil
	// error here reports on the error envelope and the run never starts. PreRun returns no envelope
	// map of its own: neither shipped filler produces one, and a second extras source would need a
	// precedence rule against PostRun's for no present benefit.
	PreRun func(ctx context.Context) error
	// PostRun runs unconditionally after shed.Run returns, including when runErr is non-nil, and
	// before either envelope is written. This placement is load-bearing, not incidental: it is what
	// preserves loom's own detectAndFileAnomalies call, which must see every hard-error return --
	// dropping this arm would lose a whole failure class, including the in-memory crash observation
	// a resumed run can never recover once this process exits.
	// PostRun's returned map is merged onto the success envelope, and onto the error envelope when runErr is non-nil, through output.ErrFields.
	// A module that wants its error envelope unchanged returns nil when it is handed a non-nil runErr.
	PostRun func(ctx context.Context, result shedengine.Result, runErr error) map[string]any
	// PreStep runs at the very start of step's RunE, before BuildShed and before shed.Step. A
	// non-nil error reports on the error envelope with the hook's own returned kind on the
	// envelope's "kind" field, and BuildShed is never called.
	PreStep func(ctx context.Context) (kind string, err error)
	// PostStep runs after a successful shed.Step and before the envelope is written -- never on an
	// error path, since the voucher it exists for records a clean handoff and a step that failed
	// produced none. It exists so loom can keep calling recordHandoffVoucher at exactly that point: a
	// completed step's persisted aftermath is byte-identical to a mid-run driver death, and that
	// voucher is the one thing letting the next run's entry observation tell the two apart -- so its
	// call site can move neither above the Step call nor below the envelope. It is filled by loom
	// and left nil by lifecycle.
	PostStep func(res shedengine.StepResult)
	// AfterStep runs once shed.Step has returned, on every path:
	// on success after PostStep, and on every shed.Step error before the refusal envelope is written.
	// The busy error is passed through, so the hook applies its own busy skip, as PostRun does.
	// It is not called when PreStep or BuildShed refuses, since no step ran,
	// and it runs after the producer call and the lock release, never before a step.
	// Its return is the envelope's friction key on the success envelope and on the error envelope of a shed.Step error alike;
	// a nil hook yields the empty string.
	AfterStep func(ctx context.Context, res shedengine.StepResult, stepErr error) string
	// InterruptPolicyFor resolves the interrupt policy for a producer name, keyed by the step
	// body's own res.Next. A nil hook yields the empty string, which is the caller's "no entry"
	// signal and never a third policy word -- a recipe with no policy table therefore yields an
	// empty next_interrupt_policy.
	InterruptPolicyFor func(row string) string
	// StatusExtras lets a module add its own keys onto status's ten-key generic core
	// (current_producer, state, error, activity, history_length, interrupt_policy, trace_dir,
	// run_id, progress, last_step),
	// keyed by the decoded shedengine.Status. A non-nil error it returns is reported verbatim on
	// the error envelope with no re-prefixing -- the hook owns its whole string. StatusExtras
	// never runs against a zero shedengine.Status: the absent-file envelope carries only the
	// generic found/trace_dir (plus status_path on the non-refusing disposition), never an extras
	// key.
	StatusExtras func(st shedengine.Status) (map[string]any, error)
	// PreGoto runs in goto's RunE after the --to check and before shedengine.Goto.
	// A non-nil error is reported verbatim on the error envelope and the status file is not touched.
	PreGoto func(ctx context.Context, target string) error
	// Waiting names, in one line, what a running run is waiting on; empty means it waits on nothing.
	// It receives the status file's content,
	// so a product can name the current producer.
	// status calls it only when the status file's state is running,
	// and a non-empty note adds a waiting key to the envelope, replaces the plain running state in the human header and extends the --watch line.
	// A non-nil error is reported verbatim on the error envelope, like StatusExtras;
	// the --watch line drops the note on an error rather than ending the tail.
	// The hook closes over whatever location the arming module was told, so shedverbs stays path-free.
	Waiting func(st shedengine.Status) (string, error)
}

// VerbTexts carries each verb's cobra Use/Short/Long strings, passed by value at Verbs'
// construction time. It travels separately from Spec because cobra builds the whole command tree
// before any PersistentPreRunE runs, so help text must exist at construction time while resolved
// paths and hooks cannot -- see the build-time-texts-run-time-spec Shared Decision.
type VerbTexts struct {
	Run    VerbText
	Step   VerbText
	Status VerbText
	Pause  VerbText
	Goto   VerbText
}

// VerbText is one verb's cobra Use/Short/Long triple and the audience annotation its mount gives it.
type VerbText struct {
	Use   string
	Short string
	Long  string
	// Audience is the clihelp audience the verb's command carries under this mount.
	Audience string
}

// Spec carries every resolution-dependent value the four generic verb bodies read, told rather
// than derived: it is filled in place by the arming module's own PersistentPreRunE, after cobra has
// already built the whole command tree from a VerbTexts value.
type Spec struct {
	// StatusPath is the durable status file this spec's shed reads and writes.
	StatusPath string
	// LockPath is the run lock shedengine.Shed itself acquires around Run and Step.
	LockPath string
	// StatusLockPath is the advisory lock internal/state takes around every status-file read and
	// write.
	StatusLockPath string
	// RunID is the resolved run-id the arming module addressed, reported as run_id on the status and
	// every step envelope. The empty string means the arming module supplied none.
	RunID string
	// MissingStatusWayForward is the told trailing "way forward:" clause for a missing status file, passed to goto.
	// The empty string falls back to shedengine's generic clause.
	MissingStatusWayForward string
	// Routing is the told projection of the recipe's producer graph that the envelopes' progress key
	// is derived from. The zero value (no producers) means no progress is known, and progress is
	// then reported as null.
	Routing shedengine.Routing
	// ScratchDir is the run's ephemeral shed scratch directory (shedrun.ScratchDir of the addressed
	// run), reported on every step envelope as scratch_dir.
	// It is never filled from shedrun.RunDir, the run's durable tracked directory, because driver
	// records written under it must never land in tracked content.
	// The empty string means the arming module supplied none.
	ScratchDir string
	// StepsDir is the ephemeral directory step keeps its per-invocation records in (shedrun.StepsDir
	// of the addressed run): an in-flight record before the producer call and the printed envelope
	// after it, and status reads the newest one as last_step.
	// It is told, never derived here; the empty string means no records are kept.
	StepsDir string
	// FrictionDir is the recipe's own agent friction-note directory, reported as friction_dir.
	// The empty string means the recipe has none or friction is off.
	FrictionDir string
	// BuildShed constructs the *shedengine.Shed run and step call, built at arming time rather than
	// carried as an already-built value: loomcli's own pre-flight assigns c.env.Landing on its way
	// through, immediately before loomrecipe.New, so a Shed built earlier than that assignment would
	// carry a nil Landing and the landing rows would fail deep in the run. A nil BuildShed is legal
	// for status and pause, which never call it, and an error for run and step.
	BuildShed func() (*shedengine.Shed, error)
	// Hooks carries the module-specific extension points; see Hooks' own field docs.
	Hooks Hooks
	// StatusLabel is the --watch line's literal prefix -- "loom" for the loom recipe.
	StatusLabel string
	// DecodeErrPrefix is the told prefix for this spec's own state.ReadJSONStrict failure on the
	// status file -- "loom:" for loom and "lifecyclecli:" for lifecycle. It also seeds
	// ensureStatusLockDir's own error text, reproducing loomcli's existing wording for loom's spec
	// rather than inventing a second prefix field for one string.
	DecodeErrPrefix string
	// RunBusyMessage is the told text run reports in place of err.Error() when shed.Run returns an
	// error wrapping shedengine.ErrShedBusy. The empty string means passthrough: run reports
	// err.Error() verbatim.
	RunBusyMessage string
	// StepBusyMessage is RunBusyMessage's sibling for step's own shed.Step call. The empty string
	// means the same passthrough.
	StepBusyMessage string
	// StepBusyKind is the refusal kind step maps shedengine.ErrShedBusy onto, reported on the
	// envelope's "kind" field.
	StepBusyKind string
	// AbsentStatus tells status what to report when its status file does not exist; see
	// AbsentDisposition's own field docs.
	AbsentStatus AbsentDisposition
	// PauseAbsentMessage is the error text pause reports when its status file does not exist --
	// there is nothing running to pause.
	PauseAbsentMessage string
	// EnsureStatusLockDir tells status and pause whether to MkdirAll the status lock's ephemeral
	// parent directory before reading or writing through it. It travels as a told boolean rather
	// than an unconditional generic step because lifecycle's status is read-only, and creating its
	// per-slug directory as a side effect of reading it would be a new, unasked-for write.
	EnsureStatusLockDir bool
}
