// slackbridge.go fills creel's MachineBridge seam from the Slack engine for the hub the daemon serves.

package creelcli

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/slackengine"
)

// slackBridge is creel's MachineBridge over one hub.
// It tells the daemon this machine's names and whether an orch may post to another machine.
// It also tells the daemon whether a post to the hub's relay program reaches Slack.
// Every call reads its inputs afresh, so a pressed button or a new token applies at the next post.
type slackBridge struct {
	// geometry reads the hub's Slack geometry.
	geometry func() (slackengine.Geometry, error)
	// beams lists the slugs of the hub's beams.
	beams func() ([]string, error)
	// names answers this machine's current name and its former names.
	names func() (string, []string, error)
}

// newSlackBridge returns the bridge of the hub at hubPath, reading the hub's files and this machine's names.
func newSlackBridge(hubPath string) *slackBridge {
	return &slackBridge{
		geometry: func() (slackengine.Geometry, error) { return hubgeom.SlackGeometry(hubPath) },
		beams:    func() ([]string, error) { return hubBeamSlugs(hubPath) },
		names:    machinename.Names,
	}
}

// hubBeamSlugs lists the slugs of the local beams of the hub at hubPath, read through the prime's dobby start.
func hubBeamSlugs(hubPath string) ([]string, error) {
	primeName := strings.TrimSuffix(filepath.Base(hubPath), fabricengine.HubSuffix)
	prime, err := lymcwd.NamedWorktree(hubPath, primeName)
	if err != nil {
		return nil, err
	}
	pairs, err := hubgeom.DobbyStart(prime).Beams()
	if err != nil {
		return nil, err
	}
	slugs := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		slugs = append(slugs, pair.Slug)
	}
	return slugs, nil
}

// Names answers this machine's current name and its former names.
func (b *slackBridge) Names() (string, []string, error) {
	return b.names()
}

// SendReady answers nil when orch may post to another machine.
// It refuses an orch that is neither the hub's own nor a beam's, a button that is off,
// and a machine or hub the Slack engine finds unconfigured; each refusal names its way forward.
func (b *slackBridge) SendReady(orch string) error {
	geom, err := b.geometry()
	if err != nil {
		return err
	}
	hasButton, err := b.hasButton(geom.Shortname, orch)
	if err != nil {
		return err
	}
	if !hasButton {
		return fmt.Errorf("creel: %s has no slack button, so it posts to no other machine; way forward: ask the hub's orch or a beam's orch to post it", orch)
	}
	on, err := slackengine.ForwardingOn(geom.ForwardingPath, orch)
	if err != nil {
		return err
	}
	if !on {
		return fmt.Errorf("creel: the slack button of %s is off, so its mail cannot cross machines; way forward: press the button on its status bar", orch)
	}
	_, _, err = slackengine.RequireConfigured(geom)
	return err
}

// RelayPostReady answers whether the hub's relay runs, for a post from sender to the relay's program.
// It refuses the senders whose mail the relay drops unposted:
// one that is no orch strand, an orch of neither the hub nor a beam, and an orch whose slack button is off.
// An orch thread counts as the orch of its worktree.
// Each refusal names its way forward.
// A read that fails refuses the post too, with a way forward.
// The daemon then cannot tell whether the relay would post it.
func (b *slackBridge) RelayPostReady(sender string) (bool, error) {
	geom, err := b.geometry()
	if err != nil {
		return false, relayUnreadable("the hub's Slack geometry", err)
	}
	if !creelengine.IsOrchStrand(sender) {
		return false, fmt.Errorf("creel: the Slack relay posts only an orch's mail and %s is no orch, so it would drop this post; "+
			"way forward: report the text in your pane, or ask your orch to post it", sender)
	}
	orch, err := agentname.Format(geom.Shortname, agentname.TaskSlug(sender), agentname.RoleOrch)
	if err != nil {
		return false, relayUnreadable("the orch name of "+sender, err)
	}
	hasButton, err := b.hasButton(geom.Shortname, orch)
	if err != nil {
		return false, relayUnreadable("the hub's beam listing", err)
	}
	if !hasButton {
		return false, fmt.Errorf("creel: %s has no slack button, so the Slack relay drops its mail; "+
			"way forward: report the text in your pane, or ask the hub's orch or a beam's orch to post it", orch)
	}
	on, err := slackengine.ForwardingOn(geom.ForwardingPath, orch)
	if err != nil {
		return false, relayUnreadable("the forwarding record", err)
	}
	if !on {
		return false, fmt.Errorf("creel: the slack button of %s is off, so the Slack relay drops its mail; "+
			"way forward: press the button on its status bar, or report the text in your pane", orch)
	}
	live, err := slackengine.RelayLive(geom)
	if err != nil {
		return false, relayUnreadable("the relay's lock", err)
	}
	return live, nil
}

// relayUnreadable refuses a post to the Slack relay because what could not be read.
// Nothing then says whether the relay would post it.
func relayUnreadable(what string, err error) error {
	return fmt.Errorf("creel: could not read %s, so the post to the Slack relay is refused: %w; "+
		"way forward: run lym slack status, or report the text in your pane", what, err)
}

// hasButton reports whether orch is the orch of the hub with shortname or the orch of one of its beams.
func (b *slackBridge) hasButton(shortname, orch string) (bool, error) {
	prime, err := agentname.Format(shortname, "", agentname.RoleOrch)
	if err != nil {
		return false, err
	}
	if orch == prime {
		return true, nil
	}
	slugs, err := b.beams()
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(slugs, func(slug string) bool {
		beamOrch, err := agentname.Format(shortname, slug, agentname.RoleOrch)
		return err == nil && beamOrch == orch
	}), nil
}
