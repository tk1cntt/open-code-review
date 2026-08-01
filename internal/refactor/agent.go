// Package refactor implements `ocr refactor` — full-file refactoring analysis.
// It follows the same architecture as internal/scan but uses refactoring-specific
// templates, rules (ResolveRefactor), and output categories/severities.
package refactor

import (
	"context"
	"encoding/json"
	"errors"
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
	OnFileDone            func(filePath string, comments []model.LlmComment)
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

// FilesReviewed returns the number of items included.
func (a *Agent) FilesReviewed() int64 { return int64(len(a.items)) }

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
// dispatch one subtask per file → collect comments.
func (a *Agent) Run(ctx context.Context) ([]model.LlmComment, error) {
	if len(a.args.Template.MainTask.Messages) == 0 {
		return nil, fmt.Errorf("refactor template MAIN_TASK is missing or empty")
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
	a.args.Tools.Freeze()

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
	fmt.Fprintf(stdout.Writer(), "[ocr] refactor: %d file(s) discovered, analyzing %d in %s\n",
		totalDiscovered, reviewable, a.args.RepoDir)

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
		telemetry.AnyToAttr("repo.dir", a.args.RepoDir))
	telemetry.RecordFilesReviewed(ctx, int64(reviewable))

	comments, err := a.dispatchSubtasks(ctx)
	if len(comments) > 0 {
		telemetry.RecordCommentsGenerated(ctx, int64(len(comments)))
	}

	a.session.Finalize()
	return comments, err
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
	var kept []model.ScanItem
	skipped := 0
	for _, it := range items {
		if reason := a.whyExcluded(it); reason != model.ExcludeNone {
			if it.IsBinary {
				fmt.Fprintf(stdout.Writer(), "[ocr] Skipping %s — binary file\n", it.Path)
			} else {
				fmt.Fprintf(stdout.Writer(), "[ocr] Skipping %s — filtered by path/extension rules\n", it.Path)
			}
			skipped++
			continue
		}
		kept = append(kept, it)
	}
	if skipped > 0 {
		fmt.Fprintf(stdout.Writer(), "[ocr] Filtered %d file(s) by include/exclude rules\n", skipped)
	}
	return kept
}

func (a *Agent) filterLargeScans(items []model.ScanItem) []model.ScanItem {
	limit := llmloop.PromptTokenLimit(a.args.Template.MaxTokens)
	if limit <= 0 {
		return items
	}
	var kept []model.ScanItem
	skipped := 0
	for _, it := range items {
		tokens := llm.CountTokens(it.Content)
		if tokens > limit {
			fmt.Fprintf(stdout.Writer(), "[ocr] Skipping %s (~%d tokens exceeds 80%% of max_tokens(%d))\n",
				it.Path, tokens, a.args.Template.MaxTokens)
			skipped++
			continue
		}
		kept = append(kept, it)
	}
	if skipped > 0 {
		fmt.Fprintf(stdout.Writer(), "[ocr] Pre-filtered %d file(s) exceeding 80%% of max_tokens\n", skipped)
	}
	return kept
}

func (a *Agent) applyResume(items []model.ScanItem) []model.ScanItem {
	resume := a.args.Resume
	if resume == nil {
		return items
	}

	toDispatch := make([]model.ScanItem, 0, len(items))
	var reused, retried int64
	for _, it := range items {
		fingerprint := scan.ScanItemFingerprint(it.Path)
		item, ok := resume.Item(fingerprint)
		if !ok {
			if prevPath, wasFailed := resume.FailedFiles[fingerprint]; wasFailed {
				fmt.Fprintf(stdout.Writer(), "[ocr] Resume retrying previously failed file: %s\n", prevPath)
				retried++
			}
			toDispatch = append(toDispatch, it)
			continue
		}
		for _, cm := range item.Comments {
			a.args.CommentCollector.Add(cm)
		}
		a.session.RecordReviewItemReused(it.Path, it.Path, it.Path, fingerprint, resume.SessionID, item.Comments)
		reused++
	}

	rerun := int64(len(toDispatch))
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
	fmt.Fprintf(stdout.Writer(), "[ocr] Resume %s: reusing %d file(s), retrying %d failed file(s), analyzing %d new file(s)\n",
		resume.SessionID, reused, retried, rerun-retried)
	return toDispatch
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

	failed := atomic.LoadInt64(&a.subtaskFailed)
	if failed > 0 && failed == dispatched {
		return nil, fmt.Errorf("all %d file refactor(s) failed — check your LLM configuration and API key", dispatched)
	}
	return a.args.CommentCollector.Comments(), nil
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
		if a.args.MaxTokensBudget > 0 {
			used := a.runner.TotalTokensUsed()
			projected := used + scan.EstimateFileTokens(batch[i], a.planEnabled())
			if projected > a.args.MaxTokensBudget {
				fmt.Fprintf(stdout.Writer(), "[ocr] token budget reached (used %s + next-file est ≈ %s > budget %s) — skipping %s and remaining files\n",
					scan.HumanTokens(used), scan.HumanTokens(projected), scan.HumanTokens(a.args.MaxTokensBudget), batch[i].Path)
				a.recordWarning("token_budget_reached", batch[i].Path,
					fmt.Sprintf("stopped in batch #%d: used %d tokens + next-file estimate exceeds budget %d", batchIdx, used, a.args.MaxTokensBudget))
				budgetHit = true
				break
			}
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

			const maxRetries = 3
			var lastErr error
			for attempt := 0; attempt < maxRetries; attempt++ {
				var fileCtx context.Context
				var cancel context.CancelFunc
				if timeout > 0 {
					retryTimeout := timeout * time.Duration(attempt+1)
					fileCtx, cancel = context.WithTimeout(ctx, retryTimeout)
				} else {
					fileCtx = ctx
				}

				lastErr = a.executeSubtask(fileCtx, it)
				if cancel != nil {
					cancel()
				}

				if lastErr == nil {
					break
				}
				if !errors.Is(lastErr, context.DeadlineExceeded) || attempt == maxRetries-1 {
					break
				}
				a.args.CommentCollector.RemoveByPath(it.Path)
				fmt.Fprintf(stdout.Writer(), "[ocr] Refactor subtask timeout for %s (batch #%d, attempt %d/%d), retrying with extended timeout...\n",
					it.Path, batchIdx, attempt+1, maxRetries)
			}

			if lastErr != nil {
				atomic.AddInt64(&a.subtaskFailed, 1)
				comments := a.args.CommentCollector.CommentsForPath(it.Path)
				a.session.RecordReviewItemFailed(it.Path, it.Path, it.Path, fingerprint, lastErr.Error(), comments)
				if a.args.OnFileDone != nil {
					a.args.OnFileDone(it.Path, comments)
				}
				fmt.Fprintf(stdout.Writer(), "[ocr] Refactor subtask error for %s (batch #%d): %v\n", it.Path, batchIdx, lastErr)
				telemetry.ErrorEvent(context.WithoutCancel(ctx), "refactor.subtask.error", lastErr,
					telemetry.AnyToAttr("file.path", it.Path),
					telemetry.AnyToAttr("batch.index", batchIdx))
				a.recordWarning("refactor_subtask_error", it.Path, lastErr.Error())
				return
			}
			comments := a.args.CommentCollector.CommentsForPath(it.Path)
			a.session.RecordReviewItemDone(it.Path, it.Path, it.Path, fingerprint, comments)
			if a.args.OnFileDone != nil {
				a.args.OnFileDone(it.Path, comments)
			}
		}(batch[i])
	}

	wg.Wait()
	return dispatched, budgetHit, nil
}

