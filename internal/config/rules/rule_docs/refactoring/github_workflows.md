#### GitHub Workflows Refactoring Rules

##### Workflow Organization
- Large workflow files with >200 lines should be split into reusable workflows or composite actions
- Repeated step patterns across workflows should be extracted into reusable actions
- Related jobs that could run in parallel should use proper `needs:` dependencies
- Job definitions sharing identical configurations should use matrix strategies or templates

##### Naming and Clarity
- Workflow `name` should describe what it does from a developer perspective
- Job names should be descriptive for readability in CI logs
- Step names should convey intent, not just the action being run
- `env` variable names should follow consistent casing conventions

##### Duplication
- Repeated checkout + setup steps across jobs should be a composite action
- Repeated caching patterns across workflows should be standardized
- Repeated deployment steps across environments should use reusable workflows with inputs
- Shared environment variables across workflows should be in organization-level variables

##### Security and Performance
- `permissions` at workflow/job level should follow least-privilege
- `pull_request_target` with checkout of PR head should be carefully scoped
- Unpinned third-party actions should use commit SHA
- Missing `timeout-minutes` on self-hosted runners
- Missing `concurrency` group to prevent redundant runs

##### Dead Code
- Unused jobs and steps should be removed
- Commented-out workflow sections should be deleted
- Stale workflow triggers matching deleted branches
- Deprecated syntax (`set-output`, `save-state`) should be updated

##### Maintainability
- Complex `if:` conditions should use intermediate environment variables for clarity
- Secret references scattered across files should be audited for consistency
- Matrix strategies with overlapping configurations should be consolidated

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
