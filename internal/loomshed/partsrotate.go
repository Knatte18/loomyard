// partsrotate.go implements the fresh-spawn preparations of the Parts-Write and Parts-Plan-Write rows:
// the rotations that archive an earlier attempt's parts file, part plans and join record before a new session starts.

package loomshed

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/partsparser"
)

// NewPartsDirRotator returns the fresh-spawn preparation for Parts-Write:
// the closure that archives every entry of the parts directory under anchorPath,
// so the new session starts from an empty directory.
// Entries move into a new archive-<stamp>[-N] directory under the parts directory, and earlier archives stay in place.
// An absent parts directory, or one with nothing to move, is a no-op that creates nothing.
// The closure returns an empty prompt amendment.
// A nil now defaults to time.Now.
func NewPartsDirRotator(anchorPath string, now func() time.Time) func() (string, error) {
	return func() (string, error) {
		if err := archivePartsEntries(anchorPath, func(string) bool { return false }, now); err != nil {
			return "", fmt.Errorf("loomshed: rotate stale parts directory: %w", err)
		}
		return "", nil
	}
}

// NewPartsPlanRotator returns the fresh-spawn preparation for Parts-Plan-Write:
// the closure that archives every part directory and the join record under anchorPath.
// It keeps the parts file and then rotates the plan directory.
// The parts archive runs first, so a retry never loses the prior-plan block.
// A failed step leaves the plan directory in place.
// On a retry, the parts archive finds nothing left to move,
// and the plan rotation still announces the prior plan.
// The closure returns the plan rotation's prompt amendment, which the producer sets as the chair's prior-plan block.
// stencilsDir and archiveWebster are passed on to the plan rotation.
// A nil now defaults to time.Now.
func NewPartsPlanRotator(anchorPath, stencilsDir string, archiveWebster func(dest string) error, now func() time.Time) func() (string, error) {
	rotatePlan := NewPlanDirRotator(anchorPath, stencilsDir, archiveWebster, now)
	partsFileName := filepath.Base(partsparser.File(anchorPath))
	return func() (string, error) {
		if err := archivePartsEntries(anchorPath, func(name string) bool { return name == partsFileName }, now); err != nil {
			return "", fmt.Errorf("loomshed: rotate stale parts directory: %w", err)
		}
		return rotatePlan()
	}
}

// archivePartsEntries moves the top-level entries of anchorPath's parts directory into a new archive directory.
// It skips entries that begin with partsparser.ArchivePrefix and entries for which keep returns true.
// Earlier archives stay in place, so archives never nest.
// An absent parts directory, or one with nothing to move, is a no-op that creates nothing.
// A nil now defaults to time.Now.
func archivePartsEntries(anchorPath string, keep func(name string) bool, now func() time.Time) error {
	if now == nil {
		now = time.Now
	}
	partsDir := partsparser.Dir(anchorPath)
	entries, err := os.ReadDir(partsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var moving []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), partsparser.ArchivePrefix) || keep(entry.Name()) {
			continue
		}
		moving = append(moving, entry.Name())
	}
	if len(moving) == 0 {
		return nil
	}

	archiveDir, err := firstFreePartsArchivePath(partsDir, now().UTC().Format(archiveTimestampFormat))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return err
	}
	for _, name := range moving {
		if err := os.Rename(filepath.Join(partsDir, name), filepath.Join(archiveDir, name)); err != nil {
			return err
		}
	}
	return nil
}

// firstFreePartsArchivePath returns the first unused archive directory path under partsDir.
// The name is partsparser.ArchivePrefix and stamp, followed by an empty suffix, then "-1", "-2", and so on.
// Any os.Stat error other than not-exist is returned as-is.
func firstFreePartsArchivePath(partsDir, stamp string) (string, error) {
	for n := 0; ; n++ {
		suffix := ""
		if n > 0 {
			suffix = fmt.Sprintf("-%d", n)
		}
		candidate := filepath.Join(partsDir, partsparser.ArchivePrefix+stamp+suffix)
		if _, err := os.Stat(candidate); err != nil {
			if os.IsNotExist(err) {
				return candidate, nil
			}
			return "", err
		}
	}
}
