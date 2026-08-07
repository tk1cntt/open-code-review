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

func TestFileEdit_ReplaceSingleLine(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	content := "package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	p := NewFileEdit(dir)
	result, err := p.Execute(context.Background(), map[string]any{
		"file_path": "main.go",
		"old_str":   "fmt.Println(\"hello\")",
		"new_str":   "fmt.Println(\"world\")",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Successfully edited") {
		t.Fatalf("expected success, got: %s", result)
	}

	got, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "fmt.Println(\"world\")") {
		t.Fatalf("expected replacement to be applied, got: %s", string(got))
	}
	if strings.Contains(string(got), "fmt.Println(\"hello\")") {
		t.Fatal("old text should have been replaced")
	}
}

func TestFileEdit_ReplaceMultipleLines(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "utils.go")
	content := "package utils\n\nfunc add(a, b int) int {\n\treturn a + b\n}\n"
	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	p := NewFileEdit(dir)
	result, err := p.Execute(context.Background(), map[string]any{
		"file_path": "utils.go",
		"old_str":   "func add(a, b int) int {\n\treturn a + b\n}",
		"new_str":   "func add(a, b int) int {\n\t// optimized\n\treturn a + b\n}",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Successfully edited") {
		t.Fatalf("expected success, got: %s", result)
	}
}

func TestFileEdit_MissingFile(t *testing.T) {
	dir := t.TempDir()
	p := NewFileEdit(dir)
	result, err := p.Execute(context.Background(), map[string]any{
		"file_path": "nonexistent.go",
		"old_str":   "some text",
		"new_str":   "other text",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Should not be "Successfully" — should be an error about file not found
	if strings.Contains(result, "Successfully") {
		t.Fatalf("expected error for missing file, got: %s", result)
	}
}

func TestFileEdit_OldStrNotFound(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "test.go")
	content := "package test\n\nvar x = 1\n"
	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	p := NewFileEdit(dir)
	result, err := p.Execute(context.Background(), map[string]any{
		"file_path": "test.go",
		"old_str":   "var y = 2",
		"new_str":   "var y = 3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "old_str not found") {
		t.Fatalf("expected old_str not found error, got: %s", result)
	}
}

func TestFileEdit_MultipleMatches(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "dup.go")
	content := "package dup\n\nfmt.Println(\"a\")\nfmt.Println(\"b\")\nfmt.Println(\"a\")\n"
	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	p := NewFileEdit(dir)
	result, err := p.Execute(context.Background(), map[string]any{
		"file_path": "dup.go",
		"old_str":   "fmt.Println(\"a\")",
		"new_str":   "fmt.Println(\"x\")",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "appears 2 times") {
		t.Fatalf("expected 'appears 2 times' error, got: %s", result)
	}
}

func TestFileEdit_EmptyFilepath(t *testing.T) {
	dir := t.TempDir()
	p := NewFileEdit(dir)
	result, err := p.Execute(context.Background(), map[string]any{
		"file_path": "",
		"old_str":   "test",
		"new_str":   "test2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "file_path is required") {
		t.Fatalf("expected file_path required error, got: %s", result)
	}
}

func TestFileEdit_EmptyOldStr(t *testing.T) {
	dir := t.TempDir()
	p := NewFileEdit(dir)
	result, err := p.Execute(context.Background(), map[string]any{
		"file_path": "test.go",
		"old_str":   "",
		"new_str":   "test2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "old_str is required") {
		t.Fatalf("expected old_str required error, got: %s", result)
	}
}

func TestFileEdit_PathOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	p := NewFileEdit(dir)
	result, err := p.Execute(context.Background(), map[string]any{
		"file_path": "../outside.go",
		"old_str":   "test",
		"new_str":   "test2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "outside repository") {
		t.Fatalf("expected outside-repository error, got: %s", result)
	}
}
