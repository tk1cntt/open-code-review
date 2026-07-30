package viewer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/reviewstore"
)

func TestHandleAPIReviews_EmptyList(t *testing.T) {
	root := t.TempDir()

	req := httptest.NewRequest("GET", "/api/reviews", nil)
	w := httptest.NewRecorder()
	handleAPIReviews(w, req, root)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	var reviews []reviewstore.ReviewSummary
	if err := json.NewDecoder(w.Body).Decode(&reviews); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(reviews) != 0 {
		t.Errorf("len = %d, want 0", len(reviews))
	}
}

func TestHandleAPIReviews_WithResults(t *testing.T) {
	root := t.TempDir()

	result := reviewstore.Result{
		Project: reviewstore.ProjectInfo{Name: "test-project"},
		Review:  reviewstore.ReviewInfo{Mode: "commit", Commit: "abc123", Model: "claude-3"},
		Comments: []model.LlmComment{
			{Path: "main.go", Content: "fix bug", Severity: "high", Category: "bug"},
		},
	}
	if _, _, err := reviewstore.Save(root, result); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/reviews", nil)
	w := httptest.NewRecorder()
	handleAPIReviews(w, req, root)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var reviews []reviewstore.ReviewSummary
	if err := json.NewDecoder(w.Body).Decode(&reviews); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(reviews) < 1 {
		t.Fatalf("len = %d, want >= 1", len(reviews))
	}
	if reviews[0].Review.Mode != "commit" {
		t.Errorf("mode = %q", reviews[0].Review.Mode)
	}
}

func TestHandleAPIReviews_FilterByProject(t *testing.T) {
	root := t.TempDir()

	r1 := reviewstore.Result{
		Project: reviewstore.ProjectInfo{Name: "alpha"},
		Review:  reviewstore.ReviewInfo{Mode: "workspace"},
	}
	r2 := reviewstore.Result{
		Project: reviewstore.ProjectInfo{Name: "beta"},
		Review:  reviewstore.ReviewInfo{Mode: "workspace"},
	}
	if _, _, err := reviewstore.Save(root, r1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := reviewstore.Save(root, r2); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/reviews?project=alpha", nil)
	w := httptest.NewRecorder()
	handleAPIReviews(w, req, root)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var reviews []reviewstore.ReviewSummary
	if err := json.NewDecoder(w.Body).Decode(&reviews); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("len = %d, want 1", len(reviews))
	}
}

