// cardfixer.go builds the cold fixer that a red `lym gate test --gate card` agent call hands its failure to.
// It holds the hand-off rule and the kit the detached runner drives the fixer through.
// It also holds the store that keeps the fixer's record beside the run.

package gatecli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gatefixer"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/languageconfig"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/state"
)

const (
	// fixerRecordFile is the file in a key directory holding the cold fixer's record, which outlives the spawning run.
	fixerRecordFile = "fixer.json"
	// fixerBodyStencil names the body stencil a card gate's fixer prompt joins behind the shared cold frame.
	fixerBodyStencil = "gatefix-body-card"
)

// cardFixerKit is what the detached runner needs to hand a red call to a cold fixer.
type cardFixerKit struct {
	// fixer spawns, waits on and stops the fixer strand.
	fixer gatefixer.Fixer
	// timeout bounds the fixer's run.
	timeout time.Duration
	// worktreeRoot is the worktree the fixer works in, whose tree identity the re-run starts on.
	worktreeRoot string
	// stencilsDir is the directory the fixer prompt's stencils are read from.
	stencilsDir string
	// model is the model the fixer runs on, and skills are the language skills it loads.
	model  shuttleengine.ModelChoice
	skills []string
}

// wallClock is the real time source of a fixer's wait.
type wallClock struct{}

func (wallClock) Now() time.Time        { return time.Now() }
func (wallClock) Sleep(d time.Duration) { time.Sleep(d) }

// flagValues returns the values the call's words give flag, each spelled `--<flag>=<value>`.
// It stops at the `--` that starts the tiers' own arguments.
func flagValues(words []string, flag string) []string {
	prefix := "--" + flag + "="
	var values []string
	for _, word := range words {
		if word == "--" {
			break
		}
		if value, found := strings.CutPrefix(word, prefix); found {
			values = append(values, value)
		}
	}
	return values
}

// handsOff returns the gate key of a request whose red verdict goes to a cold fixer, and true.
// It returns false for a request whose verdict stands.
// A request hands off only when its verb is `test`, its strand's role is no fixer role, and the slug is non-empty.
// It also names a gate key whose `fixer` is non-empty.
// A request with no role, as an older front writes, hands off.
func handsOff(request detachRequest, cfg gateslot.Config, slug string) (string, bool) {
	if request.Verb != verbTest || slug == "" || gatefixer.IsFixerRole(request.Role) {
		return "", false
	}
	keys := flagValues(request.Args, "gate")
	if len(keys) == 0 {
		return "", false
	}
	key := keys[len(keys)-1]
	if cfg.Gates[key].Fixer == "" {
		return "", false
	}
	return key, true
}

// strandRig holds the shuttle runner and reed engine a fixer strand is started, waited on and stopped through.
// It is built over one worktree's geometry.
type strandRig struct {
	runner     *shuttleengine.Runner
	reed       *reedengine.Engine
	engines    shuttleengine.EngineSet
	shuttleCfg shuttleengine.Config
	anchorRoot string
}

// newStrandRig builds the rig over the reed and pause geometry of location's worktree.
func newStrandRig(location *lymcwd.Location) (strandRig, error) {
	dirs := fabricengine.ConfigDirs(location)
	shuttleCfg, err := shuttleengine.LoadConfig(dirs, "shuttle")
	if err != nil {
		return strandRig{}, err
	}
	reedCfg, err := reedengine.LoadConfig(dirs, "reed")
	if err != nil {
		return strandRig{}, err
	}
	engines, err := shuttleengines.New(shuttleCfg, dirs)
	if err != nil {
		return strandRig{}, err
	}
	reedGeom, err := hubgeom.ReedGeometry(location)
	if err != nil {
		return strandRig{}, err
	}
	pauseGeom, err := hubgeom.PauseGeometry(location)
	if err != nil {
		return strandRig{}, err
	}
	reed := reedengine.New(reedCfg, reedGeom, shuttleengines.EnvScrub())
	runner := shuttleengine.NewRunner(reed, engines, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)
	runner.SetPause(pauseGeom)
	runner.SetSessionEnd(shedadapters.UsageRecorder(location, shuttleengines.RecordReaders()))
	return strandRig{runner: runner, reed: reed, engines: engines, shuttleCfg: shuttleCfg, anchorRoot: reedGeom.AnchorPath}, nil
}

