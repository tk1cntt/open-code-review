package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/reviewstore"
	"github.com/alibaba/open-code-review/internal/scan"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/telemetry"
	"github.com/alibaba/open-code-review/internal/tool"
	"github.com/spf13/cobra"

	"go.opentelemetry.io/otel/codes"
)

type scanOptions struct {
	toolConfigPath  string
	rulePath        string
	repoDir         string
	paths           string
	excludes        string
	outputFormat    string
	audience        string
	background      string
	resume          string // --resume: resume from a previous scan session id
	resumeMode      string // --resume-mode: continue | restart-failed
	saveResult      bool   // --save-result: persist final scan result for the WebUI review viewer
	savePerFile     bool   // --save-per-file: split output into per-file markdown files
	resultDir       string // --result-dir: root directory for persisted scan results
	resultProject   string // --result-project: display project name/path for persisted results
	concurrency     int
	perFileTimeout  int
	maxTools        int
	maxGitProcs     int
	preview         bool
	noPlan          bool
	noDedup         bool
	noSummary       bool
	batch           string
	maxTokensBudget int
	provider        string
	model           string
}

var scanOpts scanOptions

var scanCmd = &cobra.Command{
	Use:     "scan [flags]",
	Aliases: []string{"s"},
	Short:   "Scan entire files (no diff required)",
	Long:    "OpenCodeReview - Full-File Scan\n\nScan entire files for code review without requiring a diff.",
	Args:    cobra.NoArgs,
	Example: `  # Scan the entire repository
  ocr scan

  # Scan a single directory
  ocr scan --path internal/agent

  # Scan multiple files
  ocr scan --path internal/agent/agent.go,internal/diff/scan.go

  # Exclude generated files / fixtures
  ocr scan --exclude '**/generated/*,**/testdata/*'

  # Preview which files would be scanned without calling the LLM
  ocr scan --preview

  # Skip the per-file PLAN_TASK pre-pass
  ocr scan --no-plan`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateScanOptions(&scanOpts); err != nil {
			return err
		}
		return executeScan(scanOpts)
	},
}

func init() {
	registerScanFlags(scanCmd, &scanOpts)
}

