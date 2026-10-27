// green.go keeps the per-worktree green record `lym gate test` carries a passed tier forward with, and plans which targets of a tier the record already holds green.

package gatecli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// greenFileName is the green record's file name in the worktree's gate directory.
const greenFileName = "green.json"

// greenWholeCommitsKept is how many of a tier fingerprint's newest whole-green commits a write keeps beside the current tree's entries.
// Only whole entries with empty args count, as only they can serve as a diff base.
const greenWholeCommitsKept = 20

// greenCommandBudget bounds the targets a carried-over remainder renders into one command, in bytes; a longer remainder runs whole instead, since a shell refuses an overlong command line.
const greenCommandBudget = 6000

// greenEntry is one tier pass the green record holds: the tier's fingerprint, the tree and commit it passed on, the module directory and args it ran with, and either whole or the green packages.
type greenEntry struct {
	// Fingerprint is the tierFingerprint of the tier that passed.
	Fingerprint string `json:"fingerprint"`
	// Tree and Commit are HEAD's tree and commit when the tier passed.
	Tree   string `json:"tree"`
	Commit string `json:"commit"`
	// ModuleDir is the run directory relative to the worktree root, "." for the root.
	ModuleDir string `json:"module_dir"`
	// Args are the words after `--` the tier ran with.
	Args []string `json:"args"`
	// Whole means the tier passed over every target it can run.
	Whole bool `json:"whole"`
	// Packages are the module-relative package arguments that passed, when Whole is not set.
	Packages []string  `json:"packages,omitempty"`
	Written  time.Time `json:"written"`
}

// greenRecord is the content of green.json.
type greenRecord struct {
	Entries []greenEntry `json:"entries"`
}

