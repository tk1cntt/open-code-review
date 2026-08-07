// Package refactor implements `ocr refactor` — full-file refactoring analysis.
// It follows the same architecture as internal/scan but uses refactoring-specific
// templates, rules (ResolveRefactor), and output categories/severities.
package refactor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	allowedext "github.com/alibaba/open-code-review/internal/config/allowlist"
	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/refactor/crossfile"
	"github.com/alibaba/open-code-review/internal/scan"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/stdout"
	"github.com/alibaba/open-code-review/internal/telemetry"
	"github.com/alibaba/open-code-review/internal/tool"
)

// Args bundles all dependencies needed for one refactoring session.
type Args struct {
	RepoDir               string
	Paths                 []string
	Template              template.RefactorTemplate
	SystemRule            rules.Resolver
	FileFilter            *rules.FileFilter
	LLMClient             llm.LLMClient
	Tools                 *tool.Registry
	MainToolDefs          []llm.ToolDef
	CommentCollector      *tool.CommentCollector
	CommentWorkerPool     *llmloop.CommentWorkerPool
	MaxConcurrency        int
	ConcurrentTaskTimeout int
	Model                 string
	Background            string
	GitRunner             *gitcmd.Runner
	Session               *session.SessionHistory
	MaxFileSizeBytes      int64
	SkipPlan              bool
	MaxTokensBudget       int64
	Resume                *session.ResumeState
	// ResumeMode is session.ResumeModeContinue (default) or ResumeModeRestartFailed.
	ResumeMode string
	OnFileDone func(filePath string, comments []model.LlmComment)

	// Multi-file pipeline (ADR F0–F3).
	// Mode: local | cross | full (default local).
	Mode string
	// CrossFileHints enables F0 related-path citations in per-file MAIN_TASK.
	CrossFileHints bool
	// Apply enables F3 write+verify for architect plans that include suggestion_code.
	Apply bool
	// ApplyRunTests runs go test during verify when Apply is true.
	ApplyRunTests bool
}

// Agent orchestrates full-file refactoring analysis. It delegates the per-file
// LLM tool-use loop to llmloop.Runner and owns refactoring-specific concerns
// (file enumeration, template rendering, rule resolution via ResolveRefactor).
type Agent struct {
	args        Args
	items       []model.ScanItem
	currentDate string
	session     *session.SessionHistory
	runner      *llmloop.Runner
	resumeInfo  *scan.ResumeInfo
	reusedCount int64 // number of files reused from previous session (resume only)

	subtaskFailed int64
}

// NewAgent creates a refactor Agent from the given args.
func NewAgent(args Args) *Agent {
	if args.Tools == nil {
		args.Tools = tool.NewRegistry()
	}
	if args.CommentCollector == nil {
		args.CommentCollector = tool.NewCommentCollector()
	}
	if args.Session == nil {
		opts := session.SessionOptions{
			ReviewMode: session.ReviewModeFullScan,
		}
		if args.Resume != nil {
			opts.ResumedFrom = args.Resume.SessionID
		}
		args.Session = session.New(args.RepoDir, "", args.Model, opts)
	}
	a := &Agent{
		args:    args,
		session: args.Session,
	}
	a.runner = llmloop.NewRunner(llmloop.Deps{
		LLMClient:         args.LLMClient,
		Model:             args.Model,
		Template:          toLoopTemplate(args.Template),
		Tools:             args.Tools,
		MainToolDefs:      args.MainToolDefs,
		CommentCollector:  args.CommentCollector,
		CommentWorkerPool: args.CommentWorkerPool,
		Session:           args.Session,
		DiffLookup:        a.lookupDiff,
	})
	return a
}

func toLoopTemplate(t template.RefactorTemplate) template.Template {
	return template.Template{
		MemoryCompressionTask: t.MemoryCompressionTask,
		MaxTokens:             t.MaxTokens,
		MaxToolRequestTimes:   t.MaxToolRequestTimes,
		ReLocationTask:        t.ReLocationTask,
	}
}

func (a *Agent) planEnabled() bool {
	return !a.args.SkipPlan && a.args.Template.PlanTask != nil && len(a.args.Template.PlanTask.Messages) > 0
}

// Session returns the session history.
func (a *Agent) Session() *session.SessionHistory { return a.session }

// SessionID returns the current session id.
func (a *Agent) SessionID() string {
	if a == nil || a.session == nil {
		return ""
	}
	return a.session.SessionID
}

// FilesReviewed returns the number of newly-reviewed (non-reused) items dispatched
// in this refactor run. On resume, this excludes reused results from the previous
// session; use TotalFilesReviewed for the combined count.
func (a *Agent) FilesReviewed() int64 {
	n := int64(len(a.items))
	if a.reusedCount > n {
		return 0
	}
	return n - a.reusedCount
}

// FilesReused returns the number of files whose results were reused from a
// previous session. Zero for non-resume runs.
func (a *Agent) FilesReused() int64 { return a.reusedCount }

// TotalFilesReviewed returns the total count of items (new + reused).
func (a *Agent) TotalFilesReviewed() int64 { return int64(len(a.items)) }

