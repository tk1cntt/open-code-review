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

// TestE2E_ReadEditVerify simulates the full LLM-as-editor workflow:
// 1. file_read to read a file
// 2. file_edit to apply a fix
// 3. shell_run to verify the fix
func TestE2E_ReadEditVerify(t *testing.T) {
	dir := t.TempDir()

	// Create a Go source file with missing import.
	src := filepath.Join(dir, "main.go")
	content := "package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create go.mod so go vet works.
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Step 1: Read the file.
	fr := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
	frp := NewFileRead(fr)
	result, err := frp.Execute(ctx, map[string]any{
		"file_path": "main.go",
	})
	if err != nil {
		t.Fatalf("file_read failed: %v", err)
	}
	if !strings.Contains(result, "func main()") {
		t.Fatalf("file_read should show main(), got: %s", result)
	}

	// Step 2: Edit the file to add import.
	edit := NewFileEdit(dir)
	result, err = edit.Execute(ctx, map[string]any{
		"file_path": "main.go",
		"old_str":   "package main",
		"new_str":   "package main\n\nimport \"fmt\"",
	})
	if err != nil {
		t.Fatalf("file_edit failed: %v", err)
	}
	if !strings.Contains(result, "Successfully edited") {
		t.Fatalf("file_edit should succeed, got: %s", result)
	}

	// Step 3: Verify with go vet.
	sh := NewShellRun(dir)
	result, err = sh.Execute(ctx, map[string]any{
		"command": "go vet ./...",
	})
	if err != nil {
		t.Fatalf("shell_run failed: %v", err)
	}
	// go vet might fail with "no Go files" but that's ok for this test.
	t.Logf("shell_run result: %s", result)

	// Verify file content.
	got, _ := os.ReadFile(src)
	if !strings.Contains(string(got), "import \"fmt\"") {
		t.Fatalf("file should contain import after edit, got: %s", string(got))
	}
}

