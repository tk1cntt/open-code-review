package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/alibaba/open-code-review/internal/refactor/crossfile"
	"github.com/spf13/cobra"
)

func addRepoFlag(cmd *cobra.Command, target *string) {
	cmd.Flags().StringVar(target, "repo", "", "root directory of the git repository (default: current dir)")
}

func addRuleFlag(cmd *cobra.Command, target *string) {
	cmd.Flags().StringVar(target, "rule", "", "path to JSON file with system review rules")
}

func addDiffFlags(cmd *cobra.Command, from, to, commit *string) {
	cmd.Flags().StringVar(from, "from", "", "source ref to start diff from (e.g., 'main')")
	cmd.Flags().StringVar(to, "to", "", "target ref to end diff at (e.g., 'feature-branch')")
	cmd.Flags().StringVarP(commit, "commit", "c", "", "single commit hash or tag to review (vs its parent)")
}

func addBackgroundFlags(cmd *cobra.Command, background, backgroundFile *string) {
	cmd.Flags().StringVarP(background, "background", "b", "", "optional requirement/business context for the review")
	cmd.Flags().StringVarP(backgroundFile, "background-file", "B", "", "path to a Markdown file used as review background")
}

func addOutputFlags(cmd *cobra.Command, format, audience *string) {
	cmd.Flags().StringVarP(format, "format", "f", "text", "output format: text or json")
	cmd.Flags().StringVar(audience, "audience", "human", "output audience: human (show progress) or agent (summary only)")
	cmd.RegisterFlagCompletionFunc("format", completeEnum("text", "json"))
	cmd.RegisterFlagCompletionFunc("audience", completeEnum("human", "agent"))
}

func addExcludeFlag(cmd *cobra.Command, target *string) {
	cmd.Flags().StringVar(target, "exclude", "", "comma-separated gitignore-style patterns to exclude; merged with rule.json excludes")
}

func addConcurrencyFlags(cmd *cobra.Command, concurrency, timeout, maxTools, maxGitProcs, maxTokensBudget *int) {
	cmd.Flags().IntVar(concurrency, "concurrency", 8, "max concurrent file reviews")
	cmd.Flags().IntVar(timeout, "timeout", 10, "concurrent task timeout in minutes")
	cmd.Flags().IntVar(maxTools, "max-tools", 0, "max tool call rounds per file (0 = template default; min 10)")
	cmd.Flags().IntVar(maxGitProcs, "max-git-procs", 16, "max concurrent git subprocesses")
	cmd.Flags().IntVar(maxTokensBudget, "max-tokens-budget", 0, "cap total token usage (input+output) for this review; dispatch stops once exceeded and skipped files are reported as failed(budget). Partial results are published and review exits 0; it exits non-zero only if every selected item failed (0 = unlimited)")
}

func addModelFlag(cmd *cobra.Command, target *string) {
	cmd.Flags().StringVar(target, "model", "", "override LLM model for this run (e.g., claude-opus-4-6)")
}

func addToolsFlag(cmd *cobra.Command, target *string) {
	cmd.Flags().StringVar(target, "tools", "", "path to JSON tools config file (default: embedded)")
}

func addPreviewFlag(cmd *cobra.Command, target *bool) {
	cmd.Flags().BoolVarP(target, "preview", "p", false, "preview which files will be reviewed without running the LLM")
}

func completeEnum(values ...string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return values, cobra.ShellCompDirectiveNoFileComp
	}
}

// --- Validation functions ---

func validateDiffMode(from, to, commit string) error {
	modeCount := 0
	if from != "" || to != "" {
		modeCount++
	}
	if commit != "" {
		modeCount++
	}
	if modeCount > 1 {
		return fmt.Errorf("only one review mode allowed (--from/--to or --commit)")
	}
	if from != "" && to == "" {
		return fmt.Errorf("--to is required when --from is specified")
	}
	if to != "" && from == "" {
		return fmt.Errorf("--from is required when --to is specified")
	}
	return nil
}

func validateAudience(audience string) error {
	switch audience {
	case "human", "agent":
		return nil
	default:
		return fmt.Errorf("invalid --audience value %q: must be 'human' or 'agent'", audience)
	}
}

func validateResumeMode(mode string) error {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "continue", "restart-failed", "restart", "cold":
		return nil
	default:
		return fmt.Errorf("invalid --resume-mode %q: must be 'continue' or 'restart-failed'", mode)
	}
}

