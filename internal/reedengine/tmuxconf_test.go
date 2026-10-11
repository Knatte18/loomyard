// tmuxconf_test.go — tests for the operator tmux config feature's path validation.

package reedengine

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
)

func TestTmuxConfPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(name string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("set -g @marker on\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	plain := write("tmux.conf")
	bracket := write("a[b].conf")
	spaced := write("a b.conf")
	dollar := write("a$b.conf")

	tests := []struct {
		name    string
		raw     string
		noHome  bool
		want    string
		wantErr string
	}{
		{name: "EmptyValue", raw: "", want: ""},
		{name: "HomeExpansion", raw: "~/tmux.conf", want: plain},
		{name: "HomeExpansionWithoutHome", raw: "~/tmux.conf", noHome: true, wantErr: "~/tmux.conf"},
		{name: "AbsoluteRegularFile", raw: plain, want: plain},
		{name: "RelativePath", raw: "tmux.conf", wantErr: "tmux.conf"},
		{name: "MissingFile", raw: filepath.Join(dir, "missing.conf"), wantErr: filepath.Join(dir, "missing.conf")},
		{name: "Directory", raw: dir, wantErr: dir},
		{name: "BracketInName", raw: bracket, wantErr: bracket},
		{name: "SpaceInName", raw: spaced, wantErr: spaced},
		{name: "DollarInName", raw: dollar, wantErr: dollar},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			home := dir
			if tt.noHome {
				home = ""
			}
			got, err := tmuxConfPath(tt.raw, home)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("tmuxConfPath(%q) error = %v, want none", tt.raw, err)
				}
				if got != tt.want {
					t.Errorf("tmuxConfPath(%q) = %q, want %q", tt.raw, got, tt.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("tmuxConfPath(%q) = %q, want an error", tt.raw, got)
			}
			for _, name := range []string{"tmux_conf", tmuxConfEnv, tt.wantErr} {
				if !strings.Contains(err.Error(), name) {
					t.Errorf("tmuxConfPath(%q) error = %q, want it to name %q", tt.raw, err, name)
				}
			}
		})
	}
}

