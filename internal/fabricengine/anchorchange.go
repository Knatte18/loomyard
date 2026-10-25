// anchorchange.go changes one worktree's anchor: it moves the worktree's weft content to the new directory, or integrates the change the weft's upstream made, and recovers a change a kill left half done.
// A change is three calls the CLI interleaves with its session, run and dobby steps: BeginAnchorChange refuses and holds the move lock, Apply does the work, Finish ends the record.

package fabricengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/state"
)

// AnchorChange is one change of a worktree's anchor from one directory of its warp checkout to another, begun by BeginAnchorChange.
// It holds the weft's move lock from BeginAnchorChange until Close, and the move record names the change on disk for as long as it stands.
// A change that has nothing to do, because the worktree already sits at the requested anchor, takes no lock and writes no record.
type AnchorChange struct {
	pair     lyxcwd.Location
	weftPath string
	names    []string
	// from and to are the anchors the change moves between.
	from, to string
	// kind is the record's kind, empty for a change with nothing to do.
	kind string
	// upstream is the weft's upstream tip an integration rebuilds on, empty until it is known.
	upstream string
	// landed reports that the weft already carries the change, so a re-run goes straight to the wiring.
	landed bool
	// freshRecord reports that BeginAnchorChange wrote the record with none standing before.
	freshRecord bool
	applied     bool
	closed      bool
	moveLock    *lock.FileLock
	// recovered holds what the recovery of a killed change did while the change was begun, handed to the recorder Apply is given.
	recovered *Mutations
}

// NoOp reports whether the worktree already sits at the requested anchor, so Apply and Finish do nothing.
func (c *AnchorChange) NoOp() bool { return c.kind == "" }

// From returns the anchor the change moves the worktree from.
func (c *AnchorChange) From() string { return c.from }

// To returns the anchor the change moves the worktree to.
func (c *AnchorChange) To() string { return c.to }

// locationAt returns the pair's Location at anchor.
func (c *AnchorChange) locationAt(anchor string) *lyxcwd.Location {
	at := c.pair
	at.AnchorRel = anchor
	return &at
}

// rerun returns the verb that finishes this change after a stop, for a way forward.
func (c *AnchorChange) rerun() string {
	return fmt.Sprintf("`lyx fabric anchor %s --set %s`", c.pair.WorktreeName, c.to)
}

// movesLyxDir reports whether the change relocates the weft's `.lyx` directory.
func (c *AnchorChange) movesLyxDir() bool {
	return filepath.Clean(c.from) != filepath.Clean(c.to)
}

// BeginAnchorChange validates the change of the worktree pair to the directory target, refuses what cannot be settled, and holds the move lock for it.
// Nothing is moved before every refusal has been judged: an empty own marker; a target the warp checkout cannot hold; a held move lock;
// with a standing record from a killed run, a target other than the record's; and with no record, a weft with no committed marker or with uncommitted changes under its anchor.
// Each refusal names its next step.
//
// The kind of change is integrate when the weft's fetched upstream has commits its tip lacks and the two tips carry different anchors, else move when target differs from the pair's anchor, else none.
// A change of kind none takes no lock.
// Any other writes the move record with this process's pid and start time, keeping a standing record's other fields.
// A standing record whose change did not land is recovered here, before the clean tree is judged: the weft's paths go back to HEAD and `.lyx` back to the old anchor.
// The caller ends the change with Close.
func (t *Topology) BeginAnchorChange(pair *lyxcwd.Location, target string) (*AnchorChange, error) {
	slug := pair.WorktreeName
	weftPath := RecordsWorktreePath(pair, slug)
	if _, err := os.Stat(weftPath); err != nil {
		return nil, fmt.Errorf("the records worktree of %s is not readable at %s: %w; run `lyx fabric reconcile`", slug, weftPath, err)
	}
	if err := refuseEmptyOwnMarker(weftPath); err != nil {
		return nil, err
	}
	record, standing, err := ReadMoveRecord(weftPath)
	if err != nil {
		return nil, err
	}
	basis := *pair
	if standing {
		basis.AnchorRel = record.A
	}
	names, err := RepoWiredNames(&basis)
	if err != nil {
		return nil, fmt.Errorf("load the wired names to change the anchor of %s: %w", slug, err)
	}
	if err := validateAnchorTarget(&basis, target, names); err != nil {
		return nil, err
	}
	to, err := lyxcwd.ValidateAnchorRel(target)
	if err != nil {
		return nil, err
	}

	if _, err := ensureWeftLockDirAt(weftPath); err != nil {
		return nil, err
	}
	moveLock, acquired, err := lock.TryAcquireWriteLock(moveLockPath(weftPath))
	if err != nil {
		return nil, fmt.Errorf("take the move lock of %s: %w", weftPath, err)
	}
	if !acquired {
		return nil, heldMoveRefusal(slug, weftPath, to)
	}

	c := &AnchorChange{pair: basis, weftPath: weftPath, names: names, from: basis.AnchorRel, to: to, moveLock: moveLock, recovered: NewMutations(pair.HubPath)}
	if standing {
		err = c.resume(record)
	} else {
		err = c.start()
	}
	if err == nil && !c.NoOp() {
		err = c.writeRecord(record, standing)
	}
	if err != nil || c.NoOp() {
		c.Close()
		if err != nil {
			return nil, err
		}
		return c, nil
	}
	c.freshRecord = !standing
	return c, nil
}

