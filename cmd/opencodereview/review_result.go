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
			FilesReviewed:    ag.FilesReviewed(),
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