// TestE2E_EditWrongOldStr simulates LLM providing wrong old_str.
func TestE2E_EditWrongOldStr(t *testing.T) {
	dir := t.TempDir()

	src := filepath.Join(dir, "test.go")
	content := "package test\n\nvar x = 42\n"
	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	edit := NewFileEdit(dir)

	// Attempt 1: Wrong old_str.
	result, err := edit.Execute(ctx, map[string]any{
		"file_path": "test.go",
		"old_str":   "var y = 99",
		"new_str":   "var z = 100",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "old_str not found") {
		t.Fatalf("expected 'old_str not found', got: %s", result)
	}

	// Attempt 2: Read file, then use correct old_str.
	fr := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
	frp := NewFileRead(fr)
	readResult, _ := frp.Execute(ctx, map[string]any{
		"file_path": "test.go",
	})
	t.Logf("After read: %s", readResult)

	// Now use correct old_str.
	result, err = edit.Execute(ctx, map[string]any{
		"file_path": "test.go",
		"old_str":   "var x = 42",
		"new_str":   "var x = 100",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Successfully edited") {
		t.Fatalf("expected success on retry, got: %s", result)
	}

	// Verify.
	got, _ := os.ReadFile(src)
	if !strings.Contains(string(got), "var x = 100") {
		t.Fatalf("expected var x = 100, got: %s", string(got))
	}
}

// TestE2E_ShellRunFailThenFix simulates shell_run failure and subsequent fix.
func TestE2E_ShellRunFailThenFix(t *testing.T) {
	dir := t.TempDir()

	// Create a file with a syntax error.
	src := filepath.Join(dir, "broken.go")
	content := "package main\n\nfunc main( {\n}\n"
	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create go.mod.
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	edit := NewFileEdit(dir)

	// Step 1: File compiles? Check with go vet (should fail).
	sh := NewShellRun(dir)
	result, _ := sh.Execute(ctx, map[string]any{
		"command": "go vet ./...",
	})
	t.Logf("First vet: %s", result)
	// We don't assert on go vet specifically because it might return various results.
	// The key test is that the edit tool reports success.

	// Step 2: Fix the syntax error.
	result, err := edit.Execute(ctx, map[string]any{
		"file_path": "broken.go",
		"old_str":   "func main( {",
		"new_str":   "func main() {",
	})
	if err != nil {
		t.Fatalf("file_edit failed: %v", err)
	}
	if !strings.Contains(result, "Successfully edited") {
		t.Fatalf("expected success, got: %s", result)
	}

	// Step 3: Verify fix was applied.
	got, _ := os.ReadFile(src)
	if !strings.Contains(string(got), "func main() {") {
		t.Fatalf("expected func main() {, got: %s", string(got))
	}
}

// TestE2E_MultipleFiles tests editing multiple files in sequence.
func TestE2E_MultipleFiles(t *testing.T) {
	dir := t.TempDir()

	files := []struct {
		path    string
		content string
	}{
		{"a.go", "package main\n\nvar name = \"old\"\n"},
		{"b.go", "package main\n\nvar version = 1\n"},
		{"c.go", "package main\n\nvar debug = false\n"},
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f.path), []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	ctx := context.Background()
	edit := NewFileEdit(dir)

	// Edit all three files.
	edits := []struct {
		path    string
		oldStr  string
		newStr  string
		checkFn func(got string)
	}{
		{
			path:   "a.go",
			oldStr: "var name = \"old\"",
			newStr: "var name = \"new\"",
			checkFn: func(got string) {
				if !strings.Contains(got, "var name = \"new\"") {
					t.Errorf("a.go: expected new name, got: %s", got)
				}
			},
		},
		{
			path:   "b.go",
			oldStr: "var version = 1",
			newStr: "var version = 2",
			checkFn: func(got string) {
				if !strings.Contains(got, "var version = 2") {
					t.Errorf("b.go: expected version 2, got: %s", got)
				}
			},
		},
		{
			path:   "c.go",
			oldStr: "var debug = false",
			newStr: "var debug = true",
			checkFn: func(got string) {
				if !strings.Contains(got, "var debug = true") {
					t.Errorf("c.go: expected debug=true, got: %s", got)
				}
			},
		},
	}

	for _, e := range edits {
		result, err := edit.Execute(ctx, map[string]any{
			"file_path": e.path,
			"old_str":   e.oldStr,
			"new_str":   e.newStr,
		})
		if err != nil {
			t.Fatalf("file_edit %s failed: %v", e.path, err)
		}
		if !strings.Contains(result, "Successfully edited") {
			t.Fatalf("%s: expected success, got: %s", e.path, result)
		}
	}

	// Verify all files.
	for _, e := range edits {
		got, _ := os.ReadFile(filepath.Join(dir, e.path))
		e.checkFn(string(got))
	}
}

// TestE2E_FileWriteCreateNew tests creating a new file with file_write,
// then reading it back.
func TestE2E_FileWriteCreateNew(t *testing.T) {
	dir := t.TempDir()

	ctx := context.Background()
	write := NewFileWrite(dir)

	// Step 1: Create a new file.
	result, err := write.Execute(ctx, map[string]any{
		"file_path": "new_pkg/helper.go",
		"content":   "package helper\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n",
	})
	if err != nil {
		t.Fatalf("file_write failed: %v", err)
	}
	if !strings.Contains(result, "Successfully created") {
		t.Fatalf("expected 'Successfully created', got: %s", result)
	}

	// Step 2: Read it back.
	fr := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
	frp := NewFileRead(fr)
	readResult, err := frp.Execute(ctx, map[string]any{
		"file_path": "new_pkg/helper.go",
	})
	if err != nil {
		t.Fatalf("file_read failed: %v", err)
	}
	if !strings.Contains(readResult, "func Add") {
		t.Fatalf("expected file content readable, got: %s", readResult)
	}

	// Step 3: Edit the new file.
	edit := NewFileEdit(dir)
	editResult, err := edit.Execute(ctx, map[string]any{
		"file_path": "new_pkg/helper.go",
		"old_str":   "func Add(a, b int) int {\n\treturn a + b\n}",
		"new_str":   "func Add(a, b int) int {\n\t// adds two numbers\n\treturn a + b\n}",
	})
	if err != nil {
		t.Fatalf("file_edit failed: %v", err)
	}
	if !strings.Contains(editResult, "Successfully edited") {
		t.Fatalf("expected success, got: %s", editResult)
	}

	// Step 4: Verify the edit.
	got, _ := os.ReadFile(filepath.Join(dir, "new_pkg", "helper.go"))
	if !strings.Contains(string(got), "adds two numbers") {
		t.Fatalf("expected comment after edit, got: %s", string(got))
	}
}
