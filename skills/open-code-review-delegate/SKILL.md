---
name: open-code-review-delegate
description: >
  Delegation mode for open-code-review (OCR). Instead of OCR calling an LLM
  endpoint, this skill instructs the host agent to perform the code review
  itself, using OCR only for deterministic engineering: file selection and
  rule resolution. Use when the host agent should drive the review with its
  own LLM capabilities.
license: Apache-2.0
compatibility: >
  Requires the `ocr` CLI installed (via `npm install -g
  @alibaba-group/open-code-review` or GitHub release binary). Does NOT
  require a configured LLM endpoint — delegation mode is LLM-free on the
  OCR side.
metadata:
  author: alibaba
  homepage: https://github.com/alibaba/open-code-review
  version: "1.1.0"
---

# Open Code Review — Delegation Mode

A skill for performing AI code review where OCR provides deterministic engineering (file filtering, rule resolution) and the host agent performs the actual review using its own intelligence and tools.

## Workflow

### Step 1: Preview — Determine What to Review

```bash
ocr delegate preview --format json [--from <ref> --to <ref>] [--commit <hash>] [--exclude <patterns>]
```

This outputs:
- **mode** (workspace / range / commit)
- **from / to / commit / merge_base** — ref metadata for constructing git commands
- **Reviewable file list** — paths, status, insertions/deletions
- **Excluded files** — with exclusion reason

**Common invocations:**

| Scenario | Command |
|----------|---------|
| Workspace changes | `ocr delegate preview` |
| Branch comparison | `ocr delegate preview --from main --to feature` |
| Single commit | `ocr delegate preview -c abc123` |

### Step 2: Get Rules for Files

```bash
ocr delegate rule --format json <path1> <path2> ...
```

Pass the reviewable file paths from Step 1. Output is grouped by rule content — files sharing the same rule appear under one group, avoiding repetition.

### Step 3: Get Diffs

Use git directly based on the mode/ref info from Step 1:

**Range mode** (merge_base provided in preview output):
```bash
git diff <merge_base>..<to> -- <path>
```

**Commit mode**:
```bash
git show <commit> -- <path>
```

**Workspace mode**:
```bash
# Tracked files
git diff HEAD -- <path>
# New untracked files — read directly (entire file is new code)
cat <path>
```

### Step 4: Review Each File

Create a checklist containing every `reviewable_files` entry. For each reviewable file:

Use `(path, status)` as the checklist identity. Workspace mode can report the same path twice when a staged deletion is followed by an untracked recreation.

1. Get its diff (Step 3)
2. Consult its Rule Group (from Step 2) for the review checklist
3. Conduct a thorough review, using appropriate context tools as needed
4. Record findings in structured format (Step 5)
5. Mark the file `reviewed`, or `skipped` with a concrete reason

For large changes, review in bounded batches grouped by shared rules and diff size. Do not stop after finding the first high-severity issue.

### Step 5: Save and Format Output

**Save results as JSON** to `<repo>/.opencodereview/reviews/<project-key>/<review-id>.json` so they can be consumed by subsequent fix steps and the viewer.

Each comment must follow this structure:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| path | string | yes | Relative file path |
| content | string | yes | Review comment describing the issue |
| start_line | integer | no | Start line in the new file |
| end_line | integer | no | End line in the new file |
| severity | enum | no | critical, high, medium, low |
| category | enum | no | bug, security, performance, maintainability, test, style, documentation, other |
| suggestion_code | string | no | Suggested fix code |
| existing_code | string | no | Original code snippet |

### Step 6: Classify and Report

Before reporting, verify that every previewed file is accounted for. Include `total_files`, `reviewed_files`, `skipped_files`, and `coverage_rate` in the summary. A skipped file must include its reason.

Group findings by severity:

- **Critical/High**: Bugs, security issues, data loss risks — **must fix**, always report
- **Medium**: Performance concerns, error handling gaps, maintainability issues — **fix if clear**, report with context
- **Low**: Style nits, minor suggestions — **skip silently** unless clearly valuable

Discard likely false positives silently.

### Step 7: Fix Issues

Check whether the user requested automatic fixes:

- If the user explicitly said "review and fix" or similar → proceed with automatic fixes
- If the user only said "review" → ask for permission before applying any changes

**Fix process:**

1. Read the saved JSON results from Step 5
2. Start with Critical/High items:
   - If `suggestion_code` is present → apply directly
   - If line numbers are provided → navigate to that location and fix
   - If line numbers are 0 → search the file for the described code
3. Then fix Medium items where the fix is clear-cut
4. Skip Low items
5. Run build/verification after each batch of fixes: `go build ./...` or equivalent
6. Report: which fixes were applied, which need manual follow-up

