// manifest.go holds the launch manifest: the record of what a launched session receives, its file and its kept copy.
// Every engine's Prepare writes one into the run directory, and a launch keeps a copy at the run-directory root.

package shuttleengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/state"
)

// ManifestChannel names the way an item reaches a session.
type ManifestChannel string

// The channels a manifest compares between engines, and the one it only records.
const (
	// ChannelPromptEngine is the engine a prompt was rendered for.
	ChannelPromptEngine ManifestChannel = "prompt-engine"
	// ChannelEditDirective is the edit directive a spawned role's prompt carries.
	ChannelEditDirective ManifestChannel = "edit-directive"
	// ChannelStandingPassage is a heddle passage appended to the session's system prompt.
	ChannelStandingPassage ManifestChannel = "standing-passage"
	// ChannelInstructionSource is an instruction file, or a heddle file entry rendered into one, the session reads.
	ChannelInstructionSource ManifestChannel = "instruction-source"
	// ChannelSkillLoad is a skill the launch loads in the session's first turn.
	ChannelSkillLoad ManifestChannel = "skill-load"
	// ChannelSkillListing is a skill the provider lists for the session.
	ChannelSkillListing ManifestChannel = "skill-listing"
	// ChannelPluginContext is the context a plugin's session-start hook gives the session.
	ChannelPluginContext ManifestChannel = "plugin-context"
	// ChannelGuard is a tool call the session is denied.
	ChannelGuard ManifestChannel = "guard"
	// ChannelBatchingNudge is the hook that nudges the session to batch its calls.
	ChannelBatchingNudge ManifestChannel = "batching-nudge"
	// ChannelNotice is a fixed sentence lym appends to the session's instructions.
	ChannelNotice ManifestChannel = "notice"
	// ChannelSettings is the engine's own settings document; it is recorded and never compared.
	ChannelSettings ManifestChannel = "settings"
)

// ManifestDelivery says how an item reached the session.
type ManifestDelivery string

// The deliveries a manifest item has.
const (
	// DeliveryDelivered marks an item the launch wrote into an artifact of its own.
	DeliveryDelivered ManifestDelivery = "delivered"
	// DeliveryNative marks an item the provider loads itself, listed from the engine's resolution of it.
	DeliveryNative ManifestDelivery = "native"
	// DeliveryOmitted marks an item the session goes without; its Reason says why.
	DeliveryOmitted ManifestDelivery = "omitted"
)

// WebsterForkDenyKey is the guard key of the deny on `lym webster` that a session allowed to fork carries.
// It belongs to fork support, so a comparison asks for it only of an engine that meets the fork requirement.
const WebsterForkDenyKey = "webster-fork-deny"

// ManifestFileName is the file name of the manifest a launch writes into its run directory.
const ManifestFileName = "manifest.json"

// manifestsDirName is the directory at the run-directory root that holds the kept copy of each launch's manifest.
const manifestsDirName = "manifests"

// ErrHookUnrun is the error a hook runner returns for a hook it lists without running.
var ErrHookUnrun = errors.New("shuttle: the plugin hook is listed unrun")

// ManifestItem is one thing a session receives.
// Channel and Key identify it across engines.
// Key is engine-neutral.
// It is a heddle slug, a shell-guard row name, a skill name or `<plugin>/<hook command>`.
// For an instruction file it is the file's path, and for a notice a fixed notice key.
// Engine is empty for an item compared between engines, and names the engine for an item keyed to that engine alone.
// Reason says why an omitted item was left out, and Detail is free text such as a byte size or a sandbox mode.
type ManifestItem struct {
	Channel  ManifestChannel  `json:"channel"`
	Key      string           `json:"key"`
	Engine   string           `json:"engine,omitempty"`
	Delivery ManifestDelivery `json:"delivery"`
	Reason   string           `json:"reason,omitempty"`
	Detail   string           `json:"detail,omitempty"`
}

// Manifest is the record of what one launch gave its session.
// Verdict lists the omitted items, which WriteManifest sets, so a reader judges the launch from the record alone.
type Manifest struct {
	Engine       string         `json:"engine"`
	Role         string         `json:"role"`
	PromptEngine string         `json:"promptEngine"`
	Items        []ManifestItem `json:"items"`
	Verdict      []ManifestItem `json:"verdict"`
}

// Omitted returns every item whose delivery is omitted, in item order.
func (m Manifest) Omitted() []ManifestItem {
	var omitted []ManifestItem
	for _, item := range m.Items {
		if item.Delivery == DeliveryOmitted {
			omitted = append(omitted, item)
		}
	}
	return omitted
}

// WriteManifest writes m as <runDir>/manifest.json, with its Verdict set to the items m omits.
func WriteManifest(runDir string, m Manifest) error {
	m.Verdict = m.Omitted()
	path := filepath.Join(runDir, ManifestFileName)
	return state.WriteJSON(path, path+".lock", m)
}

// ReadManifest reads the manifest at path.
// found is false for an absent file; a file that does not decode is an error.
// A key the record holds that Manifest does not know is ignored.
func ReadManifest(path string) (Manifest, bool, error) {
	return state.ReadJSON[Manifest](path, path+".lock")
}

// MissingItems returns each item of from that in holds no counterpart of.
// Only a compared item counts: one with an empty Engine and a channel other than settings.
// An item matches by channel and key whatever its delivery, so each engine words its copy its own way.
func MissingItems(from, in Manifest) []ManifestItem {
	type identity struct {
		channel ManifestChannel
		key     string
	}
	held := make(map[identity]bool, len(in.Items))
	for _, item := range in.Items {
		held[identity{item.Channel, item.Key}] = true
	}
	var missing []ManifestItem
	for _, item := range from.Items {
		if item.Engine != "" || item.Channel == ChannelSettings {
			continue
		}
		if !held[identity{item.Channel, item.Key}] {
			missing = append(missing, item)
		}
	}
	return missing
}

// copyManifest keeps the manifest of the run in runDir as <root>/manifests/<runID>.json.
// An absent record writes nothing, so an engine that writes no manifest launches as before.
// Its errors name the way forward, since the launch only logs them.
func copyManifest(root, runID, runDir string) error {
	recordPath := filepath.Join(runDir, ManifestFileName)
	m, found, err := ReadManifest(recordPath)
	if err != nil {
		return fmt.Errorf("read the launch manifest %s: %w; way forward: when the file does not decode, fix the engine's shuttleengine.WriteManifest call in its Prepare, and when it cannot be read, fix the permissions of %s", recordPath, err, runDir)
	}
	if !found {
		return nil
	}
	copyPath := filepath.Join(root, manifestsDirName, runID+".json")
	if err := state.WriteJSON(copyPath, copyPath+".lock", m); err != nil {
		return fmt.Errorf("keep the launch manifest copy %s: %w; way forward: remove whatever blocks %s, or make it a writable directory with free space, so the next launch keeps its copy", copyPath, err, filepath.Join(root, manifestsDirName))
	}
	return nil
}

// isManifestsDir reports whether a run-directory root entry is the directory that holds the manifest copies.
func isManifestsDir(entry os.DirEntry) bool {
	return entry.Name() == manifestsDirName
}
