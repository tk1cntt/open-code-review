#### GitHub Config Files Refactoring Rules

##### Issue Template Organization
- Multiple templates with overlapping purpose should be consolidated
- Repeated form fields across templates should be standardized
- Template `body` structure should follow consistent organization
- Missing `name` or `description` on templates used in the chooser

##### Release Configuration
- Release categories with inconsistent label groupings
- Missing catch-all category (`*`) that may omit PRs from release notes
- Undefined labels in `categories[].labels` that don't match repository labels
- Repeated category definitions across configuration files

##### YAML Structure
- Deeply nested configuration blocks should be flattened or documented
- Repeated anchor/alias patterns suggest missing shared config or defaults
- Inconsistent key naming conventions across files
- Unused anchors and aliases

##### Dead Config
- Templates referencing directories or files that no longer exist
- Unused release configuration sections
- Commented-out YAML blocks should be deleted
- Stale label references in automation configs

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
