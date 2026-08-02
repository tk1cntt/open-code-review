package rules

import (
	"fmt"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// RefactorProfile holds numeric thresholds injected into refactor prompts.
type RefactorProfile struct {
	Name              string `json:"-"`
	MaxFnLines        int    `json:"max_fn_lines"`
	MaxParams         int    `json:"max_params"`
	MaxNesting        int    `json:"max_nesting"`
	MaxSwitchCases    int    `json:"max_switch_cases"`
	MaxFindings       int    `json:"max_findings"`
	MaxComponentLines int    `json:"max_component_lines"`
}

// CatalogRule is a structured refactor rule identity for filtering and reference.
type CatalogRule struct {
	ID              string `json:"id"`
	Category        string `json:"category"`
	Tier            string `json:"tier"`
	SeverityDefault string `json:"severity_default"`
	ConfidenceMin   string `json:"confidence_min"`
	Trigger         string `json:"trigger"`
	Action          string `json:"action"`
}

// RefactorRulePayloadStats describes the resolved refactor rule payload for telemetry.
type RefactorRulePayloadStats struct {
	Path       string
	Bytes      int
	Standalone bool
	Layers     []string // e.g. common, profile:go_idiomatic, family, language
	Profile    string
	Family     string
}

// matchFirstPathRule returns the first matching PathRule content for path.
func matchFirstPathRule(rules []PathRule, path string) (content string, ok bool) {
	lowerPath := strings.ToLower(path)
	for _, pr := range rules {
		for _, p := range expandBraces(pr.Pattern) {
			if matched, _ := doublestar.Match(strings.ToLower(p), lowerPath); matched {
				return pr.Rule, true
			}
		}
	}
	return "", false
}

// renderThresholdProfile formats numeric thresholds for the LLM prompt.
func renderThresholdProfile(p RefactorProfile) string {
	if p.Name == "" {
		p.Name = "default"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Active profile: `%s`\n", p.Name)
	fmt.Fprintf(&b, "- max function lines: **%d** (REF-COMPLEX-001)\n", p.MaxFnLines)
	fmt.Fprintf(&b, "- max parameters: **%d** (REF-COMPLEX-002)\n", p.MaxParams)
	fmt.Fprintf(&b, "- max nesting depth: **%d** (REF-COMPLEX-003)\n", p.MaxNesting)
	fmt.Fprintf(&b, "- max switch/match cases before dispatch extract: **%d**\n", p.MaxSwitchCases)
	fmt.Fprintf(&b, "- max findings per file: **%d** (REF-BUDGET-001)\n", p.MaxFindings)
	if p.MaxComponentLines > 0 {
		fmt.Fprintf(&b, "- max component/view lines: **%d**\n", p.MaxComponentLines)
	}
	b.WriteString("Use these numbers instead of any conflicting defaults in common rules.")
	return b.String()
}

// renderCatalogIndex renders a compact rule-ID checklist for the prompt.
func renderCatalogIndex(rules []CatalogRule) string {
	if len(rules) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("##### Rule ID Index (use in rule reference)\n")
	for _, r := range rules {
		fmt.Fprintf(&b, "- `%s` [%s/%s]: %s → %s\n",
			r.ID, r.Tier, r.Category, r.Trigger, r.Action)
	}
	return strings.TrimRight(b.String(), "\n")
}

// filterDisabledRefactorRules removes lines that mention any disabled rule ID.
func filterDisabledRefactorRules(text string, disabled []string) string {
	if text == "" || len(disabled) == 0 {
		return text
	}
	ids := make([]string, 0, len(disabled))
	for _, id := range disabled {
		id = strings.TrimSpace(id)
		if id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		skip := false
		for _, id := range ids {
			if strings.Contains(line, id) {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, line)
		}
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n")
}

// composeRefactorLayers builds the final refactor rule payload.
// Order: common → profile → family → language.
// Standalone (config/data) paths return language only (no common/family/profile).
func composeRefactorLayers(common, family, language string, profile *RefactorProfile, standalone bool) string {
	if standalone {
		return language
	}
	var parts []string
	if common != "" {
		parts = append(parts, common)
	}
	if profile != nil {
		parts = append(parts, "## Threshold Profile\n\n"+renderThresholdProfile(*profile))
	}
	if family != "" {
		parts = append(parts, "## Family-Specific Refactoring Rules (overrides common on the same topic)\n\n"+family)
	}
	if language != "" {
		parts = append(parts, "## Language-Specific Refactoring Rules (overrides family/common on the same topic)\n\n"+language)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// resolveProfileForPath returns the profile for path, or default, or nil if none loaded.
func (r *SystemRule) resolveProfileForPath(path, forcedName string) *RefactorProfile {
	if r == nil || len(r.RefactorProfiles) == 0 {
		return nil
	}
	name := forcedName
	if name == "" {
		if content, ok := matchFirstPathRule(r.RefactorProfileMap, path); ok {
			name = content
		}
	}
	if name == "" {
		name = "default"
	}
	if p, ok := r.RefactorProfiles[name]; ok {
		cp := p
		cp.Name = name
		return &cp
	}
	if p, ok := r.RefactorProfiles["default"]; ok {
		cp := p
		cp.Name = "default"
		return &cp
	}
	return nil
}

func (r *SystemRule) resolveFamilyForPath(path string) string {
	if r == nil {
		return ""
	}
	content, _ := matchFirstPathRule(r.RefactorFamilyMap, path)
	return content
}

// ResolveRefactorOptions customizes system refactor resolution for project layers.
type ResolveRefactorOptions struct {
	DisabledRuleIDs []string
	ForcedProfile   string
}

// ResolveRefactorWithOptions resolves refactor rules with optional project filters.
func (r *SystemRule) ResolveRefactorWithOptions(path string, opts ResolveRefactorOptions) string {
	if r == nil {
		return ""
	}
	lowerPath := strings.ToLower(path)
	standalone := isStandaloneRefactorPath(lowerPath)

	lang, langOK := matchFirstPathRule(r.RefactorPathRules, lowerPath)
	if !langOK {
		// Unmatched path: common only (plus forced profile if project set one).
		if opts.ForcedProfile != "" {
			if profile := r.resolveProfileForPath(lowerPath, opts.ForcedProfile); profile != nil {
				return filterDisabledRefactorRules(
					composeRefactorLayers(r.DefaultRefactorRule, "", "", profile, false),
					opts.DisabledRuleIDs,
				)
			}
		}
		return filterDisabledRefactorRules(r.DefaultRefactorRule, opts.DisabledRuleIDs)
	}

	if standalone {
		return filterDisabledRefactorRules(lang, opts.DisabledRuleIDs)
	}

	family := r.resolveFamilyForPath(lowerPath)
	profile := r.resolveProfileForPath(lowerPath, opts.ForcedProfile)
	text := composeRefactorLayers(r.DefaultRefactorRule, family, lang, profile, false)
	return filterDisabledRefactorRules(text, opts.DisabledRuleIDs)
}

// RefactorPayloadStats returns layer/size telemetry for a resolved path.
func (r *SystemRule) RefactorPayloadStats(path string) RefactorRulePayloadStats {
	stats := RefactorRulePayloadStats{Path: path}
	if r == nil {
		return stats
	}
	lower := strings.ToLower(path)
	standalone := isStandaloneRefactorPath(lower)
	stats.Standalone = standalone

	_, langOK := matchFirstPathRule(r.RefactorPathRules, lower)
	if standalone {
		if langOK {
			stats.Layers = []string{"language"}
		}
	} else {
		if r.DefaultRefactorRule != "" {
			stats.Layers = append(stats.Layers, "common")
		}
		if profile := r.resolveProfileForPath(lower, ""); profile != nil {
			stats.Profile = profile.Name
			stats.Layers = append(stats.Layers, "profile:"+profile.Name)
		}
		if family := r.resolveFamilyForPath(lower); family != "" {
			stats.Family = familyNameFromContent(family)
			stats.Layers = append(stats.Layers, "family")
		}
		if langOK {
			stats.Layers = append(stats.Layers, "language")
		}
	}
	stats.Bytes = len(r.ResolveRefactor(path))
	return stats
}

func familyNameFromContent(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#### Family:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "#### Family:"))
		}
	}
	return "family"
}
