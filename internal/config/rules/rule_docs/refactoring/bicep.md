#### Bicep Refactoring Rules

##### Module and Resource Organization
- Large `.bicep` files with >200 lines should be decomposed into modules by resource group or concern
- Repeated resource patterns across files suggest a shared module with parameters
- Inline resource definitions used in multiple places should be extracted into modules
- Related parameters, variables, and outputs should be grouped logically

##### Naming and Clarity
- Parameter/variable/output names should clearly convey purpose using camelCase
- Symbolic names should describe the resource's role, not just its type
- `@description()` should be present on all parameters and outputs forming the module's public API
- Avoid stutter in symbolic names: `storageAccountStorageAccount` → `storageAccount`

##### Parameter Design
- Boolean flag parameters that switch behavior suggest splitting into two modules or parameters
- Repeated parameter patterns across modules suggest a common parameter interface
- Parameters with default values that vary by environment should use `@allowed()` or clear documentation
- Hardcoded values repeated in resource properties should be parameters or variables

##### Duplication
- Repeated resource configurations should be extracted to `@batchSize()` loops or modules
- Repeated `@description()` strings suggest extraction to constants or documentation files
- Similar child resource definitions should use resource iteration
- Repeated output expressions should use computed local variables

##### Dead Resources and Parameters
- Unused parameters, variables, and outputs should be removed
- Resources deployed but never referenced by outputs or other resources
- Commented-out resource blocks should be deleted

##### Security Hardening
- Resource defaults that disable encryption or TLS should be explicitly enabled
- Public network access on storage/key vault/SQL without documented need should be restricted
- Overly broad role assignments should be scoped to minimum required permissions
- Missing `@secure()` on credential-like parameters

##### Structure and Maintainability
- Long expressions in resource properties should be broken into variables
- Complex conditional deployments should be extracted to named variables
- Repeated `dependsOn` patterns suggest missing parent-child or implicit references

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