// heldMoveRefusal returns the refusal for a move lock another process holds.
// A holder whose record names a live process is a change in progress, and any other holder is a change still starting, before its record exists or beside the record of a killed run.
func heldMoveRefusal(slug, weftPath, target string) error {
	if record, found, err := ReadMoveRecord(weftPath); err == nil && found && recordNamesLiveHolder(record) {
		if wayForward, standing, err := MoveInProgress(slug, weftPath); err == nil && standing {
			return errors.New(wayForward)
		}
	}
	return fmt.Errorf("%s, then run `lyx fabric anchor %s --set %s`", startingMoveStop(slug), slug, target)
}

// start judges a change with no record standing: it needs a committed marker and a clean tree, and settles the kind.
func (c *AnchorChange) start() error {
	slug := c.pair.WorktreeName
	marker, found, err := committedMarker(c.weftPath, "HEAD")
	if err != nil {
		return err
	}
	if !found || marker == "" {
		return fmt.Errorf("the weft of %s has no committed %s, so its anchor is not recorded; run `lyx fabric reconcile` to record it, then run the verb again", slug, lyxcwd.AnchorFileName)
	}
	if err := refuseUnsettledMoveSource(c.weftPath, slug, c.from, c.names); err != nil {
		return err
	}

	integrate, upstream, err := c.upstreamCarriesAnotherAnchor()
	if err != nil {
		return err
	}
	switch {
	case integrate:
		c.kind, c.upstream = MoveKindIntegrate, upstream
	case c.movesLyxDir():
		c.kind = MoveKindMove
	}
	if c.NoOp() || !c.movesLyxDir() {
		return nil
	}
	if present, err := lyxDirPresent(c.weftPath, c.to); err != nil {
		return err
	} else if present {
		return fmt.Errorf("the weft of %s already holds %s; move it out of the way, then run the verb again", slug, filepath.Join(c.to, lyxdirs.DotLyxDirName))
	}
	return nil
}

// upstreamCarriesAnotherAnchor reports whether the weft's fetched upstream has commits its tip lacks and a tip anchored elsewhere, and returns that upstream tip.
func (c *AnchorChange) upstreamCarriesAnotherAnchor() (bool, string, error) {
	upstream, hasUpstream, err := upstreamSHAAt(c.weftPath)
	if err != nil || !hasUpstream {
		return false, "", err
	}
	repo := gitrepo.New(c.weftPath)
	head, err := repo.CurrentSHA()
	if err != nil {
		return false, "", err
	}
	taken, err := repo.CommitsNotIn(upstream, head)
	if err != nil || len(taken) == 0 {
		return false, "", err
	}
	headAnchor, err := treeAnchor(c.weftPath, head, c.names, c.from)
	if err != nil {
		return false, "", err
	}
	upstreamAnchor, err := treeAnchor(c.weftPath, upstream, c.names, c.from)
	if err != nil {
		return false, "", err
	}
	return filepath.Clean(headAnchor) != filepath.Clean(upstreamAnchor), upstream, nil
}

