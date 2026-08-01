#### Composer.json Refactoring Rules

##### Dependency Management
- Wildcard version constraints should use explicit compatible ranges
- Inconsistent version constraints for the same package across `require` and `require-dev`
- A package used in production code but only declared in `require-dev`
- Missing `ext-*` dependencies for extensions used in the code

##### Autoloading Organization
- PSR-4 namespace prefixes with incorrect or overlapping paths should be fixed
- Moved classes without updated autoload configuration
- `autoload.files` entries that execute side effects or conflict at bootstrap
- Classes accessible only through `autoload-dev` should be in production autoload if used in production

##### Scripts and Commands
- Lifecycle scripts that run destructive or environment-dependent commands without guard
- Scripts referencing tools not declared in dependencies
- Repeated script patterns that could be consolidated into a single command

##### Duplication
- Repeated dependency declarations across related projects suggest a shared metapackage
- Repeated autoload patterns suggest a namespace reorganization
- Repeated script configurations across related projects

##### Dead Config
- Unused autoload entries for removed directories or classes
- Unused scripts and plugin references
- Stale `replace`, `provide`, or `conflict` declarations

##### Structure and Metadata
- `minimum-stability` settings allowing unintended development packages without `prefer-stable`
- Missing or incorrect `type`, `license`, `description` for published packages
- Inconsistent `config.platform` settings with actual deployment platform

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
