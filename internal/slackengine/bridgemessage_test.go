// bridgemessage_test.go covers the bridge message's envelope: its format, its parse and the width creel bounds it by.

package slackengine_test

import (
	"math"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// bridgeAddress returns the cross-machine address of the agent with the given name parts on machine.
func bridgeAddress(machine, shortname, slug, role string) agentname.MachineAddress {
	return agentname.MachineAddress{Machine: machine, Name: agentname.Name{Shortname: shortname, Slug: slug, Role: role}}
}

func TestBridgeMessage(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	from := bridgeAddress("osl-1046", "ly", "", "orch")
	to := bridgeAddress("neve", "zz", "pay", "orch")
	roundTrips := []struct {
		name     string
		envelope slackengine.BridgeEnvelope
		body     string
		wantLine string
	}{
		{
			"MailCarriesNoReplyTo",
			slackengine.BridgeEnvelope{From: from, To: to, ID: 12},
			"hello",
			"[lym-bridge] from=osl-1046::ly:orch to=neve::zz:pay:orch id=12",
		},
		{
			"ReplyCarriesReplyTo",
			slackengine.BridgeEnvelope{From: from, To: to, ID: 13, ReplyTo: 7},
			"the answer",
			"[lym-bridge] from=osl-1046::ly:orch to=neve::zz:pay:orch id=13 reply-to=7",
		},
		{
			"SeveralLineBodyComesBackWhole",
			slackengine.BridgeEnvelope{From: from, To: to, ID: 14},
			"first\n\nthird line\n",
			"[lym-bridge] from=osl-1046::ly:orch to=neve::zz:pay:orch id=14",
		},
	}
	for _, tt := range roundTrips {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			text := slackengine.FormatBridgeMessage(tt.envelope, tt.body)
			if want := tt.wantLine + "\n" + tt.body; text != want {
				t.Errorf("FormatBridgeMessage() = %q; want %q", text, want)
			}
			envelope, body, ok := slackengine.ParseBridgeMessage(text)
			if !ok || envelope != tt.envelope || body != tt.body {
				t.Errorf("ParseBridgeMessage(%q) = %+v, %q, %v; want %+v, %q, true", text, envelope, body, ok, tt.envelope, tt.body)
			}
		})
	}

	refused := []struct {
		name string
		text string
	}{
		{"PlainText", "hello there"},
		{"MissingID", "[lym-bridge] from=osl::ly:orch to=neve::zz:orch\nbody"},
		{"ZeroID", "[lym-bridge] from=osl::ly:orch to=neve::zz:orch id=0\nbody"},
		{"SignedID", "[lym-bridge] from=osl::ly:orch to=neve::zz:orch id=+5\nbody"},
		{"ZeroReplyTo", "[lym-bridge] from=osl::ly:orch to=neve::zz:orch id=3 reply-to=0\nbody"},
		{"FieldsOutOfOrder", "[lym-bridge] to=neve::zz:orch from=osl::ly:orch id=3\nbody"},
		{"ReplyToBeforeID", "[lym-bridge] from=osl::ly:orch to=neve::zz:orch reply-to=2 id=3\nbody"},
		{"FromWithoutMachinePart", "[lym-bridge] from=ly:orch to=neve::zz:orch id=3\nbody"},
		{"ToWithAMachineFailingTheRule", "[lym-bridge] from=osl::ly:orch to=Neve_1::zz:orch id=3\nbody"},
		{"FromNamingAProgram", "[lym-bridge] from=osl::ly:SLACK to=neve::zz:orch id=3\nbody"},
		{"ExtraField", "[lym-bridge] from=osl::ly:orch to=neve::zz:orch id=3 reply-to=2 more=1\nbody"},
	}
	for _, tt := range refused {
		t.Run("Refuses"+tt.name, func(t *testing.T) {
			t.Parallel()
			if envelope, body, ok := slackengine.ParseBridgeMessage(tt.text); ok {
				t.Errorf("ParseBridgeMessage(%q) = %+v, %q, true; want false", tt.text, envelope, body)
			}
		})
	}

	t.Run("TheWidestEnvelopeLineStaysWithinCreelsBound", func(t *testing.T) {
		t.Parallel()
		sender := bridgeAddress(strings.Repeat("m", 63), "ly", "", "orch")
		receiver := bridgeAddress("osl", "zz", "", "orch")
		text := slackengine.FormatBridgeMessage(slackengine.BridgeEnvelope{From: sender, To: receiver, ID: math.MaxInt64, ReplyTo: math.MaxInt64}, "body")
		line, _, _ := strings.Cut(text, "\n")
		if bound := creelengine.MachineEnvelopeWidth("ly:orch", "osl::zz:orch"); len(line) > bound {
			t.Errorf("the widest envelope line is %d bytes; want at most creel's bound of %d", len(line), bound)
		}
	})
}
