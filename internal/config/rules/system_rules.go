// Package rules loads system review rules and matches file paths against glob patterns.
package rules

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Resolver resolves a review rule for a file path.
type Resolver interface {
	Resolve(path string) string
	ResolveRefactor(path string) string
}

// PathRule is a single pattern→rule entry preserving declaration order.
type PathRule struct {
	Pattern string
	Rule    string
}

// SystemRule holds review rules loaded from an external JSON config.
type SystemRule struct {
	DefaultRule string     `json:"default_rule"`
	PathRules   []PathRule // ordered; first match wins

	// Refactoring rules (same structure as review rules, different purpose).
	DefaultRefactorRule string     `json:"default_refactor_rule"`
	RefactorPathRules   []PathRule `json:"refactor_rule_map,omitempty"`

	// Phase 2: family / profile / catalog (populated by LoadDefault).
	// RefactorFamilyMap: pattern → family markdown content (after load).
	RefactorFamilyMap []PathRule
	// RefactorProfileMap: pattern → profile name (string in Rule field).
	RefactorProfileMap []PathRule
	// RefactorProfiles: name → thresholds.
	RefactorProfiles map[string]RefactorProfile
	// RefactorCatalog: structured rule IDs.
	RefactorCatalog []CatalogRule

	// raw family file paths (name → relative path under rule_docs/), used only during load.
	refactorFamilyFiles map[string]string
}

