package crossfile

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// StripMarkdownFences removes ```json wrappers if present.
func StripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		// drop language tag line
		s = s[i+1:]
	}
	if i := strings.LastIndex(s, "```"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// ParseSmellReports accepts a JSON array, {"smells":[...]}, or prose containing
// a JSON array/object. Empty output ("", "[]", "null") is treated as no smells.
// Non-empty responses that contain no parseable JSON return an error so callers
// do not silently drop LLM output.
func ParseSmellReports(raw string) ([]SmellReport, error) {
	raw = StripMarkdownFences(raw)
	if raw == "" || raw == "null" || raw == "[]" {
		return nil, nil
	}
	jsonStr := extractJSON(raw)
	if jsonStr == "" {
		return nil, fmt.Errorf("parse smell reports: no JSON found in LLM response")
	}
	if jsonStr == "[]" || jsonStr == "null" {
		return nil, nil
	}
	var arr []SmellReport
	if err := json.Unmarshal([]byte(jsonStr), &arr); err == nil {
		return arr, nil
	}
	var wrap struct {
		Smells []SmellReport `json:"smells"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &wrap); err != nil {
		return nil, fmt.Errorf("parse smell reports: %w", err)
	}
	return wrap.Smells, nil
}

// ParseRefactorPlans accepts a single plan, array, {"plans":[...]}, or prose
// containing JSON. Empty/null output is treated as no plans. Non-empty responses
// without parseable JSON return an error.
func ParseRefactorPlans(raw string) ([]RefactorPlan, error) {
	raw = StripMarkdownFences(raw)
	if raw == "" || raw == "null" || raw == "[]" || raw == "{}" {
		return nil, nil
	}
	jsonStr := extractJSON(raw)
	if jsonStr == "" {
		return nil, fmt.Errorf("parse refactor plans: no JSON found in LLM response")
	}
	if jsonStr == "[]" || jsonStr == "{}" || jsonStr == "null" {
		return nil, nil
	}
	var one RefactorPlan
	if err := json.Unmarshal([]byte(jsonStr), &one); err == nil && (one.PlanID != "" || one.Summary != "" || len(one.Steps) > 0) {
		return []RefactorPlan{one}, nil
	}
	var arr []RefactorPlan
	if err := json.Unmarshal([]byte(jsonStr), &arr); err == nil {
		return arr, nil
	}
	var wrap struct {
		Plans []RefactorPlan `json:"plans"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &wrap); err != nil {
		return nil, fmt.Errorf("parse refactor plans: %w", err)
	}
	return wrap.Plans, nil
}

// extractJSON finds the first JSON array or object in a text blob that may be
// surrounded by prose (LLM introduction, closing remarks, etc.). Returns ""
// if no JSON candidate is found. Bracket matching ignores braces inside strings
// and requires matching pair types.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Clean JSON: matching outer brackets.
	if (s[0] == '[' && s[len(s)-1] == ']') || (s[0] == '{' && s[len(s)-1] == '}') {
		return s
	}
	for i := 0; i < len(s); i++ {
		if s[i] != '[' && s[i] != '{' {
			continue
		}
		if end := matchBalancedJSON(s, i); end >= i {
			return s[i : end+1]
		}
	}
	return ""
}

