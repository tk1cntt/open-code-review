package reviewstore

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/model"
)

var cstLoc = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	return loc
}()

const maxSuggestionLen = 3000

var severityOrder = []string{"critical", "high", "medium", "low"}

var severityHeader = map[string]string{
	"critical": "\U0001F534 Critical",
	"high":     "\U0001F7E0 High",
	"medium":   "\U0001F7E1 Medium",
	"low":      "\U0001F535 Low",
}

var categoryLabel = map[string]string{
	"bug":             "\U0001F41B Bug",
	"security":        "\U0001F512 Security",
	"performance":     "⚡ Performance",
	"maintainability": "\U0001F527 Maintainability",
	"test":            "✅ Test",
	"style":           "\U0001F3A8 Style",
	"documentation":   "\U0001F4DA Documentation",
	"other":           "\U0001F4DD Other",
}

// WriteMarkdown generates a human-readable Markdown report for the review
// result and writes it to <dir>/<result.ID>.md using an atomic write pattern.
func WriteMarkdown(root string, result Result) (string, error) {
	projectKey := ProjectKey(result.Project)
	if !isSafePathSegment(projectKey) {
		return "", fmt.Errorf("unsafe project key: %q", projectKey)
	}
	if !isSafePathSegment(result.ID) {
		return "", fmt.Errorf("unsafe review ID: %q", result.ID)
	}
	dir := filepath.Join(root, projectKey)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create review result dir: %w", err)
	}

	path := filepath.Join(dir, result.ID+".md")
	content := renderMarkdown(result)

	tmp, err := os.CreateTemp(dir, result.ID+".*")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("write markdown report: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("rename markdown report: %w", err)
	}
	return path, nil
}

func renderMarkdown(result Result) []byte {
	var b bytes.Buffer

	p := result.Project
	name := p.Name
	if name == "" {
		name = p.RepoDir
	}

	// ── Header ─────────────────────────────────────────────
	b.WriteString("# Code Review Report\n\n")
	b.WriteString(fmt.Sprintf("**Project:** %s  \n", name))
	b.WriteString(fmt.Sprintf("**Review ID:** `%s`  \n", result.ID))
	b.WriteString(fmt.Sprintf("**Date:** %s  \n", result.CreatedAt.In(cstLoc).Format("2006-01-02 15:04 MST")))
	b.WriteString(fmt.Sprintf("**Mode:** %s", result.Review.Mode))
	if result.Review.From != "" && result.Review.To != "" {
		b.WriteString(fmt.Sprintf(" (%s..%s)", result.Review.From, result.Review.To))
	}
	if result.Review.Commit != "" {
		b.WriteString(fmt.Sprintf(" (commit %s)", result.Review.Commit))
	}
	b.WriteString("  \n")
	b.WriteString(fmt.Sprintf("**Model:** %s  \n", result.Review.Model))
	b.WriteString(fmt.Sprintf("**Files Reviewed:** %d | **Comments:** %d  \n", result.Review.FilesReviewed, result.Review.CommentCount))
	b.WriteString(fmt.Sprintf("**Duration:** %s | **Tokens:** %s (input: %s, output: %s)  \n",
		result.Review.Duration,
		formatTokenCount(result.Review.TotalTokens),
		formatTokenCount(result.Review.InputTokens),
		formatTokenCount(result.Review.OutputTokens),
	))
	if result.Review.SessionID != "" {
		b.WriteString(fmt.Sprintf("**Session:** `%s`  \n", result.Review.SessionID))
	}
	b.WriteString("\n---\n\n")

	// ── Summary table ──────────────────────────────────────
	bySeverity := groupBySeverity(result.Comments)
	b.WriteString("## Summary by Severity\n\n")
	b.WriteString("| Severity | Count |\n|----------|-------|\n")
	for _, sev := range severityOrder {
		count := len(bySeverity[sev])
		if count > 0 {
			b.WriteString(fmt.Sprintf("| %s | **%d** |\n", severityHeader[sev], count))
		} else {
			b.WriteString(fmt.Sprintf("| %s | %d |\n", severityHeader[sev], count))
		}
	}
	b.WriteString("\n---\n\n")

	// ── Comments by severity ───────────────────────────────
	for _, sev := range severityOrder {
		comments := bySeverity[sev]
		if len(comments) == 0 {
			b.WriteString(fmt.Sprintf("## %s (0)\n\n", severityHeader[sev]))
			if sev == "critical" {
				b.WriteString("No critical issues found. :tada:\n\n")
			} else {
				b.WriteString("No issues found.\n\n")
			}
			continue
		}

		b.WriteString(fmt.Sprintf("## %s (%d)\n\n", severityHeader[sev], len(comments)))

		byFile := groupByFile(comments)
		for _, filePath := range sortedKeys(byFile) {
			fileComments := byFile[filePath]
			if len(fileComments) == 1 {
				b.WriteString(fmt.Sprintf("### `%s`\n\n", filePath))
			} else {
				b.WriteString(fmt.Sprintf("### `%s` (%d issues)\n\n", filePath, len(fileComments)))
			}

			for i, c := range fileComments {
				renderComment(&b, c)
				if i < len(fileComments)-1 {
					b.WriteString("---\n\n")
				}
			}
		}
		b.WriteString("\n")
	}

	// ── Warnings ───────────────────────────────────────────
	if len(result.Warnings) > 0 {
		b.WriteString("---\n\n## Warnings\n\n")
		for _, w := range result.Warnings {
			b.WriteString(fmt.Sprintf("- **`%s`** (%s): %s\n", w.File, w.Type, w.Message))
		}
		b.WriteString("\n")
	}

	// ── Footer ─────────────────────────────────────────────
	b.WriteString(fmt.Sprintf("---\n\n*Report generated by [open-code-review](https://github.com/alibaba/open-code-review) at %s*\n",
		time.Now().In(cstLoc).Format("2006-01-02 15:04 MST")))

	return b.Bytes()
}

