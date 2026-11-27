// manifest.go declares the launch manifest: what one session receives, per channel and engine-neutral key.
// It reads and writes the record in a run directory and compares two manifests.
// It also keeps a launch's copy of the record at the run-directory root.

package shuttleengine

import (
	"errors"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/state"
)

// ManifestFileName is the record's file name inside a run directory.
const ManifestFileName = "manifest.json"

// manifestsDirName is the directory under the run-directory root that holds each launch's copy of its record.
// It holds no run.json, and nothing removes the copies.
const manifestsDirName = "manifests"

// ManifestChannel is the way a manifest item reaches a session.
type ManifestChannel string

// The channels of a manifest item.
// An item of ManifestChannelSettings is listed and never compared between engines.
const (
	ManifestChannelPromptEngine      ManifestChannel = "prompt-engine"
	ManifestChannelEditDirective     ManifestChannel = "edit-directive"
	ManifestChannelStandingPassage   ManifestChannel = "standing-passage"
	ManifestChannelInstructionSource ManifestChannel = "instruction-source"
	ManifestChannelSkillLoad         ManifestChannel = "skill-load"
	ManifestChannelSkillListing      ManifestChannel = "skill-listing"
	ManifestChannelPluginContext     ManifestChannel = "plugin-context"
	ManifestChannelGuard             ManifestChannel = "guard"
	ManifestChannelBatchingNudge     ManifestChannel = "batching-nudge"
	ManifestChannelNotice            ManifestChannel = "notice"
	ManifestChannelSettings          ManifestChannel = "settings"
)

// ManifestDelivery says how an item reached the session, or that it did not.
type ManifestDelivery string

// The deliveries of a manifest item.
const (
	// ManifestDelivered is an item the launch itself carries.
	ManifestDelivered ManifestDelivery = "delivered"
	// ManifestNative is an item the provider loads itself, listed from the engine's resolution of it.
	ManifestNative ManifestDelivery = "native"
	// ManifestOmitted is an item the launch goes without.
	ManifestOmitted ManifestDelivery = "omitted"
)

// The manifest keys every engine gives the same item under.
const (
	// PromptEngineKey keys the prompt rendered for an engine, and HandWrittenPromptKey a prompt written by hand.
	PromptEngineKey      = "prompt-engine"
	HandWrittenPromptKey = "hand-written"
	// QuestionDenyKey and WebsterForkDenyKey are the guard keys of the question-tool deny and the `lym webster` fork deny.
	QuestionDenyKey    = "question-deny"
	WebsterForkDenyKey = "webster-fork-deny"
	// BatchingNudgeKey keys the batching nudge.
	BatchingNudgeKey = "batching-nudge"
	// DenyNoticeKey, CreelTrustKey and SubagentPassageKey are the notice keys.
	DenyNoticeKey      = "deny-notice"
	CreelTrustKey      = "creel-trust"
	SubagentPassageKey = "subagent-passage"
)

// HeddleFileKeyPrefix opens the key of an instruction source that is a heddle entry rendered into a file;
// the entry's slug follows it.
const HeddleFileKeyPrefix = "heddle-file/"

// ErrHookUnrun is the error a hook runner returns for a hook it lists without running.
var ErrHookUnrun = errors.New("shuttle: the hook is listed and not run")

// ManifestItem is one thing a session receives.
type ManifestItem struct {
	Channel ManifestChannel `json:"channel"`
	// Key names the item the same way for every engine.
	Key string `json:"key"`
	// Engine is empty for a compared item, and names the engine for an item keyed to that engine alone.
	Engine   string           `json:"engine,omitempty"`
	Delivery ManifestDelivery `json:"delivery"`
	// Reason says why an omitted item was left out.
	Reason string `json:"reason,omitempty"`
	// Detail is free text about the item, such as a byte size or a sandbox mode.
	Detail string `json:"detail,omitempty"`
}

// Manifest is what one engine's launch gives a session.
type Manifest struct {
	Engine       string         `json:"engine"`
	Role         string         `json:"role,omitempty"`
	PromptEngine string         `json:"promptEngine,omitempty"`
	Items        []ManifestItem `json:"items"`
	// Verdict holds the items the launch goes without; WriteManifest sets it.
	Verdict []ManifestItem `json:"verdict"`
}

// Omitted returns the items the launch goes without.
func (m Manifest) Omitted() []ManifestItem {
	var omitted []ManifestItem
	for _, item := range m.Items {
		if item.Delivery == ManifestOmitted {
			omitted = append(omitted, item)
		}
	}
	return omitted
}

// MissingItems returns the compared items of from that in holds nothing of the same channel and key for.
// An item of in counts whatever its delivery.
// An item keyed to one engine, and a settings item, is not compared.
func MissingItems(from, in Manifest) []ManifestItem {
	type identity struct {
		channel ManifestChannel
		key     string
	}
	held := map[identity]bool{}
	for _, item := range in.Items {
		held[identity{item.Channel, item.Key}] = true
	}
	var missing []ManifestItem
	for _, item := range from.Items {
		if item.Engine != "" || item.Channel == ManifestChannelSettings {
			continue
		}
		if !held[identity{item.Channel, item.Key}] {
			missing = append(missing, item)
		}
	}
	return missing
}

// WriteManifest writes m as the record in runDir, with its Verdict set to the items m goes without.
func WriteManifest(runDir string, m Manifest) error {
	m.Verdict = m.Omitted()
	path := filepath.Join(runDir, ManifestFileName)
	return state.WriteJSON(path, path+".lock", m)
}

// ReadManifest reads the record in runDir, ignoring keys it does not know.
// found is false for an absent record.
func ReadManifest(runDir string) (m Manifest, found bool, err error) {
	path := filepath.Join(runDir, ManifestFileName)
	return state.ReadJSON[Manifest](path, path+".lock")
}

// copyManifest keeps a copy of the record in runDir as <root>/manifests/<runID>.json, which outlives the run directory.
// A run directory with no record gets no copy.
// A record that cannot be read or a copy that cannot be written is logged at Warn and fails nothing.
// The record in the run directory stands.
func copyManifest(root, runID, runDir string) {
	record, found, err := ReadManifest(runDir)
	if err != nil {
		logger.Warn("shuttle: the launch manifest is unreadable, so no copy is kept; way forward: when the file does not decode, fix the engine's shuttleengine.WriteManifest call in its Prepare, and when it cannot be read, fix the permissions of the run directory's manifest.json", "path", filepath.Join(runDir, ManifestFileName), "error", err)
		return
	}
	if !found {
		return
	}
	path := filepath.Join(root, manifestsDirName, runID+".json")
	if err := state.WriteJSON(path, path+".lock", record); err != nil {
		logger.Warn("shuttle: the launch manifest copy was not written; way forward: remove whatever blocks the manifests directory, or make it a writable directory with free space, so the next launch keeps its copy", "path", path, "error", err)
	}
}