### Step 8: Re-review After Fixes

After applying fixes, re-review the changed files to verify fixes resolved the issues:

1. Re-run Steps 1-4 for the changed files only (use `git diff` to identify what changed)
2. Compare new findings against the saved JSON from the first pass
3. Verify previously reported issues are resolved
4. Flag any new issues introduced by the fixes

### Step 9: Iterate Until Clean

```
Review → Save JSON → Classify → Fix → Re-review → Repeat
```

Stop when:
- No Critical/High issues remain
- Only Medium issues that require human judgment remain
- All fixes pass `go build` or equivalent

## Sub-commands Reference

| Command | Purpose |
|---------|---------|
| `ocr delegate preview` | Which files to review + mode/ref metadata |
| `ocr delegate rule <path...>` | Review rules grouped by content |

## Shared Flags

| Flag | Description |
|------|-------------|
| `--from <ref>` | Source ref for range mode |
| `--to <ref>` | Target ref for range mode |
| `-c, --commit <hash>` | Single commit mode |
| `--repo <path>` | Repository root (default: cwd) |
| `--rule <path>` | Custom rule.json path |
| `--exclude <patterns>` | Comma-separated exclude patterns |
| `-b, --background <text>` | Business context |
| `-B, --background-file <path>` | Business context from Markdown file (takes precedence over `-b`) |
| `-f, --format <text\|json>` | Output format; use `json` for agent integrations |

## Save Results Convention

In delegation mode, always save review results to `<repo>/.opencodereview/reviews/` as JSON. This enables:

- **Fix loop**: agent reads JSON → fixes → re-reviews → compares with old JSON
- **Viewer**: results are browseable via `ocr viewer`
- **CI/CD**: artifacts can be published and analyzed over time

Directory structure:
```
<repo>/.opencodereview/reviews/
└── <project-key>/
    └── <timestamp-or-uuid>.json
```

Each JSON file should use the same schema as OCR's built-in `--save-result` output (see Step 5 for field reference).

## Gotchas

- **No LLM needed on OCR side** — delegation mode never calls an LLM. All intelligence comes from the host agent.
- **Rules are grouped** — Files sharing the same rule are grouped together in the output. You can pass any number of paths per call; for large changes, fetch rules per-batch as you review.
- **Working directory matters** — `ocr delegate` operates on the Git repo at the current directory. Use `--repo /path` to override.
- **Untracked files in workspace mode** — `preview` includes untracked files. For these, read the file directly instead of using `git diff`.
- **Background context** — pass `--background` to `preview` when you have requirement context; it appears in the output for your reference during review.
- **Always save JSON results** — the fix loop depends on structured data. Don't rely on terminal output alone.
- **Use the JSON for the fix loop** — read `path`, `start_line`, `end_line`, `suggestion_code`, `severity` fields to apply precise fixes.
- **Coverage is mandatory** — every `reviewable_files` entry must end as reviewed or explicitly skipped; do not silently omit files.

### Recovering Oversized Background Context

`--background-file` has two independent limits. The raw file must not exceed
1 MiB, and the sanitized content must not exceed 8000 characters. Either
condition aborts the command. When the command reports either limit:

1. Do not silently truncate the source file.
2. Summarize the original material while preserving its requirements,
   constraints, acceptance criteria, and other review-critical details.
3. Retry the affected command by passing the summary as one shell-safe
   argument (for example, use a quoted/escaped argument produced by the host
   shell, or write it to a new size-bounded file and pass that file). Do not
   place untrusted summary text directly in a double-quoted shell template;
   `$()`, backticks, quotes, and variable references can still be evaluated.
   Omit the original `--background-file` so the CLI does not reload the same
   oversized file and fail again.
4. If a faithful summary is not possible, omit the OCR background entirely and
   read the original material directly during the review.

### Troubleshooting CLI Version Compatibility

The `--format` flag is available in `ocr` v1.9.0 and later. The Skill and the
installed CLI can be updated independently. If a requested `preview` or `rule`
command with `--format json` fails specifically with `unknown flag: --format`,
rerun it without the flag and use text output for the rest of the delegation
run. Preserve the explicit mode, ref, file, and rule information from that
output; do not parse text output as JSON or invent missing schema fields. Do
not retry without the flag for any other error; report it and stop the affected
workflow.

The host-agent Skill may consume the equivalent text output to complete its
review checklist. Programmatic integrations that require `schema_version` or
other JSON fields must require a JSON-capable CLI instead: verify with
`ocr --version` and upgrade when necessary:

```bash
npm install -g @alibaba-group/open-code-review
```
