// profile.go parses a Go coverage profile into its blocks and derives the block sets a verdict compares.

package testcover

import (
	"bufio"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
)

// ProfileBlock is one block line of a Go coverage profile.
type ProfileBlock struct {
	// File is the block's file as the profile names it: the package import path joined with the file name.
	File string
	// StartLine and StartCol open the block, and EndLine and EndCol close it; columns count bytes from 1.
	StartLine, StartCol, EndLine, EndCol int
	Statements                           int
	Count                                int
}

// Position returns the block's file with its start and end, the form a profile line names the block by.
func (b ProfileBlock) Position() string {
	return fmt.Sprintf("%s:%d.%d,%d.%d", b.File, b.StartLine, b.StartCol, b.EndLine, b.EndCol)
}

// BlockSet is a set of block identities compared by equality, such as positions or rendered keys.
type BlockSet map[string]struct{}

// ParseProfile returns every block of a coverage profile, covered or not, in the profile's order.
// A line that is not a mode line and does not parse as a block is an error naming the line.
func ParseProfile(r io.Reader) ([]ProfileBlock, error) {
	var blocks []ProfileBlock
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		block, err := parseProfileLine(line)
		if err != nil {
			return nil, fmt.Errorf("malformed profile line %q: %w", line, err)
		}
		blocks = append(blocks, block)
	}
	return blocks, sc.Err()
}

// parseProfileLine parses one `file:startLine.startCol,endLine.endCol statements count` line.
func parseProfileLine(line string) (ProfileBlock, error) {
	fields := strings.Fields(line)
	if len(fields) != 3 {
		return ProfileBlock{}, fmt.Errorf("want a position, a statement count and a count, got %d fields", len(fields))
	}
	cut := strings.LastIndexByte(fields[0], ':')
	if cut < 0 {
		return ProfileBlock{}, fmt.Errorf("position %q names no file", fields[0])
	}
	block := ProfileBlock{File: fields[0][:cut]}
	if _, err := fmt.Sscanf(fields[0][cut+1:], "%d.%d,%d.%d", &block.StartLine, &block.StartCol, &block.EndLine, &block.EndCol); err != nil {
		return ProfileBlock{}, fmt.Errorf("position %q: %w", fields[0], err)
	}
	var err error
	if block.Statements, err = strconv.Atoi(fields[1]); err != nil {
		return ProfileBlock{}, err
	}
	if block.Count, err = strconv.Atoi(fields[2]); err != nil {
		return ProfileBlock{}, err
	}
	return block, nil
}

// PackageBlocks returns the blocks that lie in the files of the package at importPath itself.
// It leaves out blocks in a package nested below it.
func PackageBlocks(blocks []ProfileBlock, importPath string) []ProfileBlock {
	var own []ProfileBlock
	for _, b := range blocks {
		if path.Dir(b.File) == importPath {
			own = append(own, b)
		}
	}
	return own
}

// CoveredPositions returns the positions of the blocks with a non-zero count.
func CoveredPositions(blocks []ProfileBlock) BlockSet {
	covered := BlockSet{}
	for _, b := range blocks {
		if b.Count != 0 {
			covered[b.Position()] = struct{}{}
		}
	}
	return covered
}
