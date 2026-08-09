package refactor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/refactor/crossfile"
	"github.com/alibaba/open-code-review/internal/stdout"
	"github.com/alibaba/open-code-review/internal/telemetry"
)

const crossFileHintsBlock = `### CROSS-FILE HINTS (enabled)
	You may use code_search / file_read to find similar logic in related files.
	If you find clear cross-file duplication, mention related paths in the comment body
	(e.g. "Also duplicated in path/b.go"). Keep the primary finding anchored on the current file.
	Do not invent paths you have not seen in tool results or the current file.`

// runCrossFilePhase executes Layer1 cluster → X1 detect → X2 architect → comments,
// and optionally F3 apply+verify when Args.Apply is set.
func (a *Agent) runCrossFilePhase(ctx context.Context) ([]model.LlmComment, error) {
	ctx, span := telemetry.StartSpan(ctx, "refactor.cross_file")
	defer span.End()

	if a.args.LLMClient == nil {
		return nil, fmt.Errorf("cross-file phase requires LLM client")
	}
	if a.args.Template.CrossDetectTask == nil || len(a.args.Template.CrossDetectTask.Messages) == 0 {
		return nil, fmt.Errorf("CROSS_DETECT_TASK missing from refactor template")
	}
	if a.args.Template.CrossArchitectTask == nil || len(a.args.Template.CrossArchitectTask.Messages) == 0 {
		return nil, fmt.Errorf("CROSS_ARCHITECT_TASK missing from refactor template")
	}

	crossRules, err := rules.LoadCrossFileRefactorRule()
	if err != nil {
		return nil, err
	}

	inputs := make([]crossfile.FileInput, 0, len(a.items))
	for _, it := range a.items {
		if it.Content == "" {
			continue
		}
		inputs = append(inputs, crossfile.FileInput{Path: it.Path, Content: it.Content})
	}
	if len(inputs) == 0 {
		fmt.Fprintln(stdout.Writer(), "[ocr] cross-file: no file contents available")
		return nil, nil
	}

	opts := crossfile.DefaultClusterOptions()
	opts.UseSimilarity = true
	clusters := crossfile.BuildClusters(inputs, opts)
	fmt.Fprintf(stdout.Writer(), "[ocr] cross-file: %d file(s) → %d cluster(s)\n", len(inputs), len(clusters))
	telemetry.SetAttr(span, "cross.clusters", len(clusters))
	telemetry.SetAttr(span, "cross.files", len(inputs))

	var allComments []model.LlmComment
	var allPlans []crossfile.RefactorPlan

	for i, c := range clusters {
		if err := ctx.Err(); err != nil {
			return allComments, err
		}
		fmt.Fprintf(stdout.Writer(), "[ocr] cross-file cluster %d/%d id=%s files=%d\n",
			i+1, len(clusters), c.ID, len(c.Files))

		smells, err := a.runCrossDetect(ctx, c, crossRules)
		if err != nil {
			fmt.Fprintf(stdout.Writer(), "[ocr] cross-file detect failed for %s: %v\n", c.ID, err)
			a.recordWarning("cross_detect_error", c.ID, err.Error())
			continue
		}
		for _, co := range crossfile.SmellsToComments(smells, c.ID) {
			cm := commentOutToModel(co)
			a.args.CommentCollector.Add(cm)
			allComments = append(allComments, cm)
		}

		if len(smells) == 0 {
			continue
		}

		plans, err := a.runCrossArchitect(ctx, c, smells, crossRules)
		if err != nil {
			fmt.Fprintf(stdout.Writer(), "[ocr] cross-file architect failed for %s: %v\n", c.ID, err)
			a.recordWarning("cross_architect_error", c.ID, err.Error())
			continue
		}
		// X3: Transform — generate full suggestion_code before applying
		plans, err = a.runCrossTransform(ctx, c, plans, crossRules)
		if err != nil {
			fmt.Fprintf(stdout.Writer(), "[ocr] cross-file transform failed for %s: %v\n", c.ID, err)
			a.recordWarning("cross_transform_error", c.ID, err.Error())
			// Continue with original plans (architect may have populated code)
		}

		allPlans = append(allPlans, plans...)
		for _, co := range crossfile.PlansToComments(plans, c.ID) {
			cm := commentOutToModel(co)
			a.args.CommentCollector.Add(cm)
			allComments = append(allComments, cm)
		}
	}

	if a.args.Apply && len(allPlans) > 0 {
		a.runCrossApply(ctx, allPlans)
	}

	telemetry.Event(ctx, "refactor.cross_file.done",
		telemetry.AnyToAttr("clusters", len(clusters)),
		telemetry.AnyToAttr("comments", len(allComments)),
		telemetry.AnyToAttr("plans", len(allPlans)),
		telemetry.AnyToAttr("apply", a.args.Apply))
	return allComments, nil
}

