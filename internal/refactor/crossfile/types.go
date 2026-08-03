// Package crossfile implements multi-file refactor context, clustering,
// similarity, plan parsing, verification, and apply helpers (ADR F0–F3).
package crossfile

import "strings"

// Mode selects which refactor pipeline stages run.
type Mode string

const (
	ModeLocal Mode = "local" // per-file only (default)
	ModeCross Mode = "cross" // cross-file detect+architect only
	ModeFull  Mode = "full"  // local then cross
)

// ParseMode normalizes user input; empty → local.
func ParseMode(s string) Mode {
	switch Mode(toLowerTrim(s)) {
	case ModeCross:
		return ModeCross
	case ModeFull:
		return ModeFull
	default:
		return ModeLocal
	}
}

// Location is a path + line range evidence anchor.
type Location struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
	Note      string `json:"note,omitempty"`
}

// SmellReport is the strict JSON output of CrossFileSmellDetector (X1).
type SmellReport struct {
	SmellType          string     `json:"smell_type"`
	Severity           string     `json:"severity,omitempty"`
	Confidence         string     `json:"confidence,omitempty"`
	AffectedFiles      []string   `json:"affected_files"`
	Evidence           []Location `json:"evidence,omitempty"`
	ExtractedCandidate string     `json:"extracted_candidate,omitempty"`
	TargetAbstraction  string     `json:"target_abstraction,omitempty"`
	RuleID             string     `json:"rule_id,omitempty"`
	Message            string     `json:"message,omitempty"`
}

// PlanStep is one ordered edit in a RefactorPlan (X2).
type PlanStep struct {
	Order        int      `json:"order"`
	Action       string   `json:"action"` // create_file | rewrite_callsite | modify | delete
	Path         string   `json:"path"`
	Symbol       string   `json:"symbol,omitempty"`
	RelatedPaths []string `json:"related_paths,omitempty"`
	Notes        string   `json:"notes,omitempty"`
	// Optional preview from architect (not always present).
	SuggestionCode string `json:"suggestion_code,omitempty"`
}

// RefactorPlan is the output of RefactoringArchitect (X2).
type RefactorPlan struct {
	PlanID         string     `json:"plan_id"`
	Summary        string     `json:"summary"`
	BreakingChange bool       `json:"breaking_change"`
	Steps          []PlanStep `json:"steps"`
	Risks          []string   `json:"risks,omitempty"`
	TestFocus      []string   `json:"test_focus,omitempty"`
	SmellType      string     `json:"smell_type,omitempty"`
	RuleID         string     `json:"rule_id,omitempty"`
}

// FileInput is a path + content pair for context building.
type FileInput struct {
	Path    string
	Content string
}

// Cluster is a budgeted group of files for cross-file analysis.
type Cluster struct {
	ID      string
	Files   []FileInput
	Graph   *Topology
	Skeletons map[string]string // path → skeleton text
	Slices  []CodeSlice
}

// CodeSlice is a full-body window sent to the agent for evidence.
type CodeSlice struct {
	Path      string
	StartLine int
	EndLine   int
	Content   string
	Score     float64 // similarity score when from F2
}

// ClusterOptions controls packing.
type ClusterOptions struct {
	MaxFilesPerCluster int
	MaxSkeletonBytes   int
	MaxSliceBytes      int
	MinSimilarity      float64 // 0 = disabled
	UseSimilarity      bool
}

// DefaultClusterOptions returns ADR-recommended defaults.
func DefaultClusterOptions() ClusterOptions {
	return ClusterOptions{
		MaxFilesPerCluster: 8,
		MaxSkeletonBytes:   60_000,
		MaxSliceBytes:      80_000,
		MinSimilarity:      0.78,
		UseSimilarity:      true,
	}
}

// VerifyResult is Layer 3 output for one apply attempt.
type VerifyResult struct {
	OK       bool
	Messages []string
}

func toLowerTrim(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
