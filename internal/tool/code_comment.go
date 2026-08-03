package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alibaba/open-code-review/internal/model"
)

const (
	codeCommentCategoryBug             = "bug"
	codeCommentCategorySecurity        = "security"
	codeCommentCategoryPerformance     = "performance"
	codeCommentCategoryMaintainability = "maintainability"
	codeCommentCategoryTest            = "test"
	codeCommentCategoryStyle           = "style"
	codeCommentCategoryDocumentation   = "documentation"
	codeCommentCategoryOther           = "other"
	codeCommentCategoryComplexity      = "complexity"
	codeCommentCategoryNaming          = "naming"
	codeCommentCategoryDuplication     = "duplication"
	codeCommentCategoryDesign          = "design"
	codeCommentCategoryCoupling        = "coupling"
	codeCommentCategoryData            = "data"
	codeCommentCategoryControlFlow     = "control_flow"
	codeCommentCategoryErrorHandling   = "error_handling"
	codeCommentCategoryDeadCode        = "dead_code"
	codeCommentCategoryTestability     = "testability"
	codeCommentCategoryCrossDuplication = "cross_duplication"
	codeCommentCategoryCrossFile       = "cross_file"

	codeCommentSeverityCritical = "critical"
	codeCommentSeverityHigh     = "high"
	codeCommentSeverityMedium   = "medium"
	codeCommentSeverityLow      = "low"
	codeCommentSeverityBlocker  = "blocker"
	codeCommentSeverityMajor    = "major"
	codeCommentSeverityMinor    = "minor"
	codeCommentSeverityInfo     = "info"
)

var validCodeCommentCategories = map[string]struct{}{
	codeCommentCategoryBug:             {},
	codeCommentCategorySecurity:        {},
	codeCommentCategoryPerformance:     {},
	codeCommentCategoryMaintainability: {},
	codeCommentCategoryTest:            {},
	codeCommentCategoryStyle:           {},
	codeCommentCategoryDocumentation:   {},
	codeCommentCategoryOther:           {},
	codeCommentCategoryComplexity:      {},
	codeCommentCategoryNaming:          {},
	codeCommentCategoryDuplication:     {},
	codeCommentCategoryDesign:          {},
	codeCommentCategoryCoupling:        {},
	codeCommentCategoryData:            {},
	codeCommentCategoryControlFlow:     {},
	codeCommentCategoryErrorHandling:   {},
	codeCommentCategoryDeadCode:         {},
	codeCommentCategoryTestability:      {},
	codeCommentCategoryCrossDuplication: {},
	codeCommentCategoryCrossFile:        {},
}

var validCodeCommentSeverities = map[string]struct{}{
	codeCommentSeverityCritical: {},
	codeCommentSeverityHigh:     {},
	codeCommentSeverityMedium:   {},
	codeCommentSeverityLow:      {},
	codeCommentSeverityBlocker:  {},
	codeCommentSeverityMajor:    {},
	codeCommentSeverityMinor:    {},
	codeCommentSeverityInfo:     {},
}

// CodeCommentProvider submits review comments to the per-Agent CommentCollector.
type CodeCommentProvider struct {
	Collector *CommentCollector
}

func (p *CodeCommentProvider) Tool() Tool { return CodeComment }

func (p *CodeCommentProvider) Execute(_ context.Context, args map[string]any) (string, error) {
	if p.Collector == nil {
		return "Error: comment collector is not configured", nil
	}

	comments, errMsg := ParseComments(args)
	if errMsg != "" {
		return errMsg, nil
	}

	for i := range comments {
		p.Collector.Add(comments[i])
	}
	return CommentSucceed, nil
}

// ParseComments extracts LlmComment entries from tool call arguments without writing
// to the Collector. Returns parsed comments and an error message (empty on success).
func ParseComments(args map[string]any) ([]model.LlmComment, string) {
	var rawComments []any
	if arr, ok := args["comments"].([]any); ok && len(arr) > 0 {
		rawComments = arr
	} else if s, ok := args["comments"].(string); ok && s != "" {
		if err := json.Unmarshal([]byte(s), &rawComments); err != nil {
			return nil, fmt.Sprintf("Error: failed to parse 'comments' JSON string: %v", err)
		}
	}
	if len(rawComments) == 0 {
		raw, _ := json.Marshal(args)
		return nil, fmt.Sprintf("Error: 'comments' array is required. Got args: %s", string(raw))
	}

	var comments []model.LlmComment
	for _, raw := range rawComments {
		obj, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		cm := model.LlmComment{}

		if content, ok := obj["content"].(string); ok {
			cm.Content = content
		}
		if suggestion, ok := obj["suggestion_code"].(string); ok {
			cm.SuggestionCode = suggestion
		}
		if existing, ok := obj["existing_code"].(string); ok {
			cm.ExistingCode = existing
		}
		if thinking, ok := obj["thinking"].(string); ok {
			cm.Thinking = thinking
		}
		if category, ok := obj["category"].(string); ok {
			cm.Category = normalizeCodeCommentCategory(category)
		}
		if severity, ok := obj["severity"].(string); ok {
			cm.Severity = normalizeCodeCommentSeverity(severity)
		}
		if path, ok := args["path"].(string); ok {
			cm.Path = path
		}
		if path, ok := obj["path"].(string); ok && path != "" {
			cm.Path = path
		}
		if v, ok := obj["start_line"].(float64); ok {
			cm.StartLine = int(v)
		}
		if v, ok := obj["end_line"].(float64); ok {
			cm.EndLine = int(v)
		}
		if planID, ok := obj["plan_id"].(string); ok {
			cm.PlanID = planID
		}
		if smell, ok := obj["smell_type"].(string); ok {
			cm.SmellType = smell
		}
		if kind, ok := obj["refactor_kind"].(string); ok {
			cm.RefactorKind = kind
		}
		if sym, ok := obj["proposed_symbol"].(string); ok {
			cm.ProposedSymbol = sym
		}
		if rel, ok := obj["related_locations"].([]any); ok {
			for _, rawLoc := range rel {
				locObj, ok := rawLoc.(map[string]any)
				if !ok {
					continue
				}
				loc := model.RelatedLocation{}
				if p, ok := locObj["path"].(string); ok {
					loc.Path = p
				}
				if v, ok := locObj["start_line"].(float64); ok {
					loc.StartLine = int(v)
				}
				if v, ok := locObj["end_line"].(float64); ok {
					loc.EndLine = int(v)
				}
				if n, ok := locObj["note"].(string); ok {
					loc.Note = n
				}
				if loc.Path != "" {
					cm.RelatedLocations = append(cm.RelatedLocations, loc)
				}
			}
		}

		if cm.Path == "" || cm.Content == "" {
			continue
		}

		comments = append(comments, cm)
	}
	return comments, ""
}

func normalizeCodeCommentCategory(category string) string {
	normalized := strings.ToLower(category)
	if _, ok := validCodeCommentCategories[normalized]; ok {
		return normalized
	}
	return codeCommentCategoryOther
}

func normalizeCodeCommentSeverity(severity string) string {
	normalized := strings.ToLower(severity)
	if _, ok := validCodeCommentSeverities[normalized]; ok {
		return normalized
	}
	return codeCommentSeverityLow
}
