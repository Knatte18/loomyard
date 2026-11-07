// gatefixer.go wires Env.GateFixer for the Darn-Write, Plan-Write and Discussion-Write rows:
// the seam that wraps a row's gate with a cold fixer; the visit directory that keys a row's fixer records;
// and the launch of each fixer's prompt, model and skills.

package loomcli

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/gatefixer"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
)

const (
	// gateFixerDirName is the directory under a run's scratch directory that holds the row fixers' records.
	gateFixerDirName = "gatefix"
	// darnWriteGateKey is the gate.yaml key a Darn-Write row's verify gate runs, and so the call its fixer re-runs.
	darnWriteGateKey = "darn-write"
	// Plan self-checks a Plan-Write fixer re-runs; the lens-review variant serves a gate that also demands it.
	planCheckCommand           = "lym loom validate-plan"
	planLensReviewCheckCommand = "lym loom validate-plan --lens-review"
	// discussionCheckCommand is the discussion self-check a Discussion-Write fixer re-runs.
	discussionCheckCommand = "lym loom validate-discussion"
	// discussionWriteEngine is the engine of the Discussion-Write row, whose discussion gate element carries a fixer.
	discussionWriteEngine = "DiscussionWrite"
	// Gate elements that carry a fixer: darnGateEntry, planGateEntry and discussionGateEntry.
	darnGateEntry       = "verify"
	planGateEntry       = "plan"
	discussionGateEntry = "discussion"
)

// gateFixerInputs is every told value the row fixers' wrap and launch read.
// wire gathers them, so the seam opens nothing itself.
type gateFixerInputs struct {
	// pairSlug is the worktree's pair slug, empty on the prime and outside a hub, where no fixer runs.
	pairSlug string
	// runProcDir is the hub's directory of run process owner records.
	runProcDir string

	starter    gatefixer.Starter
	stopper    gatefixer.Stopper
	reed       shuttleengine.ReedOps
	engines    gatefixer.Engines
	shuttleCfg shuttleengine.Config
	anchorRoot string

	registry modelspec.Registry
	gate     gateslot.Config

	stencilsDir    string
	repoRoot       string
	languageSkills []string
	parentName     string
	frictionDir    string

	// scratchDir, statusPath and statusLockPath locate the run's scratch directory and status file, which key a row visit.
	scratchDir     string
	statusPath     string
	statusLockPath string

	// patternRecordPath is the record whose declared PATTERN changes the prompt renders, empty for a run that declares none.
	patternRecordPath string

	// The paths below are the ones the body stencils name.
	worktreeRoot       string
	decisionRecordPath string
	supportLogPath     string
	relevantPath       string
	planDir            string
	lensReviewPath     string
}

// gateFixerBody is one fixer body: the stencil joined behind the shared frame.
// It also holds the segment color of the session and the function that fills the markers.
type gateFixerBody struct {
	stencil string
	segment segmentcolor.Segment
	values  func(in gateFixerInputs, req shedrecipe.GateFixerRequest) (map[string]string, error)
}

// gateFixerBodies maps the gate entry that carries a fixer to its body.
var gateFixerBodies = map[string]gateFixerBody{
	darnGateEntry:       {"gatefix-body-darn", segmentcolor.Darn, darnFixerValues},
	planGateEntry:       {"gatefix-body-plan", segmentcolor.Plan, planFixerValues},
	discussionGateEntry: {"gatefix-body-discussion", segmentcolor.Discussion, discussionFixerValues},
}

func darnFixerValues(in gateFixerInputs, _ shedrecipe.GateFixerRequest) (map[string]string, error) {
	fixGate, err := in.gate.AgentGateCall(darnWriteGateKey)
	if err != nil {
		return nil, fmt.Errorf("loomcli: render the Darn-Write fixer's gate call: %w", err)
	}
	return map[string]string{
		"fix_gate":             fixGate,
		"worktree_root":        in.worktreeRoot,
		"decision_record_path": in.decisionRecordPath,
	}, nil
}