// UnmarshalJSON preserves the key order from JSON's path_rule_map and
// refactor_rule_map objects.
func (r *SystemRule) UnmarshalJSON(data []byte) error {
	// Decode scalar fields normally.
	var wrapper struct {
		DefaultRule         string `json:"default_rule"`
		DefaultRefactorRule string `json:"default_refactor_rule"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return err
	}
	r.DefaultRule = wrapper.DefaultRule
	r.DefaultRefactorRule = wrapper.DefaultRefactorRule

	// Use json.Decoder with UseNumber to preserve order of path_rule_map keys.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Parse path_rule_map.
	if mapData, ok := raw["path_rule_map"]; ok && len(mapData) > 0 && string(mapData) != "null" {
		pathRules, err := parseOrderedRuleMap(mapData)
		if err != nil {
			return fmt.Errorf("path_rule_map: %w", err)
		}
		r.PathRules = pathRules
	}

	// Parse refactor_rule_map.
	if mapData, ok := raw["refactor_rule_map"]; ok && len(mapData) > 0 && string(mapData) != "null" {
		refRules, err := parseOrderedRuleMap(mapData)
		if err != nil {
			return fmt.Errorf("refactor_rule_map: %w", err)
		}
		r.RefactorPathRules = refRules
	}

	// Parse refactor_families: name → file path.
	if mapData, ok := raw["refactor_families"]; ok && len(mapData) > 0 && string(mapData) != "null" {
		var files map[string]string
		if err := json.Unmarshal(mapData, &files); err != nil {
			return fmt.Errorf("refactor_families: %w", err)
		}
		r.refactorFamilyFiles = files
	}

	// Parse refactor_family_map: pattern → family name (stored temporarily in Rule).
	if mapData, ok := raw["refactor_family_map"]; ok && len(mapData) > 0 && string(mapData) != "null" {
		famRules, err := parseOrderedRuleMap(mapData)
		if err != nil {
			return fmt.Errorf("refactor_family_map: %w", err)
		}
		r.RefactorFamilyMap = famRules
	}

	// Parse refactor_profile_map: pattern → profile name.
	if mapData, ok := raw["refactor_profile_map"]; ok && len(mapData) > 0 && string(mapData) != "null" {
		profRules, err := parseOrderedRuleMap(mapData)
		if err != nil {
			return fmt.Errorf("refactor_profile_map: %w", err)
		}
		r.RefactorProfileMap = profRules
	}

	return nil
}

func parseOrderedRuleMap(data json.RawMessage) ([]PathRule, error) {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	t, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("expected '{': %w", err)
	}
	if t != json.Delim('{') {
		return nil, fmt.Errorf("expected '{', got %v", t)
	}
	var rules []PathRule
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("read key: %w", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("expected string key, got %T", keyTok)
		}
		var value string
		if err := dec.Decode(&value); err != nil {
			return nil, fmt.Errorf("read value for %q: %w", key, err)
		}
		rules = append(rules, PathRule{Pattern: key, Rule: value})
	}
	return rules, nil
}

//go:embed system_rules.json rule_docs/* rule_docs/refactoring/* rule_docs/refactoring/family/* rule_docs/refactoring/cross_file/*
var rulesFS embed.FS

// LoadDefault parses the embedded system_rules.json and resolves rule file references.
func LoadDefault() (*SystemRule, error) {
	data, err := rulesFS.ReadFile("system_rules.json")
	if err != nil {
		return nil, fmt.Errorf("read embedded system_rules.json: %w", err)
	}
	var rule SystemRule
	if err := json.Unmarshal(data, &rule); err != nil {
		return nil, fmt.Errorf("unmarshal default system rules: %w", err)
	}
	content, err := rulesFS.ReadFile("rule_docs/" + rule.DefaultRule)
	if err != nil {
		return nil, fmt.Errorf("read default rule file %q: %w", rule.DefaultRule, err)
	}
	rule.DefaultRule = strings.TrimRight(string(content), "\n")
	for i := range rule.PathRules {
		content, err := rulesFS.ReadFile("rule_docs/" + rule.PathRules[i].Rule)
		if err != nil {
			return nil, fmt.Errorf("read rule file %q for pattern %q: %w", rule.PathRules[i].Rule, rule.PathRules[i].Pattern, err)
		}
		rule.PathRules[i].Rule = strings.TrimRight(string(content), "\n")
	}
	// Resolve refactoring rule files.
	if rule.DefaultRefactorRule != "" {
		content, err := rulesFS.ReadFile("rule_docs/" + rule.DefaultRefactorRule)
		if err != nil {
			return nil, fmt.Errorf("read default refactor rule file %q: %w", rule.DefaultRefactorRule, err)
		}
		rule.DefaultRefactorRule = strings.TrimRight(string(content), "\n")
	}
	for i := range rule.RefactorPathRules {
		content, err := rulesFS.ReadFile("rule_docs/" + rule.RefactorPathRules[i].Rule)
		if err != nil {
			return nil, fmt.Errorf("read refactor rule file %q for pattern %q: %w", rule.RefactorPathRules[i].Rule, rule.RefactorPathRules[i].Pattern, err)
		}
		rule.RefactorPathRules[i].Rule = strings.TrimRight(string(content), "\n")
	}

	// Phase 2: load family markdown by name, then rewrite family map to content.
	familyContent := map[string]string{}
	for name, rel := range rule.refactorFamilyFiles {
		content, err := rulesFS.ReadFile("rule_docs/" + rel)
		if err != nil {
			return nil, fmt.Errorf("read refactor family %q file %q: %w", name, rel, err)
		}
		familyContent[name] = strings.TrimRight(string(content), "\n")
	}
	for i := range rule.RefactorFamilyMap {
		name := rule.RefactorFamilyMap[i].Rule
		body, ok := familyContent[name]
		if !ok {
			return nil, fmt.Errorf("refactor_family_map pattern %q references unknown family %q",
				rule.RefactorFamilyMap[i].Pattern, name)
		}
		rule.RefactorFamilyMap[i].Rule = body
	}

	// Phase 2: load threshold profiles.
	if err := loadRefactorProfiles(&rule); err != nil {
		return nil, err
	}

	// Phase 2: load structured catalog and append ID index to common rules.
	if err := loadRefactorCatalog(&rule); err != nil {
		return nil, err
	}
	if idx := renderCatalogIndex(rule.RefactorCatalog); idx != "" && rule.DefaultRefactorRule != "" {
		rule.DefaultRefactorRule = rule.DefaultRefactorRule + "\n\n" + idx
	}

	return &rule, nil
}

func loadRefactorProfiles(rule *SystemRule) error {
	data, err := rulesFS.ReadFile("rule_docs/refactoring/profiles.json")
	if err != nil {
		// Optional file — ignore missing for forward compatibility.
		if strings.Contains(err.Error(), "file does not exist") || strings.Contains(err.Error(), "no such file") {
			return nil
		}
		// embed.FS returns path error; treat not exist as optional.
		return nil
	}
	var profiles map[string]RefactorProfile
	if err := json.Unmarshal(data, &profiles); err != nil {
		return fmt.Errorf("unmarshal refactoring/profiles.json: %w", err)
	}
	for name, p := range profiles {
		p.Name = name
		profiles[name] = p
	}
	rule.RefactorProfiles = profiles
	return nil
}

func loadRefactorCatalog(rule *SystemRule) error {
	data, err := rulesFS.ReadFile("rule_docs/refactoring/catalog.json")
	if err != nil {
		return nil // optional
	}
	var wrapper struct {
		Rules []CatalogRule `json:"rules"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return fmt.Errorf("unmarshal refactoring/catalog.json: %w", err)
	}
	rule.RefactorCatalog = wrapper.Rules
	return nil
}