func TestHandleAPIReviewDetail(t *testing.T) {
	root := t.TempDir()

	result := reviewstore.Result{
		Project: reviewstore.ProjectInfo{Name: "my-project"},
		Review:  reviewstore.ReviewInfo{Mode: "commit", Commit: "abc123"},
		Comments: []model.LlmComment{
			{Path: "main.go", Content: "fix bug", Severity: "high", Category: "bug"},
		},
	}
	path, _, err := reviewstore.Save(root, result)
	if err != nil {
		t.Fatal(err)
	}

	savedDir := filepath.Dir(path)
	projectKey := filepath.Base(savedDir)
	savedFile := filepath.Base(path)
	reviewID := savedFile[:len(savedFile)-len(".json")]

	req := httptest.NewRequest("GET", "/api/reviews/"+projectKey+"/"+reviewID, nil)
	w := httptest.NewRecorder()
	handleAPIReviewDetail(w, req, root, projectKey, reviewID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got reviewstore.Result
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != reviewID {
		t.Errorf("ID = %q, want %q", got.ID, reviewID)
	}
	if len(got.Comments) != 1 {
		t.Errorf("len(Comments) = %d, want 1", len(got.Comments))
	}
}

func TestHandleAPIReviewDetail_NotFound(t *testing.T) {
	root := t.TempDir()

	req := httptest.NewRequest("GET", "/api/reviews/proj/nonexistent", nil)
	w := httptest.NewRecorder()
	handleAPIReviewDetail(w, req, root, "proj", "nonexistent")

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestHandleAPIReviewDetail_RejectsTraversal(t *testing.T) {
	root := t.TempDir()

	req := httptest.NewRequest("GET", "/api/reviews/../etc/passwd", nil)
	w := httptest.NewRecorder()
	handleAPIReviewDetail(w, req, root, "../etc", "passwd")

	// Traversal shouldn't reach the handler in real usage (mux rejects it),
	// but the handler should return 404 for invalid segments that bypass mux.
	if w.Code < 400 {
		t.Errorf("status = %d, want >= 400", w.Code)
	}
}

func TestServerOptions_DefaultReviewsDir(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	// Verify StartServerWithOptions works without a reviews dir (default).
	errCh := make(chan error, 1)
	go func() {
		errCh <- StartServerWithOptions(ServerOptions{Addr: "localhost:0"})
	}()
	// Don't wait — just ensure no panic during setup
}

func TestReviewHandlerEndpointRegistered(t *testing.T) {
	root := t.TempDir()

	// Save a result to make API return non-empty
	result := reviewstore.Result{
		Project: reviewstore.ProjectInfo{Name: "endpoint-test"},
		Review:  reviewstore.ReviewInfo{Mode: "workspace"},
	}
	if _, _, err := reviewstore.Save(root, result); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/reviews", nil)
	w := httptest.NewRecorder()
	handleAPIReviews(w, req, root)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}

	var reviews []reviewstore.ReviewSummary
	if err := json.NewDecoder(w.Body).Decode(&reviews); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(reviews) < 1 {
		t.Fatal("expected at least 1 review result")
	}
}

func TestHandleAPIReviews_FilterByBranches(t *testing.T) {
	root := t.TempDir()

	r1 := reviewstore.Result{
		Project: reviewstore.ProjectInfo{Name: "proj"},
		Review:  reviewstore.ReviewInfo{SourceBranch: "feature/a", TargetBranch: "main"},
	}
	r2 := reviewstore.Result{
		Project: reviewstore.ProjectInfo{Name: "proj"},
		Review:  reviewstore.ReviewInfo{SourceBranch: "feature/b", TargetBranch: "develop"},
	}
	if _, _, err := reviewstore.Save(root, r1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := reviewstore.Save(root, r2); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/reviews?source=feature/a&target=main", nil)
	w := httptest.NewRecorder()
	handleAPIReviews(w, req, root)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var reviews []reviewstore.ReviewSummary
	if err := json.NewDecoder(w.Body).Decode(&reviews); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("len = %d, want 1", len(reviews))
	}
	if reviews[0].Review.SourceBranch != "feature/a" {
		t.Errorf("SourceBranch = %q", reviews[0].Review.SourceBranch)
	}
}

func TestHandleAPIProjectReviews(t *testing.T) {
	root := t.TempDir()

	result := reviewstore.Result{
		Project: reviewstore.ProjectInfo{Name: "my-proj"},
		Review:  reviewstore.ReviewInfo{Mode: "range", SourceBranch: "feat/x", TargetBranch: "main"},
	}
	path, _, err := reviewstore.Save(root, result)
	if err != nil {
		t.Fatal(err)
	}
	projectKey := filepath.Base(filepath.Dir(path))

	req := httptest.NewRequest("GET", "/api/reviews/"+projectKey, nil)
	w := httptest.NewRecorder()
	handleAPIProjectReviews(w, req, root, projectKey)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var reviews []reviewstore.ReviewSummary
	if err := json.NewDecoder(w.Body).Decode(&reviews); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("len = %d, want 1", len(reviews))
	}
	if reviews[0].Review.SourceBranch != "feat/x" {
		t.Errorf("SourceBranch = %q", reviews[0].Review.SourceBranch)
	}
}

func TestHandleAPIProjectReviews_InvalidProject(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/reviews/../etc", nil)
	w := httptest.NewRecorder()
	handleAPIProjectReviews(w, req, os.TempDir(), "../etc")

	if w.Code == http.StatusOK {
		t.Errorf("expected non-200 for invalid project")
	}
}
