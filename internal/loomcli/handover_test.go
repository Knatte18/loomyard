package loomcli

import "testing"

// TestDecideHandover pins step 7's decision table: the four handovers, and --no-attach winning over
// each of them.
func TestDecideHandover(t *testing.T) {
	const (
		reedTmux  = "/tmp/tmux-1000/reed-sock,123,0"
		taskSess  = "task"
		otherSess = "operator"
	)
	tests := []struct {
		name           string
		noAttach       bool
		tmuxEnv        string
		reedOwns       bool
		currentSession string
		want           handover
	}{
		{"TmuxUnset_Attach", false, "", false, "", handoverAttach},
		{"ReedOwns_SameSession_Envelope", false, reedTmux, true, taskSess, handoverEnvelope},
		{"ReedOwns_OtherSession_Switch", false, reedTmux, true, otherSess, handoverSwitch},
		{"OtherServer_Hint", false, "/tmp/tmux-1000/default,9,0", false, "", handoverHint},
		{"ReedOwns_SessionUnreadable_Hint", false, reedTmux, true, "", handoverHint},

		{"NoAttach_TmuxUnset", true, "", false, "", handoverEnvelope},
		{"NoAttach_SameSession", true, reedTmux, true, taskSess, handoverEnvelope},
		{"NoAttach_OtherSession", true, reedTmux, true, otherSess, handoverEnvelope},
		{"NoAttach_OtherServer", true, "/tmp/tmux-1000/default,9,0", false, "", handoverEnvelope},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decideHandover(tt.noAttach, tt.tmuxEnv, tt.reedOwns, tt.currentSession, taskSess)
			if got != tt.want {
				t.Errorf("decideHandover(%v, %q, %v, %q, %q) = %v; want %v", tt.noAttach, tt.tmuxEnv, tt.reedOwns, tt.currentSession, taskSess, got, tt.want)
			}
		})
	}
}

// TestNoAttachFields_HintKey pins the envelope's key set with and without a hint: "hint" appears only
// when one is given, and "run_id" is always present.
func TestNoAttachFields_HintKey(t *testing.T) {
	without := noAttachFields("go", "slug", "slug", "/s/status.json", "")
	if _, ok := without["hint"]; ok {
		t.Errorf("noAttachFields with an empty hint carries a hint key: %v", without)
	}
	if len(without) != 5 || without["run_id"] != "slug" {
		t.Errorf("noAttachFields without a hint = %v; want the five keys attached, driver, slug, run_id, status_file", without)
	}

	with := noAttachFields("go", "slug", "slug", "/s/status.json", "attach from outside")
	if with["hint"] != "attach from outside" || len(with) != 6 {
		t.Errorf("noAttachFields with a hint = %v; want six keys including hint", with)
	}
}
