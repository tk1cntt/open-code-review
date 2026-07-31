package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/reviewstore"
	"github.com/alibaba/open-code-review/internal/scan"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/telemetry"
	"github.com/alibaba/open-code-review/internal/tool"

	"go.opentelemetry.io/otel/codes"
)

// scanOptions mirrors reviewOptions for the full-scan subcommand. The two
// types are kept separate so the scan flag set can evolve independently of
// the diff-based review flags (e.g. --from/--to/--commit make no sense here).
//
// Bare `ocr scan` (no --path) scans the entire repository; --path narrows.
type scanOptions struct {
	toolConfigPath  string
	rulePath        string
	repoDir         string
	paths           string // comma-separated relative paths; empty = whole repo
	excludes        string // comma-separated gitignore-style exclude patterns
	outputFormat    string
	audience        string
	background      string
	resume          string // --resume: resume from a previous scan session id
	saveResult      bool   // --save-result: persist final scan result for the WebUI review viewer
	savePerFile     bool   // --save-per-file: split output into per-file markdown files
	resultDir       string // --result-dir: root directory for persisted scan results
	resultProject   string // --result-project: display project name/path for persisted results
	concurrency     int
	perFileTimeout  int
	maxTools        int
	maxGitProcs     int
	preview         bool
	noPlan          bool   // --no-plan: skip the PLAN_TASK pre-pass per file
	noDedup         bool   // --no-dedup: skip the per-batch DEDUP_TASK
	noSummary       bool   // --no-summary: skip the post-run PROJECT_SUMMARY_TASK
	batch           string // --batch: override scan template's BATCH_STRATEGY
	maxTokensBudget int    // --max-tokens-budget: cap total token usage; 0 = unlimited
	model           string // --model: override resolved LLM model for this scan
	showHelp        bool
}

func parseScanFlags(args []string) (scanOptions, error) {
	a := newOcrFlagSet("ocr scan")
	opts := scanOptions{}

	a.StringVar(&opts.toolConfigPath, "tools", "", "path to JSON tools config file (default: embedded)")
	a.StringVar(&opts.rulePath, "rule", "", "path to JSON file with system review rules")
	a.StringVar(&opts.repoDir, "repo", "", "root directory of the git repository (default: current dir)")
	a.StringVar(&opts.paths, "path", "", "comma-separated repo-relative directories or files to scan (default: whole repo)")
	a.StringVar(&opts.excludes, "exclude", "", "comma-separated gitignore-style patterns to exclude; merged with rule.json excludes")
	a.StringVarP(&opts.outputFormat, "format", "f", "text", "output format: text or json")
	a.IntVar(&opts.concurrency, "concurrency", 8, "max concurrent file scans")
	a.IntVar(&opts.perFileTimeout, "timeout", 10, "concurrent task timeout in minutes")
	a.StringVar(&opts.audience, "audience", "human", "output audience: human (show progress) or agent (summary only)")
	a.StringVarP(&opts.background, "background", "b", "", "optional requirement/business context for the scan")
	a.IntVar(&opts.maxTools, "max-tools", 0, "max tool call rounds per file; only takes effect when greater than template default")
	a.IntVar(&opts.maxGitProcs, "max-git-procs", 16, "max concurrent git subprocesses")
	a.BoolVarP(&opts.preview, "preview", "p", false, "preview which files will be scanned without running the LLM")
	a.BoolVar(&opts.noPlan, "no-plan", false, "skip the per-file PLAN_TASK pre-pass (one fewer LLM call per file; may reduce review focus)")
	a.BoolVar(&opts.noDedup, "no-dedup", false, "skip the per-batch DEDUP_TASK (keeps raw comments; one fewer LLM call per batch)")
	a.BoolVar(&opts.noSummary, "no-summary", false, "skip the post-run PROJECT_SUMMARY_TASK (no project-level markdown summary)")
	a.StringVar(&opts.batch, "batch", "", "override BATCH_STRATEGY from scan template: none | by-language | by-directory")
	a.IntVar(&opts.maxTokensBudget, "max-tokens-budget", 0, "cap total token usage (input+output); dispatch stops once exceeded (0 = unlimited)")
	a.StringVar(&opts.model, "model", "", "override LLM model for this scan (e.g., claude-opus-4-6)")
	a.StringVar(&opts.resume, "resume", "", "resume from a previous scan session id")
	a.BoolVar(&opts.saveResult, "save-result", true, "persist final scan result for the WebUI review viewer")
	a.BoolVar(&opts.savePerFile, "save-per-file", true, "split output into per-file markdown files under a directory tree mirroring the source tree")
	a.StringVar(&opts.resultDir, "result-dir", "", "scan result storage root (env: OCR_REVIEWS_DIR, default: .opencodereview/reviews)")
	a.StringVar(&opts.resultProject, "result-project", "", "project name/path for persisted scan results")

	if err := a.Parse(args); err != nil {
		return opts, fmt.Errorf("parse flags: %w", err)
	}

	opts.showHelp = a.showHelp
	if opts.showHelp {
		return opts, nil
	}

	switch opts.audience {
	case "human", "agent":
	default:
		return opts, fmt.Errorf("invalid --audience value %q: must be 'human' or 'agent'", opts.audience)
	}

	if opts.maxTools < 0 {
		return opts, fmt.Errorf("--max-tools must be a non-negative integer (0 means use template default)")
	}
	if opts.maxGitProcs < 0 {
		return opts, fmt.Errorf("--max-git-procs must be a non-negative integer (0 means use default 16)")
	}
	if opts.maxTokensBudget < 0 {
		return opts, fmt.Errorf("--max-tokens-budget must be a non-negative integer (0 means unlimited)")
	}
	return opts, nil
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
	if state.CompletedCount() == 0 && state.FailedCount() == 0 {
		fmt.Fprintf(os.Stderr, "[ocr] Resume session %q: no completed items or failed files found — scanning all files fresh\n",
			opts.resume)
	} else {
		fmt.Fprintf(os.Stderr, "[ocr] Resume session %q: reusing %d completed file(s), retrying %d failed file(s)\n",
			opts.resume, state.CompletedCount(), state.FailedCount())
	}
	return state, nil
}

