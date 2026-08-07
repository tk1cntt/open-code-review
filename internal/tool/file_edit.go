// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alibaba/open-code-review/internal/pathutil"
)

// FileEditProvider replaces a block of text in a file by exact string matching.
// It mirrors the behaviour of the classic "str_replace_editor" pattern: the LLM
// provides old_str (the exact text to replace, including whitespace) and new_str
// (the replacement text). The tool reads the current file, locates old_str
// through exact substring search, and writes back the modified content.
type FileEditProvider struct {
	RepoDir string
}

// NewFileEdit creates a FileEditProvider scoped to the repository root.
func NewFileEdit(repoDir string) *FileEditProvider {
	return &FileEditProvider{RepoDir: repoDir}
}

func (p *FileEditProvider) Tool() Tool { return FileEdit }

func (p *FileEditProvider) Execute(ctx context.Context, args map[string]any) (string, error) {
	filePath, _ := args["file_path"].(string)
	if filePath == "" {
		return "Error: file_path is required", nil
	}

	oldStr, _ := args["old_str"].(string)
	if oldStr == "" {
		return "Error: old_str is required and must not be empty", nil
	}

	newStr, _ := args["new_str"].(string)
	// newStr can be empty (deletion) — that is allowed.

	absPath, content, err := p.readAbs(filePath)
	if err != nil {
		return err.Error(), nil
	}

	// Locate old_str via exact substring match.
	count := strings.Count(content, oldStr)
	if count == 0 {
		// Provide a short excerpt of the file content for the LLM to adjust.
		excerpt := content
		if len(excerpt) > 500 {
			excerpt = excerpt[:500]
		}
		excerpt = strings.ReplaceAll(excerpt, "\n", "\\n")
		return fmt.Sprintf(
			"Error: old_str not found in file. The exact text you provided does not appear "+
				"in %q. Please use file_read to re-read the file and provide the exact text "+
				"(including indentation and line endings).\n"+
				"File excerpt (first 500 chars): %s...", filePath, excerpt), nil
	}
	if count > 1 {
		return fmt.Sprintf(
			"Error: old_str appears %d times in the file. The match must be unique. "+
				"Please include more surrounding context lines in old_str to make the match "+
				"unambiguous. Tip: use file_read to see the lines near your target and "+
				"include a few extra lines of context.", count), nil
	}

	// Perform the replacement.
	replaced := strings.Replace(content, oldStr, newStr, 1)

	// Write back.
	if err := os.WriteFile(absPath, []byte(replaced), 0o644); err != nil {
		return fmt.Sprintf("Error: failed to write %q: %v", filePath, err), nil
	}

	// Compute a human-readable line range for the change.
	beforeOld := content[:strings.Index(content, oldStr)]
	startLine := strings.Count(beforeOld, "\n") + 1
	oldLineCount := strings.Count(oldStr, "\n") + 1
	newLineCount := strings.Count(newStr, "\n") + 1
	endLine := startLine + oldLineCount - 1

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Successfully edited %q.\n", filePath))
	if oldLineCount != newLineCount {
		sb.WriteString(fmt.Sprintf("Replaced lines %d-%d (%d lines) with %d new lines.\n",
			startLine, endLine, oldLineCount, newLineCount))
	} else {
		sb.WriteString(fmt.Sprintf("Modified lines %d-%d (%d line(s)).\n",
			startLine, endLine, oldLineCount))
	}
	sb.WriteString("Tip: use shell_run to verify the change compiles/tests pass.")
	return sb.String(), nil
}

// readAbs resolves and reads a file path relative to the repository root.
func (p *FileEditProvider) readAbs(path string) (string, string, error) {
	repoRoot, err := pathutil.CanonicalPath(p.RepoDir)
	if err != nil {
		return "", "", fmt.Errorf("Error: resolve repository path %q: %v", p.RepoDir, err)
	}

	fullPath := filepath.Join(repoRoot, path)
	if !pathutil.WithinBase(repoRoot, fullPath) {
		return "", "", fmt.Errorf("Error: file path %q is outside repository", path)
	}

	resolvedPath, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File does not exist — return a clean path so the caller
			// can report that old_str was not found (file is empty/inaccessible).
			return fullPath, "", fmt.Errorf("Error: file %q not found", path)
		}
		return "", "", fmt.Errorf("Error: resolve file %q: %v", path, err)
	}
	if !pathutil.WithinBase(repoRoot, resolvedPath) {
		return "", "", fmt.Errorf("Error: file path %q is outside repository", path)
	}

	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		return "", "", fmt.Errorf("Error: read file %q: %v", path, err)
	}

	return resolvedPath, string(content), nil
}
