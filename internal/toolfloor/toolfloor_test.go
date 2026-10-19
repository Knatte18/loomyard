// toolfloor_test.go covers the version-line parsing and floor comparison of CheckGit and CheckTmux.

package toolfloor_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/toolfloor"
)

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
		{name: "GitAtFloor", check: toolfloor.CheckGit, line: "git version 2.43.0", wantParse: true},
		{name: "GitAboveFloor", check: toolfloor.CheckGit, line: "git version 2.53.0", wantParse: true},
		{name: "GitWindowsSuffix", check: toolfloor.CheckGit, line: "git version 2.45.1.windows.1", wantParse: true},
		{name: "GitAppleSuffix", check: toolfloor.CheckGit, line: "git version 2.50.1 (Apple Git-155)", wantParse: true},
		{name: "GitHigherMajorLowerMinor", check: toolfloor.CheckGit, line: "git version 3.0.0", wantParse: true},
		{name: "GitMissingPatchCountsAsZero", check: toolfloor.CheckGit, line: "git version 2.43", wantParse: true},
		{name: "GitBelowFloor", check: toolfloor.CheckGit, line: "git version 2.42.0", wantParse: true, wantTool: "git", wantFound: "2.42.0", wantFloor: "2.43.0"},
		{name: "GitBelowFloorAppleSuffix", check: toolfloor.CheckGit, line: "git version 2.39.5 (Apple Git-154)", wantParse: true, wantTool: "git", wantFound: "2.39.5", wantFloor: "2.43.0"},
		{name: "GitUnparseable", check: toolfloor.CheckGit, line: "not git"},
		{name: "GitEmpty", check: toolfloor.CheckGit, line: ""},
		{name: "TmuxAtFloor", check: toolfloor.CheckTmux, line: "tmux 3.4", wantParse: true},
		{name: "TmuxReleaseLetter", check: toolfloor.CheckTmux, line: "tmux 3.4a", wantParse: true},
		{name: "TmuxAboveFloor", check: toolfloor.CheckTmux, line: "tmux 3.6", wantParse: true},
		{name: "TmuxMultiDigitMinor", check: toolfloor.CheckTmux, line: "tmux 3.10", wantParse: true},
		{name: "TmuxHigherMajor", check: toolfloor.CheckTmux, line: "tmux 4.0", wantParse: true},
		{name: "TmuxBelowFloor", check: toolfloor.CheckTmux, line: "tmux 3.3a", wantParse: true, wantTool: "tmux", wantFound: "3.3", wantFloor: "3.4"},
		{name: "TmuxNextBuild", check: toolfloor.CheckTmux, line: "tmux next-3.6"},
		{name: "TmuxMasterBuild", check: toolfloor.CheckTmux, line: "tmux master"},
		{name: "TmuxEmpty", check: toolfloor.CheckTmux, line: ""},
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
			var floorErr *toolfloor.FloorError
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
