package viewer

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"

	"github.com/alibaba/open-code-review/internal/reviewstore"
)

func handleAPIReviews(w http.ResponseWriter, r *http.Request, root string) {
	q := r.URL.Query()

	filter := reviewstore.ReviewFilter{
		Project:      q.Get("project"),
		SourceBranch: q.Get("source"),
		TargetBranch: q.Get("target"),
	}

	reviews, err := reviewstore.ListAllReviews(root, filter)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if reviews == nil {
		reviews = []reviewstore.ReviewSummary{}
	}
	writeJSON(w, http.StatusOK, reviews)
}

func handleAPIProjectReviews(w http.ResponseWriter, r *http.Request, root, project string) {
	q := r.URL.Query()

	filter := reviewstore.ReviewFilter{
		SourceBranch: q.Get("source"),
		TargetBranch: q.Get("target"),
	}

	reviews, err := reviewstore.ListReviews(root, project, filter)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if reviews == nil {
		reviews = []reviewstore.ReviewSummary{}
	}
	writeJSON(w, http.StatusOK, reviews)
}

func handleAPIReviewDetail(w http.ResponseWriter, r *http.Request, root, project, reviewID string) {
	result, err := reviewstore.Load(root, project, reviewID)
	if err != nil {
		if isNotFoundError(err) {
			writeJSONError(w, http.StatusNotFound, err)
		} else {
			writeJSONError(w, http.StatusInternalServerError, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("[ocr] json encode error: %v", err)
	}
}

func writeJSONError(w http.ResponseWriter, status int, err error) {
	log.Printf("[ocr] API error (status %d): %v", status, err)
	writeJSON(w, status, map[string]string{"error": http.StatusText(status)})
}

func isNotFoundError(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}
