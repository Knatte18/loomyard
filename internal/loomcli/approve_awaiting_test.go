package loomcli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

func TestApproveVerb_AwaitingAndBlockedAtPublish(t *testing.T) {
	for _, state := range []shedengine.State{shedengine.StateAwaiting, shedengine.StateBlocked} {
		t.Run(string(state), func(t *testing.T) {
			d, written := approveFixture()
			d.readStatus = func() (shedengine.Status, bool, error) {
				return shedengine.Status{State: state, CurrentProducer: loomshed.NamePublish}, true, nil
			}
			var out bytes.Buffer
			if code := approveVerb(context.Background(), &out, d); code != 0 {
				t.Fatalf("exit = %d; want 0; out = %s", code, out.String())
			}
			if len(*written) != 1 {
				t.Errorf("wrote %d approvals; want 1", len(*written))
			}
		})
	}
}

func TestApproveVerb_AwaitingAtOtherProducerRefused(t *testing.T) {
	d, written := approveFixture()
	d.readStatus = func() (shedengine.Status, bool, error) {
		return shedengine.Status{State: shedengine.StateAwaiting, CurrentProducer: loomshed.NameFinalize}, true, nil
	}
	var out bytes.Buffer
	if code := approveVerb(context.Background(), &out, d); code != 1 {
		t.Fatalf("exit = %d; want 1; out = %s", code, out.String())
	}
	if !strings.Contains(out.String(), "not awaiting or blocked") {
		t.Errorf("output %q does not name both accepted states", out.String())
	}
	if len(*written) != 0 {
		t.Errorf("wrote %d approvals on a refusal; want none", len(*written))
	}
}

func TestApproveCmd_HelpNamesAwaitingOrBlocked(t *testing.T) {
	cmd := (&loomCLI{}).approveCmd()
	if !strings.Contains(cmd.Long, "awaiting or blocked at Publish") {
		t.Errorf("Long %q does not say \"awaiting or blocked at Publish\"", cmd.Long)
	}
}

func TestShouldReflectFriction_AwaitingNeverReflects(t *testing.T) {
	if shouldReflectFriction("/some/friction", shedengine.RunAwaiting) {
		t.Error("shouldReflectFriction = true for RunAwaiting; want false")
	}
}