func validateReviewOptions(opts *reviewOptions) error {
	if err := validateDiffMode(opts.from, opts.to, opts.commit); err != nil {
		return err
	}
	if opts.preview && opts.resume != "" {
		return fmt.Errorf("--preview and --resume cannot be used together")
	}
	if err := validateAudience(opts.audience); err != nil {
		return err
	}
	if err := validateResumeMode(opts.resumeMode); err != nil {
		return err
	}
	const minMaxTools = 10
	if opts.maxTools < 0 {
		return fmt.Errorf("--max-tools must be a non-negative integer (0 means use template default)")
	}
	if opts.maxTools > 0 && opts.maxTools < minMaxTools {
		fmt.Fprintf(os.Stderr, "[ocr] --max-tools %d is below minimum %d, using %d\n", opts.maxTools, minMaxTools, minMaxTools)
		opts.maxTools = minMaxTools
	}
	if opts.maxGitProcs < 0 {
		return fmt.Errorf("--max-git-procs must be a non-negative integer (0 means use default 16)")
	}
	if opts.maxTokensBudget < 0 {
		return fmt.Errorf("--max-tokens-budget must be a non-negative integer (0 means unlimited)")
	}
	return nil
}

func validateScanOptions(opts *scanOptions) error {
	if err := validateAudience(opts.audience); err != nil {
		return err
	}
	if err := validateResumeMode(opts.resumeMode); err != nil {
		return err
	}
	if opts.maxTools < 0 {
		return fmt.Errorf("--max-tools must be a non-negative integer (0 means use template default)")
	}
	if opts.maxGitProcs < 0 {
		return fmt.Errorf("--max-git-procs must be a non-negative integer (0 means use default 16)")
	}
	if opts.maxTokensBudget < 0 {
		return fmt.Errorf("--max-tokens-budget must be a non-negative integer (0 means unlimited)")
	}
	return nil
}

func validateDelegateOptions(opts *delegateOptions) error {
	return validateDiffMode(opts.from, opts.to, opts.commit)
}

// registerReviewFlags registers all review command flags on cmd, binding to opts.
func registerReviewFlags(cmd *cobra.Command, opts *reviewOptions) {
	addToolsFlag(cmd, &opts.toolConfigPath)
	addRuleFlag(cmd, &opts.rulePath)
	addRepoFlag(cmd, &opts.repoDir)
	addDiffFlags(cmd, &opts.from, &opts.to, &opts.commit)
	cmd.Flags().StringVar(&opts.resume, "resume", "", "resume from a previous review session id")
	cmd.Flags().StringVar(&opts.resumeMode, "resume-mode", "continue", "resume mode: continue (mid-file checkpoint) or restart-failed (cold retry)")
	cmd.RegisterFlagCompletionFunc("resume-mode", completeEnum("continue", "restart-failed"))
	addExcludeFlag(cmd, &opts.excludes)
	addOutputFlags(cmd, &opts.outputFormat, &opts.audience)
	addConcurrencyFlags(cmd, &opts.concurrency, &opts.perFileTimeout, &opts.maxTools, &opts.maxGitProcs, &opts.maxTokensBudget)
	addBackgroundFlags(cmd, &opts.background, &opts.backgroundFile)
	addModelFlag(cmd, &opts.model)
	addPreviewFlag(cmd, &opts.preview)
	cmd.Flags().StringVar(&opts.rulesDir, "rules-dir", "", "directory with enterprise/project review rules (env: OCR_RULES_DIR)")
	cmd.Flags().BoolVar(&opts.saveResult, "save-result", true, "persist final review result for the WebUI review viewer")
	cmd.Flags().BoolVar(&opts.savePerFile, "save-per-file", true, "split output into per-file markdown files under a directory tree mirroring the source tree")
	cmd.Flags().StringVar(&opts.resultDir, "result-dir", "", "review result storage root (env: OCR_REVIEWS_DIR, default: <repo>/.opencodereview/reviews)")
	cmd.Flags().StringVar(&opts.resultProject, "result-project", "", "project name/path for persisted review results")
	cmd.Flags().StringVar(&opts.resultSourceBranch, "result-source-branch", "", "source branch metadata for persisted review results")
	cmd.Flags().StringVar(&opts.resultTargetBranch, "result-target-branch", "", "target branch metadata for persisted review results")
	cmd.Flags().BoolVar(&opts.apply, "apply", false, "apply suggestion_code from review comments (verify+rollback; best-effort per file)")
	cmd.Flags().BoolVar(&opts.applyRunTests, "apply-run-tests", false, "when --apply, also run go test on touched packages during verify")
}

