//go:build tier0

// toolfloor_test.go covers the version-line parsing and floor comparison of CheckGit and CheckTmux, and the allow-set-title release check.

package toolfloor

import (
	"errors"
	"strings"
	"testing"
)

func TestTmuxHasAllowSetTitle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		line      string
		wantParse bool
		wantHas   bool
	}{
		{name: "AtRelease", line: "tmux 3.5", wantParse: true, wantHas: true},
		{name: "AboveReleaseWithLetter", line: "tmux 3.5a", wantParse: true, wantHas: true},
		{name: "MultiDigitMinor", line: "tmux 3.10", wantParse: true, wantHas: true},
		{name: "Floor", line: "tmux 3.4", wantParse: true},
		{name: "FloorWithLetter", line: "tmux 3.4a", wantParse: true},
		{name: "Unparseable", line: "tmux next-3.6"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parsed, has := TmuxHasAllowSetTitle(tt.line)
			if parsed != tt.wantParse || has != tt.wantHas {
				t.Errorf("TmuxHasAllowSetTitle(%q) = (%v, %v); want (%v, %v)", tt.line, parsed, has, tt.wantParse, tt.wantHas)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		check     func(string) (bool, error)
		line      string
		wantParse bool
		// wantFound is the version a refusal reports; empty when the line is admitted.
		wantFound string
		wantTool  string
		wantFloor string
	}{
		{name: "GitAtFloor", check: CheckGit, line: "git version 2.43.0", wantParse: true},
		{name: "GitAboveFloor", check: CheckGit, line: "git version 2.53.0", wantParse: true},
		{name: "GitWindowsSuffix", check: CheckGit, line: "git version 2.45.1.windows.1", wantParse: true},
		{name: "GitAppleSuffix", check: CheckGit, line: "git version 2.50.1 (Apple Git-155)", wantParse: true},
		{name: "GitHigherMajorLowerMinor", check: CheckGit, line: "git version 3.0.0", wantParse: true},
		{name: "GitMissingPatchCountsAsZero", check: CheckGit, line: "git version 2.43", wantParse: true},
		{name: "GitBelowFloor", check: CheckGit, line: "git version 2.42.0", wantParse: true, wantTool: "git", wantFound: "2.42.0", wantFloor: "2.43.0"},
		{name: "GitBelowFloorAppleSuffix", check: CheckGit, line: "git version 2.39.5 (Apple Git-154)", wantParse: true, wantTool: "git", wantFound: "2.39.5", wantFloor: "2.43.0"},
		{name: "GitUnparseable", check: CheckGit, line: "not git"},
		{name: "GitEmpty", check: CheckGit, line: ""},
		{name: "TmuxAtFloor", check: CheckTmux, line: "tmux 3.4", wantParse: true},
		{name: "TmuxReleaseLetter", check: CheckTmux, line: "tmux 3.4a", wantParse: true},
		{name: "TmuxAboveFloor", check: CheckTmux, line: "tmux 3.6", wantParse: true},
		{name: "TmuxMultiDigitMinor", check: CheckTmux, line: "tmux 3.10", wantParse: true},
		{name: "TmuxHigherMajor", check: CheckTmux, line: "tmux 4.0", wantParse: true},
		{name: "TmuxBelowFloor", check: CheckTmux, line: "tmux 3.3a", wantParse: true, wantTool: "tmux", wantFound: "3.3", wantFloor: "3.4"},
		{name: "TmuxNextBuild", check: CheckTmux, line: "tmux next-3.6"},
		{name: "TmuxMasterBuild", check: CheckTmux, line: "tmux master"},
		{name: "TmuxEmpty", check: CheckTmux, line: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parsed, err := tt.check(tt.line)
			if parsed != tt.wantParse {
				t.Errorf("check(%q) parsed = %v; want %v", tt.line, parsed, tt.wantParse)
			}
			if tt.wantFound == "" {
				if err != nil {
					t.Errorf("check(%q) error = %v; want nil", tt.line, err)
				}
				return
			}
			var floorErr *FloorError
			if !errors.As(err, &floorErr) {
				t.Fatalf("check(%q) error = %v; want a *FloorError", tt.line, err)
			}
			if floorErr.Tool != tt.wantTool || floorErr.Found != tt.wantFound || floorErr.Floor != tt.wantFloor {
				t.Errorf("check(%q) = {%q %q %q}; want {%q %q %q}", tt.line, floorErr.Tool, floorErr.Found, floorErr.Floor, tt.wantTool, tt.wantFound, tt.wantFloor)
			}
			for _, part := range []string{tt.wantTool, tt.wantFound, tt.wantFloor, "or newer"} {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("check(%q) message %q lacks %q", tt.line, err.Error(), part)
				}
			}
		})
	}
}
