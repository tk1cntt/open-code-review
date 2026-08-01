---
name: open-code-review
description: >
  Performs AI-powered code review on Git changes using the `ocr` CLI from
  alibaba/open-code-review. Use when the user asks to review code, review
  a pull request, review staged/unstaged changes, review a commit, or
  compare branches for code quality issues. Produces line-level review
  comments and can automatically apply fixes when requested. With appropriate
  review rules, can detect various types of issues including bugs, security
  vulnerabilities, performance problems, and code quality concerns.
license: Apache-2.0
compatibility: >
  Requires the `ocr` CLI installed (via `npm install -g
  @alibaba-group/open-code-review` or GitHub release binary). Requires a
  configured LLM (Anthropic or OpenAI-compatible) before first run.
metadata:
  author: alibaba
  homepage: https://github.com/alibaba/open-code-review
  version: "1.1.0"
---

# Open Code Review

A skill for invoking [open-code-review](https://github.com/alibaba/open-code-review) (`ocr`) — an open-source AI code review CLI that reads Git diffs and generates structured, line-level review comments.

## Prerequisites check

Before starting a review, verify the environment:

```bash
# 1. Check the CLI is installed
which ocr || echo "NOT INSTALLED"

# 2. Verify LLM connectivity
ocr llm test
```

If `ocr` is not installed, install it first:

```bash
npm install -g @alibaba-group/open-code-review
```

If `ocr llm test` fails, the user must configure an LLM. Guide them with one of these options:

**Option A — Environment variables (highest priority, recommended for CI):**

```bash
export OCR_LLM_URL=https://api.anthropic.com/v1/messages
export OCR_LLM_TOKEN=<api-key>
export OCR_LLM_MODEL=claude-opus-4-6
export OCR_USE_ANTHROPIC=true
```

**Option B — Persistent config:**

```bash
ocr config set llm.url https://api.anthropic.com/v1/messages
ocr config set llm.auth_token <api-key>
ocr config set llm.model claude-opus-4-6
ocr config set llm.use_anthropic true
```

Stop here and ask the user to provide credentials — never invent or hardcode API keys.

## Workflow

### Step 1: Gather Business Context

Analyze the review target (commits, branch, or changes) to extract concise business context. Pass this context via `--background` to improve review quality.

### Step 2: Run Code Review

Run the OCR command with appropriate flags. **Always pass business context via `--background`** when available:

```bash
ocr review --audience agent --background "business context here" [user-args]
```

**`--save-result` is enabled by default.** Results are automatically persisted to `<repo>/.opencodereview/reviews/<project-key>/<review-id>.json`.

**Argument handling:**

- **Background context** (RECOMMENDED): use `--background "context"` or `-b "context"` to provide business context for better review quality
- **Default** (no user arguments): reviews staged, unstaged, and untracked changes (workspace mode)
- **Specific commit**: use `--commit` or `-c` to review a single commit against its parent
- **Branch comparison**: use `--from <ref>` and `--to <ref>` to review diff between two refs
- **Timeout**: default timeout is 10 minutes per file; adjust with `--timeout <minutes>`
- **Concurrency**: default concurrency is 8 file workers; reduce with `--concurrency <n>` if rate limits are hit
- **Preview mode**: use `--preview` or `-p` to preview which files will be reviewed without running the LLM
- **Resume**: use `--resume <session-id>` to reuse completed results and retry failed files from a previous session
- **Installation**: if `ocr` command is not found, install it by running `npm i -g @alibaba-group/open-code-review`

**Common invocation patterns:**

| User says | Command to run |
|-----------|---------------|
| "review my changes" / "review the working copy" | `ocr review --audience agent -b "context"` |
| "review this PR" / "review feature branch" | `ocr review --audience agent -b "context" --from main --to <branch>` |
| "review commit abc123" | `ocr review --audience agent -b "context" --commit abc123` |
| "what would be reviewed?" (dry-run) | `ocr review --preview` |

**Output mode:**

- Always use `--audience agent` to suppress progress UI and emit only the final summary

### Step 3: Read Structured Review Results

After the review completes, the result JSON is saved at `<repo>/.opencodereview/reviews/`. The terminal output will show the exact path:

```
[ocr] Review result saved to: /path/to/.opencodereview/reviews/<project-key>/<review-id>.json
```

**Read the JSON file** to get structured review data:

```bash
cat .opencodereview/reviews/<project-key>/<review-id>.json
```

Each comment in the JSON contains:

| Field | Description |
|-------|-------------|
| `path` | Relative file path |
| `content` | Review comment describing the issue |
| `start_line` / `end_line` | Line range of the issue (0 means positioning failed) |
| `suggestion_code` | Optional fix suggestion code |
| `existing_code` | Optional original code snippet |
| `severity` | `critical`, `high`, `medium`, or `low` |
| `category` | `bug`, `security`, `performance`, `maintainability`, `test`, `style`, `documentation` |
| `thinking` | Optional LLM reasoning |

Also note the `review.session_id` field — save this for the `--resume` flag in Step 5.

### Step 4: Classify and Prioritize

For each comment from the JSON, classify by priority:

- **Critical/High**: Obvious bugs, security issues, clear mistakes, or well-founded suggestions with precise fix proposals — **must fix**
- **Medium**: Reasonable concerns but context-dependent, style/performance suggestions, or fixes that require manual implementation — **fix if clear**
- **Low**: Likely false positives, lacking sufficient context, nitpicks, or meaningless suggestions — **skip silently**

Report all actionable findings grouped by file and priority before applying fixes.

### Step 5: Fix Issues

Check whether the user requested automatic fixes:

- If the user explicitly said "review and fix" or similar → proceed with automatic fixes
- If the user only said "review" → ask for permission before applying any changes

**Fix process:**

1. Read each file with issues (use the `path` field from JSON)
2. For each comment:
   - If `suggestion_code` is present and matches the issue → apply the suggestion
   - If `start_line`/`end_line` are non-zero → navigate to that location and fix
   - If line numbers are 0 (mispositioned) → search the file for the code described in `content`
3. After applying fixes, run verification: `go build ./...` or equivalent for the project language
4. Report which fixes were applied and which require manual follow-up

**Priority order:** Fix Critical/High items first, then Medium. Skip Low items.

### Step 6: Re-review with --resume

After applying fixes, re-run review to verify fixes resolved the issues:

```bash
ocr review --audience agent -b "context" --from main --to HEAD --resume <session-id>
```

`--resume` will:
- **Reuse** completed results for files that haven't changed
- **Retry** files that previously failed (auto-detected)
- **Re-review** files whose diff content changed (because you edited them)

### Step 7: Iterate Until Clean

```
Review → Read JSON → Fix → Re-review (--resume) → Repeat
```

Stop when:
- No Critical/High issues remain
- Only Medium issues that require human judgment remain
- All fixes pass `go build` or equivalent

## Output Format

Each comment contains:

- `path`: File path
- `content`: Review comment text
- `start_line` / `end_line`: Line range (both 0 means positioning failed)
- `suggestion_code`: Optional fix suggestion
- `existing_code`: Optional original code snippet
- `thinking`: Optional LLM reasoning process

After filtering comments by priority, present results using this template:

```markdown
## Code Review Results

**Files reviewed**: N
**Issues found**: X high priority / Y medium priority

### High Priority

- **`path/to/file.java:42`** — Brief description
  > Recommendation: How to fix

### Medium Priority

- **`path/to/file.ts:88`** — Brief description
  > Recommendation: How to fix (if applicable)
```

If the review found no issues after filtering, simply state: "Review complete — no issues found in N files."

**Priority classification:**

- **High**: Obvious bugs, security issues, clear mistakes, or well-founded suggestions with precise fix proposals
- **Medium**: Reasonable concerns but context-dependent, style/performance suggestions, or fixes that require manual implementation
- **Low**: Discarded silently (likely false positives, lacking context, nitpicks, or meaningless suggestions)

**Handling mispositioned comments:**

When `start_line` and `end_line` are both `0`, the comment failed to locate the exact position in the file. In such cases:

1. Read the comment content to understand the issue
2. Examine the target file mentioned in the comment
3. Identify the relevant code section based on the comment's context
4. Apply the fix or suggestion to the correct location

## Custom Review Rules

If the user wants project-specific rules, OCR resolves them in this priority order:

1. `--rule <path>` flag (highest)
2. `<repo>/.opencodereview/rule.json`
3. Enterprise project `<rules-dir>/projects/<project>/rule.json`
4. Enterprise global `<rules-dir>/global.json`
5. `~/.opencodereview/rule.json`
6. Built-in system defaults (lowest)

By default, the first matching user rule replaces the built-in system rule. Set `merge_system_rule: true` on a rule entry when the matched system rule and user rule should both be included.

Rule file format:

```json
{
  "rules": [
    {
      "path": "**/*.java",
      "rule": "All new methods must validate required parameters for null",
      "merge_system_rule": true
    },
    {
      "path": "**/*mapper*.xml",
      "rule": "Check SQL for injection risks and missing closing tags"
    }
  ]
}
```

To preview which rule applies to a file before reviewing:

```bash
ocr rules check src/main/java/com/example/Foo.java
```

## Gotchas

- **LLM must be configured first** — `ocr review` will fail loudly if no LLM is reachable. Always run `ocr llm test` before the first review.
- **Working directory matters** — `ocr review` operates on the Git repo at the current directory. Use `--repo /path/to/repo` to run from elsewhere.
- **Untracked files are reviewed in workspace mode** — running bare `ocr review` includes staged, unstaged, *and* untracked changes. Stage selectively if you want narrower scope.
- **Large diffs may hit token limits** — files with very large diffs may be truncated. The default `MAX_TOKENS` is 58888 per request.
- **Plan phase triggers at 50 lines** — diffs exceeding 50 changed lines run an extra risk-analysis phase before main review. This adds latency but improves quality.
- **Don't pass `--audience human`** — it streams progress UI that pollutes output. Always use `--audience agent`.
- **Comment language follows config** — set `language` config to `English` or `Chinese` (default: Chinese) to control review comment language.
- **`--save-result` is always on** — results are saved to `<repo>/.opencodereview/reviews/`. Read this JSON instead of parsing terminal output.
- **Use `--resume` after fixing** — it skips unchanged files and retries failed files, making re-review fast.

## Validation

After the review completes, verify success by checking:

1. The command exited with code 0
2. A result JSON was saved to `.opencodereview/reviews/`
3. `cat .opencodereview/reviews/<project-key>/<latest>.json | jq '.comments | length'` returns a count
4. Warnings (if any) are displayed in stderr

If errors occurred, check the stderr warnings for details about which files failed and why.

## References

- Full docs: https://github.com/alibaba/open-code-review
- NPM package: https://www.npmjs.com/package/@alibaba-group/open-code-review
- Issue tracker: https://github.com/alibaba/open-code-review/issues