func runScan(args []string) error {
	opts, err := parseScanFlags(args)
	if err != nil {
		// parseScanFlags already wraps with "parse flags: %w" — return as-is.
		return err
	}
	if opts.showHelp {
		printScanUsage()
		return nil
	}

	// scan path: git is preferred (more accurate .gitignore handling) but not required;
	// provider falls back to filepath.Walk when the dir is not a git repo.
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

	rt, err := loadLLMRuntime(cc.Template, opts.toolConfigPath, opts.model)
	if err != nil {
		return err
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
	var reviewID string
	if opts.saveResult || opts.savePerFile {
		var idErr error
		reviewID, idErr = reviewstore.GenerateID()
		if idErr != nil {
			return fmt.Errorf("generate review ID: %w", idErr)
		}
	}
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
		OnFileDone: func(filePath string, comments []model.LlmComment) {
			if perFileWriter != nil {
				if err := perFileWriter.WriteFile(filePath, comments); err != nil {
					fmt.Fprintf(os.Stderr, "[ocr] warning: per-file save failed for %s: %v\n", filePath, err)
				}
			}
		},
	})

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
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
		if id := ag.SessionID(); id != "" {
			fmt.Fprintf(os.Stderr, "[ocr] Session: %s (retry with: --resume %s)\n", id, id)
		}
		return fmt.Errorf("scan failed: %w", err)
	}

	if failed := ag.SubtaskFailed(); failed > 0 {
		if id := ag.SessionID(); id != "" {
			fmt.Fprintf(os.Stderr, "[ocr] %d file(s) failed — Session: %s (retry with: --resume %s)\n", failed, id, id)
		}
	}

	duration := time.Since(startTime)
	if opts.saveResult {
		if opts.resultDir == "" {
			if d := os.Getenv("OCR_REVIEWS_DIR"); d != "" {
				opts.resultDir = d
			} else {
				opts.resultDir = filepath.Join(cc.RepoDir, ".opencodereview", "reviews")
			}
		}
		path, mdPath, err := saveScanResult(cc.RepoDir, opts, ag, comments, ag.Warnings(), duration, reviewID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ocr] warning: failed to save scan result: %v\n", err)
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

	return emitRunResult(ctx, ag, comments, startTime, opts.outputFormat, opts.audience, q)
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

func printScanUsage() {
	fmt.Println(`OpenCodeReview - Full-File Scan

Usage:
  ocr scan [flags]
  ocr s    [flags]                (alias)

Examples:
  # Scan the entire repository (default when no --path is given)
  ocr scan

  # Scan a single directory
  ocr scan --path internal/agent

  # Scan multiple files
  ocr scan --path internal/agent/agent.go,internal/diff/scan.go

  # Exclude generated files / fixtures
  ocr scan --exclude '**/generated/*,**/testdata/*'

  # Preview which files would be scanned without calling the LLM
  ocr scan --preview

  # Skip the per-file PLAN_TASK pre-pass (saves ~1 LLM call per file, may
  # reduce review focus)
  ocr scan --no-plan

Flags:
  --path string           comma-separated repo-relative dirs/files to scan (default: whole repo)
  --exclude string        comma-separated gitignore-style patterns to exclude (merged with rule.json)
  --no-plan               skip the per-file PLAN_TASK pre-pass (faster, less focused)
  --no-dedup              skip the per-batch DEDUP_TASK (keeps raw comments)
  --no-summary            skip the post-run PROJECT_SUMMARY_TASK
  --batch string          override BATCH_STRATEGY: none | by-language | by-directory
  --max-tokens-budget int cap total token usage; dispatch stops once exceeded (0 = unlimited)
  --model string          override LLM model for this scan (e.g., claude-opus-4-6)
  --audience string       output audience: human (show progress) or agent (summary only) (default "human")
  -b, --background string optional requirement/business context for the scan
  -f, --format string     output format: text or json (default "text")
  --concurrency int       max concurrent file scans (default 8)
  --max-git-procs int     max concurrent git subprocesses (default 16)
  --max-tools int         max tool call rounds per file; only takes effect when greater than template default
  -p, --preview           preview which files will be scanned without running the LLM
  --repo string           root directory of the git repository (default: current dir)
  --resume string         resume from a previous scan session id
  --save-result           persist scan result as JSON + Markdown for the viewer (default true)
  --save-per-file         split output into per-file markdown files under a directory tree mirroring the source tree
  --result-dir string     scan result storage root (env: OCR_REVIEWS_DIR, default: .opencodereview/reviews)
  --result-project string project name/path for persisted scan results
  --rule string           path to JSON file with system review rules
  --timeout int           concurrent task timeout in minutes (default 10)
  --tools string          path to JSON tools config file (default: embedded)`)
}