// registerScanFlags registers all scan command flags on cmd, binding to opts.
func registerScanFlags(cmd *cobra.Command, opts *scanOptions) {
	addToolsFlag(cmd, &opts.toolConfigPath)
	addRuleFlag(cmd, &opts.rulePath)
	addRepoFlag(cmd, &opts.repoDir)
	cmd.Flags().StringVar(&opts.paths, "path", "", "comma-separated repo-relative directories or files to scan (default: whole repo)")
	addExcludeFlag(cmd, &opts.excludes)
	addOutputFlags(cmd, &opts.outputFormat, &opts.audience)
	cmd.Flags().IntVar(&opts.concurrency, "concurrency", 8, "max concurrent file scans")
	cmd.Flags().IntVar(&opts.perFileTimeout, "timeout", 10, "concurrent task timeout in minutes")
	cmd.Flags().IntVar(&opts.maxTools, "max-tools", 0, "max tool call rounds per file; only takes effect when greater than template default")
	cmd.Flags().IntVar(&opts.maxGitProcs, "max-git-procs", 16, "max concurrent git subprocesses")
	cmd.Flags().IntVar(&opts.maxTokensBudget, "max-tokens-budget", 0, "cap total token usage; dispatch stops once exceeded (0 = unlimited)")
	cmd.Flags().StringVarP(&opts.background, "background", "b", "", "optional requirement/business context for the scan")
	cmd.Flags().StringVar(&opts.resume, "resume", "", "resume from a previous scan session id")
	cmd.Flags().StringVar(&opts.resumeMode, "resume-mode", "continue", "resume mode: continue (mid-file checkpoint) or restart-failed (cold retry)")
	cmd.RegisterFlagCompletionFunc("resume-mode", completeEnum("continue", "restart-failed"))
	cmd.Flags().BoolVar(&opts.saveResult, "save-result", true, "persist final scan result for the WebUI review viewer")
	cmd.Flags().BoolVar(&opts.savePerFile, "save-per-file", true, "split output into per-file markdown files under a directory tree mirroring the source tree")
	cmd.Flags().StringVar(&opts.resultDir, "result-dir", "", "scan result storage root (env: OCR_REVIEWS_DIR, default: .opencodereview/reviews)")
	cmd.Flags().StringVar(&opts.resultProject, "result-project", "", "project name/path for persisted scan results")
	cmd.Flags().BoolVarP(&opts.preview, "preview", "p", false, "preview which files will be scanned without running the LLM")
	cmd.Flags().BoolVar(&opts.noPlan, "no-plan", false, "skip the per-file PLAN_TASK pre-pass")
	cmd.Flags().BoolVar(&opts.noDedup, "no-dedup", false, "skip the per-batch DEDUP_TASK")
	cmd.Flags().BoolVar(&opts.noSummary, "no-summary", false, "skip the post-run PROJECT_SUMMARY_TASK")
	cmd.Flags().StringVar(&opts.batch, "batch", "", "override BATCH_STRATEGY: none | by-language | by-directory")
	addModelFlag(cmd, &opts.model)
	cmd.RegisterFlagCompletionFunc("batch", completeEnum("none", "by-language", "by-directory"))
}

// registerDelegateFlags registers all delegate shared flags on cmd, binding to opts.
func registerDelegateFlags(cmd *cobra.Command, opts *delegateOptions) {
	addRepoFlag(cmd, &opts.repoDir)
	addDiffFlags(cmd, &opts.from, &opts.to, &opts.commit)
	addExcludeFlag(cmd, &opts.excludes)
	addRuleFlag(cmd, &opts.rulePath)
	addBackgroundFlags(cmd, &opts.background, &opts.backgroundFile)
	cmd.Flags().IntVar(&opts.maxGitProcs, "max-git-procs", 16, "max concurrent git subprocesses")
}