// resume takes up the change a standing record names, which a killed run left, and recovers it when it did not land.
// The record's anchors, kind and upstream tip govern; a target other than the record's is refused.
func (c *AnchorChange) resume(record MoveRecord) error {
	slug := c.pair.WorktreeName
	if filepath.Clean(record.B) != filepath.Clean(c.to) {
		return fmt.Errorf("a change of the anchor of %s from %s to %s was interrupted, so a change to %s is refused; run `lyx fabric anchor %s --set %s` to finish it",
			slug, record.A, record.B, c.to, slug, record.B)
	}
	c.from, c.to, c.kind, c.upstream = record.A, record.B, record.Kind, record.U

	landed, err := c.changeLanded()
	if err != nil {
		return err
	}
	if landed {
		c.landed = true
		return nil
	}
	if err := c.recoverUnlanded(); err != nil {
		return err
	}
	return refuseUnsettledMoveSource(c.weftPath, slug, c.from, c.names)
}

// changeLanded reports whether the weft's HEAD already carries the change: its committed marker is the new anchor, and an integration also holds the upstream tip it was rebuilt on.
func (c *AnchorChange) changeLanded() (bool, error) {
	marker, found, err := committedMarker(c.weftPath, "HEAD")
	if err != nil || !found || filepath.Clean(marker) != filepath.Clean(c.to) {
		return false, err
	}
	if c.kind != MoveKindIntegrate {
		return true, nil
	}
	if c.upstream == "" {
		return false, nil
	}
	return gitrepo.New(c.weftPath).IsAncestor(c.upstream, "HEAD")
}

// recoverUnlanded undoes what a killed change left before its commit: the weft's paths go back to HEAD and `.lyx` back under the old anchor.
// Both `.lyx` directories standing is refused before anything is touched, since only the operator can merge them.
func (c *AnchorChange) recoverUnlanded() error {
	if !c.movesLyxDir() {
		return restoreMovePathsToHead(c.recovered, c.weftPath, c.movePathspecs())
	}
	oldLyx, err := lyxDirPresent(c.weftPath, c.from)
	if err != nil {
		return err
	}
	newLyx, err := lyxDirPresent(c.weftPath, c.to)
	if err != nil {
		return err
	}
	if oldLyx && newLyx {
		return fmt.Errorf("the weft of %s holds both %s and %s; merge %s into %s by hand and remove it, then run %s again",
			c.pair.WorktreeName, filepath.Join(c.weftPath, c.from, lyxdirs.DotLyxDirName), filepath.Join(c.weftPath, c.to, lyxdirs.DotLyxDirName),
			filepath.Join(c.to, lyxdirs.DotLyxDirName), filepath.Join(c.from, lyxdirs.DotLyxDirName), c.rerun())
	}
	if err := restoreMovePathsToHead(c.recovered, c.weftPath, c.movePathspecs()); err != nil {
		return err
	}
	if newLyx {
		return c.moveLyxDir(c.recovered, c.to, c.from)
	}
	return nil
}

// movePathspecs returns the weft paths a move rewrites: the wired names under both anchors, and the marker.
func (c *AnchorChange) movePathspecs() []string {
	specs := append(anchorNamePathspecs(c.from, c.names), anchorNamePathspecs(c.to, c.names)...)
	return append(specs, lyxcwd.AnchorFileName)
}

// lyxDirPresent reports whether the weft at weftPath holds a `.lyx` directory under anchor.
func lyxDirPresent(weftPath, anchor string) (bool, error) {
	path := filepath.Join(weftPath, anchor, lyxdirs.DotLyxDirName)
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("inspect %s: %w", path, err)
	}
	return true, nil
}

