// manifest.go builds the launch manifest Codex's Prepare writes: one item per thing the session receives.
// Each candidate item names the text or hook command its delivery puts on the launch line,
// and the manifest keeps the candidate only when the line as built holds it.

package codexengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shellguard"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// The keys of the manifest items that are no skill, passage, file or shell-guard row.
const (
	promptEngineKey      = "prompt-engine"
	handWrittenPromptKey = "hand-written"
	questionDenyKey      = "question-deny"
	batchingNudgeItemKey = "batching-nudge"
	denyNoticeKey        = "deny-notice"
	creelTrustKey        = "creel-trust"
	subagentPassageKey   = "subagent-passage"
	sandboxModeKey       = "sandbox-mode"
	instructionsSizeKey  = "developer-instructions"
	heddleFileKeyPrefix  = "heddle-file/"
	// instructionPointerLinePrefix opens a line of the instruction-file pointers that names one CLAUDE.md.
	instructionPointerLinePrefix = "- "
)

// artifact names the place a candidate item's delivery is read back from.
type artifact int

const (
	// inNothing marks an item the provider loads itself; the manifest keeps it as listed.
	inNothing artifact = iota
	// inDeveloperInstructions is the text of the developer instructions setting on the launch line.
	inDeveloperInstructions
	// inLaunchLine is the launch line's own words, such as the skill load turn.
	inLaunchLine
	// inHookSettings is the commands of the hook settings on the launch line.
	inHookSettings
	// inMarkedOverride is the anchor's AGENTS.override.md, read only when heddle's marker opens it.
	inMarkedOverride
)

// manifestCandidate is one item the launch may deliver.
// proof is the exact text or hook command the artifact holds when the delivery reached the launch.
type manifestCandidate struct {
	item  shuttleengine.ManifestItem
	in    artifact
	proof string
}

// manifestInputs are the artifacts of one launch as written, and the items its deliveries would give.
type manifestInputs struct {
	role         string
	promptEngine string
	// shell is the shell the launch line was composed for.
	shell shell.Shell
	// launchLine is the launch command buildLaunchCmd returned.
	launchLine string
	// developerInstructions is the text the developer instructions setting carries.
	developerInstructions string
	sandboxMode           string
	// agentsOverride is the text of the anchor's AGENTS.override.md, empty when it is absent.
	agentsOverride string
	candidates     []manifestCandidate
}

// codexManifest returns the manifest of a launch: each candidate its artifact holds, then the settings items.
func codexManifest(in manifestInputs) shuttleengine.Manifest {
	var items []shuttleengine.ManifestItem
	for _, candidate := range in.candidates {
		if in.holds(candidate) {
			items = append(items, candidate.item)
		}
	}
	settings := func(key, detail string) shuttleengine.ManifestItem {
		return shuttleengine.ManifestItem{Channel: shuttleengine.ChannelSettings, Key: key, Engine: engineName, Delivery: shuttleengine.DeliveryDelivered, Detail: detail}
	}
	items = append(items,
		settings(sandboxModeKey, in.sandboxMode),
		settings(instructionsSizeKey, fmt.Sprintf("%d bytes", len(developerInstructionsSetting(in.developerInstructions)))),
	)
	return shuttleengine.Manifest{Engine: engineName, Role: in.role, PromptEngine: in.promptEngine, Items: items}
}

// holds reports whether the artifact of candidate holds its proof.
func (in manifestInputs) holds(candidate manifestCandidate) bool {
	switch candidate.in {
	case inDeveloperInstructions:
		return in.lineHolds(tomlContent(candidate.proof))
	case inLaunchLine:
		return in.lineHolds(candidate.proof)
	case inHookSettings:
		return in.lineHolds("command=" + tomlString(candidate.proof))
	case inMarkedOverride:
		return strings.HasPrefix(in.agentsOverride, heddleengine.Marker) && strings.Contains(in.agentsOverride, candidate.proof)
	default:
		return true
	}
}

// lineHolds reports whether the launch line holds fragment as the shell quoting writes it.
// Both shells quote with single quotes, so the quoted form of a fragment, less its quotes, is what a line holds of it.
func (in manifestInputs) lineHolds(fragment string) bool {
	quoted := in.shell.Quote(fragment)
	return strings.Contains(in.launchLine, quoted[1:len(quoted)-1])
}

// tomlContent returns s as a TOML basic string's content, without the quotes that frame it.
func tomlContent(s string) string {
	quoted := tomlString(s)
	return quoted[1 : len(quoted)-1]
}