func (a *Agent) runCrossDetect(ctx context.Context, c crossfile.Cluster, crossRules string) ([]crossfile.SmellReport, error) {
	pt := a.args.Template.CrossDetectTask
	clusterCtx := crossfile.RenderClusterPrompt(c)
	messages := make([]llm.Message, 0, len(pt.Messages))
	for _, m := range pt.Messages {
		content := m.Content
		content = strings.ReplaceAll(content, "{{current_system_date_time}}", a.currentDate)
		content = strings.ReplaceAll(content, "{{cross_file_rules}}", crossRules)
		content = strings.ReplaceAll(content, "{{cluster_context}}", clusterCtx)
		messages = append(messages, llm.NewTextMessage(m.Role, content))
	}
	resp, err := a.args.LLMClient.CompletionsWithCtx(ctx, llm.ChatRequest{
		Model:     a.args.Model,
		Messages:  messages,
		MaxTokens: a.args.Template.MaxTokens,
	})
	if err != nil {
		return nil, err
	}
	a.runner.RecordUsage(resp.Usage)
	return crossfile.ParseSmellReports(resp.Content())
}

func (a *Agent) runCrossArchitect(ctx context.Context, c crossfile.Cluster, smells []crossfile.SmellReport, crossRules string) ([]crossfile.RefactorPlan, error) {
	pt := a.args.Template.CrossArchitectTask
	affected := map[string]struct{}{}
	for _, s := range smells {
		for _, f := range s.AffectedFiles {
			affected[f] = struct{}{}
		}
		for _, e := range s.Evidence {
			affected[e.Path] = struct{}{}
		}
	}
	var aff []string
	for p := range affected {
		aff = append(aff, p)
	}
	smellsJSON, err := json.Marshal(smells)
	if err != nil {
		return nil, fmt.Errorf("marshal smells: %w", err)
	}
	filesBlock := crossfile.RenderFilesForArchitect(c, aff)

	messages := make([]llm.Message, 0, len(pt.Messages))
	for _, m := range pt.Messages {
		content := m.Content
		content = strings.ReplaceAll(content, "{{current_system_date_time}}", a.currentDate)
		content = strings.ReplaceAll(content, "{{cross_file_rules}}", crossRules)
		content = strings.ReplaceAll(content, "{{smells_json}}", string(smellsJSON))
		content = strings.ReplaceAll(content, "{{architect_files}}", filesBlock)
		messages = append(messages, llm.NewTextMessage(m.Role, content))
	}
	resp, err := a.args.LLMClient.CompletionsWithCtx(ctx, llm.ChatRequest{
		Model:     a.args.Model,
		Messages:  messages,
		MaxTokens: a.args.Template.MaxTokens,
	})
	if err != nil {
		return nil, err
	}
	a.runner.RecordUsage(resp.Usage)
	return crossfile.ParseRefactorPlans(resp.Content())
}

func (a *Agent) runCrossApply(ctx context.Context, plans []crossfile.RefactorPlan) {
	fmt.Fprintf(stdout.Writer(), "[ocr] cross-file apply: %d plan(s) (verify+rollback enabled)\n", len(plans))
	// Attempt apply with repair loop only if we have suggestion_code; otherwise skip.
	var attemptPlans []crossfile.RefactorPlan
	for _, p := range plans {
		hasCode := false
		for _, s := range p.Steps {
			if s.SuggestionCode != "" {
				hasCode = true
				break
			}
		}
		if hasCode {
			attemptPlans = append(attemptPlans, p)
		}
	}
	if len(attemptPlans) == 0 {
		fmt.Fprintln(stdout.Writer(), "[ocr] cross-file apply: no plan steps with suggestion_code — skipped")
		telemetry.Event(ctx, "refactor.cross_file.apply.skipped", telemetry.AnyToAttr("reason", "no_code"))
		return
	}

	for attempt := 1; attempt <= crossfile.MaxRepairAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return
		}
		// Apply in sandbox for safety
		sandbox, sbErr := crossfile.NewSandbox(a.args.RepoDir)
		if sbErr != nil {
			fmt.Fprintf(stdout.Writer(), "[ocr] apply: sandbox create failed: %v\n", sbErr)
			return
		}
		defer sandbox.Cleanup()
		res := crossfile.ApplyPlanSteps(sandbox.Dir(), attemptPlans, a.args.ApplyRunTests)
		if res.Verify.OK && !res.RolledBack {
			if mergeErr := sandbox.Merge(res.Written, res.Deleted); mergeErr != nil {
				// Append merge error to existing messages instead of overwriting
				res.Verify.OK = false
				res.Verify.Messages = append(res.Verify.Messages,
					fmt.Sprintf("sandbox merge failed: %v", mergeErr))
				fmt.Fprintf(stdout.Writer(), "[ocr] apply: sandbox merge failed: %v\n", mergeErr)
				// Note: RolledBack is false here because the sandbox still has valid state;
				// the merge failure means repo was not updated but sandbox is intact.
			}
		}
		for _, m := range res.Messages {
			fmt.Fprintf(stdout.Writer(), "[ocr] apply: %s\n", m)
		}
		for _, s := range res.Skipped {
			fmt.Fprintf(stdout.Writer(), "[ocr] apply: skip %s\n", s)
		}
		if res.Verify.OK && !res.RolledBack {
			fmt.Fprintf(stdout.Writer(), "[ocr] cross-file apply succeeded on attempt %d\n", attempt)
			telemetry.Event(ctx, "refactor.cross_file.apply.ok",
				telemetry.AnyToAttr("attempt", attempt),
				telemetry.AnyToAttr("files", len(res.Written)))
			return
		}
		fmt.Fprintf(stdout.Writer(), "[ocr] cross-file apply attempt %d failed (rolled_back=%v)\n", attempt, res.RolledBack)
		// Without a transformer LLM step that rewrites suggestion_code, further
		// retries with the same plan cannot succeed — break after first failure
		// unless we later wire X3 repair prompts.
		if attempt >= 1 {
			telemetry.Event(ctx, "refactor.cross_file.apply.failed",
				telemetry.AnyToAttr("attempt", attempt))
			return
		}
	}
}

