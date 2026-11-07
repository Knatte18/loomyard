// crosshub.go implements the sending half of mail across hubs.
// It names other hubs, refuses verbs that stay inside one hub, and posts a strand's mail to another hub's strand.

package creelengine

import (
	"fmt"
	"math"

	"github.com/Knatte18/loomyard/internal/agentname"
)

// otherHub answers the shortname of the hub name belongs to when that is not this hub's.
// A name that is neither a strand's nor a program's is no name of another hub.
func (s *Store) otherHub(name string) (string, bool) {
	if parsed, err := agentname.Parse(name); err == nil {
		return parsed.Shortname, parsed.Shortname != s.geom.Shortname
	}
	if shortname, _, err := agentname.ParseProgram(name); err == nil {
		return shortname, shortname != s.geom.Shortname
	}
	return "", false
}

// refuseOtherHub refuses verb for a receiver that is a name of another hub, and answers nil for every other name.
// It needs no lookup of other hubs, so it holds for a daemon that serves no other hub too.
func (s *Store) refuseOtherHub(receiver, verb string) error {
	if _, other := s.otherHub(receiver); !other {
		return nil
	}
	return fmt.Errorf("creel: %s is a name of another hub, and %s stays inside one hub; way forward: lym creel post %s \"<text>\"", receiver, verb, receiver)
}

// widestStampedLine renders the longest line a message from sender from with body can take.
// It uses the marked form with the longer, untrusted stamp.
func widestStampedLine(from, body string) string {
	return MessageLine(Message{ID: math.MaxInt64, From: from, Body: body, ExpectReply: true, SendingHub: "hub", Trusted: false})
}

// validateStampedLine refuses an empty body and one whose stamped line does not fit one hook delivery.
func validateStampedLine(from, body string) error {
	if err := validateBodyPresent(body); err != nil {
		return err
	}
	if widest := widestStampedLine(from, body); len(widest) > boundaryClaimBudget {
		return lineOverBudget(widest)
	}
	return nil
}

// postOtherHub journals a strand's post to a strand of the hub shortname as an outbound message in this hub's journal.
// It answers the id of that message.
// It refuses a non-strand sender or receiver, a shortname that does not resolve to one hub, and a line over the cap.
// Nothing is delivered here.
func (s *Store) postOtherHub(sender Sender, receiver, shortname, body string, opts PostOptions) (int64, *Waiter, error) {
	if sender.Kind != KindStrand {
		return 0, nil, fmt.Errorf("creel: only a strand posts to another hub, and %s is none; way forward: post from a strand pane, or run lym creel post from the receiving hub's directory", sender.Name)
	}
	if _, err := agentname.Parse(receiver); err != nil {
		return 0, nil, fmt.Errorf("creel: %s is a program or EXTERNAL name of another hub, and only a strand of another hub takes mail; way forward: post to a strand of that hub, such as %s:orch", receiver, shortname)
	}
	hubPath, err := s.hubs.Lookup(shortname)
	if err != nil {
		return 0, nil, err
	}
	if err := validateStampedLine(sender.Name, body); err != nil {
		return 0, nil, err
	}
	var (
		id     int64
		waiter *Waiter
	)
	err = s.do(func() error {
		id = s.state.newID()
		if err := s.writer.append(record{
			Event: eventOutbound, ID: id, From: sender.Name, To: receiver, ReceivingHub: hubPath,
			Kind: kindMail, Body: body, ExpectReply: opts.ExpectReply || opts.Wait,
		}); err != nil {
			return err
		}
		s.wakeOutboundLocked(hubPath)
		if opts.Wait {
			var err error
			waiter, err = s.addWaiterLocked(waitKey{kind: waitReply, id: id})
			return err
		}
		return nil
	})
	return id, waiter, err
}
