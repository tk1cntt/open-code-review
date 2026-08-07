// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileWrite_CreateNewFile(t *testing.T) {
	dir := t.TempDir()
	p := NewFileWrite(dir)
	result, err := p.Execute(context.Background(), map[string]any{
		"file_path": "newfile.go",
		"content":   "package main\n\nfunc main() {}\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Successfully created") {
		t.Fatalf("expected 'Successfully created', got: %s", result)
	}

	got, err := os.ReadFile(filepath.Join(dir, "newfile.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "package main") {
		t.Fatalf("file content mismatch: %s", string(got))
	}
}

func TestFileWrite_OverwriteExistingFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "overwrite.go")
	content := "package old\n"
	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	p := NewFileWrite(dir)
	result, err := p.Execute(context.Background(), map[string]any{
		"file_path": "overwrite.go",
		"content":   "package new\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Successfully overwrote") {
		t.Fatalf("expected 'Successfully overwrote', got: %s", result)
	}

	got, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "package new") {
		t.Fatalf("file content mismatch: %s", string(got))
	}
	if strings.Contains(string(got), "package old") {
		t.Fatal("old content should have been overwritten")
	}
}

func TestFileWrite_EmptyPath(t *testing.T) {
	dir := t.TempDir()
	p := NewFileWrite(dir)
	result, err := p.Execute(context.Background(), map[string]any{
		"file_path": "",
		"content":   "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "file_path is required") {
		t.Fatalf("expected file_path required error, got: %s", result)
	}
}

func TestFileWrite_PathOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	p := NewFileWrite(dir)
	result, err := p.Execute(context.Background(), map[string]any{
		"file_path": "../outside.go",
		"content":   "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "outside repository") {
		t.Fatalf("expected outside-repository error, got: %s", result)
	}
}

func TestFileWrite_NestedDirectory(t *testing.T) {
	dir := t.TempDir()
	p := NewFileWrite(dir)
	result, err := p.Execute(context.Background(), map[string]any{
		"file_path": "sub/dir/new.go",
		"content":   "package sub\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Successfully created") {
		t.Fatalf("expected 'Successfully created', got: %s", result)
	}

	got, err := os.ReadFile(filepath.Join(dir, "sub", "dir", "new.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "package sub") {
		t.Fatalf("file content mismatch: %s", string(got))
	}
}
