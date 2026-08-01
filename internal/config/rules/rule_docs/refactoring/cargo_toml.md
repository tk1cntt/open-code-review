#### Cargo.toml Refactoring Rules

##### Manifest Organization
- Large `Cargo.toml` files should separate dependencies, dev-dependencies, build-dependencies, and platform-specific deps clearly
- Repeated dependency features across workspace members should use workspace inheritance
- Feature flags scattered without clear grouping should be organized by capability

##### Dependency Management
- Wildcard dependency versions (`*`) should use explicit compatible ranges
- Unpinned git dependencies in production crates should have `rev`, `tag`, or documented policy
- Dependencies in wrong section (`[dependencies]` vs `[dev-dependencies]` vs `[build-dependencies]`)
- Unused dependencies should be removed

##### Feature Design
- Features should be additive; avoid features that disable behavior
- Optional dependencies without corresponding feature names should expose intentional features
- Default features should stay minimal for libraries; heavy optional integrations should be opt-in
- Feature names should clearly describe the capability they enable

##### Workspace Consistency
- Inconsistent version usage across workspace members should use workspace inheritance
- Duplicate dependency declarations across crate members suggest workspace-level management
- Inconsistent edition or resolver settings across workspace members

##### Dead Code and Metadata
- Unused feature flags and optional dependencies should be removed
- Unused `[[bin]]`, `[[example]]`, `[[test]]`, `[[bench]]` targets
- Missing `license`/`license-file`, `repository`, `description` in published crates
- `exclude`/`include` not configured when test fixtures or large assets are present

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