// launchFacts is what Prepare composed for a launch, which its manifest is read back against.
type launchFacts struct {
	spec  shuttleengine.Spec
	codex Config
	// refuseRaw is the hub's refuse list, which the shell guard rows are built over.
	refuseRaw []string
	// lymPath is the lym binary the guard rows' live checks run.
	lymPath  string
	setup    shuttleengine.UserSetup
	standing shuttleengine.Standing
	// denyNotice and subagentPassage are the developer instructions' notice and passage, empty when it has none.
	denyNotice      string
	subagentPassage string
	// instructions is the text of the developer instructions.
	instructions string
	// unread is the CLAUDE.md files in the checkout that the instruction-file pointers name.
	unread      []string
	found       checkout
	workDir     string
	skillRoots  []string
	runDir      string
	sandboxMode string
	shell       shell.Shell
	launchLine  string
}

// writeLaunchManifest writes the launch's manifest into launched.runDir.
// It reads the artifacts back from the launch line and the anchor's override file,
// so a delivery dropped from Prepare drops its item.
func writeLaunchManifest(launched launchFacts) error {
	overridePath := filepath.Join(launched.workDir, agentsOverrideFile)
	override, err := os.ReadFile(overridePath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("codexengine: read back %s for the launch manifest: %w; way forward: fix its permissions", overridePath, err)
	}
	in := manifestInputs{
		role:                  launched.spec.Role,
		promptEngine:          launched.spec.PromptEngine,
		shell:                 launched.shell,
		launchLine:            launched.launchLine,
		developerInstructions: launched.instructions,
		sandboxMode:           launched.sandboxMode,
		agentsOverride:        string(override),
		candidates:            manifestCandidates(launched),
	}
	if err := shuttleengine.WriteManifest(launched.runDir, codexManifest(in)); err != nil {
		return fmt.Errorf("codexengine: write the launch manifest in %s: %w; way forward: make the run directory writable", launched.runDir, err)
	}
	return nil
}

// manifestCandidates lists every item the launch's deliveries may give.
func manifestCandidates(launched launchFacts) []manifestCandidate {
	candidates := []manifestCandidate{promptEngineCandidate(launched.spec)}
	candidates = append(candidates, standingCandidates(launched.standing)...)
	candidates = append(candidates, noticeCandidates(launched)...)
	candidates = append(candidates, guardCandidates(launched)...)
	if launched.codex.BatchingNudge.Enabled {
		statePath := filepath.Join(launched.runDir, batchingNudgeStateFileName)
		candidates = append(candidates, manifestCandidate{
			item:  shuttleengine.ManifestItem{Channel: shuttleengine.ChannelBatchingNudge, Key: batchingNudgeItemKey, Delivery: shuttleengine.DeliveryDelivered},
			in:    inHookSettings,
			proof: batchingNudgeHookCommand(statePath),
		})
	}
	for _, skill := range launched.spec.Skills {
		file := skillPath(skill, launched.skillRoots)
		if file == "" {
			continue
		}
		candidates = append(candidates, manifestCandidate{
			item:  shuttleengine.ManifestItem{Channel: shuttleengine.ChannelSkillLoad, Key: skill, Delivery: shuttleengine.DeliveryDelivered},
			in:    inLaunchLine,
			proof: file,
		})
	}
	return append(candidates, setupCandidates(launched)...)
}

// promptEngineCandidate returns the item naming how the spec's prompt was fitted to this engine.
func promptEngineCandidate(spec shuttleengine.Spec) manifestCandidate {
	key := promptEngineKey
	if spec.HandWrittenPrompt {
		key = handWrittenPromptKey
	}
	return manifestCandidate{item: shuttleengine.ManifestItem{Channel: shuttleengine.ChannelPromptEngine, Key: key, Delivery: shuttleengine.DeliveryDelivered}}
}

// standingCandidates returns one item per heddle passage in the developer instructions.
// It also returns one item per heddle file entry rendered into the anchor's override file.
func standingCandidates(standing shuttleengine.Standing) []manifestCandidate {
	var candidates []manifestCandidate
	for _, passage := range standing.Passages {
		candidates = append(candidates, manifestCandidate{
			item:  shuttleengine.ManifestItem{Channel: shuttleengine.ChannelStandingPassage, Key: passage.Slug, Engine: passage.Engine, Delivery: shuttleengine.DeliveryDelivered},
			in:    inDeveloperInstructions,
			proof: passage.Text,
		})
	}
	for _, entry := range standing.FileEntries {
		candidates = append(candidates, manifestCandidate{
			item:  shuttleengine.ManifestItem{Channel: shuttleengine.ChannelInstructionSource, Key: heddleFileKeyPrefix + entry.Slug, Engine: entry.Engine, Delivery: shuttleengine.DeliveryDelivered},
			in:    inMarkedOverride,
			proof: entry.Text,
		})
	}
	return candidates
}

// noticeCandidates returns the fixed sentences the developer instructions carry.
func noticeCandidates(launched launchFacts) []manifestCandidate {
	notices := []struct{ key, text string }{
		{denyNoticeKey, launched.denyNotice},
		{creelTrustKey, shuttleengine.CreelTrustSentence},
		{subagentPassageKey, launched.subagentPassage},
	}
	var candidates []manifestCandidate
	for _, notice := range notices {
		if notice.text == "" {
			continue
		}
		candidates = append(candidates, manifestCandidate{
			item:  shuttleengine.ManifestItem{Channel: shuttleengine.ChannelNotice, Key: notice.key, Delivery: shuttleengine.DeliveryDelivered},
			in:    inDeveloperInstructions,
			proof: notice.text,
		})
	}
	return candidates
}

