// campaign.go holds Preflight's check of a hardener campaign: the refusals a run meets before any agent spends tokens.

package hardenshed

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/modelspec"
)

// The gate keys the campaign's live gate runs: its merge-bar run, and its concurrent copies.
const (
	LiveGateKey       = "harden-live"
	LiveCopiesGateKey = "harden-live-copies"
)

// LiveKey is what the campaign check reads of one live gate key.
type LiveKey struct {
	// Set says whether the hub's own gate.yaml sets the key.
	Set bool
	// Tiers are the key's own tiers.
	Tiers []int
	// Tested are the tiers a run of the key tests.
	// They are Tiers with each tier's covers, as `lym gate test` narrows `{test_targets}`.
	Tested []int
}

// CampaignCheck is the input of CheckCampaign.
type CampaignCheck struct {
	// Config is the loaded hardener config.
	Config Config
	// WorktreeRoot is the absolute root of the worktree the campaign's packages are relative to.
	WorktreeRoot string
	// Live is the `harden-live` key.
	Live LiveKey
	// Copies is the `harden-live-copies` key, read only when copies run.
	Copies LiveKey
	// HoldsTierTest reports whether the package directory dir holds a test that a run of tiers executes.
	HoldsTierTest func(dir string, tiers []int) bool
	// Route returns the way forward that changes the config key, by the layer the key's value comes from.
	Route func(key string) (string, error)
}

// CheckCampaign returns the first refusal of a campaign Preflight must not start, or nil.
// Each refusal names its way forward and ends with `lym loom resume`.
// The refusals come in this order:
// no package, or one whose directory the worktree lacks;
// a live key the hub's gate.yaml does not set;
// a live key naming the billed tier, or the copies key naming more than one tier when copies run;
// a package holding no test of a tier a live key runs;
// an empty reviewer or fixer list;
// a reviewer list of one entry, or one whose cyclic expansion repeats a model in neighbouring rounds;
// and a soft cap of review rounds above the hard cap.
// Copies run when the live tests drive no LLM session and the concurrency is above 1.
func CheckCampaign(in CampaignCheck) error {
	cfg := in.Config
	copiesRun := !cfg.LLMDriving && cfg.Concurrency > 1
	keys := []struct {
		name string
		key  LiveKey
	}{{LiveGateKey, in.Live}}
	if copiesRun {
		keys = append(keys, struct {
			name string
			key  LiveKey
		}{LiveCopiesGateKey, in.Copies})
	}

	if len(cfg.Packages) == 0 {
		return in.refuseKey("packages", "the hardener config names no package")
	}
	for _, pkg := range cfg.Packages {
		if info, err := os.Stat(filepath.Join(in.WorktreeRoot, pkg)); err != nil || !info.IsDir() {
			return in.refuseKey("packages", fmt.Sprintf("the hardener config names package %q, which the worktree has no directory for", pkg))
		}
	}
	for _, live := range keys {
		if !live.key.Set {
			return refuse(fmt.Sprintf("the hub's gate.yaml sets no gates.%s key", live.name), gateWrite(live.name))
		}
	}
	for _, live := range keys {
		if slices.Contains(live.key.Tiers, gateslot.BilledTier) {
			return refuse(fmt.Sprintf("gates.%s names tier%d, which drives a billed LLM session", live.name, gateslot.BilledTier), gateWrite(live.name))
		}
	}
	if copiesRun && len(in.Copies.Tiers) > 1 {
		return refuse(fmt.Sprintf("gates.%s names the tiers %v, and --copies runs one tier", LiveCopiesGateKey, in.Copies.Tiers), gateWrite(LiveCopiesGateKey))
	}
	for _, live := range keys {
		for _, pkg := range cfg.Packages {
			if in.HoldsTierTest(filepath.Join(in.WorktreeRoot, pkg), live.key.Tested) {
				continue
			}
			route, err := in.Route("packages")
			if err != nil {
				return err
			}
			return refuse(fmt.Sprintf("package %q holds no test of the tiers %v that gates.%s runs", pkg, live.key.Tested, live.name),
				fmt.Sprintf("list only packages that hold such a test: %s; or point the key at a tier the package tests: %s", route, gateWrite(live.name)))
		}
	}
	for _, list := range []struct {
		key     string
		entries []string
	}{{"reviewer", cfg.Reviewer}, {"fixer", cfg.Fixer}} {
		if len(list.entries) == 0 {
			return in.refuseKey(list.key, fmt.Sprintf("the %s list is empty, so no round has a model to run with; list at least one model", list.key))
		}
	}
	if reason := reviewerRepeat(cfg.Reviewer); reason != "" {
		return in.refuseKey("reviewer", reason)
	}
	if cfg.MaxBounces > cfg.HardMaxBounces {
		return in.refuseKey("max_bounces", fmt.Sprintf("max_bounces %d is above hard_max_bounces %d", cfg.MaxBounces, cfg.HardMaxBounces))
	}
	return nil
}

// refuseKey returns the refusal of detail whose way forward changes the config key, by the route the told Route names.
func (in CampaignCheck) refuseKey(key, detail string) error {
	route, err := in.Route(key)
	if err != nil {
		return err
	}
	return refuse(detail, route)
}

// refuse returns a campaign refusal made of what is wrong and its way forward, closed with the resume.
func refuse(detail, wayForward string) error {
	return fmt.Errorf("hardener campaign: %s; way forward: %s, then run \"lym loom resume\"", detail, wayForward)
}

// gateWrite returns the write that sets a live gate key in the hub's gate.yaml.
func gateWrite(key string) string {
	return fmt.Sprintf("run `lym config gate --set 'gates.%s={tiers: [<live tier>], scope: diff}'` from the prime", key)
}

// reviewerRepeat returns what is wrong with a reviewer list, or "" when no round repeats the model of its neighbour.
// A list of one entry repeats it every round, and the cyclic expansion's last entry meets its first.
func reviewerRepeat(reviewer []string) string {
	if len(reviewer) == 1 {
		return "the reviewer list holds one entry, so every round reviews with the same model; list at least two models, none twice in a row"
	}
	for i, entry := range reviewer {
		next := reviewer[(i+1)%len(reviewer)]
		if modelOf(entry) == modelOf(next) {
			return fmt.Sprintf("the reviewer list repeats the model %q in neighbouring rounds, %q then %q, its last and first entries included", modelOf(entry), entry, next)
		}
	}
	return ""
}

// modelOf returns the model a model-spec names, its bracket parameters left out.
// A spec that does not parse is its own model.
func modelOf(spec string) string {
	parsed, err := modelspec.Parse(spec)
	if err != nil {
		return spec
	}
	if parsed.Alias != "" {
		return parsed.Alias
	}
	return parsed.Engine + ":" + parsed.Model
}