// moveLyxDir renames the weft's `.lyx` directory from under the anchor src to under the anchor dst, through an os.Root at the weft.
// The destination anchor's directory is created first, since it may have left the tree with its last tracked file.
func (c *AnchorChange) moveLyxDir(rec *Mutations, src, dst string) error {
	root, err := os.OpenRoot(c.weftPath)
	if err != nil {
		return fmt.Errorf("open the weft root %s: %w", c.weftPath, err)
	}
	defer func() { _ = root.Close() }()
	if dst != "." {
		if err := root.MkdirAll(filepath.ToSlash(dst), 0o755); err != nil {
			return fmt.Errorf("create %s in %s: %w", dst, c.weftPath, err)
		}
	}
	oldPath, newPath := anchorJoin(filepath.ToSlash(src), lyxdirs.DotLyxDirName), anchorJoin(filepath.ToSlash(dst), lyxdirs.DotLyxDirName)
	if err := root.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("move %s to %s in %s: %w", oldPath, newPath, c.weftPath, err)
	}
	rec.Append(KindPathRenamed, filepath.Join(c.weftPath, filepath.FromSlash(newPath)), renamedFrom(c.weftPath, oldPath))
	return nil
}

// renamedFrom returns the hub-relative form of the weft-relative slash path rel, as a path_renamed entry names its source.
func renamedFrom(weftPath, rel string) string {
	return hubRelativeTarget(filepath.Dir(weftPath), filepath.Join(weftPath, filepath.FromSlash(rel)))
}

// writeRecord writes the move record for this change with this process's pid and start time.
// A standing record's anchors, kind and upstream tip are kept; a new record takes the change's own anchors and kind and no upstream tip, which Apply writes when it integrates.
func (c *AnchorChange) writeRecord(standing MoveRecord, hadRecord bool) error {
	record := MoveRecord{A: c.from, B: c.to, Kind: c.kind}
	if hadRecord {
		record = standing
	}
	record.PID = os.Getpid()
	record.StartTime = ownStartTime()
	return writeMoveRecordFile(c.weftPath, record)
}

// ownStartTime returns this process's start time as the move record writes it.
func ownStartTime() string {
	startTime, _ := proc.StartTime(os.Getpid())
	return startTime
}

// writeMoveRecordFile writes record as the move record of the weft worktree at weftPath.
func writeMoveRecordFile(weftPath string, record MoveRecord) error {
	if err := state.WriteJSON(MoveRecordPath(weftPath), moveRecordStateLockPath(weftPath), record); err != nil {
		return fmt.Errorf("fabricengine: write the move record in %s: %w", weftPath, err)
	}
	return nil
}

// Apply carries the change out: it unwires the pair at the old anchor, moves or integrates the weft, then wires it, writes its launchers and regenerates the workspace files at the new anchor.
// landed reports whether the weft's commit or branch move happened, which decides what a re-run does.
//
// A failure before the weft is committed or its branch moved restores the working tree, the index and `.lyx`, rewires the old anchor and removes the record, and returns landed false;
// when that restore itself fails the record stays, landed is false, and the error names the re-run.
// A failure after it leaves the record and returns landed true, the error naming the re-run that skips the move and finishes the wiring.
// Apply does nothing for a change with nothing to do.
func (c *AnchorChange) Apply(rec *Mutations) (landed bool, err error) {
	if c.NoOp() {
		return false, nil
	}
	c.applied = true
	rec.Extend(c.recovered.Snapshot())

	if c.landed {
		if err := c.settleLyxDirAfterLanding(rec); err != nil {
			return true, c.afterLanding(err)
		}
	} else {
		committed, err := c.moveWeft(rec)
		if err != nil {
			if !committed {
				return false, err
			}
			return true, c.afterLanding(err)
		}
	}
	if err := c.wireAtTarget(rec); err != nil {
		return true, c.afterLanding(err)
	}
	return true, nil
}

// afterLanding wraps err with the re-run that finishes a change whose weft already carries it.
func (c *AnchorChange) afterLanding(err error) error {
	return fmt.Errorf("%w; the weft already carries the change, so the record stands: run %s to finish it", err, c.rerun())
}