// newCardFixer builds the kit of a request that hands its red verdict to a cold fixer.
// It resolves the worktree from the request's Cwd and loads the gate config.
// It reads the slug from the fabric origin record.
// The kit's fixer is a strand over the worktree's reed geometry, registered under the slug with the gate fixer role.
// Its model is the one the key's `fixer` names, and its timeout is the gate config's fixer timeout.
// Its stencils and language skills come from the hub.
// A call outside a hub, or one whose verdict stands, returns false.
// A config or model that cannot be loaded or resolved is an error.
func newCardFixer(request detachRequest) (cardFixerKit, bool, error) {
	location, inHub, err := resolveHub(request.Cwd)
	if err != nil {
		return cardFixerKit{}, false, err
	}
	if !inHub {
		return cardFixerKit{}, false, nil
	}
	_, _, cfg, refusal := loadGate(request.Cwd)
	if refusal != "" {
		return cardFixerKit{}, false, errors.New(refusal)
	}
	slug := originSlug(location)
	key, hands := handsOff(request, cfg, slug)
	if !hands {
		return cardFixerKit{}, false, nil
	}

	dirs := fabricengine.ConfigDirs(location)
	modelSpec, err := modelspec.Parse(cfg.Gates[key].Fixer)
	if err != nil {
		return cardFixerKit{}, false, fmt.Errorf("gate test: the %s gate fixer %q is no model-spec: %w", key, cfg.Gates[key].Fixer, err)
	}
	registry, err := modelspec.LoadRegistry(dirs)
	if err != nil {
		return cardFixerKit{}, false, err
	}
	resolved, err := registry.Resolve(modelSpec)
	if err != nil {
		return cardFixerKit{}, false, fmt.Errorf("gate test: resolve the %s gate fixer's model-spec: %w; way forward: set the key's fixer in gate.yaml with lym config gate to an alias models.yaml holds, or to \"\" to turn the fixer off", key, err)
	}
	language, err := languageconfig.Load(dirs)
	if err != nil {
		return cardFixerKit{}, false, err
	}
	rig, err := newStrandRig(location)
	if err != nil {
		return cardFixerKit{}, false, err
	}
	fixer := gatefixer.NewStrandFixer(gatefixer.Deps{
		Starter:    rig.runner,
		Stopper:    rig.runner,
		Reed:       rig.reed,
		Engines:    rig.engines,
		ShuttleCfg: rig.shuttleCfg,
		AnchorRoot: rig.anchorRoot,
		Clock:      wallClock{},
		Register:   gatefixer.Registrar(rig.reed, fabricengine.RunProcDir(location.HubPath), slug, gatefixer.RoleGateFixer),
	})
	return cardFixerKit{
		fixer:        fixer,
		timeout:      cfg.FixerTimeout(),
		worktreeRoot: location.WorktreePath(),
		stencilsDir:  fabricengine.StencilsDir(location.HubPath),
		model:        shuttleengine.NewModelChoice(resolved),
		skills:       language.Skills,
	}, true, nil
}

// newFixerStopper builds the strand stop over request's worktree's reed geometry alone.
// It needs no fixer setting, so a live fixer an earlier run left stays stoppable.
// That holds when the key's `fixer` was emptied since, or the kit cannot be built.
func newFixerStopper(request detachRequest) (func(guid string) error, error) {
	location, inHub, err := resolveHub(request.Cwd)
	if err != nil {
		return nil, err
	}
	if !inHub {
		return nil, fmt.Errorf("gate test: %s is in no hub, so no fixer strand can be stopped from there", request.Cwd)
	}
	rig, err := newStrandRig(location)
	if err != nil {
		return nil, err
	}
	return rig.runner.StopStrand, nil
}

// keyFixerStore is a gatefixer.Store over fixer.json in a key directory, the parent of the run directory.
// The record of a live fixer outlives the run that spawned it.
// It holds one record, replaced by the next spawn once the earlier one is terminal; writes go through internal/state.
type keyFixerStore struct {
	dir string
}

func (s keyFixerStore) path() string {
	return filepath.Join(s.dir, fixerRecordFile)
}

// Live returns the record while it has no terminal status, and nil otherwise.
func (s keyFixerStore) Live() (*gatefixer.Record, error) {
	record, found, err := state.ReadJSON[gatefixer.Record](s.path(), s.path()+".lock")
	if err != nil {
		return nil, fmt.Errorf("gate: read the fixer record %s: %w", s.path(), err)
	}
	if !found || record.Status != "" {
		return nil, nil
	}
	return &record, nil
}

// Save replaces the stored record with rec.
func (s keyFixerStore) Save(rec *gatefixer.Record) error {
	if err := state.WriteJSON(s.path(), s.path()+".lock", *rec); err != nil {
		return fmt.Errorf("gate: write the fixer record %s: %w", s.path(), err)
	}
	return nil
}

// markTerminal gives the stored record the status and dead reason, leaving its other fields as they are.
func (s keyFixerStore) markTerminal(status, deadReason string) error {
	err := state.UpdateJSON(s.path(), s.path()+".lock", func(current gatefixer.Record, found bool) (gatefixer.Record, error) {
		if !found {
			return current, fmt.Errorf("no fixer record to mark %s", status)
		}
		current.Status = status
		current.DeadReason = deadReason
		return current, nil
	})
	if err != nil {
		return fmt.Errorf("gate: mark the fixer record %s: %w", s.path(), err)
	}
	return nil
}