func planFixerValues(in gateFixerInputs, req shedrecipe.GateFixerRequest) (map[string]string, error) {
	recheck := planCheckCommand
	if req.Forked {
		recheck = planLensReviewCheckCommand
	}
	return map[string]string{
		"plan_dir":         in.planDir,
		"lens_review_path": in.lensReviewPath,
		"recheck_command":  recheck,
	}, nil
}

func discussionFixerValues(in gateFixerInputs, _ shedrecipe.GateFixerRequest) (map[string]string, error) {
	return map[string]string{
		"decision_record_path": in.decisionRecordPath,
		"support_log_path":     in.supportLogPath,
		"relevant_path":        in.relevantPath,
		"recheck_command":      discussionCheckCommand,
	}, nil
}

// wallClock is the real time source of a fixer's wait.
type wallClock struct{}

func (wallClock) Now() time.Time        { return time.Now() }
func (wallClock) Sleep(d time.Duration) { time.Sleep(d) }

// pairSlugOf returns location's worktree name when the fabric origin record names it.
// It returns "" on the prime and outside a hub, and an error when the record cannot be read.
func pairSlugOf(location *lymcwd.Location) (string, error) {
	_, found, err := fabricengine.ReadOrigin(location)
	if err != nil {
		return "", fmt.Errorf("loomcli: read the fabric origin record: %w", err)
	}
	if !found {
		return "", nil
	}
	return location.WorktreeName, nil
}

// writerFixerOn reports whether a cold fixer clears the gate element gate of the row running engine in recipe.
// When it does, the writer's prompt drops the validator loop.
// It holds when the worktree has a pair slug and the element names a fixer setting that holds a non-empty value.
// A setting that cannot be read, or that holds a non-string, is returned as an error.
func writerFixerOn(recipe, engine, gate string, setting func(module, key string) (any, error), pairSlug string) (bool, error) {
	if pairSlug == "" {
		return false, nil
	}
	reference, err := loomrecipe.GateFixerSetting(recipe, engine, gate)
	if err != nil {
		return false, err
	}
	if reference == "" {
		return false, nil
	}
	module, key, dotted := strings.Cut(reference, ".")
	if !dotted || module == "" || key == "" {
		return false, fmt.Errorf("loomcli: the %s gate fixer names %q, which is not a <module>.<key> setting", gate, reference)
	}
	value, err := setting(module, key)
	if err != nil {
		return false, fmt.Errorf("loomcli: read the %s gate fixer setting %s: %w", gate, reference, err)
	}
	text, isString := value.(string)
	if !isString {
		return false, fmt.Errorf("loomcli: the %s gate fixer setting %s holds %v, want a model-spec string", gate, reference, value)
	}
	return text != "", nil
}

// rowVisitDir returns the directory the fixer records of one visit to row live in:
// `<scratchDir>/gatefix/<row>-<history length>-<newest entry's Repeats>`.
// A call that ends appends a history entry or folds into the newest,
// and a goto appends one.
// A crash re-step, and a resume that appended none, keep the directory;
// a goto, a resume of a halt that appended an entry, and a fresh entry into the row get a new one.
// A missing status file is an error naming its path.
func rowVisitDir(scratchDir, statusPath, statusLockPath, row string) (string, error) {
	status, found, err := state.ReadJSON[shedengine.Status](statusPath, statusLockPath)
	if err != nil {
		return "", fmt.Errorf("loomcli: read the run status %s to key the %s fixer directory: %w", statusPath, row, err)
	}
	if !found {
		return "", fmt.Errorf("loomcli: the run status %s is missing, so the %s fixer directory cannot be keyed; way forward: step the run again once its status file exists", statusPath, row)
	}
	repeats := 0
	if n := len(status.History); n > 0 {
		repeats = status.History[n-1].Repeats
	}
	return filepath.Join(scratchDir, gateFixerDirName, fmt.Sprintf("%s-%d-%d", row, len(status.History), repeats)), nil
}

