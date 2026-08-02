// Package reviewstore persists final review results as JSON files under a
// configurable root directory. It provides CRUD operations for downstream
// consumers (CI pipelines, dashboards, WebUI APIs).
package reviewstore

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alibaba/open-code-review/internal/model"
)

// ProjectInfo identifies the project under review.
type ProjectInfo struct {
	ID      string `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	RepoDir string `json:"repo_dir,omitempty"`
	WebURL  string `json:"web_url,omitempty"`
}

// GitLabInfo carries GitLab CI/CD metadata attached to the review run.
type GitLabInfo struct {
	ServerURL       string `json:"server_url,omitempty"`
	ProjectID       string `json:"project_id,omitempty"`
	MergeRequestIID string `json:"merge_request_iid,omitempty"`
	PipelineID      string `json:"pipeline_id,omitempty"`
	JobID           string `json:"job_id,omitempty"`
}

// ReviewInfo holds the runtime parameters and aggregated telemetry for a
// single review run.
type ReviewInfo struct {
	Mode             string `json:"mode,omitempty"`
	SourceBranch     string `json:"source_branch,omitempty"`
	TargetBranch     string `json:"target_branch,omitempty"`
	From             string `json:"from,omitempty"`
	To               string `json:"to,omitempty"`
	Commit           string `json:"commit,omitempty"`
	Model            string `json:"model,omitempty"`
	FilesReviewed    int64  `json:"files_reviewed"`
	CommentCount     int64  `json:"comments"`
	TotalTokens      int64  `json:"total_tokens"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int64  `json:"cache_write_tokens,omitempty"`
	Duration         string `json:"duration,omitempty"`
	DurationSeconds  int64  `json:"duration_seconds,omitempty"`
	SessionID        string `json:"session_id,omitempty"`
}

// Warning records a non-fatal issue encountered during review.
type Warning struct {
	File    string `json:"file,omitempty"`
	Message string `json:"message,omitempty"`
	Type    string `json:"type,omitempty"`
}

// Result is the complete, self-contained output of one review run.
type Result struct {
	ID        string             `json:"id"`
	CreatedAt time.Time          `json:"created_at"`
	Project   ProjectInfo        `json:"project"`
	GitLab    GitLabInfo         `json:"gitlab,omitempty"`
	Review    ReviewInfo         `json:"review"`
	Comments  []model.LlmComment `json:"comments"`
	Warnings  []Warning          `json:"warnings,omitempty"`
}

// PerFileEntry describes one file in the per-file output directory.
type PerFileEntry struct {
	Path         string `json:"path"`
	CommentCount int    `json:"comment_count"`
	MD           string `json:"md"`
}