// LoadCrossFileRefactorRule returns the embedded multi-file refactor rule pack.
func LoadCrossFileRefactorRule() (string, error) {
	content, err := rulesFS.ReadFile("rule_docs/refactoring/cross_file/common.md")
	if err != nil {
		return "", fmt.Errorf("read cross-file refactor rules: %w", err)
	}
	return strings.TrimRight(string(content), "\n"), nil
}

// RuleDetail contains the resolved rule along with metadata about its source.
type RuleDetail struct {
	Rule    string // rule text
	Source  string // "custom" | "project" | "global" | "system"
	Pattern string // glob pattern that matched, or "default" for fallback
}

// DetailResolver extends Resolver with source metadata.
type DetailResolver interface {
	ResolveDetail(path string) RuleDetail
}

// Resolve returns the rule text for a given file path.
// Patterns with brace expansion like "*.{go,py}" are expanded into "*.go", "*.py".
// The first match wins; if none match, it falls back to DefaultRule.
// Supports full glob syntax including ** for recursive directory matching.
func (r *SystemRule) Resolve(path string) string {
	lowerPath := strings.ToLower(path)
	for _, pr := range r.PathRules {
		expanded := expandBraces(pr.Pattern)
		for _, p := range expanded {
			if matched, _ := doublestar.Match(strings.ToLower(p), lowerPath); matched {
				return pr.Rule
			}
		}
	}
	return r.DefaultRule
}

// CanonicalConfig returns a deterministic, order-stable field list describing this
// rule set's effective rule-text configuration, for hashing into the run manifest's
// rule_config_sha256. It covers only rule-text resolution (default plus ordered
// pattern rules); include/exclude filtering is carried by FileFilter and hashed
// separately. Order is preserved because first match wins.
func (r *SystemRule) CanonicalConfig() []string {
	fields := []string{"layer", "system", "default", r.DefaultRule}
	for _, pr := range r.PathRules {
		fields = append(fields, "layer", "system", "pattern", pr.Pattern, "rule", pr.Rule)
	}
	return fields
}

func (r *SystemRule) resolveDetail(path string) RuleDetail {
	lowerPath := strings.ToLower(path)
	for _, pr := range r.PathRules {
		expanded := expandBraces(pr.Pattern)
		for _, p := range expanded {
			if matched, _ := doublestar.Match(strings.ToLower(p), lowerPath); matched {
				return RuleDetail{Rule: pr.Rule, Source: "system", Pattern: pr.Pattern}
			}
		}
	}
	return RuleDetail{Rule: r.DefaultRule, Source: "system", Pattern: "default"}
}

// standaloneRefactorPatterns are path globs whose refactor rules are self-contained
// (config, data, markup, IaC, i18n). They do NOT compose with common.md, which is
// oriented toward programming-language source.
//
// Keep in sync with config-like entries in system_rules.json refactor_rule_map
// (see REFACTOR_RULES_OPTIMIZATION_ADR.md Appendix B).
var standaloneRefactorPatterns = []string{
	"**/*.properties",
	"**/*{mapper,dao}*.xml",
	"**/pom.xml",
	"**/build.gradle",
	"**/package.json",
	"**/Cargo.toml",
	"**/composer.json",
	"**/*.{json,json5}",
	".github/workflows/**/*.{yaml,yml}",
	".github/**/*.{yaml,yml}",
	"**/*.{yaml,yml}",
	"**/*.{ftl,ftlh,ftlx}",
	"**/*.proto",
	"**/*.po",
	"**/*.pot",
	"**/*.{graphql,gql}",
	"**/*.prisma",
	"**/*.{tf,hcl,tfvars}",
	"**/*.bicep",
	"**/*.{css,scss,sass,less,html,svelte}",
}

// isStandaloneRefactorPath reports whether path should receive only its path
// rule (no common.md composition).
func isStandaloneRefactorPath(path string) bool {
	lowerPath := strings.ToLower(path)
	for _, pattern := range standaloneRefactorPatterns {
		for _, p := range expandBraces(pattern) {
			if matched, _ := doublestar.Match(strings.ToLower(p), lowerPath); matched {
				return true
			}
		}
	}
	return false
}

// composeRefactorRules is kept for tests; prefer composeRefactorLayers.
func composeRefactorRules(common, language string, standalone bool) string {
	return composeRefactorLayers(common, "", language, nil, standalone)
}

// ResolveRefactor returns the refactoring rule text for a given file path.
// Code-language paths compose: common → threshold profile → family → language delta.
// Config/data/markup paths use the path rule alone. Unmatched paths fall back to
// default_refactor_rule (plus default profile when available).
func (r *SystemRule) ResolveRefactor(path string) string {
	return r.ResolveRefactorWithOptions(path, ResolveRefactorOptions{})
}