// guardCandidates returns one item per PreToolUse deny lym may install, keyed by its engine-neutral name.
// The launch line holds a command only for a deny the launch installs.
func guardCandidates(launched launchFacts) []manifestCandidate {
	guard := func(key, command string) manifestCandidate {
		return manifestCandidate{
			item:  shuttleengine.ManifestItem{Channel: shuttleengine.ChannelGuard, Key: key, Delivery: shuttleengine.DeliveryDelivered},
			in:    inHookSettings,
			proof: command,
		}
	}
	candidates := []manifestCandidate{guard(questionDenyKey, questionDenyCommand())}
	for _, row := range shellguard.Rows(foregroundPhrase, launched.refuseRaw) {
		candidates = append(candidates, guard(row.Name, guardCommand(row, launched.lymPath)))
	}
	return candidates
}

// setupCandidates returns the items drawn from the user's Claude setup and the checkout's instruction files.
// Those are the setup's instruction files and the CLAUDE.md files Codex reads or is pointed to.
// They are also the skill listing and the plugin contexts.
func setupCandidates(launched launchFacts) []manifestCandidate {
	setup := launched.setup
	var candidates []manifestCandidate
	for _, file := range setup.InstructionFiles {
		candidates = append(candidates, manifestCandidate{
			item:  shuttleengine.ManifestItem{Channel: shuttleengine.ChannelInstructionSource, Key: file.Name, Delivery: shuttleengine.DeliveryDelivered},
			in:    inDeveloperInstructions,
			proof: file.Text,
		})
	}
	candidates = append(candidates, claudeFileCandidates(launched)...)
	if setup.SkillListing != "" {
		for _, skill := range setup.ListedSkills {
			candidates = append(candidates, manifestCandidate{
				item:  shuttleengine.ManifestItem{Channel: shuttleengine.ChannelSkillListing, Key: skill, Delivery: shuttleengine.DeliveryDelivered},
				in:    inDeveloperInstructions,
				proof: setup.SkillListing,
			})
		}
	}
	for _, context := range setup.PluginContexts {
		candidates = append(candidates, manifestCandidate{
			item:  shuttleengine.ManifestItem{Channel: shuttleengine.ChannelPluginContext, Key: context.Plugin + "/" + context.Hook, Delivery: shuttleengine.DeliveryDelivered},
			in:    inDeveloperInstructions,
			proof: context.Text,
		})
	}
	for _, hook := range setup.Omitted {
		// A hooks file that could not be read names no plugin, so its omission is keyed by the hook's own name.
		key := hook.Name
		if hook.Plugin != "" {
			key = hook.Plugin + "/" + hook.Hook
		}
		candidates = append(candidates, manifestCandidate{item: shuttleengine.ManifestItem{
			Channel: shuttleengine.ChannelPluginContext, Key: key, Delivery: shuttleengine.DeliveryOmitted, Reason: hook.Reason,
		}})
	}
	return candidates
}

// claudeFileCandidates returns one item per CLAUDE.md on the path from the checkout root to the working directory.
// A key is the file's slash-separated path relative to the root, outermost first.
// A file the instruction-file pointers name is delivered when the developer instructions hold its line;
// one Codex reads itself is native.
func claudeFileCandidates(launched launchFacts) []manifestCandidate {
	var candidates []manifestCandidate
	for dir := launched.workDir; ; dir = filepath.Dir(dir) {
		file := filepath.Join(dir, instructionFallbackFile)
		if info, err := os.Stat(file); err == nil && info.Mode().IsRegular() {
			if relative, err := filepath.Rel(launched.found.root, file); err == nil {
				candidates = append(candidates, claudeFileCandidate(filepath.ToSlash(relative), file, slices.Contains(launched.unread, file)))
			}
		}
		if dir == launched.found.root || dir == filepath.Dir(dir) {
			break
		}
	}
	slices.Reverse(candidates)
	return candidates
}

// claudeFileCandidate returns the item of the CLAUDE.md at file, keyed by its root-relative key.
func claudeFileCandidate(key, file string, pointed bool) manifestCandidate {
	if pointed {
		return manifestCandidate{
			item:  shuttleengine.ManifestItem{Channel: shuttleengine.ChannelInstructionSource, Key: key, Delivery: shuttleengine.DeliveryDelivered},
			in:    inDeveloperInstructions,
			proof: instructionPointerLinePrefix + file,
		}
	}
	return manifestCandidate{item: shuttleengine.ManifestItem{Channel: shuttleengine.ChannelInstructionSource, Key: key, Delivery: shuttleengine.DeliveryNative}}
}