// SubtaskFailed returns the number of files whose subtask failed.
func (a *Agent) SubtaskFailed() int64 { return atomic.LoadInt64(&a.subtaskFailed) }

// Diffs returns the scanned items adapted to model.Diff form.
func (a *Agent) Diffs() []model.Diff {
	out := make([]model.Diff, len(a.items))
	for i := range a.items {
		out[i] = *a.items[i].AsDiff()
	}
	return out
}

// ProjectSummary returns empty — refactoring does not have a summary phase.
func (a *Agent) ProjectSummary() string { return "" }

// BudgetExceeded always returns false for refactor.
func (a *Agent) BudgetExceeded() bool { return false }

// RunManifest returns nil because refactor is outside v1 run manifest coverage.
func (a *Agent) RunManifest() *session.RunManifest { return nil }

// ResumeInfo returns resume metadata.
func (a *Agent) ResumeInfo() *scan.ResumeInfo {
	if a.resumeInfo == nil {
		return nil
	}
	info := *a.resumeInfo
	return &info
}

// TotalTokensUsed / TotalInputTokens / ... delegate to the runner.
func (a *Agent) TotalTokensUsed() int64      { return a.runner.TotalTokensUsed() }
func (a *Agent) TotalInputTokens() int64     { return a.runner.TotalInputTokens() }
func (a *Agent) TotalOutputTokens() int64    { return a.runner.TotalOutputTokens() }
func (a *Agent) TotalCacheReadTokens() int64 { return a.runner.TotalCacheReadTokens() }
func (a *Agent) TotalCacheWriteTokens() int64 {
	return a.runner.TotalCacheWriteTokens()
}

// Warnings returns the warnings recorded by the LLM runner.
func (a *Agent) Warnings() []llmloop.AgentWarning { return a.runner.Warnings() }

// ToolCalls returns per-tool call counts.
func (a *Agent) ToolCalls() map[string]int64 { return a.runner.ToolCalls() }

func (a *Agent) recordWarning(warningType, file, message string) {
	a.runner.RecordWarning(warningType, file, message)
}

// Run executes the full refactoring pipeline: enumerate → filter → token-filter →
// dispatch (local and/or cross-file) → collect comments.
func (a *Agent) Run(ctx context.Context) ([]model.LlmComment, error) {
	mode := crossfile.ParseMode(a.args.Mode)
	if mode == crossfile.ModeLocal || mode == crossfile.ModeFull {
		if len(a.args.Template.MainTask.Messages) == 0 {
			return nil, fmt.Errorf("refactor template MAIN_TASK is missing or empty")
		}
	}

	ctx, span := telemetry.StartSpan(ctx, "refactor.enumerate")
	provider := scan.NewProvider(a.args.RepoDir, a.args.Paths, a.args.GitRunner, a.args.MaxFileSizeBytes)
	items, err := provider.Enumerate(ctx)
	if err != nil {
		span.End()
		return nil, fmt.Errorf("enumerate files: %w", err)
	}
	telemetry.SetAttr(span, "files.enumerated", len(items))
	span.End()

	a.items = items
	a.injectScanContentMap()
	if a.args.Tools != nil {
		a.args.Tools.Freeze()
	}

	totalDiscovered := len(a.items)
	a.items = a.filterScanItems(a.items)
	a.items = a.filterLargeScans(a.items)
	a.items = a.applyResume(a.items)

	if a.args.OnFileDone != nil && a.args.Resume != nil {
		for _, item := range a.args.Resume.Items {
			a.args.OnFileDone(item.FilePath, item.Comments)
		}
	}

	reviewable := len(a.items)
	fmt.Fprintf(stdout.Writer(), "[ocr] refactor: %d file(s) discovered, analyzing %d in %s (mode=%s)\n",
		totalDiscovered, reviewable, a.args.RepoDir, mode)

	if reviewable == 0 {
		fmt.Fprintln(stdout.Writer(), "[ocr] No analyzable files. Skipping refactor.")
		telemetry.Event(ctx, "refactor.no.files")
		comments := a.args.CommentCollector.Comments()
		a.session.Finalize()
		return comments, nil
	}

	est := scan.EstimateCost(a.items, a.planEnabled(), false, false)
	fmt.Fprintf(stdout.Writer(), "[ocr] estimated cost: %s\n", est)
	if a.args.MaxTokensBudget > 0 {
		fmt.Fprintf(stdout.Writer(), "[ocr] token budget: %s (dispatch stops once exceeded)\n",
			scan.HumanTokens(a.args.MaxTokensBudget))
		if est.TotalTokens > a.args.MaxTokensBudget {
			fmt.Fprintf(stdout.Writer(), "[ocr] WARNING: estimate (%s) exceeds budget (%s); refactor will stop partway\n",
				scan.HumanTokens(est.TotalTokens), scan.HumanTokens(a.args.MaxTokensBudget))
		}
	}

	a.currentDate = time.Now().Format("2006-01-02 15:04")
	telemetry.Event(ctx, "refactor.started",
		telemetry.AnyToAttr("file.count", totalDiscovered),
		telemetry.AnyToAttr("review.count", reviewable),
		telemetry.AnyToAttr("est.total.tokens", est.TotalTokens),
		telemetry.AnyToAttr("repo.dir", a.args.RepoDir),
		telemetry.AnyToAttr("mode", string(mode)),
		telemetry.AnyToAttr("cross_file_hints", a.args.CrossFileHints),
		telemetry.AnyToAttr("apply", a.args.Apply))
	telemetry.RecordFilesReviewed(ctx, int64(reviewable))

	var comments []model.LlmComment
	var runErr error

	// Phase L — per-file
	if mode == crossfile.ModeLocal || mode == crossfile.ModeFull {
		comments, runErr = a.dispatchSubtasks(ctx)
		if runErr != nil {
			a.session.Finalize()
			return comments, runErr
		}
	}

	// Phase X — cross-file detect + architect (+ optional apply)
	if mode == crossfile.ModeCross || mode == crossfile.ModeFull {
		crossComments, xerr := a.runCrossFilePhase(ctx)
		if xerr != nil {
			// Cross failures are non-fatal if local already produced comments.
			if mode == crossfile.ModeCross {
				runErr = xerr
			} else {
				fmt.Fprintf(stdout.Writer(), "[ocr] WARNING: cross-file phase error: %v\n", xerr)
				a.recordWarning("cross_file_phase_error", "(cross_file_phase)", xerr.Error())
			}
		}
		fmt.Fprintf(stdout.Writer(), "[ocr] cross-file phase: %d comment(s)\n", len(crossComments))
		comments = a.args.CommentCollector.Comments()

		// Fire OnFileDone only for cross-file findings. Per-file dispatch already
		// called OnFileDone for local results; re-emitting the full comment set
		// would double-report in ModeFull. In ModeCross there was no local pass,
		// so this is the sole OnFileDone signal.
		if a.args.OnFileDone != nil && len(crossComments) > 0 {
			byPath := map[string][]model.LlmComment{}
			for _, cm := range crossComments {
				byPath[cm.Path] = append(byPath[cm.Path], cm)
			}
			for path, pathComments := range byPath {
				a.args.OnFileDone(path, pathComments)
			}
		}
	}

	if len(comments) > 0 {
		telemetry.RecordCommentsGenerated(ctx, int64(len(comments)))
	}

	a.session.Finalize()
	return comments, runErr
}

