// channelname_test.go covers the names of an orch's channel and of a retired channel.

package slackengine_test

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// cutName returns the name a name over 80 characters is cut to: 73 characters, a dash and six hex digits of its SHA-256.
func cutName(name string) string {
	sum := sha256.Sum256([]byte(name))
	return name[:73] + "-" + hex.EncodeToString(sum[:])[:6]
}

func TestChannelName(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	day := time.Date(2026, 11, 8, 23, 0, 0, 0, time.UTC)
	// Each machine below joins "-neve-p1-x", which is 10 characters, to a whole name of 80 and of 81 characters.
	machine70 := strings.Repeat("a", 70)
	machine71 := strings.Repeat("a", 71)
	base79 := strings.Repeat("b", 79)

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"HubOrchRunsTheRootProject", slackengine.ChannelName("OSL46", "mo", "P0", "root"), "osl46-mo-p0-root"},
		{"BeamRunsItsProject", slackengine.ChannelName("OSL46", "mo", "P1", "adopt-takeover"), "osl46-mo-p1-adopt-takeover"},
		{"PartsAreLowercasedAndUnderscoreIsKept", slackengine.ChannelName("LY", "NEVE", "P1", "Pay_Ments"), "ly-neve-p1-pay_ments"},
		{"CharactersOutsideTheGrammarBecomeDashes", slackengine.ChannelName("ly", "neve+box 1", "P1", "pay.ments"), "ly-neve-box-1-p1-pay-ments"},
		{"EmptyHandleIsLeftOut", slackengine.ChannelName("ly", "neve", "", "payments"), "ly-neve-payments"},
		{"EmptyMachineIsLeftOut", slackengine.ChannelName("", "neve", "P1", "payments"), "neve-p1-payments"},
		{"EightyCharactersPassUnchanged", slackengine.ChannelName(machine70, "neve", "P1", "x"), machine70 + "-neve-p1-x"},
		{
			"EightyOneCharactersEndInTheHashOfTheWholeName",
			slackengine.ChannelName(machine71, "neve", "P1", "x"),
			cutName(machine71 + "-neve-p1-x"),
		},
		{"ArchivedFirstTryCarriesTheDate", slackengine.ArchivedName("ly-neve-p1-payments", day, 1), "ly-neve-p1-payments-2026-11-08"},
		{"ArchivedSecondTryAddsTwo", slackengine.ArchivedName("ly-neve-p1-payments", day, 2), "ly-neve-p1-payments-2026-11-08-2"},
		{"ArchivedThirdTryAddsThree", slackengine.ArchivedName("ly-neve-p1-payments", day, 3), "ly-neve-p1-payments-2026-11-08-3"},
		{"ArchivedLongBaseIsCutToEighty", slackengine.ArchivedName(base79, day, 1), cutName(base79 + "-2026-11-08")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Errorf("name = %q; want %q", tt.got, tt.want)
			}
			if len(tt.got) > 80 {
				t.Errorf("name %q is %d characters; want at most 80", tt.got, len(tt.got))
			}
		})
	}
}