// gateFixerSeam returns the Env.GateFixer seam over in.
// With no pair slug it returns the request's spec unchanged:
// a row on the prime or outside a hub spawns no fixer, and its gate failure reaches the writer.
// Otherwise it resolves the request's model-spec through the registry.
// It returns the spec wrapped so a failure at the entry goes to a cold fixer.
func gateFixerSeam(in gateFixerInputs) func(shedrecipe.GateFixerRequest) (shuttleengine.GateSpec, error) {
	return func(req shedrecipe.GateFixerRequest) (shuttleengine.GateSpec, error) {
		if in.pairSlug == "" {
			return req.Spec, nil
		}
		resolved, err := in.registry.Resolve(req.Model)
		if err != nil {
			return nil, fmt.Errorf("loomcli: resolve the %s gate fixer's model-spec: %w; way forward: set the setting that names it to an alias models.yaml holds with lym config, or to \"\" to turn the fixer off", req.Row, err)
		}
		choice := shuttleengine.NewModelChoice(resolved)
		spec, err := fixerSpec(in, req, choice)
		if err != nil {
			return nil, err
		}
		return gatefixer.WrapRowGate(req.Spec, req.Entry, gatefixer.RowFixer{
			Fixer: gatefixer.NewStrandFixer(gatefixer.Deps{
				Starter:    in.starter,
				Stopper:    in.stopper,
				Reed:       in.reed,
				Engines:    in.engines,
				ShuttleCfg: in.shuttleCfg,
				AnchorRoot: in.anchorRoot,
				Clock:      wallClock{},
				Register:   gatefixer.Registrar(in.reed, in.runProcDir, in.pairSlug, gatefixer.RoleGateFixer),
			}),
			VisitDir: func() (string, error) {
				return rowVisitDir(in.scratchDir, in.statusPath, in.statusLockPath, req.Row)
			},
			Spec:       spec,
			Timeout:    in.gate.FixerTimeout(),
			Forked:     req.Forked,
			SkipForked: req.SkipForked,
		})
	}
}

// fixerSpec returns the closure that builds one fixer's launch for the entry req.Entry names.
// An entry with no body is refused, since the row wiring sends only the entries gateFixerBodies names.
// The closure renders the prompt and reads the declared PATTERN changes at each spawn,
// so a record edited between spawns is read fresh.
func fixerSpec(in gateFixerInputs, req shedrecipe.GateFixerRequest, choice shuttleengine.ModelChoice) (func(reportPath, replyPath string) (gatefixer.Spec, error), error) {
	body, ok := gateFixerBodies[req.Entry]
	if !ok {
		return nil, fmt.Errorf("loomcli: the %s row names a gate fixer on entry %q, which has no fixer body; way forward: put the fixer element on the verify, plan or discussion entry", req.Row, req.Entry)
	}
	values, err := body.values(in, req)
	if err != nil {
		return nil, err
	}
	notePath := friction.NotePath(in.frictionDir, "gate-fixer-"+strings.ToLower(req.Row))
	return func(reportPath, replyPath string) (gatefixer.Spec, error) {
		patternChanges, err := discussionparser.DeclaredPatternChanges(in.patternRecordPath)
		if err != nil {
			return gatefixer.Spec{}, err
		}
		prompt, err := gatefixer.RenderPrompt(gatefixer.PromptInputs{
			StencilsDir:    in.stencilsDir,
			BodyStencil:    body.stencil,
			BodyValues:     values,
			ReportPath:     reportPath,
			ReplyPath:      replyPath,
			RepoRoot:       in.repoRoot,
			PatternChanges: patternChanges,
			ParentName:     in.parentName,
			NotePath:       notePath,
		})
		if err != nil {
			return gatefixer.Spec{}, err
		}
		return gatefixer.Spec{
			Prompt:      prompt,
			ReportPath:  reportPath,
			ReplyPath:   replyPath,
			Role:        gatefixer.RoleGateFixer,
			ModelChoice: choice,
			Skills:      in.languageSkills,
			Segment:     body.segment,
			Timeout:     in.gate.FixerTimeout(),
		}, nil
	}, nil
}