// expandBraces turns "{a,b,c}" style patterns into individual strings.
// e.g. "*.go.{java,kotlin}" → ["*.go.java", "*.go.kotlin"].
// If no braces exist, returns the original pattern unchanged.
func expandBraces(s string) []string {
	openIdx := strings.IndexByte(s, '{')
	if openIdx < 0 {
		return []string{s}
	}

	closeIdx := strings.IndexByte(s[openIdx:], '}')
	if closeIdx < 0 {
		return []string{s}
	}
	closeIdx += openIdx

	prefix := s[:openIdx]
	suffix := s[closeIdx+1:]
	options := strings.Split(s[openIdx+1:closeIdx], ",")

	results := make([]string, 0, len(options))
	for _, opt := range options {
		results = append(results, prefix+opt+suffix)
	}
	return results
}

// ProjectRuleEntry is a single entry in .opencodereview/rule.json.
type ProjectRuleEntry struct {
	Path            string `json:"path"`
	Rule            string `json:"rule"`
	MergeSystemRule bool   `json:"merge_system_rule,omitempty"`
}

// ProjectRule holds rules loaded from <repoDir>/.opencodereview/rule.json.
// Rules are resolved by Resolve (for review). RefactorRules are resolved by
// ResolveRefactor. When RefactorRules is empty, ResolveRefactor falls back to
// Rules so existing rule.json files work without changes.
type ProjectRule struct {
	Rules         []ProjectRuleEntry `json:"rules"`
	RefactorRules []ProjectRuleEntry `json:"refactor_rules,omitempty"`
	Include       []string           `json:"include,omitempty"`
	Exclude       []string           `json:"exclude,omitempty"`

	// Phase 2 project knobs for system refactor composition.
	// DisabledRefactorRules lists catalog IDs (e.g. REF-DI-001) to strip from system rules.
	DisabledRefactorRules []string `json:"disabled_refactor_rules,omitempty"`
	// RefactorProfile forces a named threshold profile for all paths in this layer.
	RefactorProfile string `json:"refactor_profile,omitempty"`
}

// FileFilter holds the merged user-configured include/exclude glob patterns
// collected from all rule.json layers (custom, project, global).
type FileFilter struct {
	Include []string
	Exclude []string
}

// HasInclude reports whether any include patterns are configured.
func (f *FileFilter) HasInclude() bool {
	return len(f.Include) > 0
}

// IsUserExcluded reports whether the given path matches any user exclude pattern.
func (f *FileFilter) IsUserExcluded(path string) bool {
	lowerPath := strings.ToLower(path)
	for _, pattern := range f.Exclude {
		expanded := expandBraces(pattern)
		for _, p := range expanded {
			if matched, _ := doublestar.Match(p, lowerPath); matched {
				return true
			}
		}
	}
	return false
}

// IsUserIncluded reports whether the given path matches any user include pattern.
// Returns false when Include is empty (no user include restriction defined).
func (f *FileFilter) IsUserIncluded(path string) bool {
	if !f.HasInclude() {
		return false
	}
	lowerPath := strings.ToLower(path)
	for _, pattern := range f.Include {
		expanded := expandBraces(pattern)
		for _, p := range expanded {
			if matched, _ := doublestar.Match(p, lowerPath); matched {
				return true
			}
		}
	}
	return false
}

// ResolverOptions configures rule resolution layers.
type ResolverOptions struct {
	CustomRulePath string // --rule flag value
	RulesDir       string // --rules-dir value (enterprise rules directory)
}

// composedResolver implements Resolver with layered priority.
type composedResolver struct {
	custom            *ProjectRule // highest: --rule flag
	project           *ProjectRule // high: .opencodereview/rule.json
	enterpriseProject *ProjectRule // medium: <rules-dir>/projects/<project>/rule.json
	enterpriseGlobal  *ProjectRule // medium-low: <rules-dir>/global.json
	global            *ProjectRule // low: ~/.opencodereview/rule.json
	system            *SystemRule  // lowest: embedded default
}

// NewResolver builds a Resolver with the following priority:
//  1. Custom rule file specified via --rule flag (first match wins)
//  2. Project-local .opencodereview/rule.json (first match wins)
//  3. Global ~/.opencodereview/rule.json (first match wins)
//  4. Embedded system default rules
//
// It also returns a FileFilter with the merged include/exclude patterns from all layers.
func NewResolver(repoDir, customRulePath string) (Resolver, *FileFilter, error) {
	return NewResolverWithOptions(repoDir, ResolverOptions{CustomRulePath: customRulePath})
}

