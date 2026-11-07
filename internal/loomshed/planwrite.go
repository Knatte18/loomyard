// planwrite.go implements the Plan-Write row's producer decorator, NewPlanWrite.
// It commits the plan once the wrapped producer reports an output.
// It also holds NewPlanDirRotator, which archives the stale plan directory before a fresh lead is spawned.

package loomshed

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// archiveTimestampFormat is the compact UTC stamp layout for plan-directory archive names.
// It must match the unexported constant of the same name in internal/shedadapters; this package cannot import it.
const archiveTimestampFormat = "20060102T150405Z"

// planWrite decorates inner with a commit step.
// Once inner.Call returns a nil error and a non-empty output pointer:
// planWrite invokes commit before returning that same verdict to the caller.
//
// It does no plan-directory rotation.
// Rotation moves the files a live plan lead may still be writing,
// so it must run only after the wrapped producer has probed for that lead, never before its Call.
//
// planWrite does not check the context for cancellation itself.
// The wrapped producer checks it on entry, as its first act,
// so a second check here would be duplicate work at the same seam.
// The wrapped producer owns the whole cancellation obligation.
//
// It is a distinct type from the Discussion-Write row's producer, although their logic is identical.
// A shape test in internal/loomrecipe tells the two rows' producers apart by concrete type.
// Merging the types would remove that guard.
type planWrite struct {
	name   string
	inner  shedengine.ShedProducer
	commit func() error
}

var _ shedengine.ShedProducer = (*planWrite)(nil)

// NewPlanWrite returns a producer named name that delegates to inner.
// It invokes commit whenever inner returns a nil error with a non-empty output pointer.
// The return type is the shedengine.ShedProducer interface, so code outside this package can build the producer.
func NewPlanWrite(name string, inner shedengine.ShedProducer, commit func() error) shedengine.ShedProducer {
	return &planWrite{name: name, inner: inner, commit: commit}
}

// Call implements shedengine.ShedProducer.
// It calls the wrapped producer's Call exactly once.
// When that call returns a non-nil error or an empty pointer.Path, Call returns its three results verbatim.
// Otherwise it runs commit, and the returned triple is (outcome, pointer, nil) rather than being forced to Done.
// The commit runs on Done and on a gate-failed Stuck;
// these are the only outcomes the wrapped producer reports with a non-empty pointer.
//
// A commit failure becomes a returned error, never shedengine.Stuck, on both paths.
// A git fault is infrastructure, not plan quality.
// A returned error persists failed and aborts the run, whereas Stuck persists blocked and bounces.
// Skipping the commit on a gate-failed Stuck would halt the run and leave the artifact uncommitted in a dirty tree.
// Committing it keeps the tree clean and the artifact durable, so the human can diagnose it.
// It does not certify the artifact.
func (p *planWrite) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	outcome, pointer, err := p.inner.Call(ctx)
	if err != nil || pointer.Path == "" {
		return outcome, pointer, err
	}

	// The seam is checked here rather than in the constructor.
	// The constructor returns the bare seam interface, with no error return through which to refuse a nil.
	// The registry entry guards the seam with requireSeam in internal/shedrecipe,
	// so no production path reaches this branch.
	// A nil commit would still be a nil dereference several producers into a long unattended run,
	// and a named error is diagnosable where a panic is not.
	if p.commit == nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: no commit seam wired; the produced plan would never be committed", p.name)
	}

	if err := p.commit(); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: commit produced artifacts: %w; way forward: the fault is transient, re-step the %s row", p.name, err, p.name)
	}

	return outcome, pointer, nil
}

