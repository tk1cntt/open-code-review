// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package fileutil provides shared file utilities used across packages.
package fileutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ValidateFileSyntax runs a quick syntax check on the given file.
// Returns nil if valid, error with details if invalid.
// Unknown file types are silently accepted (nil).
func ValidateFileSyntax(path string) error {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".tsx", ".ts", ".jsx", ".js":
		return validateTypeScript(path)
	case ".css", ".scss", ".less":
		return validateCSS(path)
	case ".go":
		return validateGo(path)
	default:
		return nil // unknown file type, skip validation
	}
}

func validateTypeScript(path string) error {
	// Try npx tsc --noEmit for the specific file.
	// If tsc not available, skip (don't fail).
	if _, err := exec.LookPath("npx"); err != nil {
		return nil
	}
	cmd := exec.Command("npx", "tsc", "--noEmit", "--pretty", "false", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("typescript syntax error: %s", string(output))
	}
	return nil
}

func validateCSS(path string) error {
	// Basic CSS validation: check for property:value rules outside selectors.
	// A rule outside a selector block looks like "property: value;" not inside { ... }.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil // can't read, skip validation
	}
	content := string(data)

	// Simple heuristic: find lines with "property: value" pattern
	// that are not inside a selector block.
	lines := strings.Split(content, "\n")
	inBlock := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip empty, comments, @-rules.
		if trimmed == "" || strings.HasPrefix(trimmed, "/*") ||
			strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "@") ||
			strings.HasPrefix(trimmed, "*") {
			continue
		}
		// Count braces.
		inBlock += strings.Count(trimmed, "{") - strings.Count(trimmed, "}")
		// If outside a block and looks like a CSS property.
		if inBlock <= 0 && strings.Contains(trimmed, ":") &&
			!strings.Contains(trimmed, "{") && !strings.Contains(trimmed, "}") &&
			!strings.HasPrefix(trimmed, "import") && !strings.HasPrefix(trimmed, ".") &&
			!strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "&") {
			// Check if it looks like a property:value (not a selector).
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				prop := strings.TrimSpace(parts[0])
				// CSS properties are lowercase and contain hyphens or are single words.
				if !strings.Contains(prop, " ") && len(prop) > 1 {
					return fmt.Errorf("line %d: CSS property '%s' outside selector block", i+1, prop)
				}
			}
		}
	}
	return nil
}

func validateGo(path string) error {
	// Use gofmt -e for syntax checking.
	if _, err := exec.LookPath("gofmt"); err != nil {
		return nil
	}
	cmd := exec.Command("gofmt", "-e", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go syntax error: %s", string(output))
	}
	if len(output) > 0 {
		return fmt.Errorf("go format issue: %s", string(output))
	}
	return nil
}
