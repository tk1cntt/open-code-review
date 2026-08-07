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

// FileWriteProvider writes (or overwrites) a file's entire content.
// It is intended for creating new files or fully rewriting small files
// that the LLM needs to generate from scratch.
type FileWriteProvider struct {
	RepoDir string
}

// NewFileWrite creates a FileWriteProvider scoped to the repository root.
func NewFileWrite(repoDir string) *FileWriteProvider {
	return &FileWriteProvider{RepoDir: repoDir}
}

func (p *FileWriteProvider) Tool() Tool { return FileWrite }

func (p *FileWriteProvider) Execute(ctx context.Context, args map[string]any) (string, error) {
	filePath, _ := args["file_path"].(string)
	if filePath == "" {
		return "Error: file_path is required", nil
	}

	content, _ := args["content"].(string)
	// content can be empty (create empty file) — that is allowed.

	repoRoot, err := pathutil.CanonicalPath(p.RepoDir)
	if err != nil {
		return fmt.Sprintf("Error: resolve repository path %q: %v", p.RepoDir, err), nil
	}

	fullPath := filepath.Join(repoRoot, filePath)
	if !pathutil.WithinBase(repoRoot, fullPath) {
		return fmt.Sprintf("Error: file path %q is outside repository", filePath), nil
	}

	// Resolve symlinks on the parent directory to detect escapes,
	// but tolerate the file itself not existing (we are creating it).
	parentDir := filepath.Dir(fullPath)
	resolvedParent, err := filepath.EvalSymlinks(parentDir)
	if err != nil {
		// Parent does not exist either — we'll create it below.
		resolvedParent = parentDir
	}
	if !pathutil.WithinBase(repoRoot, resolvedParent) {
		return fmt.Sprintf("Error: file path %q is outside repository", filePath), nil
	}

	// Create parent directories if needed.
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return fmt.Sprintf("Error: failed to create directory %q: %v", parentDir, err), nil
	}

	// Determine if creating or overwriting.
	_, statErr := os.Stat(fullPath)
	isNew := os.IsNotExist(statErr)

	// Ensure trailing newline for text files.
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		return fmt.Sprintf("Error: failed to write %q: %v", filePath, err), nil
	}

	lineCount := strings.Count(content, "\n")
	if isNew {
		return fmt.Sprintf("Successfully created %q with %d line(s).", filePath, lineCount), nil
	}
	return fmt.Sprintf("Successfully overwrote %q with %d line(s).", filePath, lineCount), nil
}