// registerRefactorFlags registers all refactor command flags on cmd, binding to opts.
func registerRefactorFlags(cmd *cobra.Command, opts *refactorOptions) {
	addToolsFlag(cmd, &opts.toolConfigPath)
	addRuleFlag(cmd, &opts.rulePath)
	addRepoFlag(cmd, &opts.repoDir)
	cmd.Flags().StringVar(&opts.paths, "path", "", "comma-separated repo-relative directories or files to refactor (default: whole repo)")
	addExcludeFlag(cmd, &opts.excludes)
	addOutputFlags(cmd, &opts.outputFormat, &opts.audience)
	cmd.Flags().IntVar(&opts.concurrency, "concurrency", 8, "max concurrent file analyses")
	cmd.Flags().IntVar(&opts.perFileTimeout, "timeout", 10, "concurrent task timeout in minutes")
	cmd.Flags().StringVarP(&opts.background, "background", "b", "", "optional requirement/business context for the refactoring")
	cmd.Flags().IntVar(&opts.maxTools, "max-tools", 0, "max tool call rounds per file; only takes effect when greater than template default")
	cmd.Flags().IntVar(&opts.maxGitProcs, "max-git-procs", 16, "max concurrent git subprocesses")
	cmd.Flags().IntVar(&opts.maxTokensBudget, "max-tokens-budget", 0, "cap total token usage; dispatch stops once exceeded (0 = unlimited)")
	cmd.Flags().BoolVarP(&opts.preview, "preview", "p", false, "preview which files will be analyzed without running the LLM")
	cmd.Flags().BoolVar(&opts.noPlan, "no-plan", false, "skip the per-file PLAN_TASK pre-pass")
	cmd.Flags().StringVar(&opts.mode, "mode", "local", "refactor pipeline: local (per-file), cross (multi-file detect+architect), full (local then cross)")
	cmd.Flags().StringVar(&opts.crossFile, "cross-file", "off", "per-file cross-file hints: off | hints (cite related paths via tools)")
	cmd.Flags().BoolVar(&opts.apply, "apply", false, "apply cross-file architect plans that include suggestion_code (verify+rollback; default false)")
	cmd.Flags().BoolVar(&opts.applyRunTests, "apply-run-tests", false, "when --apply, also run go test on touched packages during verify")
	addModelFlag(cmd, &opts.model)
	cmd.Flags().StringVar(&opts.resume, "resume", "", "resume from a previous refactoring session id")
	cmd.Flags().StringVar(&opts.resumeMode, "resume-mode", "continue", "resume mode: continue (mid-file checkpoint) or restart-failed (cold retry)")
	cmd.RegisterFlagCompletionFunc("resume-mode", completeEnum("continue", "restart-failed"))
	cmd.Flags().BoolVar(&opts.saveResult, "save-result", true, "persist final refactoring result for the WebUI viewer")
	cmd.Flags().BoolVar(&opts.savePerFile, "save-per-file", true, "split output into per-file markdown files")
	cmd.Flags().StringVar(&opts.resultDir, "result-dir", "", "refactoring result storage root (env: OCR_REVIEWS_DIR, default: .opencodereview/refactors)")
	cmd.Flags().StringVar(&opts.resultProject, "result-project", "", "project name/path for persisted refactoring results")
}

func validateRefactorOptions(opts *refactorOptions) error {
	if err := validateAudience(opts.audience); err != nil {
		return err
	}
	if err := validateResumeMode(opts.resumeMode); err != nil {
		return err
	}
	if opts.maxTools < 0 {
		return fmt.Errorf("--max-tools must be a non-negative integer (0 means use template default)")
	}
	if opts.maxGitProcs < 0 {
		return fmt.Errorf("--max-git-procs must be a non-negative integer (0 means use default 16)")
	}
	if opts.maxTokensBudget < 0 {
		return fmt.Errorf("--max-tokens-budget must be a non-negative integer (0 means unlimited)")
	}
	mode := crossfile.ParseMode(opts.mode)
	switch mode {
	case crossfile.ModeLocal, crossfile.ModeCross, crossfile.ModeFull:
		opts.mode = string(mode)
	default:
		return fmt.Errorf("--mode must be one of: local, cross, full")
	}
	cf := strings.ToLower(strings.TrimSpace(opts.crossFile))
	if cf == "" {
		cf = "off"
	}
	if cf != "off" && cf != "hints" {
		return fmt.Errorf("--cross-file must be one of: off, hints")
	}
	opts.crossFile = cf
	return nil
}
