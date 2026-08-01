package reviewstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/model"
)

func TestSaveAndLoad(t *testing.T) {
	root := t.TempDir()
	result := Result{
		Project: ProjectInfo{Name: "my-project", RepoDir: "/home/dev/repo"},
		Review: ReviewInfo{
			Mode:          "range",
			SourceBranch:  "feature/x",
			TargetBranch:  "main",
			From:          "main",
			To:            "feature/x",
			Model:         "claude-3",
			FilesReviewed: 5,
			CommentCount:  3,
			TotalTokens:   15000,
		},
		Comments: []model.LlmComment{
			{Path: "a.go", Content: "fix this", Severity: "high", Category: "bug"},
		},
	}

	path, _, err := Save(root, result)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty path")
	}

	// Extract project key and review ID from the saved path
	savedDir := filepath.Dir(path) // .../reviews/<project-key>
	projectKey := filepath.Base(savedDir)
	savedFile := filepath.Base(path) // <review-id>.json
	reviewID := strings.TrimSuffix(savedFile, ".json")

	loaded, err := Load(root, projectKey, reviewID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ID != reviewID {
		t.Errorf("ID = %q, want %q", loaded.ID, reviewID)
	}
	if loaded.Project.Name != "my-project" {
		t.Errorf("Project.Name = %q, want my-project", loaded.Project.Name)
	}
	if loaded.Review.Mode != "range" {
		t.Errorf("Review.Mode = %q, want range", loaded.Review.Mode)
	}
	if loaded.Review.SourceBranch != "feature/x" {
		t.Errorf("SourceBranch = %q", loaded.Review.SourceBranch)
	}
	if len(loaded.Comments) != 1 {
		t.Fatalf("len(Comments) = %d, want 1", len(loaded.Comments))
	}
	if loaded.Comments[0].Content != "fix this" {
		t.Errorf("Comments[0].Content = %q", loaded.Comments[0].Content)
	}
}

func TestSaveAutoGeneratesID(t *testing.T) {
	root := t.TempDir()
	result := Result{
		Project: ProjectInfo{Name: "test"},
	}
	path, _, err := Save(root, result)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Extract ID from the saved path
	savedFile := filepath.Base(path)
	reviewID := strings.TrimSuffix(savedFile, ".json")
	if reviewID == "" {
		t.Fatal("expected ID to be generated")
	}
}

func TestSaveFillsCreatedAt(t *testing.T) {
	root := t.TempDir()
	result := Result{Project: ProjectInfo{Name: "test"}}
	path, _, err := Save(root, result)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Extract review ID from the saved path and load back
	savedDir := filepath.Dir(path)
	projectKey := filepath.Base(savedDir)
	savedFile := filepath.Base(path)
	reviewID := strings.TrimSuffix(savedFile, ".json")
	loaded, err := Load(root, projectKey, reviewID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt to be filled")
	}
}

func TestLoadMissingFile(t *testing.T) {
	root := t.TempDir()
	if _, err := Load(root, "proj", "nonexistent"); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	tests := []struct{ project, reviewID string }{
		{"../etc", "id"},
		{"proj", "../etc"},
		{`..\windows`, "id"},
		{"", "id"},
	}
	for _, tc := range tests {
		_, err := Load(root, tc.project, tc.reviewID)
		if err == nil {
			t.Errorf("Load(%q, %q): expected error", tc.project, tc.reviewID)
		}
	}
}