// lookupDiff returns the synthetic Diff for a path.
func (a *Agent) lookupDiff(path string) *model.Diff {
	for i := range a.items {
		if a.items[i].Path == path {
			return a.items[i].AsDiff()
		}
	}
	return nil
}

func (a *Agent) injectScanContentMap() {
	m := make(map[string]string, len(a.items))
	for i := range a.items {
		it := &a.items[i]
		if it.Path != "" {
			m[it.Path] = it.Content
		}
	}
	dm := tool.NewDiffMap(m)
	if p, ok := a.args.Tools.Get(tool.FileReadDiff.Name()); ok {
		if frd, ok := p.(*tool.FileReadDiffProvider); ok {
			frd.SetDiffMap(dm)
		}
	}
}

func (a *Agent) filterScanItems(items []model.ScanItem) []model.ScanItem {
	// On resume, suppress per-file Skipping noise from whole-repo enumerate.
	quiet := a.args.Resume != nil
	var kept []model.ScanItem
	skipped := 0
	for _, it := range items {
		if reason := a.whyExcluded(it); reason != model.ExcludeNone {
			if !quiet {
				if it.IsBinary {
					fmt.Fprintf(stdout.Writer(), "[ocr] Skipping %s — binary file\n", it.Path)
				} else {
					fmt.Fprintf(stdout.Writer(), "[ocr] Skipping %s — filtered by path/extension rules\n", it.Path)
				}
			}
			skipped++
			continue
		}
		kept = append(kept, it)
	}
	if skipped > 0 && !quiet {
		fmt.Fprintf(stdout.Writer(), "[ocr] Filtered %d file(s) by include/exclude rules\n", skipped)
	}
	return kept
}

func (a *Agent) filterLargeScans(items []model.ScanItem) []model.ScanItem {
	limit := llmloop.PromptTokenLimit(a.args.Template.MaxTokens)
	if limit <= 0 {
		return items
	}
	quiet := a.args.Resume != nil
	var kept []model.ScanItem
	skipped := 0
	for _, it := range items {
		tokens := llm.CountTokens(it.Content)
		if tokens > limit {
			if !quiet {
				fmt.Fprintf(stdout.Writer(), "[ocr] Skipping %s (~%d tokens exceeds 80%% of max_tokens(%d))\n",
					it.Path, tokens, a.args.Template.MaxTokens)
			}
			skipped++
			continue
		}
		kept = append(kept, it)
	}
	if skipped > 0 && !quiet {
		fmt.Fprintf(stdout.Writer(), "[ocr] Pre-filtered %d file(s) exceeding 80%% of max_tokens\n", skipped)
	}
	return kept
}

