// config.go declares the roles lym starts and loads agents.yaml into a Config that answers skills and efforts.

package agentconfig

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

// Role is the key a role lym starts is listed under in agents.yaml.
type Role string

// The roles lym starts, one per skill list.
const (
	RoleArbiter           Role = "arbiter"
	RoleBoardTidy         Role = "board-tidy"
	RoleBouncerJudge      Role = "bouncer-judge"
	RoleBouncerSeed       Role = "bouncer-seed"
	RoleBurler            Role = "burler"
	RoleConflictResolver  Role = "conflict-resolver"
	RoleDarn              Role = "darn"
	RoleDescriber         Role = "describer"
	RoleDiscussion        Role = "discussion"
	RoleDiscussionAdvisor Role = "discussion-advisor"
	RoleDriftExpect       Role = "drift-expect"
	RoleDriftJudge        Role = "drift-judge"
	RoleDriver            Role = "driver"
	RoleFixReview         Role = "fix-review"
	RoleFriction          Role = "friction"
	RoleGateFixer         Role = "gate-fixer"
	RoleMergeFixer        Role = "merge-fixer"
	RoleOrch              Role = "orch"
	RolePlan              Role = "plan"
	RolePRReviewer        Role = "pr-reviewer"
	RoleRaddle            Role = "raddle"
	RoleSampler           Role = "sampler"
	RoleTackler           Role = "tackler"
	RoleTreadleJudge      Role = "treadle-judge"
	RoleTreadleTargeting  Role = "treadle-targeting"
	RoleWebster           Role = "webster"
)

// roles is every Role, in sort order.
var roles = []Role{
	RoleArbiter, RoleBoardTidy, RoleBouncerJudge, RoleBouncerSeed, RoleBurler, RoleConflictResolver,
	RoleDarn, RoleDescriber, RoleDiscussion, RoleDiscussionAdvisor, RoleDriftExpect, RoleDriftJudge,
	RoleDriver, RoleFixReview, RoleFriction, RoleGateFixer, RoleMergeFixer, RoleOrch, RolePlan,
	RolePRReviewer, RoleRaddle, RoleSampler, RoleTackler, RoleTreadleJudge, RoleTreadleTargeting, RoleWebster,
}

// Roles returns every role lym starts, in sort order.
func Roles() []Role {
	return slices.Clone(roles)
}

// ModuleName is the agents module's configreg name, the Name of the spec ConfigSpec returns.
const ModuleName = "agents"

const (
	// skillsKey holds each role's skill list.
	skillsKey = "skills"
	// effortsKey holds the effort table.
	effortsKey = "efforts"
	// defaultEffortKey is the efforts entry for a session or subagent no other entry names.
	defaultEffortKey = "default"
	// roleModelSeparator joins a role and a model in an efforts key.
	roleModelSeparator = "/"
)

// wayForward is the clause every content failure ends with.
const wayForward = `; way forward: fix it with "lym config agents" from the prime`

// effortVocabulary is every effort value Claude Code accepts, lowest first.
var effortVocabulary = []string{"low", "medium", "high", "xhigh", "max"}

// Efforts returns every effort value Claude Code accepts, lowest first.
func Efforts() []string {
	return slices.Clone(effortVocabulary)
}

// ErrUnusable marks a content failure of agents.yaml whose error already carries its own way forward.
var ErrUnusable = errors.New("agents config unusable")

// document holds agents.yaml's keys, the skill lists as raw nodes so Load can check their YAML shape.
type document struct {
	Skills  map[string]yaml.Node `yaml:"skills"`
	Efforts map[string]string    `yaml:"efforts"`
}

// ModelEffort is the effort a subagent on one model takes.
type ModelEffort struct {
	Model  string
	Effort string
}

// Config is the hub's resolved agents.yaml.
// The zero Config holds no role and no effort: Skills refuses every role and Effort returns "".
type Config struct {
	skills  map[Role][]string
	efforts map[string]string
}

// ConfigOpenMaps returns the agents.yaml keys whose entries are the operator's own.
func ConfigOpenMaps() []string {
	return []string{effortsKey}
}

// ConfigSpec returns the agents module's layered-load spec.
// The module is hub-wide, and a local override may not set its skill lists.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     ModuleName,
		Template: []byte(ConfigTemplate()),
		HubWide:  true,
		OpenMaps: ConfigOpenMaps(),
		Categories: map[string]configengine.Category{
			skillsKey:  configengine.CategoryGuard,
			effortsKey: configengine.CategoryNone,
		},
	}
}

// Load resolves the hub's agents.yaml through the layer stack under dirs.
// An absent file in every layer resolves the template.
// It refuses a role key outside Roles.
// It refuses a role whose value is not a list of non-empty strings without surrounding whitespace.
// It refuses an efforts map without a default entry, an empty effort and an effort outside Efforts.
// It refuses a `<role>/<model>` key whose role is outside Roles.
// A content failure returns an error marked configengine.ErrInvalid that wraps ErrUnusable and names the key.
// Any other failure, such as an unreadable file, is returned unwrapped.
func Load(dirs configengine.Dirs) (Config, error) {
	resolved, err := configengine.LoadOrTemplate(dirs, ConfigSpec())
	if err != nil {
		if errors.Is(err, configengine.ErrInvalid) {
			return Config{}, unusable("", err.Error())
		}
		return Config{}, err
	}
	var doc document
	if err := yaml.Unmarshal(resolved, &doc); err != nil {
		return Config{}, unusable("", err.Error())
	}
	skills, err := skillLists(doc.Skills)
	if err != nil {
		return Config{}, err
	}
	if err := checkEfforts(doc.Efforts); err != nil {
		return Config{}, err
	}
	return Config{skills: skills, efforts: doc.Efforts}, nil
}