// settleLyxDirAfterLanding moves `.lyx` to the new anchor for a re-run of a change that landed, when only the old anchor still holds it.
func (c *AnchorChange) settleLyxDirAfterLanding(rec *Mutations) error {
	if !c.movesLyxDir() {
		return nil
	}
	oldLyx, err := lyxDirPresent(c.weftPath, c.from)
	if err != nil {
		return err
	}
	newLyx, err := lyxDirPresent(c.weftPath, c.to)
	if err != nil {
		return err
	}
	if oldLyx && !newLyx {
		return c.moveLyxDir(rec, c.from, c.to)
	}
	return nil
}

// moveWeft holds the weft write lock and does the unwiring of the old anchor and the move or the integration of the weft.
// committed reports that the weft's commit or branch move happened, and then err is a failure after it.
// A failure before it is rolled back and returned with committed false.
func (c *AnchorChange) moveWeft(rec *Mutations) (committed bool, err error) {
	lockDir, err := ensureWeftLockDirAt(c.weftPath)
	if err != nil {
		return false, c.rollback(rec, err)
	}
	writeLock, err := lock.AcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))
	if err != nil {
		return false, c.rollback(rec, fmt.Errorf("acquire the weft write lock: %w", err))
	}
	defer func() { _ = writeLock.Release() }()

	if err := c.unwireAtSource(rec); err != nil {
		return false, c.rollback(rec, err)
	}
	if c.kind == MoveKindIntegrate {
		return c.integrate(rec)
	}
	committed, err = moveWeftContent(rec, c.weftPath, c.from, c.to, c.names)
	if !committed {
		return false, c.rollback(rec, err)
	}
	return true, err
}

// integrate rebuilds the weft on its upstream tip anchored at the new anchor, then moves `.lyx` after it.
// The upstream tip is written into the record before the branch moves, so a killed run's re-run can tell whether the integration landed.
func (c *AnchorChange) integrate(rec *Mutations) (committed bool, err error) {
	if c.upstream == "" {
		upstream, hasUpstream, err := upstreamSHAAt(c.weftPath)
		if err != nil {
			return false, c.rollback(rec, err)
		}
		if !hasUpstream {
			return false, c.rollback(rec, fmt.Errorf("the weft of %s has no fetched upstream to integrate", c.pair.WorktreeName))
		}
		c.upstream = upstream
	}
	record := MoveRecord{A: c.from, B: c.to, Kind: c.kind, U: c.upstream, PID: os.Getpid(), StartTime: ownStartTime()}
	if err := writeMoveRecordFile(c.weftPath, record); err != nil {
		return false, c.rollback(rec, err)
	}
	if err := runAnchorFailHook(false); err != nil {
		return false, c.rollback(rec, err)
	}
	if _, err := integrateWeft(rec, c.weftPath, c.upstream, c.to, c.names, c.from); err != nil {
		return false, c.rollback(rec, err)
	}
	if err := runAnchorFailHook(true); err != nil {
		return true, err
	}
	return true, c.settleLyxDirAfterLanding(rec)
}

// unwireAtSource removes the pair's junctions, their exclude entries and the root scratch link at the old anchor.
func (c *AnchorChange) unwireAtSource(rec *Mutations) error {
	source, slug := c.locationAt(c.from), c.pair.WorktreeName
	if _, err := unseedLyxJunction(rec, source, slug, c.names); err != nil {
		return err
	}
	if _, err := unseedGitExclude(rec, source, slug, c.names, false); err != nil {
		return err
	}
	return unlinkScratchLink(rec, source, slug)
}

// rollback undoes a failure before the weft was committed: it puts the weft's paths and `.lyx` back, rewires the old anchor and removes the record.
// It returns cause.
// Should an undoing step fail, it stops there, keeps the record and returns cause with the failure and the re-run that recovers the record.
func (c *AnchorChange) rollback(rec *Mutations, cause error) error {
	steps := []func() error{
		func() error { return restoreMovePathsToHead(rec, c.weftPath, c.movePathspecs()) },
		func() error {
			if !c.movesLyxDir() {
				return nil
			}
			if newLyx, err := lyxDirPresent(c.weftPath, c.to); err != nil || !newLyx {
				return err
			}
			return c.moveLyxDir(rec, c.to, c.from)
		},
		func() error { return WireJunctionsWith(rec, c.locationAt(c.from), c.pair.WorktreeName, c.names) },
		func() error { return RemoveMoveRecord(c.weftPath) },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return fmt.Errorf("%w; undoing the change failed: %v; the record stands, run %s to recover it", cause, err, c.rerun())
		}
	}
	return cause
}