func (a *Agent) executeSubtask(ctx context.Context, it model.ScanItem) error {
	ctx, span := telemetry.StartSpan(ctx, "refactor.subtask."+it.Path)
	defer span.End()
	telemetry.SetAttr(span, "file.path", it.Path)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	rule := ""
	if a.args.SystemRule != nil {
		rule = a.args.SystemRule.ResolveRefactor(strings.ToLower(it.Path))
	}

	planGuidance := a.maybeRunPlan(ctx, it, rule)
	messages := a.renderMessages(it, rule, planGuidance)

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

	_, err := a.runner.RunPerFile(ctx, messages, it.Path)
	return err
}

func (a *Agent) maybeRunPlan(ctx context.Context, it model.ScanItem, rule string) string {
	const noPlan = "(no pre-analysis plan; analyze the entire file as usual)"

	if !a.planEnabled() {
		return noPlan
	}
	pt := a.args.Template.PlanTask

	messages := make([]llm.Message, 0, len(pt.Messages))
	for _, m := range pt.Messages {
		content := m.Content
		content = strings.ReplaceAll(content, "{{current_system_date_time}}", a.currentDate)
		content = strings.ReplaceAll(content, "{{current_file_path}}", it.Path)
		content = strings.ReplaceAll(content, "{{system_rule}}", rule)
		content = strings.ReplaceAll(content, "{{file_content}}", it.Content)
		messages = append(messages, llm.NewTextMessage(m.Role, content))
	}

	fs := a.session.GetOrCreateFileSession(it.Path)
	rec := fs.AppendTaskRecord(session.PlanTask, messages)
	startTime := time.Now()

	resp, err := a.args.LLMClient.CompletionsWithCtx(ctx, llm.ChatRequest{
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

func (a *Agent) renderMessages(it model.ScanItem, rule, planGuidance string) []llm.Message {
	rawMsgs := a.args.Template.MainTask.Messages
	messages := make([]llm.Message, 0, len(rawMsgs))
	for _, m := range rawMsgs {
		content := m.Content
		content = strings.ReplaceAll(content, "{{plan_guidance}}", planGuidance)
		content = strings.ReplaceAll(content, "{{current_system_date_time}}", a.currentDate)
		content = strings.ReplaceAll(content, "{{current_file_path}}", it.Path)
		content = strings.ReplaceAll(content, "{{system_rule}}", rule)
		content = strings.ReplaceAll(content, "{{file_content}}", it.Content)
		content = strings.ReplaceAll(content, "{{requirement_background}}", a.args.Background)
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


