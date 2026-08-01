#### YAML Refactoring Rules

##### Structure and Organization
- Large YAML files (>300 lines) should be split by logical section into separate files
- Deeply nested structures (>4 levels) should be flattened with anchors or split files
- Repeated sections should use YAML anchors/aliases or be extracted to included files
- Related keys should be grouped with explanatory comments

##### Naming
- Keys should use consistent casing throughout the file (snake_case or kebab-case recommended)
- Key names should clearly describe their values
- Avoid redundant key prefixes when the parent key already provides context
- Boolean-like keys should use consistent prefixes (`enable_`, `use_`, `allow_`)

##### Duplication
- Repeated configuration blocks should use anchors (`&name`) and aliases (`*name`)
- Repeated `<<:` merge patterns suggest extraction to shared defaults
- Similar sections across multiple YAML files suggest a shared base template

##### Dead Config
- Unused anchors and aliases should be removed
- Commented-out sections should be deleted
- Keys that are never referenced by consuming applications
- Empty mappings and sequences

##### Maintainability
- Overly complex merge patterns (`<<:` chains) should be simplified or documented
- Long string values should use block scalars (`|` or `>-`) for readability
- Multi-document files without clear document separators (`---`) should add them
- Inconsistent value formats for the same concept (booleans: `yes`/`true`, nulls: `~`/`null`)

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
