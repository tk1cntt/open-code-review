package crossfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestApplyPlanSteps_CreateFile verifies creating a new file works end-to-end.
func TestApplyPlanSteps_CreateFile(t *testing.T) {
	dir := t.TempDir()

	// Pre-create an existing file that will be modified
	src := filepath.Join(dir, "user.go")
	content := "package user\n\nfunc GetName() string {\n\treturn \"anonymous\"\n}\n"
	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Plan: create new file + modify existing file
	plans := []RefactorPlan{
		{
			PlanID:  "plan-1",
			Summary: "Extract helper to shared package",
			Steps: []PlanStep{
				{
					Order:          1,
					Action:         "create_file",
					Path:           "pkg/helper/helper.go",
					Symbol:         "Helper",
					SuggestionCode: "package helper\n\nfunc GetGreeting() string {\n\treturn \"hello\"\n}\n",
				},
				{
					Order:          2,
					Action:         "modify",
					Path:           "user.go",
					SuggestionCode: "package user\n\nimport \"pkg/helper\"\n\nfunc GetName() string {\n\treturn helper.GetGreeting()\n}\n",
				},
			},
		},
	}

	res := ApplyPlanSteps(dir, plans, false)

	if !res.Verify.OK {
		t.Fatalf("apply failed: %v", res.Messages)
	}
	if res.RolledBack {
		t.Fatal("unexpected rollback")
	}
	if len(res.Written) != 2 {
		t.Fatalf("expected 2 files written, got %d: %v", len(res.Written), res.Written)
	}

	// Verify new file exists and has correct content
	helperPath := filepath.Join(dir, "pkg", "helper", "helper.go")
	helperContent, err := os.ReadFile(helperPath)
	if err != nil {
		t.Fatalf("helper file not created: %v", err)
	}
	if !strings.Contains(string(helperContent), "GetGreeting") {
		t.Fatalf("helper file missing expected content: %s", string(helperContent))
	}

	// Verify user.go was modified correctly
	userContent, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("user.go missing: %v", err)
	}
	if !strings.Contains(string(userContent), "pkg/helper") {
		t.Fatalf("user.go missing import: %s", string(userContent))
	}
	if !strings.Contains(string(userContent), "helper.GetGreeting()") {
		t.Fatalf("user.go missing call to helper: %s", string(userContent))
	}
}

// TestApplyPlanSteps_DeleteFile verifies delete action works.
func TestApplyPlanSteps_DeleteFile(t *testing.T) {
	dir := t.TempDir()

	// Create a file to delete
	src := filepath.Join(dir, "deprecated.go")
	if err := os.WriteFile(src, []byte("package main\n\nfunc OldFunc() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	plans := []RefactorPlan{
		{
			PlanID:  "plan-2",
			Summary: "Remove deprecated code",
			Steps: []PlanStep{
				{
					Order:  1,
					Action: "delete",
					Path:   "deprecated.go",
				},
			},
		},
	}

	res := ApplyPlanSteps(dir, plans, false)

	if !res.Verify.OK {
		t.Fatalf("apply failed: %v", res.Messages)
	}
	if res.RolledBack {
		t.Fatal("unexpected rollback")
	}

	// Verify file is deleted
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("deprecated.go should be deleted but still exists")
	}
}

// TestApplyPlanSteps_RollbackOnVerifyFailure verifies rollback on bad code.
func TestApplyPlanSteps_RollbackOnVerifyFailure(t *testing.T) {
	dir := t.TempDir()

	// Pre-create a valid file
	src := filepath.Join(dir, "main.go")
	content := "package main\n\nfunc main() {\n\tprintln(\"ok\")\n}\n"
	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Backup original
	origData, _ := os.ReadFile(src)

	// Plan with suggestion_code that will fail Go parse (syntax error)
	plans := []RefactorPlan{
		{
			PlanID:  "plan-3",
			Summary: "Invalid Go syntax",
			Steps: []PlanStep{
				{
					Order:          1,
					Action:         "modify",
					Path:           "main.go",
					SuggestionCode: "package main\n\nfunc main() {\n\tprintln(\"ok\"\n}\n", // missing closing paren
				},
			},
		},
	}

	// Create go.mod so VerifyFiles can parse as Go
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := ApplyPlanSteps(dir, plans, false)

	if res.Verify.OK {
		t.Fatal("expected verify to fail for invalid Go syntax")
	}
	if !res.RolledBack {
		t.Fatal("expected rollback on verify failure")
	}

	// Verify original content was restored
	restored, _ := os.ReadFile(src)
	if string(restored) != string(origData) {
		t.Fatalf("rollback did not restore original content.\nExpected: %s\nGot: %s", origData, restored)
	}
}