func (a *Agent) applyResume(items []model.ScanItem) []model.ScanItem {
	resume := a.args.Resume
	if resume == nil {
		return items
	}

	// Full-scan resume must not treat every unscoped repo file as "new work".
	// Only files recorded in the prior session (done / failed / checkpoint /
	// partial) are considered; everything else is skipped.
	scoped := resume.HasSessionScope()

	toDispatch := make([]model.ScanItem, 0, len(items))
	var reused, continuing, coldRetry, newFiles int64
	opts := session.PrepareOpts{
		ResumeMode:   session.NormalizeResumeMode(a.args.ResumeMode),
		CurrentModel: a.args.Model,
		TemplateHash: a.templateHash(),
	}
	for _, it := range items {
		fingerprint := scan.ScanItemFingerprint(it.Path)
		if scoped && !resume.InSession(fingerprint) {
			continue
		}
		start := session.PrepareFileStart(resume, fingerprint, it.Path, opts)
		if start.Mode == session.ModeReuse {
			for _, cm := range start.SeedComments {
				a.args.CommentCollector.Add(cm)
			}
			a.session.RecordReviewItemReused(it.Path, it.Path, it.Path, fingerprint, resume.SessionID, start.SeedComments)
			reused++
			continue
		}
		if start.Mode == session.ModeContinue {
			fmt.Fprintf(stdout.Writer(), "[ocr] Resume continuing mid-file: %s (round %d)\n", it.Path, start.Round)
			continuing++
		} else if _, wasFailed := resume.FailedFiles[fingerprint]; wasFailed {
			fmt.Fprintf(stdout.Writer(), "[ocr] Resume retrying previously failed file (cold): %s\n", it.Path)
			coldRetry++
		} else {
			// Only reached when session has no scope (legacy empty) or file is
			// in-session via partial without failed/checkpoint metadata.
			newFiles++
		}
		toDispatch = append(toDispatch, it)
	}

	rerun := int64(len(toDispatch))
	a.reusedCount = reused
	a.resumeInfo = &scan.ResumeInfo{
		ResumedFrom:   resume.SessionID,
		ReusedFiles:   reused,
		RerunFiles:    rerun,
		PreviousModel: resume.Model,
		CurrentModel:  a.args.Model,
	}
	if resume.Model != "" && resume.Model != a.args.Model {
		fmt.Fprintf(stdout.Writer(), "[ocr] Warning: resume session %q used model %q, current model is %q\n",
			resume.SessionID, resume.Model, a.args.Model)
	}
	fmt.Fprintln(stdout.Writer(), session.FormatResumeSummary(resume.SessionID, reused, continuing, coldRetry, newFiles))
	return toDispatch
}

func (a *Agent) templateHash() string {
	return session.ComputeTemplateHashFrom(&a.args.Template)
}

func (a *Agent) prepareFileStart(it model.ScanItem) session.FileStart {
	fingerprint := scan.ScanItemFingerprint(it.Path)
	// Prefer live same-run checkpoint over cross-run resume state.
	if cp, ok := a.session.LastConversationCheckpoint(fingerprint); ok && len(cp.Messages) > 0 {
		return session.FileStart{
			Mode:         session.ModeContinue,
			Messages:     cp.Messages,
			PlanGuidance: cp.PlanGuidance,
			SeedComments: cp.Comments,
			Round:        cp.Round,
			Checkpoint:   cp,
		}
	}
	if a.args.Resume == nil {
		return session.FileStart{Mode: session.ModeCold}
	}
	return session.PrepareFileStart(a.args.Resume, fingerprint, it.Path, session.PrepareOpts{
		ResumeMode:   session.NormalizeResumeMode(a.args.ResumeMode),
		CurrentModel: a.args.Model,
		TemplateHash: a.templateHash(),
	})
}

func (a *Agent) whyExcluded(it model.ScanItem) model.ExcludeReason {
	if it.IsBinary {
		return model.ExcludeBinary
	}
	path := it.Path
	if a.args.FileFilter != nil && a.args.FileFilter.IsUserExcluded(path) {
		return model.ExcludeUserRule
	}
	if a.args.FileFilter != nil && a.args.FileFilter.HasInclude() && a.args.FileFilter.IsUserIncluded(path) {
		return model.ExcludeNone
	}
	ext := scan.ExtFromPath(path)
	if ext != "" && !allowedext.IsAllowedExt(ext) {
		return model.ExcludeExtension
	}
	if allowedext.IsExcludedPath(path) {
		return model.ExcludeDefaultPath
	}
	return model.ExcludeNone
}

func (a *Agent) dispatchSubtasks(ctx context.Context) ([]model.LlmComment, error) {
	startTime := time.Now()
	defer func() {
		telemetry.RecordReviewDuration(ctx, time.Since(startTime))
	}()

	if len(a.items) == 0 {
		return []model.LlmComment{}, nil
	}

	atomic.StoreInt64(&a.subtaskFailed, 0)

	strategy := a.resolveBatchStrategy()
	batches := scan.GroupBatches(a.items, strategy, a.args.Template.BatchSize)
	fmt.Fprintf(stdout.Writer(), "[ocr] refactor dispatch: %d batch(es) by %s strategy\n", len(batches), strategy)

	var dispatched int64
	for bi, batch := range batches {
		if err := ctx.Err(); err != nil {
			return a.args.CommentCollector.Comments(), err
		}

		n, budgetHit, err := a.dispatchBatch(ctx, bi, batch)
		dispatched += n
		if err != nil {
			return a.args.CommentCollector.Comments(), err
		}

		if a.args.CommentWorkerPool != nil {
			a.args.CommentWorkerPool.Await()
		}

		if budgetHit {
			break
		}
	}

	comments := a.args.CommentCollector.Comments()
	failed := atomic.LoadInt64(&a.subtaskFailed)
	if failed > 0 && failed == dispatched {
		// Still return any partial findings collected before timeout/failure so
		// the CLI can persist and display them.
		return comments, fmt.Errorf("all %d file refactor(s) failed — check your LLM configuration and API key", dispatched)
	}
	return comments, nil
}