// wireAtTarget wires the pair at the new anchor: its junctions and scratch link, its launchers, and the hub and beam workspace files.
// A pair with a launcher directory at the old anchor has it written at the new one and removed from the old;
// the prime has none, and loses the menu launcher of the old anchor instead.
func (c *AnchorChange) wireAtTarget(rec *Mutations) error {
	slug := c.pair.WorktreeName
	source, target := c.locationAt(c.from), c.locationAt(c.to)
	if err := WireJunctionsWith(rec, target, slug, c.names); err != nil {
		return err
	}
	prime, err := IsPrimeWorktree(source)
	if err != nil {
		return err
	}
	if prime {
		if err := removeMenuLauncher(rec, source); err != nil {
			return err
		}
	} else if _, err := os.Stat(LauncherDir(source, slug)); err == nil {
		if err := writeLaunchers(rec, target, slug); err != nil {
			return err
		}
		if err := removeLaunchers(rec, source, slug); err != nil {
			return err
		}
	}
	_, err = RegenerateHubWorkspace(rec, target)
	return err
}

// removeMenuLauncher removes the menu launcher of l's anchor, which opens the prime at that anchor.
// An absent launcher is no error, and the emptied launcher directories above it go too.
func removeMenuLauncher(rec *Mutations, l *lyxcwd.Location) error {
	menu := menuLauncherPath(l)
	req := pathRequest{
		what:      "remove the menu launcher of the old anchor",
		container: launchersDir(l),
		target:    menu,
		ownership: ownedUnderGeometryRoot(launchersDir(l)),
		dirtiness: dirtinessNA("launcher scripts are generated artifacts, never edited content"),
		force:     false,
	}
	if err := removePath(rec, req); err != nil {
		return fmt.Errorf("remove the menu launcher %s: %w", menu, err)
	}
	pruneEmptyAncestors(filepath.Dir(menu), launchersDir(l))
	return nil
}

// Finish ends the change: it removes the record and spawns the detached push of the pair's commit.
// A failed spawn is returned after the record is gone, and the next push of the pair carries the commit.
// Finish does nothing for a change with nothing to do.
func (c *AnchorChange) Finish(rec *Mutations) error {
	if c.NoOp() {
		return nil
	}
	if err := RemoveMoveRecord(c.weftPath); err != nil {
		return err
	}
	pair, err := lyxcwd.NamedWorktree(c.pair.HubPath, c.pair.WorktreeName)
	if err != nil {
		return err
	}
	spawn, err := SpawnStatusPush(pair, "")
	if err != nil {
		return fmt.Errorf("the change finished, but the push of %s is not spawned, so the next push of the pair carries the commit: %w", c.pair.WorktreeName, err)
	}
	if spawn.Spawned {
		for _, side := range []string{spawn.Request.WarpPath, spawn.Request.WeftPath} {
			if side != "" {
				rec.Append(KindPushSpawned, side, "detached")
			}
		}
	}
	return nil
}

// Close releases the move lock.
// A change whose Apply never ran removes the record BeginAnchorChange wrote with none standing before, so a verb that fails between the two leaves nothing; a record it found standing is kept.
// Close may be called more than once.
func (c *AnchorChange) Close() {
	if c == nil || c.closed {
		return
	}
	c.closed = true
	if c.moveLock == nil {
		return
	}
	if c.freshRecord && !c.applied {
		if err := RemoveMoveRecord(c.weftPath); err != nil {
			logger.Warn("fabricengine: the move record of an unapplied anchor change is not removed", "weft", c.weftPath, "error", err)
		}
	}
	if err := c.moveLock.Release(); err != nil {
		logger.Warn("fabricengine: the move lock is not released", "weft", c.weftPath, "error", err)
	}
}