// skillLists returns the skill list of every role, refusing a role key outside Roles and a role without a usable list.
func skillLists(nodes map[string]yaml.Node) (map[Role][]string, error) {
	for key := range nodes {
		if !slices.Contains(roles, Role(key)) {
			return nil, unusable(skillsKey, fmt.Sprintf("names %q, which is no role lym starts", key))
		}
	}
	lists := make(map[Role][]string, len(roles))
	for _, role := range roles {
		node, ok := nodes[string(role)]
		if !ok {
			return nil, unusable(skillsKey, fmt.Sprintf("holds no list for role %q", role))
		}
		list, err := stringList(skillsKey+"."+string(role), node)
		if err != nil {
			return nil, err
		}
		lists[role] = list
	}
	return lists, nil
}

// stringList returns node's entries in order when it is a list of non-empty strings without surrounding whitespace.
// A violation returns the marked content-failure error naming key.
func stringList(key string, node yaml.Node) ([]string, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, unusable(key, "must be a list of strings")
	}
	list := make([]string, 0, len(node.Content))
	for _, item := range node.Content {
		if item.Kind != yaml.ScalarNode || item.ShortTag() != "!!str" {
			return nil, unusable(key, "must be a list of strings")
		}
		if item.Value == "" {
			return nil, unusable(key, "holds an empty entry")
		}
		if item.Value != strings.TrimSpace(item.Value) {
			return nil, unusable(key, fmt.Sprintf("entry %q carries leading or trailing whitespace", item.Value))
		}
		list = append(list, item.Value)
	}
	return list, nil
}

// checkEfforts refuses an efforts map without a default entry and one holding an empty effort or one outside Efforts.
// It also refuses a `<role>/<model>` key naming no role.
func checkEfforts(efforts map[string]string) error {
	if efforts[defaultEffortKey] == "" {
		return unusable(effortsKey, fmt.Sprintf("needs a non-empty %q entry", defaultEffortKey))
	}
	for key, effort := range efforts {
		if effort == "" {
			return unusable(effortsKey+"."+key, "is empty")
		}
		if !slices.Contains(effortVocabulary, effort) {
			return unusable(effortsKey+"."+key, fmt.Sprintf("holds %q, which is none of %s", effort, strings.Join(effortVocabulary, ", ")))
		}
		role, model, paired := strings.Cut(key, roleModelSeparator)
		if paired && (model == "" || !slices.Contains(roles, Role(role))) {
			return unusable(effortsKey+"."+key, fmt.Sprintf("must name a role lym starts and a model around %q", roleModelSeparator))
		}
	}
	return nil
}

// unusable returns the marked content-failure error with detail and the way forward, naming key when it is not empty.
func unusable(key, detail string) error {
	if key == "" {
		return configengine.MarkInvalid(fmt.Errorf("agents config: %s (%w)%s", detail, ErrUnusable, wayForward))
	}
	return configengine.MarkInvalid(fmt.Errorf("agents config: %s: %s (%w)%s", key, detail, ErrUnusable, wayForward))
}

// Skills returns role's skill list followed by each entry of extra not already present, in order, as a fresh slice.
// extra carries skills the spawning code adds, such as the hub's language skills.
// It refuses a role the config holds no list for, which only a zero Config or a value outside Roles meets.
func (c Config) Skills(role Role, extra []string) ([]string, error) {
	own, ok := c.skills[role]
	if !ok {
		return nil, fmt.Errorf("agents config: role %q has no skill list; way forward: start the role under one of %v", role, roles)
	}
	merged := make([]string, 0, len(own)+len(extra))
	for _, skill := range append(slices.Clone(own), extra...) {
		if !slices.Contains(merged, skill) {
			merged = append(merged, skill)
		}
	}
	return merged, nil
}

// Effort returns the effort a session or subagent of role takes on model:
// the efforts entry for `<role>/<model>`, else role, else model, else default.
// An empty role or model skips the entries that name it; the zero Config returns "".
func (c Config) Effort(role Role, model string) string {
	keys := make([]string, 0, 4)
	if role != "" && model != "" {
		keys = append(keys, string(role)+roleModelSeparator+model)
	}
	if role != "" {
		keys = append(keys, string(role))
	}
	if model != "" {
		keys = append(keys, model)
	}
	for _, key := range append(keys, defaultEffortKey) {
		if effort, ok := c.efforts[key]; ok {
			return effort
		}
	}
	return ""
}

// ModelEfforts returns the effort a subagent of role takes on each model an efforts key names, sorted by model,
// and the effort it takes on any other model.
// A model is named by a key that is neither default nor a role, or by a `<role>/<model>` key of this role.
func (c Config) ModelEfforts(role Role) ([]ModelEffort, string) {
	var models []string
	for key := range c.efforts {
		keyRole, model, paired := strings.Cut(key, roleModelSeparator)
		switch {
		case paired && Role(keyRole) == role:
		case !paired && key != defaultEffortKey && !slices.Contains(roles, Role(key)):
			model = key
		default:
			continue
		}
		if !slices.Contains(models, model) {
			models = append(models, model)
		}
	}
	sort.Strings(models)
	efforts := make([]ModelEffort, len(models))
	for i, model := range models {
		efforts[i] = ModelEffort{Model: model, Effort: c.Effort(role, model)}
	}
	return efforts, c.Effort(role, "")
}
