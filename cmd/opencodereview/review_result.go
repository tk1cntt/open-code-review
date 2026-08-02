package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/reviewstore"
	"github.com/alibaba/open-code-review/internal/session"
)

func saveReviewResult(repoDir string, opts reviewOptions, ag *agent.Agent, comments []model.LlmComment, warnings []agent.AgentWarning, duration time.Duration, resultID string) (string, string, error) {
	sess := ag.Session()
	if sess == nil {
		return "", "", fmt.Errorf("agent session is nil, cannot save review result")
	}
	sourceBranch := firstNonEmpty(opts.resultSourceBranch, os.Getenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME"))
	targetBranch := firstNonEmpty(opts.resultTargetBranch, os.Getenv("CI_MERGE_REQUEST_TARGET_BRANCH_NAME"))
	projectName := firstNonEmpty(opts.resultProject, os.Getenv("CI_PROJECT_PATH"), filepath.Base(repoDir))
	projectID := firstNonEmpty(os.Getenv("CI_PROJECT_ID"), filepath.Base(repoDir))

	reviewMode := session.ReviewModeWorkspace
	if opts.commit != "" {
		reviewMode = session.ReviewModeCommit
	} else if opts.from != "" && opts.to != "" {
		reviewMode = session.ReviewModeRange
	}

	result := reviewstore.Result{
		ID: resultID,
		Project: reviewstore.ProjectInfo{
			ID:      projectID,
			Name:    projectName,
			RepoDir: repoDir,
			WebURL:  os.Getenv("CI_PROJECT_URL"),
		},
		GitLab: reviewstore.GitLabInfo{
			ServerURL:       os.Getenv("CI_SERVER_URL"),
			ProjectID:       projectID,
			MergeRequestIID: os.Getenv("CI_MERGE_REQUEST_IID"),
			PipelineID:      os.Getenv("CI_PIPELINE_ID"),
			JobID:           os.Getenv("CI_JOB_ID"),
		},
		Review: reviewstore.ReviewInfo{
			Mode:             reviewMode,
			SourceBranch:     sourceBranch,
			TargetBranch:     targetBranch,
			From:             opts.from,
			To:               opts.to,
			Commit:           opts.commit,
			Model:            sess.Model,
			FilesReviewed:    ag.TotalFilesReviewed(),
			CommentCount:     int64(len(comments)),
			TotalTokens:      ag.TotalTokensUsed(),
			InputTokens:      ag.TotalInputTokens(),
			OutputTokens:     ag.TotalOutputTokens(),
			CacheReadTokens:  ag.TotalCacheReadTokens(),
			CacheWriteTokens: ag.TotalCacheWriteTokens(),
			Duration:         duration.String(),
			DurationSeconds:  int64(duration.Seconds()),
			SessionID:        ag.SessionID(),
		},
		Comments: comments,
		Warnings: mapWarnings(warnings),
	}

	if sess.ResumedFrom != "" {
		projectKey := reviewstore.ProjectKey(result.Project)
		if existing, loadErr := reviewstore.Load(opts.resultDir, projectKey, resultID); loadErr == nil {
			result = mergeResults(*existing, result)
		}
	}

	return reviewstore.Save(opts.resultDir, result)
}

func mapWarnings(warnings []llmloop.AgentWarning) []reviewstore.Warning {
	if len(warnings) == 0 {
		return nil
	}
	out := make([]reviewstore.Warning, 0, len(warnings))
	for _, w := range warnings {
		out = append(out, reviewstore.Warning{
			File:    w.File,
			Message: w.Message,
			Type:    w.Type,
		})
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// mergeResults merges a current review result into an existing one on resume.
// Merged fields:
//   - Comments: deduplicated by file path (current run wins for duplicate paths)
//   - Tokens: summed across both runs
//   - Duration: summed (parsed as time.Duration)
//   - FilesReviewed: deduplicated unique file paths across both runs
//   - CreatedAt: preserved from existing
//   - Warnings: deduplicated by message content
func mergeResults(existing, current reviewstore.Result) reviewstore.Result {
	merged := existing
	merged.CreatedAt = existing.CreatedAt // keep original timestamp

	// Merge comments: append current, deduplicate by file path (current wins)
	currentPaths := make(map[string]bool)
	for _, c := range current.Comments {
		currentPaths[c.Path] = true
	}
	var deduped []model.LlmComment
	for _, c := range existing.Comments {
		if currentPaths[c.Path] {
			continue // current run has a newer review for this file
		}
		deduped = append(deduped, c)
	}
	deduped = append(deduped, current.Comments...)
	merged.Comments = deduped
	merged.Review.CommentCount = int64(len(deduped))

	// Sum tokens
	merged.Review.TotalTokens = existing.Review.TotalTokens + current.Review.TotalTokens
	merged.Review.InputTokens = existing.Review.InputTokens + current.Review.InputTokens
	merged.Review.OutputTokens = existing.Review.OutputTokens + current.Review.OutputTokens
	merged.Review.CacheReadTokens = existing.Review.CacheReadTokens + current.Review.CacheReadTokens
	merged.Review.CacheWriteTokens = existing.Review.CacheWriteTokens + current.Review.CacheWriteTokens

	// Sum duration
	merged.Review.DurationSeconds = existing.Review.DurationSeconds + current.Review.DurationSeconds
	if existingDur, err := time.ParseDuration(existing.Review.Duration); err == nil {
		if currentDur, err2 := time.ParseDuration(current.Review.Duration); err2 == nil {
			merged.Review.Duration = (existingDur + currentDur).String()
		}
	}

	// FilesReviewed: count unique file paths across both runs
	seenPaths := make(map[string]bool)
	for _, c := range merged.Comments {
		seenPaths[c.Path] = true
	}
	merged.Review.FilesReviewed = int64(len(seenPaths))

	// Merge warnings: deduplicate by type + file + message. Including
	// type in the key prevents two warnings with the same message text
	// but different severity/type from colliding.
	existingWarn := make(map[string]bool)
	for _, w := range existing.Warnings {
		key := w.Type + "\x00" + w.File + "\x00" + w.Message
		existingWarn[key] = true
	}
	for _, w := range current.Warnings {
		key := w.Type + "\x00" + w.File + "\x00" + w.Message
		if !existingWarn[key] {
			merged.Warnings = append(merged.Warnings, w)
			existingWarn[key] = true
		}
	}

	return merged
}