// TestApplyPlanSteps_EmptySuggestionCodeSkipped verifies that steps
// without suggestion_code are skipped gracefully (not applied, no crash).
func TestApplyPlanSteps_EmptySuggestionCodeSkipped(t *testing.T) {
	dir := t.TempDir()

	plans := []RefactorPlan{
		{
			PlanID:  "plan-4",
			Summary: "Plan without code",
			Steps: []PlanStep{
				{
					Order:          1,
					Action:         "create_file",
					Path:           "newfile.go",
					SuggestionCode: "", // empty — should be skipped
				},
			},
		},
	}

	res := ApplyPlanSteps(dir, plans, false)

	// Should not crash, just skip
	if len(res.Skipped) != 1 {
		t.Fatalf("expected 1 skipped step, got %d: %v", len(res.Skipped), res.Skipped)
	}
	if !strings.Contains(res.Skipped[0], "no suggestion_code") {
		t.Fatalf("expected skip message about suggestion_code, got: %s", res.Skipped[0])
	}
	// File should NOT exist
	newFilePath := filepath.Join(dir, "newfile.go")
	if _, err := os.Stat(newFilePath); !os.IsNotExist(err) {
		t.Fatal("newfile.go should NOT exist but was created")
	}
}

// TestApplyPlanSteps_MultiFileDependencyOrder verifies topological ordering.
// create_file must happen before modify that references the new file.
func TestApplyPlanSteps_MultiFileDependencyOrder(t *testing.T) {
	dir := t.TempDir()

	// Two existing files with duplicated logic
	content1 := "package a\n\nfunc DoA() {\n\t// duplicated\n\tprintln(\"hello\")\n}\n"
	content2 := "package b\n\nfunc DoB() {\n\t// duplicated\n\tprintln(\"hello\")\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte(content1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte(content2), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	plans := []RefactorPlan{
		{
			PlanID:  "plan-5",
			Summary: "Extract shared print helper",
			Steps: []PlanStep{
				{
					Order:          1,
					Action:         "create_file",
					Path:           "shared/print.go",
					Symbol:         "PrintHello",
					SuggestionCode: "package shared\n\nimport \"fmt\"\n\nfunc PrintHello() {\n\tfmt.Println(\"hello\")\n}\n",
				},
				{
					Order:          2,
					Action:         "modify",
					Path:           "a.go",
					SuggestionCode: "package a\n\nimport \"example/shared\"\n\nfunc DoA() {\n\tshared.PrintHello()\n}\n",
				},
				{
					Order:          3,
					Action:         "modify",
					Path:           "b.go",
					SuggestionCode: "package b\n\nimport \"example/shared\"\n\nfunc DoB() {\n\tshared.PrintHello()\n}\n",
				},
			},
		},
	}

	res := ApplyPlanSteps(dir, plans, false)

	if !res.Verify.OK {
		t.Fatalf("apply failed: %v", res.Messages)
	}
	if res.RolledBack {
		t.Fatal("unexpected rollback")
	}
	if len(res.Written) != 3 {
		t.Fatalf("expected 3 files written, got %d: %v", len(res.Written), res.Written)
	}

	// Verify all files
	for _, p := range []string{"shared/print.go", "a.go", "b.go"} {
		fullPath := filepath.Join(dir, filepath.FromSlash(p))
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			t.Fatalf("expected file to exist: %s", p)
		}
	}

	// Verify a.go imports shared
	aContent, _ := os.ReadFile(filepath.Join(dir, "a.go"))
	if !strings.Contains(string(aContent), "example/shared") {
		t.Fatalf("a.go missing import: %s", string(aContent))
	}
	if !strings.Contains(string(aContent), "shared.PrintHello()") {
		t.Fatalf("a.go missing call: %s", string(aContent))
	}
}