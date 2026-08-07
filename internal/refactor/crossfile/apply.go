package crossfile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alibaba/open-code-review/internal/model"
)

type backupEntry struct {
	data []byte
	mode os.FileMode
}

// ApplyResult summarizes a multi-file apply attempt.
type ApplyResult struct {
	Written    []string
	Skipped    []string
	Verify     VerifyResult
	RolledBack bool
	Messages   []string
}

// ApplyPlanSteps writes suggestion_code from plan steps under repoDir.
// On verify failure, restores previous content for written files.
// runTests enables go test in verify for Go packages.
func ApplyPlanSteps(repoDir string, plans []RefactorPlan, runTests bool) ApplyResult {
	res := ApplyResult{}
	backup := map[string]backupEntry{} // abs path → previous content and mode (nil content if new)

	rollback := func() {
		for abs, e := range backup {
			if e.data == nil {
				_ = os.Remove(abs)
			} else {
				_ = os.WriteFile(abs, e.data, e.mode)
			}
		}
		res.RolledBack = true
	}

	for _, plan := range plans {
		steps := append([]PlanStep(nil), plan.Steps...)
		sortSteps(steps)
		for _, st := range steps {
			if st.Path == "" {
				res.Skipped = append(res.Skipped, "(empty path)")
				continue
			}
			if st.SuggestionCode == "" && st.Action != "delete" {
				res.Skipped = append(res.Skipped, st.Path+": no suggestion_code")
				continue
			}
			abs := filepath.Join(repoDir, filepath.FromSlash(st.Path))
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				res.Messages = append(res.Messages, err.Error())
				rollback()
				res.Verify = VerifyResult{OK: false, Messages: res.Messages}
				return res
			}
			if _, exists := backup[abs]; !exists {
				prev, err := os.ReadFile(abs)
				if err != nil {
					if !os.IsNotExist(err) {
						res.Messages = append(res.Messages, err.Error())
						rollback()
						res.Verify = VerifyResult{OK: false, Messages: res.Messages}
						return res
					}
					backup[abs] = backupEntry{data: nil}
				} else {
					fi, _ := os.Stat(abs)
					mode := os.FileMode(0o644)
					if fi != nil {
						mode = fi.Mode()
					}
					backup[abs] = backupEntry{data: prev, mode: mode}
				}
			}
			if st.Action == "delete" {
				_ = os.Remove(abs)
			} else {
				content := st.SuggestionCode
				if !strings.HasSuffix(content, "\n") {
					content += "\n"
				}
				if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
					res.Messages = append(res.Messages, err.Error())
					rollback()
					res.Verify = VerifyResult{OK: false, Messages: res.Messages}
					return res
				}
			}
			res.Written = append(res.Written, st.Path)
		}
	}

	if len(res.Written) == 0 {
		res.Messages = append(res.Messages, "nothing applied (no suggestion_code on plan steps)")
		res.Verify = VerifyResult{OK: true, Messages: res.Messages}
		return res
	}

	v := VerifyFiles(repoDir, res.Written, runTests)
	res.Verify = v
	if !v.OK {
		res.Messages = append(res.Messages, "verification failed; rolling back")
		res.Messages = append(res.Messages, v.Messages...)
		rollback()
		return res
	}
	res.Messages = append(res.Messages, fmt.Sprintf("applied %d file(s); verify ok", len(res.Written)))
	res.Messages = append(res.Messages, v.Messages...)
	return res
}