// NewResolverWithOptions builds a Resolver with the following priority:
//  1. Custom rule file specified via --rule flag (first match wins)
//  2. Project-local .opencodereview/rule.json (first match wins)
//  3. Enterprise project <rules-dir>/projects/<project>/rule.json (first match wins)
//  4. Enterprise global <rules-dir>/global.json (first match wins)
//  5. Global ~/.opencodereview/rule.json (first match wins)
//  6. Embedded system default rules
//
// It also returns a FileFilter with the merged include/exclude patterns from all layers.
func NewResolverWithOptions(repoDir string, opts ResolverOptions) (Resolver, *FileFilter, error) {
	sysRule, err := LoadDefault()
	if err != nil {
		return nil, nil, err
	}

	var customRule *ProjectRule
	if opts.CustomRulePath != "" {
		cr, err := loadRuleFile(opts.CustomRulePath)
		if err != nil {
			return nil, nil, err
		}
		customRule = cr
	}

	var projectRule *ProjectRule
	if repoDir != "" {
		pr, err := loadProjectRule(repoDir)
		if err != nil {
			return nil, nil, err
		}
		projectRule = pr
	}

	var entProjectRule, entGlobalRule *ProjectRule
	if opts.RulesDir != "" {
		entGlobalRule, err = loadEnterpriseGlobalRule(opts.RulesDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: failed to load enterprise global rule: %v\n", err)
		}
		if repoDir != "" {
			entProjectRule, err = loadEnterpriseProjectRule(opts.RulesDir, repoDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[ocr] WARNING: failed to load enterprise project rule: %v\n", err)
			}
		}
	}

	globalRule, err := loadGlobalRule()
	if err != nil {
		return nil, nil, err
	}

	filter := buildFileFilter(customRule, projectRule, entProjectRule, entGlobalRule, globalRule)

	return &composedResolver{
		custom:            customRule,
		project:           projectRule,
		enterpriseProject: entProjectRule,
		enterpriseGlobal:  entGlobalRule,
		global:            globalRule,
		system:            sysRule,
	}, filter, nil
}

// buildFileFilter picks the highest-priority layer that has any include/exclude
// configured. Priority order: custom (--rule) > project > enterprise-project > enterprise-global > global.
func buildFileFilter(layers ...*ProjectRule) *FileFilter {
	for _, pr := range layers {
		if pr == nil {
			continue
		}
		if len(pr.Include) == 0 && len(pr.Exclude) == 0 {
			continue
		}
		f := &FileFilter{}
		for _, p := range pr.Include {
			f.Include = append(f.Include, strings.ToLower(p))
		}
		for _, p := range pr.Exclude {
			f.Exclude = append(f.Exclude, strings.ToLower(p))
		}
		return f
	}
	return nil
}

func loadGlobalRule() (*ProjectRule, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}
	path := filepath.Join(home, ".opencodereview", "rule.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read global rule %s: %w", path, err)
	}
	var pr ProjectRule
	if err := json.Unmarshal(data, &pr); err != nil {
		return nil, fmt.Errorf("unmarshal global rule: %w", err)
	}
	resolveRuleEntries(pr.Rules, filepath.Dir(path)); resolveRuleEntries(pr.RefactorRules, filepath.Dir(path))
	return &pr, nil
}

// loadEnterpriseGlobalRule loads <rulesDir>/global.json.
func loadEnterpriseGlobalRule(rulesDir string) (*ProjectRule, error) {
	path := filepath.Join(rulesDir, "global.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read enterprise global rule %s: %w", path, err)
	}
	var pr ProjectRule
	if err := json.Unmarshal(data, &pr); err != nil {
		return nil, fmt.Errorf("unmarshal enterprise global rule: %w", err)
	}
	resolveRuleEntries(pr.Rules, rulesDir); resolveRuleEntries(pr.RefactorRules, rulesDir)
	return &pr, nil
}

// loadEnterpriseProjectRule loads <rulesDir>/projects/<project>/rule.json
// where the project segment is derived from repoDir via encodeProjectKey.
func loadEnterpriseProjectRule(rulesDir, repoDir string) (*ProjectRule, error) {
	projectKey := encodeProjectKey(repoDir)
	if !isSafeRulePathSegment(projectKey) {
		return nil, fmt.Errorf("unsafe project key %q derived from repo dir %q", projectKey, repoDir)
	}
	path := filepath.Join(rulesDir, "projects", projectKey, "rule.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read enterprise project rule %s: %w", path, err)
	}
	var pr ProjectRule
	if err := json.Unmarshal(data, &pr); err != nil {
		return nil, fmt.Errorf("unmarshal enterprise project rule: %w", err)
	}
	resolveRuleEntries(pr.Rules, filepath.Dir(path)); resolveRuleEntries(pr.RefactorRules, filepath.Dir(path))
	return &pr, nil
}

