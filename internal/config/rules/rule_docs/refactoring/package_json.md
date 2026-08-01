#### package.json Refactoring Rules

##### Dependency Management
- `latest` or `*` version ranges should use specific compatible ranges
- Dependencies duplicated in both `dependencies` and `devDependencies`
- Tools referenced in `scripts` but not declared in `devDependencies`
- Unused dependencies should be removed

##### Scripts Organization
- Complex one-liner scripts should be extracted to separate tool configuration or script files
- Repeated script patterns across projects suggest shared tooling or npm packages
- Scripts calling each other with inconsistent naming conventions
- Long script commands should use `--` flag separation clearly

##### Package Metadata
- Missing or inconsistent `type` field for ESM vs CJS projects
- `files` field not configured, potentially publishing unnecessary files
- Outdated `engines` declarations inconsistent with actual Node requirements
- Missing `exports`, `main`, `module`, `types` alignment for published packages

##### Duplication
- Repeated config sections (`eslintConfig`, `browserslist`, `jest`) that could be separate config files
- Repeated `workspaces` patterns suggesting missing root workspace config
- Duplicate dependency declarations in monorepo packages

##### Dead Config
- Unused npm scripts should be removed
- Stale configuration for removed tools or plugins
- Obsolete `engines` or `os`/`cpu` restrictions

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