func commentOutToModel(co crossfile.CommentOut) model.LlmComment {
	cm := model.LlmComment{
		Path:           co.Path,
		StartLine:      co.StartLine,
		EndLine:        co.EndLine,
		Content:        co.Content,
		SuggestionCode: co.SuggestionCode,
		ExistingCode:   co.ExistingCode,
		Category:       co.Category,
		Severity:       co.Severity,
		PlanID:         co.PlanID,
		SmellType:      co.SmellType,
		RefactorKind:   co.RefactorKind,
		ProposedSymbol: co.ProposedSymbol,
	}
	for _, r := range co.RelatedLocations {
		cm.RelatedLocations = append(cm.RelatedLocations, model.RelatedLocation{
			Path: r.Path, StartLine: r.StartLine, EndLine: r.EndLine, Note: r.Note,
		})
	}
	return cm
}

// runCrossTransform (X3) calls the LLM to generate complete suggestion_code
// for each plan step. Input: RefactorPlan from X2 architect. Output: RefactorPlan
// with suggestion_code fully populated.
func (a *Agent) runCrossTransform(ctx context.Context, c crossfile.Cluster, plans []crossfile.RefactorPlan, crossRules string) ([]crossfile.RefactorPlan, error) {
	ctx, span := telemetry.StartSpan(ctx, "refactor.cross_transform")
	defer span.End()

	pt := a.args.Template.CrossTransformTask
	if pt == nil || len(pt.Messages) == 0 {
		// Fallback: return plans as-is (architect may have already populated code)
		fmt.Fprintln(stdout.Writer(), "[ocr] cross-file transform: CROSS_TRANSFORM_TASK not configured, using architect code as-is")
		return plans, nil
	}

	if a.args.LLMClient == nil {
		return plans, fmt.Errorf("cross-file transform requires LLM client")
	}

	planJSON, filesBlock := crossfile.RenderTransformPrompt(c, plans)

	messages := make([]llm.Message, 0, len(pt.Messages))
	for _, m := range pt.Messages {
		content := m.Content
		content = strings.ReplaceAll(content, "{{current_system_date_time}}", a.currentDate)
		content = strings.ReplaceAll(content, "{{cross_file_rules}}", crossRules)
		content = strings.ReplaceAll(content, "{{plan_json}}", planJSON)
		content = strings.ReplaceAll(content, "{{transform_files}}", filesBlock)
		messages = append(messages, llm.NewTextMessage(m.Role, content))
	}

	timeout := pt.Timeout
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
	}

	resp, err := a.args.LLMClient.CompletionsWithCtx(ctx, llm.ChatRequest{
		Model:     a.args.Model,
		Messages:  messages,
		MaxTokens: a.args.Template.MaxTokens,
	})
	if err != nil {
		return plans, fmt.Errorf("transform LLM call: %w", err)
	}
	a.runner.RecordUsage(resp.Usage)

	transformed, err := crossfile.ParseRefactorPlans(resp.Content())
	if err != nil {
		return plans, fmt.Errorf("parse transform response: %w", err)
	}

	if len(transformed) == 0 {
		return plans, fmt.Errorf("transform response contained no plans")
	}

	// Merge transformed suggestion_code into original plans.
	// NOTE: This mutates orig.Steps in place via the planMap pointers.
	// Callers should not rely on the original plans after this call.
	planMap := make(map[string]*crossfile.RefactorPlan)
	for i := range plans {
		planMap[plans[i].PlanID] = &plans[i]
	}
	for _, tp := range transformed {
		if orig, ok := planMap[tp.PlanID]; ok {
			stepMap := make(map[int]int) // order -> index
			for j, s := range tp.Steps {
				stepMap[s.Order] = j
			}
			for k := range orig.Steps {
				if j, ok := stepMap[orig.Steps[k].Order]; ok && tp.Steps[j].SuggestionCode != "" {
					orig.Steps[k].SuggestionCode = tp.Steps[j].SuggestionCode
				}
			}
		}
	}

	fmt.Fprintf(stdout.Writer(), "[ocr] cross-file transform: %d plan(s) transformed\n", len(transformed))
	telemetry.Event(ctx, "refactor.cross_transform.done",
		telemetry.AnyToAttr("plans", len(transformed)))
	return plans, nil
}
