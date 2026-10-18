// notify_test.go proves notifyPrime over fake creel clients: which identity posts the notice, when it falls back to the program, and what a refusal returns.

package orchcli

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/creelengine"
)

// fakePoster records each post as "<identity> <receiver>" and answers the error it is given.
type fakePoster struct {
	identity string
	posts    *[]string
	err      error
}

func (f fakePoster) Post(receiver, body string) (int64, error) {
	*f.posts = append(*f.posts, f.identity+" "+receiver+" "+body)
	return 1, f.err
}

func TestNotifyPrime_SignsAndPosts(t *testing.T) {
	t.Parallel()

	const line = "[batten notice] a-slug: blocked"
	refused := errors.New("creel: the receiver is not a listed strand")
	tests := []struct {
		name         string
		recorded     bool
		strandClaim  string
		strandErr    error
		programErr   error
		wantPosts    []string
		wantPosted   bool
		wantErrParts []string
	}{
		{name: "NoOrchStrandRecordedPostsNothing", recorded: false, strandClaim: "tst:task:driver"},
		{
			name: "NonOrchStrandClaimCreelAcceptsPostsOnceWithTheClaim", recorded: true, strandClaim: "tst:task:driver",
			wantPosts: []string{"strand tst:orch " + line}, wantPosted: true,
		},
		{
			name: "ClaimRefusedOutsideThePaneIsPostedOnceMoreAsTheProgram", recorded: true, strandClaim: "tst:task:driver",
			strandErr: fmt.Errorf("refused: %w", creelengine.ErrStrandClaimOutsidePane),
			wantPosts: []string{"strand tst:orch " + line, "program tst:orch " + line}, wantPosted: true,
		},
		{
			name: "OrchStrandClaimPostsAsTheProgramOnly", recorded: true, strandClaim: "tst:orch",
			wantPosts: []string{"program tst:orch " + line}, wantPosted: true,
		},
		{
			name: "EmptyClaimPostsAsTheProgramOnly", recorded: true,
			wantPosts: []string{"program tst:orch " + line}, wantPosted: true,
		},
		{
			name: "OtherRefusalPostsNothingMoreAndReturnsTheLineWithTheRefusal", recorded: true, strandClaim: "tst:task:driver",
			strandErr: refused,
			wantPosts: []string{"strand tst:orch " + line}, wantErrParts: []string{line, refused.Error()},
		},
		{
			name: "ProgramRefusalReturnsTheLineWithTheRefusal", recorded: true,
			programErr: refused,
			wantPosts:  []string{"program tst:orch " + line}, wantErrParts: []string{line, refused.Error()},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var posts []string
			clients := notifyClients{
				strand: func(string) (notifyPoster, error) {
					return fakePoster{identity: "strand", posts: &posts, err: tt.strandErr}, nil
				},
				program: func() (notifyPoster, error) {
					return fakePoster{identity: "program", posts: &posts, err: tt.programErr}, nil
				},
			}

			posted, err := notifyPrime(func() (string, bool, error) { return "tst:orch", tt.recorded, nil }, line, tt.strandClaim, clients)

			if !slices.Equal(posts, tt.wantPosts) {
				t.Errorf("posts = %q; want %q", posts, tt.wantPosts)
			}
			if posted != tt.wantPosted {
				t.Errorf("notifyPrime() posted = %v; want %v", posted, tt.wantPosted)
			}
			if len(tt.wantErrParts) == 0 {
				if err != nil {
					t.Errorf("notifyPrime() error = %v; want nil", err)
				}
				return
			}
			for _, part := range tt.wantErrParts {
				if err == nil || !strings.Contains(err.Error(), part) {
					t.Errorf("notifyPrime() error = %v; want it to hold %q", err, part)
				}
			}
		})
	}
}