// NewPlanDirRotator returns the closure that archives the plan directory's stale top-level ".md"
// files into a fresh archive-<stamp>[-N] subdirectory under it, resolving that directory from
// anchorPath via planparser.PlanDir. A nil now defaults to time.Now.
//
// The closure runs as the fresh-spawn preparation of the seat producer.
// It therefore runs only after the seat probe has established that no live lead is writing into the directory.
// Running it earlier moves 00-overview.md, the lead's sole declared output file, from under a lead that resumes.
// The shuttle's Wait polls for bare existence at the lead's output paths;
// a finished plan then never classifies Done and times out into a hard run failure.
//
// Only files move, never directories, so a second rotation can never nest a previous archive
// directory inside a new one. An absent plan directory, or one with no top-level .md file to move,
// is a no-op with a nil error and creates nothing.
//
// The closure's string result is the prompt amendment the seat producer sets as the table's Amendment.
// The seat engine appends it to the respawned lead's prompt.
// It is empty when nothing moved.
// Otherwise it is a blank line followed by loomengine.PriorPlanBlock.
// That block names the archive directory and the moved files, so the new session knows where the prior plan went.
// The lens review file (planparser.LensReviewFileName) and the lens reports beside it move with the other files;
// they are never named in the block.
// When they are the only stale files, the rotation still moves them and returns an empty amendment.
// There is no plan to carry forward.
// stencilsDir holds the loom-template-prior-plan stencil that the block renders from.
// The block is rendered before any file moves.
// A render failure is returned as the closure's error.
// The producer then aborts before spawning a session that would silently rewrite the plan.
// The plan stays in place on that failure;
// the next attempt renders the block again rather than finding nothing to announce.
//
// archiveWebster archives webster's run record into the directory it is handed.
// The closure calls it once, with the archive's webster subdirectory, when stale files are being rotated;
// it runs after the block renders and before any plan file moves.
// A failure of it, including a webster run holding its run lock, is returned as the closure's error;
// the plan stays in place, so a retry rotates again.
func NewPlanDirRotator(anchorPath, stencilsDir string, archiveWebster func(dest string) error, now func() time.Time) func() (string, error) {
	if now == nil {
		now = time.Now
	}
	return func() (string, error) {
		planDir, archiveDir, staleFiles, err := stalePlanRotation(anchorPath, now)
		if err != nil {
			return "", fmt.Errorf("loomshed: rotate stale plan directory: %w", err)
		}
		if len(staleFiles) == 0 {
			return "", nil
		}
		var announcedFiles []string
		for _, name := range staleFiles {
			if name != planparser.LensReviewFileName && !planparser.IsLensReportFileName(name) {
				announcedFiles = append(announcedFiles, name)
			}
		}
		var block string
		if len(announcedFiles) > 0 {
			block, err = loomengine.PriorPlanBlock(stencilsDir, archiveDir, announcedFiles)
			if err != nil {
				return "", fmt.Errorf("loomshed: announce prior plan: %w", err)
			}
		}
		if err := archiveWebster(filepath.Join(archiveDir, reworkPriorWebster)); err != nil {
			return "", fmt.Errorf("loomshed: archive webster run record: %w", err)
		}
		if err := rotateStalePlanDir(planDir, archiveDir, staleFiles); err != nil {
			return "", fmt.Errorf("loomshed: rotate stale plan directory: %w", err)
		}
		if block == "" {
			return "", nil
		}
		return "\n" + block, nil
	}
}

// ArchivedPlanWebsterDirs returns the webster subdirectory of every archive-* directory under planDir.
// Each one is where the plan rotation archives webster's run record.
// A subdirectory is listed whether or not it exists, since a rotation with no record to move creates none.
func ArchivedPlanWebsterDirs(planDir string) []string {
	entries, err := os.ReadDir(planDir)
	if err != nil {
		return nil
	}
	archivePrefix := planparser.ArchiveDirName("", "")
	var dirs []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), archivePrefix) {
			dirs = append(dirs, filepath.Join(planDir, e.Name(), reworkPriorWebster))
		}
	}
	return dirs
}

// stalePlanRotation plans a rotation without performing it.
// It returns the plan directory resolved from anchorPath and the first free archive-<stamp>[-N] path under it;
// it also returns the names of the plan directory's top-level ".md" files.
// Any other entry (a directory, or a non-.md file) is not listed and stays in place.
// It returns no names and touches nothing on disk when there is nothing to rotate:
// the plan directory is absent, or holds no top-level .md file.
// It resolves the plan directory via planparser.PlanDir(anchorPath);
// it never spells the "_lym" literal, which production path construction must not repeat.
func stalePlanRotation(anchorPath string, now func() time.Time) (planDir, archiveDir string, staleFiles []string, err error) {
	planDir = planparser.PlanDir(anchorPath)

	entries, err := os.ReadDir(planDir)
	if err != nil {
		if os.IsNotExist(err) {
			return planDir, "", nil, nil
		}
		return "", "", nil, err
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) != ".md" {
			continue
		}
		staleFiles = append(staleFiles, e.Name())
	}
	if len(staleFiles) == 0 {
		return planDir, "", nil, nil
	}

	stamp := now().UTC().Format(archiveTimestampFormat)
	archiveDir, err = firstFreePlanArchivePath(planDir, stamp)
	if err != nil {
		return "", "", nil, err
	}
	return planDir, archiveDir, staleFiles, nil
}

// rotateStalePlanDir creates archiveDir and moves each of staleFiles into it from planDir.
func rotateStalePlanDir(planDir, archiveDir string, staleFiles []string) error {
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return err
	}
	for _, name := range staleFiles {
		if err := os.Rename(filepath.Join(planDir, name), filepath.Join(archiveDir, name)); err != nil {
			return err
		}
	}
	return nil
}

// firstFreePlanArchivePath returns the first archive directory path under planDir that does not already exist.
// Each candidate comes from planparser.ArchiveDirName(stamp, suffix), with suffix "", "-1", "-2", and so on.
// An os.Stat error other than not-exist is returned as-is.
func firstFreePlanArchivePath(planDir, stamp string) (string, error) {
	for n := 0; ; n++ {
		suffix := ""
		if n > 0 {
			suffix = fmt.Sprintf("-%d", n)
		}
		candidate := filepath.Join(planDir, planparser.ArchiveDirName(stamp, suffix))
		if _, err := os.Stat(candidate); err != nil {
			if os.IsNotExist(err) {
				return candidate, nil
			}
			return "", err
		}
	}
}