// matchBalancedJSON returns the index of the closing bracket that balances the
// opener at start, or -1 if unmatched. Tracks a stack of openers and skips
// content inside JSON strings (including escaped quotes).
func matchBalancedJSON(s string, start int) int {
	stack := make([]byte, 0, 8)
	inString := false
	escape := false
	for j := start; j < len(s); j++ {
		c := s[j]
		if inString {
			if escape {
				escape = false
				continue
			}
			if c == '\\' {
				escape = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '[', '{':
			stack = append(stack, c)
		case ']', '}':
			if len(stack) == 0 {
				return -1
			}
			top := stack[len(stack)-1]
			wantOpen := byte('[')
			if c == '}' {
				wantOpen = '{'
			}
			if top != wantOpen {
				return -1
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return j
			}
		}
	}
	return -1
}

// SmellsToComments converts detector output into multi-anchor comments.
func SmellsToComments(smells []SmellReport, clusterID string) []CommentOut {
	var out []CommentOut
	for _, s := range smells {
		if len(s.AffectedFiles) == 0 && len(s.Evidence) == 0 {
			continue
		}
		primary := Location{}
		if len(s.Evidence) > 0 {
			primary = s.Evidence[0]
		} else {
			primary.Path = s.AffectedFiles[0]
			primary.StartLine = 1
			primary.EndLine = 1
		}
		msg := s.Message
		if msg == "" {
			msg = fmt.Sprintf("[%s] cross-file smell involving %s", s.SmellType, strings.Join(s.AffectedFiles, ", "))
			if s.ExtractedCandidate != "" {
				msg += "; candidate: " + s.ExtractedCandidate
			}
			if s.TargetAbstraction != "" {
				msg += " → " + s.TargetAbstraction
			}
		}
		if s.RuleID != "" {
			msg += "\nRule: " + s.RuleID
		}
		msg += "\nCluster: " + clusterID
		if s.Confidence != "" {
			msg += "\nConfidence: " + s.Confidence
		}

		var related []Location
		for i, e := range s.Evidence {
			if i == 0 {
				continue
			}
			related = append(related, e)
		}
		for _, f := range s.AffectedFiles {
			if f == primary.Path {
				continue
			}
			// add path-only related if not already in evidence
			found := false
			for _, r := range related {
				if r.Path == f {
					found = true
					break
				}
			}
			if !found {
				related = append(related, Location{Path: f})
			}
		}

		cat := "duplication"
		switch strings.ToUpper(s.SmellType) {
		case "TIGHT_COUPLING", "FEATURE_ENVY":
			cat = "coupling"
		case "DATA_CLUMP", "EXTRACT_CANDIDATE":
			cat = "design"
		case "INCONSISTENT_API":
			cat = "naming"
		case "DEAD_EXPORT":
			cat = "dead_code"
		}

		out = append(out, CommentOut{
			Path:              primary.Path,
			StartLine:         max1(primary.StartLine),
			EndLine:           max1(primary.EndLine),
			Content:           msg,
			Category:          cat,
			Severity:          mapSeverity(s.Severity),
			RelatedLocations:  related,
			PlanID:            "",
			SmellType:         s.SmellType,
			RefactorKind:      "cross_file",
			ProposedSymbol:    s.ExtractedCandidate,
		})
	}
	return out
}

// PlansToComments converts architect plans into actionable multi-file comments.
func PlansToComments(plans []RefactorPlan, clusterID string) []CommentOut {
	var out []CommentOut
	for _, p := range plans {
		if len(p.Steps) == 0 && p.Summary == "" {
			continue
		}
		primaryPath := ""
		if len(p.Steps) > 0 {
			primaryPath = p.Steps[0].Path
		}
		var b strings.Builder
		fmt.Fprintf(&b, "**Cross-file refactor plan** `%s`\n\n%s\n", p.PlanID, p.Summary)
		if p.BreakingChange {
			b.WriteString("\n⚠ Breaking change: true\n")
		}
		if p.RuleID != "" {
			fmt.Fprintf(&b, "\nRule: %s\n", p.RuleID)
		}
		b.WriteString("\n**Steps (topological order):**\n")
		steps := append([]PlanStep(nil), p.Steps...)
		sortSteps(steps)
		for _, st := range steps {
			fmt.Fprintf(&b, "%d. `%s` %s", st.Order, st.Action, st.Path)
			if st.Symbol != "" {
				fmt.Fprintf(&b, " (%s)", st.Symbol)
			}
			if st.Notes != "" {
				fmt.Fprintf(&b, " — %s", st.Notes)
			}
			b.WriteString("\n")
		}
		if len(p.Risks) > 0 {
			b.WriteString("\nRisks: " + strings.Join(p.Risks, "; ") + "\n")
		}
		fmt.Fprintf(&b, "\nCluster: %s\n", clusterID)

		var related []Location
		seen := map[string]struct{}{primaryPath: {}}
		for _, st := range steps {
			if st.Path == "" || st.Path == primaryPath {
				continue
			}
			if _, ok := seen[st.Path]; ok {
				continue
			}
			seen[st.Path] = struct{}{}
			related = append(related, Location{Path: st.Path, Note: st.Action})
		}

		suggestion := ""
		if len(steps) > 0 {
			suggestion = steps[0].SuggestionCode
		}

		out = append(out, CommentOut{
			Path:             primaryPath,
			StartLine:        1,
			EndLine:          1,
			Content:          b.String(),
			SuggestionCode:   suggestion,
			Category:         "design",
			Severity:         "major",
			RelatedLocations: related,
			PlanID:           p.PlanID,
			SmellType:        p.SmellType,
			RefactorKind:     "extract_shared",
		})
	}
	return out
}

// CommentOut is a DTO before conversion to model.LlmComment (avoids import cycles in tests).
type CommentOut struct {
	Path             string
	StartLine        int
	EndLine          int
	Content          string
	SuggestionCode   string
	ExistingCode     string
	Category         string
	Severity         string
	RelatedLocations []Location
	PlanID           string
	SmellType        string
	RefactorKind     string
	ProposedSymbol   string
}

func mapSeverity(s string) string {
	switch strings.ToLower(s) {
	case "blocker", "critical", "high", "major", "medium", "minor", "low", "info":
		return strings.ToLower(s)
	default:
		return "major"
	}
}

func max1(n int) int {
	if n <= 0 {
		return 1
	}
	return n
}

func sortSteps(steps []PlanStep) {
	sort.Slice(steps, func(i, j int) bool {
		return steps[i].Order < steps[j].Order
	})
}