// enterpriseProjectRulePaths returns the enterprise rule paths that would be
// consulted for a given repoDir and rulesDir.
func enterpriseProjectRulePaths(dir, repoDir string) []string {
	key := encodeProjectKey(repoDir)
	if !isSafeRulePathSegment(key) {
		return nil
	}
	return []string{filepath.Join(dir, "projects", key, "rule.json")}
}

// enterpriseProjectRulePath returns the resolved path and true when a valid
// enterprise project rule file exists for the given project name.
func enterpriseProjectRulePath(dir, project string) (string, bool) {
	if !isSafeRulePathSegment(project) {
		return "", false
	}
	return filepath.Join(dir, "projects", project, "rule.json"), true
}

// encodeProjectKey converts a repo directory path into a flat filesystem-safe
// segment. Slashes and backslashes become hyphens; volume colons become
// underscores. Returns "empty" when the result would be blank.
func encodeProjectKey(key string) string {
	if key == "" {
		return "empty"
	}
	vol := filepath.VolumeName(key)
	key = key[len(vol):]
	key = strings.TrimLeft(key, "/\\")
	key = strings.ReplaceAll(key, "/", "-")
	key = strings.ReplaceAll(key, "\\", "-")
	vol = strings.ReplaceAll(vol, ":", "_")
	result := vol + key
	if result == "" {
		return "empty"
	}
	return result
}

// isSafeRulePathSegment reports whether segment is safe to use as a filesystem
// path component (no traversal, no separators, non-empty, not "." or "..").
func isSafeRulePathSegment(segment string) bool {
	if segment == "" || segment == "." || segment == ".." {
		return false
	}
	if strings.ContainsRune(segment, 0) {
		return false
	}
	if strings.ContainsAny(segment, "/\\:") {
		return false
	}
	return !filepath.IsAbs(segment) && filepath.Base(segment) == segment
}

func loadRuleFile(path string) (*ProjectRule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read rule file %s: %w", path, err)
	}
	var pr ProjectRule
	if err := json.Unmarshal(data, &pr); err != nil {
		return nil, fmt.Errorf("unmarshal rule file %s: %w", path, err)
	}
	resolveRuleEntries(pr.Rules, filepath.Dir(path)); resolveRuleEntries(pr.RefactorRules, filepath.Dir(path))
	return &pr, nil
}

// loadProjectRule reads <repoDir>/.opencodereview/rule.json. Since #287 anchored
// RepoDir at the git top-level, `ocr review` from a monorepo subdirectory loads
// the repo-root rule file — which is consistent, since rule entries match against
// root-relative diff paths. A subproject-local rule.json under the subdirectory is
// intentionally not consulted; put shared rules at the repo root, or pass --rule.
func loadProjectRule(repoDir string) (*ProjectRule, error) {
	path := filepath.Join(repoDir, ".opencodereview", "rule.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read project rule %s: %w", path, err)
	}
	var pr ProjectRule
	if err := json.Unmarshal(data, &pr); err != nil {
		return nil, fmt.Errorf("unmarshal project rule: %w", err)
	}
	resolveRuleEntries(pr.Rules, filepath.Dir(path)); resolveRuleEntries(pr.RefactorRules, filepath.Dir(path))
	return &pr, nil
}

// Resolve checks each layer in priority order; first match wins. User rules
// replace the system rule by default; rules with merge_system_rule keep the
// matched system rule alongside the user rule.
func (c *composedResolver) Resolve(path string) string {
	for _, layer := range []*ProjectRule{c.custom, c.project, c.enterpriseProject, c.enterpriseGlobal, c.global} {
		if entry := matchProjectRuleEntry(layer, path); entry != nil {
			if entry.MergeSystemRule {
				return c.mergeWithSystemRule(path, entry.Rule)
			}
			return entry.Rule
		}
	}
	return c.system.Resolve(path)
}

func (c *composedResolver) ResolveRefactor(path string) string {
	opts := c.refactorResolveOptions()
	for _, layer := range []*ProjectRule{c.custom, c.project, c.enterpriseProject, c.enterpriseGlobal, c.global} {
		if entry := matchRefactorRuleEntry(layer, path); entry != nil {
			if entry.MergeSystemRule {
				return c.mergeWithSystemRefactorRule(path, entry.Rule)
			}
			// User-supplied rule text; still apply disabled-ID filter if present.
			return filterDisabledRefactorRules(entry.Rule, opts.DisabledRuleIDs)
		}
	}
	if c.system == nil {
		return ""
	}
	return c.system.ResolveRefactorWithOptions(path, opts)
}