func renderComment(b *bytes.Buffer, c model.LlmComment) {
	cat := categoryLabel[c.Category]
	if cat == "" {
		cat = c.Category
	}

	fmt.Fprintf(b, "**%s**", cat)
	if c.StartLine > 0 || c.EndLine > 0 {
		if c.StartLine == c.EndLine {
			fmt.Fprintf(b, " · line %d", c.StartLine)
		} else if c.StartLine > 0 && c.EndLine > 0 {
			fmt.Fprintf(b, " · lines %d-%d", c.StartLine, c.EndLine)
		} else if c.StartLine > 0 {
			fmt.Fprintf(b, " · line %d", c.StartLine)
		} else if c.EndLine > 0 {
			fmt.Fprintf(b, " · line %d", c.EndLine)
		}
	}
	b.WriteString("\n\n")

	b.WriteString(c.Content)
	b.WriteString("\n\n")

	if c.SuggestionCode != "" {
		b.WriteString("<details>\n<summary>:bulb: Suggestion</summary>\n\n```")
		lang := detectCodeLang(c.Path)
		b.WriteString(lang)
		b.WriteString("\n")
		b.WriteString(truncate(c.SuggestionCode, maxSuggestionLen))
		b.WriteString("\n```\n</details>\n\n")
	}

	if c.ExistingCode != "" {
		b.WriteString("<details>\n<summary>:clipboard: Existing Code</summary>\n\n```")
		lang := detectCodeLang(c.Path)
		b.WriteString(lang)
		b.WriteString("\n")
		b.WriteString(truncate(c.ExistingCode, maxSuggestionLen))
		b.WriteString("\n```\n</details>\n\n")
	}
}

func groupBySeverity(comments []model.LlmComment) map[string][]model.LlmComment {
	m := make(map[string][]model.LlmComment)
	for _, c := range comments {
		sev := strings.ToLower(c.Severity)
		if sev == "" {
			sev = "low"
		}
		m[sev] = append(m[sev], c)
	}
	return m
}

func groupByFile(comments []model.LlmComment) map[string][]model.LlmComment {
	m := make(map[string][]model.LlmComment)
	for _, c := range comments {
		m[c.Path] = append(m[c.Path], c)
	}
	return m
}

func sortedKeys(m map[string][]model.LlmComment) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func detectCodeLang(path string) string {
	switch {
	case strings.HasSuffix(path, ".go"):
		return "go"
	case strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".jsx"):
		return "javascript"
	case strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".tsx"):
		return "typescript"
	case strings.HasSuffix(path, ".py"):
		return "python"
	case strings.HasSuffix(path, ".rs"):
		return "rust"
	case strings.HasSuffix(path, ".java"):
		return "java"
	case strings.HasSuffix(path, ".rb"):
		return "ruby"
	case strings.HasSuffix(path, ".c") || strings.HasSuffix(path, ".h"):
		return "c"
	case strings.HasSuffix(path, ".cpp") || strings.HasSuffix(path, ".hpp") || strings.HasSuffix(path, ".cc"):
		return "cpp"
	case strings.HasSuffix(path, ".sh") || strings.HasSuffix(path, ".bash"):
		return "bash"
	case strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml"):
		return "yaml"
	case strings.HasSuffix(path, ".json"):
		return "json"
	case strings.HasSuffix(path, ".md"):
		return "markdown"
	case strings.HasSuffix(path, ".sql"):
		return "sql"
	case strings.HasSuffix(path, ".tf"):
		return "hcl"
	default:
		return ""
	}
}

func formatTokenCount(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n... (truncated)"
}

// writePerFileMarkdown writes a per-file markdown report for a single reviewed file.
func writePerFileMarkdown(baseDir, mdRelPath string, result Result, filePath string, comments []model.LlmComment) error {
	mdFullPath := filepath.Join(baseDir, filepath.FromSlash(mdRelPath))
	content := renderFileMarkdown(filePath, result, comments)

	dir := filepath.Dir(mdFullPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create per-file md dir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(mdFullPath)+".*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, mdFullPath)
}

// renderFileMarkdown produces a compact per-file markdown report.
func renderFileMarkdown(filePath string, result Result, comments []model.LlmComment) []byte {
	var b bytes.Buffer

	projectName := result.Project.Name
	if projectName == "" {
		projectName = result.Project.RepoDir
	}

	b.WriteString(fmt.Sprintf("# Review: `%s`\n\n", filePath))
	b.WriteString(fmt.Sprintf("**Project:** %s | **Review:** `%s`\n\n", projectName, result.ID))
	b.WriteString(fmt.Sprintf("**Comments:** %d\n\n", len(comments)))
	b.WriteString("---\n\n")

	bySeverity := groupBySeverity(comments)
	for _, sev := range severityOrder {
		sevComments := bySeverity[sev]
		if len(sevComments) == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("## %s (%d)\n\n", severityHeader[sev], len(sevComments)))
		for i, c := range sevComments {
			renderComment(&b, c)
			if i < len(sevComments)-1 {
				b.WriteString("---\n\n")
			}
		}
		b.WriteString("\n")
	}

	return b.Bytes()
}
