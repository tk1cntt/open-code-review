#### Pom.xml Refactoring Rules

##### Build Organization
- Large `pom.xml` files with >300 lines should consider extracting profiles or parent POM
- Repeated dependency version declarations should use `<dependencyManagement>` or properties
- Plugin configurations scattered across modules should be centralized in `<pluginManagement>`
- Repeated `<parent>` configurations suggest a missing BOM or parent POM hierarchy

##### Dependency Management
- Inconsistent scopes for the same dependency across modules
- Direct dependencies using `test` scope that should be `compile` or vice versa
- Unused declared dependencies and plugins
- Transitive dependency exclusions repeated across modules

##### Duplication
- Repeated `<dependency>` entries across POMs should be in parent `<dependencyManagement>`
- Repeated `<plugin>` configurations across modules should use `<pluginManagement>`
- Similar `<execution>` blocks should be consolidated
- Repeated `<properties>` across modules should be in parent POM

##### Dead Config
- Unused `<profile>` definitions should be removed
- Stale `<repository>` and `<pluginRepository>` entries
- Commented-out dependency blocks should be deleted
- Build plugins applied but never executing any goals

##### Structure and Clarity
- Properties for version numbers should use consistent naming: `<version.lib.>` or `<lib.version>`
- Module declarations (`<modules>`) should match actual project structure
- Maven coordinates (`groupId`, `artifactId`) should follow consistent conventions

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