// tierFingerprint hashes tier's declaration without its parallel, timeout and slots, so a changed command, env, targets, counter, filter or carry setting never carries a pass over.
// build is hashed only for a tier with shared_build, and inert only when non-empty;
// every key added after the original fields is hashed at a non-default value alone, so a tier that sets none of them keeps its fingerprint.
func tierFingerprint(tier gateslot.Tier, build gateslot.BuildStep, inert []string) string {
	declaration := struct {
		About       string
		Command     string
		Dir         string
		Env         map[string]string
		AllTargets  string
		Counter     string
		Covers      []int
		PrebuiltLym bool
		Manual      bool
		Filter      string              `json:",omitempty"`
		SharedBuild bool                `json:",omitempty"`
		NoCarry     bool                `json:",omitempty"`
		Build       *gateslot.BuildStep `json:",omitempty"`
		Inert       []string            `json:",omitempty"`
	}{
		About: tier.About, Command: tier.Command, Dir: tier.Dir, Env: tier.Env, AllTargets: tier.AllTargets, Counter: tier.Counter, Covers: tier.Covers, PrebuiltLym: tier.PrebuiltLym, Manual: tier.Manual,
		Filter: tier.Filter, SharedBuild: tier.SharedBuild, NoCarry: tier.NoCarry, Inert: inert,
	}
	if tier.SharedBuild {
		declaration.Build = &build
	}
	data, err := json.Marshal(declaration)
	if err != nil {
		panic(fmt.Sprintf("gate test: encode a tier declaration: %v", err))
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// readGreen reads the green record at path.
// An absent, unreadable or malformed file reads as an empty record, which only costs a re-run, and a failure other than an absent file is logged.
func readGreen(path string) greenRecord {
	record, _, err := state.ReadJSON[greenRecord](path, path+".lock")
	if err != nil {
		logger.Warn("gate test: could not read the green record; running as if it held nothing", "path", path, "cause", err)
		return greenRecord{}
	}
	return record
}

// recordGreen adds entry, stamped now, to the green record at path under the record's lock.
// A same-key whole entry replaces the key's entries and a same-key package entry joins the key's packages; a write then drops the entries of other trees but each fingerprint's greenWholeCommitsKept newest whole-green commits, counting only whole entries with empty args.
// A malformed file is replaced by a record holding entry alone.
func recordGreen(path string, entry greenEntry, now time.Time) error {
	update := func(current greenRecord, _ bool) (greenRecord, error) {
		return current.with(entry, now), nil
	}
	err := state.UpdateJSON(path, path+".lock", update)
	if errors.Is(err, state.ErrDecode) {
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
			return fmt.Errorf("replace a malformed green record: %w", removeErr)
		}
		err = state.UpdateJSON(path, path+".lock", update)
	}
	return err
}

// with returns r with entry added and the entries pruned.
func (r greenRecord) with(entry greenEntry, now time.Time) greenRecord {
	entry.Written = now
	sameKey := func(other greenEntry) bool {
		return other.Fingerprint == entry.Fingerprint && other.Tree == entry.Tree && other.ModuleDir == entry.ModuleDir && slices.Equal(other.Args, entry.Args)
	}

	var entries []greenEntry
	for _, held := range r.Entries {
		switch {
		case !sameKey(held):
			entries = append(entries, held)
		case held.Whole && !entry.Whole:
			// A whole entry already covers every package the new one names.
			return r
		case !held.Whole && !entry.Whole:
			entry.Packages = unionSorted(entry.Packages, held.Packages)
		}
	}
	entries = append(entries, entry)

	type wholeCommit struct{ fingerprint, commit string }
	newest := slices.Clone(entries)
	sort.SliceStable(newest, func(i, j int) bool { return newest[i].Written.After(newest[j].Written) })
	kept := map[wholeCommit]bool{}
	perFingerprint := map[string]int{}
	for _, held := range newest {
		key := wholeCommit{held.Fingerprint, held.Commit}
		if !held.Whole || len(held.Args) > 0 || kept[key] || perFingerprint[held.Fingerprint] >= greenWholeCommitsKept {
			continue
		}
		kept[key] = true
		perFingerprint[held.Fingerprint]++
	}
	pruned := entries[:0]
	for _, held := range entries {
		if held.Tree == entry.Tree || (held.Whole && len(held.Args) == 0 && kept[wholeCommit{held.Fingerprint, held.Commit}]) {
			pruned = append(pruned, held)
		}
	}
	return greenRecord{Entries: pruned}
}

// covered returns what the record holds green for the tier of fingerprint on tree, in moduleDir, under args:
// whole when an entry passed over every target, else the union of the packages the matching entries passed.
// An entry with empty args matches a request with empty args and the guard step's own -run filter; an entry with any other args matches only exactly.
func (r greenRecord) covered(fingerprint, tree, moduleDir string, args []string) (whole bool, packages []string) {
	for _, held := range r.Entries {
		if held.Fingerprint != fingerprint || held.Tree != tree || held.ModuleDir != moduleDir || !argsMatch(held.Args, args) {
			continue
		}
		if held.Whole {
			return true, nil
		}
		packages = unionSorted(packages, held.Packages)
	}
	return false, packages
}

// newestWholeAncestor returns the commit of the newest whole-green entry for the tier of fingerprint in moduleDir whose commit is an ancestor of HEAD, and whether there is one.
// isAncestor reports whether its first commit is an ancestor of its second, which may be "HEAD".
// Newest is by history: a commit another accepted commit descends from loses to it, and only commits unrelated by history compare by write time.
// Only an entry that ran with empty args counts: a whole pass under `-- -run X` covers less than the tier, and a package entry covers only part of it.
func (r greenRecord) newestWholeAncestor(fingerprint, moduleDir string, isAncestor func(commit, of string) bool) (string, bool) {
	var newest greenEntry
	found := false
	for _, held := range r.Entries {
		if !held.Whole || held.Fingerprint != fingerprint || held.ModuleDir != moduleDir || len(held.Args) > 0 {
			continue
		}
		if !isAncestor(held.Commit, "HEAD") {
			continue
		}
		switch {
		case !found, isAncestor(newest.Commit, held.Commit):
			newest, found = held, true
		case isAncestor(held.Commit, newest.Commit):
		case !held.Written.Before(newest.Written):
			newest = held
		}
	}
	return newest.Commit, found
}

// argsMatch reports whether an entry that ran with entryArgs covers a request with requestArgs.
func argsMatch(entryArgs, requestArgs []string) bool {
	if len(entryArgs) == 0 {
		return len(requestArgs) == 0 || isGuardRunFilter(requestArgs)
	}
	return slices.Equal(entryArgs, requestArgs)
}

// isGuardRunFilter reports whether args are exactly the -run filter the guard step builds over its guard test names.
func isGuardRunFilter(args []string) bool {
	return len(args) == 2 && args[0] == "-run" && strings.HasPrefix(args[1], "^(") && strings.HasSuffix(args[1], ")$")
}

// unionSorted returns the sorted union of a and b without repeats.
func unionSorted(a, b []string) []string {
	merged := slices.Concat(a, b)
	sort.Strings(merged)
	return slices.Compact(merged)
}

// greenRun is one call's use of the green record: the clean tree it started on and the record read for it.
// A call on a dirty tree, or one whose HEAD cannot be read, holds none.
type greenRun struct {
	path     string
	root     string
	tree     string
	commit   string
	record   greenRecord
	scoperGo bool
	goBinary string
	// cfg is the call's gate config, whose build step and inert_paths join a tier's fingerprint and whose inert_paths decide an inert carry.
	cfg gateslot.Config
}

// fingerprint returns the fingerprint a pass of tier is recorded and looked up under in this call.
func (g *greenRun) fingerprint(tier gateslot.Tier) string {
	return tierFingerprint(tier, g.cfg.Build, g.cfg.InertPaths)
}

// openGreen reads the green record for a call at root when the worktree is clean, and returns nil when no record applies.
// A failed status or HEAD read is logged and also returns nil, so the call runs every tier.
func openGreen(gateDir, root, goBinary string, cfg gateslot.Config) *greenRun {
	dirty, err := verifytree.DirtyPaths(root)
	if err != nil {
		logger.Warn("gate test: could not check the worktree for the green record; running every tier", "cause", err)
		return nil
	}
	if len(dirty) > 0 {
		logger.Info("gate test: the worktree is dirty, so the green record is neither read nor written", "paths", len(dirty))
		return nil
	}
	tree, commit, err := headIdentity(root)
	if err != nil {
		logger.Warn("gate test: could not read HEAD for the green record; running every tier", "cause", err)
		return nil
	}
	recordPath := filepath.Join(gateDir, greenFileName)
	return &greenRun{path: recordPath, root: root, tree: tree, commit: commit, record: readGreen(recordPath), scoperGo: cfg.Scoper == "go", goBinary: goBinary, cfg: cfg}
}

// headIdentity returns HEAD's tree and commit in root.
func headIdentity(root string) (tree, commit string, err error) {
	out, err := gitexec.Run([]string{"rev-parse", "HEAD^{tree}", "HEAD"}, root)
	if err != nil {
		return "", "", err
	}
	lines := strings.Fields(out)
	if len(lines) != 2 {
		return "", "", fmt.Errorf("git rev-parse HEAD^{tree} HEAD printed %q; want a tree and a commit", out)
	}
	return lines[0], lines[1], nil
}

// moduleKey returns dir, resolved against root, as the slash path relative to root that entries name their module by.
func moduleKey(root, dir string) string {
	absolute := resolveRunDir(root, dir)
	relative, err := filepath.Rel(root, absolute)
	if err != nil {
		return filepath.ToSlash(absolute)
	}
	return filepath.ToSlash(relative)
}

// greenPlan is what a green record leaves a tier run to do.
type greenPlan struct {
	// invocations are the invocations still to run; none means the whole run is carried.
	invocations []invocation
	// carried maps a module key to the packages of its invocation the record held green.
	carried map[string][]string
	// inertCarried lists the module keys whose whole invocation is carried from an ancestor's whole entry over an inert-only diff; each needs a whole entry for the current tree.
	inertCarried []string
}

// subtract returns run with what the record holds green for the current tree taken out.
// A whole entry drops its module's invocation, and under the go scoper a package entry drops the packages it names from the invocation's targets.
// A command that renders into more than greenCommandBudget bytes of targets, or whose packages cannot be listed, keeps the whole invocation.
// A tier with carry: false keeps every invocation.
// Under inert_paths, a whole invocation the record holds nothing for is also carried when a whole entry of an ancestor stands for it over an inert-only diff, and its module key joins the plan's inertCarried.
func (g *greenRun) subtract(ctx context.Context, run tierRun, env []string) greenPlan {
	plan := greenPlan{carried: map[string][]string{}}
	if run.tier.NoCarry {
		plan.invocations = run.invocations
		return plan
	}
	fingerprint := g.fingerprint(run.tier)
	hasTargets := strings.Contains(run.tier.Command, "{targets}")
	for _, inv := range run.invocations {
		key := moduleKey(g.root, inv.dir)
		whole, packages := g.record.covered(fingerprint, g.tree, key, run.args)
		if whole {
			continue
		}
		if len(packages) == 0 && g.carriesOverInert(ctx, run, inv, key, fingerprint) {
			plan.inertCarried = append(plan.inertCarried, key)
			continue
		}
		if len(packages) == 0 || !g.scoperGo || !hasTargets || len(inv.targets) == 0 {
			plan.invocations = append(plan.invocations, inv)
			continue
		}
		listed, err := g.expandPackages(ctx, inv.dir, run.tier, inv.targets, env)
		if err != nil {
			logger.Warn("gate test: could not list the packages to subtract the green record from; running the whole invocation", "dir", inv.dir, "cause", err)
			plan.invocations = append(plan.invocations, inv)
			continue
		}
		remainder := subtractPackages(listed, packages)
		switch {
		case len(remainder) == len(listed):
			plan.invocations = append(plan.invocations, inv)
		case len(remainder) == 0:
		case len(strings.Join(remainder, " ")) > greenCommandBudget:
			plan.invocations = append(plan.invocations, inv)
		default:
			plan.carried[key] = slices.DeleteFunc(slices.Clone(listed), func(pkg string) bool { return slices.Contains(remainder, pkg) })
			inv.targets = remainder
			plan.invocations = append(plan.invocations, inv)
		}
	}
	return plan
}

// carriesOverInert reports whether the whole invocation inv of run, which the record holds no entry for on the current tree, is carried over an inert-only diff:
// the record holds a whole entry under the same fingerprint, empty args and module key for an ancestor whose diff to the working tree is inert.
// Only a whole invocation of a tier's own run qualifies, never the guard or vet step, a run with args or a partial invocation, and a hub with no inert_paths carries nothing this way.
func (g *greenRun) carriesOverInert(ctx context.Context, run tierRun, inv invocation, key, fingerprint string) bool {
	if len(g.cfg.InertPaths) == 0 || len(run.args) > 0 || run.guardStep || run.vetStep || inv.partial {
		return false
	}
	if strings.Contains(run.tier.Command, "{targets}") && (key != moduleKey(g.root, run.tier.Dir) || !slices.Equal(inv.targets, strings.Fields(run.tier.AllTargets))) {
		return false
	}
	isAncestor := func(commit, of string) bool {
		held, err := gitrepo.New(g.root).IsAncestor(commit, of)
		return err == nil && held
	}
	base, found := g.record.newestWholeAncestor(fingerprint, key, isAncestor)
	return found && allInertSince(ctx, g.cfg, g.root, base)
}

// recordInertCarried writes a whole entry for the current tree for each module key plan carried over an inert-only diff, so a later gate carries it directly.
// A failed write is logged and decides nothing.
func (g *greenRun) recordInertCarried(run tierRun, plan greenPlan, now time.Time) {
	fingerprint := g.fingerprint(run.tier)
	for _, key := range plan.inertCarried {
		entry := greenEntry{Fingerprint: fingerprint, Tree: g.tree, Commit: g.commit, ModuleDir: key, Whole: true}
		if err := recordGreen(g.path, entry, now); err != nil {
			logger.Warn("gate test: could not write the green record", "tier", tierName(run.number), "cause", err)
		}
	}
}

// subtractPackages returns the packages of listed that green does not name, in listed's order.
func subtractPackages(listed, green []string) []string {
	return slices.DeleteFunc(slices.Clone(listed), func(pkg string) bool { return slices.Contains(green, pkg) })
}

// stillGreen reports whether the worktree is still clean and HEAD's tree is still the one the call started on, which a pass needs before it is recorded.
func (g *greenRun) stillGreen() bool {
	dirty, err := verifytree.DirtyPaths(g.root)
	if err != nil || len(dirty) > 0 {
		return false
	}
	tree, _, err := headIdentity(g.root)
	return err == nil && tree == g.tree
}

// recordPass records the invocations of run that just passed, with the carried packages plan names beside them, when the tree is still the one the call started on.
// An invocation is recorded whole when it ran every target of its tier, or when the packages it ran and the packages carried cover every package the tier's all_targets expand to; otherwise, under the go scoper, it records the packages it ran.
// A partial invocation, one narrowed to a selected project and its classes, is never recorded, so a filtered pass never stands in for a whole run.
// A tier with carry: false records nothing.
// A failed write is logged and decides nothing.
func (g *greenRun) recordPass(ctx context.Context, run tierRun, ran []invocation, plan greenPlan, env []string, now time.Time) {
	if !g.stillGreen() {
		logger.Info("gate test: the tree changed during the run, so the pass is not recorded", "tier", tierName(run.number))
		return
	}
	if run.tier.NoCarry {
		return
	}
	fingerprint := g.fingerprint(run.tier)
	for _, inv := range ran {
		key := moduleKey(g.root, inv.dir)
		entry := greenEntry{Fingerprint: fingerprint, Tree: g.tree, Commit: g.commit, ModuleDir: key, Args: run.args}
		switch {
		case inv.partial:
			continue
		case run.guardStep:
			entry.Packages = inv.targets
		case !strings.Contains(run.tier.Command, "{targets}"):
			entry.Whole = true
		case len(plan.carried[key]) == 0 && slices.Equal(inv.targets, strings.Fields(run.tier.AllTargets)) && key == moduleKey(g.root, run.tier.Dir):
			entry.Whole = true
		case !g.scoperGo:
			continue
		default:
			packages, err := g.expandPackages(ctx, inv.dir, run.tier, inv.targets, env)
			if err != nil {
				logger.Warn("gate test: could not list the packages that passed; the pass is not recorded", "tier", tierName(run.number), "cause", err)
				continue
			}
			entry.Packages = packages
			if len(plan.carried[key]) > 0 && key == moduleKey(g.root, run.tier.Dir) {
				entry.Whole = g.coversAllTargets(ctx, run.tier, unionSorted(packages, plan.carried[key]), env)
			}
		}
		if entry.Whole {
			entry.Packages = nil
		}
		if err := recordGreen(g.path, entry, now); err != nil {
			logger.Warn("gate test: could not write the green record", "tier", tierName(run.number), "cause", err)
		}
	}
}

// coversAllTargets reports whether packages hold every package the tier's all_targets expand to.
func (g *greenRun) coversAllTargets(ctx context.Context, tier gateslot.Tier, packages []string, env []string) bool {
	all, err := g.expandPackages(ctx, tier.Dir, tier, strings.Fields(tier.AllTargets), env)
	if err != nil || len(all) == 0 {
		return false
	}
	return len(subtractPackages(all, packages)) == 0
}

// buildTagsPattern finds the build tags a tier's command compiles with.
var buildTagsPattern = regexp.MustCompile(`-tags[= ]+"?([A-Za-z0-9_,!.-]+)"?`)

// expandPackages returns targets as module-relative package arguments of the tier's module at dir, sorted and without repeats.
// A target with no `...` is only cleaned; a pattern is listed with `go list` under the build tags the tier's command names.
func (g *greenRun) expandPackages(ctx context.Context, dir string, tier gateslot.Tier, targets []string, env []string) ([]string, error) {
	if !slices.ContainsFunc(targets, func(target string) bool { return strings.Contains(target, "...") }) {
		packages := make([]string, len(targets))
		for i, target := range targets {
			packages[i] = normalizePackage(target)
		}
		return unionSorted(packages, nil), nil
	}

	moduleDir := resolveRunDir(g.root, dir)
	args := []string{"list"}
	if match := buildTagsPattern.FindStringSubmatch(tier.Command); match != nil {
		args = append(args, "-tags", match[1])
	}
	args = append(args, "-f", "{{.Dir}}")
	args = append(args, targets...)
	cmd := exec.CommandContext(ctx, g.goBinary, args...)
	cmd.Dir = moduleDir
	cmd.Env = slices.Concat(env, renderTierEnv(tier))
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list %s in %s: %w", strings.Join(targets, " "), moduleDir, err)
	}
	var packages []string
	for _, listed := range strings.Fields(string(out)) {
		relative, err := filepath.Rel(moduleDir, listed)
		if err != nil {
			return nil, fmt.Errorf("package directory %s outside %s: %w", listed, moduleDir, err)
		}
		packages = append(packages, normalizePackage("./"+filepath.ToSlash(relative)))
	}
	return unionSorted(packages, nil), nil
}

// normalizePackage returns a relative package argument as "./<dir>" cleaned, "." for the module root, and any other argument unchanged.
func normalizePackage(target string) string {
	if !strings.HasPrefix(target, ".") {
		return target
	}
	cleaned := path.Clean(target)
	if cleaned == "." {
		return "."
	}
	return "./" + cleaned
}