// refactorResolveOptions collects the highest-priority non-empty project knobs.
func (c *composedResolver) refactorResolveOptions() ResolveRefactorOptions {
	opts := ResolveRefactorOptions{}
	for _, layer := range []*ProjectRule{c.custom, c.project, c.enterpriseProject, c.enterpriseGlobal, c.global} {
		if layer == nil {
			continue
		}
		if opts.ForcedProfile == "" && layer.RefactorProfile != "" {
			opts.ForcedProfile = layer.RefactorProfile
		}
		if len(opts.DisabledRuleIDs) == 0 && len(layer.DisabledRefactorRules) > 0 {
			opts.DisabledRuleIDs = append([]string(nil), layer.DisabledRefactorRules...)
		}
	}
	return opts
}

// matchRefactorRuleEntry matches against a layer's RefactorRules first.
// Falls back to Rules when RefactorRules is empty so existing rule.json files
// work without changes.
func matchRefactorRuleEntry(pr *ProjectRule, path string) *ProjectRuleEntry {
	if pr == nil {
		return nil
	}
	// If RefactorRules are explicitly defined, use them exclusively.
	if len(pr.RefactorRules) > 0 {
		return matchProjectRuleEntry(&ProjectRule{Rules: pr.RefactorRules}, path)
	}
	// Otherwise fall back to Rules (backwards-compatible).
	return matchProjectRuleEntry(pr, path)
}

func (c *composedResolver) mergeWithSystemRefactorRule(path, rule string) string {
	systemRule := c.system.ResolveRefactorWithOptions(path, c.refactorResolveOptions())
	if systemRule == "" {
		return rule
	}
	if rule == "" {
		return systemRule
	}
	return "## System-Specific Refactoring Rules (Mandatory)\n\n" +
		systemRule +
		"\n\n---\n\n" +
		"## User-Specific Refactoring Rules (Mandatory)\n\n" +
		rule
}

// RefactorPayloadStats delegates to the system rule set for payload telemetry.
func (c *composedResolver) RefactorPayloadStats(path string) RefactorRulePayloadStats {
	if c.system == nil {
		return RefactorRulePayloadStats{Path: path}
	}
	return c.system.RefactorPayloadStats(path)
}

// CanonicalConfig returns a deterministic, order-stable field list describing the
// resolver's effective rule-text configuration across every layer (custom >
// project > global > system, each in declaration order), for hashing into the run
// manifest's rule_config_sha256. It covers only rule-text resolution; the
// include/exclude file filter is carried by FileFilter and hashed separately. Each
// field is tagged with its layer and role so two structurally different configs
// cannot collide once length-prefixed. Order is never sorted — first match wins.
func (c *composedResolver) CanonicalConfig() []string {
	var fields []string
	appendLayer := func(name string, pr *ProjectRule) {
		if pr == nil {
			return
		}
		for _, e := range pr.Rules {
			merge := "0"
			if e.MergeSystemRule {
				merge = "1"
			}
			fields = append(fields, "layer", name, "path", e.Path, "rule", e.Rule, "merge", merge)
		}
	}
	appendLayer("custom", c.custom)
	appendLayer("project", c.project)
	appendLayer("global", c.global)
	if c.system != nil {
		fields = append(fields, c.system.CanonicalConfig()...)
	}
	return fields
}

func (c *composedResolver) mergeWithSystemRule(path, rule string) string {
	systemRule := c.system.Resolve(path)

	if systemRule == "" {
		return rule
	}
	if rule == "" {
		return systemRule
	}

	return "## System-Specific Rules (Mandatory)\n\n" +
		systemRule +
		"\n\n---\n\n" +
		"## User-Specific Rules (Mandatory)\n\n" +
		rule
}

// ResolveDetail returns the matched rule along with its source layer and pattern.
// When a user rule sets merge_system_rule, Rule contains the merged system+user
// rule text while Source and Pattern still describe the user rule that won the
// priority chain.
func (c *composedResolver) ResolveDetail(path string) RuleDetail {
	if detail := c.matchProjectRuleDetail(c.custom, path, "custom"); detail != nil {
		return *detail
	}
	if detail := c.matchProjectRuleDetail(c.project, path, "project"); detail != nil {
		return *detail
	}
	if detail := c.matchProjectRuleDetail(c.enterpriseProject, path, "enterprise-project"); detail != nil {
		return *detail
	}
	if detail := c.matchProjectRuleDetail(c.enterpriseGlobal, path, "enterprise-global"); detail != nil {
		return *detail
	}
	if detail := c.matchProjectRuleDetail(c.global, path, "global"); detail != nil {
		return *detail
	}
	return c.system.resolveDetail(path)
}