// ApplyComments writes line-based suggestion_code from review/refactor comments
// under repoDir using backup → write → verify → rollback. Comments are grouped
// by path and applied in reverse StartLine order to preserve line numbers.
// Comments with empty SuggestionCode, empty Path, or StartLine <= 0 are skipped.
// Overlapping ranges on the same path cause all comments for that path to be skipped.
func ApplyComments(repoDir string, comments []model.LlmComment, runTests bool) ApplyResult {
	res := ApplyResult{}
	backup := map[string]backupEntry{}

	rollback := func() {
		for abs, e := range backup {
			if e.data == nil {
				_ = os.Remove(abs)
			} else {
				_ = os.WriteFile(abs, e.data, e.mode)
			}
		}
		res.RolledBack = true
	}

	// Group actionable comments by path, filtering invalid ones.
	type grouped struct {
		comments []model.LlmComment
	}
	byPath := map[string]*grouped{}

	for _, cm := range comments {
		if cm.SuggestionCode == "" || cm.Path == "" || cm.StartLine <= 0 {
			res.Skipped = append(res.Skipped, cm.Path+": no suggestion_code or invalid line numbers")
			continue
		}
		// Normalize unset or inverted EndLine to prevent capturing the entire file
		// in overlap detection and line replacement.
		if cm.EndLine < cm.StartLine {
			cm.EndLine = cm.StartLine
		}
		g, ok := byPath[cm.Path]
		if !ok {
			g = &grouped{}
			byPath[cm.Path] = g
		}
		g.comments = append(g.comments, cm)
	}

	if len(byPath) == 0 {
		res.Messages = append(res.Messages, "nothing applied (no actionable comments)")
		res.Verify = VerifyResult{OK: true, Messages: res.Messages}
		return res
	}

	// Sort paths for deterministic output.
	paths := make([]string, 0, len(byPath))
	for p := range byPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, relPath := range paths {
		g := byPath[relPath]
		// Sort by StartLine descending so replacements don't shift each other.
		sort.Slice(g.comments, func(i, j int) bool {
			return g.comments[i].StartLine > g.comments[j].StartLine
		})

		// Check for overlapping ranges.
		overlap := false
		for i := 0; i < len(g.comments)-1; i++ {
			a, b := g.comments[i], g.comments[i+1]
			// After descending sort, a.StartLine >= b.StartLine.
			// They overlap if b.EndLine >= a.StartLine.
			if b.EndLine >= a.StartLine {
				overlap = true
				break
			}
		}
		if overlap {
			res.Skipped = append(res.Skipped, relPath+": overlapping line ranges, skipped all")
			continue
		}

		abs := filepath.Join(repoDir, filepath.FromSlash(relPath))
		// Backup.
		if _, exists := backup[abs]; !exists {
			prev, err := os.ReadFile(abs)
			if err != nil {
				if !os.IsNotExist(err) {
					res.Messages = append(res.Messages, err.Error())
					rollback()
					res.Verify = VerifyResult{OK: false, Messages: res.Messages}
					return res
				}
				backup[abs] = backupEntry{data: nil}
			} else {
				fi, _ := os.Stat(abs)
				mode := os.FileMode(0o644)
				if fi != nil {
					mode = fi.Mode()
				}
				backup[abs] = backupEntry{data: prev, mode: mode}
			}
		}

		// Apply replacements.
		content := backup[abs].data
		if content == nil {
			content = []byte{}
		}
		for _, cm := range g.comments {
			content = replaceLineRange(content, cm.StartLine, cm.EndLine, cm.SuggestionCode)
		}

		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			res.Messages = append(res.Messages, err.Error())
			rollback()
			res.Verify = VerifyResult{OK: false, Messages: res.Messages}
			return res
		}
		if err := os.WriteFile(abs, content, 0o644); err != nil {
			res.Messages = append(res.Messages, err.Error())
			rollback()
			res.Verify = VerifyResult{OK: false, Messages: res.Messages}
			return res
		}
		res.Written = append(res.Written, relPath)
	}

	if len(res.Written) == 0 {
		res.Messages = append(res.Messages, "nothing applied (all comments skipped)")
		res.Verify = VerifyResult{OK: true, Messages: res.Messages}
		return res
	}

	v := VerifyFiles(repoDir, res.Written, runTests)
	res.Verify = v
	if !v.OK {
		res.Messages = append(res.Messages, "verification failed; rolling back")
		res.Messages = append(res.Messages, v.Messages...)
		rollback()
		return res
	}
	res.Messages = append(res.Messages, fmt.Sprintf("applied %d file(s); verify ok", len(res.Written)))
	res.Messages = append(res.Messages, v.Messages...)
	return res
}

// replaceLineRange replaces lines [startLine, endLine] (1-indexed, inclusive)
// in content with suggestion. Preserves trailing newline convention.
func replaceLineRange(content []byte, startLine, endLine int, suggestion string) []byte {
	if len(content) == 0 {
		return []byte(suggestion)
	}
	hasTrailingNewline := content[len(content)-1] == '\n'
	lines := strings.Split(string(content), "\n")
	// Drop empty trailing element from split on trailing newline.
	if hasTrailingNewline && len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	if startLine < 1 {
		startLine = 1
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	if startLine > len(lines) {
		// Append to end.
		suggestionLines := strings.Split(suggestion, "\n")
		lines = append(lines, suggestionLines...)
	} else {
		before := lines[:startLine-1]
		// endLine may be 0 when unset; normalize to startLine.
		if endLine < startLine {
			endLine = startLine
		}
		after := lines[endLine:] // endLine is inclusive
		suggestionLines := strings.Split(suggestion, "\n")
		lines = append(append(before, suggestionLines...), after...)
	}

	joined := strings.Join(lines, "\n")
	if hasTrailingNewline && !strings.HasSuffix(joined, "\n") {
		joined += "\n"
	}
	return []byte(joined)
}

// MaxRepairAttempts is the golden-rule self-correct limit.
const MaxRepairAttempts = 3