func (a *Agent) resolveBatchStrategy() scan.BatchStrategy {
	s := strings.ToLower(strings.TrimSpace(a.args.Template.BatchStrategy))
	switch scan.BatchStrategy(s) {
	case scan.BatchByLanguage:
		return scan.BatchByLanguage
	case scan.BatchByDirectory:
		return scan.BatchByDirectory
	default:
		return scan.BatchNone
	}
}

func (a *Agent) dispatchBatch(ctx context.Context, batchIdx int, batch []model.ScanItem) (int64, bool, error) {
	concurrency := a.args.MaxConcurrency
	if concurrency <= 0 {
		concurrency = 8
	}
	sem := make(chan struct{}, concurrency)
	timeout := time.Duration(a.args.ConcurrentTaskTimeout) * time.Minute

	var (
		wg         sync.WaitGroup
		dispatched int64
		budgetHit  bool
	)

	for i := range batch {
		if a.tokenBudgetHit(batch[i], batchIdx) {
			budgetHit = true
			break
		}

		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return dispatched, budgetHit, ctx.Err()
		}

		dispatched++
		wg.Add(1)
		go func(it model.ScanItem) {
			defer wg.Done()
			defer func() { <-sem }()

			fingerprint := scan.ScanItemFingerprint(it.Path)
			err := a.executeWithRetries(ctx, it, batchIdx, timeout)
			comments := a.args.CommentCollector.CommentsForPath(it.Path)

			if err != nil {
				a.recordFileFailure(ctx, it, fingerprint, batchIdx, err, comments)
				return
			}
			a.recordFileSuccess(it, fingerprint, comments)
			if a.args.Apply && a.args.Mode == "local" {
				a.applyFileComments(ctx, it.Path, comments)
			}
		}(batch[i])
	}

	wg.Wait()
	return dispatched, budgetHit, nil
}

func (a *Agent) executeSubtask(ctx context.Context, it model.ScanItem, start session.FileStart) error {
	ctx, span := telemetry.StartSpan(ctx, "refactor.subtask."+it.Path)
	defer span.End()
	telemetry.SetAttr(span, "file.path", it.Path)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	fingerprint := scan.ScanItemFingerprint(it.Path)
	a.session.RecordFileStarted(it.Path, fingerprint, session.PhaseMain)
	defer llmloop.ClearCheckpointHook(a.runner)

	// Seed prior findings (resume / same-run continue); collector dedupes.
	for _, cm := range start.SeedComments {
		a.args.CommentCollector.Add(cm)
	}

	rule := ""
	if a.args.SystemRule != nil {
		rule = a.args.SystemRule.ResolveRefactor(strings.ToLower(it.Path))
		// Phase 1/2 telemetry: rule payload size and composition layers.
		if statsProvider, ok := a.args.SystemRule.(interface {
			RefactorPayloadStats(path string) rules.RefactorRulePayloadStats
		}); ok {
			st := statsProvider.RefactorPayloadStats(strings.ToLower(it.Path))
			telemetry.SetAttr(span, "refactor.rule.bytes", st.Bytes)
			telemetry.SetAttr(span, "refactor.rule.standalone", st.Standalone)
			if st.Profile != "" {
				telemetry.SetAttr(span, "refactor.rule.profile", st.Profile)
			}
			if len(st.Layers) > 0 {
				telemetry.SetAttr(span, "refactor.rule.layers", strings.Join(st.Layers, ","))
			}
			telemetry.Event(ctx, "refactor.rule.payload",
				telemetry.AnyToAttr("file.path", it.Path),
				telemetry.AnyToAttr("rule.bytes", st.Bytes),
				telemetry.AnyToAttr("rule.standalone", st.Standalone),
				telemetry.AnyToAttr("rule.profile", st.Profile),
				telemetry.AnyToAttr("rule.layers", strings.Join(st.Layers, ",")))
		}
	}

	var (
		messages        []llm.Message
		planGuidance    string
		completedRounds int
	)
	if start.Mode == session.ModeContinue && len(start.Messages) > 0 {
		messages = start.Messages
		planGuidance = start.PlanGuidance
		completedRounds = start.Round
		fmt.Fprintf(stdout.Writer(), "[ocr] Mid-file continue %s from round %d (%d messages)\n",
			it.Path, completedRounds, len(messages))
		telemetry.Event(ctx, "refactor.midfile.continue",
			telemetry.AnyToAttr("file.path", it.Path),
			telemetry.AnyToAttr("round", completedRounds),
			telemetry.AnyToAttr("messages", len(messages)))
	} else {
		// Cold start: reuse plan guidance from partial/checkpoint when present.
		if start.PlanGuidance != "" {
			planGuidance = start.PlanGuidance
		} else {
			planGuidance = a.maybeRunPlan(ctx, it, rule)
		}
		messages = a.renderMessages(it, rule, planGuidance)
	}

	tokenCount := llmloop.CountMessagesTokens(messages)
	maxAllowed := a.args.Template.MaxTokens
	tokenLimit := llmloop.PromptTokenLimit(maxAllowed)
	if tokenCount > tokenLimit {
		msg := fmt.Sprintf("prompt tokens (%d) exceed %d%% of max_tokens(%d)", tokenCount, 80, maxAllowed)
		fmt.Fprintf(stdout.Writer(), "[ocr] WARNING: %s for %s\n", msg, it.Path)
		a.recordWarning("token_threshold_exceeded", it.Path, msg)
		telemetry.Event(ctx, "token.threshold.exceeded",
			telemetry.AnyToAttr("file.path", it.Path),
			telemetry.AnyToAttr("tokens", tokenCount),
			telemetry.AnyToAttr("max_tokens", maxAllowed))
		return nil
	}

	llmloop.BindSessionCheckpoint(a.runner, a.session, fingerprint, planGuidance, a.args.Model, a.templateHash())
	a.runner.SetCompletedRounds(completedRounds)
	_, _, err := a.runner.RunPerFile(ctx, messages, it.Path)
	return err
}

