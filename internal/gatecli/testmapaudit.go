// testmapaudit.go runs the audit round that ends a `lym gate testmap` pass:
// it re-measures the oldest passing entries of the maps at HEAD and counts each fresh result that differs.

package gatecli

import (
	"maps"
	"path"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/testcover"
)

// audit runs the closing audit round of a pass whose queue is empty.
// It re-measures up to auditTests passing entries of the maps of packages, oldest measurement first.
// It takes only maps that stand at the call's HEAD and record no failed build there.
// An entry with no recorded measurement time counts as the oldest.
// It runs only when the call holds a gate slot or one is free at once.
// It skips a package with an uncommitted or untracked Go file, or whose build lock another call holds.
// Each re-measured entry is replaced, and one whose fresh coverage differs counts as an audit mismatch;
// replacing an equal one moves its measurement time, so the next round reaches other entries.
// The error reports a slot, read or write failure, or a cancelled call, that ends the call.
func (r *testMapRun) audit(packages []string) error {
	if r.request.auditTests < 1 {
		return nil
	}
	type candidate struct {
		pkg, test string
		measured  time.Time
	}
	var candidates []candidate
	for _, pkg := range packages {
		m, found, err := testcover.ReadMap(r.mapsDir, pkg)
		if err != nil || !found || m.Commit != r.head || m.FailedAt(r.head) {
			continue
		}
		for _, test := range m.Tests {
			if !test.Measured() {
				continue
			}
			candidates = append(candidates, candidate{pkg: pkg, test: test.Name, measured: test.MeasuredAt})
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	slices.SortStableFunc(candidates, func(a, b candidate) int { return a.measured.Compare(b.measured) })
	candidates = candidates[:min(len(candidates), r.request.auditTests)]
	held, err := r.takeSlot(false)
	if err != nil || !held {
		return err
	}
	tests := map[string][]string{}
	for _, c := range candidates {
		tests[c.pkg] = append(tests[c.pkg], c.test)
	}
	for _, pkg := range slices.Sorted(maps.Keys(tests)) {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		dir := path.Join(r.moduleRel, pkg)
		if r.dirtyGoFile(dir) != "" {
			continue
		}
		files, err := r.fileHashes(r.head, dir)
		if err != nil {
			return err
		}
		buildLock, held, err := r.lockPackage(pkg)
		if err != nil {
			return err
		}
		if !held {
			continue
		}
		_, err = r.mapPackage(testMapJob{pkg: pkg, dir: dir, files: files, tests: tests[pkg], audit: true})
		releaseBuildLock(buildLock, pkg)
		if err != nil {
			return err
		}
	}
	return nil
}