func (c *composedResolver) matchProjectRuleDetail(pr *ProjectRule, path string, source string) *RuleDetail {
	entry := matchProjectRuleEntry(pr, path)
	if entry == nil {
		return nil
	}
	rule := entry.Rule
	if entry.MergeSystemRule {
		rule = c.mergeWithSystemRule(path, rule)
	}
	return &RuleDetail{Rule: rule, Source: source, Pattern: entry.Path}
}

func matchProjectRuleEntry(pr *ProjectRule, path string) *ProjectRuleEntry {
	if pr == nil {
		return nil
	}
	lowerPath := strings.ToLower(path)
	for i := range pr.Rules {
		entry := &pr.Rules[i]
		if entry.Rule == "" && !entry.MergeSystemRule {
			continue
		}
		expanded := expandBraces(entry.Path)
		for _, p := range expanded {
			if matched, _ := doublestar.Match(strings.ToLower(p), lowerPath); matched {
				return entry
			}
		}
	}
	return nil
}

// allowedRuleExts is the set of file extensions permitted for rule file references.
var allowedRuleExts = map[string]bool{".md": true, ".txt": true, ".markdown": true}

// looksLikeFilePath returns true when s is likely a file path (not inline content).
// Heuristic: multi-line text is always inline; single-line text without spaces
// ending in .md/.txt/.markdown is treated as a file path. Values containing spaces
// (e.g. "Follow rules from team.md") are treated as inline to avoid false positives.
func looksLikeFilePath(s string) bool {
	if strings.Contains(s, "\n") {
		return false
	}
	if strings.Contains(s, " ") {
		return false
	}
	return allowedRuleExts[strings.ToLower(filepath.Ext(s))]
}

// resolveRuleEntries scans each entry's Rule field. When the value looks like a file
// path, it reads the file content and replaces the Rule. Absolute paths are used
// directly; relative paths are resolved against repoDir only. Multi-line and short
// inline rules are left unchanged. If the file cannot be read, the Rule is cleared
// (set to empty) and a warning is emitted.
func resolveRuleEntries(entries []ProjectRuleEntry, repoDir string) {
	for i := range entries {
		e := &entries[i]
		if strings.TrimSpace(e.Rule) == "" || !looksLikeFilePath(e.Rule) {
			continue
		}
		if content := tryReadRuleFile(e.Rule, repoDir); content != nil {
			e.Rule = *content
		} else {
			e.Rule = ""
		}
	}
}

// tryReadRuleFile attempts to read a rule file. Absolute paths are used directly.
// Relative paths are resolved against repoDir and validated to stay within repoDir.
// Returns nil when the file cannot be read safely or does not exist.
func tryReadRuleFile(rule string, repoDir string) *string {
	if repoDir == "" {
		if !filepath.IsAbs(rule) {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: cannot resolve relative rule path %q without a repo dir\n", rule)
			return nil
		}
	}
	if filepath.IsAbs(rule) {
		content, err := readRuleFileSafe(rule)
		if err == nil {
			return &content
		}
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: rule file not found: %s\n", rule)
		} else {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: cannot read rule file %s: %v\n", rule, err)
		}
		return nil
	}

	// Relative path: resolve against repoDir, validate no traversal.
	resolved := filepath.Clean(filepath.Join(repoDir, rule))
	cleanRepo := filepath.Clean(repoDir)
	if !strings.HasPrefix(resolved, cleanRepo+string(os.PathSeparator)) {
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: rule file path escapes repo dir: %s\n", rule)
		return nil
	}

	content, err := readRuleFileSafe(resolved)
	if err == nil {
		return &content
	}
	if os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: rule file not found: %s\n", rule)
	} else {
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: cannot read rule file %s: %v\n", resolved, err)
	}
	return nil
}

// readRuleFileSafe reads and validates a rule file. It enforces extension whitelist
// (.md / .txt / .markdown), a 512 KB size cap, and resolves symlinks before checking
// the path. Symlinks are resolved first, then size is checked via Stat before reading.
// Returns the trimmed content on success.
func readRuleFileSafe(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}

	if !allowedRuleExts[strings.ToLower(filepath.Ext(resolved))] {
		return "", fmt.Errorf("unsupported extension %q, only .md/.txt/.markdown allowed", filepath.Ext(resolved))
	}

	const maxSize = 512 * 1024
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if info.Size() > maxSize {
		return "", fmt.Errorf("file too large (%d bytes, max %d)", info.Size(), maxSize)
	}

	content, err := os.ReadFile(resolved)
	if err != nil {
		return "", err
	}

	return strings.TrimRight(string(content), "\n"), nil
}