// executeWithRetries runs executeSubtask with up to 3 attempts for transient
// failures only (timeout, 429/502/503, network blips). Permanent errors
// (auth, bad request, task failed, cancel) fail immediately so they stay
// visible and do not amplify rate limits.
// On each retry the per-file timeout is extended (attempt+1 * base timeout).
// When a conversation checkpoint exists, retries continue mid-file instead of
// cold-restarting the tool loop.
func (a *Agent) executeWithRetries(ctx context.Context, it model.ScanItem, batchIdx int, timeout time.Duration) error {
	const maxRetries = 3
	var lastErr error
	fingerprint := scan.ScanItemFingerprint(it.Path)
	start := a.prepareFileStart(it)

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Prefer continue-from-checkpoint over cold restart (Phase 2a).
			if cp, ok := a.session.LastConversationCheckpoint(fingerprint); ok && len(cp.Messages) > 0 {
				start = session.FileStart{
					Mode:         session.ModeContinue,
					Messages:     cp.Messages,
					PlanGuidance: cp.PlanGuidance,
					SeedComments: cp.Comments,
					Round:        cp.Round,
					Checkpoint:   cp,
				}
				if reason := llm.RetryReason(lastErr); reason == llm.RetryReasonRateLimit {
					select {
					case <-time.After(2 * time.Second):
					case <-ctx.Done():
					}
				}
				fmt.Fprintf(stdout.Writer(), "[ocr] Refactor subtask continuing from checkpoint for %s (batch #%d, attempt %d/%d, round %d)\n",
					it.Path, batchIdx, attempt+1, maxRetries, cp.Round)
			} else {
				a.args.CommentCollector.RemoveByPath(it.Path)
				start = session.FileStart{Mode: session.ModeCold}
			}
		}

		var fileCtx context.Context
		var cancel context.CancelFunc
		if timeout > 0 {
			retryTimeout := timeout * time.Duration(attempt+1)
				// When continuing from a mid-file checkpoint the remaining
				// work is less than a full cold start — keep the base timeout
				// instead of doubling it each retry.
				if start.Mode == session.ModeContinue && retryTimeout > timeout {
					retryTimeout = timeout
				}
			fileCtx, cancel = context.WithTimeout(ctx, retryTimeout)
		} else {
			fileCtx = ctx
		}

		lastErr = a.executeSubtask(fileCtx, it, start)
		if cancel != nil {
			cancel()
		}

		if lastErr == nil {
			return nil
		}

		reason := llm.RetryReason(lastErr)
		shouldRetry := llm.IsRetryableLLMError(lastErr)
		if !shouldRetry || attempt == maxRetries-1 {
			if shouldRetry {
				fmt.Fprintf(stdout.Writer(), "[ocr] Refactor subtask exhausted retries for %s (batch #%d, %d attempt(s), reason=%s): %v\n",
					it.Path, batchIdx, maxRetries, reason, lastErr)
			} else {
				fmt.Fprintf(stdout.Writer(), "[ocr] Refactor subtask error for %s (batch #%d, reason=%s, not retrying): %v\n",
					it.Path, batchIdx, reason, lastErr)
			}
			break
		}

		switch reason {
		case llm.RetryReasonTimeout:
			fmt.Fprintf(stdout.Writer(), "[ocr] Refactor subtask timeout for %s (batch #%d, attempt %d/%d), retrying with extended timeout...\n",
				it.Path, batchIdx, attempt+1, maxRetries)
		case llm.RetryReasonRateLimit:
			fmt.Fprintf(stdout.Writer(), "[ocr] Refactor subtask rate-limited (429/502/503) for %s (batch #%d, attempt %d/%d), retrying...\n",
				it.Path, batchIdx, attempt+1, maxRetries)
		default:
			fmt.Fprintf(stdout.Writer(), "[ocr] Refactor subtask retryable error for %s (batch #%d, attempt %d/%d, reason=%s): %v\n",
				it.Path, batchIdx, attempt+1, maxRetries, reason, lastErr)
		}
	}
	return lastErr
}

