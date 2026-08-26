// entries_singlellm.go implements singleLLMEntry, the Constructor for the "SingleLLM" registry row:
// the only entry with real logic of its own, since its shedadapters.SpecSource closure composes the
// shuttleengine.Spec every Call fills a stencil template to produce.

package shedrecipe

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// singleLLMReservedTokens names the four geometry-derived tokens singleLLMEntry supplies to every
// stencil.Fill call unconditionally, so both the Config.tokens rejection check and the fill itself
// read this one declaration. A Config.tokens entry naming one of these is rejected at construction
// rather than silently overriding, or being overridden by, the geometry-derived value.
var singleLLMReservedTokens = map[string]bool{
	"anchor_path":   true,
	"output_files":  true,
	"stencils_dir":  true,
	"worktree_root": true,
}

// singleLLMEntry is the Constructor for the "SingleLLM" registry row: it validates cfg and env,
// resolves output_files against env.WorktreeRoot, composes a shedadapters.SpecSource closure over a
// stencilstore-named template and the union of the four reserved geometry tokens with Config.tokens,
// and returns shedadapters.NewSingleLLMProducer(name, specSource, env.Shuttle, env.Now, nil).
func singleLLMEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
	stencilName, err := configString(cfg, "stencil", true)
	if err != nil {
		return nil, err
	}
	outputFiles, err := configStringSlice(cfg, "output_files", true)
	if err != nil {
		return nil, err
	}
	model, err := configString(cfg, "model", false)
	if err != nil {
		return nil, err
	}
	effort, err := configString(cfg, "effort", false)
	if err != nil {
		return nil, err
	}
	version, err := configString(cfg, "version", false)
	if err != nil {
		return nil, err
	}
	role, err := configString(cfg, "role", false)
	if err != nil {
		return nil, err
	}
	interactive, err := configBool(cfg, "interactive", false)
	if err != nil {
		return nil, err
	}
	tokens, err := configStringMap(cfg, "tokens", false)
	if err != nil {
		return nil, err
	}
	if err := configRejectUnknown(cfg, "stencil", "output_files", "model", "effort", "version", "role", "interactive", "tokens"); err != nil {
		return nil, err
	}

	// All three roots are validated unconditionally, whether or not the stencil names the
	// corresponding marker: making the requirement depend on template text would let a stencil edit
	// change a recipe row's validity without the recipe changing.
	if err := requireAbsRoot("SingleLLM", "WorktreeRoot", env.WorktreeRoot); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("SingleLLM", "AnchorPath", env.AnchorPath); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("SingleLLM", "StencilsDir", env.StencilsDir); err != nil {
		return nil, err
	}
	if err := requireSeam("SingleLLM", "Shuttle", env.Shuttle); err != nil {
		return nil, err
	}

	// Absolute is not optional here -- SingleLLMProducer.Call rejects a non-absolute
	// spec.OutputFiles entry outright rather than resolving it, because resolution would need a
	// worktree root that adapter is barred from reading.
	resolvedOutputs := make([]string, len(outputFiles))
	for i, f := range outputFiles {
		resolved, err := resolveUnderRoot("SingleLLM", "output_files", env.WorktreeRoot, f)
		if err != nil {
			return nil, err
		}
		resolvedOutputs[i] = resolved
	}

	// Two construction-time rejections over Config.tokens, both naming the offending token: a
	// reserved name is rejected rather than silently overriding or being overridden, and an empty
	// or whitespace-only value is rejected because stencil.Fill treats an empty value exactly like a
	// missing one -- a token that legitimately has no value must carry an explicit placeholder, the
	// way shedadapters.Bouncer passes the literal "(none)".
	for tokenName, value := range tokens {
		if singleLLMReservedTokens[tokenName] {
			return nil, fmt.Errorf("shedrecipe: SingleLLM: config key \"tokens\" name %q is reserved", tokenName)
		}
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("shedrecipe: SingleLLM: config key \"tokens\" value for %q must not be empty or whitespace-only", tokenName)
		}
	}

	// The stencil.Fill token map is the union of two closed sets: the four reserved geometry-derived
	// names, whose values never change between Call invocations, and Config.tokens. It is built once
	// here rather than per Call, since none of its values depend on anything a later Call could
	// change.
	fillTokens := make(map[string]string, len(tokens)+4)
	for k, v := range tokens {
		fillTokens[k] = v
	}
	fillTokens["worktree_root"] = env.WorktreeRoot
	fillTokens["anchor_path"] = env.AnchorPath
	fillTokens["stencils_dir"] = env.StencilsDir
	fillTokens["output_files"] = strings.Join(resolvedOutputs, "\n")

	// This probe mirrors shedadapters.NewBouncer's own eager rubric probe: it exists so a mistyped
	// stencil name fails at construction rather than at first Call. The bytes it reads are discarded
	// -- the stencil is read again per Call below, so a stencil edited mid-run takes effect without a
	// restart.
	if _, err := stencilstore.Read(env.StencilsDir, stencilName); err != nil {
		return nil, fmt.Errorf("shedrecipe: SingleLLM: stencil %q: %w", stencilName, err)
	}

	specSource := func() (shuttleengine.Spec, error) {
		template, err := stencilstore.Read(env.StencilsDir, stencilName)
		if err != nil {
			return shuttleengine.Spec{}, fmt.Errorf("shedrecipe: SingleLLM: stencil %q: %w", stencilName, err)
		}

		// Do not call stencil.StripLeadingComment here: the stencil here is the template, and
		// stencil.Fill strips its own stamp banner. A future row injecting stencil content as a
		// token *value* would need to strip that value itself, the way shedadapters.Bouncer strips
		// its rubric before handing it to stencil.Fill as a value.
		filled, err := stencil.Fill(template, fillTokens)
		if err != nil {
			return shuttleengine.Spec{}, fmt.Errorf("shedrecipe: SingleLLM: fill stencil %q: %w", stencilName, err)
		}

		return shuttleengine.Spec{
			Prompt:      string(filled),
			OutputFiles: resolvedOutputs,
			Model:       model,
			Effort:      effort,
			Version:     version,
			Role:        role,
			Interactive: interactive,
			// Timeout, ForkSubagents, KeepPane, Round, Parent, and Display are deliberately left at
			// their zero values -- a decision, not an oversight: they are not recipe-authorable in
			// this task, and Round/Parent/Display are per-run strand-display values a recipe is the
			// wrong place for regardless.
		}, nil
	}

	return shedadapters.NewSingleLLMProducer(name, specSource, env.Shuttle, env.Now, nil), nil
}
