#### Terraform Refactoring Rules

##### Module and Resource Organization
- Large `.tf` files with >200 lines should be split by resource group or concern
- Repeated resource blocks with similar configuration suggest a module or `for_each`
- Resources that always appear together should be grouped into a child module
- Related locals, variables, and outputs should be organized by domain

##### Naming and Clarity
- Resource type labels should clearly describe purpose using snake_case
- Variable names should convey intent; boolean variables should use `is_`/`enable_`/`has_` prefixes
- `description` should be present on all variables and outputs forming the module interface
- Avoid redundant type info in names: `vpc_id` not `vpc_id_string`

##### Duplication
- Repeated resource blocks with only name differences should use `for_each` or `count`
- Repeated locals expressions should be consolidated
- Similar module blocks with different inputs should use iteration patterns
- Repeated `depends_on` patterns suggest missing implicit references

##### Variable and Output Design
- Variables with no default or validation on critical inputs should add validation
- Boolean variables controlling complex behavior suggest separate modules or feature flags
- Outputs that duplicate input values without transformation should be removed
- Sensitive outputs without `sensitive = true`

##### Dead Resources and Code
- Unused variables, locals, and outputs should be removed
- Resources defined but never referenced should be cleaned
- Commented-out resource blocks and stale backup files should be deleted
- Empty or no-op modules with no resources

##### State and Lifecycle
- Stateful resources (databases, storage, KMS) without `lifecycle` blocks should add appropriate protection
- Inconsistent `prevent_destroy` usage across similar resources suggests a design guideline gap
- Resources that should use `create_before_destroy` for zero-downtime deployments

##### Security and Configuration
- Repeated hardcoded values across files should be variables or locals
- Resource configurations missing encryption/TLS/access controls should be hardened
- Provider version constraints should be consistent across the codebase

##### Maintainability
- Long conditional expressions in resource properties should use locals
- Dynamic blocks obscuring resource structure should be simplified or documented
- `try()` / `can()` patterns should be replaced with explicit validation where possible

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