// tokenBudgetHit checks whether processing item would exceed the token budget.
func (a *Agent) tokenBudgetHit(it model.ScanItem, batchIdx int) bool {
	if a.args.MaxTokensBudget <= 0 {
		return false
	}
	used := a.runner.TotalTokensUsed()
	projected := used + scan.EstimateFileTokens(it, a.planEnabled())
	if projected <= a.args.MaxTokensBudget {
		return false
	}
	fmt.Fprintf(stdout.Writer(), "[ocr] token budget reached (used %s + next-file est ≈ %s > budget %s) — skipping %s and remaining files\n",
		scan.HumanTokens(used), scan.HumanTokens(projected), scan.HumanTokens(a.args.MaxTokensBudget), it.Path)
	a.recordWarning("token_budget_reached", it.Path,
		fmt.Sprintf("stopped in batch #%d: used %d tokens + next-file estimate exceeds budget %d", batchIdx, used, a.args.MaxTokensBudget))
	return true
}

// recordFileFailure increments the failure counter, logs, wires telemetry, and records in session.
func (a *Agent) recordFileFailure(ctx context.Context, it model.ScanItem, fingerprint string, batchIdx int, err error, comments []model.LlmComment) {
	atomic.AddInt64(&a.subtaskFailed, 1)
	reason := llm.RetryReason(err)
	status := session.CheckpointFailed
	if reason == llm.RetryReasonTimeout {
		status = session.CheckpointTimedOut
	}
	planGuidance := ""
	round := 0
	if cp, ok := a.session.LastConversationCheckpoint(fingerprint); ok {
		planGuidance = cp.PlanGuidance
		round = cp.Round
		a.session.MarkCheckpointStopped(fingerprint, status, err.Error())
	}
	a.session.RecordReviewItemPartial(it.Path, fingerprint, session.PhaseMain, planGuidance, err.Error(), round, comments)
	a.session.RecordReviewItemFailed(it.Path, it.Path, it.Path, fingerprint, err.Error(), comments)
	if a.args.OnFileDone != nil {
		a.args.OnFileDone(it.Path, comments)
	}
	fmt.Fprintf(stdout.Writer(), "[ocr] Refactor subtask error for %s (batch #%d, reason=%s): %v\n", it.Path, batchIdx, reason, err)
	telemetry.ErrorEvent(context.WithoutCancel(ctx), "refactor.subtask.error", err,
		telemetry.AnyToAttr("file.path", it.Path),
		telemetry.AnyToAttr("batch.index", batchIdx),
		telemetry.AnyToAttr("error.reason", reason))
	a.recordWarning("refactor_subtask_error", it.Path, err.Error())
}

// recordFileSuccess records the item as done in session and fires onFileDone callback.
func (a *Agent) recordFileSuccess(it model.ScanItem, fingerprint string, comments []model.LlmComment) {
	a.session.RecordReviewItemDone(it.Path, it.Path, it.Path, fingerprint, comments)
	if a.args.OnFileDone != nil {
		a.args.OnFileDone(it.Path, comments)
	}
}

// applyFileComments applies suggestion_code from per-file comments using
// backup+verify+rollback. Runs in the per-file dispatch goroutine. Best-effort:
// verify failures roll back the file and log a warning but never fail the run.
// Filtering (empty suggestion_code, empty path, invalid StartLine, overlapping
// ranges) is handled by ApplyComments which logs a skip reason for each
// non-actionable comment via res.Skipped.
func (a *Agent) applyFileComments(ctx context.Context, relPath string, comments []model.LlmComment) {
	res := crossfile.ApplyComments(a.args.RepoDir, comments, a.args.ApplyRunTests)
	for _, m := range res.Messages {
		fmt.Fprintf(stdout.Writer(), "[ocr] apply %s: %s\n", relPath, m)
	}
	for _, s := range res.Skipped {
		fmt.Fprintf(stdout.Writer(), "[ocr] apply %s: skip %s\n", relPath, s)
	}
	if res.AppliedCount > 0 && res.Verify.OK && !res.RolledBack {
		fmt.Fprintf(stdout.Writer(), "[ocr] apply %d/%d suggestion(s) to %s\n", res.AppliedCount, len(comments), relPath)
	} else if len(res.Written) > 0 {
		fmt.Fprintf(stdout.Writer(), "[ocr] WARNING: apply+verify failed for %s (rolled back)\n", relPath)
	}
}

