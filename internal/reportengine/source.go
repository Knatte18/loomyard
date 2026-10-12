// source.go declares the told geometry of a report, the read set the engine uses, and the single snapshot of refs every later read sees.

package reportengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// Geometry holds the absolute checkout paths and branch names a report reads, all told by the caller.
type Geometry struct {
	CodeRepo      string
	RecordsRepo   string
	BoardRepo     string
	ParentBranch  string
	RecordsBranch string
	BoardBranch   string
}

// Reader is the read set the engine uses over one repository; *gitrepo.Repo satisfies it.
type Reader interface {
	FileAtRevision(rev, relPath string) ([]byte, error)
	FilesInDirAtRevision(rev, dir string) ([]string, error)
	SubdirsAtRevision(rev, dir string) ([]string, error)
	Log(tip string, q gitrepo.LogQuery) ([]gitrepo.LogCommit, error)
	PreImageLines(sha string) (map[string][]int, error)
	BlameLines(rev, relPath string, lines []int) (map[int]string, error)
}

// Sources is one snapshot of the repositories a report reads.
// The tips and the archive tags are resolved once, so every later read sees the same history while runs land concurrently.
type Sources struct {
	Code    Reader
	Records Reader
	Board   Reader

	ParentBranch  string
	RecordsBranch string
	BoardBranch   string

	ParentTip  string
	RecordsTip string
	BoardTip   string

	// Tags are the archive tags of the records repository, oldest commit first.
	Tags []gitrepo.TagRef
}

// Open builds a reader per repository of g, resolves each branch tip once and lists the archive tags once.
// A branch that does not resolve is an error naming it.
func Open(g Geometry) (Sources, error) {
	code, records, board := gitrepo.New(g.CodeRepo), gitrepo.New(g.RecordsRepo), gitrepo.New(g.BoardRepo)

	parentTip, err := branchTip(code, g.ParentBranch)
	if err != nil {
		return Sources{}, err
	}
	recordsTip, err := branchTip(records, g.RecordsBranch)
	if err != nil {
		return Sources{}, err
	}
	boardTip, err := branchTip(board, g.BoardBranch)
	if err != nil {
		return Sources{}, err
	}
	tags, err := records.TagRefs(fabricengine.ArchiveTagPrefix)
	if err != nil {
		return Sources{}, fmt.Errorf("reportengine: list archive tags: %w", err)
	}

	return Sources{
		Code:          code,
		Records:       records,
		Board:         board,
		ParentBranch:  g.ParentBranch,
		RecordsBranch: g.RecordsBranch,
		BoardBranch:   g.BoardBranch,
		ParentTip:     parentTip,
		RecordsTip:    recordsTip,
		BoardTip:      boardTip,
		Tags:          tags,
	}, nil
}

// branchTip resolves the local branch of repo to its commit.
func branchTip(repo *gitrepo.Repo, branch string) (string, error) {
	tip, err := repo.RefSHA("refs/heads/" + branch)
	if err != nil {
		return "", fmt.Errorf("reportengine: resolve branch %q: %w", branch, err)
	}
	return tip, nil
}
