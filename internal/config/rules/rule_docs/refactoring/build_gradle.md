#### Gradle Build Refactoring Rules

##### Build Script Organization
- Large `build.gradle` files with >200 lines should be split into `buildSrc` conventions or included builds
- Repeated dependency versions should use version catalogs (`libs.versions.toml`) or version properties
- Repeated plugin configurations across subprojects should use `subprojects {}` or convention plugins
- Complex task definitions should be extracted to standalone task classes or scripts

##### Dependency Management
- Inconsistent version declarations for the same dependency across modules should use a central version catalog
- Direct dependency coordinates repeated across modules should be managed centrally
- Transitive dependency exclusions repeated across modules suggest a global resolution strategy
- `implementation` vs `api` vs `compileOnly` should correctly reflect the dependency boundary

##### Naming and Organization
- Module/project names should follow consistent naming conventions (kebab-case or dot-notation)
- Task names should clearly describe their purpose
- Configuration names should reflect their role in the build lifecycle
- Source set customizations should use descriptive names

##### Duplication
- Repeated plugin application blocks across subprojects should use `allprojects` or convention plugins
- Repeated task configuration should be extracted to shared methods or plugins
- Similar build logic across projects suggests a composite build or convention plugin
- Repeated repository declarations should be centralized in settings or init scripts

##### Dead Code
- Unused plugins, dependencies, and custom tasks should be removed
- Commented-out dependency blocks should be deleted
- Unused build script variables and ext properties
- Empty configurations and source sets

##### Simplicity and Readability
- Complex conditional build logic should be extracted to documented helper methods
- Long task doLast/doFirst blocks should be extracted to named methods
- Build scripts mixing Groovy and Kotlin DSL should be consistent

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