func (a *Agent) maybeRunPlan(ctx context.Context, it model.ScanItem, rule string) string {
	const noPlan = "(no pre-analysis plan; analyze the entire file as usual)"

	if !a.planEnabled() {
		return noPlan
	}
	pt := a.args.Template.PlanTask

	messages := make([]llm.Message, 0, len(pt.Messages))
	for _, m := range pt.Messages {
		content := a.fillPlaceholders(m.Content, it, rule, nil)
		messages = append(messages, llm.NewTextMessage(m.Role, content))
	}

	// Nested timeout on the parent ctx: plan deadline only cancels planCtx,
	// leaving remaining fileCtx budget for main review. Parent cancel (Ctrl+C)
	// still propagates — do not use WithoutCancel here.
	if ctx.Err() != nil {
		return noPlan
	}
	planCtx, planCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer planCancel()

	fs := a.session.GetOrCreateFileSession(it.Path)
	rec := fs.AppendTaskRecord(session.PlanTask, messages)
	startTime := time.Now()

	resp, err := a.args.LLMClient.CompletionsWithCtx(planCtx, llm.ChatRequest{
		Model:     a.args.Model,
		Messages:  messages,
		MaxTokens: a.args.Template.MaxTokens,
	})
	if err != nil {
		rec.SetError(err, time.Since(startTime))
		fmt.Fprintf(stdout.Writer(), "[ocr] refactor plan failed for %s: %v (falling back to plan-less)\n", it.Path, err)
		return noPlan
	}
	rec.SetResponse(resp, time.Since(startTime))
	a.runner.RecordUsage(resp.Usage)

	guidance := formatPlanGuidance(resp.Content())
	if guidance == "" {
		return noPlan
	}
	return guidance
}

func formatPlanGuidance(raw string) string {
	stripped := llmloop.StripMarkdownFences(raw)
	stripped = strings.TrimSpace(stripped)
	if stripped == "" {
		return ""
	}

	var plan struct {
		Summary     string `json:"summary"`
		Checkpoints []struct {
			Focus string `json:"focus"`
			Lines string `json:"lines,omitempty"`
			Why   string `json:"why,omitempty"`
		} `json:"checkpoints"`
	}
	if err := json.Unmarshal([]byte(stripped), &plan); err != nil {
		return stripped
	}

	var sb strings.Builder
	if plan.Summary != "" {
		sb.WriteString("**Summary**: ")
		sb.WriteString(plan.Summary)
		sb.WriteString("\n\n")
	}
	if len(plan.Checkpoints) == 0 {
		return strings.TrimRight(sb.String(), "\n")
	}
	sb.WriteString("**Focus areas (give these extra attention; not exhaustive):**\n")
	for i, cp := range plan.Checkpoints {
		fmt.Fprintf(&sb, "%d. `%s`", i+1, cp.Focus)
		if cp.Lines != "" {
			fmt.Fprintf(&sb, " (lines %s)", cp.Lines)
		}
		if cp.Why != "" {
			fmt.Fprintf(&sb, " — %s", cp.Why)
		}
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// fillPlaceholders replaces standard template variables in content.
func (a *Agent) fillPlaceholders(content string, it model.ScanItem, rule string, extra map[string]string) string {
	content = strings.ReplaceAll(content, "{{current_system_date_time}}", a.currentDate)
	content = strings.ReplaceAll(content, "{{current_file_path}}", it.Path)
	content = strings.ReplaceAll(content, "{{system_rule}}", rule)
	content = strings.ReplaceAll(content, "{{file_content}}", it.Content)
	for k, v := range extra {
		content = strings.ReplaceAll(content, k, v)
	}
	return content
}

func (a *Agent) renderMessages(it model.ScanItem, rule, planGuidance string) []llm.Message {
	rawMsgs := a.args.Template.MainTask.Messages
	messages := make([]llm.Message, 0, len(rawMsgs))
	hints := ""
	if a.args.CrossFileHints {
		hints = crossFileHintsBlock
	}
	for _, m := range rawMsgs {
		extra := map[string]string{
			"{{plan_guidance}}":          planGuidance,
			"{{requirement_background}}": a.args.Background,
			"{{cross_file_hints}}":       hints,
		}
		content := a.fillPlaceholders(m.Content, it, rule, extra)
		messages = append(messages, llm.NewTextMessage(m.Role, content))
	}
	return messages
}

// Preview returns the list of files that would be analyzed without invoking the LLM.
func (a *Agent) Preview(ctx context.Context) (*model.Preview, error) {
	provider := scan.NewProvider(a.args.RepoDir, a.args.Paths, a.args.GitRunner, a.args.MaxFileSizeBytes)
	items, err := provider.Enumerate(ctx)
	if err != nil {
		return nil, fmt.Errorf("enumerate files: %w", err)
	}

	a.items = items
	a.items = a.filterScanItems(a.items)
	a.items = a.filterLargeScans(a.items)

	result := &model.Preview{
		TotalFiles: len(items),
		Entries:    make([]model.PreviewEntry, 0, len(a.items)),
	}

	for _, it := range a.items {
		entry := model.PreviewEntry{
			Path:       it.Path,
			Status:     "scan",
			Insertions: int64(it.LineCount),
		}
		reason := a.whyExcluded(it)
		entry.WillReview = reason == model.ExcludeNone
		entry.ExcludeReason = reason
		if entry.WillReview {
			result.ReviewableCount++
			result.TotalInsertions += entry.Insertions
		} else {
			result.ExcludedCount++
		}
		result.Entries = append(result.Entries, entry)
	}
	return result, nil
}


