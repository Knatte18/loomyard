// landed.go finds the commit a finished run landed on the code repository, by the title of the change description archived at its tag, and counts the lines that commit changed.

package main

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// landingSlack widens a run's span each way when matching its landed commit, so a pull request merged on GitHub, whose server clock stamps the commit, still matches.
const landingSlack = time.Minute

// Reasons a finished run has no landed commit.
const (
	noDescription         = "no change description at its tag"
	noParsableDescription = "change description at its tag does not parse"
	noTitledCommit        = "no commit with its change description's title within its span"
	noShedRecord          = "no shed record read"
)

// summaryRel is the change description's slash-separated path in an archived tree.
var summaryRel = path.Join(filepath.ToSlash(loomengine.LandingDirRel()), summaryparser.FileName)

// landingRepo is the code repository runs land on: its commits found by subject, and the lines a commit changed.
type landingRepo interface {
	CommitsWithSubject(subject string) ([]gitrepo.SubjectCommit, error)
	ChangedLines(sha string) (int, error)
}

// codeRepo reads landings from the code repository at dir.
type codeRepo struct {
	*gitrepo.Repo
	dir string
}

// ChangedLines is the lines added plus the lines deleted between sha's first parent and sha, binary files left out.
// A commit with no parent changes every line it holds.
func (c codeRepo) ChangedLines(sha string) (int, error) {
	out, err := gitexec.Run([]string{"show", "--numstat", "--format=", "-m", "--first-parent", sha}, c.dir)
	if err != nil {
		return 0, fmt.Errorf("counting the lines %s changed in %s: %w", sha, c.dir, err)
	}
	return numstatLines(out), nil
}

// numstatLines sums the added and deleted counts of git's --numstat output; a binary file's "-" counts add nothing.
func numstatLines(out string) int {
	total := 0
	for _, line := range strings.Split(out, "\n") {
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 {
			continue
		}
		for _, count := range fields[:2] {
			if n, err := strconv.Atoi(count); err == nil {
				total += n
			}
		}
	}
	return total
}

// Landing is the commit a finished run landed and the lines it changed.
type Landing struct {
	// SHA is the landed commit, empty when none was found.
	SHA   string
	Lines int
	// Missing says why no landed commit was found, empty when SHA is set.
	Missing string
}

// findLanding finds the commit the run whose shed record is rec landed on code:
// the newest commit whose subject is the title of the change description archived at the record's tag, committed within span widened by landingSlack each way.
// A run with no description at its tag, a description that does not parse, or no such commit, gets a Landing naming why.
// A read that fails is an error naming the slug.
func findLanding(trees ArchiveTrees, code landingRepo, slug string, rec ShedRecord, span Interval) (Landing, error) {
	data, err := trees.FileAtRevision(rec.Tag.Commit, summaryRel)
	if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
		return Landing{Missing: noDescription}, nil
	}
	if err != nil {
		return Landing{}, fmt.Errorf("change description of %s from tag %s: %w", slug, rec.Tag.Name, err)
	}
	summary, err := summaryparser.ParseData(data, rec.Tag.Name+":"+summaryRel)
	if err != nil {
		return Landing{Missing: noParsableDescription + ": " + err.Error()}, nil
	}
	commits, err := code.CommitsWithSubject(summary.Title)
	if err != nil {
		return Landing{}, fmt.Errorf("landed commit of %s: %w", slug, err)
	}
	from, to := span.From.Add(-landingSlack), span.To.Add(landingSlack)
	for _, commit := range commits {
		if commit.Committed.Before(from) || commit.Committed.After(to) {
			continue
		}
		lines, err := code.ChangedLines(commit.SHA)
		if err != nil {
			return Landing{}, fmt.Errorf("landed commit of %s: %w", slug, err)
		}
		return Landing{SHA: commit.SHA, Lines: lines}, nil
	}
	return Landing{Missing: noTitledCommit}, nil
}