func splitPaths(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func saveScanResult(repoDir string, opts scanOptions, ag *scan.Agent, comments []model.LlmComment, warnings []llmloop.AgentWarning, duration time.Duration, resultID string) (string, string, error) {
	sess := ag.Session()
	if sess == nil {
		return "", "", fmt.Errorf("agent session is nil, cannot save scan result")
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

func loadScanResumeState(repoDir string, opts scanOptions) (*session.ResumeState, error) {
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
	if !state.HasSessionScope() {
		fmt.Fprintf(os.Stderr, "[ocr] Resume session %q: no completed/failed/in-progress files found — scanning all files fresh\n",
			opts.resume)
	} else {
		fmt.Fprintf(os.Stderr, "[ocr] Resume session %q: reusing %d completed, re-running %d session file(s) (%d mid-file)\n",
			opts.resume, state.CompletedCount(), state.FailedCount(), state.InProgressCount())
	}
	return state, nil
}

func executeScan(opts scanOptions) error {
	cc, err := loadCommonContext(opts.repoDir, opts.rulePath, "", opts.maxTools, opts.maxGitProcs, false)
	if err != nil {
		return err
	}
	applyCLIExcludes(cc, splitPaths(opts.excludes))

	// scan owns its own template (scan_template.json) independent from the
	// diff-review template loaded by loadCommonContext above. Apply --max-tools
	// as an "only raise" override to the scan template's per-file budget.
	scanTpl, err := template.LoadScanDefault()
	if err != nil {
		return fmt.Errorf("load scan template: %w", err)
	}
	if err := scanTpl.Validate(); err != nil {
		return fmt.Errorf("invalid scan template: %w", err)
	}
	if opts.maxTools > scanTpl.MaxToolRequestTimes {
		scanTpl.MaxToolRequestTimes = opts.maxTools
	}
	if opts.batch != "" {
		// CLI override of BATCH_STRATEGY; validated downstream by parseBatchStrategy
		// (unknown values silently fall back to "none").
		scanTpl.BatchStrategy = opts.batch
	}
	// Token budget: --max-tokens-budget overrides the template value when set.
	budget := scanTpl.MaxTokensBudget
	if opts.maxTokensBudget > 0 {
		budget = int64(opts.maxTokensBudget)
	}

	scanPaths := splitPaths(opts.paths)

	if opts.preview {
		return runScanPreview(cc, scanTpl, scanPaths)
	}

	resumeState, err := loadScanResumeState(cc.RepoDir, opts)
	if err != nil {
		return err
	}

	rt, err := loadLLMRuntime(cc.Template, opts.toolConfigPath, llm.ResolveOptions{
		Provider: opts.provider,
		Model:    opts.model,
	})
	if err != nil {
		return err
	}
	llmIdentity := &jsonLLMIdentity{
		Provider: rt.Provider,
		Model:    rt.Model,
	}
	// Apply language to the scan template too (loadLLMRuntime only mutates
	// the diff-review template it was handed).
	if rt.AppCfg != nil {
		scanTpl.ApplyLanguage(rt.AppCfg.Language)
	}

	// file_read_diff is meaningless in scan mode (no diff exists). Hiding it
	// from MainToolDefs stops the LLM from burning tool-call rounds probing
	// for diff content that does not exist.
	scanToolDefs := excludeToolDef(rt.MainToolDefs, "file_read_diff")

	// Scan mode always reads file contents from the working tree.
	fileReader := &tool.FileReader{
		RepoDir: cc.RepoDir,
		Mode:    tool.ModeWorkspace,
		Runner:  cc.GitRunner,
	}
	tools := buildToolRegistry(rt.Collector, fileReader)

	var perFileWriter *reviewstore.PerFileWriter

	ag := scan.NewAgent(scan.Args{
		RepoDir:               cc.RepoDir,
		Paths:                 scanPaths,
		Template:              *scanTpl,
		SystemRule:            cc.Resolver,
		FileFilter:            cc.FileFilter,
		LLMClient:             rt.Client,
		Tools:                 tools,
		MainToolDefs:          scanToolDefs,
		CommentCollector:      rt.Collector,
		CommentWorkerPool:     llmloop.NewCommentWorkerPool(opts.concurrency),
		MaxConcurrency:        opts.concurrency,
		ConcurrentTaskTimeout: opts.perFileTimeout,
		Model:                 rt.Model,
		Background:            opts.background,
		GitRunner:             cc.GitRunner,
		MaxFileSizeBytes:      scanTpl.MaxFileSizeBytes,
		MaxTokensBudget:       budget,
		SkipPlan:              opts.noPlan,
		SkipDedup:             opts.noDedup,
		SkipSummary:           opts.noSummary,
		Resume:                resumeState,
		ResumeMode:            opts.resumeMode,
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

	ctx, span := telemetry.StartSpan(telemetry.ContextWithTraceParentFromEnv(context.Background()), "scan.run")
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

	// Persist/emit partial results even when every file fails (e.g. timeout).
	persistScanOutputs(cc.RepoDir, opts, ag, comments, duration, reviewID, perFileWriter, &finalized)

	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
		if id := ag.SessionID(); id != "" {
			fmt.Fprintf(os.Stderr, "[ocr] Session: %s (retry with: --resume %s)\n", id, id)
		}
		if len(comments) > 0 {
			fmt.Fprintf(os.Stderr, "[ocr] Wrote %d partial finding(s) before failure\n", len(comments))
		}
		if emitErr := emitRunResult(ctx, ag, comments, duration, opts.outputFormat, opts.audience, q, llmIdentity); emitErr != nil {
			fmt.Fprintf(os.Stderr, "[ocr] warning: failed to emit partial scan result: %v\n", emitErr)
		}
		return fmt.Errorf("scan failed: %w", err)
	}

	if failed := ag.SubtaskFailed(); failed > 0 {
		if id := ag.SessionID(); id != "" {
			fmt.Fprintf(os.Stderr, "[ocr] %d file(s) failed — Session: %s (retry with: --resume %s)\n", failed, id, id)
		}
	}

	return emitRunResult(ctx, ag, comments, duration, opts.outputFormat, opts.audience, q, llmIdentity)
}

func persistScanOutputs(
	repoDir string,
	opts scanOptions,
	ag *scan.Agent,
	comments []model.LlmComment,
	duration time.Duration,
	reviewID string,
	perFileWriter *reviewstore.PerFileWriter,
	finalized *bool,
) {
	if opts.saveResult {
		if opts.resultDir == "" {
			if d := os.Getenv("OCR_REVIEWS_DIR"); d != "" {
				opts.resultDir = d
			} else {
				opts.resultDir = filepath.Join(repoDir, ".opencodereview", "reviews")
			}
		}
		path, mdPath, saveErr := saveScanResult(repoDir, opts, ag, comments, ag.Warnings(), duration, reviewID)
		if saveErr != nil {
			fmt.Fprintf(os.Stderr, "[ocr] warning: failed to save scan result: %v\n", saveErr)
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
			ProjectID:       firstNonEmpty(os.Getenv("CI_PROJECT_ID"), filepath.Base(repoDir)),
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
		if finalized != nil {
			*finalized = true
		}
	}
}

func runScanPreview(cc *commonContext, scanTpl *template.ScanTemplate, scanPaths []string) error {
	ag := scan.NewAgent(scan.Args{
		RepoDir:          cc.RepoDir,
		Paths:            scanPaths,
		FileFilter:       cc.FileFilter,
		GitRunner:        cc.GitRunner,
		MaxFileSizeBytes: scanTpl.MaxFileSizeBytes,
		// Template's prompt fields are unused by Preview; pass the same
		// value so MaxFileSizeBytes is consistent.
		Template: *scanTpl,
	})

	preview, err := ag.Preview(context.Background())
	if err != nil {
		return fmt.Errorf("scan preview failed: %w", err)
	}
	outputPreviewText(preview)
	return nil
}