// TestTmuxConfBootGuard pins the boot guard against the fake tmux.
// It swaps the process-global logger output, so it does not run parallel.
func TestTmuxConfBootGuard(t *testing.T) {
	const path = "/home/operator/tmux.conf"
	claimWithPath := "set-option -s @lyx_tmux_conf_claimed sourced ; source-file " + path
	claimWithoutPath := "set-option -s @lyx_tmux_conf_claimed empty"
	pins := []string{"set-option", "-g", "destroy-unattached", "off", ";", "set-option", "-s", "exit-unattached", "off"}
	withPins := func(head ...string) []string { return append(append([]string{}, head...), pins...) }

	guardEngine := func(t *testing.T) (*Engine, *fakeTmux, *bytes.Buffer, *bytes.Buffer) {
		t.Helper()
		var logBuf, warnBuf bytes.Buffer
		logger.SetOutput(&logBuf)
		t.Cleanup(func() { logger.SetOutput(os.Stderr) })
		e := newTestEngine(t)
		e.warnOut = &warnBuf
		fake := installFakeTmux(t, e)
		fake.answer("show-options", "off\noff\n", nil)
		return e, fake, &logBuf, &warnBuf
	}
	// identities scripts display-message to answer each read in turn.
	identities := func(fake *fakeTmux, answers ...string) {
		next := 0
		fake.answerFunc("display-message", func([]string) (string, error) {
			answer := answers[min(next, len(answers)-1)]
			next++
			return answer, nil
		})
	}

	t.Run("ServerIdentityIsTrimmedAndEmptyOnError", func(t *testing.T) {
		e, fake, _, _ := guardEngine(t)
		fake.answer("display-message", " 4242 1700000000\n", nil)
		if got := e.serverIdentityLocked(); got != "4242 1700000000" {
			t.Errorf("serverIdentityLocked() = %q, want the trimmed answer", got)
		}
		fake.answer("display-message", "", errors.New("no server running"))
		if got := e.serverIdentityLocked(); got != "" {
			t.Errorf("serverIdentityLocked() on error = %q, want empty", got)
		}
	})

	t.Run("BootGuardArgv", func(t *testing.T) {
		tests := []struct {
			name      string
			candidate bool
			path      string
			want      []string
		}{
			{"CandidateWithPath", true, path, withPins("if-shell", "-F", "#{@lyx_tmux_conf_claimed}", "", claimWithPath, ";")},
			{"CandidateWithoutPath", true, "", withPins("if-shell", "-F", "#{@lyx_tmux_conf_claimed}", "", claimWithoutPath, ";")},
			{"NonCandidate", false, path, pins},
		}
		for _, tt := range tests {
			if got := bootGuardArgv(tt.candidate, tt.path); !slices.Equal(got, tt.want) {
				t.Errorf("%s: bootGuardArgv(%v, %q) = %q, want %q", tt.name, tt.candidate, tt.path, got, tt.want)
			}
		}
	})

	t.Run("CandidacyFollowsTheServerIdentity", func(t *testing.T) {
		tests := []struct {
			name           string
			before         string
			after          []string
			wantFirstVerb  string
			wantClaimInArg bool
		}{
			{"IdentityChanged", "100 1", []string{"200 2"}, "if-shell", true},
			{"NoServerSeenBefore", "", []string{"200 2"}, "if-shell", true},
			{"JoinedTheSeenServer", "100 1", []string{"100 1"}, "set-option", false},
		}
		for _, tt := range tests {
			e, fake, _, _ := guardEngine(t)
			identities(fake, tt.after...)
			if err := e.guardBootSessionLocked(tt.before, path); err != nil {
				t.Fatalf("%s: guardBootSessionLocked() = %v, want nil", tt.name, err)
			}
			got := fake.ArgvFor(tt.wantFirstVerb)
			if len(got) != 1 || slices.Contains(got[0], claimWithPath) != tt.wantClaimInArg {
				t.Errorf("%s: %s calls = %q, want one call carrying the claim: %v", tt.name, tt.wantFirstVerb, got, tt.wantClaimInArg)
			}
		}
	})

	t.Run("GuardExitStatusIsOnlyAWarning", func(t *testing.T) {
		e, fake, logBuf, warnBuf := guardEngine(t)
		identities(fake, "200 2")
		fake.answer("if-shell", "", errors.New("exit status 1: /home/operator/tmux.conf:3: unknown command: bogus"))

		if err := e.guardBootSessionLocked("100 1", path); err != nil {
			t.Fatalf("guardBootSessionLocked() = %v, want nil", err)
		}
		for name, buf := range map[string]*bytes.Buffer{"warnOut": warnBuf, "log": logBuf} {
			for _, want := range []string{path, "unknown command: bogus"} {
				if !strings.Contains(buf.String(), want) {
					t.Errorf("%s = %q, want it to carry %q", name, buf, want)
				}
			}
		}
	})

	t.Run("SessionGoneFailsTheBootNamingTheConfig", func(t *testing.T) {
		e, fake, _, _ := guardEngine(t)
		identities(fake, "200 2")
		fake.answer("has-session", "", exitCodeErr{code: 1})

		err := e.guardBootSessionLocked("100 1", path)
		if err == nil {
			t.Fatal("guardBootSessionLocked() = nil, want an error for the vanished session")
		}
		for _, want := range []string{"tmux_conf", tmuxConfEnv, path} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q, want it to name %q", err, want)
			}
		}
	})

	t.Run("PinReadBackNotOffFailsTheBoot", func(t *testing.T) {
		for _, readBack := range []string{"on\noff\n", "off\non\n"} {
			e, fake, _, _ := guardEngine(t)
			identities(fake, "200 2")
			fake.answer("show-options", readBack, nil)
			if err := e.guardBootSessionLocked("100 1", path); err == nil {
				t.Errorf("guardBootSessionLocked() with read-back %q = nil, want an error", readBack)
			}
		}
	})
}