func TestListProjects(t *testing.T) {
	root := t.TempDir()

	// Create two projects
	r1 := Result{Project: ProjectInfo{Name: "alpha"}}
	r2 := Result{Project: ProjectInfo{Name: "beta"}}
	if _, _, err := Save(root, r1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Save(root, r2); err != nil {
		t.Fatal(err)
	}

	projects, err := ListProjects(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("len(projects) = %d, want 2", len(projects))
	}
	names := map[string]int{}
	for _, p := range projects {
		names[p.Project.Name]++
	}
	if names["alpha"] != 1 || names["beta"] != 1 {
		t.Errorf("unexpected project names: %v", names)
	}
}

func TestListProjectsExcludesEmptyDirs(t *testing.T) {
	root := t.TempDir()
	// Create an empty directory (no JSON files)
	emptyDir := filepath.Join(root, "empty-project")
	if err := os.MkdirAll(emptyDir, 0700); err != nil {
		t.Fatal(err)
	}

	// Also create a real project
	result := Result{Project: ProjectInfo{Name: "real"}}
	if _, _, err := Save(root, result); err != nil {
		t.Fatal(err)
	}

	projects, err := ListProjects(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Errorf("len(projects) = %d, want 1 (empty dir skipped)", len(projects))
	}
}

func TestListReviewsFilterByBranch(t *testing.T) {
	root := t.TempDir()

	r1 := Result{
		Project: ProjectInfo{Name: "proj"},
		Review:  ReviewInfo{SourceBranch: "feature/a", TargetBranch: "main"},
	}
	r2 := Result{
		Project: ProjectInfo{Name: "proj"},
		Review:  ReviewInfo{SourceBranch: "feature/b", TargetBranch: "main"},
	}
	if _, _, err := Save(root, r1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Save(root, r2); err != nil {
		t.Fatal(err)
	}

	key := ProjectKey(r1.Project)
	all, err := ListReviews(root, key, ReviewFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("unfiltered len = %d, want 2", len(all))
	}

	filtered, err := ListReviews(root, key, ReviewFilter{SourceBranch: "feature/a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 {
		t.Fatalf("filtered len = %d, want 1", len(filtered))
	}
	if filtered[0].Review.SourceBranch != "feature/a" {
		t.Errorf("SourceBranch = %q", filtered[0].Review.SourceBranch)
	}
}

func TestListReviewsRejectsUnsafeSegment(t *testing.T) {
	root := t.TempDir()
	if _, err := ListReviews(root, "../etc", ReviewFilter{}); err == nil {
		t.Fatal("expected error for unsafe segment")
	}
}

func TestListAllReviews(t *testing.T) {
	root := t.TempDir()

	r1 := Result{Project: ProjectInfo{Name: "proj-a"}}
	r2 := Result{Project: ProjectInfo{Name: "proj-b"}}
	if _, _, err := Save(root, r1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Save(root, r2); err != nil {
		t.Fatal(err)
	}

	all, err := ListAllReviews(root, ReviewFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("len = %d, want 2", len(all))
	}
}

func TestListAllReviewsFilterByProject(t *testing.T) {
	root := t.TempDir()

	r1 := Result{Project: ProjectInfo{Name: "alpha"}}
	r2 := Result{Project: ProjectInfo{Name: "beta"}}
	if _, _, err := Save(root, r1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Save(root, r2); err != nil {
		t.Fatal(err)
	}

	filtered, err := ListAllReviews(root, ReviewFilter{Project: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 {
		t.Fatalf("len = %d, want 1", len(filtered))
	}
}

func TestProjectKey(t *testing.T) {
	tests := []struct {
		name    string
		project ProjectInfo
		want    string
	}{
		{
			name:    "project name path",
			project: ProjectInfo{Name: "group/project"},
			want:    "group-project",
		},
		{
			name:    "repo dir path",
			project: ProjectInfo{RepoDir: "/Users/kite/Desktop/my-project"},
			want:    "Users-kite-Desktop-my-project",
		},
		{
			name:    "mixed separators",
			project: ProjectInfo{Name: `group\project/service`},
			want:    "group-project-service",
		},
		{
			name:    "dotted name",
			project: ProjectInfo{Name: "my..project"},
			want:    "my..project",
		},
		{
			name:    "fallback unknown",
			project: ProjectInfo{},
			want:    "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ProjectKey(tt.project); got != tt.want {
				t.Fatalf("ProjectKey() = %q, want %q", got, tt.want)
			}
			if !isSafePathSegment(ProjectKey(tt.project)) {
				t.Fatalf("ProjectKey() produced unsafe segment %q", ProjectKey(tt.project))
			}
		})
	}
}

func TestResultJSONRoundTrip(t *testing.T) {
	result := Result{
		ID:        "test-id",
		CreatedAt: time.Date(2025, 6, 10, 8, 0, 0, 0, time.UTC),
		Project:   ProjectInfo{ID: "123", Name: "my/project", WebURL: "https://gitlab.com/my/project"},
		GitLab:    GitLabInfo{MergeRequestIID: "42", PipelineID: "100"},
		Review: ReviewInfo{
			Mode: "commit", Commit: "abc123", Model: "claude-3",
			FilesReviewed: 10, CommentCount: 5, TotalTokens: 50000,
		},
		Comments: []model.LlmComment{
			{Path: "main.go", Content: "nil check", Severity: "high", Category: "bug"},
		},
		Warnings: []Warning{{File: "main.go", Message: "slow", Type: "subtask_error"}},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	var decoded Result
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ID != "test-id" {
		t.Errorf("ID = %q", decoded.ID)
	}
	if decoded.GitLab.MergeRequestIID != "42" {
		t.Errorf("MR IID = %q", decoded.GitLab.MergeRequestIID)
	}
	if len(decoded.Comments) != 1 || decoded.Comments[0].Content != "nil check" {
		t.Errorf("Comments mismatch")
	}
}

func TestSaveWritesMarkdown(t *testing.T) {
	root := t.TempDir()
	result := Result{
		Project: ProjectInfo{Name: "my-project", RepoDir: "/home/dev/repo"},
		Review: ReviewInfo{
			Mode:          "range",
			SourceBranch:  "feature/x",
			TargetBranch:  "main",
			From:          "main",
			To:            "feature/x",
			Model:         "claude-3",
			FilesReviewed: 3,
			CommentCount:  2,
			TotalTokens:   12345,
			InputTokens:   10000,
			OutputTokens:  2345,
			Duration:      "2m30s",
		},
		Comments: []model.LlmComment{
			{Path: "a.go", Content: "fix this bug", Severity: "high", Category: "bug", StartLine: 42, EndLine: 42, SuggestionCode: "// fixed"},
			{Path: "b.go", Content: "use const", Severity: "medium", Category: "maintainability", StartLine: 10, EndLine: 15},
		},
		Warnings: []Warning{{File: "x.go", Message: "slow", Type: "warning"}},
	}

	path, mdPath, err := Save(root, result)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty JSON path")
	}
	if mdPath == "" {
		t.Fatal("expected non-empty Markdown path")
	}

	mdData, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("read markdown: %v", err)
	}
	mdStr := string(mdData)

	// Verify key sections are present
	for _, want := range []string{
		"# Code Review Report",
		"**Project:** my-project",
		"**Mode:** range",
		"## Summary by Severity",
		"## \U0001F7E0 High (1)",
		"## \U0001F7E1 Medium (1)",
		"## \U0001F534 Critical (0)",
		"### `a.go`",
		"fix this bug",
		"### `b.go`",
		"use const",
		"## Warnings",
		"slow",
		"open-code-review",
	} {
		if !strings.Contains(mdStr, want) {
			t.Errorf("Markdown missing %q", want)
		}
	}

	// Verify no raw JSON artifacts leak
	for _, bad := range []string{`"id"`, `"created_at"`, `"project":`} {
		if strings.Contains(mdStr, bad) {
			t.Errorf("Markdown contains raw JSON field %q", bad)
		}
	}
}

func TestIsSafeFilePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"simple", "src/main.go", true},
		{"nested", "internal/agent/agent.go", true},
		{"empty", "", false},
		{"dot", ".", true},
		{"dotdot", "..", false},
		{"traversal", "../etc/passwd", false},
		{"absolute unix", "/etc/passwd", false},
		{"with dotdot inside", "a/../b", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSafeFilePath(tt.path); got != tt.want {
				t.Errorf("isSafeFilePath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestSavePerFile_Basic(t *testing.T) {
	root := t.TempDir()
	result := Result{
		Project: ProjectInfo{Name: "test-proj"},
		Review: ReviewInfo{
			Mode:          "full_scan",
			Model:         "test-model",
			FilesReviewed: 2,
			CommentCount:  2,
		},
		Comments: []model.LlmComment{
			{Path: "src/main.go", Content: "bug in main", Severity: "high", Category: "bug"},
			{Path: "pkg/util.go", Content: "style nit", Severity: "low", Category: "style"},
		},
	}
	indexPath, err := SavePerFile(root, result)
	if err != nil {
		t.Fatalf("SavePerFile: %v", err)
	}

	// Verify index.json exists and parses
	raw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index.json: %v", err)
	}
	var idx PerFileIndex
	if err := json.Unmarshal(raw, &idx); err != nil {
		t.Fatalf("parse index.json: %v", err)
	}
	if len(idx.Files) != 2 {
		t.Fatalf("expected 2 files in index, got %d", len(idx.Files))
	}

	// Verify each per-file .md exists
	for _, entry := range idx.Files {
		mdPath := filepath.Join(filepath.Dir(indexPath), filepath.FromSlash(entry.MD))
		if _, err := os.Stat(mdPath); err != nil {
			t.Errorf("per-file md missing: %s", mdPath)
		}
		md, err := os.ReadFile(mdPath)
		if err != nil {
			t.Errorf("read per-file md: %v", err)
			continue
		}
		if string(md) == "" {
			t.Errorf("per-file md empty: %s", mdPath)
		}
	}
}

func TestSavePerFile_EmptyComments(t *testing.T) {
	root := t.TempDir()
	result := Result{
		Project:  ProjectInfo{Name: "test-proj"},
		Review:   ReviewInfo{Mode: "full_scan"},
		Comments: nil,
	}
	indexPath, err := SavePerFile(root, result)
	if err != nil {
		t.Fatalf("SavePerFile: %v", err)
	}

	raw, _ := os.ReadFile(indexPath)
	var idx PerFileIndex
	json.Unmarshal(raw, &idx)
	if len(idx.Files) != 0 {
		t.Errorf("expected 0 files for empty comments, got %d", len(idx.Files))
	}
}

func TestPerFileWriter_Basic(t *testing.T) {
	root := t.TempDir()
	project := ProjectInfo{Name: "test-proj", RepoDir: "/repo"}
	reviewID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

	w, err := NewPerFileWriter(root, project, reviewID)
	if err != nil {
		t.Fatalf("NewPerFileWriter: %v", err)
	}
	if w.ReviewID != reviewID {
		t.Errorf("ReviewID = %q, want %q", w.ReviewID, reviewID)
	}

	// Write 2 files
	if err := w.WriteFile("src/main.go", []model.LlmComment{
		{Path: "src/main.go", Content: "nil check missing", Severity: "high", Category: "bug"},
	}); err != nil {
		t.Fatalf("WriteFile main.go: %v", err)
	}
	if err := w.WriteFile("pkg/util.go", []model.LlmComment{
		{Path: "pkg/util.go", Content: "use const", Severity: "medium", Category: "maintainability"},
	}); err != nil {
		t.Fatalf("WriteFile util.go: %v", err)
	}

	// Verify entries
	entries := w.Entries()
	if len(entries) != 2 {
		t.Fatalf("Entries len = %d, want 2", len(entries))
	}

	// Finalize
	indexPath, err := w.Finalize(ReviewInfo{Mode: "full_scan", Model: "test"}, GitLabInfo{}, nil)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	// Verify index.json
	raw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index.json: %v", err)
	}
	var idx PerFileIndex
	if err := json.Unmarshal(raw, &idx); err != nil {
		t.Fatalf("parse index.json: %v", err)
	}
	if len(idx.Files) != 2 {
		t.Fatalf("idx.Files len = %d, want 2", len(idx.Files))
	}
	// Files must be sorted by path
	if idx.Files[0].Path != "pkg/util.go" || idx.Files[1].Path != "src/main.go" {
		t.Errorf("Files not sorted: %v", idx.Files)
	}

	// Verify per-file .md existence
	for _, entry := range idx.Files {
		mdPath := filepath.Join(filepath.Dir(indexPath), filepath.FromSlash(entry.MD))
		mdData, err := os.ReadFile(mdPath)
		if err != nil {
			t.Errorf("read per-file md %s: %v", mdPath, err)
			continue
		}
		if len(mdData) == 0 {
			t.Errorf("per-file md %s is empty", mdPath)
		}
	}
}

func TestPerFileWriter_Concurrent(t *testing.T) {
	root := t.TempDir()
	project := ProjectInfo{Name: "concurrent-proj"}
	reviewID := "11111111-2222-3333-4444-555555555555"

	w, err := NewPerFileWriter(root, project, reviewID)
	if err != nil {
		t.Fatalf("NewPerFileWriter: %v", err)
	}

	const goroutines = 10
	const filesPerRoutine = 5

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for f := 0; f < filesPerRoutine; f++ {
				filePath := "src/file" + string(rune('0'+id)) + "_" + string(rune('0'+f)) + ".go"
				err := w.WriteFile(filePath, []model.LlmComment{
					{Path: filePath, Content: "issue", Severity: "low", Category: "style"},
				})
				if err != nil {
					t.Errorf("WriteFile %s: %v", filePath, err)
				}
			}
		}(g)
	}
	wg.Wait()

	entries := w.Entries()
	if len(entries) != goroutines*filesPerRoutine {
		t.Errorf("Entries len = %d, want %d", len(entries), goroutines*filesPerRoutine)
	}

	indexPath, err := w.Finalize(ReviewInfo{Mode: "full_scan"}, GitLabInfo{}, nil)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	raw, _ := os.ReadFile(indexPath)
	var idx PerFileIndex
	json.Unmarshal(raw, &idx)
	if len(idx.Files) != goroutines*filesPerRoutine {
		t.Errorf("idx.Files len = %d, want %d", len(idx.Files), goroutines*filesPerRoutine)
	}
}

func TestPerFileWriter_EmptyComments(t *testing.T) {
	root := t.TempDir()
	project := ProjectInfo{Name: "empty-proj"}
	reviewID := "22222222-3333-4444-5555-666666666666"

	w, err := NewPerFileWriter(root, project, reviewID)
	if err != nil {
		t.Fatalf("NewPerFileWriter: %v", err)
	}

	// Write file with empty comments — should be silently skipped.
	if err := w.WriteFile("src/clean.go", nil); err != nil {
		t.Fatalf("WriteFile with nil comments: %v", err)
	}
	// Also test with empty slice.
	if err := w.WriteFile("src/clean2.go", []model.LlmComment{}); err != nil {
		t.Fatalf("WriteFile with empty slice: %v", err)
	}

	entries := w.Entries()
	if len(entries) != 0 {
		t.Fatalf("Entries len = %d, want 0 (empty comments skipped)", len(entries))
	}

	// Write a file with comments to verify Finalize still works.
	if err := w.WriteFile("src/bug.go", []model.LlmComment{{Path: "src/bug.go", Content: "bug", Severity: "high"}}); err != nil {
		t.Fatalf("WriteFile with comment: %v", err)
	}

	indexPath, err := w.Finalize(ReviewInfo{Mode: "full_scan"}, GitLabInfo{}, nil)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	raw, _ := os.ReadFile(indexPath)
	var idx PerFileIndex
	json.Unmarshal(raw, &idx)
	if len(idx.Files) != 1 {
		t.Errorf("idx.Files len = %d, want 1", len(idx.Files))
	}
}

func TestExistingPerFilePaths(t *testing.T) {
	root := t.TempDir()
	project := ProjectInfo{Name: "existing-proj"}
	reviewID := "33333333-4444-5555-6666-777777777777"

	w, err := NewPerFileWriter(root, project, reviewID)
	if err != nil {
		t.Fatalf("NewPerFileWriter: %v", err)
	}

	// Write 3 files (one with empty comments is silently skipped)
	w.WriteFile("src/a.go", []model.LlmComment{{Path: "src/a.go", Content: "bug", Severity: "high"}})
	w.WriteFile("src/b.go", []model.LlmComment{{Path: "src/b.go", Content: "style", Severity: "low"}})
	w.WriteFile("pkg/c.go", nil)
	w.Finalize(ReviewInfo{Mode: "full_scan"}, GitLabInfo{}, nil)

	projectKey := ProjectKey(project)
	paths, err := ExistingPerFilePaths(root, projectKey, reviewID)
	if err != nil {
		t.Fatalf("ExistingPerFilePaths: %v", err)
	}

	if len(paths) != 2 {
		t.Fatalf("paths len = %d, want 2", len(paths))
	}
	if !paths["src/a.go"] || !paths["src/b.go"] {
		t.Errorf("missing expected paths: %v", paths)
	}
}

func TestExistingPerFilePaths_NonExistent(t *testing.T) {
	root := t.TempDir()
	paths, err := ExistingPerFilePaths(root, "noproj", "noreview")
	if err != nil {
		t.Fatalf("ExistingPerFilePaths: %v", err)
	}
	if paths != nil {
		t.Errorf("expected nil for non-existent dir, got %v", paths)
	}
}

func TestPerFileWriter_RejectsEmptyReviewID(t *testing.T) {
	root := t.TempDir()
	_, err := NewPerFileWriter(root, ProjectInfo{Name: "test"}, "")
	if err == nil {
		t.Fatal("expected error for empty review ID")
	}
}

func TestPerFileWriter_RejectsUnsafeFilePath(t *testing.T) {
	root := t.TempDir()
	w, _ := NewPerFileWriter(root, ProjectInfo{Name: "test"}, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	if err := w.WriteFile("../etc/passwd", nil); err == nil {
		t.Fatal("expected error for unsafe file path")
	}
}

func TestSavePerFile_MarkdownContent(t *testing.T) {
	root := t.TempDir()
	result := Result{
		Project: ProjectInfo{Name: "md-proj"},
		Review: ReviewInfo{
			Mode:  "full_scan",
			Model: "md-model",
		},
		Comments: []model.LlmComment{
			{Path: "src/app.go", Content: "critical bug", Severity: "critical", Category: "bug",
				SuggestionCode: "// fix", ExistingCode: "// old"},
		},
	}
	indexPath, err := SavePerFile(root, result)
	if err != nil {
		t.Fatalf("SavePerFile: %v", err)
	}

	mdPath := filepath.Join(filepath.Dir(indexPath), "src", "app.go.md")
	md, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("read %s: %v", mdPath, err)
	}
	mdStr := string(md)

	for _, want := range []string{
		"# Review: `src/app.go`",
		"**Project:** md-proj",
		"critical bug",
		"// fix",
		"// old",
	} {
		if !strings.Contains(mdStr, want) {
			t.Errorf("per-file md missing %q", want)
		}
	}
}

func TestMergePerFileIndex_PreservesCreatedAt(t *testing.T) {
	createdAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	existing := &PerFileIndex{
		ID:        "old-id",
		CreatedAt: createdAt,
		Project:   ProjectInfo{Name: "proj"},
		Review: ReviewInfo{
			Mode:            "full_scan",
			FilesReviewed:   3,
			CommentCount:    5,
			TotalTokens:     10000,
			InputTokens:     8000,
			OutputTokens:    2000,
			CacheReadTokens: 500,
		},
		Files: []PerFileEntry{
			{Path: "src/a.go", CommentCount: 2, MD: "src/a.go.md"},
			{Path: "src/b.go", CommentCount: 3, MD: "src/b.go.md"},
		},
		Warnings: []Warning{{File: "src/a.go", Message: "slow file", Type: "subtask_warning"}},
	}

	// New (resume) run: file a.go was re-reviewed, file b.go was reused,
	// and file c.go is new.
	newIdx := &PerFileIndex{
		ID:        "new-id",
		CreatedAt: time.Now().UTC(),
		Project:   ProjectInfo{Name: "proj"},
		Review: ReviewInfo{
			Mode:            "full_scan",
			FilesReviewed:   3,
			CommentCount:    5,
			TotalTokens:     3000,
			InputTokens:     2500,
			OutputTokens:    500,
			CacheReadTokens: 200,
		},
		Files: []PerFileEntry{
			{Path: "src/a.go", CommentCount: 4, MD: "src/a.go.md"},
			{Path: "src/c.go", CommentCount: 1, MD: "src/c.go.md"},
		},
		Warnings: []Warning{{File: "src/c.go", Message: "new warning", Type: "subtask_warning"}},
	}

	merged := mergePerFileIndex(existing, newIdx)

	// Preserved created_at from existing (original run).
	if !merged.CreatedAt.Equal(createdAt) {
		t.Errorf("CreatedAt = %v, want %v", merged.CreatedAt, createdAt)
	}

	// Files: a.go updated (re-reviewed), b.go kept (reused), c.go added (new).
	if len(merged.Files) != 3 {
		t.Fatalf("Files len = %d, want 3", len(merged.Files))
	}
	fileMap := make(map[string]PerFileEntry)
	for _, f := range merged.Files {
		fileMap[f.Path] = f
	}
	if fileMap["src/a.go"].CommentCount != 4 {
		t.Errorf("a.go CommentCount = %d, want 4 (re-reviewed)", fileMap["src/a.go"].CommentCount)
	}
	if fileMap["src/b.go"].CommentCount != 3 {
		t.Errorf("b.go CommentCount = %d, want 3 (reused)", fileMap["src/b.go"].CommentCount)
	}
	if _, ok := fileMap["src/c.go"]; !ok {
		t.Error("c.go missing (should be added as new)")
	}

	// Token accumulation.
	if merged.Review.TotalTokens != 13000 {
		t.Errorf("TotalTokens = %d, want 13000", merged.Review.TotalTokens)
	}
	if merged.Review.InputTokens != 10500 {
		t.Errorf("InputTokens = %d, want 10500", merged.Review.InputTokens)
	}
	if merged.Review.CacheReadTokens != 700 {
		t.Errorf("CacheReadTokens = %d, want 700", merged.Review.CacheReadTokens)
	}

	// Warnings: deduplicated across runs (existing + new).
	warnCount := len(merged.Warnings)
	if warnCount != 2 {
		t.Errorf("Warnings len = %d, want 2 (one from each run)", warnCount)
	}
}

func TestMergePerFileIndex_WarnDedup(t *testing.T) {
	existing := &PerFileIndex{
		ID:      "old",
		Project: ProjectInfo{Name: "proj"},
		Review:  ReviewInfo{Mode: "full_scan"},
		Warnings: []Warning{
			{File: "f.go", Message: "dup msg", Type: "warn"},
			{File: "f.go", Message: "unique old", Type: "warn"},
		},
	}
	newIdx := &PerFileIndex{
		ID:      "new",
		Project: ProjectInfo{Name: "proj"},
		Review:  ReviewInfo{Mode: "full_scan"},
		Warnings: []Warning{
			{File: "f.go", Message: "dup msg", Type: "warn"},
			{File: "f.go", Message: "unique new", Type: "warn"},
		},
	}

	merged := mergePerFileIndex(existing, newIdx)
	if len(merged.Warnings) != 3 {
		t.Errorf("Warnings len = %d, want 3 (deduplicated)", len(merged.Warnings))
	}
}

func TestPerFileWriter_FinalizeMerge(t *testing.T) {
	root := t.TempDir()
	project := ProjectInfo{Name: "merge-proj"}

	// Simulate run 1: write an index.json directly (as if Finalize was called).
	baseDir := filepath.Join(root, ProjectKey(project), "merge-id")
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		t.Fatal(err)
	}
	existing := PerFileIndex{
		ID:        "merge-id",
		CreatedAt: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
		Project:   project,
		Review: ReviewInfo{
			Mode:          "full_scan",
			FilesReviewed: 2,
			CommentCount:  3,
			TotalTokens:   5000,
			InputTokens:   4000,
			OutputTokens:  1000,
		},
		Files: []PerFileEntry{
			{Path: "src/x.go", CommentCount: 2, MD: "src/x.go.md"},
			{Path: "src/y.go", CommentCount: 1, MD: "src/y.go.md"},
		},
	}
	if err := writePerFileJSON(filepath.Join(baseDir, "index.json"), existing); err != nil {
		t.Fatal(err)
	}

	// Run 2 (resume): create a PerFileWriter in the same directory.
	pw := &PerFileWriter{
		BaseDir:  baseDir,
		ReviewID: "merge-id",
		Project:  project,
		entries: []PerFileEntry{
			{Path: "src/x.go", CommentCount: 3, MD: "src/x.go.md"},
			{Path: "src/z.go", CommentCount: 2, MD: "src/z.go.md"},
		},
	}

	// Write per-file md for new entries.
	for _, e := range pw.entries {
		mdPath := filepath.Join(baseDir, filepath.FromSlash(e.MD))
		if err := os.MkdirAll(filepath.Dir(mdPath), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(mdPath, []byte("dummy"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	indexPath, err := pw.Finalize(ReviewInfo{
		Mode:          "full_scan",
		FilesReviewed: 2,
		CommentCount:  3,
		TotalTokens:   2000,
		InputTokens:   1500,
		OutputTokens:  500,
	}, GitLabInfo{}, nil)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	raw, _ := os.ReadFile(indexPath)
	var merged PerFileIndex
	if err := json.Unmarshal(raw, &merged); err != nil {
		t.Fatalf("parse merged index.json: %v", err)
	}

	// Preserved created_at.
	if !merged.CreatedAt.Equal(existing.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", merged.CreatedAt, existing.CreatedAt)
	}

	// Files: x.go (re-reviewed), y.go (reused), z.go (new).
	if len(merged.Files) != 3 {
		t.Fatalf("Files len = %d, want 3", len(merged.Files))
	}
	fileMap := make(map[string]PerFileEntry)
	for _, f := range merged.Files {
		fileMap[f.Path] = f
	}
	if fileMap["src/x.go"].CommentCount != 3 {
		t.Errorf("x.go CommentCount = %d, want 3", fileMap["src/x.go"].CommentCount)
	}
	if _, ok := fileMap["src/y.go"]; !ok {
		t.Error("y.go missing (should be kept from run 1)")
	}
	if _, ok := fileMap["src/z.go"]; !ok {
		t.Error("z.go missing (should be added from run 2)")
	}

	// Tokens accumulated.
	if merged.Review.TotalTokens != 7000 {
		t.Errorf("TotalTokens = %d, want 7000", merged.Review.TotalTokens)
	}
}

func TestPerFileWriter_FinalizeNoMerge(t *testing.T) {
	root := t.TempDir()
	project := ProjectInfo{Name: "fresh-proj"}

	pw, err := NewPerFileWriter(root, project, "fresh-id")
	if err != nil {
		t.Fatalf("NewPerFileWriter: %v", err)
	}
	pw.WriteFile("src/f.go", []model.LlmComment{{Path: "src/f.go", Content: "bug", Severity: "high"}})

	before := time.Now().UTC()
	_, err = pw.Finalize(ReviewInfo{Mode: "full_scan", FilesReviewed: 1, CommentCount: 1}, GitLabInfo{}, nil)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	raw, _ := os.ReadFile(filepath.Join(pw.BaseDir, "index.json"))
	var idx PerFileIndex
	json.Unmarshal(raw, &idx)

	// No existing index — created_at should be recent.
	if idx.CreatedAt.Before(before) {
		t.Errorf("CreatedAt = %v, should be >= %v", idx.CreatedAt, before)
	}
	if len(idx.Files) != 1 {
		t.Errorf("Files len = %d, want 1", len(idx.Files))
	}
}
