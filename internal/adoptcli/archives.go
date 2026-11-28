// archives.go lists the owner archive tags for `lym adopt --archives`.
// They are the tags the old owner of a subject pushed whose commits the slug's branch on origin lacks.
// It also names that call in the end envelope of an adopt whose owner write stood.

package adoptcli

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/output"
)

// archiveDeps holds the seams of `lym adopt --archives`.
// ParentOf names the slug of the pair a slug was forked from, or "" for the prime.
// ReadArchives returns the owner archive tags of the slugs keep admits, sorted by slug.
// It returns only tags whose commit the slug's branch on origin does not contain.
type archiveDeps struct {
	ReadOwners   func() (boardengine.Owners, error)
	ParentOf     func(slug string) (string, error)
	ReadArchives func(keep func(slug string) bool) ([]fabricengine.OwnerArchive, error)
}

// archiveTag is one listed owner archive tag and the command that shows its commits.
type archiveTag struct {
	Tag    string `json:"tag"`
	Side   string `json:"side"`
	Commit string `json:"commit"`
	Branch string `json:"branch"`
	Look   string `json:"look"`
}

// archiveSlug is the listed owner archive tags of one slug, with the subject the slug moves with.
type archiveSlug struct {
	Slug    string       `json:"slug"`
	Subject string       `json:"subject"`
	Tags    []archiveTag `json:"tags"`
}

// runArchives runs `lym adopt --archives` over the subjects req names, which are beam projects or `root`.
// It prints the list on out.
// It refuses no subject, any flag but --archives, a slug the board does not hold and an entry that is not a beam project.
// It runs no step, and it reads and writes no adopt record.
// It returns the exit code.
func runArchives(deps archiveDeps, req request, flags callFlags, out io.Writer) int {
	if req.All || req.KeepLocal || req.DropLocal || req.NoOrch || flags.Discard || flags.Here {
		return output.Err(out, "lym adopt: --archives only lists archives and takes no other flag: run `lym adopt --archives <subject>...` alone")
	}
	if len(req.Slugs) == 0 {
		return output.Err(out, "lym adopt: --archives lists the archives of the subjects it names: name a beam project or `root`, as in `lym adopt --archives root`")
	}
	owners, err := deps.ReadOwners()
	if err != nil {
		return output.Err(out, fmt.Sprintf("lym adopt: read the board's owners: %v", err))
	}
	var subjects []string
	for _, subject := range req.Slugs {
		if err := checkArchiveSubject(deps, owners, subject); err != nil {
			return output.Err(out, err.Error())
		}
		if !slices.Contains(subjects, subject) {
			subjects = append(subjects, subject)
		}
	}

	subjectOf := map[string]string{}
	var placeErr error
	archives, err := deps.ReadArchives(func(slug string) bool {
		subject, err := archiveSubject(deps, owners, slug)
		if err != nil {
			placeErr = errors.Join(placeErr, err)
			return false
		}
		subjectOf[slug] = subject
		return slices.Contains(subjects, subject)
	})
	if err = errors.Join(err, placeErr); err != nil {
		return output.Err(out, fmt.Sprintf("lym adopt: read the owner archives: %v; way forward: fix the cause it names, then re-run `lym adopt --archives %s`", err, strings.Join(subjects, " ")))
	}

	listed := []archiveSlug{}
	for _, archive := range archives {
		if len(listed) == 0 || listed[len(listed)-1].Slug != archive.Slug {
			listed = append(listed, archiveSlug{Slug: archive.Slug, Subject: subjectOf[archive.Slug]})
		}
		last := &listed[len(listed)-1]
		last.Tags = append(last.Tags, archiveTag{
			Tag: archive.Tag, Side: archive.Side, Commit: archive.Commit, Branch: archive.Branch,
			Look: fmt.Sprintf("git -C %s log %s", archive.Dir, archive.Tag),
		})
	}
	return output.Ok(out, map[string]any{"subjects": subjects, "archives": listed})
}

// checkArchiveSubject refuses a subject that is neither `root` nor a beam project the board holds.
// A task or grouping project is refused naming the subject that moves it.
func checkArchiveSubject(deps archiveDeps, owners boardengine.Owners, subject string) error {
	if subject == boardengine.RootSubject {
		return nil
	}
	entry, held := owners.Entries[subject]
	if !held {
		return fmt.Errorf("lym adopt: %q is not on the board: `lym board list` shows the entries", subject)
	}
	if entry.IsProject && entry.Beam {
		return nil
	}
	mover, err := owners.AdoptSubject(subject, deps.ParentOf)
	if err != nil {
		return fmt.Errorf("lym adopt: read the parent of %s: %w", subject, err)
	}
	return fmt.Errorf("lym adopt: %q is not a beam project, so its archives are listed with the subject that moves it: run `lym adopt --archives %s`", subject, mover)
}

// archiveSubject returns the subject an archived slug moves with:
// the slug itself for `root` and a beam project, else the nearest beam project above it, else `root`.
// A slug the board no longer holds moves with the subject of its parent.
func archiveSubject(deps archiveDeps, owners boardengine.Owners, slug string) (string, error) {
	if slug == boardengine.RootSubject {
		return slug, nil
	}
	if _, held := owners.Entries[slug]; held {
		return owners.AdoptSubject(slug, deps.ParentOf)
	}
	parent, err := deps.ParentOf(slug)
	if err != nil {
		return "", fmt.Errorf("read the parent of %s: %w", slug, err)
	}
	if parent == "" {
		return boardengine.RootSubject, nil
	}
	return owners.AdoptSubject(parent, deps.ParentOf)
}

// archivesCommand returns the `lym adopt --archives` call over the subjects whose owner write stood in result.
// The subjects are the beams sorted and then `root`; it returns "" when none stood.
// The old owner archives its unpushed tips only once it reads that write, so the call is one to repeat later.
func archivesCommand(result outcome) string {
	var subjects []string
	for _, slug := range result.Slugs {
		if slug.OwnerRecorded {
			subjects = append(subjects, slug.Slug)
		}
	}
	slices.Sort(subjects)
	if result.PrimeRecorded {
		subjects = append(subjects, boardengine.RootSubject)
	}
	if len(subjects) == 0 {
		return ""
	}
	return "lym adopt --archives " + strings.Join(subjects, " ")
}
