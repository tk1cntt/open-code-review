package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/refactor"
	"github.com/alibaba/open-code-review/internal/reviewstore"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/telemetry"
	"github.com/alibaba/open-code-review/internal/tool"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/codes"
)

type refactorOptions struct {
	toolConfigPath, rulePath, repoDir, paths, excludes string
	outputFormat, audience, background, resume         string
	saveResult, savePerFile                            bool
	resultDir, resultProject                           string
	concurrency, perFileTimeout, maxTools, maxGitProcs, maxTokensBudget int
	noPlan                                                               bool
	model                                              string
	showHelp                                           bool
	preview                                            bool
}

var refactorOpts refactorOptions

var refactorCmd = &cobra.Command{
	Use:     "refactor [flags]",
	Aliases: []string{"rf"},
	Short:   "Analyze code and suggest refactoring improvements",
	Long:    "OpenCodeReview - AI-Powered Code Refactoring\n\nAnalyze code and suggest refactoring improvements using a configurable LLM.",
	Args:    cobra.NoArgs,
	Example: `  # Analyze the entire repository
  ocr refactor

  # Analyze a single directory
  ocr refactor --path internal/agent

  # Analyze multiple files
  ocr refactor --path internal/agent/agent.go,internal/diff/scan.go

  # Preview which files would be analyzed without calling the LLM
  ocr refactor --preview

  # Skip the per-file PLAN_TASK pre-pass (saves ~1 LLM call per file)
  ocr refactor --no-plan

  # Exclude generated files / fixtures
  ocr refactor --exclude '**/generated/*,**/testdata/*'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateRefactorOptions(&refactorOpts); err != nil {
			return err
		}
		return executeRefactor(refactorOpts)
	},
}

func init() {
	registerRefactorFlags(refactorCmd, &refactorOpts)
}

func executeRefactor(opts refactorOptions) error {

	cc, err := loadCommonContext(opts.repoDir, opts.rulePath, "", opts.maxTools, opts.maxGitProcs, false)
	if err != nil {
		return err
	}
	applyCLIExcludes(cc, splitPaths(opts.excludes))

	refactorTpl, err := template.LoadRefactorDefault()
	if err != nil {
		return fmt.Errorf("load refactor template: %w", err)
	}
	if err := refactorTpl.Validate(); err != nil {
		return fmt.Errorf("invalid refactor template: %w", err)
	}
	if opts.maxTools > refactorTpl.MaxToolRequestTimes {
		refactorTpl.MaxToolRequestTimes = opts.maxTools
	}
	if opts.maxTokensBudget > 0 {
		refactorTpl.MaxTokensBudget = int64(opts.maxTokensBudget)
	}

	refactorPaths := splitPaths(opts.paths)

	if opts.preview {
		return runRefactorPreview(cc, refactorTpl, refactorPaths)
	}

	resumeState, err := loadRefactorResumeState(cc.RepoDir, opts)
	if err != nil {
		return err
	}

	rt, err := loadLLMRuntime(cc.Template, opts.toolConfigPath, opts.model)
	if err != nil {
		return err
	}
	if rt.AppCfg != nil {
		refactorTpl.ApplyLanguage(rt.AppCfg.Language)
	}

	refactorToolDefs := excludeToolDef(rt.MainToolDefs, "file_read_diff")

	fileReader := &tool.FileReader{
		RepoDir: cc.RepoDir,
		Mode:    tool.ModeWorkspace,
		Runner:  cc.GitRunner,
	}
	tools := buildToolRegistry(rt.Collector, fileReader)

	var perFileWriter *reviewstore.PerFileWriter

	ag := refactor.NewAgent(refactor.Args{
		RepoDir:               cc.RepoDir,
		Paths:                 refactorPaths,
		Template:              *refactorTpl,
		SystemRule:            cc.Resolver,
		FileFilter:            cc.FileFilter,
		LLMClient:             rt.Client,
		Tools:                 tools,
		MainToolDefs:          refactorToolDefs,
		CommentCollector:      rt.Collector,
		CommentWorkerPool:     llmloop.NewCommentWorkerPool(opts.concurrency),
		MaxConcurrency:        opts.concurrency,
		ConcurrentTaskTimeout: opts.perFileTimeout,
		Model:                 rt.Model,
		Background:            opts.background,
		GitRunner:             cc.GitRunner,
		MaxFileSizeBytes:      refactorTpl.MaxFileSizeBytes,
		MaxTokensBudget:       refactorTpl.MaxTokensBudget,
		SkipPlan:              opts.noPlan,
		Resume:                resumeState,
		OnFileDone: func(filePath string, comments []model.LlmComment) {
			if perFileWriter != nil {
				if err := perFileWriter.WriteFile(filePath, comments); err != nil {
					fmt.Fprintf(os.Stderr, "[ocr] warning: per-file save failed for %s: %v\n", filePath, err)
				}
			}
		},
	})

	// Use the session ID as the review result ID so --resume and review
	// result persistence share one consistent identifier.
	reviewID := ag.SessionID()
	if opts.savePerFile {
		if opts.resultDir == "" {
			if d := os.Getenv("OCR_REVIEWS_DIR"); d != "" {
				opts.resultDir = d
			} else {
				opts.resultDir = filepath.Join(cc.RepoDir, ".opencodereview", "reviews")
			}
		}
		projectName := firstNonEmpty(opts.resultProject, os.Getenv("CI_PROJECT_PATH"), filepath.Base(cc.RepoDir))
		projectID := firstNonEmpty(os.Getenv("CI_PROJECT_ID"), filepath.Base(cc.RepoDir))
		project := reviewstore.ProjectInfo{
			ID:      projectID,
			Name:    projectName,
			RepoDir: cc.RepoDir,
			WebURL:  os.Getenv("CI_PROJECT_URL"),
		}
		pfw, pfwErr := reviewstore.NewPerFileWriter(opts.resultDir, project, reviewID)
		if pfwErr != nil {
			return fmt.Errorf("create per-file writer: %w", pfwErr)
		}
		perFileWriter = pfw
	}

	q := newQuietHandle(opts.outputFormat, opts.audience)
	defer q.Restore()

	ctx, span := telemetry.StartSpan(telemetry.ContextWithTraceParentFromEnv(context.Background()), "refactor.run")
	defer span.End()
	var traceID string
	if telemetry.IsEnabled() {
		traceID = telemetry.TraceIDFromContext(ctx)
		if opts.outputFormat != "json" {
			fmt.Fprintf(os.Stderr, "[ocr] TraceID: %s\n", traceID)
		}
	}
	startTime := time.Now()

	var finalized bool
	defer func() {
		if perFileWriter != nil && !finalized {
			if _, fErr := perFileWriter.Finalize(reviewstore.ReviewInfo{}, reviewstore.GitLabInfo{}, nil); fErr != nil {
				fmt.Fprintf(os.Stderr, "[ocr] warning: failed to finalize partial per-file output: %v\n", fErr)
			}
		}
	}()

	comments, err := ag.Run(ctx)
	duration := time.Since(startTime)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
		if id := ag.SessionID(); id != "" {
			fmt.Fprintf(os.Stderr, "[ocr] Session: %s (retry with: --resume %s)\n", id, id)
		}
		return fmt.Errorf("refactor failed: %w", err)
	}

	if failed := ag.SubtaskFailed(); failed > 0 {
		if id := ag.SessionID(); id != "" {
			fmt.Fprintf(os.Stderr, "[ocr] %d file(s) failed — Session: %s (retry with: --resume %s)\n", failed, id, id)
		}
	}

	if opts.saveResult {
		if opts.resultDir == "" {
			if d := os.Getenv("OCR_REVIEWS_DIR"); d != "" {
				opts.resultDir = d
			} else {
				opts.resultDir = filepath.Join(cc.RepoDir, ".opencodereview", "reviews")
			}
		}
		path, mdPath, err := saveRefactorResult(cc.RepoDir, opts, ag, comments, ag.Warnings(), duration, reviewID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ocr] warning: failed to save refactor result: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "[ocr] JSON saved to: %s\n", path)
			if mdPath != "" {
				fmt.Fprintf(os.Stderr, "[ocr] Markdown report saved to: %s\n", mdPath)
			}
		}
	}
	if perFileWriter != nil {
		sess := ag.Session()
		gitlab := reviewstore.GitLabInfo{
			ServerURL:       os.Getenv("CI_SERVER_URL"),
			ProjectID:       firstNonEmpty(os.Getenv("CI_PROJECT_ID"), filepath.Base(cc.RepoDir)),
			MergeRequestIID: os.Getenv("CI_MERGE_REQUEST_IID"),
			PipelineID:      os.Getenv("CI_PIPELINE_ID"),
			JobID:           os.Getenv("CI_JOB_ID"),
		}
		reviewInfo := reviewstore.ReviewInfo{
			Mode:             session.ReviewModeFullScan,
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
		}
		if sess != nil {
			reviewInfo.Model = sess.Model
		}
		perFileIdxPath, fErr := perFileWriter.Finalize(reviewInfo, gitlab, mapWarnings(ag.Warnings()))
		if fErr != nil {
			fmt.Fprintf(os.Stderr, "[ocr] warning: failed to finalize per-file output: %v\n", fErr)
		} else {
			fmt.Fprintf(os.Stderr, "[ocr] Per-file output saved to: %s\n", perFileIdxPath)
		}
		finalized = true
	}

	return emitRunResult(ctx, ag, comments, duration, opts.outputFormat, opts.audience, q)
}

func runRefactorPreview(cc *commonContext, refactorTpl *template.RefactorTemplate, refactorPaths []string) error {
	ag := refactor.NewAgent(refactor.Args{
		RepoDir:          cc.RepoDir,
		Paths:            refactorPaths,
		FileFilter:       cc.FileFilter,
		GitRunner:        cc.GitRunner,
		MaxFileSizeBytes: refactorTpl.MaxFileSizeBytes,
		Template:         *refactorTpl,
	})

	preview, err := ag.Preview(context.Background())
	if err != nil {
		return fmt.Errorf("refactor preview failed: %w", err)
	}
	outputPreviewText(preview)
	return nil
}

func saveRefactorResult(repoDir string, opts refactorOptions, ag *refactor.Agent, comments []model.LlmComment, warnings []llmloop.AgentWarning, duration time.Duration, resultID string) (string, string, error) {
	sess := ag.Session()
	if sess == nil {
		return "", "", fmt.Errorf("agent session is nil, cannot save refactor result")
	}
	projectName := firstNonEmpty(opts.resultProject, os.Getenv("CI_PROJECT_PATH"), filepath.Base(repoDir))
	projectID := firstNonEmpty(os.Getenv("CI_PROJECT_ID"), filepath.Base(repoDir))

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
			Mode:             session.ReviewModeFullScan,
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

	jsonPath, mdPath, saveErr := reviewstore.Save(opts.resultDir, result)
	if saveErr != nil {
		return "", "", saveErr
	}
	return jsonPath, mdPath, nil
}

func loadRefactorResumeState(repoDir string, opts refactorOptions) (*session.ResumeState, error) {
	if opts.resume == "" {
		return nil, nil
	}
	current := session.SessionOptions{
		ReviewMode: session.ReviewModeFullScan,
	}
	state, err := session.LoadResumeState(repoDir, opts.resume)
	if err != nil {
		return nil, fmt.Errorf("load resume session: %w (run 'ocr session list' to see available sessions)", err)
	}
	if err := state.ValidateOptions(current); err != nil {
		return nil, fmt.Errorf("%w (run 'ocr session list' to see available sessions)", err)
	}
	if state.CompletedCount() == 0 && state.FailedCount() == 0 {
		fmt.Fprintf(os.Stderr, "[ocr] Resume session %q: no completed items or failed files found — analyzing all files fresh\n",
			opts.resume)
	} else {
		fmt.Fprintf(os.Stderr, "[ocr] Resume session %q: reusing %d completed file(s), retrying %d failed file(s)\n",
			opts.resume, state.CompletedCount(), state.FailedCount())
	}
	return state, nil
}