// PerFileIndex is the index.json at the root of a per-file output directory.
type PerFileIndex struct {
	ID        string         `json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	Project   ProjectInfo    `json:"project"`
	GitLab    GitLabInfo     `json:"gitlab,omitempty"`
	Review    ReviewInfo     `json:"review"`
	Warnings  []Warning      `json:"warnings,omitempty"`
	Files     []PerFileEntry `json:"files"`
}

// ProjectSummary is a lightweight listing entry for one project.
type ProjectSummary struct {
	EncodedKey  string
	Project     ProjectInfo
	ReviewCount int
	LastReview  time.Time
}

// ReviewSummary is a lightweight listing entry for one review.
type ReviewSummary struct {
	ID        string
	CreatedAt time.Time
	Project   ProjectInfo
	GitLab    GitLabInfo
	Review    ReviewInfo
}

type reviewSummaryFile struct {
	ID        string      `json:"id"`
	CreatedAt time.Time   `json:"created_at"`
	Project   ProjectInfo `json:"project"`
	GitLab    GitLabInfo  `json:"gitlab,omitempty"`
	Review    ReviewInfo  `json:"review"`
}

// ReviewFilter narrows listing results.
type ReviewFilter struct {
	Project      string
	SourceBranch string
	TargetBranch string
}

// DefaultRoot returns the default directory for persisted review results.
func DefaultRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".opencodereview", "reviews"), nil
}

// Save persists a Result to disk as both JSON and Markdown. It fills in ID
// and CreatedAt when unset. Returns the JSON path, Markdown path, and error.
func Save(root string, result Result) (string, string, error) {
	if root == "" {
		var err error
		root, err = DefaultRoot()
		if err != nil {
			return "", "", err
		}
	}
	if result.ID == "" {
		id, err := generateID()
		if err != nil {
			return "", "", err
		}
		result.ID = id
	}
	if result.CreatedAt.IsZero() {
		result.CreatedAt = time.Now().UTC()
	}
	if result.Project.Name == "" {
		result.Project.Name = result.Project.RepoDir
	}
	projectKey := ProjectKey(result.Project)
	if !isSafePathSegment(projectKey) {
		return "", "", fmt.Errorf("unsafe project key: %q", projectKey)
	}
	dir := filepath.Join(root, projectKey)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", "", fmt.Errorf("create review result dir: %w", err)
	}

	if !isSafePathSegment(result.ID) {
		return "", "", fmt.Errorf("unsafe review ID: %q", result.ID)
	}
	path := filepath.Join(dir, result.ID+".json")
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("marshal review result: %w", err)
	}
	tmp, err := os.CreateTemp(dir, result.ID+".*")
	if err != nil {
		return "", "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", "", fmt.Errorf("write review result: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return "", "", fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return "", "", fmt.Errorf("rename review result: %w", err)
	}

	mdPath, err := WriteMarkdown(root, result)
	if err != nil {
		// JSON was saved successfully; log md failure but don't fail the save.
		log.Printf("[ocr] warning: failed to write markdown report: %v", err)
		mdPath = ""
	}
	return path, mdPath, nil
}

// Load reads a single Result from disk.
func Load(root, encodedProject, reviewID string) (*Result, error) {
	path, err := reviewResultPath(root, encodedProject, reviewID)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open review result: %w", err)
	}
	defer f.Close()

	var result Result
	dec := json.NewDecoder(io.LimitReader(f, 10<<20))
	if err := dec.Decode(&result); err != nil {
		return nil, fmt.Errorf("decode review result: %w", err)
	}
	return &result, nil
}

func loadSummary(root, encodedProject, reviewID string) (ReviewSummary, error) {
	path, err := reviewResultPath(root, encodedProject, reviewID)
	if err != nil {
		return ReviewSummary{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return ReviewSummary{}, fmt.Errorf("open review result: %w", err)
	}
	defer f.Close()

	var sf reviewSummaryFile
	if err := json.NewDecoder(io.LimitReader(f, 10<<20)).Decode(&sf); err != nil {
		return ReviewSummary{}, fmt.Errorf("decode review result: %w", err)
	}
	return ReviewSummary{
		ID:        sf.ID,
		CreatedAt: sf.CreatedAt,
		Project:   sf.Project,
		GitLab:    sf.GitLab,
		Review:    sf.Review,
	}, nil
}

// ListProjects returns all projects that have at least one review result.
func ListProjects(root string) ([]ProjectSummary, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read reviews dir: %w", err)
	}

	var projects []ProjectSummary
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		reviews, err := listReviewSummaries(root, entry.Name())
		if err != nil || len(reviews) == 0 {
			continue
		}
		summary := ProjectSummary{
			EncodedKey:  entry.Name(),
			Project:     reviews[0].Project,
			ReviewCount: len(reviews),
			LastReview:  reviews[0].CreatedAt,
		}
		projects = append(projects, summary)
	}

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].LastReview.After(projects[j].LastReview)
	})
	return projects, nil
}

// listReviewSummaries is like ListReviews but optimized for project listing:
// it reads directory entries and only decodes the most recent file for
// Project metadata, using os.FileInfo for counts and timestamps.
func listReviewSummaries(root, encodedProject string) ([]ReviewSummary, error) {
	if !isSafePathSegment(encodedProject) {
		return nil, fmt.Errorf("invalid review path")
	}
	dir := filepath.Join(root, encodedProject)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read project dir: %w", err)
	}

	var jsonFiles []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			jsonFiles = append(jsonFiles, e)
		}
	}
	if len(jsonFiles) == 0 {
		return nil, nil
	}

	// Sort by modification time descending for LastReview
	sort.Slice(jsonFiles, func(i, j int) bool {
		ii, _ := jsonFiles[i].Info()
		jj, _ := jsonFiles[j].Info()
		if ii == nil || jj == nil {
			return false
		}
		return ii.ModTime().After(jj.ModTime())
	})

	// Load the most recent readable file for Project metadata.
	var result ReviewSummary
	var lastMod os.FileInfo
	for _, entry := range jsonFiles {
		var err error
		result, err = loadSummary(root, encodedProject, strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			log.Printf("[ocr] warning: skip unreadable review result %s/%s: %v", encodedProject, entry.Name(), err)
			continue
		}
		lastMod, _ = entry.Info()
		break
	}
	if result.ID == "" {
		return nil, nil
	}

	lastTime := result.CreatedAt
	if lastMod != nil && result.CreatedAt.IsZero() {
		lastTime = lastMod.ModTime()
	}

	return []ReviewSummary{{
		ID:        result.ID,
		CreatedAt: lastTime,
		Project:   result.Project,
		GitLab:    result.GitLab,
		Review:    result.Review,
	}}, nil
}

// ListReviews returns review summaries for a project, optionally filtered.
func ListReviews(root, encodedProject string, filter ReviewFilter) ([]ReviewSummary, error) {
	if !isSafePathSegment(encodedProject) {
		return nil, fmt.Errorf("invalid review path")
	}
	dir := filepath.Join(root, encodedProject)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read project review dir: %w", err)
	}

	var reviews []ReviewSummary
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		review, err := loadSummary(root, encodedProject, strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			log.Printf("[ocr] warning: skip review result %s/%s: %v", encodedProject, entry.Name(), err)
			continue
		}
		if filter.SourceBranch != "" && review.Review.SourceBranch != filter.SourceBranch {
			continue
		}
		if filter.TargetBranch != "" && review.Review.TargetBranch != filter.TargetBranch {
			continue
		}
		reviews = append(reviews, review)
	}

	sort.Slice(reviews, func(i, j int) bool {
		return reviews[i].CreatedAt.After(reviews[j].CreatedAt)
	})
	return reviews, nil
}

// ListAllReviews returns review summaries across all projects.
func ListAllReviews(root string, filter ReviewFilter) ([]ReviewSummary, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read reviews dir: %w", err)
	}

	var reviews []ReviewSummary
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projectReviews, err := ListReviews(root, entry.Name(), ReviewFilter{
			SourceBranch: filter.SourceBranch,
			TargetBranch: filter.TargetBranch,
		})
		if err != nil {
			return nil, err
		}
		for _, review := range projectReviews {
			if filter.Project != "" && !matchesProjectFilter(entry.Name(), review.Project, filter.Project) {
				continue
			}
			reviews = append(reviews, review)
		}
	}

	sort.Slice(reviews, func(i, j int) bool {
		return reviews[i].CreatedAt.After(reviews[j].CreatedAt)
	})
	return reviews, nil
}

// ProjectKey encodes project identity into a safe filesystem directory name.
func ProjectKey(project ProjectInfo) string {
	key := project.ID
	if key == "" {
		key = project.Name
	}
	if key == "" {
		key = project.RepoDir
	}
	if key == "" {
		key = "unknown"
	}
	return encodeProjectKey(key)
}

func encodeProjectKey(key string) string {
	if key == "" {
		return "empty"
	}
	vol := filepath.VolumeName(key)
	key = key[len(vol):]
	key = strings.TrimLeft(key, "/\\")
	key = strings.ReplaceAll(key, "/", "-")
	key = strings.ReplaceAll(key, "\\", "-")
	vol = strings.ReplaceAll(vol, ":", "_")
	result := vol + key
	if result == "" {
		return "empty"
	}
	return result
}

func reviewResultPath(root, encodedProject, reviewID string) (string, error) {
	if !isSafePathSegment(encodedProject) || !isSafePathSegment(reviewID) {
		return "", fmt.Errorf("invalid review path")
	}
	return filepath.Join(root, encodedProject, reviewID+".json"), nil
}

// IsSafePathSegment reports whether s is safe to use as a filesystem path
// component. It rejects empty strings, relative traversal ("..", "."),
// separators, colons (Windows drive-relative paths), null bytes, absolute
// paths, and values that would escape their directory.
func IsSafePathSegment(segment string) bool {
	if segment == "" || segment == "." || segment == ".." {
		return false
	}
	if strings.ContainsRune(segment, 0) {
		return false
	}
	if strings.ContainsAny(segment, "/\\:") {
		return false
	}
	return !filepath.IsAbs(segment) && filepath.Base(segment) == segment
}

// isSafePathSegment delegates to IsSafePathSegment for internal use.
func isSafePathSegment(segment string) bool {
	return IsSafePathSegment(segment)
}

func matchesProjectFilter(encodedProject string, project ProjectInfo, filter string) bool {
	return filter == encodedProject || filter == project.ID || filter == project.Name || filter == project.RepoDir
}

// SavePerFile creates a per-file output directory under <root>/<project-key>/<uuid>/
// with individual .md files per reviewed path and an index.json at the root.
// Returns the path to index.json and any error.
func SavePerFile(root string, result Result) (string, error) {
	if root == "" {
		var err error
		root, err = DefaultRoot()
		if err != nil {
			return "", err
		}
	}
	if result.ID == "" {
		id, err := generateID()
		if err != nil {
			return "", err
		}
		result.ID = id
	}
	if result.CreatedAt.IsZero() {
		result.CreatedAt = time.Now().UTC()
	}
	projectKey := ProjectKey(result.Project)
	if !isSafePathSegment(projectKey) {
		return "", fmt.Errorf("unsafe project key: %q", projectKey)
	}
	if !isSafePathSegment(result.ID) {
		return "", fmt.Errorf("unsafe review ID: %q", result.ID)
	}

	baseDir := filepath.Join(root, projectKey, result.ID)
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return "", fmt.Errorf("create per-file output dir: %w", err)
	}

	byFile := groupByFile(result.Comments)
	filePaths := sortedKeys(byFile)

	entries := make([]PerFileEntry, 0, len(filePaths))
	for _, filePath := range filePaths {
		if !isSafeFilePath(filePath) {
			return "", fmt.Errorf("unsafe file path %q in comments", filePath)
		}

		comments := byFile[filePath]
		mdRelPath := filePath + ".md"

		if err := writePerFileMarkdown(baseDir, mdRelPath, result, filePath, comments); err != nil {
			return "", fmt.Errorf("write markdown for %s: %w", filePath, err)
		}

		entries = append(entries, PerFileEntry{
			Path:         filePath,
			CommentCount: len(comments),
			MD:           mdRelPath,
		})
	}

	indexPath := filepath.Join(baseDir, "index.json")
	index := PerFileIndex{
		ID:        result.ID,
		CreatedAt: result.CreatedAt,
		Project:   result.Project,
		GitLab:    result.GitLab,
		Review:    result.Review,
		Warnings:  result.Warnings,
		Files:     entries,
	}
	if err := writePerFileJSON(indexPath, index); err != nil {
		return "", fmt.Errorf("write index.json: %w", err)
	}

	return indexPath, nil
}

// isSafeFilePath validates a repo-relative file path for use in per-file output.
func isSafeFilePath(path string) bool {
	if path == "" {
		return false
	}
	// Check raw path for traversal components before cleaning.
	rawParts := strings.Split(filepath.ToSlash(path), "/")
	for _, part := range rawParts {
		if part == ".." {
			return false
		}
	}
	// Reject absolute paths on all platforms.
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") {
		return false
	}
	cleaned := filepath.ToSlash(filepath.Clean(path))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return false
	}
	for _, part := range strings.Split(cleaned, "/") {
		if part == "." || part == ".." || part == "" {
			continue
		}
		if !IsSafePathSegment(part) {
			return false
		}
	}
	return true
}

// writePerFileJSON atomically writes a JSON value to disk.
func writePerFileJSON(path string, data any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

// PerFileWriter handles incremental per-file markdown output during a
// scan/review run. It is safe for concurrent use from multiple goroutines.
type PerFileWriter struct {
	BaseDir  string
	ReviewID string
	Project  ProjectInfo

	mu      sync.Mutex
	entries []PerFileEntry
}

// NewPerFileWriter creates the per-file output directory tree and returns a
// writer ready for concurrent use.
func NewPerFileWriter(root string, project ProjectInfo, reviewID string) (*PerFileWriter, error) {
	if reviewID == "" {
		return nil, fmt.Errorf("review ID is required")
	}
	projectKey := ProjectKey(project)
	if !isSafePathSegment(projectKey) {
		return nil, fmt.Errorf("unsafe project key: %q", projectKey)
	}
	if !isSafePathSegment(reviewID) {
		return nil, fmt.Errorf("unsafe review ID: %q", reviewID)
	}
	baseDir := filepath.Join(root, projectKey, reviewID)
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, fmt.Errorf("create per-file output dir: %w", err)
	}
	return &PerFileWriter{
		BaseDir:  baseDir,
		ReviewID: reviewID,
		Project:  project,
	}, nil
}

// WriteFile writes a single file's markdown report and records it for later
// index.json generation. Safe for concurrent use. Files with zero comments are
// silently skipped (no .md is written to disk).
func (w *PerFileWriter) WriteFile(filePath string, comments []model.LlmComment) error {
	if !isSafeFilePath(filePath) {
		return fmt.Errorf("unsafe file path %q", filePath)
	}
	if len(comments) == 0 {
		return nil
	}

	mdRelPath := filePath + ".md"

	// Write the file first; only record the entry on success so index.json
	// never references a non-existent .md file.
	if err := writePerFileMarkdown(w.BaseDir, mdRelPath, Result{
		ID:      w.ReviewID,
		Project: w.Project,
	}, filePath, comments); err != nil {
		return err
	}

	w.mu.Lock()
	// Replace existing entry for the same file path, or append if new.
	found := false
	for i := range w.entries {
		if w.entries[i].Path == filePath {
			w.entries[i].CommentCount = len(comments)
			found = true
			break
		}
	}
	if !found {
		w.entries = append(w.entries, PerFileEntry{
			Path:         filePath,
			CommentCount: len(comments),
			MD:           mdRelPath,
		})
	}
	w.mu.Unlock()

	return nil
}

// Finalize writes the index.json and returns its path.
// If an existing index.json is present (e.g. from a previous run being
// resumed), the new index is merged into the existing one to preserve the
// original created_at timestamp, file entries from earlier runs, and
// accumulated token counts.
func (w *PerFileWriter) Finalize(review ReviewInfo, gitlab GitLabInfo, warnings []Warning) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	sort.Slice(w.entries, func(i, j int) bool {
		return w.entries[i].Path < w.entries[j].Path
	})

	indexPath := filepath.Join(w.BaseDir, "index.json")
	index := PerFileIndex{
		ID:        w.ReviewID,
		CreatedAt: time.Now().UTC(),
		Project:   w.Project,
		GitLab:    gitlab,
		Review:    review,
		Warnings:  warnings,
		Files:     w.entries,
	}

	// On resume: merge with existing index to preserve original metadata,
	// accumulated statistics, and file entries from previous runs.
	if existing, err := w.loadExistingIndex(); err == nil {
		index = mergePerFileIndex(existing, &index)
	}

	// Override FilesReviewed to reflect the actual number of files that
	// produced per-file output (len(w.entries)), not the dispatchable diff
	// count that callers pass in via ReviewInfo.FilesReviewed.
	index.Review.FilesReviewed = int64(len(index.Files))

	if err := writePerFileJSON(indexPath, index); err != nil {
		return "", fmt.Errorf("write index.json: %w", err)
	}
	return indexPath, nil
}

// loadExistingIndex reads and decodes an existing index.json from the
// writer's base directory. It returns an error if the file is absent or
// unreadable.
func (w *PerFileWriter) loadExistingIndex() (*PerFileIndex, error) {
	indexPath := filepath.Join(w.BaseDir, "index.json")
	f, err := os.Open(indexPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var idx PerFileIndex
	if err := json.NewDecoder(io.LimitReader(f, 10<<20)).Decode(&idx); err != nil {
		return nil, fmt.Errorf("decode existing index.json: %w", err)
	}
	return &idx, nil
}

// mergePerFileIndex merges a previous-run index into a new (current-run)
// index. It preserves the original created_at timestamp, merges file
// entries (new entries win for re-reviewed files; existing entries are
// kept for files that were reused without re-review), and accumulates
// token counts from both runs.
func mergePerFileIndex(existing *PerFileIndex, newIdx *PerFileIndex) PerFileIndex {
	merged := *newIdx
	merged.CreatedAt = existing.CreatedAt

	// Build lookup maps for file entries.
	existingByPath := make(map[string]PerFileEntry, len(existing.Files))
	for _, f := range existing.Files {
		existingByPath[f.Path] = f
	}
	newByPath := make(map[string]PerFileEntry, len(newIdx.Files))
	for _, f := range newIdx.Files {
		newByPath[f.Path] = f
	}

	// Merge files: for each existing file, keep the existing entry unless
	// the new index has a re-reviewed version of the same file.
	mergedFiles := make(map[string]PerFileEntry, len(existingByPath)+len(newByPath))
	for path, entry := range existingByPath {
		if newEntry, ok := newByPath[path]; ok {
			// File was re-reviewed; use the updated entry.
			mergedFiles[path] = newEntry
		} else {
			// File was reused without re-review; keep the existing entry.
			mergedFiles[path] = entry
		}
	}
	// Add files that only appear in the new index (newly added files).
	for path, entry := range newByPath {
		if _, ok := existingByPath[path]; !ok {
			mergedFiles[path] = entry
		}
	}

	merged.Files = make([]PerFileEntry, 0, len(mergedFiles))
	for _, entry := range mergedFiles {
		merged.Files = append(merged.Files, entry)
	}
	sort.Slice(merged.Files, func(i, j int) bool {
		return merged.Files[i].Path < merged.Files[j].Path
	})

	// Accumulate summary fields.
	// CommentCount and FilesReviewed on resume already include the reused
	// items (the agent's CommentCollector receives them in applyResume),
	// so use the larger value to avoid double-counting.
	merged.Review.FilesReviewed = max(existing.Review.FilesReviewed, newIdx.Review.FilesReviewed)
	merged.Review.CommentCount = max(existing.Review.CommentCount, newIdx.Review.CommentCount)
	// Token counts reflect only new LLM calls made during the current
	// resume run, so add them to the existing totals.
	merged.Review.TotalTokens = existing.Review.TotalTokens + newIdx.Review.TotalTokens
	merged.Review.InputTokens = existing.Review.InputTokens + newIdx.Review.InputTokens
	merged.Review.OutputTokens = existing.Review.OutputTokens + newIdx.Review.OutputTokens
	merged.Review.CacheReadTokens = existing.Review.CacheReadTokens + newIdx.Review.CacheReadTokens
	merged.Review.CacheWriteTokens = existing.Review.CacheWriteTokens + newIdx.Review.CacheWriteTokens

	// Merge warnings: keep warnings from both runs, deduplicated by
	// type + file + message.
	seen := make(map[string]bool)
	for _, w := range newIdx.Warnings {
		key := w.Type + "\x00" + w.File + "\x00" + w.Message
		seen[key] = true
	}
	for _, w := range existing.Warnings {
		key := w.Type + "\x00" + w.File + "\x00" + w.Message
		if !seen[key] {
			merged.Warnings = append(merged.Warnings, w)
			seen[key] = true
		}
	}

	return merged
}

// Entries returns a copy of the current entry list.
func (w *PerFileWriter) Entries() []PerFileEntry {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]PerFileEntry, len(w.entries))
	copy(out, w.entries)
	return out
}

// ExistingPerFilePaths returns the set of file paths that already have per-file
// .md output for the given review. Used during resume to avoid re-saving files
// that were already persisted.
func ExistingPerFilePaths(root, projectKey, reviewID string) (map[string]bool, error) {
	if !isSafePathSegment(projectKey) || !isSafePathSegment(reviewID) {
		return nil, fmt.Errorf("invalid path segment")
	}
	dir := filepath.Join(root, projectKey, reviewID)
	paths := make(map[string]bool)
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".md") {
			rel, rerr := filepath.Rel(dir, path)
			if rerr != nil {
				return rerr
			}
			rel = strings.TrimSuffix(rel, ".md")
			rel = filepath.ToSlash(rel)
			paths[rel] = true
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return paths, nil
}

// GenerateID creates a new random UUID v4 string.
func GenerateID() (string, error) {
	return generateID()
}

func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", fmt.Errorf("generate review id: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
